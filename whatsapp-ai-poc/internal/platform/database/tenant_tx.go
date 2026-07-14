package database

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func WithPlatformTx[T any](ctx context.Context, pool *pgxpool.Pool, fn func(pgx.Tx) (T, error)) (T, error) {
	return withTx(ctx, pool, nil, fn)
}

func WithTenantTx[T any](ctx context.Context, pool *pgxpool.Pool, tenantID uuid.UUID, fn func(pgx.Tx) (T, error)) (T, error) {
	setup := func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "SELECT set_config('app.tenant_id', $1, true)", tenantID.String())
		return err
	}
	return withTx(ctx, pool, setup, fn)
}

func ResolveInvitationTenant(ctx context.Context, pool *pgxpool.Pool, tokenHash string) (uuid.UUID, error) {
	return withTx(ctx, pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "SELECT set_config('app.invitation_token_hash', $1, true)", tokenHash)
		return err
	}, func(tx pgx.Tx) (uuid.UUID, error) {
		var tenantID uuid.UUID
		err := tx.QueryRow(ctx,
			"SELECT tenant_id FROM member_invitations WHERE token_hash = $1", tokenHash,
		).Scan(&tenantID)
		return tenantID, err
	})
}

func withTx[T any](
	ctx context.Context,
	pool *pgxpool.Pool,
	setup func(pgx.Tx) error,
	fn func(pgx.Tx) (T, error),
) (T, error) {
	var zero T
	if pool == nil {
		return zero, wrapDatabaseError("begin transaction", pgx.ErrTxClosed)
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return zero, wrapDatabaseError("begin transaction", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	if setup != nil {
		if err := setup(tx); err != nil {
			return zero, wrapDatabaseError("set transaction context", err)
		}
	}
	value, err := fn(tx)
	if err != nil {
		return zero, err
	}
	if err := tx.Commit(ctx); err != nil {
		return zero, wrapDatabaseError("commit transaction", err)
	}
	return value, nil
}
