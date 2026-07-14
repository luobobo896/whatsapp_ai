package operations

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"whatsapp-ai-poc/internal/auth"
	"whatsapp-ai-poc/internal/members"
	"whatsapp-ai-poc/internal/platform/apperror"
	"whatsapp-ai-poc/internal/platform/config"
	"whatsapp-ai-poc/internal/platform/httpx"
)

type handler struct{ service *Service }

func RegisterRoutes(router *gin.Engine, cfg config.Config, pool *pgxpool.Pool, service *Service) {
	h := &handler{service: service}
	router.GET("/api/accounts",
		auth.Authenticate(pool, cfg.SessionCookieName), members.RequirePermission(pool, members.PermissionAccountsRead), httpx.Adapt(h.listAccounts),
	)
	router.POST("/api/accounts",
		auth.Authenticate(pool, cfg.SessionCookieName), auth.RequireMutation(cfg), members.RequirePermission(pool, members.PermissionAccountsManage), httpx.Adapt(h.createAccount),
	)
	router.GET("/api/knowledge/bases",
		auth.Authenticate(pool, cfg.SessionCookieName), members.RequirePermission(pool, members.PermissionKnowledgeRead), httpx.Adapt(h.listKnowledgeBases),
	)
	router.POST("/api/knowledge/bases",
		auth.Authenticate(pool, cfg.SessionCookieName), auth.RequireMutation(cfg), members.RequirePermission(pool, members.PermissionKnowledgeManage), httpx.Adapt(h.createKnowledgeBase),
	)
	router.GET("/api/conversations",
		auth.Authenticate(pool, cfg.SessionCookieName), members.RequirePermission(pool, members.PermissionConversationsRead), httpx.Adapt(h.listConversations),
	)
}

func (h *handler) listAccounts(c *gin.Context) error {
	tenant, _ := members.TenantFrom(c)
	accounts, err := h.service.ListAccounts(c.Request.Context(), tenant)
	if err != nil {
		return err
	}
	c.JSON(http.StatusOK, gin.H{"accounts": accounts})
	return nil
}

func (h *handler) createAccount(c *gin.Context) error {
	var input struct {
		Name       string `json:"name"`
		DailyLimit int    `json:"dailyLimit"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		return apperror.Validation("Valid account details are required.", nil)
	}
	tenant, _ := members.TenantFrom(c)
	account, err := h.service.CreateAccount(c.Request.Context(), tenant, input.Name, input.DailyLimit)
	if err != nil {
		return err
	}
	c.JSON(http.StatusCreated, gin.H{"account": account})
	return nil
}

func (h *handler) listKnowledgeBases(c *gin.Context) error {
	tenant, _ := members.TenantFrom(c)
	bases, err := h.service.ListKnowledgeBases(c.Request.Context(), tenant)
	if err != nil {
		return err
	}
	c.JSON(http.StatusOK, gin.H{"bases": bases})
	return nil
}

func (h *handler) createKnowledgeBase(c *gin.Context) error {
	var input struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		return apperror.Validation("Valid knowledge base details are required.", nil)
	}
	tenant, _ := members.TenantFrom(c)
	base, err := h.service.CreateKnowledgeBase(c.Request.Context(), tenant, input.Name, input.Description)
	if err != nil {
		return err
	}
	c.JSON(http.StatusCreated, gin.H{"base": base})
	return nil
}

func (h *handler) listConversations(c *gin.Context) error {
	tenant, _ := members.TenantFrom(c)
	conversations, err := h.service.ListConversations(c.Request.Context(), tenant)
	if err != nil {
		return err
	}
	c.JSON(http.StatusOK, gin.H{"conversations": conversations})
	return nil
}
