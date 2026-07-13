## Destination

Refactor the WhatsApp AI POC into a cleanly-architected multi-language customer service system where: (1) reply language is auto-detected from the customer's first message and consistently used throughout the session, (2) conversation context persists across messages within a session until the customer explicitly ends it, and (3) the admin backend is architecturally separated from OpenClaw — OpenClaw is a standalone component, not bundled into the deployment. Credentials live natively in OpenClaw's storage; the admin backend may keep a transparent cache copy for its own use but must not interfere with OpenClaw's native credential paths.

## Notes

- Domain: WhatsApp AI customer service bot (Node.js, vanilla HTTP server, OpenClaw as WhatsApp gateway, DeepSeek as LLM)
- Key skills: `/domain-modeling`, `/grilling`, `/orca-cli`
- The local app runs at http://127.0.0.1:8790 (admin) + http://127.0.0.1:18789 (OpenClaw gateway)
- OpenClaw state dir: `~/.openclaw-whatsapp-poc/`
- Source: `whatsapp-ai-poc/src/` (server.js, openclaw-bridge.js, build-agent-prompt.js, store.js, config.js, knowledge.js)

## Decisions so far

- [Language Auto-Detection](ticket-01-language-detection.md) — **Prompt-based detection**: 无需外部库，通过三层 prompt 强化实现（去中文偏置 + 强化语言规则 + 语言锚定规则）。LLM 原生多语言能力即可完成检测。
- [Session Context Management](ticket-02-session-context.md) — **Leverage OpenClaw native sessions**: OpenClaw 已通过 `session.dmScope: "per-account-channel-peer"` 实现完美会话管理，每个客户有独立 JSONL 会话文件。问题不在基础设施而在 prompt 缺少上下文利用指令。新增规则 #8 即可。
- [Credential Architecture Separation](ticket-03-credential-architecture.md) — **Use OpenClaw CLI for credentials**: 凭证操作必须使用 OpenClaw CLI (`channels logout`, `channels remove --delete`)。配置读取保留只读。管理后台自有数据已在 `data/` 正确分离。
- [Deployment Separation](ticket-04-deployment-separation.md) — **Split scripts, remove watchdog**: 拆分为 `start-admin.sh` + `start-gateway.sh` 两个独立脚本。移除 `server.js` 中的 gateway watchdog。`scheduleGatewayRestart` 降级为手动触发。
- [Prompt Redesign](ticket-05-prompt-redesign.md) — **Implemented**: AGENTS.md 新增规则 #0（语言锚定）、重写规则 #5（回复风格）、重写规则 #7（多语言身份保密示例）、新增规则 #8（会话上下文）。`build-agent-prompt.js` 已更新并重新生成 prompt。
- [Admin Session Management](ticket-06-admin-session-management.md) — **Implemented**: `openclaw-bridge.js` 新增 CLI-based 凭证操作（`logoutAccountViaCli`, `removeAccountViaCli`, `deleteAgentViaCli`），旧函数标记 deprecated。`server.js` 移除 gateway watchdog，`clearAllSessions` 和 `deleteAccount` 改用 CLI 透传。启动脚本拆分为 `start-admin.sh` + `start-gateway.sh`。

## Not yet specified

- Exact session timeout / expiry policy (currently relies on OpenClaw defaults)
- Whether the "restart needed" indicator should be surfaced in the admin UI
- Production deployment model (Docker? systemd? PM2?)

## Out of scope

<!-- work beyond the destination -->
