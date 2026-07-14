# WhatsApp AI Foundation

WhatsApp AI commercial MVP 的 Go/PostgreSQL 基础阶段。当前后端提供严格运行配置、checksummed migrations、强制 RLS、服务端会话、CSRF、RBAC、租户/成员生命周期、审计和只读遗留数据盘点。

OpenClaw 消息处理、知识服务、会话流、SSE 和 React 管理端属于后续阶段。`src/` 下的 JavaScript 仅保留为迁移参考，不是运行、测试或部署入口。

## 环境要求

- Go 1.26
- PostgreSQL 17
- Docker-compatible daemon（仅集成测试需要）

复制 [.env.example](.env.example) 后，通过进程环境注入变量。服务不会自动加载 `.env`。

运行时只读取 `DATABASE_URL`。建库和迁移分别只读取 `DATABASE_ADMIN_URL` 与 `DATABASE_MIGRATION_URL`，不会回退到运行时凭据。

## 初始化

```bash
go run ./cmd/db-create
go run ./cmd/db-migrate
go run ./cmd/bootstrap-admin
go run ./cmd/import-legacy --dry-run
```

`db-create` 固定创建受限角色 `whatsapp_app`，以及 `whatsapp_ai`、`whatsapp_ai_test` 两个数据库。管理员 bootstrap 拒绝覆盖已有邮箱。

## 启动

```bash
go run ./cmd/server
```

服务监听 `HTTP_HOST:PORT`。安全健康检查位于 `GET /health`，只返回整体状态与 `database: up|down`。

## Foundation API

```text
POST  /api/auth/login
POST  /api/auth/logout
GET   /api/auth/me
POST  /api/auth/select-tenant

POST  /api/platform/tenants
PATCH /api/platform/tenants/:tenantId/status

GET   /api/members
POST  /api/members/invitations
POST  /api/invitations/:token/accept
PATCH /api/members/:userId
```

所有变更型认证请求必须携带精确 `Origin` 和 `X-CSRF-Token`。Tenant 权限来自服务端 session 的 selected tenant；请求 body、query 或 path 中的 tenant ID 不能作为访问凭据。

## 验证

```bash
go test ./...
go test -race ./...
go vet ./...
go build ./cmd/...
go run ./cmd/import-legacy --dry-run
```

部署细节见 [docs/deployment/postgresql.md](docs/deployment/postgresql.md)，测试说明见 [docs/testing/foundation.md](docs/testing/foundation.md)。
