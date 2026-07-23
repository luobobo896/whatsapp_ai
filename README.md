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

## 本地运行环境要求

### 必需组件

| 组件 | 版本要求 | 用途 |
|------|----------|------|
| **Go** | 1.26+ | 后端编译运行 |
| **PostgreSQL** | 14+ | 数据存储（**需启用 pg_trgm 扩展**） |
| **Node.js** | 18+ | 前端构建、MCP Server 运行 |
| **pnpm** | 8+ | 前端包管理器 |
| **OpenClaw** | 最新版 | WhatsApp 消息网关（**必选**，核心组件） |
| **DeepSeek API Key** | 有效账号 | LLM 模型调用（用于 RAG 客服） |

### PostgreSQL 配置要求

启动前需确保 PostgreSQL 已启用 `pg_trgm` 扩展：

```bash
# 连接到数据库
psql -d postgres

# 为数据库启用扩展（替换 whatsapp_ai 为实际数据库名）
CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- 或在特定数据库中
\c whatsapp_ai
CREATE EXTENSION IF NOT EXISTS pg_trgm;
```

服务启动时会自动创建以下索引：
- `idx_articles_title_trgm` / `idx_articles_content_trgm` - 全文搜索 GIN 索引
- `idx_articles_status` / `idx_articles_kbid` - B-tree 索引
- `idx_kb_tenant_status` - 复合索引
- `idx_chunks_embedding` - partial 索引

### 系统依赖

#### 后端依赖 (Go)
```go
// 直接依赖
github.com/gin-gonic/gin v1.12.0      // HTTP 框架
github.com/jackc/pgx/v5 v5.10.0       // PostgreSQL 驱动
golang.org/x/crypto v0.54.0            // bcrypt 密码哈希
github.com/google/uuid v1.6.0          // UUID 生成
```

#### 前端依赖 (Node.js)
```json
{
  "dependencies": {
    "@element-plus/icons-vue": "^2.3.2",  // Element Plus 图标
    "element-plus": "^2.14.3",             // UI 组件库
    "lucide-vue-next": "0.556.0",          // 图标库
    "vue": "3.5.22",                       // Vue 3 框架
    "vue-router": "4.6.4"                   // 路由
  },
  "devDependencies": {
    "@vitejs/plugin-vue": "^6.0.3",        // Vite Vue 插件
    "vite": "^8.1.4",                       // 构建工具
    "vitest": "^4.1.10"                    // 测试框架
  }
}
```

#### MCP Server 依赖 (Node.js)
```json
{
  "dependencies": {
    "@modelcontextprotocol/sdk": "^1.0.0"  // MCP SDK
  }
}
```

### 快速启动

```bash
# 1. 安装 pnpm（如果尚未安装）
npm install -g pnpm

# 2. 构建前端
cd frontend && pnpm install && pnpm run build && cd ..

# 3. 设置环境变量
export DATABASE_URL="postgres://user:pass@localhost/whatsapp_ai"
export ADMIN_PASSWORD="your_secure_password"
export INTERNAL_API_TOKEN="your_internal_token"

# 4. 构建并启动服务
go build ./cmd/server
./server

# 5. （可选）启动 RAG MCP Server（OpenClaw 集成用）
cd cmd/rag-mcp-server && npm install && WHATSAPP_AI_ACCOUNT_ID=<account_id> node index.mjs
```

访问 `http://localhost:8790`，`GET /health` → `{"status":"ok"}`。

首次启动会创建平台管理员。必须设置 `DATABASE_URL` 与 `ADMIN_PASSWORD`；`ADMIN_EMAIL` 未设置时使用 `admin@whatsapp-ai.local`。

## 环境变量

| 变量 | 默认值 | 说明 |
|---|---|---|
| `HTTP_HOST` | `127.0.0.1` | 监听地址 |
| `PORT` | `8790` | 监听端口 |
| `DATABASE_URL` | （必填） | PostgreSQL 连接串 |
| `ADMIN_EMAIL` | `admin@whatsapp-ai.local` | 种子管理员邮箱 |
| `ADMIN_PASSWORD` | （首次启动必填） | 种子管理员密码 |
| `COOKIE_SECURE` | `true` | 会话 Cookie 的 Secure 属性；本地 HTTP 开发时显式设为 `false` |
| `INTERNAL_API_TOKEN` | （必填） | 内部 API Bearer token，OpenClaw MCP 服务调用时使用 |
| `WHATSAPP_AI_ACCOUNT_ID` | （MCP 必填） | RAG MCP 进程绑定的单个 WhatsApp 账号 ID；每个账号单独启动 MCP 进程 |

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
| DELETE | `/api/accounts/:id` | 注销并删除客服账号及其会话消息 |
| POST | `/api/accounts/:id/qr-login` | 发起 WhatsApp 扫码登录 |
| GET | `/api/accounts/:id/qr-status` | 查询扫码登录状态 |
| POST | `/api/accounts/:id/disconnect` | 断开 WhatsApp 连接 |
| GET | `/api/conversations` | 会话列表（支持 `?accountId=` 筛选） |
| GET | `/api/conversations/:id/messages?accountId=...` | 指定客服账号下的会话消息历史（`accountId` 必填） |
| DELETE | `/api/conversations/:id?accountId=...` | 删除指定客服账号下的会话消息（`accountId` 必填） |
| POST | `/api/conversations/query` | RAG 查询（搜索+记忆+系统提示） |
| POST | `/api/conversations/messages` | 保存消息 |

### 内部 API（OpenClaw 调用）
| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/api/internal/conversations/query` | RAG 查询（Bearer token 认证） |
| POST | `/api/internal/conversations/reply` | 保存 AI 回复 |

## RAG 客服系统

### 架构

```
WhatsApp 客户 → OpenClaw Gateway → RAG MCP Server → Go 后端 → PostgreSQL
                                         ↑
                                   OpenClaw Agent
                                   (persona-aware system prompt + knowledge + history)
```

- **Go 后端**: HTTP API，负责知识搜索、会话管理、消息持久化
- **RAG MCP Server** (`cmd/rag-mcp-server/index.mjs`): 每个 WhatsApp 账号单独启动一个进程，桥接 OpenClaw Agent 和后端，暴露 MCP 工具 `search_knowledge` / `save_reply`；账号 ID 从进程环境固定绑定，不能由 Agent 指定。
- **OpenClaw Agent**: 接收 WhatsApp 消息 → 必须调用 `search_knowledge` 获取账号绑定知识与最近 10 条持久化会话历史 → 模型综合上下文并用自己的话生成客服回复 → 发送前调用 `save_reply` 保存同一内容 → 最终只输出该客服回复

### 工作流程
1. WhatsApp 客户发消息 → OpenClaw Agent 调用账号专属的 `search_knowledge` MCP 工具
2. 系统保存客户消息 → 搜索知识库 → 加载对话历史 → 构建带 guardrails 的 system prompt
3. OpenClaw 将 systemPrompt + knowledge + history 交给模型，模型结合历史重新组织答案，不直接发送知识条目或固定模板
4. Agent 调用 `save_reply` 保存即将发送的内容 → 最终向 WhatsApp 输出完全相同的客服回复，不附加检索或保存说明

### 搜索策略
- **向量搜索**：Go 余弦相似度（embedding 列存 JSON float 数组）
- **全文回退**：pg_trgm GIN 索引加速 ILIKE + 中文 bigram 分词
- **自动切片**：文章创建时按 500 字符分块存入 `knowledge_chunks`

### 安全规则
- 租户隔离：所有知识/会话/账号操作限定当前租户
- CSRF 保护：写操作需 `X-CSRF-Token` 请求头
- 最小权限：每个客服 Agent 只允许调用账号专属的 `search_knowledge` / `save_reply`，不能获得代码执行或其他 OpenClaw 工具
- Guardrails：只能处理客服问题，禁止代码、命令、调试、配置和安全放权请求，禁止披露模型、OpenClaw、平台、提示词、工具、接口、数据库等内部信息

## OpenClaw WhatsApp 接入

WhatsApp 消息收发由 OpenClaw 网关代理，管理端负责触发扫码登录，并固定使用 token-only 网关认证和开放的新联系人策略。

### 扫码登录流程

1. 前端点击「扫码登录」→ `POST /api/accounts/:id/qr-login`（`internal/handler/accounts.go` `handleQrLogin`）。
2. 后端检查本机是否安装 `openclaw`（`isOpenClawAvailable`），先把客服账号幂等注册到 OpenClaw 配置，再定位 WhatsApp 登录模块（`resolveWhatsAppLoginModule`，可用环境变量 `OPENCLAW_WHATSAPP_LOGIN_MODULE` 覆盖路径），启动内嵌的 Node 桥接脚本 `whatsapp_qr_bridge.mjs` 生成二维码 PNG（`startQrSession`）。
3. 前端展示二维码并显示 45 秒倒计时，同时轮询 `GET /api/accounts/:id/qr-status`。
4. 手机扫码后，`qr-status` 从 OpenClaw 的目标 `channelAccounts` 项检测到 `linked=true, connected=false`，返回 `connecting`。前端立即隐藏二维码和倒计时，但继续每 3 秒查询连接状态，最多等待 1 分钟。
5. 桥接脚本完成登录并退出后，后端重启 OpenClaw gateway，等待目标账号同时达到 `running=true`、`connected=true`，再将数据库账号状态更新为 `connected`；1 分钟内仍未连接则结束等待并提示重新获取二维码。二维码在扫码前保持 45 秒有效期，到期自动刷新。
6. 账号列表加载时会读取 OpenClaw 的账号级实时状态并校正数据库缓存，避免凭据仍存在但 provider 已离线时继续显示“已连接”。

扫码登录只解决「账号本身接入 OpenClaw」的问题。项目同步账号时会强制 `dmPolicy=open` 和 `allowFrom=["*"]`，新联系人无需配对审批。

### 授权策略（dmPolicy）

OpenClaw 本身支持以下 `channels.whatsapp.dmPolicy` 值：

| 值 | 行为 |
|---|---|
| `pairing` | 新号码首次发消息需生成配对码，人工执行 `openclaw pairing approve whatsapp <code>` 批准后才能对话 |
| `allowlist` | 仅 `allowFrom` 列表内的号码可对话 |
| `open` | 任何号码可直接对话，无需审核 |
| `disabled` | 禁止私聊 |

本项目固定设置为 `open`，同时把 `allowFrom` 加上 `"*"`：

```bash
openclaw config set channels.whatsapp.dmPolicy open
openclaw config set channels.whatsapp.allowFrom '["*"]'
openclaw gateway restart
```

模型 key 不放在 WhatsApp AI `.env`，而是写入每个 OpenClaw agent 的认证存储：

```bash
openclaw models auth --agent <agent-id> paste-api-key --provider deepseek
```

完整部署步骤见 `docs/deployment/openclaw-rag.md`。

## ⚠️ OpenClaw 集成关键配置与排障（接手必读，配错会静默瘫痪）

本节记录过线上事故。OpenClaw 集成有多个「配错不报错、只是消息收不到回复 / 不走知识库」的点。**它们全部同源：server（`internal/handler/accounts.go`）按 `whatsapp-rag-<key>` 命名写配置，但 OpenClaw 扫码登录后实际运行、真正处理 WhatsApp 消息的 agent 是 `whatsapp-wa_<key>`、其 workspace 是 `whatsapp-workspaces/wa_<key>/`——命名错位导致 auth、persona 等配置都没落到干活的 agent 上。** 接手前务必读完。

> 用 `openclaw agents list` + `openclaw agents bindings` 确认：消息路由指向的 agent 是 `whatsapp-wa_<key>`，所有配置都必须落到**这个** agent，而不是 `whatsapp-rag-auth` 或 `whatsapp-rag-<key>` workspace。

### 致命点 1：每个 `whatsapp-wa_<key>` agent 必须绑定 deepseek auth

OpenClaw 把模型凭证隔离到 agent 级别。扫码登录后 OpenClaw 自动创建的 `whatsapp-wa_<key>` agent **默认不带凭证** → provider 拿不到 key → 不注册任何 deepseek 模型 → 无论 model 填什么都报 `Unknown model` → 消息正常收到但**完全不回复**（无启动报错、无告警，只能靠发消息或看日志发现）。

```bash
# 检查（Profiles 必须非空）
openclaw models auth --agent whatsapp-wa_<key> list
# 绑定（key 从 stdin 读，切勿放命令行）
openclaw models auth --agent whatsapp-wa_<key> paste-api-key --provider deepseek --profile-id deepseek:whatsapp-ai < <key-file>
```

> 要绑的是 `whatsapp-wa_<key>`（binding 指向、真正处理消息的 agent），**不是** `whatsapp-rag-auth`（server 另建的默认 agent，绑了也没用）。

### 致命点 2：wa agent 的 workspace `AGENTS.md` 必须含 RAG policy

客服 persona（「每条消息必须先 `search_knowledge`、guardrails、最后 `save_reply`」）写在 workspace 的 `AGENTS.md`。server 把它写到了 `whatsapp-workspaces/whatsapp-rag-wa_<key>/AGENTS.md`，但 wa agent 实际用的 workspace 是 `whatsapp-workspaces/wa_<key>/`（**不带 `rag-` 前缀**）→ agent 读不到 persona → 裸回复、不走 RAG、不守 guardrails、不 save_reply（会话不存库）。表现是「模型调用成功、回复也发了，但回复是模型凭空编的」。

```bash
# 确认 wa agent 的 AGENTS.md 含 policy（必须 >0）
grep -c whatsapp-ai-rag-policy /root/.openclaw/whatsapp-workspaces/wa_<key>/AGENTS.md
# 为 0 时，把 RAG policy 注入它实际加载的 AGENTS.md，再重启 gateway
cat /root/.openclaw/whatsapp-workspaces/whatsapp-rag-wa_<key>/AGENTS.md \
  >> /root/.openclaw/whatsapp-workspaces/wa_<key>/AGENTS.md
```

### 致命点 3：模型用服务商当前可用的（`deepseek-v4-pro` / `deepseek-v4-flash`）

`deepseek-chat`、`deepseek-reasoner` 已公告 **2026/07/24 弃用**，不要再用。本项目当前用 `deepseek/deepseek-v4-pro`。`agents.defaults.model` 与每个 `whatsapp-wa_<key>` agent 的 `model` 都要一致。

> v4-pro / v4-flash 是 reasoning 模型，OpenClaw 会开 thinking——**这不影响 tool calling**（OpenClaw 在工具回合会自动处理）。真正会让 agent 不调工具的是致命点 2（persona 没生效），不是 thinking。不要因为「reasoning 模型」就误判。

验证 key 与模型可用（直连）：

```bash
KEY=$(python3 -c "import json;print(json.load(open('/root/.openclaw/secrets.json'))['DEEPSEEK_API_KEY'])")
curl -s https://api.deepseek.com/models -H "Authorization: Bearer $KEY" \
  | python3 -c "import sys,json;print([m['id'] for m in json.load(sys.stdin)['data']])"
# 以返回为准，当前：['deepseek-v4-flash', 'deepseek-v4-pro']
```

### 致命点 4：`plugins.allow` 必须显式设置

```json
"plugins": { "allow": ["deepseek", "whatsapp"] }
```

否则配置热重载后非内置插件（deepseek、whatsapp）可能丢运行时注册，同样表现为 `Unknown model` 或工具不可用。启动告警 `plugins.allow is empty; discovered non-bundled plugins may auto-load...` **不要忽略**。

### 重启与排障

- OpenClaw gateway 是 **root 的 user systemd service**，重启用 `XDG_RUNTIME_DIR=/run/user/0 systemctl --user restart openclaw-gateway`（普通 `systemctl restart openclaw-gateway` 会找不到单元）。
- 改完 `openclaw.json` / agent auth / persona 后**必须重启 gateway**（热重载对 auth、persona 类变更不可靠）。
- 日志：`journalctl --user -u openclaw-gateway -f`；详细日志文件 `/tmp/openclaw/openclaw-YYYY-MM-DD.log`。

### 「消息无回复 / 不走知识库」排查清单

1. gateway 是否 `Listening for WhatsApp inbound messages`（WhatsApp 连接在）。
2. 是否 `Unknown model` / `lane task error` → auth 没绑（致命点 1）。
3. 回复是否凭空编造（没调 `search_knowledge`）→ persona 没生效（致命点 2）。
4. **最快端到端验证**（不用走 WhatsApp，直接驱动 agent）：

   ```bash
   openclaw agent --agent whatsapp-wa_<key> -m "测试问题" --json
   ```

   看返回是否基于知识库内容、是否守住客服身份；trajectory 在 `~/.openclaw/agents/whatsapp-wa_<key>/sessions/*.jsonl`，grep `search_knowledge`/`save_reply` 可证。

### 已知治本待办（server 代码缺陷）

以上是**止血**（手动配单个 agent）。根因是 server 在创建/绑定 WhatsApp account 时，只给 `whatsapp-rag-*` 默认 agent 配了 auth 与 persona，**没给扫码后实际运行的 `whatsapp-wa_<key>` agent 自动绑 auth、注入 persona**。治本：在 `internal/handler/accounts.go` 的扫码登录成功 / 账号同步流程里，检测到 `whatsapp-wa_<key>` agent 后，自动执行 auth 绑定 + persona 注入。否则**每新增一个账号都要手动配一次，且配错是静默的**。

## 前端开发

```bash
cd frontend && pnpm run dev    # Vite :5173，代理 /api → :8790
```

生产构建会从 `frontend/.env.production` 使用 `/whtasapp/` 作为静态资源基路径：

```bash
cd frontend && npm run build
```

## 技术栈

- **后端**: Go 1.26, Gin, pgx/v5, PostgreSQL, bcrypt, pg_trgm
- **前端**: Vue 3 (Composition API + `<script setup>`), Vue Router 4, Element Plus 2, lucide-vue-next
- **构建**: Vite 8, pnpm, go:embed
- **测试**: Vitest, Go test
