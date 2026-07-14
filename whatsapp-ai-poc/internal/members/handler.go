package members

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"whatsapp-ai-poc/internal/auth"
	"whatsapp-ai-poc/internal/platform/apperror"
	"whatsapp-ai-poc/internal/platform/config"
	"whatsapp-ai-poc/internal/platform/httpx"
)

type handler struct {
	cfg     config.Config
	service *Service
}

func RegisterRoutes(router *gin.Engine, cfg config.Config, pool *pgxpool.Pool, service *Service) {
	h := &handler{cfg: cfg, service: service}
	router.GET("/api/members",
		auth.Authenticate(pool, cfg.SessionCookieName),
		RequirePermission(pool, PermissionMembersRead),
		httpx.Adapt(h.list),
	)
	router.POST("/api/members/invitations",
		auth.Authenticate(pool, cfg.SessionCookieName), auth.RequireMutation(cfg),
		RequirePermission(pool, PermissionMembersManage), httpx.Adapt(h.invite),
	)
	router.PATCH("/api/members/:userId",
		auth.Authenticate(pool, cfg.SessionCookieName), auth.RequireMutation(cfg),
		RequirePermission(pool, PermissionMembersManage), httpx.Adapt(h.update),
	)
	acceptLimits := auth.NewLoginLimiter(cfg.LoginRateLimit, cfg.LoginRateWindow)
	router.POST("/api/invitations/:token/accept", acceptLimits.Middleware(), httpx.Adapt(h.accept))
}

func (h *handler) list(c *gin.Context) error {
	tenant, _ := TenantFrom(c)
	result, err := h.service.List(c.Request.Context(), tenant)
	if err != nil {
		return err
	}
	c.JSON(http.StatusOK, gin.H{"members": result})
	return nil
}

func (h *handler) invite(c *gin.Context) error {
	var input struct {
		Email string `json:"email"`
		Role  Role   `json:"role"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		return apperror.Validation("A valid email and role are required.", nil)
	}
	tenant, _ := TenantFrom(c)
	result, err := h.service.Invite(c.Request.Context(), tenant, InviteInput{Email: input.Email, Role: input.Role})
	if err != nil {
		return err
	}
	c.JSON(http.StatusCreated, gin.H{"invitation": result})
	return nil
}

func (h *handler) accept(c *gin.Context) error {
	var input struct {
		Email       string `json:"email"`
		DisplayName string `json:"displayName"`
		Password    string `json:"password"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		return apperror.Validation("Valid invitation details are required.", nil)
	}
	result, err := h.service.Accept(c.Request.Context(), c.Param("token"), AcceptInput{
		Email: input.Email, DisplayName: input.DisplayName, Password: input.Password,
		RequestID: httpx.RequestIDFrom(c), IP: c.ClientIP(), UserAgent: c.Request.UserAgent(),
	})
	if err != nil {
		return err
	}
	auth.SetSessionCookie(c, h.cfg, result.Tokens.SessionToken, int(h.cfg.SessionTTL.Seconds()))
	c.JSON(http.StatusCreated, gin.H{
		"csrfToken": result.Tokens.CSRFToken,
		"tenantId":  result.TenantID,
		"user":      gin.H{"id": result.UserID},
	})
	return nil
}

func (h *handler) update(c *gin.Context) error {
	userID, err := uuid.Parse(c.Param("userId"))
	if err != nil {
		return apperror.Validation("A valid userId is required.", nil)
	}
	var input struct {
		Role   Role   `json:"role"`
		Status string `json:"status"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		return apperror.Validation("A valid role and status are required.", nil)
	}
	tenant, _ := TenantFrom(c)
	if err := h.service.Update(c.Request.Context(), tenant, userID, UpdateInput{Role: input.Role, Status: input.Status}); err != nil {
		return err
	}
	c.Status(http.StatusNoContent)
	return nil
}
