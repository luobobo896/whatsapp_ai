# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

WhatsApp AI 智能客服 — a multi-tenant WhatsApp customer service platform. Two components:

1. **Go management service** (Gin + pgx) — REST API, serves embedded React frontend
2. **React/Vite admin dashboard** — tenant/member/account/knowledge-base/conversation management

WhatsApp 消息收发由外部独立组件通过 CLI 对接，不在此项目内。

## Build, Test, and Verify

```bash
# Go (run from project root)
go test ./...                    # all tests (integration tests need Docker)
go test -race ./...              # race detector
go test ./internal/platform/config -v   # single package
go vet ./...
go build ./cmd/...

# Frontend (run from frontend/)
cd frontend && pnpm install
pnpm run build       # outputs to ../web/dist/ for Go embed
pnpm run dev         # Vite dev server (proxies /api to :8790)
pnpm run test        # vitest
pnpm run lint        # eslint

# Completion gate (all must pass with exit 0)
go test ./... && go test -race ./... && go vet ./... && go build ./cmd/... && go run ./cmd/import-legacy --dry-run
```

## Environment & Configuration

The service reads config exclusively from process environment variables (see `.env.example`). It never auto-loads `.env` files.

Three separate database URLs with no fallback chain:
- `DATABASE_ADMIN_URL` — `cmd/db-create` (create databases + restricted role)
- `DATABASE_MIGRATION_URL` — `cmd/db-migrate` (own schema, apply migrations)
- `DATABASE_URL` — server runtime (connects as restricted `whatsapp_app` role)

The `whatsapp_app` role must have `rolbypassrls=false` and `rolinherit=false`. Never give the server admin/migration credentials.

## Initialization Sequence

```bash
go run ./cmd/db-create          # creates whatsapp_ai + whatsapp_ai_test DBs, whatsapp_app role
go run ./cmd/db-migrate         # applies migrations with SHA-256 checksums
go run ./cmd/bootstrap-admin    # creates initial platform admin (refuses to overwrite)
go run ./cmd/import-legacy --dry-run  # read-only legacy data inventory (no --dry-run fails)
```

## Running

```bash
go run ./cmd/server             # Go backend on HTTP_HOST:PORT (default :8790)
```

The server serves the React SPA from embedded `web/dist/` (built by `frontend/` into `../web/dist`). `GET /health` returns `{"status":"ok","database":"up"}` with no leaked connection details.

WhatsApp 消息收发由外部独立组件对接，本项目仅负责管理端。

## Architecture

### Request Flow

```
Browser → Gin router
  ├── /api/*  → auth middleware (session cookie → Identity)
  │               → CSRF/Origin check for mutations
  │               → tenant RLS context (app.tenant_id)
  │               → handler → service → pgx pool
  ├── /health  → DB ping check
  └── /*       → embedded SPA (web/embed.go, fallback to index.html)
```

### Key Layers

- **`cmd/`** — CLI entry points. `cmd/server` wires config → pool → app.New() → http.Server with graceful shutdown.
- **`internal/app/app.go`** — assembles the Gin engine: global middleware (RequestID, Recovery, SecurityHeaders, RequestLogger), mounts auth/tenant/member/operations routes, SPA catch-all via `NoRoute`.
- **`internal/auth/`** — session-based auth with SHA-256 hashed tokens in `auth_sessions`. Login issues session cookie + CSRF token; all mutations require `Origin` match + `X-CSRF-Token`. Login rate limiter is in-memory per-IP with LRU eviction.
- **`internal/tenants/`** — tenant CRUD. `Create` auto-generates owner account with random password (returned once, never stored). `ListAccessible` honors both tenant membership and platform_admin role.
- **`internal/members/`** — 4 roles (owner/admin/agent/viewer) × 14 granular permissions. Invitation flow: create → token hash → accept with token.
- **`internal/operations/`** — accounts, knowledge bases, conversations (API surface for admin dashboard).
- **`internal/audit/`** — writes structured audit events with recursive JSON redaction.
- **`internal/legacy/`** — read-only SQLite inventory of pre-migration data.

### Database & Tenant Isolation

- **Migrations** are append-only SQL files in `migrations/` with SHA-256 checksums. Changing an applied migration causes the migrator to fail.
- **Tenant isolation** uses PostgreSQL RLS. Every tenant-scoped query must go through `database.WithTenantTx(ctx, pool, tenantID, fn)` which sets `app.tenant_id` via `SET LOCAL`. The `whatsapp_app` role has `rolbypassrls=false` so RLS policies (defined in `00002_tenant_rls.sql`) are enforced at the database level.
- **`database.DBTX`** is the common interface (`Exec`/`Query`/`QueryRow`) accepted by services. Use `pgxpool.Pool` for direct queries, `pgx.Tx` inside `WithPlatformTx`/`WithTenantTx`.

### Error Handling

All HTTP handlers use `httpx.Adapt(fn)`, converting `func(*gin.Context) error` to `gin.HandlerFunc`. Errors from `internal/platform/apperror` carry structured `Code` + `Status` + `Message`. The Adapt wrapper writes JSON `{"error":{"code":"...","message":"..."}}` with the matching HTTP status. Non-apperror errors become 500 with `INTERNAL_ERROR`.

### Testing

- **Integration tests** use `testkit.StartPostgres(t)` which spins a PostgreSQL 17 testcontainer, creates the `whatsapp_app` role, and returns separate migration/app connection URLs. Database names must end in `_test` or the helper panics.
- Tests call `database.Migrate(ctx, migrationURL, migrations.FS)` before using the pool.
- Unit tests for packages without DB dependencies (config, password hashing, permissions, error types) run without Docker.
- Frontend tests use `vitest` with `jsdom` environment.

### Frontend

React 19 + Vite 8. Single-page admin dashboard with dark green sidebar (WhatsApp brand colors: `#128C7E`/`#075E54`/`#25D366`). Styling in `styles.css`, no CSS framework. Components in `components.jsx`, main views in `dashboard.jsx`, API client in `api.js`. Built via `pnpm build` (outputs to `../web/dist/`) and embedded into the Go binary via `web/embed.go` (`//go:embed dist`).

## API Summary

```
POST  /api/auth/login           # email+password → session cookie + CSRF token
POST  /api/auth/logout          # revoke session
GET   /api/auth/me              # current identity + rotate CSRF
POST  /api/auth/select-tenant   # set active tenant on session
GET   /api/tenants              # list accessible tenants
POST  /api/platform/tenants     # platform_admin: create tenant
PATCH /api/platform/tenants/:id/status  # platform_admin: suspend/activate
GET   /api/accounts             # tenant-scoped WhatsApp accounts
POST  /api/accounts
GET   /api/knowledge/bases      # tenant-scoped knowledge bases
POST  /api/knowledge/bases
GET   /api/conversations        # tenant-scoped conversations
GET   /api/members              # tenant members
POST  /api/members/invitations  # invite member
POST  /api/invitations/:token/accept
PATCH /api/members/:userId
```

All mutation endpoints require `Origin` header matching `APP_ORIGIN` + valid `X-CSRF-Token` header.
