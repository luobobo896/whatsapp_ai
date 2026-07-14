package database_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"whatsapp-ai-poc/internal/platform/database"
	"whatsapp-ai-poc/internal/testkit"
	"whatsapp-ai-poc/migrations"
)

func TestTenantTransactionCannotCrossTenant(t *testing.T) {
	db := testkit.StartPostgres(t)
	if _, err := database.Migrate(t.Context(), db.MigrationURL, migrations.FS); err != nil {
		t.Fatal(err)
	}

	tenantA := uuid.MustParse("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa")
	tenantB := uuid.MustParse("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb")
	userID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	seedTenantIsolation(t, db.MigrationURL, tenantA, tenantB, userID)

	pool, err := database.OpenPool(t.Context(), db.AppURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	rows, err := database.WithTenantTx(t.Context(), pool, tenantA, func(tx pgx.Tx) ([]uuid.UUID, error) {
		query, err := tx.Query(t.Context(), "SELECT tenant_id FROM tenant_memberships ORDER BY tenant_id")
		if err != nil {
			return nil, err
		}
		defer query.Close()
		var tenantIDs []uuid.UUID
		for query.Next() {
			var tenantID uuid.UUID
			if err := query.Scan(&tenantID); err != nil {
				return nil, err
			}
			tenantIDs = append(tenantIDs, tenantID)
		}
		return tenantIDs, query.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0] != tenantA {
		t.Fatalf("tenant A transaction saw rows for %v", rows)
	}

	_, err = database.WithTenantTx(t.Context(), pool, tenantA, func(tx pgx.Tx) (struct{}, error) {
		_, err := tx.Exec(t.Context(), `
			INSERT INTO member_invitations
				(id, tenant_id, email, role, token_hash, expires_at, created_by)
			VALUES ($1, $2, 'cross@example.com', 'viewer', 'cross-hash', now() + interval '1 hour', $3)
		`, uuid.New(), tenantB, userID)
		return struct{}{}, err
	})
	if err == nil {
		t.Fatal("expected RLS to reject a cross-tenant insert")
	}
}

func TestInvitationLookupRevealsOnlyTenantID(t *testing.T) {
	db := testkit.StartPostgres(t)
	if _, err := database.Migrate(t.Context(), db.MigrationURL, migrations.FS); err != nil {
		t.Fatal(err)
	}

	tenantA := uuid.MustParse("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa")
	tenantB := uuid.MustParse("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb")
	userID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	seedTenantIsolation(t, db.MigrationURL, tenantA, tenantB, userID)

	admin, err := pgx.Connect(t.Context(), db.MigrationURL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(t.Context(), `
		INSERT INTO member_invitations
			(id, tenant_id, email, role, token_hash, expires_at, created_by)
		VALUES ($1, $2, 'invite@example.com', 'owner', 'known-hash', now() + interval '1 hour', $3)
	`, uuid.New(), tenantB, userID); err != nil {
		t.Fatal(err)
	}
	_ = admin.Close(t.Context())

	pool, err := database.OpenPool(t.Context(), db.AppURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	got, err := database.ResolveInvitationTenant(t.Context(), pool, "known-hash")
	if err != nil {
		t.Fatal(err)
	}
	if got != tenantB {
		t.Fatalf("resolved tenant %s, want %s", got, tenantB)
	}
	if _, err := database.ResolveInvitationTenant(t.Context(), pool, "wrong-hash"); err == nil {
		t.Fatal("expected unknown invitation hash to fail")
	}
}

func seedTenantIsolation(t *testing.T, databaseURL string, tenantA, tenantB, userID uuid.UUID) {
	t.Helper()
	conn, err := pgx.Connect(t.Context(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close(context.Background()) })
	batch := &pgx.Batch{}
	batch.Queue(`
		INSERT INTO users (id, email, display_name, password_hash, status)
		VALUES ($1, 'owner@example.com', 'Owner', 'test-hash', 'active')
	`, userID)
	batch.Queue(`
		INSERT INTO tenants (id, name, slug, status) VALUES
			($1, 'Tenant A', 'tenant-a', 'active'),
			($2, 'Tenant B', 'tenant-b', 'active')
	`, tenantA, tenantB)
	batch.Queue(`
		INSERT INTO tenant_memberships (tenant_id, user_id, role, status) VALUES
			($2, $1, 'owner', 'active'),
			($3, $1, 'owner', 'active')
	`, userID, tenantA, tenantB)
	results := conn.SendBatch(t.Context(), batch)
	if err := results.Close(); err != nil {
		t.Fatal(err)
	}
}
