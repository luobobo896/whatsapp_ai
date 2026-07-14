package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"whatsapp-ai-poc/internal/middleware"
	"whatsapp-ai-poc/internal/model"
)

func init() { gin.SetMode(gin.TestMode) }

func setupTestContext() (*gin.Context, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/", nil)
	c.Request.Header.Set("Content-Type", "application/json")
	return c, w
}

func setSession(c *gin.Context, tenantID string) {
	c.Set(middleware.SessionKey, &model.Session{
		ActiveTenantID: tenantID,
		CSRFToken:      "test-csrf",
		User:           model.User{ID: "u1", Email: "a@b.com"},
	})
}

func TestSaveReplyNoTenant(t *testing.T) {
	c, w := setupTestContext()
	handleSaveReply(nil)(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	var body map[string]model.ErrorDetail
	json.Unmarshal(w.Body.Bytes(), &body)
	if body["error"].Code != "TENANT_REQUIRED" {
		t.Fatalf("error code = %q, want TENANT_REQUIRED", body["error"].Code)
	}
}

func TestSaveReplyMissingFields(t *testing.T) {
	c, w := setupTestContext()
	setSession(c, "t1")
	handleSaveReply(nil)(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (empty body)", w.Code)
	}
}

func TestSaveReplyMissingContent(t *testing.T) {
	c, w := setupTestContext()
	setSession(c, "t1")
	c.Request = httptest.NewRequest("POST", "/",
		strings.NewReader(`{"conversationId":"c1","accountId":"a1"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	handleSaveReply(nil)(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestSaveReplyMissingConversationID(t *testing.T) {
	c, w := setupTestContext()
	setSession(c, "t1")
	c.Request = httptest.NewRequest("POST", "/",
		strings.NewReader(`{"content":"hello","accountId":"a1"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	handleSaveReply(nil)(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}
