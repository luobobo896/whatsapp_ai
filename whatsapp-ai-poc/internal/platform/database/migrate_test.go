package database_test

import (
	"context"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"whatsapp-ai-poc/internal/platform/database"
	"whatsapp-ai-poc/internal/testkit"
	"whatsapp-ai-poc/migrations"
)

func TestMigrationsAreRepeatableAndRestricted(t *testing.T) {
	db := testkit.StartPostgres(t)

	applied, err := database.Migrate(t.Context(), db.MigrationURL, migrations.FS)
	if err != nil {
		t.Fatal(err)
	}
	second, err := database.Migrate(t.Context(), db.MigrationURL, migrations.FS)
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 0 {
		t.Fatalf("second migration applied versions: %v", second)
	}
	if !slices.Contains(applied, "00001_foundation.sql") {
		t.Fatalf("foundation migration not applied: %v", applied)
	}

	conn, err := pgx.Connect(t.Context(), db.MigrationURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close(context.Background()) })

	for _, table := range []string{
		"users", "platform_roles", "tenants", "tenant_memberships",
		"member_invitations", "auth_sessions", "audit_logs",
	} {
		var exists bool
		if err := conn.QueryRow(t.Context(), `
			SELECT EXISTS (
				SELECT 1 FROM information_schema.tables
				WHERE table_schema = 'public' AND table_name = $1
			)`, table).Scan(&exists); err != nil {
			t.Fatal(err)
		}
		if !exists {
			t.Errorf("table %s does not exist", table)
		}
	}

	var superuser, createDB, createRole, bypassRLS, inherit bool
	if err := conn.QueryRow(t.Context(), `
		SELECT rolsuper, rolcreatedb, rolcreaterole, rolbypassrls, rolinherit
		FROM pg_roles WHERE rolname = 'whatsapp_app'
	`).Scan(&superuser, &createDB, &createRole, &bypassRLS, &inherit); err != nil {
		t.Fatal(err)
	}
	if superuser || createDB || createRole || bypassRLS || inherit {
		t.Fatalf("application role is privileged: super=%v createdb=%v createrole=%v bypassrls=%v inherit=%v",
			superuser, createDB, createRole, bypassRLS, inherit)
	}
	var ownsSchema, ledgerAccess bool
	if err := conn.QueryRow(t.Context(), `
		SELECT EXISTS (
			SELECT 1 FROM pg_namespace n JOIN pg_roles r ON r.oid = n.nspowner
			WHERE n.nspname = 'public' AND r.rolname = 'whatsapp_app'
		)
	`).Scan(&ownsSchema); err != nil {
		t.Fatal(err)
	}
	if err := conn.QueryRow(t.Context(), `
		SELECT has_table_privilege('whatsapp_app', 'schema_migrations', 'SELECT')
			OR has_table_privilege('whatsapp_app', 'schema_migrations', 'INSERT')
			OR has_table_privilege('whatsapp_app', 'schema_migrations', 'UPDATE')
			OR has_table_privilege('whatsapp_app', 'schema_migrations', 'DELETE')
	`).Scan(&ledgerAccess); err != nil {
		t.Fatal(err)
	}
	if ownsSchema || ledgerAccess {
		t.Fatalf("application role owns schema or can modify migration ledger: owner=%v ledger=%v", ownsSchema, ledgerAccess)
	}
}

func TestMigrationsRejectChangedChecksum(t *testing.T) {
	db := testkit.StartPostgres(t)
	files := testkit.MigrationFS(t, map[string]string{"00001_sample.sql": "CREATE TABLE sample (id int);"})
	if _, err := database.Migrate(t.Context(), db.MigrationURL, files); err != nil {
		t.Fatal(err)
	}

	changed := testkit.MigrationFS(t, map[string]string{"00001_sample.sql": "CREATE TABLE sample (id bigint);"})
	if _, err := database.Migrate(t.Context(), db.MigrationURL, changed); err == nil {
		t.Fatal("expected a changed migration checksum to fail")
	}
}
