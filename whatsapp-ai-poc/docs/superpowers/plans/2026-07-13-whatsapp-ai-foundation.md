# WhatsApp AI Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the PostgreSQL, tenant, authentication, RBAC, audit, and migration foundation required by the approved multi-tenant WhatsApp AI commercial MVP.

**Architecture:** Add a new Fastify modular-monolith backend beside the current demo and keep the legacy server runnable during migration. PostgreSQL is the only new business source of truth; tenant-scoped service calls run in transactions with Row Level Security, while platform operations stay on platform tables and enter a specific tenant context only when creating tenant-owned records.

**Tech Stack:** Node.js ESM, Fastify, PostgreSQL (`pg`), Zod, `@fastify/cookie`, `@fastify/helmet`, `@fastify/rate-limit`, Node `crypto`, Node test runner

**Source design:** `docs/superpowers/specs/2026-07-13-whatsapp-ai-commercial-mvp-design.md`

---

## Scope

This plan delivers only implementation stage 1 from the approved design:

- Fastify application shell and stable API error contract.
- PostgreSQL database bootstrap and versioned migrations.
- Tenant context and PostgreSQL RLS.
- Users, platform administrators, tenant memberships, invitations, and four-role RBAC.
- Server-side sessions, CSRF protection, and login rate limiting.
- Tenant/platform audit logging.
- Legacy SQLite/JSON dry-run inventory.

OpenClaw event handling, reply jobs, knowledge authorization, React screens, SSE, and the final data import belong to later plans.

## File Map

```text
package.json                              Runtime scripts and dependencies
.env.example                              Non-secret configuration contract
server/index.js                           Production entry point
server/app.js                             Fastify composition root
server/config.js                          Environment parsing and validation
server/errors.js                          Stable application error types
server/plugins/authenticate.js            Session authentication decorator
server/plugins/authorize.js               Platform/RBAC authorization decorators
server/plugins/database.js                PostgreSQL pool lifecycle
server/plugins/error-handler.js           Stable API error response mapping
server/db/pool.js                         Pool and transaction helpers
server/db/migrate.js                      Migration runner
server/db/tenant-context.js               RLS tenant transaction helper
server/db/migrations/0001_foundation.sql   Foundation schema, constraints, grants, RLS
server/modules/audit/service.js            Structured audit writes
server/modules/auth/password.js            Scrypt password hashing and verification
server/modules/auth/session-service.js     Session issue, resolve, revoke
server/modules/auth/routes.js              Login, logout, current-user APIs
server/modules/members/permissions.js      Four-role permission map
server/modules/members/service.js          Membership and invitation rules
server/modules/members/routes.js           Tenant member and invitation APIs
server/modules/tenants/service.js          Platform tenant lifecycle
server/modules/tenants/routes.js           Platform tenant APIs
server/modules/tenants/schemas.js          Zod request schemas
scripts/db-create.js                       Creates databases and non-superuser app role
scripts/db-migrate.js                      CLI migration entry point
scripts/bootstrap-platform-admin.js        One-time platform administrator bootstrap
scripts/import-legacy.js                   Dry-run inventory CLI
test/helpers/postgres.js                   Guarded test database setup
test/helpers/app.js                        Fastify test app/session helpers
test/foundation/config.test.js             Configuration contract tests
test/foundation/migrations.test.js         Schema and migration idempotency tests
test/foundation/tenant-isolation.test.js   RLS cross-tenant tests
test/foundation/auth.test.js               Login/session/CSRF tests
test/foundation/permissions.test.js        RBAC matrix tests
test/foundation/tenants.test.js            Tenant creation/invitation tests
test/foundation/audit.test.js              Audit and secret-redaction tests
test/foundation/legacy-dry-run.test.js      Legacy inventory tests
docs/deployment/postgresql.md              Database bootstrap and migration runbook
docs/testing/foundation.md                  Foundation verification commands
```

## Task 1: Establish the New Runtime and Test Harness

**Files:**
- Modify: `package.json`
- Modify: `.env.example`
- Create: `server/config.js`
- Create: `test/foundation/config.test.js`

- [ ] **Step 1: Write the failing configuration tests**

Create `test/foundation/config.test.js` with tests that call an exported `parseConfig(env)` function and assert:

```js
import assert from "node:assert/strict";
import test from "node:test";
import { parseConfig } from "../../server/config.js";

const validEnv = {
  NODE_ENV: "test",
  PORT: "8790",
  APP_ORIGIN: "http://localhost:8790",
  DATABASE_URL: "postgres://whatsapp_app:secret@localhost:5432/whatsapp_ai_test",
  SESSION_COOKIE_NAME: "wa_session",
  SESSION_TTL_HOURS: "12"
};

test("parseConfig accepts the complete runtime contract", () => {
  const config = parseConfig(validEnv);
  assert.equal(config.port, 8790);
  assert.equal(config.sessionTtlHours, 12);
  assert.equal(config.isProduction, false);
});

test("parseConfig rejects a missing database URL", () => {
  assert.throws(
    () => parseConfig({ ...validEnv, DATABASE_URL: "" }),
    /DATABASE_URL/
  );
});

test("parseConfig rejects a non-http application origin", () => {
  assert.throws(
    () => parseConfig({ ...validEnv, APP_ORIGIN: "javascript:alert(1)" }),
    /APP_ORIGIN/
  );
});
```

- [ ] **Step 2: Run the configuration test and verify it fails**

Run: `node --test test/foundation/config.test.js`

Expected: FAIL because `server/config.js` does not exist.

- [ ] **Step 3: Add dependencies and scripts**

Update `package.json` so the existing legacy entry point remains available while the new server becomes the default only after Task 9:

```json
{
  "scripts": {
    "start": "node src/server.js",
    "start:legacy": "node src/server.js",
    "dev:new": "node --watch server/index.js",
    "test": "node --test --test-concurrency=1",
    "test:foundation": "node --test --test-concurrency=1 test/foundation/*.test.js",
    "db:create": "node scripts/db-create.js",
    "db:migrate": "node scripts/db-migrate.js",
    "db:bootstrap-admin": "node scripts/bootstrap-platform-admin.js",
    "legacy:dry-run": "node scripts/import-legacy.js --dry-run",
    "generate:prompt": "node src/build-agent-prompt.js"
  },
  "dependencies": {
    "@fastify/cookie": "^11.0.2",
    "@fastify/helmet": "^13.0.2",
    "@fastify/rate-limit": "^10.3.0",
    "better-sqlite3": "^12.11.1",
    "dotenv": "^17.2.3",
    "fastify": "^5.6.2",
    "pg": "^8.16.3",
    "pg-format": "^1.0.4",
    "zod": "^4.1.12"
  }
}
```

Run: `npm install`

Expected: `package-lock.json` updates without audit errors that block installation.

- [ ] **Step 4: Implement strict configuration parsing**

Create `server/config.js` using `dotenv/config` and a Zod schema. Export `parseConfig(env)` for tests and `config` for runtime. The returned object must contain `nodeEnv`, `isProduction`, `port`, `appOrigin`, `databaseUrl`, `sessionCookieName`, and `sessionTtlHours`. Never export database credentials as separately loggable fields.

```js
import "dotenv/config";
import { z } from "zod";

const schema = z.object({
  NODE_ENV: z.enum(["development", "test", "production"]).default("development"),
  PORT: z.coerce.number().int().min(1).max(65535).default(8790),
  APP_ORIGIN: z.string().url().refine((value) => /^https?:/.test(value), "APP_ORIGIN must use http or https"),
  DATABASE_URL: z.string().min(1),
  SESSION_COOKIE_NAME: z.string().regex(/^[a-zA-Z0-9_]+$/).default("wa_session"),
  SESSION_TTL_HOURS: z.coerce.number().int().min(1).max(168).default(12)
});

export function parseConfig(env) {
  const parsed = schema.parse(env);
  return Object.freeze({
    nodeEnv: parsed.NODE_ENV,
    isProduction: parsed.NODE_ENV === "production",
    port: parsed.PORT,
    appOrigin: parsed.APP_ORIGIN,
    databaseUrl: parsed.DATABASE_URL,
    sessionCookieName: parsed.SESSION_COOKIE_NAME,
    sessionTtlHours: parsed.SESSION_TTL_HOURS
  });
}

export const config = parseConfig(process.env);
```

- [ ] **Step 5: Document environment variable names without values**

Update `.env.example` with `APP_ORIGIN`, `DATABASE_URL`, `DATABASE_ADMIN_URL`, `DATABASE_MIGRATION_URL`, `DATABASE_APP_PASSWORD`, `TEST_DATABASE_URL`, `TEST_DATABASE_MIGRATION_URL`, `SESSION_COOKIE_NAME`, `SESSION_TTL_HOURS`, `BOOTSTRAP_ADMIN_EMAIL`, and `BOOTSTRAP_ADMIN_PASSWORD`. Use local placeholders only; do not include any real host, username, password, or token. `DATABASE_URL` and `TEST_DATABASE_URL` must use the restricted `whatsapp_app` role; migration URLs use the administrative migration role and are never loaded by the HTTP runtime.

- [ ] **Step 6: Run tests and commit**

Run: `node --test test/foundation/config.test.js`

Expected: 3 tests pass.

Commit:

```bash
git add package.json package-lock.json .env.example server/config.js test/foundation/config.test.js
git commit -m "build: add commercial backend runtime"
```

## Task 2: Create PostgreSQL Bootstrap and Versioned Migrations

**Files:**
- Create: `scripts/db-create.js`
- Create: `scripts/db-migrate.js`
- Create: `server/db/pool.js`
- Create: `server/db/migrate.js`
- Create: `server/db/migrations/0001_foundation.sql`
- Create: `test/helpers/postgres.js`
- Create: `test/foundation/migrations.test.js`

- [ ] **Step 1: Write guarded migration tests**

Create `test/helpers/postgres.js` that reads both `TEST_DATABASE_MIGRATION_URL` and `TEST_DATABASE_URL`, parses both database names, and refuses to run unless both names are identical and end with `_test`. Schema reset and migrations use the migration connection; RLS assertions use the restricted application connection. Create `test/foundation/migrations.test.js` that verifies migrations are idempotent and that all expected tables exist.

```js
test("foundation migrations are idempotent", async () => {
  await resetTestDatabase();
  await migrate(testPool);
  await migrate(testPool);
  const tables = await listPublicTables(testPool);
  assert.deepEqual(
    ["audit_logs", "auth_sessions", "member_invitations", "platform_roles", "tenant_memberships", "tenants", "users"].every((name) => tables.includes(name)),
    true
  );
});
```

- [ ] **Step 2: Run the migration test and verify it fails**

Run: `node --test test/foundation/migrations.test.js`

Expected: FAIL because the database helpers and migrations do not exist. If `TEST_DATABASE_URL` is absent, the test must skip with an explicit reason rather than connect to `DATABASE_URL`.

- [ ] **Step 3: Implement the database bootstrap CLI**

Create `scripts/db-create.js`. It must:

1. Require `DATABASE_ADMIN_URL` and `DATABASE_APP_PASSWORD`.
2. Connect to an existing administrative database.
3. Create fixed roles `whatsapp_app` and fixed databases `whatsapp_ai` and `whatsapp_ai_test` only when absent.
4. Make both roles `NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT`.
5. Use `pg-format` for identifiers and literals.
6. Never print connection strings or passwords.

The command output is limited to created/already-exists status for role and database names.

- [ ] **Step 4: Implement pool and migration runner**

Create `server/db/pool.js` with `createPool(databaseUrl)` and `withTransaction(pool, callback)`. Create `server/db/migrate.js` that:

- Creates `schema_migrations(version text primary key, checksum text not null, applied_at timestamptz not null default now())`.
- Loads sorted `.sql` files.
- Computes SHA-256 checksums.
- Applies each unapplied migration in a transaction.
- Rejects a changed checksum for an already-applied migration.

- [ ] **Step 5: Write the foundation SQL migration**

Create `server/db/migrations/0001_foundation.sql` with UUID primary keys supplied by the application and UTC timestamps. Include exact enum checks instead of PostgreSQL enum types so later migrations remain easier:

```sql
CREATE TABLE users (
  id uuid PRIMARY KEY,
  email text NOT NULL,
  display_name text NOT NULL,
  password_hash text NOT NULL,
  status text NOT NULL CHECK (status IN ('active', 'disabled')),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX users_email_unique ON users (lower(email));

CREATE TABLE platform_roles (
  user_id uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  role text NOT NULL CHECK (role IN ('platform_admin')),
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
  created_by uuid NOT NULL REFERENCES users(id),
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX member_invitations_open_unique
  ON member_invitations (tenant_id, lower(email)) WHERE accepted_at IS NULL;

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
```

Grant only required CRUD privileges to `whatsapp_app`; do not grant schema ownership, role creation, database creation, or RLS bypass.

- [ ] **Step 6: Add the migration CLI and run the tests**

Create `scripts/db-migrate.js` to migrate `DATABASE_MIGRATION_URL`. It must refuse to fall back to `DATABASE_URL` and print applied migration versions only. After each migration, grant only the required schema/table/sequence privileges to `whatsapp_app`; the runtime role never owns schema objects.

Run: `node --test test/foundation/migrations.test.js`

Expected: migration test passes against `whatsapp_ai_test`.

- [ ] **Step 7: Commit**

```bash
git add scripts/db-create.js scripts/db-migrate.js server/db test/helpers/postgres.js test/foundation/migrations.test.js
git commit -m "feat: add PostgreSQL foundation schema"
```

## Task 3: Enforce Tenant Isolation with PostgreSQL RLS

**Files:**
- Modify: `server/db/migrations/0001_foundation.sql`
- Create: `server/db/tenant-context.js`
- Create: `test/foundation/tenant-isolation.test.js`

- [ ] **Step 1: Write cross-tenant failing tests**

Seed tenants A and B using the migration connection. Query through the non-superuser test application role and assert:

```js
test("tenant context cannot read another tenant membership", async () => {
  const visible = await withTenantTransaction(appPool, tenantA.id, (client) =>
    client.query("SELECT tenant_id, user_id FROM tenant_memberships ORDER BY user_id")
  );
  assert.equal(visible.rows.every((row) => row.tenant_id === tenantA.id), true);
  assert.equal(visible.rows.some((row) => row.tenant_id === tenantB.id), false);
});

test("tenant context cannot insert a row for another tenant", async () => {
  await assert.rejects(
    () => withTenantTransaction(appPool, tenantA.id, (client) =>
      client.query(
        "INSERT INTO member_invitations (id, tenant_id, email, role, token_hash, expires_at, created_by) VALUES ($1,$2,$3,$4,$5,$6,$7)",
        [randomUUID(), tenantB.id, "cross@example.com", "agent", "hash", tomorrow, ownerA.id]
      )
    ),
    /row-level security/
  );
});
```

- [ ] **Step 2: Run tests and verify the cross-tenant read is visible before RLS**

Run: `node --test test/foundation/tenant-isolation.test.js`

Expected: FAIL because RLS policies and `withTenantTransaction` are absent.

- [ ] **Step 3: Add tenant context helper**

Create `server/db/tenant-context.js`:

```js
export async function withTenantTransaction(pool, tenantId, callback) {
  const client = await pool.connect();
  try {
    await client.query("BEGIN");
    await client.query("SELECT set_config('app.tenant_id', $1, true)", [tenantId]);
    const result = await callback(client);
    await client.query("COMMIT");
    return result;
  } catch (error) {
    await client.query("ROLLBACK");
    throw error;
  } finally {
    client.release();
  }
}
```

- [ ] **Step 4: Add and force RLS policies**

Add a stable SQL function that returns the current tenant setting as UUID or null. Enable and force RLS on `tenant_memberships`, `member_invitations`, and tenant-scoped `audit_logs`. Each `USING` and `WITH CHECK` clause must compare `tenant_id` to `app_current_tenant_id()`.

Do not enable tenant RLS on `users`, `platform_roles`, `tenants`, or `auth_sessions`; those tables are reached only through authenticated services with explicit lookup rules.

- [ ] **Step 5: Run isolation tests and commit**

Run: `node --test test/foundation/tenant-isolation.test.js`

Expected: cross-tenant reads return no rows and cross-tenant writes fail.

```bash
git add server/db/migrations/0001_foundation.sql server/db/tenant-context.js test/foundation/tenant-isolation.test.js
git commit -m "feat: enforce tenant row isolation"
```

## Task 4: Add Stable Errors, Fastify Composition, and Audit Writes

**Files:**
- Create: `server/errors.js`
- Create: `server/app.js`
- Create: `server/plugins/database.js`
- Create: `server/plugins/error-handler.js`
- Create: `server/modules/audit/service.js`
- Create: `test/helpers/app.js`
- Create: `test/foundation/audit.test.js`

- [ ] **Step 1: Write failing error and audit tests**

Test that unknown routes return `NOT_FOUND` with a request ID, validation errors return `VALIDATION_FAILED`, and audit summaries remove keys matching `password`, `secret`, `token`, `apiKey`, or `authorization` recursively.

```js
test("audit summaries redact nested secrets", () => {
  assert.deepEqual(
    redactSecrets({ label: "Model", credentials: { apiKey: "real-value" } }),
    { label: "Model", credentials: { apiKey: "[REDACTED]" } }
  );
});
```

- [ ] **Step 2: Verify tests fail**

Run: `node --test test/foundation/audit.test.js`

Expected: FAIL because the application/error/audit modules do not exist.

- [ ] **Step 3: Implement stable application errors**

Create `AppError` with `code`, `statusCode`, `message`, and optional `details`. Export named factories for `AUTH_REQUIRED`, `SESSION_EXPIRED`, `FORBIDDEN`, `TENANT_SUSPENDED`, `CONFLICT`, and `VALIDATION_FAILED`.

The error handler returns:

```json
{
  "error": {
    "code": "FORBIDDEN",
    "message": "You do not have permission to perform this action.",
    "requestId": "req_..."
  }
}
```

Unexpected errors are logged with the request ID and returned as `INTERNAL_ERROR` without stack traces or database details.

- [ ] **Step 4: Compose the initial Fastify app**

Create `buildApp({ config, pool })` in `server/app.js`. Register Helmet, cookies, request IDs, the database decorator, error handler, and `GET /health`. The health response may report database availability but must not include hostnames or connection strings.

- [ ] **Step 5: Implement redacted audit writes**

Create `writeAudit(client, event)` and `redactSecrets(value)`. Audit writes participating in a business action must use that action's transaction client. Failure audit for rejected HTTP requests may use a separate platform transaction.

- [ ] **Step 6: Run tests and commit**

Run: `node --test test/foundation/audit.test.js`

Expected: all audit/error tests pass.

```bash
git add server/errors.js server/app.js server/plugins server/modules/audit test/helpers/app.js test/foundation/audit.test.js
git commit -m "feat: add API errors and audit foundation"
```

## Task 5: Implement Passwords and Server-Side Sessions

**Files:**
- Create: `server/modules/auth/password.js`
- Create: `server/modules/auth/session-service.js`
- Create: `server/modules/auth/routes.js`
- Create: `server/plugins/authenticate.js`
- Create: `scripts/bootstrap-platform-admin.js`
- Modify: `server/app.js`
- Create: `test/foundation/auth.test.js`

- [ ] **Step 1: Write failing authentication tests**

Cover:

- Correct password logs in and returns an `HttpOnly`, `SameSite=Lax` session cookie.
- Wrong password returns `AUTH_INVALID` without revealing whether an email exists.
- Session lookup rejects expired and revoked sessions.
- Logout revokes the server-side session.
- State-changing routes reject missing or invalid `X-CSRF-Token`.
- Login rate limiting returns 429 after the configured test threshold.

- [ ] **Step 2: Verify tests fail**

Run: `node --test test/foundation/auth.test.js`

Expected: FAIL because authentication routes and services do not exist.

- [ ] **Step 3: Implement scrypt password storage**

Use `crypto.scrypt`, `randomBytes`, and `timingSafeEqual`. Store hashes in the self-describing form:

```text
scrypt$N=16384,r=8,p=1$<base64-salt>$<base64-derived-key>
```

Reject malformed hashes. Never compare derived keys with normal string equality.

- [ ] **Step 4: Implement opaque server-side sessions**

Generate 32-byte random session and CSRF tokens. Store only SHA-256 token hashes. Session resolution must join `users`, reject disabled users, and verify expiration/revocation. Cookie configuration is:

```js
{
  httpOnly: true,
  secure: config.isProduction,
  sameSite: "lax",
  path: "/",
  maxAge: config.sessionTtlHours * 60 * 60
}
```

Return the raw CSRF token only from login and `GET /api/auth/me`; do not put it in a readable cookie.

- [ ] **Step 5: Add authentication routes and plugin**

Implement:

- `POST /api/auth/login`
- `POST /api/auth/logout`
- `GET /api/auth/me`
- `POST /api/auth/select-tenant`

The authentication plugin decorates the request with `identity`, and enforces origin plus CSRF checks on `POST`, `PUT`, `PATCH`, and `DELETE` except login and invitation acceptance.

- [ ] **Step 6: Add bootstrap platform administrator CLI**

Read `BOOTSTRAP_ADMIN_EMAIL` and `BOOTSTRAP_ADMIN_PASSWORD`, create an active user and `platform_admin` role in one transaction, and refuse to overwrite an existing user. Print only the created email and user ID.

- [ ] **Step 7: Run tests and commit**

Run: `node --test test/foundation/auth.test.js`

Expected: authentication, session, CSRF, and rate-limit tests pass.

```bash
git add server/modules/auth server/plugins/authenticate.js server/app.js scripts/bootstrap-platform-admin.js test/foundation/auth.test.js
git commit -m "feat: add secure administrator sessions"
```

## Task 6: Implement Four-Role RBAC

**Files:**
- Create: `server/modules/members/permissions.js`
- Create: `server/plugins/authorize.js`
- Modify: `server/app.js`
- Create: `test/foundation/permissions.test.js`

- [ ] **Step 1: Write the complete failing permission matrix test**

Define permissions as stable strings:

```js
export const permissions = [
  "members:manage", "members:read",
  "accounts:manage", "accounts:read",
  "knowledge:manage", "knowledge:read", "models:manage",
  "conversations:manage", "conversations:read",
  "customers:read", "settings:manage",
  "metrics:read", "alerts:read", "audit:read"
];
```

Test every role against every permission. Expected policy:

- `owner`: all permissions.
- `admin`: all except `members:manage`.
- `agent`: `accounts:read`, `knowledge:read`, `conversations:manage`, `conversations:read`, `customers:read`, `metrics:read`, `alerts:read`.
- `viewer`: `accounts:read`, `metrics:read`, `alerts:read`, `audit:read`.

- [ ] **Step 2: Run and verify failure**

Run: `node --test test/foundation/permissions.test.js`

Expected: FAIL because the permission module does not exist.

- [ ] **Step 3: Implement permission mapping and authorization decorators**

Export `hasPermission(role, permission)`. Add `requirePlatformAdmin` and `requirePermission(permission)` Fastify decorators. `requirePermission` must load the active membership for the authenticated user's selected tenant, reject suspended tenants, and attach `tenantContext` containing `tenantId`, `userId`, and `role`.

- [ ] **Step 4: Run tests and commit**

Run: `node --test test/foundation/permissions.test.js`

Expected: all matrix and decorator tests pass.

```bash
git add server/modules/members/permissions.js server/plugins/authorize.js server/app.js test/foundation/permissions.test.js
git commit -m "feat: enforce tenant role permissions"
```

## Task 7: Implement Platform Tenant Creation and Invitations

**Files:**
- Create: `server/modules/tenants/schemas.js`
- Create: `server/modules/tenants/service.js`
- Create: `server/modules/tenants/routes.js`
- Create: `server/modules/members/service.js`
- Create: `server/modules/members/routes.js`
- Modify: `server/app.js`
- Create: `test/foundation/tenants.test.js`

- [ ] **Step 1: Write failing tenant lifecycle tests**

Cover:

- Only platform administrators can create a tenant.
- Creating a tenant also creates one owner invitation in one transaction.
- Invitation raw tokens are returned once and only hashes are stored.
- Accepting an invitation creates or activates a user membership with the invited role.
- Expired, accepted, wrong-email, or revoked invitations fail.
- An owner can invite members; an admin cannot invite or change members.
- The last active owner cannot be demoted or disabled.
- Suspending a tenant blocks tenant APIs but preserves platform access.

- [ ] **Step 2: Verify tests fail**

Run: `node --test test/foundation/tenants.test.js`

Expected: FAIL because tenant/member services and routes do not exist.

- [ ] **Step 3: Implement validated tenant creation**

Use Zod to validate `name`, `slug`, `ownerEmail`, and `ownerDisplayName`. Normalize email to lowercase and slug to lowercase ASCII plus hyphens. The service inserts the tenant, owner invitation, and audit record in one transaction. Return `{ tenant, invitation: { email, role, token, expiresAt } }`; never return `token_hash`.

- [ ] **Step 4: Implement invitation acceptance and member rules**

Implement:

- `POST /api/platform/tenants`
- `PATCH /api/platform/tenants/:tenantId/status`
- `GET /api/members`
- `POST /api/members/invitations`
- `POST /api/invitations/:token/accept`
- `PATCH /api/members/:userId`

Invitation acceptance creates or updates the user and active membership in one transaction, sets `accepted_at`, issues a session, and writes audit records. Never accept an invitation for a different normalized email.

- [ ] **Step 5: Run tests and commit**

Run: `node --test test/foundation/tenants.test.js`

Expected: tenant lifecycle, invitation, suspension, and last-owner tests pass.

```bash
git add server/modules/tenants server/modules/members server/app.js test/foundation/tenants.test.js
git commit -m "feat: add tenant and membership lifecycle"
```

## Task 8: Build the Legacy Dry-Run Inventory

**Files:**
- Create: `scripts/import-legacy.js`
- Create: `server/modules/tenants/legacy-inventory.js`
- Create: `test/fixtures/legacy/accounts.json`
- Create: `test/fixtures/legacy/knowledge.json`
- Create: `test/foundation/legacy-dry-run.test.js`

- [ ] **Step 1: Write failing dry-run tests**

Use fixtures containing two accounts, three roles, duplicate product IDs, an account referencing an unknown role, and one malformed JSON source. Assert that dry-run returns a deterministic report without writing PostgreSQL:

```js
{
  accounts: { valid: 1, invalid: 1 },
  roles: { valid: 3, invalid: 0 },
  entries: { valid: 2, invalid: 1 },
  conflicts: [
    { code: "UNKNOWN_ACCOUNT_ROLE", source: "accounts.json", value: "missing-role" },
    { code: "DUPLICATE_ENTRY_ID", source: "knowledge.json", value: "product-1" }
  ]
}
```

- [ ] **Step 2: Verify tests fail**

Run: `node --test test/foundation/legacy-dry-run.test.js`

Expected: FAIL because the inventory module does not exist.

- [ ] **Step 3: Implement read-only structured inventory**

Read the legacy paths passed as arguments; do not import runtime globals. Validate each source with Zod, collect errors instead of stopping at the first record, and sort conflicts by `code`, `source`, then `value` so output is reproducible. The module must not import the PostgreSQL pool.

- [ ] **Step 4: Implement dry-run CLI**

`node scripts/import-legacy.js --dry-run` reads the current `config/accounts.json`, `config/knowledge.json`, and SQLite path from `DB_PATH` only for inventory. It prints JSON counts and conflicts without printing product content, customer data, credentials, or absolute home paths. Without `--dry-run`, it exits with a message that formal import belongs to the migration stage.

- [ ] **Step 5: Run tests and commit**

Run: `node --test test/foundation/legacy-dry-run.test.js`

Expected: dry-run tests pass and PostgreSQL table counts remain unchanged.

```bash
git add scripts/import-legacy.js server/modules/tenants/legacy-inventory.js test/fixtures/legacy test/foundation/legacy-dry-run.test.js
git commit -m "feat: add legacy migration inventory"
```

## Task 9: Switch the Default Server and Document Operations

**Files:**
- Create: `server/index.js`
- Modify: `package.json`
- Modify: `README.md`
- Create: `docs/deployment/postgresql.md`
- Create: `docs/testing/foundation.md`

- [ ] **Step 1: Add the production entry point**

Create `server/index.js` to create the pool, build the Fastify app, listen on the configured host/port, and close cleanly on `SIGINT`/`SIGTERM`. Log only port, environment, migration version, and request IDs; never log URLs containing credentials.

- [ ] **Step 2: Switch the default script while retaining legacy access**

Update scripts:

```json
{
  "start": "node server/index.js",
  "start:legacy": "node src/server.js",
  "dev": "node --watch server/index.js"
}
```

- [ ] **Step 3: Write deployment documentation**

`docs/deployment/postgresql.md` must document:

1. Required environment variable names and the separation between migration and runtime roles.
2. Creating `whatsapp_ai`, `whatsapp_ai_test`, and the non-superuser app role.
3. Running migrations.
4. Bootstrapping the platform administrator.
5. Running the legacy dry-run.
6. Confirming that the app role has no `rolsuper`, `rolbypassrls`, `rolcreatedb`, or `rolcreaterole` privileges.
7. Secret rotation without showing real values.

- [ ] **Step 4: Write verification documentation**

`docs/testing/foundation.md` lists exact commands and expected outcomes for config tests, migrations, RLS, authentication, RBAC, tenant lifecycle, audit, legacy dry-run, and the full foundation suite.

- [ ] **Step 5: Run all foundation verification**

Run:

```bash
npm run test:foundation
npm run legacy:dry-run
npm start
```

Expected:

- All foundation tests pass.
- Dry-run reports counts/conflicts and writes no PostgreSQL rows.
- `GET /health` reports healthy without exposing hostnames or credentials.
- `POST /api/auth/login` is rate limited and returns stable errors.
- A cross-tenant integration test cannot read or mutate another tenant.

- [ ] **Step 6: Commit the foundation completion**

```bash
git add server/index.js package.json package-lock.json README.md docs/deployment/postgresql.md docs/testing/foundation.md
git commit -m "docs: complete commercial foundation setup"
```

## Foundation Completion Gate

Do not start the OpenClaw runtime plan until all conditions are true:

- `npm run test:foundation` passes.
- Migrations apply twice without changes.
- Tests use a database whose name ends with `_test`.
- The application database role is non-superuser and cannot bypass RLS.
- Platform administrators cannot query tenant message/knowledge data because those tables are not part of this stage and no bypass endpoint exists.
- Owner/admin/agent/viewer permission tests cover every permission.
- Raw session, CSRF, invitation, password, database, GitHub, OpenClaw, and model secrets are absent from Git diff and API responses.
- Legacy dry-run performs zero writes.
- Existing user modifications in `AGENTS.md`, `src/admin-page.js`, and `src/api/knowledge.js` remain preserved unless explicitly incorporated in a later migration task.
