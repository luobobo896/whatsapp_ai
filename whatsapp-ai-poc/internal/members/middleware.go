package members

import (
	"errors"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"whatsapp-ai-poc/internal/auth"
	"whatsapp-ai-poc/internal/platform/apperror"
	"whatsapp-ai-poc/internal/platform/database"
	"whatsapp-ai-poc/internal/platform/httpx"
)

const tenantContextKey = "tenant_context"

type TenantContext struct {
	TenantID  uuid.UUID
	UserID    uuid.UUID
	Role      Role
	RequestID string
	IP        string
	UserAgent string
}

func RequirePlatformAdmin(pool *pgxpool.Pool) gin.HandlerFunc {
	return func(c *gin.Context) {
		identity, ok := auth.IdentityFrom(c)
		if !ok {
			httpx.WriteError(c, apperror.AuthRequired())
			return
		}
		var role string
		err := pool.QueryRow(c.Request.Context(),
			"SELECT role FROM platform_roles WHERE user_id = $1", identity.UserID,
		).Scan(&role)
		if errors.Is(err, pgx.ErrNoRows) {
			httpx.WriteError(c, apperror.Forbidden())
			return
		}
		if err != nil {
			httpx.WriteError(c, err)
			return
		}
		if role != "platform_admin" {
			httpx.WriteError(c, apperror.Forbidden())
			return
		}
		c.Next()
	}
}

func RequirePermission(pool *pgxpool.Pool, permission Permission) gin.HandlerFunc {
	return func(c *gin.Context) {
		identity, ok := auth.IdentityFrom(c)
		if !ok {
			httpx.WriteError(c, apperror.AuthRequired())
			return
		}
		if identity.ActiveTenantID == nil {
			httpx.WriteError(c, apperror.Forbidden())
			return
		}

		tenant, err := database.WithTenantTx(c.Request.Context(), pool, *identity.ActiveTenantID, func(tx pgx.Tx) (TenantContext, error) {
			var tenantStatus, membershipStatus string
			var role Role
			err := tx.QueryRow(c.Request.Context(), `
				SELECT t.status, m.status, m.role
				FROM tenants t
				JOIN tenant_memberships m ON m.tenant_id = t.id
				WHERE t.id = $1 AND m.user_id = $2
			`, *identity.ActiveTenantID, identity.UserID).Scan(&tenantStatus, &membershipStatus, &role)
			if errors.Is(err, pgx.ErrNoRows) {
				return TenantContext{}, apperror.Forbidden()
			}
			if err != nil {
				return TenantContext{}, err
			}
			if tenantStatus == "suspended" {
				return TenantContext{}, apperror.TenantSuspended()
			}
			if membershipStatus != "active" || !HasPermission(role, permission) {
				return TenantContext{}, apperror.Forbidden()
			}
			return TenantContext{
				TenantID: *identity.ActiveTenantID, UserID: identity.UserID, Role: role,
				RequestID: httpx.RequestIDFrom(c), IP: c.ClientIP(), UserAgent: c.Request.UserAgent(),
			}, nil
		})
		if err != nil {
			httpx.WriteError(c, err)
			return
		}
		c.Set(tenantContextKey, tenant)
		c.Next()
	}
}

func TenantFrom(c *gin.Context) (TenantContext, bool) {
	value, ok := c.Get(tenantContextKey)
	if !ok {
		return TenantContext{}, false
	}
	tenant, ok := value.(TenantContext)
	return tenant, ok
}
