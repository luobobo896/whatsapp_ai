package app_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"whatsapp-ai-poc/internal/app"
	"whatsapp-ai-poc/internal/platform/config"
)

type fakePinger struct{ err error }

func (p fakePinger) Ping(context.Context) error { return p.err }

func TestUnknownRouteHasStableErrorAndRequestID(t *testing.T) {
	router := app.New(testConfig(), nil, fakePinger{})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/missing", nil))

	assertAPIError(t, response, http.StatusNotFound, "NOT_FOUND")
	requestID := response.Header().Get("X-Request-ID")
	if !strings.HasPrefix(requestID, "req_") {
		t.Fatalf("unexpected request ID %q", requestID)
	}
	if response.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("security headers missing")
	}
}

func TestHealthReportsOnlySafeDatabaseState(t *testing.T) {
	router := app.New(testConfig(), nil, fakePinger{err: errors.New("postgres://user:" + "secret@private/db")})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/health", nil))

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
	if strings.Contains(response.Body.String(), "secret") || strings.Contains(response.Body.String(), "private") {
		t.Fatalf("health response leaked connection details: %s", response.Body.String())
	}
	var body map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "degraded" || body["database"] != "down" {
		t.Fatalf("unexpected health response: %v", body)
	}
}

func assertAPIError(t *testing.T, response *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if response.Code != status {
		t.Fatalf("status = %d, want %d; body=%s", response.Code, status, response.Body.String())
	}
	var body struct {
		Error struct {
			Code      string `json:"code"`
			RequestID string `json:"requestId"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error.Code != code || !strings.HasPrefix(body.Error.RequestID, "req_") {
		t.Fatalf("unexpected API error: %#v", body.Error)
	}
}

func testConfig() config.Config {
	origin, _ := url.Parse("http://localhost:8790")
	return config.Config{
		Environment:       "test",
		Host:              "127.0.0.1",
		Port:              8790,
		AppOrigin:         origin,
		SessionCookieName: "wa_session",
	}
}
