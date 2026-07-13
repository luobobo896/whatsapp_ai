# WhatsApp AI Go Foundation Design

Date: 2026-07-13
Status: approved in conversation, pending written review
Source design: `docs/superpowers/specs/2026-07-13-whatsapp-ai-commercial-mvp-design.md`
Scope: implementation stage 1, foundation only

## 1. Decision and Scope

The backend, command-line tools, database migrations, and backend tests will be
implemented in Go. Gin provides the public HTTP service and pgx/v5 provides
PostgreSQL access. The Go backend will not use Node.js, Fastify, Zod, an ORM,
Redis, or an external message queue.

This document supersedes the Node.js and Fastify choices in the source design
and in `docs/superpowers/plans/2026-07-13-whatsapp-ai-foundation.md`. It does not
change the approved product behavior, PostgreSQL data model, tenant isolation,
authentication, RBAC, audit, or migration requirements.

The later React/Vite administration frontend may use Node.js as a build tool.
It is not part of this foundation stage. Existing JavaScript backend files and
legacy JSON or SQLite data may be retained as read-only migration references,
but they are not started, tested, deployed, or used as a production data
source after the Go foundation becomes the default backend.

## 2. Frontend and Backend Responsibility

The frontend is a presentation client. It may render pages, manage navigation,
collect form input, and display loading or error state. It must not implement
business decisions.

The Go backend is authoritative for:

- authentication and session state;
- input normalization and validation;
- tenant selection and isolation;
- permissions and membership rules;
- resource filtering and state transitions;
- metrics and derived business status;
- secret handling and redaction;
- all persistent reads and writes.

Frontend validation may provide immediate input feedback, but every rule is
repeated and enforced by the backend. Hiding a button is not authorization.
The API returns presentation-ready data and stable error codes so that the
frontend does not duplicate permission matrices or domain policies.

## 3. Architecture

The foundation is a modular monolith with one HTTP process and reusable Go
packages:

```text
cmd/server                 production HTTP entry point
cmd/db-migrate             versioned migration CLI
cmd/bootstrap-admin        platform administrator bootstrap CLI
cmd/import-legacy          read-only legacy inventory CLI
internal/platform/config   environment configuration
internal/platform/database pools, transactions, and tenant context
internal/platform/apperror stable application errors
internal/platform/http     Gin middleware and response helpers
internal/auth              passwords, sessions, routes, and services
internal/tenants           platform tenant lifecycle
internal/members           invitations, membership, and RBAC
internal/audit             redaction and transactional audit writes
migrations                 versioned PostgreSQL SQL files
```

Gin handlers decode HTTP input, call one service operation, and encode the
result. Handlers do not issue SQL and do not make domain decisions. Services
own validation, authorization, cross-module orchestration, and transaction
boundaries. Repositories execute parameterized SQL and contain no HTTP logic.
Cross-module behavior is performed through service interfaces rather than
cross-module SQL in handlers.

The production server uses only the application database role. Database
creation and schema migration use separate administrative credentials and are
available only to the migration CLI.

## 4. Request and Transaction Flow

Every request passes through recovery, request ID, security headers, and
structured logging middleware. Authentication resolves an opaque server-side
session. State-changing routes additionally enforce request Origin and CSRF.
The login endpoint has an independent rate limit.

Tenant IDs are derived from the authenticated server-side session and active
membership. A tenant ID received in a body, query string, or path is never
used as proof of access.

Tenant operations run through `WithTenantTx`. It begins a transaction and
uses `set_config('app.tenant_id', tenantID, true)` before any tenant-scoped
query. PostgreSQL Row Level Security is forced on tenant-owned tables and is a
second isolation layer in addition to explicit tenant predicates.

Platform operations use platform tables without tenant context. When a
platform operation creates tenant-owned records, such as the first owner
invitation, the transaction enters that specific new tenant context before
the tenant-owned insert. Tenant creation, invitation creation, and audit write
are atomic.

Audit records for successful business writes use the same transaction as the
business change. Rejected HTTP-request audits may use a separate controlled
platform transaction. Audit summaries recursively redact password, secret,
token, API key, and Authorization fields.

## 5. Authentication and Authorization

Passwords use `golang.org/x/crypto/scrypt` with a unique random salt. Stored
hashes are self-describing and comparisons use constant-time operations.
Malformed hashes are rejected.

Login creates independent 32-byte random session and CSRF tokens. PostgreSQL
stores only SHA-256 hashes. The session token is sent as an HttpOnly,
SameSite=Lax cookie and is Secure in production. The raw CSRF token is returned
only from login and `GET /api/auth/me`; it is not stored in a browser-readable
cookie.

Sessions support expiry, logout revocation, administrator revocation, disabled
users, and invalidation after a password change. The selected tenant is stored
in the server-side session and is revalidated against active membership and
tenant status on each protected tenant request.

The backend owns the complete owner, admin, agent, and viewer permission
matrix. It rejects suspension of access to tenant APIs, prevents demotion or
disablement of the final active owner, and keeps platform administrator access
separate from tenant membership. Platform administrators have no default API
for tenant message, customer, or knowledge content.

Invitation tokens are returned once and stored only as hashes. Acceptance
requires the same normalized email as the invitation and rejects expired,
accepted, revoked, or mismatched invitations.

## 6. Errors and API Contract

Services return typed application errors with a stable code, HTTP status,
safe message, and optional safe details. Gin has one error response path:

```json
{
  "error": {
    "code": "FORBIDDEN",
    "message": "You do not have permission to perform this action.",
    "requestId": "req_..."
  }
}
```

Known database constraints are translated into stable domain errors. Unknown
errors are logged with the request ID and returned as `INTERNAL_ERROR` without
stack traces, SQL, connection details, or secrets. The first stage includes at
least `AUTH_REQUIRED`, `SESSION_EXPIRED`, `AUTH_INVALID`, `FORBIDDEN`,
`TENANT_SUSPENDED`, `CONFLICT`, `VALIDATION_FAILED`, `RATE_LIMITED`, and
`INTERNAL_ERROR`.

## 7. Database and Migrations

PostgreSQL is the only new business source of truth. Versioned SQL migrations
create the foundation schema, constraints, indexes, application role grants,
the tenant-context function, and forced RLS policies. The migration CLI reads
only `DATABASE_MIGRATION_URL`; it never falls back to `DATABASE_URL`. The
runtime server reads only the application connection string.

The application role is non-superuser and has no `BYPASSRLS`, `CREATEDB`,
`CREATEROLE`, or schema ownership. Migration tests apply all versions twice;
the second application produces no schema change.

## 8. Legacy Inventory

The legacy inventory is a read-only Go package and CLI. It receives paths as
arguments rather than importing runtime globals. It validates all available
records, collects errors instead of stopping at the first one, and sorts
conflicts by code, source, then value for deterministic output.

Dry-run output contains counts and safe conflict identifiers only. It does not
print product content, customer data, credentials, or absolute home paths, and
it performs no PostgreSQL writes. A non-dry-run invocation exits with a clear
message that formal import belongs to the later migration stage.

## 9. Testing and Verification

Implementation follows test-driven development: each behavior starts with a
test that fails for the expected missing behavior, followed by the minimum
implementation that makes it pass.

Unit tests cover configuration, password hashing, the complete permission
matrix, stable errors, recursive audit redaction, validation, and deterministic
legacy inventory. HTTP tests use `httptest` for cookies, CSRF, request IDs,
rate limiting, and error responses.

PostgreSQL integration tests use testcontainers-go with a real PostgreSQL
database named `whatsapp_ai_test`. Tenant-isolation tests connect as a real
non-superuser application role and prove that cross-tenant reads are invisible
and cross-tenant writes fail. Migration tests prove repeatability and role
privilege restrictions.

The completion commands are:

```bash
go test ./...
go test -race ./...
go vet ./...
go build ./cmd/...
```

The Go server is then started and `GET /health` is checked for a healthy
response that contains no hostname, connection string, or credential.

## 10. Foundation Completion Boundary

This stage delivers configuration, migrations, PostgreSQL RLS, stable errors,
Gin composition, audit writes, passwords, server-side sessions, CSRF, login
rate limiting, RBAC, platform tenant creation, invitations, membership rules,
the legacy dry-run, Go operational entry points, and deployment/testing docs.

It does not deliver the OpenClaw runtime, message pipeline, knowledge service,
conversations, SSE, or the React/Vite administration frontend. Those remain in
the later stages of the source commercial MVP design.
