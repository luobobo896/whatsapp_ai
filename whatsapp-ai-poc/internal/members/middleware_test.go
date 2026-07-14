package members_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"whatsapp-ai-poc/internal/auth"
	"whatsapp-ai-poc/internal/members"
	"whatsapp-ai-poc/internal/platform/database"
	"whatsapp-ai-poc/internal/platform/httpx"
	"whatsapp-ai-poc/internal/testkit"
	"whatsapp-ai-poc/migrations"
)

func TestRequirePermissionUsesSelectedTenantAndStatus(t *testing.T) {
	fixture := newAuthorizationFixture(t)
	router := gin.New()
	router.Use(httpx.RequestID())
	router.GET("/allowed", auth.Authenticate(fixture.pool, "wa_session"),
		members.RequirePermission(fixture.pool, members.PermissionMetricsRead),
		func(c *gin.Context) {
			tenant, ok := members.TenantFrom(c)
			if !ok || tenant.TenantID != fixture.tenantID || tenant.Role != members.RoleViewer {
				c.Status(http.StatusInternalServerError)
				return
			}
			c.Status(http.StatusNoContent)
		})
	router.GET("/denied", auth.Authenticate(fixture.pool, "wa_session"),
		members.RequirePermission(fixture.pool, members.PermissionKnowledgeManage),
		func(c *gin.Context) { c.Status(http.StatusNoContent) })

	if response := authorizeRequest(router, "/allowed", fixture.sessionToken); response.Code != http.StatusNoContent {
		t.Fatalf("allowed status=%d body=%s", response.Code, response.Body.String())
	}
	denied := authorizeRequest(router, "/denied", fixture.sessionToken)
	assertMiddlewareError(t, denied, http.StatusForbidden, "FORBIDDEN")

	if _, err := fixture.admin.Exec(t.Context(), "UPDATE tenants SET status = 'suspended', suspended_reason = 'billing' WHERE id = $1", fixture.tenantID); err != nil {
		t.Fatal(err)
	}
	suspended := authorizeRequest(router, "/allowed", fixture.sessionToken)
	assertMiddlewareError(t, suspended, http.StatusForbidden, "TENANT_SUSPENDED")
}

func TestRequirePlatformAdminIsIndependentOfTenantRole(t *testing.T) {
	fixture := newAuthorizationFixture(t)
	router := gin.New()
	router.Use(httpx.RequestID())
	router.GET("/platform", auth.Authenticate(fixture.pool, "wa_session"),
		members.RequirePlatformAdmin(fixture.pool), func(c *gin.Context) { c.Status(http.StatusNoContent) })

	denied := authorizeRequest(router, "/platform", fixture.sessionToken)
	assertMiddlewareError(t, denied, http.StatusForbidden, "FORBIDDEN")
	if _, err := fixture.admin.Exec(t.Context(), "INSERT INTO platform_roles (user_id, role) VALUES ($1, 'platform_admin')", fixture.userID); err != nil {
		t.Fatal(err)
	}
	if response := authorizeRequest(router, "/platform", fixture.sessionToken); response.Code != http.StatusNoContent {
		t.Fatalf("platform status=%d body=%s", response.Code, response.Body.String())
	}
}

type authorizationFixture struct {
	pool         *pgxpool.Pool
	admin        *pgx.Conn
	userID       uuid.UUID
	tenantID     uuid.UUID
	sessionToken string
}

func newAuthorizationFixture(t *testing.T) authorizationFixture {
	t.Helper()
	db := testkit.StartPostgres(t)
	if _, err := database.Migrate(t.Context(), db.MigrationURL, migrations.FS); err != nil {
		t.Fatal(err)
	}
	admin, err := pgx.Connect(t.Context(), db.MigrationURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close(context.Background()) })
	userID, tenantID := uuid.New(), uuid.New()
	batch := &pgx.Batch{}
	batch.Queue(`INSERT INTO users (id, email, display_name, password_hash, status)
		VALUES ($1, 'viewer@example.com', 'Viewer', 'test', 'active')`, userID)
	batch.Queue("INSERT INTO tenants (id, name, slug, status) VALUES ($1, 'Acme', 'acme', 'active')", tenantID)
	batch.Queue(`INSERT INTO tenant_memberships (tenant_id, user_id, role, status)
		VALUES ($1, $2, 'viewer', 'active')`, tenantID, userID)
	if err := admin.SendBatch(t.Context(), batch).Close(); err != nil {
		t.Fatal(err)
	}
	pool, err := database.OpenPool(t.Context(), db.AppURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	tokens, err := database.WithPlatformTx(t.Context(), pool, func(tx pgx.Tx) (auth.SessionTokens, error) {
		return auth.Issue(t.Context(), tx, userID, time.Hour)
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.SelectTenant(t.Context(), pool, tokens.SessionID, tenantID); err != nil {
		t.Fatal(err)
	}
	return authorizationFixture{pool: pool, admin: admin, userID: userID, tenantID: tenantID, sessionToken: tokens.SessionToken}
}

func authorizeRequest(handler http.Handler, path, sessionToken string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodGet, path, nil)
	request.AddCookie(&http.Cookie{Name: "wa_session", Value: sessionToken})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func assertMiddlewareError(t *testing.T, response *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if response.Code != status || !strings.Contains(response.Body.String(), `"code":"`+code+`"`) {
		t.Fatalf("unexpected response status=%d body=%s", response.Code, response.Body.String())
	}
}
