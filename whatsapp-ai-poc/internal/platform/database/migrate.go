package database

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

func Migrate(ctx context.Context, databaseURL string, files fs.FS) ([]string, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, wrapDatabaseError("create migration pool", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		return nil, wrapDatabaseError("ping migration database", err)
	}

	if _, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version text PRIMARY KEY,
			checksum text NOT NULL,
			applied_at timestamptz NOT NULL DEFAULT now()
		)
	`); err != nil {
		return nil, wrapDatabaseError("create migration ledger", err)
	}

	entries, err := fs.ReadDir(files, ".")
	if err != nil {
		return nil, wrapDatabaseError("read migration files", err)
	}
	versions := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			versions = append(versions, entry.Name())
		}
	}
	slices.Sort(versions)

	applied := make([]string, 0, len(versions))
	for _, version := range versions {
		contents, err := fs.ReadFile(files, version)
		if err != nil {
			return nil, wrapDatabaseError("read migration "+version, err)
		}
		digest := sha256.Sum256(contents)
		checksum := hex.EncodeToString(digest[:])

		var stored string
		err = pool.QueryRow(ctx, "SELECT checksum FROM schema_migrations WHERE version = $1", version).Scan(&stored)
		if err == nil {
			if stored != checksum {
				return nil, wrapDatabaseError("validate migration checksum for "+version, fmt.Errorf("checksum mismatch"))
			}
			continue
		}
		if !isNoRows(err) {
			return nil, wrapDatabaseError("read migration ledger", err)
		}

		tx, err := pool.Begin(ctx)
		if err != nil {
			return nil, wrapDatabaseError("begin migration "+version, err)
		}
		if _, err := tx.Exec(ctx, string(contents)); err != nil {
			_ = tx.Rollback(ctx)
			return nil, wrapDatabaseError("apply migration "+version, err)
		}
		if _, err := tx.Exec(ctx,
			"INSERT INTO schema_migrations (version, checksum) VALUES ($1, $2)", version, checksum,
		); err != nil {
			_ = tx.Rollback(ctx)
			return nil, wrapDatabaseError("record migration "+version, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, wrapDatabaseError("commit migration "+version, err)
		}
		applied = append(applied, version)
	}

	return applied, nil
}
