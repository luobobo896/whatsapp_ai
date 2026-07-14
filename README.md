# WhatsApp AI 管理端

Go/Gin 后端 + Vue 3/Element Plus 前端，前端打包进 Go 二进制，启动即用。

## 项目结构

```
├── cmd/server/          # HTTP 服务入口（Gin, graceful shutdown）
├── internal/
│   ├── model/           # 数据模型（请求/响应/DB 行类型）
│   ├── store/           # PostgreSQL 数据访问层（pgx/v5）
│   ├── handler/         # HTTP 处理器（auth, tenants, members, accounts, knowledge, conversations）
│   └── middleware/       # 认证/CSRF/权限中间件
├── web/                 # 前端嵌入层（go:embed dist → SPA fallback）
├── frontend/            # Vue 3/Vite 管理端
│   ├── src/
│   │   ├── api/         # API 客户端（fetch 封装）
│   │   ├── components/  # 业务组件（Brand, Dialog 等）
│   │   ├── composables/ # 共享状态（useSession）
│   │   ├── router/      # Vue Router 路由
│   │   ├── views/       # 页面组件
│   │   └── styles/      # WhatsApp 品牌 Element Plus 主题覆写
│   └── ...
└── .env.example
```

## 快速启动

```bash
# 构建前端
cd frontend && pnpm install && pnpm run build

# 构建并启动服务（前端已嵌入）
go build ./cmd/server
./server
```

访问 `http://localhost:8790`，`GET /health` → `{"status":"ok"}`。

默认管理员：`admin@whatsapp-ai.local` / `admin123456`（环境变量 `ADMIN_EMAIL` / `ADMIN_PASSWORD` 可覆盖）。

## 环境变量

| 变量 | 默认值 | 说明 |
|---|---|---|
| `HTTP_HOST` | `127.0.0.1` | 监听地址 |
| `PORT` | `8790` | 监听端口 |
| `DATABASE_URL` | `postgres://admin:aircen123@new.hsddns.com:5432/whatsapp_ai?sslmode=disable` | PostgreSQL 连接串 |
| `ADMIN_EMAIL` | `admin@whatsapp-ai.local` | 种子管理员邮箱 |
| `ADMIN_PASSWORD` | `admin123456` | 种子管理员密码 |

## API 总览

### 认证
| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/api/auth/login` | 登录 |
| POST | `/api/auth/logout` | 退出 |
| GET | `/api/auth/me` | 当前会话 |
| POST | `/api/auth/select-tenant` | 切换租户 |

### 租户 & 成员
| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/tenants` | 可访问的租户列表 |
| POST | `/api/platform/tenants` | 创建租户（平台管理员） |
| PATCH | `/api/platform/tenants/:id/status` | 暂停/恢复租户 |
| GET | `/api/members` | 租户成员列表 |
| POST | `/api/members/invitations` | 邀请成员 |
| PATCH | `/api/members/:userId` | 更新成员角色/状态 |
| POST | `/api/invitations/:token/accept` | 接受邀请 |

### 知识库（RAG）
| 方法 | 路径 | 说明 |
|---|---|---|
| GET/POST | `/api/knowledge/bases` | 知识库列表/创建 |
| GET/PATCH/DELETE | `/api/knowledge/bases/:id` | 知识库详情/编辑/删除 |
| GET/POST | `/api/knowledge/bases/:id/articles` | 知识条目列表/创建 |
| PATCH/DELETE | `/api/knowledge/bases/:id/articles/:articleId` | 编辑/删除条目 |
| POST | `/api/knowledge/search` | 知识搜索（向量+全文） |

### 会话 & 客服
| 方法 | 路径 | 说明 |
|---|---|---|
| GET | `/api/accounts` | 客服账号列表 |
| POST | `/api/accounts` | 创建客服账号 |
| GET | `/api/conversations` | 会话列表 |
| GET | `/api/conversations/:id/messages` | 会话消息历史 |
| POST | `/api/conversations/query` | RAG 查询（搜索+记忆+系统提示） |
| POST | `/api/conversations/messages` | 保存消息 |

## RAG 客服系统

### 工作流程
1. WhatsApp 客户发消息 → OpenClaw 调用 `POST /api/conversations/query`
2. 系统保存客户消息 → 搜索知识库 → 加载对话历史 → 构建带 guardrails 的 system prompt
3. OpenClaw 将 systemPrompt + knowledge + history 发给 AI 模型
4. AI 回复后 → OpenClaw 调用 `POST /api/conversations/messages` 保存客服回复

### 搜索策略
- **向量搜索**：Go 余弦相似度（embedding 列存 JSON float 数组）
- **全文回退**：pg_trgm GIN 索引加速 ILIKE + 中文 bigram 分词
- **自动切片**：文章创建时按 500 字符分块存入 `knowledge_chunks`

### 安全规则
- 租户隔离：所有知识/会话/账号操作限定当前租户
- CSRF 保护：写操作需 `X-CSRF-Token` 请求头
- Guardrails：system prompt 禁止 AI 超范围回答

## 前端开发

```bash
cd frontend && pnpm run dev    # Vite :5173，代理 /api → :8790
```

## 技术栈

- **后端**: Go 1.26, Gin, pgx/v5, PostgreSQL, bcrypt, pg_trgm
- **前端**: Vue 3 (Composition API + `<script setup>`), Vue Router 4, Element Plus 2, lucide-vue-next
- **构建**: Vite 8, pnpm, go:embed
- **测试**: Vitest, Go test
