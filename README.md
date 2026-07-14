# WhatsApp AI 管理端

Go/Gin 管理端服务 + React/Vite 前端。

## 项目结构

```
├── cmd/server/    # HTTP 服务入口（Gin）
├── web/           # 前端嵌入层（go:embed dist → http.Handler）
├── frontend/      # React/Vite 管理端（独立项目，构建输出到 ../web/dist）
└── .env.example   # 环境变量
```

## 启动

```bash
# 构建前端
cd frontend && pnpm install && pnpm run build

# 启动服务
go run ./cmd/server
```

访问 `http://localhost:8790` 打开管理端，`/health` 返回服务状态。

## 前端开发

```bash
cd frontend && pnpm run dev    # Vite dev server，代理 /api 到 :8790
```
