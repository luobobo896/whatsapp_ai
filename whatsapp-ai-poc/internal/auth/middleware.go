package auth

import (
	"errors"
	"net/netip"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/time/rate"

	"whatsapp-ai-poc/internal/platform/apperror"
	"whatsapp-ai-poc/internal/platform/config"
	"whatsapp-ai-poc/internal/platform/httpx"
)

const identityKey = "auth_identity"

func Authenticate(pool *pgxpool.Pool, cookieName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		rawToken, err := c.Cookie(cookieName)
		if err != nil || rawToken == "" {
			httpx.WriteError(c, apperror.AuthRequired())
			return
		}
		identity, err := Resolve(c.Request.Context(), pool, rawToken)
		if errors.Is(err, ErrSessionNotFound) {
			httpx.WriteError(c, apperror.AuthRequired())
			return
		}
		if errors.Is(err, ErrSessionExpired) {
			httpx.WriteError(c, apperror.SessionExpired())
			return
		}
		if err != nil {
			httpx.WriteError(c, err)
			return
		}
		c.Set(identityKey, identity)
		c.Next()
	}
}

func RequireMutation(cfg config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		identity, ok := IdentityFrom(c)
		if !ok || cfg.AppOrigin == nil || c.GetHeader("Origin") != cfg.AppOrigin.String() ||
			!verifyCSRF(identity, c.GetHeader("X-CSRF-Token")) {
			httpx.WriteError(c, apperror.Forbidden())
			return
		}
		c.Next()
	}
}

func IdentityFrom(c *gin.Context) (Identity, bool) {
	value, ok := c.Get(identityKey)
	if !ok {
		return Identity{}, false
	}
	identity, ok := value.(Identity)
	return identity, ok
}

type limiterEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

type LoginLimiter struct {
	mu      sync.Mutex
	entries map[string]*limiterEntry
	limit   int
	window  time.Duration
	maximum int
}

func NewLoginLimiter(limit int, window time.Duration) *LoginLimiter {
	if limit < 1 {
		limit = 5
	}
	if window <= 0 {
		window = time.Minute
	}
	return &LoginLimiter{entries: make(map[string]*limiterEntry), limit: limit, window: window, maximum: 10_000}
}

func (limits *LoginLimiter) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !limits.allow(normalizedClientIP(c.ClientIP())) {
			c.Header("Retry-After", strconv.Itoa(max(1, int(limits.window.Seconds()))))
			httpx.WriteError(c, apperror.RateLimited())
			return
		}
		c.Next()
	}
}

func (limits *LoginLimiter) allow(key string) bool {
	now := time.Now()
	limits.mu.Lock()
	defer limits.mu.Unlock()
	if len(limits.entries) >= limits.maximum {
		limits.prune(now)
	}
	entry := limits.entries[key]
	if entry == nil {
		if len(limits.entries) >= limits.maximum {
			limits.removeOldest()
		}
		interval := limits.window / time.Duration(limits.limit)
		entry = &limiterEntry{limiter: rate.NewLimiter(rate.Every(interval), limits.limit)}
		limits.entries[key] = entry
	}
	entry.lastSeen = now
	return entry.limiter.Allow()
}

func (limits *LoginLimiter) prune(now time.Time) {
	for key, entry := range limits.entries {
		if now.Sub(entry.lastSeen) > 2*limits.window {
			delete(limits.entries, key)
		}
	}
}

func (limits *LoginLimiter) removeOldest() {
	var oldestKey string
	var oldest time.Time
	for key, entry := range limits.entries {
		if oldestKey == "" || entry.lastSeen.Before(oldest) {
			oldestKey, oldest = key, entry.lastSeen
		}
	}
	delete(limits.entries, oldestKey)
}

func normalizedClientIP(value string) string {
	address, err := netip.ParseAddr(value)
	if err != nil {
		return "unknown"
	}
	return address.Unmap().String()
}
