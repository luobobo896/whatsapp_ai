package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"whatsapp-ai-poc/internal/handler"
	"whatsapp-ai-poc/internal/middleware"
	"whatsapp-ai-poc/internal/store"
	"whatsapp-ai-poc/web"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	host := env("HTTP_HOST", "127.0.0.1")
	port := env("PORT", "8790")
	dbPath := env("DB_PATH", "data/app.db")

	// Ensure data dir exists
	if err := os.MkdirAll("data", 0755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	st, err := store.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer st.Close()

	// Seed platform admin if no users exist
	if err := seedPlatformAdmin(st); err != nil {
		return fmt.Errorf("seed admin: %w", err)
	}

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Recovery())

	// Health
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Auth routes: login is public, rest require auth
	authGroup := router.Group("/api/auth")
	authGroup.POST("/login", handler.HandleLogin(st))
	authGroup.Use(middleware.Auth(st))
	{
		authGroup.POST("/logout", handler.HandleLogout(st))
		authGroup.GET("/me", handler.HandleMe(st))
		authGroup.POST("/select-tenant", handler.HandleSelectTenant(st))
	}

	// Public invitation accept (no auth)
	handler.RegisterInvitations(router.Group("/api/invitations"), st)

	// Protected routes (require auth + CSRF)
	api := router.Group("/api", middleware.Auth(st), middleware.RequireCSRF())
	{
		handler.RegisterTenants(api.Group("/tenants"), st)
		handler.RegisterAccounts(api.Group("/accounts"), st)
		handler.RegisterKnowledge(api.Group("/knowledge"), st)
		handler.RegisterConversations(api.Group("/conversations"), st)
		handler.RegisterMembers(api.Group("/members"), st)

		// Platform admin only
		platform := api.Group("/platform/tenants", middleware.RequirePlatformAdmin())
		handler.RegisterPlatformTenants(platform, st)
	}

	// SPA fallback
	frontend := web.Handler()
	router.NoRoute(func(c *gin.Context) {
		frontend.ServeHTTP(c.Writer, c.Request)
	})

	addr := net.JoinHostPort(host, port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", addr, err)
	}

	server := &http.Server{
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		slog.Default().Info("shutting down...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(shutdownCtx)
	}()

	slog.Default().Info("server starting", "address", addr)
	if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func seedPlatformAdmin(st *store.Store) error {
	email := env("ADMIN_EMAIL", "admin@whatsapp-ai.local")
	password := env("ADMIN_PASSWORD", "admin123456")

	// Check if admin user already exists
	if _, err := st.UserByEmail(email); err == nil {
		return nil // already seeded
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = st.CreateUser(email, "平台管理员", string(hash), "platform_admin")
	if err != nil {
		return err
	}
	slog.Default().Info("platform admin seeded", "email", email)
	return nil
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
