# 验证记录

## 自动化

- `go test ./...`：会话、OpenClaw 标识规范化、二维码桥接和知识库 CSV/JSON 解析。
- `pnpm --dir frontend test`：前端 API multipart 请求和现有聊天导航行为。
- `pnpm --dir frontend run build`：前端生产构建。

## OpenClaw

使用当前安装的 OpenClaw 版本执行：

```sh
openclaw message send --dry-run --channel whatsapp --account default --target +8613800000000 --message 'verification only' --json
```

命令返回 `action: send`、`channel: whatsapp` 和 `dryRun: true`，验证了人工客服外发调用的参数契约，不会向客户发送测试消息。

## 运行时界面

在 `http://127.0.0.1:5173` 验证：会话列表可打开历史，点击“继续处理”会进入该会话所属账号的聊天工作台；知识库详情显示“批量导入”和“CSV 模板”入口。

运行中的 `:8790` 为变更前启动的二进制。部署本次后端接口前，需要用部署环境既有的 `DATABASE_URL`、`ADMIN_PASSWORD` 和 `INTERNAL_API_TOKEN` 重建并重启服务。
