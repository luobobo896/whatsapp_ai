# WhatsApp AI Go Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the PostgreSQL, tenant, authentication, RBAC, audit, and legacy-inventory foundation for the approved WhatsApp AI commercial MVP with a Go backend.

**Architecture:** Build a Gin modular monolith whose handlers only adapt HTTP, whose services own validation, authorization, and transactions, and whose pgx repositories execute parameterized SQL. PostgreSQL is the only new business source of truth; every tenant operation sets a transaction-local tenant context and is also protected by forced Row Level Security.

**Tech Stack:** Go 1.26, Gin, pgx/v5, PostgreSQL 17, `golang.org/x/crypto/scrypt`, `golang.org/x/time/rate`, Google UUID, testcontainers-go, Go `testing` and `httptest`

**Source design:** `docs/superpowers/specs/2026-07-13-whatsapp-ai-go-foundation-design.md`

---

## Scope

This plan delivers implementation stage 1 only:

- strict Go runtime configuration and production entry points;
- PostgreSQL bootstrap, checksummed migrations, grants, and forced RLS;
- stable Gin API errors, request IDs, security middleware, and health checks;
- passwords, opaque server-side sessions, CSRF, Origin checks, and login rate limiting;
- platform administrators, tenant memberships, invitations, and four-role RBAC;
- transactional and recursively redacted audit writes;
- read-only deterministic JSON/SQLite legacy inventory;
- Go deployment and test documentation.

OpenClaw event handling, knowledge and conversation services, SSE, React/Vite screens, and formal legacy import remain later stages. Existing JavaScript is migration reference only and is not a runtime or test path.

## File Map

```text
go.mod, go.sum                              Go module and locked dependencies
.env.example                               Non-secret environment contract
cmd/server/main.go                         Production Gin server and shutdown
cmd/db-create/main.go                      Database and restricted role bootstrap
cmd/db-migrate/main.go                     Migration-only CLI
cmd/bootstrap-admin/main.go                Platform administrator bootstrap
cmd/import-legacy/main.go                  Read-only legacy inventory CLI
internal/app/app.go                        Gin composition root
internal/platform/config/config.go         Strict environment parsing
internal/platform/apperror/error.go        Stable application errors
internal/platform/httpx/handler.go          One error-to-response adapter
internal/platform/httpx/middleware.go       Request ID, recovery, headers, logging
internal/platform/database/pool.go          pgx pool construction
internal/platform/database/migrate.go       Checksummed migration runner
internal/platform/database/tenant_tx.go     Platform and tenant transactions
internal/audit/redact.go                    Recursive secret redaction
internal/audit/service.go                   Transactional audit inserts
internal/auth/password.go                   Scrypt password storage
internal/auth/session.go                    Session issue, resolve, revoke, select
internal/auth/middleware.go                 Authentication, Origin, and CSRF
internal/auth/handler.go                    Login/logout/me/select-tenant routes
internal/members/permissions.go             Four-role permission matrix
internal/members/middleware.go              Platform and tenant authorization
internal/members/service.go                 Invitation and membership rules
internal/members/handler.go                 Member APIs
internal/tenants/service.go                 Platform tenant lifecycle
internal/tenants/handler.go                 Platform tenant APIs
internal/legacy/inventory.go                Structured legacy dry-run
internal/testkit/postgres.go                Guarded PostgreSQL testcontainer
migrations/embed.go                        Embedded migration filesystem
migrations/00001_foundation.sql             Foundation schema and restricted grants
migrations/00002_tenant_rls.sql              Immutable tenant isolation policies
test/fixtures/legacy/accounts.json          Deterministic account fixture
test/fixtures/legacy/knowledge.json         Deterministic knowledge fixture
docs/deployment/postgresql.md               Go/PostgreSQL runbook
docs/testing/foundation.md                  Exact verification commands
```

## Task 1: Establish the Go Runtime and Configuration Contract

**Files:**
- Create: `go.mod`
- Create: `go.sum`
- Modify: `.env.example`
- Create: `internal/platform/config/config.go`
- Create: `internal/platform/config/config_test.go`

- [ ] **Step 1: Initialize the module and test dependencies**

Run:

```bash
go mod init whatsapp-ai-poc
go get github.com/gin-gonic/gin@latest
go get github.com/google/uuid@latest
go get github.com/jackc/pgx/v5@latest
go get github.com/testcontainers/testcontainers-go/modules/postgres@latest
go get golang.org/x/crypto@latest
go get golang.org/x/time@latest
go get modernc.org/sqlite@latest
```

Expected: `go.mod` is created and all selected modules are pinned by `go.sum`.

- [ ] **Step 2: Write failing configuration tests**

Create table-driven tests around this public contract:

```go
func TestParseAcceptsRuntimeContract(t *testing.T) {
	getenv := envGetter(map[string]string{
		"APP_ENV": "test", "HTTP_HOST": "127.0.0.1", "PORT": "8790",
		"APP_ORIGIN": "http://localhost:8790",
		"DATABASE_URL": "postgres://whatsapp_app:secret@localhost:5432/whatsapp_ai_test",
		"SESSION_COOKIE_NAME": "wa_session", "SESSION_TTL_HOURS": "12",
		"LOGIN_RATE_LIMIT": "5", "LOGIN_RATE_WINDOW_SECONDS": "60",
	})
	cfg, err := Parse(getenv)
	if err != nil { t.Fatal(err) }
	if cfg.Port != 8790 || cfg.SessionTTL != 12*time.Hour || cfg.Production {
		t.Fatalf("unexpected config: %#v", cfg)
	}
}

func TestParseRejectsUnsafeValues(t *testing.T) {
	for _, key := range []string{"DATABASE_URL", "APP_ORIGIN"} {
		t.Run(key, func(t *testing.T) {
			env := validTestEnv()
			env[key] = ""
			if _, err := Parse(envGetter(env)); err == nil {
				t.Fatalf("expected %s error", key)
			}
		})
	}
}
```

- [ ] **Step 3: Run the tests and verify RED**

Run: `go test ./internal/platform/config -run TestParse -v`

Expected: FAIL because package `internal/platform/config` does not exist.

- [ ] **Step 4: Implement strict parsing**

Implement this immutable-by-convention value and parser:

```go
type Config struct {
	Environment string
	Production bool
	Host string
	Port int
	AppOrigin *url.URL
	DatabaseURL string
	SessionCookieName string
	SessionTTL time.Duration
	LoginRateLimit int
	LoginRateWindow time.Duration
}

func Parse(getenv func(string) string) (Config, error)
```

Accept only `development`, `test`, and `production`; require an HTTP(S)
`APP_ORIGIN` without credentials; require `DATABASE_URL`; restrict ports to
1-65535, cookie names to ASCII letters/numbers/underscore, session TTL to
1-168 hours, and rate settings to positive integers. Do not split or expose
database credentials as fields.

- [ ] **Step 5: Document only variable names and local placeholders**

Write `.env.example` with `APP_ENV`, `HTTP_HOST`, `PORT`, `APP_ORIGIN`,
`DATABASE_URL`, `DATABASE_ADMIN_URL`, `DATABASE_MIGRATION_URL`,
`DATABASE_APP_PASSWORD`, `SESSION_COOKIE_NAME`, `SESSION_TTL_HOURS`,
`LOGIN_RATE_LIMIT`, `LOGIN_RATE_WINDOW_SECONDS`, `BOOTSTRAP_ADMIN_EMAIL`, and
`BOOTSTRAP_ADMIN_PASSWORD`. Runtime uses only `DATABASE_URL`; migration and
admin CLIs refuse to fall back to it.

- [ ] **Step 6: Verify GREEN and commit**

Run: `go test ./internal/platform/config -v`

Expected: all configuration tests pass.

```bash
git add whatsapp-ai-poc/go.mod whatsapp-ai-poc/go.sum whatsapp-ai-poc/.env.example whatsapp-ai-poc/internal/platform/config
git commit -m "build: establish Go backend runtime"
```

## Task 2: Bootstrap PostgreSQL and Apply Checksummed Migrations

**Files:**
- Create: `cmd/db-create/main.go`
- Create: `cmd/db-migrate/main.go`
- Create: `internal/platform/database/pool.go`
- Create: `internal/platform/database/migrate.go`
- Create: `internal/platform/database/migrate_test.go`
- Create: `internal/testkit/postgres.go`
- Create: `migrations/embed.go`
- Create: `migrations/00001_foundation.sql`

- [ ] **Step 1: Write the guarded migration integration test**

Use testcontainers-go to start PostgreSQL with database `whatsapp_ai_test`,
create restricted login role `whatsapp_app`, and return separate migration and
application URLs. The helper must fail before connecting if a configured test
database name does not end in `_test`.

```go
func TestMigrationsAreRepeatableAndRestricted(t *testing.T) {
	db := testkit.StartPostgres(t)
	applied, err := database.Migrate(t.Context(), db.MigrationURL, migrations.FS)
	if err != nil { t.Fatal(err) }
	if diff, err := database.Migrate(t.Context(), db.MigrationURL, migrations.FS); err != nil || len(diff) != 0 {
		t.Fatalf("second migration changed schema: %v %v", diff, err)
	}
	assertTables(t, db.MigrationURL, "users", "platform_roles", "tenants",
		"tenant_memberships", "member_invitations", "auth_sessions", "audit_logs")
	assertRestrictedRole(t, db.MigrationURL, "whatsapp_app")
	if !slices.Contains(applied, "00001_foundation.sql") {
		t.Fatalf("unexpected versions: %v", applied)
	}
}
```

- [ ] **Step 2: Run the test and verify RED**

Run: `go test ./internal/platform/database -run TestMigrations -v`

Expected: FAIL because the database, migration, and testkit packages are absent.

- [ ] **Step 3: Implement pool and checksum migration APIs**

```go
func OpenPool(ctx context.Context, databaseURL string) (*pgxpool.Pool, error)
func Migrate(ctx context.Context, databaseURL string, files fs.FS) ([]string, error)

type DBTX interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}
```

`Migrate` creates `schema_migrations(version text primary key, checksum text not
null, applied_at timestamptz not null default now())`, loads sorted `*.sql`
files, computes SHA-256, applies every new file in its own transaction, and
rejects a checksum change for an applied version. Return only newly applied
file names.

Expose migrations through:

```go
//go:embed *.sql
var FS embed.FS
```

- [ ] **Step 4: Create the exact foundation schema**

The migration creates UUID-keyed `users`, `platform_roles`, `tenants`,
`tenant_memberships`, `member_invitations`, `auth_sessions`, and `audit_logs`.
Use check constraints for all statuses and roles. Include `users.password_changed_at`,
`member_invitations.revoked_at`, token-hash unique constraints, a partial unique
open-invitation index, and tenant/time audit indexes.

```sql
CREATE TABLE users (
  id uuid PRIMARY KEY,
  email text NOT NULL,
  display_name text NOT NULL,
  password_hash text NOT NULL,
  status text NOT NULL CHECK (status IN ('active', 'disabled')),
  password_changed_at timestamptz NOT NULL DEFAULT now(),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX users_email_unique ON users (lower(email));

CREATE TABLE platform_roles (
  user_id uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  role text NOT NULL CHECK (role = 'platform_admin'),
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE tenants (
  id uuid PRIMARY KEY,
  name text NOT NULL,
  slug text NOT NULL UNIQUE,
  status text NOT NULL CHECK (status IN ('active', 'suspended')),
  suspended_reason text,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE tenant_memberships (
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  role text NOT NULL CHECK (role IN ('owner', 'admin', 'agent', 'viewer')),
  status text NOT NULL CHECK (status IN ('active', 'disabled')),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, user_id)
);

CREATE TABLE member_invitations (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  email text NOT NULL,
  role text NOT NULL CHECK (role IN ('owner', 'admin', 'agent', 'viewer')),
  token_hash text NOT NULL UNIQUE,
  expires_at timestamptz NOT NULL,
  accepted_at timestamptz,
  revoked_at timestamptz,
  created_by uuid NOT NULL REFERENCES users(id),
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX member_invitations_open_unique
  ON member_invitations (tenant_id, lower(email))
  WHERE accepted_at IS NULL AND revoked_at IS NULL;

CREATE TABLE auth_sessions (
  id uuid PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token_hash text NOT NULL UNIQUE,
  csrf_hash text NOT NULL,
  active_tenant_id uuid REFERENCES tenants(id),
  expires_at timestamptz NOT NULL,
  revoked_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE audit_logs (
  id uuid PRIMARY KEY,
  tenant_id uuid REFERENCES tenants(id) ON DELETE CASCADE,
  actor_user_id uuid REFERENCES users(id),
  actor_role text,
  action text NOT NULL,
  target_type text NOT NULL,
  target_id text NOT NULL,
  request_id text NOT NULL,
  result text NOT NULL CHECK (result IN ('success', 'failure')),
  change_summary jsonb NOT NULL DEFAULT '{}'::jsonb,
  ip inet,
  user_agent text,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX audit_logs_tenant_created_idx ON audit_logs (tenant_id, created_at DESC);

GRANT USAGE ON SCHEMA public TO whatsapp_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO whatsapp_app;
```

- [ ] **Step 5: Implement database CLIs**

`cmd/db-create` requires `DATABASE_ADMIN_URL` and `DATABASE_APP_PASSWORD`. It
creates fixed role `whatsapp_app` and fixed databases `whatsapp_ai` and
`whatsapp_ai_test` only when absent, using
`NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS NOINHERIT`. It prints only
role/database names and created/already-exists state.

`cmd/db-migrate` requires `DATABASE_MIGRATION_URL`, calls `Migrate`, and prints
only applied versions. Neither CLI logs URLs or passwords.

- [ ] **Step 6: Verify GREEN and commit**

Run: `go test ./internal/platform/database -run TestMigrations -v`

Expected: migrations apply twice, all tables exist, and the application role is restricted.

```bash
git add whatsapp-ai-poc/cmd/db-create whatsapp-ai-poc/cmd/db-migrate whatsapp-ai-poc/internal/platform/database whatsapp-ai-poc/internal/testkit whatsapp-ai-poc/migrations
git commit -m "feat: add PostgreSQL foundation schema"
```

## Task 3: Enforce Tenant Isolation with Forced RLS

**Files:**
- Create: `migrations/00002_tenant_rls.sql`
- Create: `internal/platform/database/tenant_tx.go`
- Create: `internal/platform/database/tenant_tx_test.go`

- [ ] **Step 1: Write cross-tenant failing tests**

Seed tenants A and B as the migration role, then query as `whatsapp_app`:

```go
func TestTenantTransactionCannotCrossTenant(t *testing.T) {
	db := migratedDatabase(t)
	seedTwoTenants(t, db.MigrationURL)
	rows, err := database.WithTenantTx(t.Context(), db.AppPool, tenantA, func(tx pgx.Tx) ([]membership, error) {
		return listMemberships(t.Context(), tx)
	})
	if err != nil { t.Fatal(err) }
	for _, row := range rows {
		if row.TenantID != tenantA { t.Fatalf("cross-tenant row: %#v", row) }
	}

	_, err = database.WithTenantTx(t.Context(), db.AppPool, tenantA, func(tx pgx.Tx) (struct{}, error) {
		_, err := tx.Exec(t.Context(), crossTenantInvitationSQL, invitationID, tenantB)
		return struct{}{}, err
	})
	if err == nil { t.Fatal("expected RLS write rejection") }
}
```

- [ ] **Step 2: Run the test and verify RED**

Run: `go test ./internal/platform/database -run TestTenantTransactionCannotCrossTenant -v`

Expected: FAIL because `WithTenantTx` and RLS policies are absent.

- [ ] **Step 3: Implement transaction helpers**

```go
func WithPlatformTx[T any](ctx context.Context, pool *pgxpool.Pool, fn func(pgx.Tx) (T, error)) (T, error)
func WithTenantTx[T any](ctx context.Context, pool *pgxpool.Pool, tenantID uuid.UUID, fn func(pgx.Tx) (T, error)) (T, error)
func ResolveInvitationTenant(ctx context.Context, pool *pgxpool.Pool, tokenHash string) (uuid.UUID, error)
```

`WithTenantTx` begins a transaction, runs
`SELECT set_config('app.tenant_id', $1, true)`, invokes the callback, and commits.
Any error rolls back. Never set tenant context at pool/session scope.

`ResolveInvitationTenant` starts a short read transaction, sets only
`app.invitation_token_hash`, and selects the tenant ID of the invitation whose
hash exactly matches. It returns no invitation fields. Acceptance then opens a
normal `WithTenantTx` transaction, locks and revalidates the invitation, and
performs all writes atomically.

- [ ] **Step 4: Add and force exact RLS policies**

```sql
CREATE FUNCTION app_current_tenant_id() RETURNS uuid
LANGUAGE sql STABLE AS $$
  SELECT NULLIF(current_setting('app.tenant_id', true), '')::uuid
$$;

ALTER TABLE tenant_memberships ENABLE ROW LEVEL SECURITY;
ALTER TABLE tenant_memberships FORCE ROW LEVEL SECURITY;
CREATE POLICY tenant_memberships_tenant ON tenant_memberships
  USING (tenant_id = app_current_tenant_id())
  WITH CHECK (tenant_id = app_current_tenant_id());

ALTER TABLE member_invitations ENABLE ROW LEVEL SECURITY;
ALTER TABLE member_invitations FORCE ROW LEVEL SECURITY;
CREATE POLICY member_invitations_tenant ON member_invitations
  USING (tenant_id = app_current_tenant_id())
  WITH CHECK (tenant_id = app_current_tenant_id());
CREATE POLICY member_invitations_token_lookup ON member_invitations
  FOR SELECT USING (
    token_hash = NULLIF(current_setting('app.invitation_token_hash', true), '')
  );

ALTER TABLE audit_logs ENABLE ROW LEVEL SECURITY;
ALTER TABLE audit_logs FORCE ROW LEVEL SECURITY;
CREATE POLICY audit_logs_tenant ON audit_logs
  USING (tenant_id = app_current_tenant_id())
  WITH CHECK (tenant_id = app_current_tenant_id());
CREATE POLICY audit_logs_platform_insert ON audit_logs
  FOR INSERT WITH CHECK (tenant_id IS NULL);
CREATE POLICY audit_logs_platform_select ON audit_logs
  FOR SELECT USING (tenant_id IS NULL AND app_current_tenant_id() IS NULL);
```

- [ ] **Step 5: Verify GREEN and commit**

Run: `go test ./internal/platform/database -run TestTenantTransactionCannotCrossTenant -v`

Expected: tenant A sees only tenant A rows and cannot write tenant B rows.

```bash
git add whatsapp-ai-poc/migrations/00002_tenant_rls.sql whatsapp-ai-poc/internal/platform/database/tenant_tx.go whatsapp-ai-poc/internal/platform/database/tenant_tx_test.go
git commit -m "feat: enforce tenant row isolation"
```

## Task 4: Compose Gin, Stable Errors, and Audit Writes

**Files:**
- Create: `internal/platform/apperror/error.go`
- Create: `internal/platform/httpx/handler.go`
- Create: `internal/platform/httpx/middleware.go`
- Create: `internal/audit/redact.go`
- Create: `internal/audit/redact_test.go`
- Create: `internal/audit/service.go`
- Create: `internal/app/app.go`
- Create: `internal/app/app_test.go`

- [ ] **Step 1: Write failing audit and HTTP contract tests**

```go
func TestRedactSecretsNested(t *testing.T) {
	got := audit.Redact(map[string]any{"label": "Model", "credentials": map[string]any{"apiKey": "real"}})
	want := map[string]any{"label": "Model", "credentials": map[string]any{"apiKey": "[REDACTED]"}}
	if !reflect.DeepEqual(got, want) { t.Fatalf("got %#v", got) }
}

func TestUnknownRouteHasStableErrorAndRequestID(t *testing.T) {
	r := app.New(testConfig(), nil, fakePinger{err: nil})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/missing", nil))
	assertAPIError(t, w, http.StatusNotFound, "NOT_FOUND")
}
```

- [ ] **Step 2: Run tests and verify RED**

Run: `go test ./internal/audit ./internal/app -v`

Expected: FAIL because the packages do not exist.

- [ ] **Step 3: Implement stable error and Gin adapter contracts**

```go
type Error struct { Code string; Status int; Message string; Details any; Cause error }
func E(code string, status int, message string, cause error) *Error
type Handler func(*gin.Context) error
func Adapt(handler Handler) gin.HandlerFunc
```

`Adapt` writes `{error:{code,message,requestId}}`; unexpected errors are logged
once with request ID and become `INTERNAL_ERROR`. Add named constructors for
`AUTH_REQUIRED`, `SESSION_EXPIRED`, `AUTH_INVALID`, `FORBIDDEN`,
`TENANT_SUSPENDED`, `CONFLICT`, `VALIDATION_FAILED`, `RATE_LIMITED`, and
`NOT_FOUND`. Recovery uses the same response contract.

- [ ] **Step 4: Implement Gin composition and audit APIs**

```go
type Pinger interface { Ping(context.Context) error }
func New(cfg config.Config, pool *pgxpool.Pool, pinger Pinger) *gin.Engine
func Redact(value any) any
func Write(ctx context.Context, tx pgx.Tx, event audit.Event) error
```

Register request IDs prefixed `req_`, recovery, structured request logging,
security headers, `GET /health`, and a JSON no-route handler. Health may say
`database: up|down` but cannot expose host or connection data. `Write` uses its
caller transaction and marshals only redacted summaries.

- [ ] **Step 5: Verify GREEN and commit**

Run: `go test ./internal/audit ./internal/app -v`

Expected: nested secrets are redacted and HTTP errors have a request ID.

```bash
git add whatsapp-ai-poc/internal/platform/apperror whatsapp-ai-poc/internal/platform/httpx whatsapp-ai-poc/internal/audit whatsapp-ai-poc/internal/app
git commit -m "feat: add Gin errors and audit foundation"
```

## Task 5: Implement Passwords and Server-Side Sessions

**Files:**
- Create: `internal/auth/password.go`
- Create: `internal/auth/password_test.go`
- Create: `internal/auth/session.go`
- Create: `internal/auth/middleware.go`
- Create: `internal/auth/handler.go`
- Create: `internal/auth/auth_test.go`
- Create: `cmd/bootstrap-admin/main.go`
- Modify: `internal/app/app.go`

- [ ] **Step 1: Write failing password and HTTP authentication tests**

Cover correct and incorrect passwords, malformed hashes, HttpOnly/SameSite
cookies, expired/revoked sessions, logout revocation, Origin and CSRF rejection,
and 429 after the configured login threshold.

```go
func TestPasswordRoundTripAndMalformedHash(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil { t.Fatal(err) }
	if !VerifyPassword(hash, "correct horse battery staple") { t.Fatal("password mismatch") }
	if VerifyPassword(hash, "wrong") || VerifyPassword("broken", "wrong") { t.Fatal("unsafe match") }
}

func TestLoginAndCSRFContract(t *testing.T) {
	server := newAuthTestServer(t, authFixture())
	login := server.PostJSON("/api/auth/login", map[string]string{"email": "admin@example.com", "password": "secret"})
	login.AssertStatus(http.StatusOK)
	login.AssertCookie("wa_session", true, http.SameSiteLaxMode)
	csrf := login.JSONString("csrfToken")
	server.PostJSON("/api/auth/logout", nil).AssertError(http.StatusForbidden, "FORBIDDEN")
	server.PostJSONWithHeaders("/api/auth/logout", nil, map[string]string{"Origin": server.Origin, "X-CSRF-Token": csrf}).AssertStatus(http.StatusNoContent)
}
```

- [ ] **Step 2: Run tests and verify RED**

Run: `go test ./internal/auth -v`

Expected: FAIL because password, session, middleware, and handlers are absent.

- [ ] **Step 3: Implement scrypt and opaque session primitives**

Store `scrypt$N=16384,r=8,p=1$<base64-salt>$<base64-key>`. Generate 16-byte
password salts, 32-byte session tokens, and 32-byte CSRF tokens with
`crypto/rand`. Compare derived keys and token hashes with `subtle.ConstantTimeCompare`.

```go
func HashPassword(password string) (string, error)
func VerifyPassword(encoded, password string) bool
func Issue(ctx context.Context, db database.DBTX, userID uuid.UUID, ttl time.Duration) (SessionTokens, error)
func Resolve(ctx context.Context, pool *pgxpool.Pool, rawToken string) (Identity, error)
func Revoke(ctx context.Context, pool *pgxpool.Pool, rawToken string) error
func SelectTenant(ctx context.Context, pool *pgxpool.Pool, sessionID, tenantID uuid.UUID) error
func RotateCSRF(ctx context.Context, pool *pgxpool.Pool, sessionID uuid.UUID) (string, error)
```

Resolution joins users, rejects disabled users, expiry, revocation, and sessions
created before `password_changed_at`.

- [ ] **Step 4: Implement middleware and routes**

Register `POST /api/auth/login`, `POST /api/auth/logout`, `GET /api/auth/me`,
and `POST /api/auth/select-tenant`. Login returns the raw CSRF token and sets
the session cookie with `HttpOnly`, `SameSite=Lax`, path `/`, production-only
`Secure`, and configured max age. `me` atomically rotates the CSRF token hash
and returns the new raw token, so no raw CSRF value is stored. Wrong email and
wrong password both return `AUTH_INVALID`.

Authentication places `Identity` in Gin context. For POST/PUT/PATCH/DELETE,
except login and invitation acceptance, require exact configured Origin and
valid `X-CSRF-Token`. Rate-limit login by normalized client IP with a bounded,
periodically pruned `x/time/rate` map.

- [ ] **Step 5: Implement bootstrap administrator CLI**

Require `BOOTSTRAP_ADMIN_EMAIL`, `BOOTSTRAP_ADMIN_PASSWORD`, and `DATABASE_URL`.
Normalize email, hash the password, and create the active user plus
`platform_admin` row in one transaction. Refuse to overwrite any existing
email and print only email and user ID.

- [ ] **Step 6: Verify GREEN and commit**

Run: `go test ./internal/auth -v`

Expected: password, session, cookie, CSRF, Origin, logout, and rate-limit tests pass.

```bash
git add whatsapp-ai-poc/internal/auth whatsapp-ai-poc/internal/app/app.go whatsapp-ai-poc/cmd/bootstrap-admin
git commit -m "feat: add secure Go administrator sessions"
```

## Task 6: Implement the Four-Role Permission Matrix

**Files:**
- Create: `internal/members/permissions.go`
- Create: `internal/members/permissions_test.go`
- Create: `internal/members/middleware.go`
- Create: `internal/members/middleware_test.go`
- Modify: `internal/app/app.go`

- [ ] **Step 1: Write the complete failing permission matrix**

Test every role against every stable permission:

```go
var Permissions = []Permission{
	"members:manage", "members:read", "accounts:manage", "accounts:read",
	"knowledge:manage", "knowledge:read", "models:manage",
	"conversations:manage", "conversations:read", "customers:read",
	"settings:manage", "metrics:read", "alerts:read", "audit:read",
}

// owner: all
// admin: all except members:manage
// agent: accounts:read, knowledge:read, conversations:manage,
//        conversations:read, customers:read, metrics:read, alerts:read
// viewer: accounts:read, metrics:read, alerts:read, audit:read
```

The test loops over all four roles and all 14 permissions and compares each
result with an explicit expected map.

- [ ] **Step 2: Run tests and verify RED**

Run: `go test ./internal/members -run 'TestPermission|TestRequire' -v`

Expected: FAIL because permission and authorization middleware are absent.

- [ ] **Step 3: Implement permission and authorization APIs**

```go
func HasPermission(role Role, permission Permission) bool
func RequirePlatformAdmin(pool *pgxpool.Pool) gin.HandlerFunc
func RequirePermission(pool *pgxpool.Pool, permission Permission) gin.HandlerFunc
```

`RequirePlatformAdmin` verifies an authenticated user's platform role.
`RequirePermission` uses the session-selected tenant, opens `WithTenantTx`,
loads active membership and tenant status, rejects suspended tenants, checks
the matrix, and attaches `{TenantID, UserID, Role}` to Gin context.

- [ ] **Step 4: Verify GREEN and commit**

Run: `go test ./internal/members -run 'TestPermission|TestRequire' -v`

Expected: all 56 matrix cases and middleware allow/deny cases pass.

```bash
git add whatsapp-ai-poc/internal/members/permissions.go whatsapp-ai-poc/internal/members/permissions_test.go whatsapp-ai-poc/internal/members/middleware.go whatsapp-ai-poc/internal/members/middleware_test.go whatsapp-ai-poc/internal/app/app.go
git commit -m "feat: enforce tenant role permissions"
```

## Task 7: Implement Tenant Creation, Invitations, and Member Rules

**Files:**
- Create: `internal/tenants/service.go`
- Create: `internal/tenants/handler.go`
- Create: `internal/tenants/tenants_test.go`
- Create: `internal/members/service.go`
- Create: `internal/members/handler.go`
- Modify: `internal/app/app.go`

- [ ] **Step 1: Write failing tenant lifecycle integration tests**

Cover platform-only tenant creation, atomic owner invitation and audit, one-time
raw invitation tokens, hashed storage, acceptance, expired/accepted/revoked/
wrong-email rejection, owner-only member management, last-owner protection,
tenant suspension, and rejection of any forged tenant ID sent by the frontend.

```go
func TestTenantLifecycle(t *testing.T) {
	s := newFoundationServer(t)
	platform := s.LoginPlatformAdmin()
	created := platform.Post("/api/platform/tenants", map[string]string{
		"name": "Acme", "slug": "acme", "ownerEmail": "owner@example.com", "ownerDisplayName": "Owner",
	}).AssertStatus(http.StatusCreated)
	created.AssertJSONPath("invitation.role", "owner")
	created.AssertJSONPathPresent("invitation.token")
	s.AssertInvitationTokenIsHashed(created.JSONString("invitation.token"))
	s.AcceptInvitation(created.JSONString("invitation.token"), "owner@example.com", "Owner", "safe password")
	s.AssertMembership("acme", "owner@example.com", "owner", "active")
}
```

- [ ] **Step 2: Run tests and verify RED**

Run: `go test ./internal/tenants ./internal/members -run TestTenantLifecycle -v`

Expected: FAIL because tenant/member services and handlers are absent.

- [ ] **Step 3: Implement validated tenant creation**

```go
type CreateInput struct { Name, Slug, OwnerEmail, OwnerDisplayName string }
type Created struct { Tenant Tenant; Invitation members.IssuedInvitation }
func (s *Service) Create(ctx context.Context, actor audit.Actor, input CreateInput) (Created, error)
func (s *Service) SetStatus(ctx context.Context, actor audit.Actor, tenantID uuid.UUID, status, reason string) error
```

Trim names, normalize email to lowercase, and require slug to match
`^[a-z0-9]+(?:-[a-z0-9]+)*$`. In one platform transaction insert the tenant,
set the new tenant context, create a 32-byte owner invitation whose SHA-256
hash is stored, and write audit. Return the raw token once. `SetStatus` accepts
only `active` or `suspended` and requires a non-empty reason for suspension.

- [ ] **Step 4: Implement invitation and membership services**

```go
func (s *Service) Invite(ctx context.Context, tenant TenantContext, input InviteInput) (IssuedInvitation, error)
func (s *Service) Accept(ctx context.Context, token string, input AcceptInput) (auth.SessionTokens, error)
func (s *Service) List(ctx context.Context, tenant TenantContext) ([]Member, error)
func (s *Service) Update(ctx context.Context, tenant TenantContext, userID uuid.UUID, input UpdateInput) error
```

All tenant operations use `WithTenantTx`. Acceptance hashes the raw token, uses
`ResolveInvitationTenant` to obtain only its tenant ID, then opens a tenant
transaction that locks the invitation, rechecks token hash, email, expiry, and
accepted/revoked state, creates or activates the user and membership, marks
`accepted_at`, calls transaction-aware `auth.Issue`, and audits before commit.
`Update` locks active owners and returns `CONFLICT` when it would disable or
demote the final owner.

- [ ] **Step 5: Register the exact APIs**

```text
POST  /api/platform/tenants
PATCH /api/platform/tenants/:tenantId/status
GET   /api/members
POST  /api/members/invitations
POST  /api/invitations/:token/accept
PATCH /api/members/:userId
```

Platform routes require platform administrator middleware. Member reads
require `members:read`; invites and updates require `members:manage`. Invitation
acceptance is unauthenticated and exempt from CSRF but still rate limited.
Handlers bind only documented command fields; tenant context always comes from
the authenticated server-side session, never from request JSON.

- [ ] **Step 6: Verify GREEN and commit**

Run: `go test ./internal/tenants ./internal/members -v`

Expected: all tenant, invitation, suspension, and last-owner cases pass.

```bash
git add whatsapp-ai-poc/internal/tenants whatsapp-ai-poc/internal/members whatsapp-ai-poc/internal/app/app.go
git commit -m "feat: add tenant and membership lifecycle"
```

## Task 8: Build the Read-Only Legacy Inventory

**Files:**
- Create: `internal/legacy/inventory.go`
- Create: `internal/legacy/inventory_test.go`
- Create: `cmd/import-legacy/main.go`
- Create: `test/fixtures/legacy/accounts.json`
- Create: `test/fixtures/legacy/knowledge.json`

- [ ] **Step 1: Write failing deterministic dry-run tests**

Fixtures contain two accounts, three roles, duplicate product IDs, an account
referencing an unknown role, and one malformed record. Assert:

```go
want := legacy.Report{
	Accounts: legacy.Count{Valid: 1, Invalid: 1},
	Roles: legacy.Count{Valid: 3, Invalid: 0},
	Entries: legacy.Count{Valid: 2, Invalid: 1},
	Conflicts: []legacy.Conflict{
		{Code: "DUPLICATE_ENTRY_ID", Source: "knowledge.json", Value: "product-1"},
		{Code: "UNKNOWN_ACCOUNT_ROLE", Source: "accounts.json", Value: "missing-role"},
	},
}
```

Also snapshot PostgreSQL table counts before and after the CLI and require no
change.

- [ ] **Step 2: Run tests and verify RED**

Run: `go test ./internal/legacy -v`

Expected: FAIL because inventory types and parser are absent.

- [ ] **Step 3: Implement structured inventory**

```go
type Paths struct { AccountsJSON, KnowledgeJSON, SQLite string }
type Count struct { Valid, Invalid int }
type Conflict struct { Code, Source, Value string }
type Report struct { Accounts, Roles, Entries Count; Conflicts []Conflict }
func Inspect(ctx context.Context, paths Paths) (Report, error)
```

Decode JSON with `encoding/json` into typed wire structs, validate every record,
collect conflicts, and sort by code/source/value. Open SQLite read-only only
when a path is supplied. The package must not import pgx or any database pool.

- [ ] **Step 4: Implement the dry-run CLI**

`go run ./cmd/import-legacy --dry-run` reads defaults `config/accounts.json`,
`config/knowledge.json`, and optional `DB_PATH`. It prints indented JSON counts
and conflicts without product content, customer data, credentials, or absolute
paths. Without `--dry-run`, exit non-zero with `formal import is not part of the
foundation stage`.

- [ ] **Step 5: Verify GREEN and commit**

Run: `go test ./internal/legacy -v`

Expected: deterministic counts/conflicts and zero PostgreSQL writes.

```bash
git add whatsapp-ai-poc/internal/legacy whatsapp-ai-poc/cmd/import-legacy whatsapp-ai-poc/test/fixtures/legacy
git commit -m "feat: add Go legacy migration inventory"
```

## Task 9: Make Go the Default Backend and Document Operations

**Files:**
- Create: `cmd/server/main.go`
- Modify: `README.md`
- Create: `docs/deployment/postgresql.md`
- Create: `docs/testing/foundation.md`
- Delete: `package.json`
- Delete: `package-lock.json`

- [ ] **Step 1: Write a failing server shutdown test**

Extract this testable server contract:

```go
func Run(ctx context.Context, cfg config.Config, pool *pgxpool.Pool, listener net.Listener) error
```

The test starts on an ephemeral listener, verifies `/health`, cancels context,
and requires shutdown within five seconds.

- [ ] **Step 2: Run the test and verify RED**

Run: `go test ./cmd/server -v`

Expected: FAIL because the production entry point is absent.

- [ ] **Step 3: Implement production startup and shutdown**

Load config, open and ping the application pool, build Gin, listen on configured
host/port, and stop cleanly on SIGINT/SIGTERM. Logs may contain environment,
port, migration version, and request IDs only. Never log a database URL.

- [ ] **Step 4: Remove Node backend entry points and update docs**

Delete the backend `package.json` and `package-lock.json`. Leave legacy source
files untouched as migration reference, but remove all Node start/test commands
from README. Document these Go commands:

```bash
go run ./cmd/db-create
go run ./cmd/db-migrate
go run ./cmd/bootstrap-admin
go run ./cmd/import-legacy --dry-run
go run ./cmd/server
```

`docs/deployment/postgresql.md` documents migration/runtime credential
separation, restricted role checks, bootstrap, secret rotation, and legacy
dry-run. `docs/testing/foundation.md` lists every package test plus the full
verification commands and expected results.

- [ ] **Step 5: Run the complete completion gate**

Run:

```bash
go test ./...
go test -race ./...
go vet ./...
go build ./cmd/...
go run ./cmd/import-legacy --dry-run
```

Then start `go run ./cmd/server`, request `GET /health`, and stop it with
SIGTERM. Expected: all commands exit zero; health is safe; dry-run writes no
rows; cross-tenant tests cannot read or mutate another tenant.

- [ ] **Step 6: Review secrets and commit completion**

Run:

```bash
git diff --check
git diff --cached | rg -n "(postgres://[^[:space:]]+:[^[:space:]]+@|BEGIN [A-Z ]*PRIVATE KEY|api[_-]?key.*=|password.*=)"
```

Expected: formatting check passes and the secret scan returns no real secrets.

```bash
git add whatsapp-ai-poc/cmd/server whatsapp-ai-poc/README.md whatsapp-ai-poc/docs/deployment/postgresql.md whatsapp-ai-poc/docs/testing/foundation.md whatsapp-ai-poc/package.json whatsapp-ai-poc/package-lock.json
git commit -m "docs: complete Go foundation setup"
```

## Foundation Completion Gate

- `go test ./...`, `go test -race ./...`, `go vet ./...`, and `go build ./cmd/...` pass.
- Migrations apply twice without changes.
- Every integration database name ends in `_test`.
- Runtime role is non-superuser and cannot bypass RLS, create databases, or create roles.
- Cross-tenant reads are invisible and cross-tenant writes fail.
- Permission tests cover every role/permission pair.
- Raw session, CSRF, invitation, password, database, GitHub, OpenClaw, and model secrets are absent from Git diff and API responses.
- Legacy dry-run performs zero writes and produces deterministic output.
- The Go server is the only backend start path; existing JavaScript is not built, tested, started, or deployed.
- Existing user changes in `AGENTS.md`, `src/admin-page.js`, and `src/api/knowledge.js` remain untouched.
