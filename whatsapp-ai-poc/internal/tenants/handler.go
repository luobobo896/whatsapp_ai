package tenants

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"whatsapp-ai-poc/internal/audit"
	"whatsapp-ai-poc/internal/auth"
	"whatsapp-ai-poc/internal/members"
	"whatsapp-ai-poc/internal/platform/apperror"
	"whatsapp-ai-poc/internal/platform/config"
	"whatsapp-ai-poc/internal/platform/httpx"
)

type handler struct{ service *Service }

func RegisterRoutes(router *gin.Engine, cfg config.Config, pool *pgxpool.Pool, service *Service) {
	h := &handler{service: service}
	router.GET("/api/tenants",
		auth.Authenticate(pool, cfg.SessionCookieName), httpx.Adapt(h.listAccessible),
	)
	router.POST("/api/platform/tenants",
		auth.Authenticate(pool, cfg.SessionCookieName), auth.RequireMutation(cfg),
		members.RequirePlatformAdmin(pool), httpx.Adapt(h.create),
	)
	router.PATCH("/api/platform/tenants/:tenantId/status",
		auth.Authenticate(pool, cfg.SessionCookieName), auth.RequireMutation(cfg),
		members.RequirePlatformAdmin(pool), httpx.Adapt(h.setStatus),
	)
}

func (h *handler) listAccessible(c *gin.Context) error {
	identity, _ := auth.IdentityFrom(c)
	result, platformRole, err := h.service.ListAccessible(c.Request.Context(), identity.UserID)
	if err != nil {
		return err
	}
	c.JSON(http.StatusOK, gin.H{"tenants": result, "platformRole": platformRole})
	return nil
}

func (h *handler) create(c *gin.Context) error {
	var input struct {
		Name             string `json:"name"`
		Slug             string `json:"slug"`
		OwnerEmail       string `json:"ownerEmail"`
		OwnerDisplayName string `json:"ownerDisplayName"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		return apperror.Validation("Valid tenant and owner details are required.", nil)
	}
	result, err := h.service.Create(c.Request.Context(), platformActor(c), CreateInput{
		Name: input.Name, Slug: input.Slug, OwnerEmail: input.OwnerEmail, OwnerDisplayName: input.OwnerDisplayName,
	})
	if err != nil {
		return err
	}
	c.JSON(http.StatusCreated, gin.H{"tenant": result.Tenant, "invitation": result.Invitation})
	return nil
}

func (h *handler) setStatus(c *gin.Context) error {
	tenantID, err := uuid.Parse(c.Param("tenantId"))
	if err != nil {
		return apperror.Validation("A valid tenantId is required.", nil)
	}
	var input struct {
		Status string `json:"status"`
		Reason string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		return apperror.Validation("A valid status is required.", nil)
	}
	if err := h.service.SetStatus(c.Request.Context(), platformActor(c), tenantID, input.Status, input.Reason); err != nil {
		return err
	}
	c.Status(http.StatusNoContent)
	return nil
}

func platformActor(c *gin.Context) audit.Actor {
	identity, _ := auth.IdentityFrom(c)
	return audit.Actor{
		UserID: identity.UserID, Role: "platform_admin", RequestID: httpx.RequestIDFrom(c),
		IP: c.ClientIP(), UserAgent: c.Request.UserAgent(),
	}
}
