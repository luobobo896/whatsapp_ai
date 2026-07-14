package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"whatsapp-ai-poc/internal/middleware"
	"whatsapp-ai-poc/internal/model"
	"whatsapp-ai-poc/internal/store"
)

func HandleLogin(st *store.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req model.LoginRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "INVALID_INPUT", Message: "Invalid request."}})
			return
		}
		user, err := st.UserByEmail(req.Email)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": model.ErrorDetail{Code: "AUTH_INVALID", Message: "邮箱或密码不正确。"}})
			return
		}
		if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)) != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": model.ErrorDetail{Code: "AUTH_INVALID", Message: "邮箱或密码不正确。"}})
			return
		}
		sess, err := st.CreateSession(user.ID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "INTERNAL", Message: "Failed to create session."}})
			return
		}
		c.SetCookie("session_id", sess.ID, 86400, "/", "", false, true)
		c.Status(http.StatusNoContent)
	}
}

func HandleLogout(st *store.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		cookie, err := c.Cookie("session_id")
		if err == nil {
			st.DeleteSession(cookie)
		}
		c.SetCookie("session_id", "", -1, "/", "", false, true)
		c.Status(http.StatusNoContent)
	}
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
