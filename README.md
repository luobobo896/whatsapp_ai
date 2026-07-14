# WhatsApp AI 管理端

Go/Gin 后端 + Vue 3/Element Plus 前端，前端打包进 Go 二进制，启动即用。

## 项目结构

```
├── cmd/
│   ├── server/          # HTTP 服务入口（Gin, graceful shutdown）
│   ├── rag-mcp-server/  # MCP Server，桥接 OpenClaw Agent 与后端 RAG API
│   └── kbload/          # 知识库数据导入工具
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

# 启动 RAG MCP Server（OpenClaw 集成用）
cd cmd/rag-mcp-server && npm install && node index.mjs
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
| `INTERNAL_API_TOKEN` | （必填） | 内部 API Bearer token，OpenClaw MCP 服务调用时使用 |

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
| PATCH | `/api/accounts/:id` | 编辑客服账号（名称、知识库绑定、限额） |
| POST | `/api/accounts/:id/qr-login` | 发起 WhatsApp 扫码登录 |
| GET | `/api/accounts/:id/qr-status` | 查询扫码登录状态 |
| POST | `/api/accounts/:id/disconnect` | 断开 WhatsApp 连接 |
| GET | `/api/conversations` | 会话列表（支持 `?accountId=` 筛选） |
| GET | `/api/conversations/:id/messages` | 会话消息历史 |
| DELETE | `/api/conversations/:id` | 删除会话及所有消息 |
| POST | `/api/conversations/query` | RAG 查询（搜索+记忆+系统提示） |
| POST | `/api/conversations/messages` | 保存消息 |

### 内部 API（OpenClaw 调用）
| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/api/internal/conversations/query` | RAG 查询（Bearer token 认证） |
| POST | `/api/internal/conversations/reply` | 保存 AI 回复 |
| POST | `/api/internal/conversations/accounts/list` | 列出所有客服账号 |

## RAG 客服系统

### 架构

```
WhatsApp 客户 → OpenClaw Gateway → RAG MCP Server → Go 后端 → PostgreSQL
                                         ↑
                                   OpenClaw Agent
                                   (persona-aware system prompt + knowledge + history)
```

- **Go 后端**: HTTP API，负责知识搜索、会话管理、消息持久化
- **RAG MCP Server** (`cmd/rag-mcp-server/index.mjs`): 桥接 OpenClaw Agent 和后端，暴露 MCP 工具 `search_knowledge` / `list_accounts` / `save_reply`
- **OpenClaw Agent**: 接收 WhatsApp 消息 → 调 MCP 工具获取 RAG 上下文 → LLM 生成回复 → 发送并调用 `save_reply`

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

## OpenClaw WhatsApp 接入

WhatsApp 消息收发由 OpenClaw 网关代理，管理端负责触发扫码登录并控制新联系人的授权策略。

### 扫码登录流程

1. 前端点击「扫码登录」→ `POST /api/accounts/:id/qr-login`（`internal/handler/accounts.go` `handleQrLogin`）。
2. 后端检查本机是否安装 `openclaw`（`isOpenClawAvailable`），先把客服账号幂等注册到 OpenClaw 配置，再定位 WhatsApp 登录模块（`resolveWhatsAppLoginModule`，可用环境变量 `OPENCLAW_WHATSAPP_LOGIN_MODULE` 覆盖路径），启动内嵌的 Node 桥接脚本 `whatsapp_qr_bridge.mjs` 生成二维码 PNG（`startQrSession`）。
3. 前端展示二维码并显示 45 秒倒计时，同时轮询 `GET /api/accounts/:id/qr-status`。
4. 手机扫码后，`qr-status` 从 OpenClaw 的目标 `channelAccounts` 项检测到 `linked=true, connected=false`，返回 `connecting`。前端立即隐藏二维码和倒计时，但继续每 3 秒查询连接状态，最多等待 1 分钟。
5. 桥接脚本完成登录并退出后，后端重启 OpenClaw gateway，等待目标账号同时达到 `running=true`、`connected=true`，再将数据库账号状态更新为 `connected`；1 分钟内仍未连接则结束等待并提示重新获取二维码。二维码在扫码前保持 45 秒有效期，到期自动刷新。
6. 账号列表加载时会读取 OpenClaw 的账号级实时状态并校正数据库缓存，避免凭据仍存在但 provider 已离线时继续显示“已连接”。

扫码登录只解决「账号本身接入 OpenClaw」的问题；账号接入后，新联系人首次发消息是否需要人工审核，由下面的授权策略决定。

### 授权策略（dmPolicy）

是否需要人工审核新联系人由 `~/.openclaw/openclaw.json` 中 `channels.whatsapp.dmPolicy` 控制：

| 值 | 行为 |
|---|---|
| `pairing`（默认） | 新号码首次发消息需生成配对码，人工执行 `openclaw pairing approve whatsapp <code>` 批准后才能对话 |
| `allowlist` | 仅 `allowFrom` 列表内的号码可对话 |
| `open` | 任何号码可直接对话，无需审核 |
| `disabled` | 禁止私聊 |

设置为 `open` 时必须同时把 `allowFrom` 加上 `"*"`，否则所有消息会被丢弃（网关会给出警告提示）：

```bash
openclaw config set channels.whatsapp.dmPolicy open
openclaw config set channels.whatsapp.allowFrom '["*"]'
openclaw daemon restart   # 修改配置后需重启网关才能生效
```

只想临时放行某个待审核的号码，不改全局策略：

```bash
openclaw pairing approve whatsapp <code>
```

## 前端开发

```bash
cd frontend && pnpm run dev    # Vite :5173，代理 /api → :8790
```

## 技术栈

- **后端**: Go 1.26, Gin, pgx/v5, PostgreSQL, bcrypt, pg_trgm
- **前端**: Vue 3 (Composition API + `<script setup>`), Vue Router 4, Element Plus 2, lucide-vue-next
- **构建**: Vite 8, pnpm, go:embed
- **测试**: Vitest, Go test
