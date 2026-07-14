package handler

import (
	"crypto/rand"
	"net/http"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"whatsapp-ai-poc/internal/middleware"
	"whatsapp-ai-poc/internal/model"
	"whatsapp-ai-poc/internal/store"
)

func RegisterTenants(r *gin.RouterGroup, st *store.Store) {
	r.GET("", handleListTenants(st))
}

func RegisterPlatformTenants(r *gin.RouterGroup, st *store.Store) {
	r.POST("", handleCreateTenant(st))
	r.PATCH("/:id/status", handleUpdateTenantStatus(st))
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
			userTenants, err := st.TenantsForUser(session.User.ID)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "INTERNAL", Message: "Failed to load tenant memberships."}})
				return
			}
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
		session := middleware.GetSession(c)
		if session == nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": model.ErrorDetail{Code: "AUTH_REQUIRED", Message: "Authentication is required."}})
			return
		}
		var req model.CreateTenantRequest
		if err := c.ShouldBindJSON(&req); err != nil || req.Name == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": model.ErrorDetail{Code: "INVALID_INPUT", Message: "Tenant name is required."}})
			return
		}
		// Generate admin credentials before creating the tenant transaction.
		password := randomPassword(16)
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "INTERNAL", Message: "Failed to hash password."}})
			return
		}
		tenant, owner, err := st.CreateTenantWithOwner(req.Name, string(hash), session.User.ID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": model.ErrorDetail{Code: "INTERNAL", Message: "Failed to create tenant."}})
			return
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
		resp.Credentials.Email = owner.Email
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
	for i := 0; i < n; {
		var randomByte [1]byte
		if _, err := rand.Read(randomByte[:]); err != nil {
			panic("cryptographic random source unavailable")
		}
		limit := byte(256 - (256 % len(letters)))
		if randomByte[0] >= limit {
			continue
		}
		b[i] = letters[int(randomByte[0])%len(letters)]
		i++
	}
	return string(b)
}
