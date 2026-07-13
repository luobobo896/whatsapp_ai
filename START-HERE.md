---
title: WhatsApp AI POC Migration Runbook
date: 2026-07-12
type: Guide
status: Approved
---

# WhatsApp AI POC macOS Migration

## 架构

```
WhatsApp 消息 → OpenClaw Gateway (:18789) → main agent → DeepSeek API → 回复
                                                  ↑
                                          AGENTS.md (动态生成)
                                          knowledge.json + accounts.json
```

**只有一个 agent（main）**，不再为每个账号创建独立 agent。所有 WhatsApp 账号共享一个 main agent，由 prompt 中的账号→角色映射表做权限隔离。

## Bundle

- `whatsapp-ai-poc/`: 管理后台、知识库、prompt 生成器、启动脚本。

OpenClaw 运行时配置在 `~/.openclaw-whatsapp-poc/`。包含 DeepSeek API key 和 WhatsApp 会话状态，视为敏感数据。

## On The Mac

1. 安装 Node.js 20+。
2. 安装 OpenClaw CLI（验证版本: `2026.6.11`）。
3. 解压到英文路径，例如 `~/Desktop/whatsapp-ai-poc-migration`。

## One-Command Start

```bash
cd ~/Desktop/whatsapp-ai-poc-migration/whatsapp-ai-poc
bash scripts/start-all.sh
```

脚本会：

- 重写 `~/.openclaw-whatsapp-poc/openclaw.json` 中的工作区路径
- `npm install`（node_modules 缺失时）
- 生成 AGENTS.md（`node src/build-agent-prompt.js`）
- 启动管理后台 `http://127.0.0.1:8790`
- 启动 OpenClaw Gateway `http://127.0.0.1:18789`

## 管理后台

```text
http://127.0.0.1:8790
```

功能：账号池管理（新增/删除/启停/扫码/清空会话）、知识库上传、问答历史。

## Verify

```bash
openclaw --profile whatsapp-poc channels status --deep
```

期望：

```text
WhatsApp phone_a: enabled, configured, linked, running, connected, health:healthy
```

## Re-login

WhatsApp Web 会话迁移后可能失效。打开管理后台 → 账号池 → 扫码登录。

## 知识库更新

编辑 `config/knowledge.json` 后，重新生成 prompt 并重启 gateway：

```bash
node src/build-agent-prompt.js
# 然后重启 gateway 或在管理后台保存知识库（自动触发）
```

## 权限模型

| 层级 | 机制 |
|------|------|
| 账号→角色 | `accounts.json` 中 `allowedProducts` 字段 |
| 角色→知识 | `knowledge.json` 中每个 role 的 products/faq/keywords |
| 越权检测 | main agent prompt 第 2、4 条规则：只答允许的角色，越权转人工 |
| 知识边界 | prompt 第 3 条规则：库外信息不编造 |
| 身份保密 | prompt 第 7 条规则：不透露模型/架构，只自称客服 |
| 安全限制 | prompt 第 6 条规则 + `tools.profile: minimal` |

## 项目结构

```
whatsapp-ai-poc/
├── src/
│   ├── server.js              # HTTP 服务 + 管理后台
│   ├── openclaw-bridge.js     # OpenClaw 交互层（配置/网关/WhatsApp API）
│   ├── build-agent-prompt.js  # 从 knowledge.json + accounts.json 生成 AGENTS.md
│   ├── router.js              # 消息路由
│   ├── store.js               # 数据存储
│   └── config.js              # 环境配置
├── config/
│   ├── accounts.json          # 账号→角色映射
│   └── knowledge.json         # 知识库（角色/产品/FAQ）
├── scripts/
│   ├── start-all.sh           # 一键启动
│   └── prepare-migration.sh   # 迁移准备
├── AGENTS.md                  # main agent prompt（由 build-agent-prompt.js 生成）
└── package.json
```

## 常见问题

- **UI 加载但无回复**：检查 `openclaw --profile whatsapp-poc channels status --deep`，确认账号 `connected` 且 `healthy`。
- **登录状态丢失**：重新扫码。
- **消息"正在输入"但无回复**：检查 `logs/openclaw-gateway.err.log`，确认 DeepSeek API key 已配置在 `openclaw.json` 的 `models.providers.deepseek.apiKey` 中。
- **不要用 localtunnel**，保持本地 `127.0.0.1`。
