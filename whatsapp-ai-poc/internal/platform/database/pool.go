package database

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type DBTX interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

func OpenPool(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, wrapDatabaseError("parse database configuration", err)
	}

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, wrapDatabaseError("create database pool", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, wrapDatabaseError("ping database", err)
	}

	return pool, nil
}

type databaseError struct {
	operation string
	cause     error
}

func wrapDatabaseError(operation string, cause error) error {
	return &databaseError{operation: operation, cause: cause}
}

func (err *databaseError) Error() string {
	return fmt.Sprintf("%s failed", err.operation)
}

func (err *databaseError) Unwrap() error {
	return err.cause
}
