package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"whatsapp-ai-poc/internal/platform/config"
	"whatsapp-ai-poc/internal/platform/database"
	"whatsapp-ai-poc/internal/testkit"
	"whatsapp-ai-poc/migrations"
)

func TestRunServesHealthAndShutsDown(t *testing.T) {
	db := testkit.StartPostgres(t)
	if _, err := database.Migrate(t.Context(), db.MigrationURL, migrations.FS); err != nil {
		t.Fatal(err)
	}
	pool, err := database.OpenPool(t.Context(), db.AppURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	origin, _ := url.Parse("http://" + listener.Addr().String())
	cfg := config.Config{
		Environment: "test", Host: "127.0.0.1", AppOrigin: origin,
		SessionCookieName: "wa_session", SessionTTL: time.Hour,
		LoginRateLimit: 5, LoginRateWindow: time.Minute,
	}

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- Run(ctx, cfg, pool, listener) }()

	client := &http.Client{Timeout: 2 * time.Second}
	var response *http.Response
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); {
		response, err = client.Get("http://" + listener.Addr().String() + "/health")
		if err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("health request failed: %v", err)
	}
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || !strings.Contains(string(body), `"database":"up"`) {
		t.Fatalf("unsafe health response status=%d body=%s", response.StatusCode, body)
	}
	if strings.Contains(string(body), "postgres://") || strings.Contains(string(body), "whatsapp_app") {
		t.Fatalf("health leaked database details: %s", body)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not shut down within five seconds")
	}
}
