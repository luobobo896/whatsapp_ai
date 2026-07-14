package httpx_test

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"whatsapp-ai-poc/internal/platform/httpx"
)

func TestRequestLoggerDoesNotLogPathSecrets(t *testing.T) {
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(httpx.RequestID(), httpx.RequestLogger())
	router.POST("/api/invitations/:token/accept", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	request := httptest.NewRequest(http.MethodPost, "/api/invitations/raw-secret-token/accept", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if strings.Contains(logs.String(), "raw-secret-token") {
		t.Fatalf("request log leaked a path secret: %s", logs.String())
	}
	if !strings.Contains(logs.String(), "/api/invitations/:token/accept") {
		t.Fatalf("request log did not contain the safe route template: %s", logs.String())
	}
}
