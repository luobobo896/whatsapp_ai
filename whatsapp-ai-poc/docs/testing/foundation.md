# Foundation Testing

## Prerequisites

安装 Go 1.26，并确保 Docker-compatible daemon 可访问。PostgreSQL 集成测试使用 testcontainers 和固定 `_test` 数据库；测试 helper 会在连接前拒绝非 `_test` 名称。

如果当前 Docker context 使用非默认 socket，请为测试进程设置对应 `DOCKER_HOST`。例如 OrbStack 默认 socket 通常位于用户目录的 `.orbstack/run/docker.sock`。

## Focused Packages

```bash
go test ./internal/platform/config -v
go test ./internal/platform/database -v
go test ./internal/audit ./internal/app -v
go test ./internal/auth -v
go test ./internal/members -v
go test ./internal/tenants -v
go test ./internal/legacy -v
go test ./cmd/server -v
```

预期行为：

- 配置拒绝缺失/不安全值且不回显 secret。
- migrations 首次应用、二次 no-op，并拒绝 checksum 变化。
- `whatsapp_app` 无高权限，forced RLS 隔离 tenant read/write。
- HTTP 错误稳定携带 `requestId`，审计 summary 递归脱敏。
- password、opaque session、Cookie、CSRF、Origin、logout 和 rate limit 通过。
- 4 个角色 × 14 个权限全部覆盖，平台角色与 tenant 角色分离。
- 租户创建、邀请接受、成员管理、最后 owner 和暂停规则通过。
- 遗留报告确定且 SQLite 源保持未修改。
- server 健康检查安全，取消 context 后 5 秒内停止。

## Completion Gate

```bash
go test ./...
go test -race ./...
go vet ./...
go build ./cmd/...
go run ./cmd/import-legacy --dry-run
git diff --check
```

所有命令应退出 0。Dry-run 输出只含 accounts/roles/entries 计数和 conflicts。完整测试不得启动、构建或依赖遗留 JavaScript 后端。

集成测试结束后 testcontainers 应自动销毁 PostgreSQL 容器。若测试被强制中断，使用本地容器运行时检查残留的 testcontainers 资源。
