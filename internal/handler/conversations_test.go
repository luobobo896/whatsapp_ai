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

func TestAccountKnowledgeBaseIDs(t *testing.T) {
	account := &model.AccountRow{KbID: `["kb-1","kb-2"]`}
	ids := accountKnowledgeBaseIDs(account)
	if len(ids) != 2 || ids[0] != "kb-1" || ids[1] != "kb-2" {
		t.Fatalf("knowledge base IDs = %#v", ids)
	}
	if ids := accountKnowledgeBaseIDs(&model.AccountRow{KbID: "invalid"}); ids != nil {
		t.Fatalf("invalid knowledge base IDs = %#v, want nil", ids)
	}
}

func TestEffectiveMessageLimit(t *testing.T) {
	tests := []struct {
		name      string
		requested int
		account   int
		want      int
	}{
		{name: "default request", requested: 0, account: 30, want: 30},
		{name: "request below account maximum", requested: 10, account: 30, want: 10},
		{name: "request capped by account maximum", requested: 100, account: 30, want: 30},
		{name: "invalid account maximum uses default", requested: 100, account: 0, want: 30},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := effectiveMessageLimit(tt.requested, tt.account); got != tt.want {
				t.Fatalf("effectiveMessageLimit(%d, %d) = %d, want %d", tt.requested, tt.account, got, tt.want)
			}
		})
	}
}

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
