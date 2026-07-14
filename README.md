# WhatsApp AI 管理端

Go/Gin 管理端服务 + Vue 3/Element Plus 前端，前端打包进 Go 二进制，启动即用。

## 项目结构

```
├── cmd/server/    # HTTP 服务入口（Gin, graceful shutdown）
├── web/           # 前端嵌入层（go:embed dist → http.Handler, SPA fallback）
├── frontend/      # Vue 3/Vite 管理端（独立项目，构建输出到 ../web/dist）
└── .env.example   # 环境变量
```

## 快速启动

```bash
# 构建前端
cd frontend && pnpm install && pnpm run build

# 构建并启动服务（前端已嵌入）
go build ./cmd/server
./server
```

访问 `http://localhost:8790` 打开管理端，`GET /health` 返回 `{"status":"ok"}`。

## 前端开发

```bash
cd frontend && pnpm run dev    # Vite :5173，代理 /api → :8790
```

## 技术栈

- **后端**: Go, Gin, go:embed
- **前端**: Vue 3 (Composition API), Vue Router 4, Element Plus 2
- **构建**: Vite 8, pnpm
- **测试**: Vitest
