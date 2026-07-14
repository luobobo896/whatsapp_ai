package tenants_test

import (
	"bytes"
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
		"name": "Acme",
	}, protectedHeaders, platformCookie)
	if created.Code != http.StatusCreated {
		t.Fatalf("create tenant status=%d body=%s", created.Code, created.Body.String())
	}
	tenantID := uuid.MustParse(tenantJSONNestedString(t, created, "tenant", "id"))
	tenantSlug := tenantJSONNestedString(t, created, "tenant", "slug")
	ownerEmail := tenantJSONNestedString(t, created, "credentials", "email")
	ownerPassword := tenantJSONNestedString(t, created, "credentials", "password")
	if !strings.HasPrefix(tenantSlug, "tenant-") || !strings.HasPrefix(ownerEmail, "admin@tenant-") || len(ownerPassword) < 20 {
		t.Fatalf("tenant creation did not generate usable credentials: slug=%q email=%q passwordLength=%d", tenantSlug, ownerEmail, len(ownerPassword))
	}
	platformTenants := tenantRequest(t, router, http.MethodGet, "/api/tenants", nil, nil, platformCookie)
	if platformTenants.Code != http.StatusOK || !strings.Contains(platformTenants.Body.String(), `"platformRole":"platform_admin"`) ||
		!strings.Contains(platformTenants.Body.String(), `"slug":"`+tenantSlug+`"`) {
		t.Fatalf("platform tenant list status=%d body=%s", platformTenants.Code, platformTenants.Body.String())
	}
	var platformID uuid.UUID
	if err := admin.QueryRow(t.Context(), "SELECT user_id FROM platform_roles LIMIT 1").Scan(&platformID); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(t.Context(), `
		INSERT INTO tenant_memberships (tenant_id, user_id, role, status)
		VALUES ($1, $2, 'viewer', 'active')
	`, tenantID, platformID); err != nil {
		t.Fatal(err)
	}
	platformMemberTenants := tenantRequest(t, router, http.MethodGet, "/api/tenants", nil, nil, platformCookie)
	if platformMemberTenants.Code != http.StatusOK || !strings.Contains(platformMemberTenants.Body.String(), `"role":"viewer"`) ||
		!strings.Contains(platformMemberTenants.Body.String(), `"metrics:read"`) {
		t.Fatalf("platform member tenant list status=%d body=%s", platformMemberTenants.Code, platformMemberTenants.Body.String())
	}
	ownerLogin := tenantRequest(t, router, http.MethodPost, "/api/auth/login", map[string]any{
		"email": ownerEmail, "password": ownerPassword,
	}, nil, nil)
	if ownerLogin.Code != http.StatusOK {
		t.Fatalf("generated owner login status=%d body=%s", ownerLogin.Code, ownerLogin.Body.String())
	}
	ownerID := uuid.MustParse(tenantJSONNestedString(t, ownerLogin, "user", "id"))
	ownerCookie := tenantCookie(t, ownerLogin, "wa_session")
	ownerCSRF := tenantJSONString(t, ownerLogin, "csrfToken")
	ownerHeaders := map[string]string{"Origin": "http://localhost:8790", "X-CSRF-Token": ownerCSRF}
	var originalPasswordHash, originalDisplayName string
	if err := admin.QueryRow(t.Context(), "SELECT password_hash, display_name FROM users WHERE id = $1", ownerID).Scan(&originalPasswordHash, &originalDisplayName); err != nil {
		t.Fatal(err)
	}

	secondTenant := tenantRequest(t, router, http.MethodPost, "/api/platform/tenants", map[string]any{
		"name": "Second",
	}, protectedHeaders, platformCookie)
	if secondTenant.Code != http.StatusCreated {
		t.Fatalf("create second tenant status=%d body=%s", secondTenant.Code, secondTenant.Body.String())
	}
	secondTenantID := uuid.MustParse(tenantJSONNestedString(t, secondTenant, "tenant", "id"))
	secondEmail := tenantJSONNestedString(t, secondTenant, "credentials", "email")
	secondPassword := tenantJSONNestedString(t, secondTenant, "credentials", "password")
	secondLogin := tenantRequest(t, router, http.MethodPost, "/api/auth/login", map[string]any{
		"email": secondEmail, "password": secondPassword,
	}, nil, nil)
	secondCookie := tenantCookie(t, secondLogin, "wa_session")
	secondCSRF := tenantJSONString(t, secondLogin, "csrfToken")
	secondHeaders := map[string]string{"Origin": "http://localhost:8790", "X-CSRF-Token": secondCSRF}
	selectedSecond := tenantRequest(t, router, http.MethodPost, "/api/auth/select-tenant", map[string]any{
		"tenantId": secondTenantID,
	}, secondHeaders, secondCookie)
	if selectedSecond.Code != http.StatusNoContent {
		t.Fatalf("select second tenant status=%d body=%s", selectedSecond.Code, selectedSecond.Body.String())
	}
	secondInvitation := tenantRequest(t, router, http.MethodPost, "/api/members/invitations", map[string]any{
		"email": ownerEmail, "role": "owner",
	}, secondHeaders, secondCookie)
	if secondInvitation.Code != http.StatusCreated {
		t.Fatalf("invite existing owner status=%d body=%s", secondInvitation.Code, secondInvitation.Body.String())
	}
	secondToken := tenantJSONNestedString(t, secondInvitation, "invitation", "token")
	takeover := tenantRequest(t, router, http.MethodPost, "/api/invitations/"+secondToken+"/accept", map[string]any{
		"email": ownerEmail, "displayName": "Hijacked", "password": "attacker password",
	}, nil, nil)
	tenantAssertError(t, takeover, http.StatusUnauthorized, "AUTH_INVALID")
	acceptedExisting := tenantRequest(t, router, http.MethodPost, "/api/invitations/"+secondToken+"/accept", map[string]any{
		"email": ownerEmail, "displayName": "Hijacked", "password": ownerPassword,
	}, nil, nil)
	if acceptedExisting.Code != http.StatusCreated {
		t.Fatalf("existing user acceptance status=%d body=%s", acceptedExisting.Code, acceptedExisting.Body.String())
	}
	var passwordHashAfter, displayNameAfter string
	if err := admin.QueryRow(t.Context(), "SELECT password_hash, display_name FROM users WHERE id = $1", ownerID).Scan(&passwordHashAfter, &displayNameAfter); err != nil {
		t.Fatal(err)
	}
	if passwordHashAfter != originalPasswordHash || displayNameAfter != originalDisplayName {
		t.Fatal("existing-user invitation changed global credentials or profile")
	}
	ownerTenants := tenantRequest(t, router, http.MethodGet, "/api/tenants", nil, nil, ownerCookie)
	if ownerTenants.Code != http.StatusOK || strings.Contains(ownerTenants.Body.String(), `"platformRole":"platform_admin"`) ||
		!strings.Contains(ownerTenants.Body.String(), `"slug":"`+tenantSlug+`"`) || !strings.Contains(ownerTenants.Body.String(), tenantJSONNestedString(t, secondTenant, "tenant", "slug")) ||
		!strings.Contains(ownerTenants.Body.String(), `"role":"owner"`) || !strings.Contains(ownerTenants.Body.String(), `"members:manage"`) {
		t.Fatalf("owner tenant list status=%d body=%s", ownerTenants.Code, ownerTenants.Body.String())
	}

	selected := tenantRequest(t, router, http.MethodPost, "/api/auth/select-tenant", map[string]any{
		"tenantId": tenantID,
	}, ownerHeaders, ownerCookie)
	if selected.Code != http.StatusNoContent {
		t.Fatalf("select tenant status=%d body=%s", selected.Code, selected.Body.String())
	}
	createdAccount := tenantRequest(t, router, http.MethodPost, "/api/accounts", map[string]any{
		"name": "WhatsApp Support", "dailyLimit": 30,
	}, ownerHeaders, ownerCookie)
	if createdAccount.Code != http.StatusCreated || !strings.Contains(createdAccount.Body.String(), `"status":"pending"`) {
		t.Fatalf("create account status=%d body=%s", createdAccount.Code, createdAccount.Body.String())
	}
	accounts := tenantRequest(t, router, http.MethodGet, "/api/accounts", nil, nil, ownerCookie)
	if accounts.Code != http.StatusOK || !strings.Contains(accounts.Body.String(), "WhatsApp Support") {
		t.Fatalf("account list status=%d body=%s", accounts.Code, accounts.Body.String())
	}
	createdBase := tenantRequest(t, router, http.MethodPost, "/api/knowledge/bases", map[string]any{
		"name": "Product Knowledge", "description": "Product catalog and policies",
	}, ownerHeaders, ownerCookie)
	if createdBase.Code != http.StatusCreated {
		t.Fatalf("create knowledge base status=%d body=%s", createdBase.Code, createdBase.Body.String())
	}
	bases := tenantRequest(t, router, http.MethodGet, "/api/knowledge/bases", nil, nil, ownerCookie)
	if bases.Code != http.StatusOK || !strings.Contains(bases.Body.String(), "Product Knowledge") {
		t.Fatalf("knowledge base list status=%d body=%s", bases.Code, bases.Body.String())
	}
	conversations := tenantRequest(t, router, http.MethodGet, "/api/conversations", nil, nil, ownerCookie)
	if conversations.Code != http.StatusOK || !strings.Contains(conversations.Body.String(), `"conversations":[]`) {
		t.Fatalf("conversation list status=%d body=%s", conversations.Code, conversations.Body.String())
	}
	reinviteOwner := tenantRequest(t, router, http.MethodPost, "/api/members/invitations", map[string]any{
		"email": ownerEmail, "role": "viewer",
	}, ownerHeaders, ownerCookie)
	tenantAssertError(t, reinviteOwner, http.StatusConflict, "CONFLICT")

	forgedTenant := uuid.New()
	invited := tenantRequest(t, router, http.MethodPost, "/api/members/invitations", map[string]any{
		"email": "viewer@example.com", "role": "viewer", "tenantId": forgedTenant,
	}, ownerHeaders, ownerCookie)
	if invited.Code != http.StatusCreated {
		t.Fatalf("invite status=%d body=%s", invited.Code, invited.Body.String())
	}
	viewerToken := tenantJSONNestedString(t, invited, "invitation", "token")
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
	if members.Code != http.StatusOK || !strings.Contains(members.Body.String(), ownerEmail) {
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
	suspendedAcceptance := tenantRequest(t, router, http.MethodPost, "/api/invitations/"+viewerToken+"/accept", map[string]any{
		"email": "viewer@example.com", "displayName": "Viewer", "password": "viewer password",
	}, nil, nil)
	tenantAssertError(t, suspendedAcceptance, http.StatusForbidden, "TENANT_SUSPENDED")

	var audits int
	if err := admin.QueryRow(t.Context(), "SELECT count(*) FROM audit_logs WHERE tenant_id = $1", tenantID).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if audits < 3 {
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
