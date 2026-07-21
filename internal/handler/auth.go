package handler

import (
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"whatsapp-ai-poc/internal/middleware"
	"whatsapp-ai-poc/internal/model"
	"whatsapp-ai-poc/internal/store"
)

// bcryptCost controls the cost for newly-generated password hashes. It is
// intentionally higher than bcrypt.DefaultCost (10) to slow offline brute
// force. Existing stored hashes remain verifiable regardless of their own
// cost because bcrypt hashes are self-describing.
const bcryptCost = 12

// loginRateLimit caps how many failed login attempts a single subject (IP or
// email) may accumulate within the sliding window before being locked out. The
// thresholds are intentionally conservative: a coordinated attacker can still
// try a handful of passwords, but automated brute force is stopped cold without
// needing to spin up external rate-limit infrastructure. Counts live in-process
// (single replica) which matches the current deployment topology.
const (
	loginFailureWindow    = 5 * time.Minute
	loginFailureThreshold = 5
	loginLockoutDuration  = 15 * time.Minute
	loginRateLimitErrCode = "AUTH_RATE_LIMITED"
)

// loginAttempt tracks the per-subject failure history. Successful logins clear
// the window so a user who mistypes once is not penalised for it later.
type loginAttempt struct {
	mu       sync.Mutex
	failures map[string][]time.Time
}

var loginLimiter = &loginAttempt{failures: make(map[string][]time.Time)}

// recordLoginFailure appends the current timestamp to the subject's recent
// failure list, trimming entries outside the sliding window so memory does not
// grow unbounded for dormant subjects.
func (l *loginAttempt) recordLoginFailure(subject string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	cutoff := now.Add(-loginFailureWindow)
	fails := l.failures[subject]
	pruned := fails[:0]
	for _, t := range fails {
		if t.After(cutoff) {
			pruned = append(pruned, t)
		}
	}
	pruned = append(pruned, now)
	l.failures[subject] = pruned
}

// clearLoginFailures resets the subject on a successful auth so a user who
// eventually types the right password is not left with a half-full window.
func (l *loginAttempt) clearLoginFailures(subject string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.failures, subject)
}

// recentFailureCount returns the number of failures within the sliding window
// for the given subject, pruning expired entries in place.
func (l *loginAttempt) recentFailureCount(subject string, now time.Time) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	cutoff := now.Add(-loginFailureWindow)
	fails := l.failures[subject]
	pruned := fails[:0]
	for _, t := range fails {
		if t.After(cutoff) {
			pruned = append(pruned, t)
		}
	}
	l.failures[subject] = pruned
	return len(pruned)
}

// loginRateLimitSubjects returns the keys we count failures against: the
// presented email (covers one account being brute-forced from many IPs) and
// the client IP (covers one botnet node trying many accounts). Either being
// over threshold locks the request out.
func loginRateLimitSubjects(c *gin.Context, email string) []string {
	ip := c.ClientIP()
	if ip == "" {
		ip = "unknown"
	}
	subjects := []string{"ip:" + ip}
	if normalized := strings.ToLower(strings.TrimSpace(email)); normalized != "" {
		subjects = append(subjects, "email:"+normalized)
	}
	return subjects
}

// enforceLoginRateLimit returns true when the caller is allowed to proceed
// with password verification. When false, the request has already been
// rejected with 429.
func enforceLoginRateLimit(c *gin.Context, email string) bool {
	now := time.Now()
	for _, subject := range loginRateLimitSubjects(c, email) {
		if loginLimiter.recentFailureCount(subject, now) >= loginFailureThreshold {
			slog.Warn("login rate limit engaged",
				"subject", subject,
				"ip", c.ClientIP(),
				"email", email,
				"lockout", loginLockoutDuration.String(),
			)
			c.JSON(http.StatusTooManyRequests, gin.H{"error": model.ErrorDetail{
				Code:    loginRateLimitErrCode,
				Message: "登录尝试过于频繁，请稍后再试。",
			}})
			return false
		}
	}
	return true
}

// HashPassword generates a bcrypt hash at the package-configured cost. It is
// the single entry point for new password hashing so the cost cannot drift
// between the seed, accept-invitation, and create-tenant call sites.
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func HandleLogin(st *store.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req model.LoginRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "INVALID_INPUT", Message: "Invalid request."}})
			return
		}
		// Pre-check rate limit BEFORE bcrypt verification so a locked-out
		// subject cannot trigger the expensive password check (which is the
		// CPU-amplification an attacker would otherwise abuse).
		if !enforceLoginRateLimit(c, req.Email) {
			return
		}
		user, err := st.UserByEmail(req.Email)
		if err != nil {
			loginRecordFailure(c, req.Email)
			c.JSON(http.StatusUnauthorized, gin.H{"error": model.ErrorDetail{Code: "AUTH_INVALID", Message: "邮箱或密码不正确。"}})
			return
		}
		if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)) != nil {
			loginRecordFailure(c, req.Email)
			c.JSON(http.StatusUnauthorized, gin.H{"error": model.ErrorDetail{Code: "AUTH_INVALID", Message: "邮箱或密码不正确。"}})
			return
		}
		// Successful auth: clear the failure window for this subject so a
		// legitimate user who mistyped a few times is not left near the
		// threshold.
		for _, subject := range loginRateLimitSubjects(c, req.Email) {
			loginLimiter.clearLoginFailures(subject)
		}
		sess, err := st.CreateSession(user.ID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "INTERNAL", Message: "Failed to create session."}})
			return
		}
		c.SetSameSite(http.SameSiteLaxMode)
		c.SetCookie("session_id", sess.ID, 86400, "/", "", sessionCookieSecure(), true)
		c.Status(http.StatusNoContent)
	}
}

// loginRecordFailure is a small helper that records a failed attempt against
// every subject that applies to this request (both IP and email) using the
// same timestamp so the sliding window advances consistently.
func loginRecordFailure(c *gin.Context, email string) {
	now := time.Now()
	for _, subject := range loginRateLimitSubjects(c, email) {
		loginLimiter.recordLoginFailure(subject, now)
	}
}

func HandleLogout(st *store.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		cookie, err := c.Cookie("session_id")
		if err == nil {
			if err := st.DeleteSession(cookie); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "INTERNAL", Message: "Failed to end session."}})
				return
			}
		}
		c.SetSameSite(http.SameSiteLaxMode)
		c.SetCookie("session_id", "", -1, "/", "", sessionCookieSecure(), true)
		c.Status(http.StatusNoContent)
	}
}

func sessionCookieSecure() bool {
	return strings.ToLower(strings.TrimSpace(os.Getenv("COOKIE_SECURE"))) != "false"
}

func HandleMe(st *store.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		session := middleware.GetSession(c)
		if session == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": model.ErrorDetail{Code: "AUTH_REQUIRED", Message: "Authentication is required."}})
			return
		}
		c.JSON(http.StatusOK, session)
	}
}

func HandleSelectTenant(st *store.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		session := middleware.GetSession(c)
		if session == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": model.ErrorDetail{Code: "AUTH_REQUIRED", Message: "Authentication is required."}})
			return
		}
		var req model.SelectTenantRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "INVALID_INPUT", Message: "Invalid request."}})
			return
		}
		member, err := st.TenantMember(req.TenantID, session.User.ID)
		if err != nil || member.Status != "active" {
			c.JSON(http.StatusForbidden, gin.H{"error": model.ErrorDetail{Code: "FORBIDDEN", Message: "Not a member of this tenant."}})
			return
		}
		tenant, err := st.TenantByID(req.TenantID)
		if err != nil || tenant.Status != "active" {
			c.JSON(http.StatusForbidden, gin.H{"error": model.ErrorDetail{Code: "TENANT_SUSPENDED", Message: "Tenant is suspended."}})
			return
		}
		cookie, err := c.Cookie("session_id")
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": model.ErrorDetail{Code: "AUTH_REQUIRED", Message: "No session cookie."}})
			return
		}
		if err := st.UpdateSessionTenant(cookie, req.TenantID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "INTERNAL", Message: "Failed to update session."}})
			return
		}
		c.Status(http.StatusNoContent)
	}
}
