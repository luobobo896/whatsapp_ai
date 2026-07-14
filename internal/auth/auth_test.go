package auth_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"whatsapp-ai-poc/internal/app"
	"whatsapp-ai-poc/internal/auth"
	"whatsapp-ai-poc/internal/platform/config"
	"whatsapp-ai-poc/internal/platform/database"
	"whatsapp-ai-poc/internal/testkit"
	"whatsapp-ai-poc/migrations"
)

func TestLoginMeAndLogoutCSRFContract(t *testing.T) {
	router, pool := newAuthServer(t, 5)

	login := performJSON(t, router, http.MethodPost, "/api/auth/login", map[string]string{
		"email": " ADMIN@EXAMPLE.COM ", "password": "safe password",
	}, nil, nil)
	if login.Code != http.StatusOK {
		t.Fatalf("login status=%d body=%s", login.Code, login.Body.String())
	}
	cookie := responseCookie(t, login, "wa_session")
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode || cookie.Secure {
		t.Fatalf("unsafe session cookie: %#v", cookie)
	}
	csrf := jsonString(t, login, "csrfToken")
	if csrf == "" {
		t.Fatal("login did not return a CSRF token")
	}

	missingProtection := performJSON(t, router, http.MethodPost, "/api/auth/logout", nil, nil, cookie)
	assertError(t, missingProtection, http.StatusForbidden, "FORBIDDEN")

	me := performJSON(t, router, http.MethodGet, "/api/auth/me", nil, nil, cookie)
	if me.Code != http.StatusOK {
		t.Fatalf("me status=%d body=%s", me.Code, me.Body.String())
	}
	rotatedCSRF := jsonString(t, me, "csrfToken")
	if rotatedCSRF == "" || rotatedCSRF == csrf {
		t.Fatal("me did not rotate the CSRF token")
	}

	oldToken := performJSON(t, router, http.MethodPost, "/api/auth/logout", nil, map[string]string{
		"Origin": "http://localhost:8790", "X-CSRF-Token": csrf,
	}, cookie)
	assertError(t, oldToken, http.StatusForbidden, "FORBIDDEN")

	logout := performJSON(t, router, http.MethodPost, "/api/auth/logout", nil, map[string]string{
		"Origin": "http://localhost:8790", "X-CSRF-Token": rotatedCSRF,
	}, cookie)
	if logout.Code != http.StatusNoContent {
		t.Fatalf("logout status=%d body=%s", logout.Code, logout.Body.String())
	}

	afterLogout := performJSON(t, router, http.MethodGet, "/api/auth/me", nil, nil, cookie)
	assertError(t, afterLogout, http.StatusUnauthorized, "SESSION_EXPIRED")

	var active int
	if err := pool.QueryRow(t.Context(), "SELECT count(*) FROM auth_sessions WHERE revoked_at IS NULL").Scan(&active); err != nil {
		t.Fatal(err)
	}
	if active != 0 {
		t.Fatalf("active sessions after logout: %d", active)
	}
}

func TestExpiredSessionAndLoginRateLimit(t *testing.T) {
	router, pool := newAuthServer(t, 2)
	login := performJSON(t, router, http.MethodPost, "/api/auth/login", map[string]string{
		"email": "admin@example.com", "password": "safe password",
	}, nil, nil)
	cookie := responseCookie(t, login, "wa_session")
	if _, err := pool.Exec(t.Context(), "UPDATE auth_sessions SET expires_at = now() - interval '1 second'"); err != nil {
		t.Fatal(err)
	}
	expired := performJSON(t, router, http.MethodGet, "/api/auth/me", nil, nil, cookie)
	assertError(t, expired, http.StatusUnauthorized, "SESSION_EXPIRED")

	for attempt := 1; attempt <= 2; attempt++ {
		invalid := performJSON(t, router, http.MethodPost, "/api/auth/login", map[string]string{
			"email": "admin@example.com", "password": "wrong",
		}, nil, nil)
		if attempt == 1 {
			assertError(t, invalid, http.StatusUnauthorized, "AUTH_INVALID")
		} else {
			assertError(t, invalid, http.StatusTooManyRequests, "RATE_LIMITED")
		}
	}
}

func TestAdministratorCanRevokeAllUserSessions(t *testing.T) {
	_, pool := newAuthServer(t, 10)
	var userID uuid.UUID
	if err := pool.QueryRow(t.Context(), "SELECT id FROM users WHERE email = 'admin@example.com'").Scan(&userID); err != nil {
		t.Fatal(err)
	}
	var issued []auth.SessionTokens
	for range 2 {
		tokens, err := database.WithPlatformTx(t.Context(), pool, func(tx pgx.Tx) (auth.SessionTokens, error) {
			return auth.Issue(t.Context(), tx, userID, time.Hour)
		})
		if err != nil {
			t.Fatal(err)
		}
		issued = append(issued, tokens)
	}
	if err := auth.RevokeUserSessions(t.Context(), pool, userID); err != nil {
		t.Fatal(err)
	}
	for _, tokens := range issued {
		if _, err := auth.Resolve(t.Context(), pool, tokens.SessionToken); !errors.Is(err, auth.ErrSessionExpired) {
			t.Fatalf("revoked session resolved with error %v", err)
		}
	}
}

func newAuthServer(t *testing.T, rateLimit int) (http.Handler, *pgxpool.Pool) {
	t.Helper()
	db := testkit.StartPostgres(t)
	if _, err := database.Migrate(t.Context(), db.MigrationURL, migrations.FS); err != nil {
		t.Fatal(err)
	}
	hash, err := auth.HashPassword("safe password")
	if err != nil {
		t.Fatal(err)
	}
	admin, err := pgx.Connect(t.Context(), db.MigrationURL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(t.Context(), `
		INSERT INTO users (id, email, display_name, password_hash, status)
		VALUES ($1, 'admin@example.com', 'Admin', $2, 'active')
	`, uuid.New(), hash); err != nil {
		t.Fatal(err)
	}
	_ = admin.Close(t.Context())

	pool, err := database.OpenPool(t.Context(), db.AppURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	origin, _ := url.Parse("http://localhost:8790")
	cfg := config.Config{
		Environment:       "test",
		Host:              "127.0.0.1",
		Port:              8790,
		AppOrigin:         origin,
		DatabaseURL:       db.AppURL,
		SessionCookieName: "wa_session",
		SessionTTL:        12 * time.Hour,
		LoginRateLimit:    rateLimit,
		LoginRateWindow:   time.Minute,
	}
	return app.New(cfg, pool, pool), pool
}

func performJSON(t *testing.T, handler http.Handler, method, path string, body any, headers map[string]string, cookie *http.Cookie) *httptest.ResponseRecorder {
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

func responseCookie(t *testing.T, response *httptest.ResponseRecorder, name string) *http.Cookie {
	t.Helper()
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == name {
			return cookie
		}
	}
	t.Fatalf("response did not set cookie %s", name)
	return nil
}

func jsonString(t *testing.T, response *httptest.ResponseRecorder, key string) string {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	value, _ := body[key].(string)
	return value
}

func assertError(t *testing.T, response *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if response.Code != status {
		t.Fatalf("status=%d want=%d body=%s", response.Code, status, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), `"code":"`+code+`"`) {
		t.Fatalf("missing error code %s in %s", code, response.Body.String())
	}
}
