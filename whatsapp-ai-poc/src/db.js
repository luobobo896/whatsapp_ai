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

export function migrateFromJson() {
  // 检查是否需要迁移（accounts 表为空）
  const count = db.prepare("SELECT COUNT(*) as c FROM accounts").get();
  if (count.c > 0) return { migrated: false, reason: "already has data" };

  const accountsPath = path.join(rootDir, "config", "accounts.json");
  const knowledgePath = path.join(rootDir, "config", "knowledge.json");

  let accountCount = 0;
  let roleCount = 0;
  let productCount = 0;

  // 迁移知识库（必须在迁移账号之前，因为 account_roles 引用 knowledge_roles）
  if (fs.existsSync(knowledgePath)) {
    let knowledge;
    try {
      knowledge = JSON.parse(fs.readFileSync(knowledgePath, "utf8"));
    } catch (err) {
      console.error(`Failed to parse ${knowledgePath}: ${err.message}`);
      knowledge = null;
    }
    if (!knowledge) { /* skip knowledge migration */ } else {
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
  }

  // 迁移账号
  if (fs.existsSync(accountsPath)) {
    let accounts;
    try {
      accounts = JSON.parse(fs.readFileSync(accountsPath, "utf8"));
    } catch (err) {
      console.error(`Failed to parse ${accountsPath}: ${err.message}`);
      accounts = null;
    }
    if (!accounts) { /* skip accounts migration */ } else {
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
  }

  console.log(`Migration complete: ${accountCount} accounts, ${roleCount} roles, ${productCount} products`);
  return { migrated: true, accountCount, roleCount, productCount };
}

// 自动迁移（如果数据库为空且 JSON 文件存在）
migrateFromJson();
