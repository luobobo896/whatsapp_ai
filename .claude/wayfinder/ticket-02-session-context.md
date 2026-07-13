---
parent: map.md
type: research
status: closed
assignee: agent
resolution: leverage-openclaw-sessions
---

## Question

How should the system maintain conversation context across multiple messages from the same customer within a session?

## Resolution

**OpenClaw already provides persistent session context. The fix is prompt rules, not new infrastructure.**

### 关键发现：OpenClaw 已完美管理会话

通过检查 `sessions.json` 和 JSONL 会话文件，发现 OpenClaw 已经实现了完整的会话管理：

| 功能 | OpenClaw 实现 | 状态 |
|------|--------------|------|
| 会话标识 | `agent:{agentId}:whatsapp:{accountId}:direct:{phone}` | ✅ 已存在 |
| 会话隔离 | `session.dmScope: "per-account-channel-peer"` | ✅ 每个客户独立会话 |
| 消息历史 | JSONL 文件包含完整历史 | ✅ LLM 每轮都能看到全部历史 |
| 会话持久化 | `~/.openclaw-whatsapp-poc/agents/{id}/sessions/` | ✅ 跨消息持久化 |
| 会话生命周期 | `sessionStartedAt`, `lastInteractionAt` 时间戳 | ✅ 自动跟踪 |

### 问题根因：Prompt 缺少上下文利用指令

检查 `router-phone-whatsapp-2026-7-13-00-03-39` 的会话文件后发现：
- LLM 收到的 `systemPrompt.chars: 38351` 包含了 AGENTS.md
- LLM 能看到所有历史消息（JSONL 中的完整对话）
- 但 AGENTS.md 没有告诉 LLM 要**使用**这些历史消息来维持上下文
- 硬编码的中文回复破坏了对话连贯性

### 决定：添加会话上下文规则到 Prompt

在 AGENTS.md 中新增规则：

```
### 8. 会话上下文（Context Awareness）
- 你收到的每条消息都包含完整的对话历史。仔细阅读历史消息。
- 记住客户告诉你的信息：名字、偏好、之前问过的问题、已经讨论过的产品。
- 如果客户之前提到过名字，后续对话中自然地使用它。
- 如果客户之前询问过某个产品，后续关于该产品的追问不需要客户重复说明。
- 当客户说"and you?"、"what about..."等指代性语句时，从历史中找到指代对象。
- 如果客户的问题和之前的对话相关，在回答中体现你已经知道上下文。
- 只有当客户明确说"结束"/"bye"/"goodbye"或会话超过30分钟无消息时，视为对话结束。
- 对话未结束时，保持话题的连贯性和记忆。
```

### 会话生命周期策略

| 事件 | 行为 |
|------|------|
| 客户发第一条消息 | OpenClaw 自动创建会话 (sessionStartedAt) |
| 客户持续对话 | 会话持续，所有消息追加到同一 JSONL |
| 客户说 "bye"/"goodbye"/"结束" | LLM 回复告别语，会话标记为结束 |
| 客户超过 30 分钟无消息 | OpenClaw 自然断开会话（由 session 配置控制） |
| 管理员清空会话 | 通过管理后台的"清空全部会话"按钮手动触发 |

### 实施要点

- 不需要修改 `server.js`、`openclaw-bridge.js`、`store.js`
- 不需要新增数据存储或 API
- 只需修改 `src/build-agent-prompt.js` 中的规则生成逻辑
- 新增规则 #8 与现有规则不冲突

### 验证标准

1. 客户说 "My name is X" → 后续回复中看到 LLM 使用 "X"
2. 客户说 "And you?" → LLM 从上下文理解指代并正确回复
3. 客户连续讨论同一产品 → LLM 不需要客户重复产品信息
4. 客户说 "bye" → LLM 结束对话，不再追问
