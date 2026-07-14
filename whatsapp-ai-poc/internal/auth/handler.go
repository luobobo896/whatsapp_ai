package auth

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"whatsapp-ai-poc/internal/platform/apperror"
	"whatsapp-ai-poc/internal/platform/config"
	"whatsapp-ai-poc/internal/platform/database"
	"whatsapp-ai-poc/internal/platform/httpx"
)

type handler struct {
	cfg       config.Config
	pool      *pgxpool.Pool
	dummyHash string
}

func RegisterRoutes(router *gin.Engine, cfg config.Config, pool *pgxpool.Pool) {
	dummyHash, err := HashPassword("invalid-password-placeholder")
	if err != nil {
		panic("initialize authentication password verifier")
	}
	h := &handler{cfg: cfg, pool: pool, dummyHash: dummyHash}
	limits := NewLoginLimiter(cfg.LoginRateLimit, cfg.LoginRateWindow)

	router.POST("/api/auth/login", limits.Middleware(), httpx.Adapt(h.login))
	protected := router.Group("/api/auth")
	protected.Use(Authenticate(pool, cfg.SessionCookieName))
	protected.GET("/me", httpx.Adapt(h.me))
	protected.POST("/logout", RequireMutation(cfg), httpx.Adapt(h.logout))
	protected.POST("/select-tenant", RequireMutation(cfg), httpx.Adapt(h.selectTenant))
}

func (h *handler) login(c *gin.Context) error {
	var input struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		return apperror.Validation("Email and password are required.", nil)
	}
	email := strings.ToLower(strings.TrimSpace(input.Email))
	var userID uuid.UUID
	var encoded string
	err := h.pool.QueryRow(c.Request.Context(), `
		SELECT id, password_hash FROM users
		WHERE lower(email) = $1 AND status = 'active'
	`, email).Scan(&userID, &encoded)
	if errors.Is(err, pgx.ErrNoRows) {
		_ = VerifyPassword(h.dummyHash, input.Password)
		return apperror.AuthInvalid()
	}
	if err != nil {
		return err
	}
	if !VerifyPassword(encoded, input.Password) {
		return apperror.AuthInvalid()
	}

	tokens, err := database.WithPlatformTx(c.Request.Context(), h.pool, func(tx pgx.Tx) (SessionTokens, error) {
		return Issue(c.Request.Context(), tx, userID, h.cfg.SessionTTL)
	})
	if err != nil {
		return err
	}
	SetSessionCookie(c, h.cfg, tokens.SessionToken, int(h.cfg.SessionTTL.Seconds()))
	returnJSON := gin.H{"csrfToken": tokens.CSRFToken, "user": gin.H{"id": userID, "email": email}}
	c.JSON(http.StatusOK, returnJSON)
	return nil
}

func (h *handler) me(c *gin.Context) error {
	identity, _ := IdentityFrom(c)
	csrfToken, err := RotateCSRF(c.Request.Context(), h.pool, identity.SessionID)
	if errors.Is(err, ErrSessionExpired) {
		return apperror.SessionExpired()
	}
	if err != nil {
		return err
	}
	c.JSON(http.StatusOK, gin.H{
		"csrfToken": csrfToken,
		"user": gin.H{
			"id": identity.UserID, "email": identity.Email, "displayName": identity.DisplayName,
		},
		"activeTenantId": identity.ActiveTenantID,
	})
	return nil
}

func (h *handler) logout(c *gin.Context) error {
	rawToken, _ := c.Cookie(h.cfg.SessionCookieName)
	if err := Revoke(c.Request.Context(), h.pool, rawToken); err != nil {
		return err
	}
	SetSessionCookie(c, h.cfg, "", -1)
	c.Status(http.StatusNoContent)
	return nil
}

func (h *handler) selectTenant(c *gin.Context) error {
	var input struct {
		TenantID uuid.UUID `json:"tenantId" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil || input.TenantID == uuid.Nil {
		return apperror.Validation("A valid tenantId is required.", nil)
	}
	identity, _ := IdentityFrom(c)
	if err := SelectTenant(c.Request.Context(), h.pool, identity.SessionID, input.TenantID); err != nil {
		return err
	}
	c.Status(http.StatusNoContent)
	return nil
}

func SetSessionCookie(c *gin.Context, cfg config.Config, value string, maxAge int) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(cfg.SessionCookieName, value, maxAge, "/", "", cfg.Production, true)
}
