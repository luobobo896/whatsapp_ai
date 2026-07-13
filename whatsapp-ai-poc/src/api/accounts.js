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
