# PostgreSQL Deployment

## Credential Separation

Foundation 使用三套数据库权限：

| 变量 | 使用者 | 权限 |
|---|---|---|
| `DATABASE_ADMIN_URL` | `cmd/db-create` | 创建固定数据库和受限角色 |
| `DATABASE_MIGRATION_URL` | `cmd/db-migrate` | 拥有 schema、执行版本迁移 |
| `DATABASE_URL` | server、bootstrap CLI | `whatsapp_app` 运行时 DML，无建库/建角色/RLS bypass |

不要让生产 server 进程获得 admin 或 migration 变量。CLI 缺少自己的专用变量时会直接失败，不会回退到 `DATABASE_URL`。

## Bootstrap Sequence

1. 将 `.env.example` 中的本地 placeholder 替换为部署平台 secret。
2. 使用数据库管理员身份运行 `go run ./cmd/db-create`。
3. 为 `whatsapp_ai` 设置 `DATABASE_MIGRATION_URL`，运行 `go run ./cmd/db-migrate`。
4. 为受限 `whatsapp_app` 设置 `DATABASE_URL`，运行 `go run ./cmd/bootstrap-admin`。
5. 只向 server 注入运行时配置和 `DATABASE_URL`，运行 `go run ./cmd/server`。
6. 请求 `GET /health`，确认 HTTP 200、`database` 为 `up`，且响应没有 host、URL 或凭据。

迁移是带 SHA-256 checksum 的 append-only 文件。已经应用的文件发生任何变化时，迁移器会拒绝继续；修复 schema 必须新增迁移版本。

## Restricted Role Check

用 migration/admin 连接执行：

```sql
SELECT rolname, rolsuper, rolcreatedb, rolcreaterole, rolbypassrls, rolinherit
FROM pg_roles
WHERE rolname = 'whatsapp_app';
```

`rolsuper`、`rolcreatedb`、`rolcreaterole`、`rolbypassrls` 和 `rolinherit` 必须全部为 `false`。Tenant-owned 表必须同时显示 `relrowsecurity=true` 与 `relforcerowsecurity=true`。

```sql
SELECT relname, relrowsecurity, relforcerowsecurity
FROM pg_class
WHERE relname IN ('tenant_memberships', 'member_invitations', 'audit_logs');
```

## Secret Rotation

- 数据库密码：在 PostgreSQL 中轮换 `whatsapp_app` 密码，原子更新 secret store，再滚动重启 server。不要把 URL 打进日志或 shell history。
- Bootstrap 管理员密码：通过后续受控密码变更流程轮换；`password_changed_at` 会使旧 session 失效。Bootstrap CLI 不覆盖现有用户。
- Session 和邀请 token：数据库只保存 SHA-256；原始值无法从数据库恢复。需要时撤销 session/邀请并重新签发。

## Legacy Dry-Run

```bash
go run ./cmd/import-legacy --dry-run
```

默认读取 `config/accounts.json` 和 `config/knowledge.json`。只有设置 `DB_PATH` 或 `--sqlite` 时才以 `mode=ro` 打开 SQLite。报告只包含计数和安全冲突标识，不写 PostgreSQL，也不输出产品内容、客户数据、凭据或绝对路径。

正式导入不属于 foundation；不带 `--dry-run` 的调用会失败。
