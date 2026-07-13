---
parent: map.md
type: grilling
status: closed
assignee: agent
resolution: use-openclaw-cli-for-credentials
---

## Question

How should credential storage be architected so that the admin backend does not interfere with OpenClaw's native credential management?

## Resolution

**Credential operations MUST use OpenClaw CLI. Config reads are fine. Config writes are a bridge necessity but should prefer CLI when available.**

### 当前问题分析

| 函数 | 当前做法 | 问题 |
|------|---------|------|
| `wipeAllCredentials()` | `fs.rmSync(credentials/whatsapp/)` | 直接删除 OpenClaw 凭证目录 |
| `removeAccountDirs()` | `fs.rmSync(credentials/... + sessions/...)` | 绕过 OpenClaw 的清理逻辑 |
| `clearAllRouterSessions()` | `fs.rmSync(agents/router-*/sessions/)` | 直接删除会话文件 |
| `readOpenClawConfig()` | `JSON.parse(fs.readFileSync(openclaw.json))` | ✅ 只读，可以保留 |
| `writeOpenClawConfig()` | `fs.writeFileSync(openclaw.json)` | ⚠️ 必要时保留，但优先用 CLI |
| `startWebLogin()` / `logoutAccount()` | 使用 OpenClaw runtime API | ✅ 已经是透传 |

### 决定：三层策略

#### 层1：凭证操作 → 必须用 OpenClaw CLI

```
openclaw channels logout --channel whatsapp --account <accountKey>
```
替代：`wipeAllCredentials()`, `removeAccountDirs()` 中的凭证删除部分

```
openclaw channels remove --channel whatsapp --account <accountKey> --delete
```
替代：`deleteAccount()` 中的账号删除流程

#### 层2：Agent 管理 → 优先用 OpenClaw CLI

OpenClaw 已有完整的 agent 管理命令：

| 操作 | CLI 命令 |
|------|---------|
| 新增 agent | `openclaw agents add` |
| 删除 agent | `openclaw agents delete <id>` (会自动清理 sessions) |
| 绑定路由 | `openclaw agents bind` |
| 解绑路由 | `openclaw agents unbind` |
| 新增 channel | `openclaw channels add` |

替代：`ensureAgentAndRoute()`, `removeAgentAndRoute()`, `clearAllRouterSessions()`

#### 层3：配置读取 → 保留，仅用于展示

`readOpenClawConfig()` 保留用于管理后台展示账号状态、模型列表、通道配置。
只读不写的原则确保不会破坏 OpenClaw 的配置完整性。

#### 层4：管理后台自己的数据 → 已在 `data/` 正确分离

`store.js` 管理的数据（`data/usage.json`, `data/messages.ndjson` 等）是管理后台自己的数据，
不涉及 OpenClaw 状态，架构已经正确。

### 凭证缓存（管理后台自用）

管理后台可以在 `data/` 下缓存账号状态快照用于 UI 展示，但：
- 缓存的凭证信息仅用于 UI（如显示"已登录/未登录"状态）
- 不从缓存中恢复或写入 OpenClaw 状态
- 缓存来源是 `openclaw channels status` 命令的输出，不是直接读凭证文件

### 边界总结

```
管理后台 (data/)                    OpenClaw (~/.openclaw-whatsapp-poc/)
├── usage.json          (自用)       ├── openclaw.json         ← 只读
├── messages.ndjson     (自用)       ├── credentials/whatsapp/  ← 只通过CLI操作
├── account-overrides.json (自用)    ├── agents/*/sessions/     ← 只通过CLI操作
├── settings.json       (自用)       └── agents/*/agent/       ← 只通过CLI操作
├── sandbox-messages.ndjson (自用)
├── audit-log.ndjson    (自用)
├── alerts.ndjson       (自用)
└── anti-ban.json       (自用)

交互方式:
  READ  → fs.readFileSync(openclaw.json)        ✅ 只读
  WRITE → openclaw CLI / runtime API             ✅ 透传
  NEVER → fs.rmSync / fs.writeFileSync            ❌ 不再直接操作
```

### 实施要点

修改文件：`src/openclaw-bridge.js`
- 替换 `wipeAllCredentials()` → 调用 `openclaw channels logout`
- 替换 `removeAccountDirs()` → 调用 `openclaw channels remove --delete`
- 替换 `clearAllRouterSessions()` → 调用 `openclaw agents delete` 或通过 logout 自动清理
- 保留 `readOpenClawConfig()` 只读
- `writeOpenClawConfig()` 降级为 fallback：只有 CLI 不支持的操作才使用
