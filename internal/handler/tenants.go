package handler

import (
	"fmt"
	"math/rand"
	"net/http"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"whatsapp-ai-poc/internal/middleware"
	"whatsapp-ai-poc/internal/model"
	"whatsapp-ai-poc/internal/store"
)

func RegisterTenants(r *gin.RouterGroup, st *store.Store) {
	r.GET("", handleListTenants(st))
	r.POST("/:id/status", handleUpdateTenantStatus(st))
}

func RegisterPlatformTenants(r *gin.RouterGroup, st *store.Store) {
	r.POST("", handleCreateTenant(st))
}

func handleListTenants(st *store.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		session := middleware.GetSession(c)
		if session == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": model.ErrorDetail{Code: "AUTH_REQUIRED", Message: "Authentication is required."}})
			return
		}

		var tenants []model.TenantWithMembership
		if session.IsPlatformAdmin() {
			all, err := st.AllTenants()
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "INTERNAL", Message: "Failed to load tenants."}})
				return
			}
			// Platform admin sees all tenants without membership info
			// But also needs to see their own memberships
			userTenants, _ := st.TenantsForUser(session.User.ID)
			userTenantMap := make(map[string]model.TenantWithMembership)
			for _, t := range userTenants {
				userTenantMap[t.ID] = t
			}
			for _, t := range all {
				if ut, ok := userTenantMap[t.ID]; ok {
					tenants = append(tenants, ut)
				} else {
					tenants = append(tenants, t)
				}
			}
		} else {
			var err error
			tenants, err = st.TenantsForUser(session.User.ID)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "INTERNAL", Message: "Failed to load tenants."}})
				return
			}
			if tenants == nil {
				tenants = []model.TenantWithMembership{}
			}
		}

		c.JSON(http.StatusOK, model.TenantsResponse{
			PlatformRole: session.User.PlatformRole,
			Tenants:      tenants,
		})
	}
}

func handleCreateTenant(st *store.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req model.CreateTenantRequest
		if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "INVALID_INPUT", Message: "Tenant name is required."}})
			return
		}
		// Create tenant
		tenant, err := st.CreateTenant(req.Name)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "INTERNAL", Message: "Failed to create tenant."}})
			return
		}
		// Generate admin credentials
		password := randomPassword(16)
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "INTERNAL", Message: "Failed to hash password."}})
			return
		}
		email := fmt.Sprintf("admin@%s.local", tenant.ID[:8])
		user, err := st.CreateUser(email, "管理员", string(hash), "")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "INTERNAL", Message: "Failed to create admin user."}})
			return
		}
		// Add tenant admin as owner
		if err := st.AddTenantMember(tenant.ID, user.ID, "owner"); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "INTERNAL", Message: "Failed to add tenant member."}})
			return
		}
		// Also add current platform admin as owner
		session := middleware.GetSession(c)
		if session != nil {
			st.AddTenantMember(tenant.ID, session.User.ID, "owner")
		}

		resp := model.CreateTenantResponse{
			Tenant: model.TenantWithMembership{
				ID:     tenant.ID,
				Name:   tenant.Name,
				Status: tenant.Status,
				Role:   "owner",
				Permissions: model.PermissionsForRole("owner"),
			},
		}
		resp.Credentials.Email = email
		resp.Credentials.Password = password

		c.JSON(http.StatusOK, resp)
	}
}

func handleUpdateTenantStatus(st *store.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenantID := c.Param("id")
		var req model.UpdateTenantStatusRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "INVALID_INPUT", Message: "Invalid request."}})
			return
		}
		if req.Status != "active" && req.Status != "suspended" {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "INVALID_INPUT", Message: "Status must be active or suspended."}})
			return
		}
		if _, err := st.TenantByID(tenantID); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": model.ErrorDetail{Code: "NOT_FOUND", Message: "Tenant not found."}})
			return
		}
		if err := st.UpdateTenantStatus(tenantID, req.Status); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "INTERNAL", Message: "Failed to update tenant."}})
			return
		}
		c.Status(http.StatusNoContent)
	}
}

func randomPassword(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}
