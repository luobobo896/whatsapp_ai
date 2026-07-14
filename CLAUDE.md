# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

WhatsApp AI 管理端 — Go/Gin HTTP 服务 + React/Vite 前端（SPA 内嵌）。

WhatsApp 消息收发由外部独立组件对接，本项目仅负责管理端。

## Project Structure

```
├── cmd/server/    # HTTP 服务入口（Gin, graceful shutdown）
├── web/           # 前端嵌入层（go:embed dist, SPA fallback）
├── frontend/      # React/Vite 管理端（独立项目，构建输出到 ../web/dist）
├── .env.example   # 环境变量模板
├── README.md
└── CLAUDE.md
```

## Build & Run

```bash
# 前端
cd frontend && pnpm install && pnpm run build   # → ../web/dist/

# 后端
go build ./cmd/server
go run ./cmd/server          # 默认 :8790
```

`GET /health` → `{"status":"ok"}`，`/*` → 前端 SPA。

## Frontend Dev

```bash
cd frontend && pnpm run dev   # Vite :5173，代理 /api → :8790
```

## Tech Stack

- **Go**: Gin (HTTP router), `go:embed` (frontend embedding)
- **Frontend**: React 19, Vite 8, Vitest, pnpm
- **Design**: WhatsApp brand colors (#128C7E / #075E54 / #25D366), Chinese-first, dark green sidebar, no CSS framework
