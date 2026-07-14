# CLAUDE.md

WhatsApp AI 管理端 — Go/Gin HTTP 服务 + Vue 3/Element Plus 前端（SPA 内嵌）。

WhatsApp 消息收发由外部 OpenClaw 组件对接，本项目提供管理端 + RAG 知识检索 API。

## 项目结构

```
├── cmd/
│   ├── server/        # HTTP 服务入口（Gin, graceful shutdown）
│   ├── rag-mcp-server/ # MCP Server（OpenClaw 集成桥接）
│   └── kbload/        # 知识库数据导入工具
├── internal/
│   ├── model/         # 数据模型
│   ├── store/         # PostgreSQL 数据访问（pgx/v5, pg_trgm）
│   ├── handler/       # HTTP 处理器
│   └── middleware/     # Auth/CSRF/Permission 中间件
├── web/               # 前端嵌入层（go:embed dist, SPA fallback）
├── frontend/           # Vue 3/Vite 管理端
│   ├── src/
│   │   ├── api/          # API 客户端（fetch 封装）
│   │   ├── components/   # 业务组件
│   │   ├── composables/  # 共享状态（useSession）
│   │   ├── router/       # Vue Router 路由
│   │   ├── views/        # 页面组件
│   │   └── styles/       # WhatsApp 品牌 Element Plus 主题覆写
│   └── ...
├── .env.example
├── README.md
└── CLAUDE.md
```

## 构建 & 运行

```bash
# 前端
cd frontend && pnpm install && pnpm run build   # → ../web/dist/

# 后端
go build ./cmd/server
go run ./cmd/server          # 默认 :8790，自动启动前端
```

`GET /health` → `{"status":"ok"}`，`/*` → 前端 SPA。

## 前端开发

```bash
cd frontend && pnpm run dev   # Vite :5173，代理 /api → :8790
```

## 数据库

PostgreSQL `new.hsddns.com:5432/whatsapp_ai`，账号 `admin` / `aircen123`。

启动时自动执行 DDL（`CREATE TABLE IF NOT EXISTS`）+ 索引 + pg_trgm 扩展。

管理员种子：`admin@whatsapp-ai.local` / `admin123456`（环境变量可覆盖）。

### 性能索引
- `idx_articles_title_trgm` / `idx_articles_content_trgm` — pg_trgm GIN 索引，加速 ILIKE
- `idx_articles_status` / `idx_articles_kbid` — B-tree
- `idx_kb_tenant_status` — 复合索引 (tenant_id, status)
- `idx_chunks_embedding` — partial 索引，仅索引有 embedding 的 chunks

## 技术栈

- **Go**: Gin (HTTP router), pgx/v5 (PostgreSQL), bcrypt, crypto/rand
- **Frontend**: Vue 3 (Composition API + `<script setup>`), Vue Router 4, Element Plus 2 (中文), Vite 8, Vitest, pnpm
- **Design**: WhatsApp brand colors (#128C7E / #075E54 / #25D366), dark green sidebar, Element Plus 主题覆写
- **DB**: PostgreSQL + pg_trgm 扩展

## RAG 搜索

- `POST /api/knowledge/search` — 向量（Go 余弦相似度）+ ILIKE bigram 回退
- `POST /api/conversations/query` — 一站式 RAG 接口（保存消息 + 搜索知识 + 加载历史 + 构建 system prompt）
- `POST /api/internal/conversations/query` — 内部 API（Bearer token），供 MCP Server 调用
- `DELETE /api/conversations/:id` — 删除会话及消息
- 文章创建/更新时自动按 500 字符切片到 `knowledge_chunks`
- system prompt 包含 guardrails：禁止超范围回答
- MCP Server (`cmd/rag-mcp-server/`) 暴露 `search_knowledge` / `list_accounts` / `save_reply` 三个工具给 OpenClaw Agent
