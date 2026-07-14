package tenants_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"whatsapp-ai-poc/internal/app"
	"whatsapp-ai-poc/internal/auth"
	"whatsapp-ai-poc/internal/platform/config"
	"whatsapp-ai-poc/internal/platform/database"
	"whatsapp-ai-poc/internal/testkit"
	"whatsapp-ai-poc/migrations"
)

func TestTenantLifecycle(t *testing.T) {
	router, admin := newFoundationServer(t)
	platformLogin := tenantRequest(t, router, http.MethodPost, "/api/auth/login", map[string]any{
		"email": "platform@example.com", "password": "platform password",
	}, nil, nil)
	platformCookie := tenantCookie(t, platformLogin, "wa_session")
	platformCSRF := tenantJSONString(t, platformLogin, "csrfToken")
	protectedHeaders := map[string]string{"Origin": "http://localhost:8790", "X-CSRF-Token": platformCSRF}

	created := tenantRequest(t, router, http.MethodPost, "/api/platform/tenants", map[string]any{
		"name": "Acme", "slug": "acme", "ownerEmail": "owner@example.com", "ownerDisplayName": "Owner",
	}, protectedHeaders, platformCookie)
	if created.Code != http.StatusCreated {
		t.Fatalf("create tenant status=%d body=%s", created.Code, created.Body.String())
	}
	tenantID := uuid.MustParse(tenantJSONNestedString(t, created, "tenant", "id"))
	invitationToken := tenantJSONNestedString(t, created, "invitation", "token")
	if invitationToken == "" {
		t.Fatal("tenant creation did not return the owner invitation token")
	}
	var storedHash string
	if err := admin.QueryRow(t.Context(), "SELECT token_hash FROM member_invitations WHERE tenant_id = $1", tenantID).Scan(&storedHash); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(invitationToken))
	if storedHash != hex.EncodeToString(digest[:]) || storedHash == invitationToken {
		t.Fatal("owner invitation was not stored as a SHA-256 hash")
	}

	mismatch := tenantRequest(t, router, http.MethodPost, "/api/invitations/"+invitationToken+"/accept", map[string]any{
		"email": "attacker@example.com", "displayName": "Attacker", "password": "attacker password",
	}, nil, nil)
	tenantAssertError(t, mismatch, http.StatusForbidden, "FORBIDDEN")

	accepted := tenantRequest(t, router, http.MethodPost, "/api/invitations/"+invitationToken+"/accept", map[string]any{
		"email": "owner@example.com", "displayName": "Owner", "password": "owner password",
	}, nil, nil)
	if accepted.Code != http.StatusCreated {
		t.Fatalf("accept status=%d body=%s", accepted.Code, accepted.Body.String())
	}
	ownerID := uuid.MustParse(tenantJSONNestedString(t, accepted, "user", "id"))
	ownerCookie := tenantCookie(t, accepted, "wa_session")
	ownerCSRF := tenantJSONString(t, accepted, "csrfToken")
	ownerHeaders := map[string]string{"Origin": "http://localhost:8790", "X-CSRF-Token": ownerCSRF}

	selected := tenantRequest(t, router, http.MethodPost, "/api/auth/select-tenant", map[string]any{
		"tenantId": tenantID,
	}, ownerHeaders, ownerCookie)
	if selected.Code != http.StatusNoContent {
		t.Fatalf("select tenant status=%d body=%s", selected.Code, selected.Body.String())
	}

	forgedTenant := uuid.New()
	invited := tenantRequest(t, router, http.MethodPost, "/api/members/invitations", map[string]any{
		"email": "viewer@example.com", "role": "viewer", "tenantId": forgedTenant,
	}, ownerHeaders, ownerCookie)
	if invited.Code != http.StatusCreated {
		t.Fatalf("invite status=%d body=%s", invited.Code, invited.Body.String())
	}
	var invitationTenant uuid.UUID
	if err := admin.QueryRow(t.Context(), `
		SELECT tenant_id FROM member_invitations
		WHERE lower(email) = 'viewer@example.com'
	`).Scan(&invitationTenant); err != nil {
		t.Fatal(err)
	}
	if invitationTenant != tenantID || invitationTenant == forgedTenant {
		t.Fatalf("forged tenant selected: got %s want %s", invitationTenant, tenantID)
	}

	members := tenantRequest(t, router, http.MethodGet, "/api/members", nil, nil, ownerCookie)
	if members.Code != http.StatusOK || !strings.Contains(members.Body.String(), "owner@example.com") {
		t.Fatalf("member list status=%d body=%s", members.Code, members.Body.String())
	}

	demote := tenantRequest(t, router, http.MethodPatch, "/api/members/"+ownerID.String(), map[string]any{
		"role": "admin", "status": "active",
	}, ownerHeaders, ownerCookie)
	tenantAssertError(t, demote, http.StatusConflict, "CONFLICT")

	suspended := tenantRequest(t, router, http.MethodPatch, "/api/platform/tenants/"+tenantID.String()+"/status", map[string]any{
		"status": "suspended", "reason": "billing review",
	}, protectedHeaders, platformCookie)
	if suspended.Code != http.StatusNoContent {
		t.Fatalf("suspend status=%d body=%s", suspended.Code, suspended.Body.String())
	}
	tenantAccess := tenantRequest(t, router, http.MethodGet, "/api/members", nil, nil, ownerCookie)
	tenantAssertError(t, tenantAccess, http.StatusForbidden, "TENANT_SUSPENDED")

	var audits int
	if err := admin.QueryRow(t.Context(), "SELECT count(*) FROM audit_logs WHERE tenant_id = $1", tenantID).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if audits < 4 {
		t.Fatalf("expected lifecycle audit records, got %d", audits)
	}
}

func newFoundationServer(t *testing.T) (http.Handler, *pgx.Conn) {
	t.Helper()
	db := testkit.StartPostgres(t)
	if _, err := database.Migrate(t.Context(), db.MigrationURL, migrations.FS); err != nil {
		t.Fatal(err)
	}
	encoded, err := auth.HashPassword("platform password")
	if err != nil {
		t.Fatal(err)
	}
	admin, err := pgx.Connect(t.Context(), db.MigrationURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close(t.Context()) })
	platformID := uuid.New()
	batch := &pgx.Batch{}
	batch.Queue(`INSERT INTO users (id, email, display_name, password_hash, status)
		VALUES ($1, 'platform@example.com', 'Platform', $2, 'active')`, platformID, encoded)
	batch.Queue("INSERT INTO platform_roles (user_id, role) VALUES ($1, 'platform_admin')", platformID)
	if err := admin.SendBatch(t.Context(), batch).Close(); err != nil {
		t.Fatal(err)
	}
	pool, err := database.OpenPool(t.Context(), db.AppURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	origin, _ := url.Parse("http://localhost:8790")
	cfg := config.Config{
		Environment: "test", Host: "127.0.0.1", Port: 8790, AppOrigin: origin,
		DatabaseURL: db.AppURL, SessionCookieName: "wa_session", SessionTTL: 12 * time.Hour,
		LoginRateLimit: 10, LoginRateWindow: time.Minute,
	}
	return app.New(cfg, pool, pool), admin
}

func tenantRequest(t *testing.T, handler http.Handler, method, path string, body any, headers map[string]string, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	var encoded []byte
	if body != nil {
		var err error
		encoded, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	request := httptest.NewRequest(method, path, bytes.NewReader(encoded))
	request.RemoteAddr = "127.0.0.1:12345"
	request.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		request.Header.Set(key, value)
	}
	if cookie != nil {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func tenantCookie(t *testing.T, response *httptest.ResponseRecorder, name string) *http.Cookie {
	t.Helper()
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == name {
			return cookie
		}
	}
	t.Fatalf("response missing cookie %s; status=%d body=%s", name, response.Code, response.Body.String())
	return nil
}

func tenantJSONString(t *testing.T, response *httptest.ResponseRecorder, key string) string {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	value, _ := body[key].(string)
	return value
}

func tenantJSONNestedString(t *testing.T, response *httptest.ResponseRecorder, object, key string) string {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	nested, _ := body[object].(map[string]any)
	value, _ := nested[key].(string)
	return value
}

func tenantAssertError(t *testing.T, response *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if response.Code != status || !strings.Contains(response.Body.String(), `"code":"`+code+`"`) {
		t.Fatalf("unexpected response status=%d body=%s", response.Code, response.Body.String())
	}
}
