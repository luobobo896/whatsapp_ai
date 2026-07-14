package middleware

import (
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"

	"whatsapp-ai-poc/internal/model"
	"whatsapp-ai-poc/internal/store"
)

type contextKey string

const (
	SessionKey contextKey = "session"
	StoreKey   contextKey = "store"
)

func GetSession(c *gin.Context) *model.Session {
	if s, ok := c.Get(SessionKey); ok {
		return s.(*model.Session)
	}
	return nil
}

func GetStore(c *gin.Context) *store.Store {
	return c.MustGet(StoreKey).(*store.Store)
}

func Auth(store *store.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(StoreKey, store)

		cookie, err := c.Cookie("session_id")
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": model.ErrorDetail{Code: "AUTH_REQUIRED", Message: "Authentication is required."}})
			return
		}

		sess, err := store.SessionByID(cookie)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": model.ErrorDetail{Code: "SESSION_EXPIRED", Message: "Session expired."}})
			return
		}

		user, err := store.UserByID(sess.UserID)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": model.ErrorDetail{Code: "AUTH_REQUIRED", Message: "User not found."}})
			return
		}

		c.Set(SessionKey, &model.Session{
			CSRFToken:      sess.CSRFToken,
			ActiveTenantID: sess.ActiveTenantID,
			User: model.User{
				ID:           user.ID,
				Email:        user.Email,
				DisplayName:  user.DisplayName,
				PlatformRole: user.PlatformRole,
			},
		})
		c.Next()
	}
}

// RequireCSRF checks the X-CSRF-Token header for mutating requests.
func RequireCSRF() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == "GET" || c.Request.Method == "HEAD" || c.Request.Method == "OPTIONS" {
			c.Next()
			return
		}
		session := GetSession(c)
		if session == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": model.ErrorDetail{Code: "AUTH_REQUIRED", Message: "Authentication is required."}})
			return
		}
		token := c.GetHeader("X-CSRF-Token")
		if token == "" || token != session.CSRFToken {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": model.ErrorDetail{Code: "FORBIDDEN", Message: "Invalid CSRF token."}})
			return
		}
		c.Next()
	}
}

// RequireTenant ensures the session has an active tenant selected.
func RequireTenant() gin.HandlerFunc {
	return func(c *gin.Context) {
		session := GetSession(c)
		if session == nil || session.ActiveTenantID == "" {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "TENANT_REQUIRED", Message: "No tenant selected."}})
			return
		}
		c.Next()
	}
}

// RequireActiveTenant ensures the selected tenant and the user's membership
// are both active before accessing tenant-scoped resources.
func RequireActiveTenant() gin.HandlerFunc {
	return func(c *gin.Context) {
		session := GetSession(c)
		if session == nil || session.ActiveTenantID == "" {
			c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "TENANT_REQUIRED", Message: "No tenant selected."}})
			return
		}
		st := GetStore(c)
		tenant, err := st.TenantByID(session.ActiveTenantID)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": model.ErrorDetail{Code: "FORBIDDEN", Message: "Tenant is unavailable."}})
			return
		}
		if tenant.Status != "active" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": model.ErrorDetail{Code: "TENANT_SUSPENDED", Message: "Tenant is suspended."}})
			return
		}
		member, err := st.TenantMember(session.ActiveTenantID, session.User.ID)
		if err != nil || member.Status != "active" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": model.ErrorDetail{Code: "FORBIDDEN", Message: "Membership is not active."}})
			return
		}
		c.Next()
	}
}

// RequirePlatformAdmin ensures the user is a platform admin.
func RequirePlatformAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		session := GetSession(c)
		if session == nil || !session.IsPlatformAdmin() {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": model.ErrorDetail{Code: "FORBIDDEN", Message: "Platform admin required."}})
			return
		}
		c.Next()
	}
}

// InternalAuth checks for a valid bearer token (INTERNAL_API_TOKEN env var).
// When valid, it injects a synthetic session using the tenant derived from the
// request body or query. This is intended for internal service-to-service calls
// (e.g. MCP server → backend API).
func InternalAuth(store *store.Store) gin.HandlerFunc {
	expectedToken := strings.TrimSpace(os.Getenv("INTERNAL_API_TOKEN"))
	return func(c *gin.Context) {
		c.Set(StoreKey, store)

		if expectedToken == "" {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "CONFIG_ERROR", Message: "Internal API token not configured."}})
			return
		}

		auth := c.GetHeader("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") || strings.TrimPrefix(auth, "Bearer ") != expectedToken {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": model.ErrorDetail{Code: "AUTH_REQUIRED", Message: "Invalid internal token."}})
			return
		}

		// Build a minimal synthetic session. Tenant will be resolved per-handler
		// from the request (e.g. accountId → tenantId), but we need a non-nil
		// session so downstream handlers don't bail out.
		c.Set(SessionKey, &model.Session{
			CSRFToken:      "internal",
			ActiveTenantID: "internal", // placeholder; handlers resolve real tenant
			User: model.User{
				ID:           "internal",
				Email:        "internal@whatsapp-ai.local",
				DisplayName:  "Internal Service",
				PlatformRole: "platform_admin",
			},
		})
		c.Next()
	}
}

// RequireTenantPermission returns middleware that checks for a specific permission.
func RequireTenantPermission(perm string) gin.HandlerFunc {
	return func(c *gin.Context) {
		session := GetSession(c)
		if session == nil || session.ActiveTenantID == "" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": model.ErrorDetail{Code: "FORBIDDEN", Message: "No tenant selected."}})
			return
		}
		st := GetStore(c)
		member, err := st.TenantMember(session.ActiveTenantID, session.User.ID)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": model.ErrorDetail{Code: "FORBIDDEN", Message: "Not a member of this tenant."}})
			return
		}
		if member.Status != "active" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": model.ErrorDetail{Code: "FORBIDDEN", Message: "Membership is not active."}})
			return
		}
		for _, p := range model.PermissionsForRole(member.Role) {
			if p == perm {
				c.Next()
				return
			}
		}
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": model.ErrorDetail{Code: "FORBIDDEN", Message: "Insufficient permissions."}})
	}
}
