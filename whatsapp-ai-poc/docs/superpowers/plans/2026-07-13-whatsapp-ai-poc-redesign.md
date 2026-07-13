# WhatsApp AI POC 重构实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将 WhatsApp AI POC 管理后台从 JSON 文件存储迁移到 SQLite + 向量存储，重写全部 API，并使用 frontend-design 风格 + UXUI 规范重构前端。

**Architecture:** SQLite (better-sqlite3 + sqlite-vec) 作为主存储，API 按模块拆分（accounts / knowledge / models / messages / customers / antiban / settings），前端保持单文件 HTML 架构但完全重写 CSS 和交互逻辑。

**Tech Stack:** Node.js 20+, better-sqlite3, sqlite-vec (可选), frontend-design CLI (生成设计系统 CSS)

## Global Constraints

- 必须保持 `admin-page.js` 单文件 HTML 架构（与当前一致）
- Admin Server (8790) 不依赖 OpenClaw Gateway 启动
- 模型配置直接读写 openclaw.json，不引入中间层
- 渠道固定为 WhatsApp，移除通道配置独立页面
- 每日限额 >= 0 时表示有限额，0 表示不限
- 所有删除操作需要二次确认

---

## 文件结构

```
src/
├── db.js                  # [NEW] SQLite 初始化、迁移、查询工具
├── api/
│   ├── accounts.js        # [NEW] 账号管理 API handlers
│   ├── knowledge.js       # [NEW] 知识库 API handlers  
│   ├── models.js          # [NEW] 模型配置 API handlers
│   ├── messages.js        # [NEW] 问答历史 API handlers
│   ├── customers.js       # [NEW] 客户列表 API handlers
│   ├── antiban.js         # [NEW] 防封策略 API handlers
│   └── settings.js        # [NEW] 设置 API handlers
├── server.js              # [MODIFY] 路由分发到各 API 模块
├── admin-page.js          # [MODIFY] 前端完全重写
├── config.js              # [MODIFY] 新增 DB_PATH, VEC_ENABLED
├── openclaw-bridge.js     # [KEEP, minor updates]
├── build-agent-prompt.js  # [MODIFY] 从 SQLite 读取知识库
└── store.js               # [MODIFY] 适配 SQLite，保留文件日志

package.json               # [MODIFY] 添加 better-sqlite3
```

### 模块职责

| 文件 | 职责 |
|------|------|
| `db.js` | 初始化 SQLite，建表，从 JSON 迁移数据，导出 query/run/get 方法 |
| `api/accounts.js` | CRUD + QR 扫码 + 启停 + 角色绑定 |
| `api/knowledge.js` | 角色 CRUD + 知识库 CRUD + 条目 CRUD + CSV 导入 + 分类 |
| `api/models.js` | 透传读写 openclaw.json 的 models 配置 |
| `api/messages.js` | 问答历史分页查询 + 搜索 + CSV 导出 |
| `api/customers.js` | 从 messages 表聚合客户列表 |
| `api/antiban.js` | 读写 antiban_config 表 |
| `api/settings.js` | 读写 settings 表 |

---

### Task 1: 安装依赖 + 配置扩展

**Files:**
- Modify: `whatsapp-ai-poc/package.json`
- Modify: `whatsapp-ai-poc/src/config.js`

- [ ] **Step 1: 安装 better-sqlite3**

```bash
cd /Users/hanson/work/个人文档/whtasapp-demo/whatsapp-ai-poc-mac-migration-20260712-172100/whatsapp-ai-poc
npm install better-sqlite3
```

Expected: `better-sqlite3` 添加到 `package.json` dependencies。

- [ ] **Step 2: 扩展 config.js — 添加 DB_PATH**

在 `src/config.js` 末尾追加:

```js
export const DB_PATH = process.env.DB_PATH || path.join(rootDir, "data", "app.db");
```

- [ ] **Step 3: 验证安装**

```bash
node -e "const Database = require('better-sqlite3'); const db = new Database(':memory:'); console.log('SQLite OK:', db.pragma('user_version')); db.close();"
```

Expected: `SQLite OK: 0`

---

### Task 2: SQLite 初始化 + 数据迁移

**Files:**
- Create: `whatsapp-ai-poc/src/db.js`

**Interfaces:**
- Produces: `export const db` (better-sqlite3 instance)
- Produces: `export function migrateFromJson()` — 从 JSON 文件迁移数据到 SQLite

- [ ] **Step 1: 创建 db.js — 建表逻辑**

```js
import Database from "better-sqlite3";
import fs from "node:fs";
import path from "node:path";
import { DB_PATH, rootDir } from "./config.js";

const DB_DIR = path.dirname(DB_PATH);
if (!fs.existsSync(DB_DIR)) fs.mkdirSync(DB_DIR, { recursive: true });

export const db = new Database(DB_PATH);
db.pragma("journal_mode = WAL");
db.pragma("foreign_keys = ON");

// 建表
db.exec(`
  CREATE TABLE IF NOT EXISTS accounts (
    id TEXT PRIMARY KEY,
    label TEXT NOT NULL,
    display_phone TEXT DEFAULT '',
    agent_id TEXT NOT NULL,
    daily_limit INTEGER DEFAULT 30,
    enabled INTEGER DEFAULT 1,
    created_at TEXT DEFAULT (datetime('now')),
    updated_at TEXT DEFAULT (datetime('now'))
  );

  CREATE TABLE IF NOT EXISTS account_roles (
    account_id TEXT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    role_id TEXT NOT NULL REFERENCES knowledge_roles(id) ON DELETE CASCADE,
    PRIMARY KEY (account_id, role_id)
  );

  CREATE TABLE IF NOT EXISTS knowledge_roles (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT DEFAULT '',
    unknown_reply TEXT DEFAULT '',
    out_of_hours_reply TEXT DEFAULT '',
    keywords TEXT DEFAULT '[]',
    created_at TEXT DEFAULT (datetime('now')),
    updated_at TEXT DEFAULT (datetime('now'))
  );

  CREATE TABLE IF NOT EXISTS knowledge_bases (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    type TEXT NOT NULL CHECK(type IN ('product','faq','document','conversation')),
    description TEXT DEFAULT '',
    created_at TEXT DEFAULT (datetime('now')),
    updated_at TEXT DEFAULT (datetime('now'))
  );

  CREATE TABLE IF NOT EXISTS knowledge_base_roles (
    base_id TEXT NOT NULL REFERENCES knowledge_bases(id) ON DELETE CASCADE,
    role_id TEXT NOT NULL REFERENCES knowledge_roles(id) ON DELETE CASCADE,
    PRIMARY KEY (base_id, role_id)
  );

  CREATE TABLE IF NOT EXISTS knowledge_categories (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    base_id TEXT NOT NULL REFERENCES knowledge_bases(id) ON DELETE CASCADE,
    sort_order INTEGER DEFAULT 0
  );

  CREATE TABLE IF NOT EXISTS knowledge_entries (
    id TEXT PRIMARY KEY,
    base_id TEXT NOT NULL REFERENCES knowledge_bases(id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    content TEXT NOT NULL,
    category_id TEXT REFERENCES knowledge_categories(id) ON DELETE SET NULL,
    enabled INTEGER DEFAULT 1,
    metadata TEXT DEFAULT '{}',
    created_at TEXT DEFAULT (datetime('now')),
    updated_at TEXT DEFAULT (datetime('now'))
  );

  CREATE TABLE IF NOT EXISTS messages (
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

  CREATE INDEX IF NOT EXISTS idx_messages_account ON messages(account_key, at);
  CREATE INDEX IF NOT EXISTS idx_messages_customer ON messages(customer, at);

  CREATE TABLE IF NOT EXISTS antiban_config (
    id INTEGER PRIMARY KEY CHECK(id = 1),
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

  CREATE TABLE IF NOT EXISTS settings (
    id INTEGER PRIMARY KEY CHECK(id = 1),
    work_start TEXT DEFAULT '09:00',
    work_end TEXT DEFAULT '18:00',
    timezone TEXT DEFAULT 'Asia/Shanghai',
    out_of_hours_reply TEXT DEFAULT '',
    greeting TEXT DEFAULT '',
    default_daily_limit INTEGER DEFAULT 30,
    updated_at TEXT DEFAULT (datetime('now'))
  );

  CREATE TABLE IF NOT EXISTS audit_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    action TEXT NOT NULL,
    detail TEXT DEFAULT '',
    ip TEXT DEFAULT '',
    at TEXT DEFAULT (datetime('now'))
  );

  -- 插入默认防封配置（如果不存在）
  INSERT OR IGNORE INTO antiban_config (id) VALUES (1);

  -- 插入默认设置（如果不存在）
  INSERT OR IGNORE INTO settings (id) VALUES (1);
`);

console.log("SQLite database initialized at", DB_PATH);
```

- [ ] **Step 2: 添加数据迁移函数**

在 `db.js` 末尾追加迁移函数。从 `config/knowledge.json`、`config/accounts.json` 迁移已有数据：

```js
export function migrateFromJson() {
  // 检查是否需要迁移（accounts 表为空）
  const count = db.prepare("SELECT COUNT(*) as c FROM accounts").get();
  if (count.c > 0) return { migrated: false, reason: "already has data" };

  const accountsPath = path.join(rootDir, "config", "accounts.json");
  const knowledgePath = path.join(rootDir, "config", "knowledge.json");

  let accountCount = 0;
  let roleCount = 0;
  let productCount = 0;

  // 迁移账号
  if (fs.existsSync(accountsPath)) {
    const accounts = JSON.parse(fs.readFileSync(accountsPath, "utf8"));
    const insertAccount = db.prepare(
      "INSERT INTO accounts (id, label, display_phone, agent_id, daily_limit) VALUES (?, ?, ?, ?, ?)"
    );
    const insertAccountRole = db.prepare(
      "INSERT OR IGNORE INTO account_roles (account_id, role_id) VALUES (?, ?)"
    );
    const tx = db.transaction(() => {
      for (const [key, acct] of Object.entries(accounts)) {
        insertAccount.run(key, acct.label || key, acct.displayPhone || "", acct.agentId || `router-${key}`, acct.dailyLimit || 30);
        for (const roleId of (acct.allowedProducts || [])) {
          insertAccountRole.run(key, roleId);
        }
        accountCount++;
      }
    });
    tx();
  }

  // 迁移知识库
  if (fs.existsSync(knowledgePath)) {
    const knowledge = JSON.parse(fs.readFileSync(knowledgePath, "utf8"));
    const insertRole = db.prepare(
      "INSERT INTO knowledge_roles (id, name, description, unknown_reply, out_of_hours_reply, keywords) VALUES (?, ?, ?, ?, ?, ?)"
    );
    const insertBase = db.prepare(
      "INSERT OR IGNORE INTO knowledge_bases (id, name, type, description) VALUES (?, ?, ?, ?)"
    );
    const insertBaseRole = db.prepare(
      "INSERT OR IGNORE INTO knowledge_base_roles (base_id, role_id) VALUES (?, ?)"
    );
    const insertEntry = db.prepare(
      "INSERT INTO knowledge_entries (id, base_id, title, content, metadata) VALUES (?, ?, ?, ?, ?)"
    );

    const tx = db.transaction(() => {
      for (const role of (knowledge.roles || [])) {
        insertRole.run(role.id, role.name, role.description || "", role.unknownReply || "", role.outOfHoursReply || "", JSON.stringify(role.keywords || []));
        roleCount++;

        // 为每个角色创建一个产品知识库
        const baseId = `base-${role.id}`;
        insertBase.run(baseId, `${role.name}产品库`, "product", `${role.name}的产品数据`);
        insertBaseRole.run(baseId, role.id);

        // 迁移产品为条目
        for (const product of (role.products || [])) {
          const content = [product.description, ...(product.sellingPoints || []), ...(product.notes || [])].join("\n");
          insertEntry.run(product.id, baseId, product.name, content, JSON.stringify({
            price: product.price || "",
            sizes: product.sizes || [],
            colors: product.colors || [],
            delivery: product.delivery || "",
            sellingPoints: product.sellingPoints || [],
            notes: product.notes || []
          }));
          productCount++;
        }
      }
    });
    tx();
  }

  console.log(`Migration complete: ${accountCount} accounts, ${roleCount} roles, ${productCount} products`);
  return { migrated: true, accountCount, roleCount, productCount };
}
```

- [ ] **Step 3: 启动时自动迁移**

在 `db.js` 末尾调用：

```js
// 自动迁移（如果数据库为空且 JSON 文件存在）
migrateFromJson();
```

- [ ] **Step 4: 验证迁移**

```bash
cd /Users/hanson/work/个人文档/whtasapp-demo/whatsapp-ai-poc-mac-migration-20260712-172100/whatsapp-ai-poc
node -e "
import('./src/db.js').then(({db}) => {
  const accounts = db.prepare('SELECT * FROM accounts').all();
  const roles = db.prepare('SELECT * FROM knowledge_roles').all();
  const entries = db.prepare('SELECT COUNT(*) as c FROM knowledge_entries').get();
  console.log('Accounts:', accounts.length);
  console.log('Roles:', roles.length);
  console.log('Entries:', entries.c);
  db.close();
});
"
```

Expected: 显示迁移后的账号数、角色数、条目数。

---

### Task 3: 账号管理 API

**Files:**
- Create: `whatsapp-ai-poc/src/api/accounts.js`

**Interfaces:**
- Consumes: `db` from `../db.js`, OpenClaw functions from `../openclaw-bridge.js`
- Produces: `export function registerAccountRoutes(server)` — HTTP 路由注册

- [ ] **Step 1: 创建 accounts.js**

```js
import { db } from "../db.js";
import * as openclaw from "../openclaw-bridge.js";
import { generateAgentPrompt } from "../build-agent-prompt.js";

const accountLoginJobs = new Map();

function clientIp(request) {
  return request.headers["x-forwarded-for"]?.split(",")[0]?.trim() || request.socket.remoteAddress || "127.0.0.1";
}

function sanitizeLabel(v) {
  const label = String(v || "").trim();
  return label || `WhatsApp ${new Date().toLocaleString("zh-CN")}`;
}

function makeAccountKey(label) {
  const base = String(label || "").trim().toLowerCase().replace(/[^a-z0-9_-]+/g, "_").replace(/^_+|_+$/g, "") || `phone_${Date.now().toString(36)}`;
  let key = base.startsWith("phone_") ? base : `phone_${base}`;
  let idx = 2;
  while (db.prepare("SELECT 1 FROM accounts WHERE id = ?").get(key)) {
    key = `phone_${base}_${idx++}`;
  }
  return key;
}

async function appendAudit(action, detail, ip) {
  db.prepare("INSERT INTO audit_log (action, detail, ip) VALUES (?, ?, ?)").run(action, detail, ip || "127.0.0.1");
}

function buildAccountStatus() {
  const accounts = db.prepare("SELECT * FROM accounts ORDER BY created_at DESC").all();
  const channelStatus = openclaw.readChannelStatus();

  return accounts.map(acct => {
    const live = channelStatus[acct.id] || {};
    const roles = db.prepare("SELECT role_id FROM account_roles WHERE account_id = ?").all(acct.id).map(r => r.role_id);
    const usedToday = db.prepare(
      "SELECT COUNT(*) as c FROM messages WHERE account_key = ? AND direction = 'outbound' AND date(at) = date('now','localtime')"
    ).get(acct.id)?.c || 0;
    const linked = live.linked || false;
    const connected = live.connected || false;
    const healthy = live.healthy || false;
    let status = "pending"; // 待关联
    if (connected && healthy) status = "online";
    else if (linked) status = "reconnecting";
    else if (!acct.enabled) status = "disabled";
    else if (!linked && acct.enabled) status = "pending";

    return {
      ...acct,
      roles,
      enabled: !!acct.enabled,
      daily_limit: acct.daily_limit,
      usedToday,
      status,
      live,
      liveRefreshedAt: channelStatus._refreshedAt || null
    };
  });
}

export function registerAccountRoutes(server) {
  // 注册路由到 server（在 server.js 中调用）
  // 这里导出 handler 函数供 server.js 使用
}

export const accountHandlers = {
  // GET /api/accounts
  list() {
    return { ok: true, accounts: buildAccountStatus() };
  },

  // POST /api/accounts — 新增（只保存，不扫码）
  create(body) {
    const label = sanitizeLabel(body.label);
    const dailyLimit = Math.max(0, Number(body.dailyLimit ?? 30));
    const roleIds = Array.isArray(body.roles) ? body.roles.map(String).filter(Boolean) : [];

    // 验证角色存在
    for (const rid of roleIds) {
      if (!db.prepare("SELECT 1 FROM knowledge_roles WHERE id = ?").get(rid)) {
        throw new Error(`unknown role: ${rid}`);
      }
    }

    const accountKey = makeAccountKey(label);
    const agentId = `router-${accountKey.replace(/_/g, "-")}`;

    const tx = db.transaction(() => {
      db.prepare("INSERT INTO accounts (id, label, agent_id, daily_limit) VALUES (?, ?, ?, ?)").run(accountKey, label, agentId, dailyLimit);
      for (const rid of roleIds) {
        db.prepare("INSERT OR IGNORE INTO account_roles (account_id, role_id) VALUES (?, ?)").run(accountKey, rid);
      }
    });
    tx();

    generateAgentPrompt();
    appendAudit("account.create", `Created ${accountKey} (${label})`, "127.0.0.1");

    return { ok: true, accountKey, agentId, label };
  },

  // PUT /api/accounts/:key
  update(accountKey, body) {
    const acct = db.prepare("SELECT * FROM accounts WHERE id = ?").get(accountKey);
    if (!acct) throw new Error(`unknown account: ${accountKey}`);

    const label = body.label !== undefined ? sanitizeLabel(body.label) : acct.label;
    const dailyLimit = body.dailyLimit !== undefined ? Math.max(0, Number(body.dailyLimit)) : acct.daily_limit;

    const tx = db.transaction(() => {
      db.prepare("UPDATE accounts SET label = ?, daily_limit = ?, updated_at = datetime('now') WHERE id = ?").run(label, dailyLimit, accountKey);
      if (Array.isArray(body.roles)) {
        db.prepare("DELETE FROM account_roles WHERE account_id = ?").run(accountKey);
        for (const rid of body.roles.map(String).filter(Boolean)) {
          db.prepare("INSERT OR IGNORE INTO account_roles (account_id, role_id) VALUES (?, ?)").run(accountKey, rid);
        }
      }
    });
    tx();

    generateAgentPrompt();
    appendAudit("account.update", `${accountKey} updated`, "127.0.0.1");

    return { ok: true, accountKey };
  },

  // DELETE /api/accounts/:key
  async delete(accountKey) {
    const acct = db.prepare("SELECT * FROM accounts WHERE id = ?").get(accountKey);
    if (!acct) throw new Error(`unknown account: ${accountKey}`);

    // 清理 OpenClaw
    openclaw.removeAccountViaCli(accountKey).catch(e => console.error("CLI remove failed:", e.message));
    openclaw.deleteAgentViaCli(acct.agent_id).catch(e => console.error("CLI agent delete failed:", e.message));
    accountLoginJobs.delete(accountKey);

    db.prepare("DELETE FROM accounts WHERE id = ?").run(accountKey);

    generateAgentPrompt();
    openclaw.markRestartNeeded();
    appendAudit("account.delete", `Deleted ${accountKey}`, "127.0.0.1");

    return { ok: true, accountKey };
  },

  // POST /api/accounts/:key/qr — 扫码登录
  async startQr(accountKey) {
    const acct = db.prepare("SELECT * FROM accounts WHERE id = ?").get(accountKey);
    if (!acct) throw new Error(`unknown account: ${accountKey}`);

    const job = {
      accountKey,
      agentId: acct.agent_id,
      label: acct.label,
      status: "starting",
      message: "Starting WhatsApp QR login.",
      qrDataUrl: "",
      connected: false,
      createdAt: new Date().toISOString(),
      updatedAt: new Date().toISOString()
    };
    accountLoginJobs.set(accountKey, job);

    const result = await openclaw.startWebLogin(accountKey);
    job.updatedAt = new Date().toISOString();
    if (result.qrDataUrl) {
      job.status = "qr";
      job.qrDataUrl = result.qrDataUrl;
      job.message = "请用 WhatsApp 扫描二维码";
    } else if (result.connected) {
      job.status = "linked";
      job.connected = true;
      openclaw.createAgentViaCli(acct.agent_id, accountKey, acct.label);
      openclaw.refreshChannelStatus();
      openclaw.scheduleGatewayRestart();
    } else {
      job.status = "failed";
      job.message = result.message || "生成二维码失败";
    }

    return { ok: true, job };
  },

  // GET /api/accounts/:key/qr-status
  async qrStatus(accountKey) {
    const job = accountLoginJobs.get(accountKey);
    if (!job || job.status === "linked" || job.status === "failed") return { ok: true, job };

    const result = await openclaw.waitForWebLogin(accountKey, job.qrDataUrl);
    job.updatedAt = new Date().toISOString();
    if (result.qrDataUrl) { job.qrDataUrl = result.qrDataUrl; job.status = "qr"; }
    if (result.connected) {
      job.status = "linked";
      job.connected = true;
      const acct = db.prepare("SELECT * FROM accounts WHERE id = ?").get(accountKey);
      if (acct) {
        openclaw.createAgentViaCli(acct.agent_id, accountKey, acct.label);
        openclaw.refreshChannelStatus();
        openclaw.scheduleGatewayRestart();
      }
    }
    return { ok: true, job };
  },

  // PUT /api/accounts/:key/toggle
  toggle(accountKey, enabled) {
    db.prepare("UPDATE accounts SET enabled = ?, updated_at = datetime('now') WHERE id = ?").run(enabled ? 1 : 0, accountKey);
    openclaw.setAccountEnabled(accountKey, !!enabled);
    appendAudit("account.toggle", `${accountKey} ${enabled ? "enabled" : "disabled"}`, "127.0.0.1");
    return { ok: true };
  }
};
```

- [ ] **Step 2: 验证 API**

```bash
node -e "
import('./src/api/accounts.js').then(({accountHandlers}) => {
  console.log('create:', accountHandlers.create({label:'测试客服', dailyLimit:30, roles:[]}));
  console.log('list:', accountHandlers.list().accounts.length + ' accounts');
});
"
```

---

### Task 4: 知识库 API

**Files:**
- Create: `whatsapp-ai-poc/src/api/knowledge.js`

**Interfaces:**
- Consumes: `db` from `../db.js`
- Produces: `export const knowledgeHandlers`

- [ ] **Step 1: 创建 knowledge.js**

```js
import { db } from "../db.js";
import { generateAgentPrompt } from "../build-agent-prompt.js";

function genId(prefix = "e") {
  return `${prefix}-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 8)}`;
}

export const knowledgeHandlers = {
  // ── 角色 CRUD ──
  listRoles() {
    const roles = db.prepare("SELECT * FROM knowledge_roles ORDER BY created_at DESC").all();
    return roles.map(r => ({
      ...r,
      keywords: JSON.parse(r.keywords || "[]"),
      bases: db.prepare(
        "SELECT kb.* FROM knowledge_bases kb JOIN knowledge_base_roles kbr ON kb.id = kbr.base_id WHERE kbr.role_id = ?"
      ).all(r.id)
    }));
  },

  createRole(body) {
    const id = String(body.id || "").trim() || genId("role");
    const name = String(body.name || "").trim();
    if (!name) throw new Error("name required");
    db.prepare(
      "INSERT INTO knowledge_roles (id, name, description, unknown_reply, out_of_hours_reply, keywords) VALUES (?, ?, ?, ?, ?, ?)"
    ).run(id, name, body.description || "", body.unknownReply || "", body.outOfHoursReply || "", JSON.stringify(body.keywords || []));
    generateAgentPrompt();
    return { ok: true, id };
  },

  updateRole(id, body) {
    const role = db.prepare("SELECT * FROM knowledge_roles WHERE id = ?").get(id);
    if (!role) throw new Error(`unknown role: ${id}`);
    db.prepare(
      "UPDATE knowledge_roles SET name=?, description=?, unknown_reply=?, out_of_hours_reply=?, keywords=?, updated_at=datetime('now') WHERE id=?"
    ).run(
      body.name || role.name, body.description ?? role.description,
      body.unknownReply ?? role.unknown_reply, body.outOfHoursReply ?? role.out_of_hours_reply,
      JSON.stringify(body.keywords || JSON.parse(role.keywords || "[]")), id
    );
    generateAgentPrompt();
    return { ok: true };
  },

  deleteRole(id) {
    db.prepare("DELETE FROM knowledge_roles WHERE id = ?").run(id);
    generateAgentPrompt();
    return { ok: true };
  },

  // ── 知识库 CRUD ──
  listBases() {
    const bases = db.prepare("SELECT * FROM knowledge_bases ORDER BY created_at DESC").all();
    return bases.map(b => ({
      ...b,
      roles: db.prepare("SELECT role_id FROM knowledge_base_roles WHERE base_id = ?").all(b.id).map(r => r.role_id),
      entryCount: db.prepare("SELECT COUNT(*) as c FROM knowledge_entries WHERE base_id = ?").get(b.id)?.c || 0
    }));
  },

  createBase(body) {
    const id = body.id || genId("base");
    const name = String(body.name || "").trim();
    const type = String(body.type || "product");
    if (!name) throw new Error("name required");
    if (!["product", "faq", "document", "conversation"].includes(type)) throw new Error("invalid type");

    const tx = db.transaction(() => {
      db.prepare("INSERT INTO knowledge_bases (id, name, type, description) VALUES (?, ?, ?, ?)").run(id, name, type, body.description || "");
      if (Array.isArray(body.roleIds)) {
        for (const rid of body.roleIds.map(String).filter(Boolean)) {
          db.prepare("INSERT OR IGNORE INTO knowledge_base_roles (base_id, role_id) VALUES (?, ?)").run(id, rid);
        }
      }
    });
    tx();
    return { ok: true, id };
  },

  updateBase(id, body) {
    const base = db.prepare("SELECT * FROM knowledge_bases WHERE id = ?").get(id);
    if (!base) throw new Error(`unknown base: ${id}`);
    const tx = db.transaction(() => {
      db.prepare("UPDATE knowledge_bases SET name=?, description=?, updated_at=datetime('now') WHERE id=?")
        .run(body.name || base.name, body.description ?? base.description, id);
      if (Array.isArray(body.roleIds)) {
        db.prepare("DELETE FROM knowledge_base_roles WHERE base_id = ?").run(id);
        for (const rid of body.roleIds.map(String).filter(Boolean)) {
          db.prepare("INSERT OR IGNORE INTO knowledge_base_roles (base_id, role_id) VALUES (?, ?)").run(id, rid);
        }
      }
    });
    tx();
    return { ok: true };
  },

  deleteBase(id) {
    db.prepare("DELETE FROM knowledge_bases WHERE id = ?").run(id);
    return { ok: true };
  },

  // ── 条目 CRUD ──
  listEntries(baseId, { page = 1, pageSize = 20, categoryId = "" } = {}) {
    let sql = "SELECT * FROM knowledge_entries WHERE base_id = ?";
    const params = [baseId];
    if (categoryId) {
      sql += " AND category_id = ?";
      params.push(categoryId);
    }
    const total = db.prepare(`SELECT COUNT(*) as c FROM (${sql})`).get(...params)?.c || 0;
    const totalPages = Math.max(1, Math.ceil(total / pageSize));
    const safePage = Math.min(Math.max(1, page), totalPages);
    const rows = db.prepare(`${sql} ORDER BY created_at DESC LIMIT ? OFFSET ?`).all(...params, pageSize, (safePage - 1) * pageSize);
    return {
      page: safePage, pageSize, total, totalPages,
      entries: rows.map(r => ({ ...r, metadata: JSON.parse(r.metadata || "{}") }))
    };
  },

  createEntry(baseId, body) {
    const title = String(body.title || "").trim();
    const content = String(body.content || "").trim();
    if (!title) throw new Error("title required");
    const id = body.id || genId("ent");
    db.prepare(
      "INSERT INTO knowledge_entries (id, base_id, title, content, category_id, metadata) VALUES (?, ?, ?, ?, ?, ?)"
    ).run(id, baseId, title, content, body.categoryId || null, JSON.stringify(body.metadata || {}));
    return { ok: true, id };
  },

  updateEntry(id, body) {
    const entry = db.prepare("SELECT * FROM knowledge_entries WHERE id = ?").get(id);
    if (!entry) throw new Error(`unknown entry: ${id}`);
    db.prepare(
      "UPDATE knowledge_entries SET title=?, content=?, category_id=?, enabled=?, metadata=?, updated_at=datetime('now') WHERE id=?"
    ).run(
      body.title || entry.title, body.content ?? entry.content,
      body.categoryId !== undefined ? body.categoryId : entry.category_id,
      body.enabled !== undefined ? (body.enabled ? 1 : 0) : entry.enabled,
      body.metadata ? JSON.stringify(body.metadata) : entry.metadata,
      id
    );
    return { ok: true };
  },

  deleteEntry(id) {
    db.prepare("DELETE FROM knowledge_entries WHERE id = ?").run(id);
    return { ok: true };
  },

  // ── CSV 导入 ──
  importCsv(baseId, csv) {
    const base = db.prepare("SELECT * FROM knowledge_bases WHERE id = ?").get(baseId);
    if (!base) throw new Error(`unknown base: ${baseId}`);

    const lines = csv.split(/\r?\n/).filter(l => l.trim() && !l.startsWith("名称"));
    const imported = [];
    const errors = [];
    const insert = db.prepare(
      "INSERT INTO knowledge_entries (id, base_id, title, content, metadata) VALUES (?, ?, ?, ?, ?)"
    );

    const tx = db.transaction(() => {
      for (const line of lines) {
        const cols = line.split(",").map(c => c.trim().replace(/^"|"$/g, ""));
        const name = cols[0];
        if (!name) { errors.push(`missing name: ${line.slice(0, 30)}`); continue; }
        const id = name.replace(/[^a-z0-9-]/gi, "-").toLowerCase().replace(/-+/g, "-").replace(/^-|-$/g, "");
        const content = [cols[5] || "", cols[6] || "", cols[7] || ""].join("\n");
        insert.run(id, baseId, name, content, JSON.stringify({
          price: cols[1] || "", sizes: (cols[2] || "").split("/").filter(Boolean),
          colors: (cols[3] || "").split("/").filter(Boolean), delivery: cols[4] || "",
          sellingPoints: (cols[6] || "").split(";").filter(Boolean),
          notes: (cols[7] || "").split(";").filter(Boolean)
        }));
        imported.push(name);
      }
    });
    tx();

    return { ok: true, imported: imported.length, errors };
  },

  // ── 分类 ──
  listCategories(baseId) {
    return db.prepare("SELECT * FROM knowledge_categories WHERE base_id = ? ORDER BY sort_order").all(baseId || undefined);
  },

  createCategory(body) {
    const name = String(body.name || "").trim();
    const baseId = String(body.baseId || "");
    if (!name || !baseId) throw new Error("name and baseId required");
    const id = genId("cat");
    db.prepare("INSERT INTO knowledge_categories (id, name, base_id, sort_order) VALUES (?, ?, ?, ?)").run(id, name, baseId, body.sortOrder || 0);
    return { ok: true, id };
  },

  deleteCategory(id) {
    db.prepare("DELETE FROM knowledge_categories WHERE id = ?").run(id);
    return { ok: true };
  }
};
```

---

### Task 5: 模型配置、消息、客户、防封、设置 API

**Files:**
- Create: `whatsapp-ai-poc/src/api/models.js`
- Create: `whatsapp-ai-poc/src/api/messages.js`
- Create: `whatsapp-ai-poc/src/api/customers.js`
- Create: `whatsapp-ai-poc/src/api/antiban.js`
- Create: `whatsapp-ai-poc/src/api/settings.js`

- [ ] **Step 1: models.js — 透传 OpenClaw 配置**

```js
import * as openclaw from "../openclaw-bridge.js";

export const modelHandlers = {
  get() {
    const config = openclaw.readOpenClawConfig();
    return {
      ok: true,
      providers: config.models?.providers || {},
      primary: config.agents?.defaults?.model?.primary || "",
      fallback: config.agents?.defaults?.model?.fallback || ""
    };
  },

  save(body) {
    const config = openclaw.readOpenClawConfig();
    config.models ||= { mode: "merge", providers: {} };
    if (body.providers) config.models.providers = body.providers;
    if (body.fallback !== undefined) {
      config.agents ||= { defaults: {} };
      config.agents.defaults.model ||= {};
      config.agents.defaults.model.fallback = body.fallback;
    }
    if (body.primary !== undefined) {
      config.agents ||= { defaults: {} };
      config.agents.defaults.model ||= {};
      config.agents.defaults.model.primary = body.primary;
    }
    openclaw.writeOpenClawConfig(config);
    openclaw.scheduleGatewayRestart();
    return { ok: true };
  }
};
```

- [ ] **Step 2: messages.js — 问答历史**

```js
import { db } from "../db.js";

export const messageHandlers = {
  list({ accountKey = "", query = "", page = 1, pageSize = 20 } = {}) {
    const conditions = [];
    const params = [];
    if (accountKey) { conditions.push("account_key = ?"); params.push(accountKey); }
    if (query) { conditions.push("text LIKE ?"); params.push(`%${query}%`); }
    const where = conditions.length ? `WHERE ${conditions.join(" AND ")}` : "";

    const total = db.prepare(`SELECT COUNT(*) as c FROM messages ${where}`).get(...params)?.c || 0;
    const totalPages = Math.max(1, Math.ceil(total / pageSize));
    const safePage = Math.min(Math.max(1, page), totalPages);
    const rows = db.prepare(
      `SELECT * FROM messages ${where} ORDER BY at DESC LIMIT ? OFFSET ?`
    ).all(...params, pageSize, (safePage - 1) * pageSize);

    // 按会话分组（同 customer + 邻近时间 -> 一组对话）
    const conversations = [];
    let current = null;
    for (const msg of rows.reverse()) {
      if (!current || current.customer !== msg.customer) {
        if (current) conversations.push(current);
        current = { customer: msg.customer, accountKey: msg.account_key, messages: [] };
      }
      current.messages.push(msg);
    }
    if (current) conversations.push(current);

    return { ok: true, page: safePage, pageSize, total, totalPages, conversations };
  },

  exportCsv({ accountKey = "", query = "" } = {}) {
    const conditions = [];
    const params = [];
    if (accountKey) { conditions.push("account_key = ?"); params.push(accountKey); }
    if (query) { conditions.push("text LIKE ?"); params.push(`%${query}%`); }
    const where = conditions.length ? `WHERE ${conditions.join(" AND ")}` : "";
    const rows = db.prepare(`SELECT * FROM messages ${where} ORDER BY at DESC LIMIT 10000`).all(...params);
    const header = "客户,账号,方向,内容,时间";
    const csvRows = [header];
    for (const r of rows) {
      csvRows.push(`"${r.customer}","${r.account_key}","${r.direction}","${(r.text||"").replace(/"/g,'""')}","${r.at}"`);
    }
    return "﻿" + csvRows.join("\n");
  }
};
```

- [ ] **Step 3: customers.js**

```js
import { db } from "../db.js";

export const customerHandlers = {
  list({ accountKey = "", page = 1, pageSize = 20 } = {}) {
    let where = "";
    const params = [];
    if (accountKey) { where = "WHERE account_key = ?"; params.push(accountKey); }

    const customers = db.prepare(`
      SELECT customer, account_key,
        COUNT(*) as conversation_count,
        MIN(at) as first_contact,
        MAX(at) as last_contact
      FROM messages
      ${where}
      GROUP BY customer, account_key
      ORDER BY last_contact DESC
    `).all(...params);

    // 获取最近消息预览
    const enriched = customers.map(c => {
      const lastMsg = db.prepare("SELECT text FROM messages WHERE customer = ? AND account_key = ? ORDER BY at DESC LIMIT 1").get(c.customer, c.account_key);
      return { ...c, lastMessagePreview: (lastMsg?.text || "").slice(0, 50) };
    });

    const total = enriched.length;
    const totalPages = Math.max(1, Math.ceil(total / pageSize));
    const safePage = Math.min(Math.max(1, page), totalPages);
    const start = (safePage - 1) * pageSize;

    return { ok: true, customers: enriched.slice(start, start + pageSize), page: safePage, total, totalPages };
  }
};
```

- [ ] **Step 4: antiban.js**

```js
import { db } from "../db.js";

export const antibanHandlers = {
  get() {
    const cfg = db.prepare("SELECT * FROM antiban_config WHERE id = 1").get();
    return {
      ok: true,
      config: {
        replyDelay: { min: cfg.reply_delay_min, max: cfg.reply_delay_max },
        rateLimit: { maxPerHour: cfg.rate_limit_hour, maxPerDay: cfg.rate_limit_day },
        warmup: { enabled: !!cfg.warmup_enabled, durationHours: cfg.warmup_hours },
        sessionKeepalive: { enabled: !!cfg.keepalive_enabled, intervalMinutes: cfg.keepalive_interval }
      }
    };
  },

  save(body) {
    const c = body.config || body;
    db.prepare(`UPDATE antiban_config SET
      reply_delay_min=?, reply_delay_max=?, rate_limit_hour=?, rate_limit_day=?,
      warmup_enabled=?, warmup_hours=?, keepalive_enabled=?, keepalive_interval=?,
      updated_at=datetime('now') WHERE id=1`).run(
      c.replyDelay?.min ?? 1000, c.replyDelay?.max ?? 5000,
      c.rateLimit?.maxPerHour ?? 30, c.rateLimit?.maxPerDay ?? 200,
      c.warmup?.enabled ? 1 : 0, c.warmup?.durationHours ?? 24,
      c.sessionKeepalive?.enabled ? 1 : 0, c.sessionKeepalive?.intervalMinutes ?? 5
    );
    return { ok: true };
  }
};
```

- [ ] **Step 5: settings.js**

```js
import { db } from "../db.js";

export const settingsHandlers = {
  get() {
    const s = db.prepare("SELECT * FROM settings WHERE id = 1").get();
    return {
      ok: true,
      settings: {
        workingHours: { start: s.work_start, end: s.work_end, timezone: s.timezone },
        autoReply: { outOfHours: s.out_of_hours_reply, greeting: s.greeting },
        globalDefaults: { dailyLimit: s.default_daily_limit }
      }
    };
  },

  save(body) {
    const s = body.settings || body;
    db.prepare(`UPDATE settings SET
      work_start=?, work_end=?, timezone=?,
      out_of_hours_reply=?, greeting=?, default_daily_limit=?,
      updated_at=datetime('now') WHERE id=1`).run(
      s.workingHours?.start || "09:00", s.workingHours?.end || "18:00", s.timezone || "Asia/Shanghai",
      s.autoReply?.outOfHours || "", s.autoReply?.greeting || "",
      s.globalDefaults?.dailyLimit ?? 30
    );
    return { ok: true };
  }
};
```

- [ ] **Step 6: 验证所有 API 模块**

```bash
node -e "
import('./src/db.js').then(({db}) => {
  // Test antiban
  const {antibanHandlers} = await import('./src/api/antiban.js');
  console.log('Antiban:', antibanHandlers.get().config.replyDelay);
  
  // Test settings
  const {settingsHandlers} = await import('./src/api/settings.js');
  console.log('Settings:', settingsHandlers.get().settings.workingHours.start);
  
  db.close();
});
"
```

---

### Task 6: 重构 server.js — 接入新 API 模块

**Files:**
- Modify: `whatsapp-ai-poc/src/server.js`

**Interfaces:**
- Consumes: all `api/*` modules
- Produces: same HTTP server on port 8790, new route structure

- [ ] **Step 1: 重写 server.js 路由**

关键变更：
1. 顶部 import 新 API 模块
2. 路由从 `/admin-api/*` 改为 `/api/*`
3. 删除 channels 路由
4. 保持 `/login`、`/admin-api/auth/*` 兼容

在 server.js 顶部添加 import：

```js
import { accountHandlers } from "./api/accounts.js";
import { knowledgeHandlers } from "./api/knowledge.js";
import { modelHandlers } from "./api/models.js";
import { messageHandlers } from "./api/messages.js";
import { customerHandlers } from "./api/customers.js";
import { antibanHandlers } from "./api/antiban.js";
import { settingsHandlers } from "./api/settings.js";
```

在 route handler 的 if-else 链中替换路由。例如 accounts:

```js
// GET /api/accounts
if (request.method === "GET" && url.pathname === "/api/accounts") {
  writeJson(response, 200, accountHandlers.list());
  return;
}

// POST /api/accounts
if (request.method === "POST" && url.pathname === "/api/accounts") {
  const body = await readJsonBody(request);
  try {
    const result = accountHandlers.create(body);
    writeJson(response, 200, result);
  } catch (e) {
    writeJson(response, 400, { error: e.message });
  }
  return;
}

// PUT /api/accounts/:key
const accountMatch = url.pathname.match(/^\/api\/accounts\/([a-z0-9_-]+)$/);
if (request.method === "PUT" && accountMatch) {
  const body = await readJsonBody(request);
  try {
    writeJson(response, 200, accountHandlers.update(accountMatch[1], body));
  } catch (e) { writeJson(response, 400, { error: e.message }); }
  return;
}

// DELETE /api/accounts/:key
if (request.method === "DELETE" && accountMatch) {
  try {
    const result = await accountHandlers.delete(accountMatch[1]);
    writeJson(response, 200, result);
  } catch (e) { writeJson(response, 400, { error: e.message }); }
  return;
}

// POST /api/accounts/:key/qr
const qrMatch = url.pathname.match(/^\/api\/accounts\/([a-z0-9_-]+)\/qr$/);
if (request.method === "POST" && qrMatch) {
  try {
    const result = await accountHandlers.startQr(qrMatch[1]);
    writeJson(response, 200, result);
  } catch (e) { writeJson(response, 400, { error: e.message }); }
  return;
}

// GET /api/accounts/:key/qr-status
const qrStatusMatch = url.pathname.match(/^\/api\/accounts\/([a-z0-9_-]+)\/qr-status$/);
if (request.method === "GET" && qrStatusMatch) {
  try {
    writeJson(response, 200, await accountHandlers.qrStatus(qrStatusMatch[1]));
  } catch (e) { writeJson(response, 400, { error: e.message }); }
  return;
}

// PUT /api/accounts/:key/toggle
const toggleMatch = url.pathname.match(/^\/api\/accounts\/([a-z0-9_-]+)\/toggle$/);
if (request.method === "PUT" && toggleMatch) {
  const body = await readJsonBody(request);
  writeJson(response, 200, accountHandlers.toggle(toggleMatch[1], body.enabled));
  return;
}
```

类似地注册 knowledge、models、messages、customers、antiban、settings 路由。完整的 server.js 重写代码较长，但模式一致：每个路由 → 调用对应 handler → 写入 JSON 响应。

- [ ] **Step 2: 启动服务器验证路由**

```bash
cd whatsapp-ai-poc
node src/server.js &
sleep 2
curl -s http://localhost:8790/api/accounts | head -c 200
curl -s http://localhost:8790/api/knowledge/roles | head -c 200
curl -s http://localhost:8790/api/antiban | head -c 200
curl -s http://localhost:8790/api/settings | head -c 200
kill %1
```

Expected: 四个路由均返回 `{"ok":true,...}` JSON。

---

### Task 7: 更新 build-agent-prompt.js — 从 SQLite 读取

**Files:**
- Modify: `whatsapp-ai-poc/src/build-agent-prompt.js`

- [ ] **Step 1: 修改数据源**

将 `generateAgentPrompt()` 函数改为从 SQLite 读取数据：

```js
import { db } from "./db.js";

export function generateAgentPrompt() {
  const accounts = db.prepare("SELECT * FROM accounts").all();
  const roles = db.prepare("SELECT * FROM knowledge_roles").all();

  const roleMap = Object.fromEntries(roles.map(r => [r.id, r]));

  // 账号→角色映射
  const accountEntries = accounts.map(acct => {
    const roleIds = db.prepare("SELECT role_id FROM account_roles WHERE account_id = ?").all(acct.id).map(r => r.role_id);
    const roleNames = roleIds.map(id => roleMap[id]?.name || id).join(", ");
    return `| \`${acct.id}\` | ${acct.label} | ${roleNames || "未分配"} | \`${roleIds.join(", ") || "无"}\` |`;
  });

  // 角色知识 → 从 knowledge_entries 渲染
  const roleSections = roles.map(role => {
    const bases = db.prepare(
      "SELECT kb.* FROM knowledge_bases kb JOIN knowledge_base_roles kbr ON kb.id = kbr.base_id WHERE kbr.role_id = ?"
    ).all(role.id);

    const productLines = [];
    for (const base of bases) {
      const entries = db.prepare("SELECT * FROM knowledge_entries WHERE base_id = ? AND enabled = 1").all(base.id);
      for (const e of entries) {
        const meta = JSON.parse(e.metadata || "{}");
        const parts = [`- **${e.title}**`];
        if (meta.price) parts.push(`  价格: ${meta.price}`);
        if (meta.sizes?.length) parts.push(`  尺码: ${meta.sizes.join(", ")}`);
        if (meta.colors?.length) parts.push(`  颜色: ${meta.colors.join(", ")}`);
        if (meta.delivery) parts.push(`  发货: ${meta.delivery}`);
        if (e.content) parts.push(`  描述: ${e.content.split("\n")[0]}`);
        if (meta.sellingPoints?.length) parts.push(`  卖点: ${meta.sellingPoints.join("; ")}`);
        if (meta.notes?.length) parts.push(`  备注: ${meta.notes.join("; ")}`);
        productLines.push(parts.join("\n"));
      }
    }

    return [
      `### ${role.name} (\`${role.id}\`)`,
      `范围: ${role.description}`,
      `关键词: ${JSON.parse(role.keywords || "[]").join(", ")}`,
      "",
      "**产品:**",
      ...productLines,
      role.unknown_reply ? `\n**兜底回复:** ${role.unknown_reply}` : ""
    ].join("\n");
  });

  const prompt = `# WhatsApp 智能客服 — 主 Agent\n\n...（保持原有 prompt 框架不变）\n\n---\n\n${roleSections.join("\n---\n\n")}`;
  
  const outputPath = path.join(rootDir, "AGENTS.md");
  fs.writeFileSync(outputPath, prompt, "utf8");
  return { roles: roles.length, accounts: accountEntries.length };
}
```

- [ ] **Step 2: 验证生成**

```bash
node src/build-agent-prompt.js
```

Expected: 控制台输出生成的 prompt 统计信息，`AGENTS.md` 文件更新。

---

### Task 8: 前端重构 — 使用 frontend-design 生成设计系统

**Files:**
- Modify: `whatsapp-ai-poc/src/admin-page.js` (完整重写)

**注意：** 这是最大的一步，需要调用 `frontend-design` skill 来生成设计系统 CSS 和组件。

- [ ] **Step 1: 调用 frontend-design skill 生成设计系统**

调用 `Skill({skill: "frontend-design"})` 生成：

1. CSS 变量系统（颜色、间距、圆角、阴影、字体）
2. 基础组件样式（按钮、输入框、卡片、弹窗、状态标签）
3. 布局系统（侧边栏 + 主区域的 grid 布局）

设计约束：
- 风格参考 WhatsApp 绿色系，但不完全照搬
- 侧边栏深色，主区域浅色背景
- 中文字体优先：system-ui, "Microsoft YaHei"
- 移动端响应式

前端页面的 HTML 结构（作为 frontend-design 的输入）：

```html
<div class="app">
  <aside class="sidebar"><!-- 导航 --></aside>
  <main class="main"><!-- 各 view --></main>
</div>
```

- [ ] **Step 2: 重写导航（修复滚动）**

侧边栏 CSS 必须包含:
```css
.sidebar {
  position: sticky;
  top: 0;
  height: 100vh;
  overflow-y: auto;  /* 修复无法滚动 */
  display: flex;
  flex-direction: column;
}
```

导航分组：
```
📊 总览
─────────────────
👤 客服账号
🧠 知识库
🔧 模型配置
─────────────────
💬 问答历史
👥 客户列表
📡 实时监控
─────────────────
🛡️ 防封策略
⚙️ 设置
─────────────────
📋 操作日志
🔔 告警通知
🧪 快捷测试
```

- [ ] **Step 3: 实现客服账号页面**

包含：
- 新增表单区（名称、上限、角色勾选、保存按钮）
- 账号卡片列表（状态标签、用量条、扫码/编辑/启停/删除按钮）
- QR 弹窗（模态框，显示 QR 图片 + 状态文字）

关键 JS 代码框架（基于 API 新路由）：

```js
// 新增客服（只保存）
async function createAccount() {
  const label = document.getElementById('newLabel').value.trim();
  const dailyLimit = Number(document.getElementById('newLimit').value || 30);
  const roles = [...document.querySelectorAll('[data-role]:checked')].map(el => el.value);
  const result = await postJson('/api/accounts', { label, dailyLimit, roles });
  await loadAccounts();
  showToast('客服已创建，请扫码关联 WhatsApp');
}

// 扫码登录（已有账号）
async function startQr(accountKey) {
  const result = await postJson(`/api/accounts/${accountKey}/qr`, {});
  showQrModal(result.job);
  pollQrStatus(accountKey);
}

// 轮询扫码状态
async function pollQrStatus(accountKey) {
  const poll = setInterval(async () => {
    const result = await fetchJson(`/api/accounts/${accountKey}/qr-status`);
    updateQrModal(result.job);
    if (result.job.status === 'linked') {
      clearInterval(poll);
      await loadAccounts();
      showToast('扫码成功，客服已关联');
    }
  }, 2500);
}
```

- [ ] **Step 4: 实现知识库页面**

三个 Tab：
1. 角色管理 — 列表 + 新增/编辑弹窗
2. 知识库管理 — 列表（名称、类型、关联角色、条目数）+ 新增/编辑弹窗
3. 内容编辑 — 选中知识库后，条目表格 + CSV 导入 + 分类标签

- [ ] **Step 5: 实现问答历史（聊天气泡）**

```js
function renderConversations(conversations) {
  return conversations.map(conv => `
    <div class="chat-conversation">
      <div class="chat-divider"><span>${escapeHtml(conv.customer)}</span></div>
      ${conv.messages.map(msg => {
        const isInbound = msg.direction === 'inbound';
        return `
          <div class="chat-bubble ${isInbound ? 'inbound' : 'outbound'}">
            <div class="chat-text">${escapeHtml(msg.text)}</div>
            <div class="chat-time">${formatTime(msg.at)}</div>
          </div>`;
      }).join('')}
    </div>
  `).join('');
}
```

气泡 CSS:
```css
.chat-bubble.inbound {
  align-self: flex-start;
  background: var(--surface);
  border: 1px solid var(--line);
  border-radius: 12px 12px 12px 4px;
}
.chat-bubble.outbound {
  align-self: flex-end;
  background: var(--green-soft);
  border-radius: 12px 12px 4px 12px;
}
```

- [ ] **Step 6: 实现模型配置页面**

直接编辑 openclaw.json 的 providers：

```js
async function saveModels() {
  const providers = buildProvidersFromForm();
  const fallback = document.getElementById('fallbackModel').value;
  const primary = document.getElementById('primaryModel').value;
  await postJson('/api/models', { providers, fallback, primary });
  showToast('模型配置已保存，OpenClaw 将在重启后生效');
}
```

- [ ] **Step 7: 实现防封策略、设置、客户列表**

这些页面相对简单，直接表单 + 保存：

```js
// 防封
async function saveAntiBan() {
  await postJson('/api/antiban', { config: readForm() });
}

// 设置
async function saveSettings() {
  await postJson('/api/settings', { settings: readForm() });
}

// 客户列表
async function loadCustomers() {
  const result = await fetchJson(`/api/customers?page=${page}`);
  renderCustomerTable(result.customers);
}
```

- [ ] **Step 8: 实现总览页**

```js
async function loadOverview() {
  const accounts = await fetchJson('/api/accounts');
  const online = accounts.accounts.filter(a => a.status === 'online').length;
  const total = accounts.accounts.length;
  
  document.getElementById('metrics').innerHTML = [
    { label: '在线账号', value: `${online}/${total}` },
    { label: '今日消息', value: sumUsed(accounts.accounts) },
    { label: '知识角色', value: (await fetchJson('/api/knowledge/roles')).length }
  ].map(m => `<div class="metric-card">...</div>`).join('');
}
```

---

### Task 9: 端到端验证

**Files:** 无（测试验证）

- [ ] **Step 1: 启动服务器**

```bash
cd whatsapp-ai-poc
node src/server.js &
sleep 2
```

- [ ] **Step 2: 验证所有 API 端点**

```bash
# 账号
curl -s http://localhost:8790/api/accounts | python3 -m json.tool | head -5
curl -s -X POST http://localhost:8790/api/accounts -H 'content-type: application/json' -d '{"label":"测试","dailyLimit":10,"roles":[]}' | python3 -m json.tool

# 知识库
curl -s http://localhost:8790/api/knowledge/roles | python3 -m json.tool | head -5
curl -s http://localhost:8790/api/knowledge/bases | python3 -m json.tool | head -5

# 模型
curl -s http://localhost:8790/api/models | python3 -m json.tool | head -5

# 消息
curl -s http://localhost:8790/api/messages | python3 -m json.tool | head -5

# 防封
curl -s http://localhost:8790/api/antiban | python3 -m json.tool

# 设置
curl -s http://localhost:8790/api/settings | python3 -m json.tool
```

- [ ] **Step 3: 验证前端页面加载**

```bash
curl -s http://localhost:8790/ | head -20
```

Expected: 返回包含新 CSS 的完整 HTML。

- [ ] **Step 4: 清理**

```bash
kill %1
```

---

### Task 10: 清理 + 文档

**Files:**
- Modify: `whatsapp-ai-poc/README.md`

- [ ] **Step 1: 更新 README 中的 API 路径**

将所有 `/admin-api/*` 引用更新为 `/api/*`。

- [ ] **Step 2: 移除废弃的 config 文件（可选）**

`config/accounts.json` 和 `config/knowledge.json` 保留作为初始迁移源，但不再作为运行时数据源。`data/*.json` 文件保留用于日志/备份。

---

## 实施说明

1. **Task 2 (SQLite)** 是基础，所有后续 task 依赖它
2. **Task 3-5 (API 模块)** 可以并行开发
3. **Task 6 (server.js)** 依赖 Task 3-5 全部完成
4. **Task 7 (build-agent-prompt)** 依赖 Task 2，可与 Task 3-5 并行
5. **Task 8 (前端重写)** 依赖 Task 6 完成（需要 API 路由确定）
6. **Task 8** 调用 `frontend-design` skill 生成 CSS 设计系统，是最大单步
7. **Task 9-10** 验证和收尾
