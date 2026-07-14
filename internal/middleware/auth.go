package middleware

import (
	"net/http"

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
