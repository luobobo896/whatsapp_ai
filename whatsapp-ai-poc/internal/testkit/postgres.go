package testkit

import (
	"context"
	"fmt"
	"io/fs"
	"net/url"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

const (
	testDatabase    = "whatsapp_ai_test"
	testPassword    = "postgres_test_password"
	appTestPassword = "whatsapp_app_test_password"
)

type Postgres struct {
	MigrationURL string
	AppURL       string
}

func StartPostgres(t *testing.T) Postgres {
	t.Helper()
	return StartPostgresWithDatabase(t, testDatabase)
}

func StartPostgresWithDatabase(t *testing.T, databaseName string) Postgres {
	t.Helper()
	if !strings.HasSuffix(databaseName, "_test") {
		t.Fatalf("refusing to start PostgreSQL for database %q: name must end in _test", databaseName)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	container, err := postgres.Run(ctx, "postgres:17-alpine",
		postgres.WithDatabase(databaseName),
		postgres.WithUsername("postgres"),
		postgres.WithPassword(testPassword),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatalf("start PostgreSQL test container: %v", err)
	}
	t.Cleanup(func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer stopCancel()
		if err := container.Terminate(stopCtx); err != nil {
			t.Errorf("terminate PostgreSQL test container: %v", err)
		}
	})

	migrationURL, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("resolve PostgreSQL test connection: %v", err)
	}
	conn, err := pgx.Connect(ctx, migrationURL)
	if err != nil {
		t.Fatalf("connect to PostgreSQL test database: %v", err)
	}
	if _, err := conn.Exec(ctx, fmt.Sprintf(
		"CREATE ROLE whatsapp_app LOGIN PASSWORD '%s' NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS NOINHERIT",
		appTestPassword,
	)); err != nil {
		_ = conn.Close(ctx)
		t.Fatalf("create restricted test role: %v", err)
	}
	if _, err := conn.Exec(ctx, "GRANT CONNECT ON DATABASE "+pgx.Identifier{databaseName}.Sanitize()+" TO whatsapp_app"); err != nil {
		_ = conn.Close(ctx)
		t.Fatalf("grant application database access: %v", err)
	}
	if err := conn.Close(ctx); err != nil {
		t.Fatalf("close PostgreSQL setup connection: %v", err)
	}

	appURL, err := url.Parse(migrationURL)
	if err != nil {
		t.Fatalf("parse PostgreSQL test connection: %v", err)
	}
	appURL.User = url.UserPassword("whatsapp_app", appTestPassword)

	return Postgres{MigrationURL: migrationURL, AppURL: appURL.String()}
}

func MigrationFS(t *testing.T, files map[string]string) fs.FS {
	t.Helper()
	result := fstest.MapFS{}
	for name, contents := range files {
		result[name] = &fstest.MapFile{Data: []byte(contents), Mode: 0o644}
	}
	return result
}
