# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

WhatsApp AI 管理端 — Go/Gin HTTP 服务 + Vue 3/Element Plus 前端（SPA 内嵌）。

WhatsApp 消息收发由外部独立组件对接，本项目仅负责管理端。

## Project Structure

```
├── cmd/server/    # HTTP 服务入口（Gin, graceful shutdown）
├── web/           # 前端嵌入层（go:embed dist, SPA fallback）
├── frontend/      # Vue 3/Vite 管理端（构建输出到 ../web/dist）
│   ├── src/
│   │   ├── api/          # API 客户端（fetch 封装）
│   │   ├── components/   # 业务组件（Brand, 各 Dialog）
│   │   ├── composables/  # 共享状态（useSession, useToast）
│   │   ├── router/       # Vue Router 路由
│   │   ├── views/        # 页面组件
│   │   └── styles/       # WhatsApp 品牌 Element Plus 主题覆写
│   └── ...
├── .env.example
└── CLAUDE.md
```

## Build & Run

```bash
# 前端
cd frontend && pnpm install && pnpm run build   # → ../web/dist/

# 后端
go build ./cmd/server
go run ./cmd/server          # 默认 :8790，自动启动前端
```

`GET /health` → `{"status":"ok"}`，`/*` → 前端 SPA。

## Frontend Dev

```bash
cd frontend && pnpm run dev   # Vite :5173，代理 /api → :8790
```

## Tech Stack

- **Go**: Gin (HTTP router), `go:embed` (前端嵌入)
- **Frontend**: Vue 3 (Composition API + `<script setup>`), Vue Router 4, Element Plus 2 (中文), Vite 8, Vitest, pnpm
- **Design**: WhatsApp brand colors (#128C7E / #075E54 / #25D366), dark green sidebar, Element Plus 主题覆写
