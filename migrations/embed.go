package migrations

import "embed"

// FS contains the immutable, ordered PostgreSQL migrations.
//
//go:embed *.sql
var FS embed.FS
