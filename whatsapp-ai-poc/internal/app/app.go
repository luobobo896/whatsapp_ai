package app

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"whatsapp-ai-poc/internal/auth"
	"whatsapp-ai-poc/internal/members"
	"whatsapp-ai-poc/internal/platform/config"
	"whatsapp-ai-poc/internal/platform/httpx"
	"whatsapp-ai-poc/internal/tenants"
)

type Pinger interface {
	Ping(context.Context) error
}

func New(cfg config.Config, pool *pgxpool.Pool, pinger Pinger) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	_ = router.SetTrustedProxies(nil)
	router.Use(httpx.RequestID(), httpx.Recovery(), httpx.SecurityHeaders(), httpx.RequestLogger())

	if pinger == nil && pool != nil {
		pinger = pool
	}
	router.GET("/health", healthHandler(pinger))
	if pool != nil {
		auth.RegisterRoutes(router, cfg, pool)
		memberService := members.NewService(pool, cfg.SessionTTL)
		members.RegisterRoutes(router, cfg, pool, memberService)
		tenantService := tenants.NewService(pool, memberService)
		tenants.RegisterRoutes(router, cfg, pool, tenantService)
	}
	router.NoRoute(httpx.Adapt(httpx.NoRoute))
	return router
}

func healthHandler(pinger Pinger) gin.HandlerFunc {
	return func(c *gin.Context) {
		status := http.StatusOK
		body := gin.H{"status": "ok", "database": "up"}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		if pinger == nil || pinger.Ping(ctx) != nil {
			status = http.StatusServiceUnavailable
			body["status"] = "degraded"
			body["database"] = "down"
		}
		c.JSON(status, body)
	}
}
