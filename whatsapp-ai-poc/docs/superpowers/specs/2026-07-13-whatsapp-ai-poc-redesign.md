# WhatsApp AI POC 全面重构设计文档

日期: 2026-07-13 | 状态: Draft

## 1. 概述

对 WhatsApp AI POC 管理后台进行全面重构，覆盖数据层迁移、API 重构、前端 UI 重写。

### 1.1 目标

1. 数据从 JSON 文件迁移到 SQLite + 向量存储
2. 客服账号流程闭环（新增 → 保存 → 扫码分离）
3. 知识库支持多类型、多对多角色关联
4. 模型配置透传 OpenClaw，支持多 provider 和兜底模型
5. 前端使用 frontend-design 风格 + UXUI 规范全面重构
6. 修复侧边栏滚动、防封策略为空、客户列表为空等问题

### 1.2 架构

```
Admin Server (8790)          OpenClaw Gateway (独立进程)
┌──────────────┐            ┌──────────────────┐
│  admin-page  │            │  WhatsApp 通道    │
│  (HTML/CSS)  │            │  Agent 执行       │
└──────┬───────┘            └────────┬─────────┘
       │ REST API                    │ CLI / Runtime API
┌──────▼──────────────────────────────▼─────────┐
│              server.js                         │
│  路由 · 认证 · 业务逻辑                          │
└──┬──────────┬──────────┬──────────────────────┘
   │          │          │
┌──▼────┐ ┌──▼────┐ ┌───▼──────────┐
│SQLite │ │OpenClaw│ │ 文件存储      │
│+vec   │ │Bridge  │ │ (备份/日志)   │
└───────┘ └────────┘ └──────────────┘
```

Admin Server 启动不依赖 OpenClaw。OpenClaw 独立部署，未运行时扫码/测试/发送操作返回错误提示。

---

## 2. 数据层

### 2.1 SQLite 表结构

```sql
-- 账号配置
CREATE TABLE accounts (
  id TEXT PRIMARY KEY,         -- accountKey, e.g. phone_dress
  label TEXT NOT NULL,         -- 客服名称
  display_phone TEXT DEFAULT '',
  agent_id TEXT NOT NULL,      -- router-xxx
  daily_limit INTEGER DEFAULT 30,
  enabled INTEGER DEFAULT 1,
  created_at TEXT DEFAULT (datetime('now')),
  updated_at TEXT DEFAULT (datetime('now'))
);

-- 账号-角色关联
CREATE TABLE account_roles (
  account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
  role_id TEXT NOT NULL REFERENCES knowledge_roles(id) ON DELETE CASCADE,
  PRIMARY KEY (account_id, role_id)
);

-- 知识角色
CREATE TABLE knowledge_roles (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  description TEXT DEFAULT '',
  unknown_reply TEXT DEFAULT '',
  out_of_hours_reply TEXT DEFAULT '',
  keywords TEXT DEFAULT '[]',  -- JSON array
  created_at TEXT DEFAULT (datetime('now')),
  updated_at TEXT DEFAULT (datetime('now'))
);

-- 知识库
CREATE TABLE knowledge_bases (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  type TEXT NOT NULL CHECK(type IN ('product','faq','document','conversation')),
  description TEXT DEFAULT '',
  created_at TEXT DEFAULT (datetime('now')),
  updated_at TEXT DEFAULT (datetime('now'))
);

-- 知识库-角色关联 (多对多)
CREATE TABLE knowledge_base_roles (
  base_id TEXT NOT NULL REFERENCES knowledge_bases(id) ON DELETE CASCADE,
  role_id TEXT NOT NULL REFERENCES knowledge_roles(id) ON DELETE CASCADE,
  PRIMARY KEY (base_id, role_id)
);

-- 知识条目
CREATE TABLE knowledge_entries (
  id TEXT PRIMARY KEY,
  base_id TEXT NOT NULL REFERENCES knowledge_bases(id) ON DELETE CASCADE,
  title TEXT NOT NULL,
  content TEXT NOT NULL,
  category_id TEXT REFERENCES knowledge_categories(id) ON DELETE SET NULL,
  enabled INTEGER DEFAULT 1,
  embedding BLOB,              -- sqlite-vec 向量
  metadata TEXT DEFAULT '{}',  -- JSON: 产品扩展字段
  created_at TEXT DEFAULT (datetime('now')),
  updated_at TEXT DEFAULT (datetime('now'))
);

-- 产品扩展字段存在 metadata JSON 中: price, sizes, colors, delivery, selling_points, notes

-- 产品分类标签
CREATE TABLE knowledge_categories (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  base_id TEXT NOT NULL REFERENCES knowledge_bases(id) ON DELETE CASCADE,
  sort_order INTEGER DEFAULT 0
);

-- 消息记录
CREATE TABLE messages (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  account_key TEXT NOT NULL,
  agent_id TEXT DEFAULT '',
  customer TEXT DEFAULT '',
  direction TEXT NOT NULL CHECK(direction IN ('inbound','outbound')),
  text TEXT NOT NULL,
  status TEXT DEFAULT 'recorded',
  session_id TEXT DEFAULT '',
  at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_messages_account ON messages(account_key, at);
CREATE INDEX idx_messages_customer ON messages(customer, at);

-- 防封策略
CREATE TABLE antiban_config (
  id INTEGER PRIMARY KEY CHECK(id = 1),  -- 单行配置
  reply_delay_min INTEGER DEFAULT 1000,
  reply_delay_max INTEGER DEFAULT 5000,
  rate_limit_hour INTEGER DEFAULT 30,
  rate_limit_day INTEGER DEFAULT 200,
  warmup_enabled INTEGER DEFAULT 1,
  warmup_hours INTEGER DEFAULT 24,
  keepalive_enabled INTEGER DEFAULT 1,
  keepalive_interval INTEGER DEFAULT 5,
  updated_at TEXT DEFAULT (datetime('now'))
);

-- 设置
CREATE TABLE settings (
  id INTEGER PRIMARY KEY CHECK(id = 1),
  work_start TEXT DEFAULT '09:00',
  work_end TEXT DEFAULT '18:00',
  timezone TEXT DEFAULT 'Asia/Shanghai',
  out_of_hours_reply TEXT DEFAULT '',
  greeting TEXT DEFAULT '',
  default_daily_limit INTEGER DEFAULT 30,
  updated_at TEXT DEFAULT (datetime('now'))
);

-- 审计日志
CREATE TABLE audit_log (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  action TEXT NOT NULL,
  detail TEXT DEFAULT '',
  ip TEXT DEFAULT '',
  at TEXT DEFAULT (datetime('now'))
);
```

### 2.2 向量存储

使用 `sqlite-vec` 扩展，在 `knowledge_entries.embedding` 列存储向量。产品/FAQ 条目保存时自动生成 embedding（title + content 拼接），用于语义搜索。

### 2.3 数据迁移

启动时检测 SQLite 是否存在，若不存在则从 `config/knowledge.json`、`config/accounts.json`、`data/*.json` 迁移数据。

---

## 3. API 设计

### 3.1 账号管理

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/accounts` | 列表（含在线状态） |
| POST | `/api/accounts` | 新增（只保存，不扫码） |
| PUT | `/api/accounts/:key` | 编辑（名称、上限、角色） |
| DELETE | `/api/accounts/:key` | 删除（同时清理 OpenClaw） |
| POST | `/api/accounts/:key/qr` | 生成扫码二维码 |
| GET | `/api/accounts/:key/qr-status` | 轮询扫码状态 |
| PUT | `/api/accounts/:key/toggle` | 启停账号 |

### 3.2 知识库

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/knowledge/roles` | 角色列表 |
| POST | `/api/knowledge/roles` | 新增角色 |
| PUT | `/api/knowledge/roles/:id` | 编辑角色 |
| DELETE | `/api/knowledge/roles/:id` | 删除角色 |
| GET | `/api/knowledge/bases` | 知识库列表 |
| POST | `/api/knowledge/bases` | 新建知识库 |
| PUT | `/api/knowledge/bases/:id` | 编辑知识库 |
| DELETE | `/api/knowledge/bases/:id` | 删除知识库 |
| GET | `/api/knowledge/bases/:id/entries` | 条目列表（分页） |
| POST | `/api/knowledge/bases/:id/entries` | 新增条目 |
| PUT | `/api/knowledge/entries/:id` | 编辑条目 |
| DELETE | `/api/knowledge/entries/:id` | 删除条目 |
| POST | `/api/knowledge/bases/:id/import-csv` | CSV 批量导入 |
| POST | `/api/knowledge/search` | 语义搜索（向量） |
| GET | `/api/knowledge/categories` | 分类列表 |
| POST | `/api/knowledge/categories` | 新增分类 |

### 3.3 模型配置

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/models` | 读取 OpenClaw models 配置 |
| PUT | `/api/models` | 写入 OpenClaw models 配置 |
| POST | `/api/models/providers` | 新增 provider |
| DELETE | `/api/models/providers/:name` | 删除 provider |

直接读写 `openclaw.json` 的 `models` 和 `agents.defaults.model` 字段，透传不转换。

### 3.4 其他

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/messages` | 问答历史（分页+搜索） |
| GET | `/api/customers` | 客户列表（从消息聚合） |
| GET/PUT | `/api/antiban` | 防封策略读写 |
| GET/PUT | `/api/settings` | 设置读写 |
| GET | `/api/stats/overview` | 总览统计 |
| GET | `/api/audit-log` | 操作日志 |
| GET | `/api/alerts` | 告警列表 |
| POST | `/api/sandbox/test` | 快捷测试 |

### 3.5 移除的 API

- `/admin-api/channels` — 通道合并到账号管理
- `/admin-api/channels/save` — 同上

---

## 4. 前端设计

### 4.1 技术方案

保持单文件 HTML 架构（`admin-page.js`），使用 frontend-design 技能生成完整设计系统，配合 UXUI 规范约束交互。技术栈不变：原生 HTML/CSS/JS，Node.js 服务端渲染。

### 4.2 导航结构

```
WhatsApp AI Ops
────────────────────
📊 总览
────────────────────
👤 客服账号          ← 合并原"账号池"+"通道配置"
🧠 知识库
🔧 模型配置
────────────────────
💬 问答历史          ← 聊天气泡风格
👥 客户列表
📡 实时监控
────────────────────
🛡️ 防封策略
⚙️ 设置
────────────────────
📋 操作日志
🔔 告警通知
🧪 快捷测试
```

### 4.3 各页面要点

**总览** — 指标卡片 + 账号健康 + 7天趋势图 + 热门品类

**客服账号** — 上：新增表单（名称+上限+角色→保存）下：账号卡片列表（状态+扫码+启停+删除）。扫码 QR 弹窗展示，轮询状态。

**知识库** — 三 Tab：角色管理 / 知识库管理 / 分类标签。角色关联多个知识库。知识库类型：产品/FAQ/文档/对话。

**模型配置** — Provider 列表 → 模型列表 → 编辑。兜底模型下拉选择。直接映射 OpenClaw 配置结构。

**问答历史** — 聊天气泡布局：客户消息左对齐灰色气泡，客服回复右对齐绿色气泡。按时间线穿插排列，同会话多轮合并展示。支持搜索、筛选、分页、CSV 导出。

**客户列表** — 从消息表聚合：客户号码、所属账号、对话次数、首次/最近联系时间、最近消息预览。

**防封策略** — 回复延迟 / 频率限制 / 新号预热 / 会话保温，四个配置组，保存后接入 store.js 防封逻辑。

**设置** — 工作时间、自动回复模板、默认限额。

### 4.4 侧边栏修复

- `overflow-y: auto` + `max-height: 100vh`
- 菜单项精简后不需要滚动即可容纳全部项目

### 4.5 交互规范 (UXUI)

- 所有操作提供即时反馈（loading/success/error 状态）
- 删除操作需要二次确认弹窗
- 表单校验在前端和后端各做一次
- QR 扫码轮询 2.5s 间隔，超时或失败有明确提示
- 账号状态：待关联(灰色) / 在线(绿色) / 异常(红色) / 已停用(黄色)

---

## 5. 实施顺序

| 阶段 | 内容 | 依赖 |
|------|------|------|
| P1 | SQLite 建表 + 数据迁移 + API 层替换 | - |
| P2 | 客服账号闭环（前端+后端） | P1 |
| P3 | 知识库管理重构（角色+知识库+分类+向量）| P1 |
| P4 | 模型配置透传 | P1 |
| P5 | 问答历史聊天气泡 | P1 |
| P6 | 防封策略接入 + 客户列表 + 设置 | P1 |
| P7 | 前端整体重构（frontend-design 风格）| P1-P6 |
| P8 | 总览 + 快捷测试 + 实时监控 + 告警 + 日志 | P7 |

---

## 6. 自审清单

- [x] 无 TBD/TODO 占位符
- [x] 架构图与功能描述一致
- [x] 数据模型覆盖所有业务实体
- [x] API 设计覆盖所有页面需求
- [x] 前端导航明确分组
- [x] 实施顺序合理，每阶段可独立验证
