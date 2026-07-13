import http from "node:http";
import fs from "node:fs";
import path from "node:path";
import { env, rootDir, openClawConfigPath } from "./config.js";
import { appendAlert, appendAuditLog, appendSandboxMessage, getDailyUsage, markAlertsRead, readAccountOverrides, readAlerts, readAuditLog, readMessageLogs, readOpenClawMessageLogs, readSandboxMessages, todayKey } from "./store.js";
import * as openclaw from "./openclaw-bridge.js";
import { generateAgentPrompt } from "./build-agent-prompt.js";
import { adminPage, loginPage } from "./admin-page.js";
import { readKnowledgeConfig } from "./knowledge.js";
import { accountHandlers } from "./api/accounts.js";
import { knowledgeHandlers } from "./api/knowledge.js";
import { modelHandlers } from "./api/models.js";
import { messageHandlers } from "./api/messages.js";
import { customerHandlers } from "./api/customers.js";
import { antibanHandlers } from "./api/antiban.js";
import { settingsHandlers } from "./api/settings.js";

const adminPageHtml = adminPage();
const ACCOUNTS_CONFIG_PATH = path.join(rootDir, "config", "accounts.json");
const CHANNEL_STATUS_REFRESH_MS = 15_000;

const AUTH_TOKEN = process.env.ADMIN_TOKEN || (() => {
  try { return JSON.parse(fs.readFileSync(openClawConfigPath || path.join(process.env.HOME, ".openclaw-whatsapp-poc", "openclaw.json"), "utf8")).gateway?.auth?.token || "admin"; }
  catch { return "admin"; }
})();

function parseCookies(request) {
  const cookie = request.headers.cookie || "";
  return Object.fromEntries(cookie.split(";").map(c => c.trim().split("=").map(decodeURIComponent)));
}

function checkAuth(request) {
  const url = new URL(request.url || "/", `http://${request.headers.host}`);
  if (url.pathname === "/login" || url.pathname === "/admin-api/auth/login" || url.pathname === "/admin-api/auth/logout" || url.pathname === "/health") return true;
  const cookies = parseCookies(request);
  return cookies.admin_token === AUTH_TOKEN;
}

const accountLoginJobs = new Map();

function clientIp(request) {
  return request.headers["x-forwarded-for"]?.split(",")[0]?.trim() || request.socket.remoteAddress || "127.0.0.1";
}

/**
 * @param {http.IncomingMessage} request
 * @returns {Promise<Record<string, unknown>>}
 */
async function readJsonBody(request) {
  const chunks = [];
  for await (const chunk of request) {
    chunks.push(chunk);
  }
  const raw = Buffer.concat(chunks).toString("utf8");
  return raw ? JSON.parse(raw) : {};
}

/**
 * @param {http.ServerResponse} response
 * @param {number} statusCode
 * @param {unknown} payload
 */
function writeJson(response, statusCode, payload) {
  response.writeHead(statusCode, { "content-type": "application/json; charset=utf-8" });
  response.end(JSON.stringify(payload, null, 2));
}

/**
 * @param {http.ServerResponse} response
 * @param {number} statusCode
 * @param {string} html
 */
function writeHtml(response, statusCode, html) {
  response.writeHead(statusCode, { "content-type": "text/html; charset=utf-8" });
  response.end(html);
}

function readAccountsConfig() {
  return JSON.parse(fs.readFileSync(ACCOUNTS_CONFIG_PATH, "utf8"));
}

/**
 * @returns {Record<string, Record<string, unknown>>}
 */
function buildAccountStatus() {
  const accounts = readAccountsConfig();
  const overrides = readAccountOverrides();
  const channelStatus = openclaw.readChannelStatus();
  const cs = openclaw.getChannelStatus();
  const openClawMessages = readOpenClawMessageLogs(1000);
  return Object.fromEntries(Object.entries(accounts).map(([accountKey, account]) => {
    const dailyLimit = Number(overrides[accountKey]?.dailyLimit ?? account.dailyLimit ?? 0);
    const live = channelStatus[accountKey] || {};
    const sessionUsed = openClawMessages.filter((message) => {
      if (message.direction !== "outbound" || message.agentId !== account.agentId) return false;
      const date = new Date(String(message.at));
      const local = new Date(date.getTime() - date.getTimezoneOffset() * 60_000);
      return local.toISOString().slice(0, 10) === todayKey();
    }).length;
    const used = Math.max(getDailyUsage(accountKey), sessionUsed);
    const enabled = overrides[accountKey]?.enabled !== false && live.disabled !== true;
    return [accountKey, {
      ...account,
      dailyLimit,
      enabled,
      usedToday: used,
      remainingToday: dailyLimit > 0 ? Math.max(dailyLimit - used, 0) : null,
      date: todayKey(),
      override: overrides[accountKey] || null,
      live,
      liveRefreshedAt: cs.refreshedAt,
      liveRefreshError: cs.error
    }];
  }));
}

function checkAlerts() {
  try {
    const status = buildAccountStatus();
    const existingAlerts = readAlerts();
    for (const [accountKey, account] of Object.entries(status)) {
      if (!account.live?.healthy && account.enabled && account.live?.linked) {
        const todayKeyStr = new Date().toISOString().slice(0,10);
        const alreadyAlerted = existingAlerts.some(a =>
          a.type === "disconnected" && a.accountKey === accountKey && String(a.at||"").startsWith(todayKeyStr)
        );
        if (!alreadyAlerted) {
          appendAlert({ type: "disconnected", level: "error", title: "账号断连", body: `${account.label||accountKey} 已断开连接`, accountKey });
        }
      }
      const limit = Number(account.dailyLimit||0);
      const used = Number(account.usedToday||0);
      if (limit > 0 && used >= limit && account.enabled) {
        const todayKeyStr = new Date().toISOString().slice(0,10);
        const alreadyAlerted = existingAlerts.some(a =>
          a.type === "limit-reached" && a.accountKey === accountKey && String(a.at||"").startsWith(todayKeyStr)
        );
        if (!alreadyAlerted) {
          appendAlert({ type: "limit-reached", level: "warn", title: "回复上限已达", body: `${account.label||accountKey} 今日回复已达上限 (${used}/${limit})`, accountKey });
        }
      }
    }
  } catch(e) { console.error("alert check failed", e); }
}

function readAllMessages(limit = 1000) {
  return [...readMessageLogs(limit), ...readOpenClawMessageLogs(limit)]
    .sort((left, right) => new Date(String(left.at)).getTime() - new Date(String(right.at)).getTime());
}

function messageAccountKey(message, accounts) {
  if (message.accountKey) {
    return String(message.accountKey);
  }
  const found = Object.entries(accounts).find(([, account]) => account.agentId === message.agentId);
  return found ? found[0] : "";
}

function messageCustomer(message) {
  const direct = message.customer || message.from || message.to;
  if (direct) {
    return String(direct);
  }
  const text = String(message.text || "");
  const phoneMatch = text.match(/客户\s+([+\d][\d\s-]{5,})\s*问/);
  if (phoneMatch) {
    return phoneMatch[1].trim();
  }
  return "";
}

function buildConversationRows({ accountKey, query, page, pageSize }) {
  const accounts = readAccountsConfig();
  const normalizedQuery = String(query || "").trim().toLowerCase();
  const rows = [];
  const pendingByScope = new Map();
  for (const message of readAllMessages(2000)) {
    const currentAccountKey = messageAccountKey(message, accounts);
    if (accountKey && currentAccountKey !== accountKey) {
      continue;
    }
    const scope = `${currentAccountKey || "unknown"}:${message.sessionId || message.customer || message.from || message.to || "default"}`;
    if (message.direction === "inbound") {
      const row = {
        id: `${scope}:${message.at}`,
        accountKey: currentAccountKey,
        agentId: message.agentId || accounts[currentAccountKey]?.agentId || "",
        customer: messageCustomer(message),
        inboundAt: message.at,
        inboundText: message.text || "",
        outboundAt: "",
        outboundText: "",
        status: message.status || ""
      };
      rows.push(row);
      pendingByScope.set(scope, row);
      continue;
    }
    if (message.direction === "outbound") {
      const pending = pendingByScope.get(scope);
      if (pending && !pending.outboundText) {
        pending.outboundAt = message.at;
        pending.outboundText = message.text || "";
        pending.status = message.status || pending.status;
      } else {
        rows.push({
          id: `${scope}:${message.at}`,
          accountKey: currentAccountKey,
          agentId: message.agentId || accounts[currentAccountKey]?.agentId || "",
          customer: messageCustomer(message),
          inboundAt: "",
          inboundText: "",
          outboundAt: message.at,
          outboundText: message.text || "",
          status: message.status || ""
        });
      }
    }
  }
  const filtered = normalizedQuery ? rows.filter((row) => {
    return [row.accountKey, row.agentId, row.customer, row.inboundText, row.outboundText]
      .join(" ")
      .toLowerCase()
      .includes(normalizedQuery);
  }) : rows;
  const total = filtered.length;
  const safePageSize = Math.max(1, Math.min(Number(pageSize) || 20, 100));
  const totalPages = Math.max(1, Math.ceil(total / safePageSize));
  const safePage = Math.min(Math.max(1, Number(page) || 1), totalPages);
  const start = (safePage - 1) * safePageSize;
  return {
    page: safePage,
    pageSize: safePageSize,
    total,
    totalPages,
    rows: filtered.slice().reverse().slice(start, start + safePageSize)
  };
}

const server = http.createServer(async (request, response) => {
  try {
    const url = new URL(request.url || "/", `http://${request.headers.host}`);

    // ── Public routes (no auth required) ──

    if (request.method === "GET" && url.pathname === "/health") {
      writeJson(response, 200, { ok: true, accounts: readAccountsConfig() });
      return;
    }

    if (request.method === "GET" && url.pathname === "/login") {
      writeHtml(response, 200, loginPage());
      return;
    }

    if (request.method === "POST" && url.pathname === "/admin-api/auth/login") {
      const body = await readJsonBody(request);
      if (body.token === AUTH_TOKEN) {
        response.writeHead(200, {
          "content-type": "application/json",
          "set-cookie": `admin_token=${AUTH_TOKEN}; Path=/; HttpOnly; SameSite=Strict; Max-Age=86400`
        });
        response.end(JSON.stringify({ ok: true }));
        return;
      }
      writeJson(response, 401, { error: "invalid token" });
      return;
    }

    if (request.method === "POST" && url.pathname === "/admin-api/auth/logout") {
      response.writeHead(200, {
        "content-type": "application/json",
        "set-cookie": "admin_token=; Path=/; HttpOnly; SameSite=Strict; Max-Age=0"
      });
      response.end(JSON.stringify({ ok: true }));
      return;
    }

    // ── Auth check ──

    if (!checkAuth(request)) {
      if (request.method === "GET" && !url.pathname.startsWith("/admin-api")) {
        response.writeHead(302, { location: "/login" });
        response.end();
        return;
      }
      writeJson(response, 401, { error: "unauthorized" });
      return;
    }

    // ── HTML page ──

    if (request.method === "GET" && url.pathname === "/") {
      writeHtml(response, 200, adminPageHtml);
      return;
    }

    // ═══════════════════════════════════════════════════════════════
    // NEW API ROUTES (/api/*) — using SQLite-backed modules
    // ═══════════════════════════════════════════════════════════════

    // ── Accounts ──

    if (request.method === "GET" && url.pathname === "/api/accounts") {
      try {
        writeJson(response, 200, accountHandlers.list());
      } catch (e) { writeJson(response, 500, { error: e.message }); }
      return;
    }

    if (request.method === "POST" && url.pathname === "/api/accounts") {
      const body = await readJsonBody(request);
      try {
        writeJson(response, 200, accountHandlers.create(body));
      } catch (e) { writeJson(response, 400, { error: e.message }); }
      return;
    }

    const accountMatch = url.pathname.match(/^\/api\/accounts\/([a-z0-9_-]+)$/);
    if (request.method === "PUT" && accountMatch) {
      const body = await readJsonBody(request);
      try {
        writeJson(response, 200, accountHandlers.update(accountMatch[1], body));
      } catch (e) { writeJson(response, 400, { error: e.message }); }
      return;
    }
    if (request.method === "DELETE" && accountMatch) {
      try {
        writeJson(response, 200, await accountHandlers.delete(accountMatch[1]));
      } catch (e) { writeJson(response, 400, { error: e.message }); }
      return;
    }

    const qrMatch = url.pathname.match(/^\/api\/accounts\/([a-z0-9_-]+)\/qr$/);
    if (request.method === "POST" && qrMatch) {
      try {
        writeJson(response, 200, await accountHandlers.startQr(qrMatch[1]));
      } catch (e) { writeJson(response, 400, { error: e.message }); }
      return;
    }

    const qrStatusMatch = url.pathname.match(/^\/api\/accounts\/([a-z0-9_-]+)\/qr-status$/);
    if (request.method === "GET" && qrStatusMatch) {
      try {
        writeJson(response, 200, await accountHandlers.qrStatus(qrStatusMatch[1]));
      } catch (e) { writeJson(response, 400, { error: e.message }); }
      return;
    }

    const toggleMatch = url.pathname.match(/^\/api\/accounts\/([a-z0-9_-]+)\/toggle$/);
    if (request.method === "PUT" && toggleMatch) {
      const body = await readJsonBody(request);
      try {
        writeJson(response, 200, accountHandlers.toggle(toggleMatch[1], body.enabled));
      } catch (e) { writeJson(response, 400, { error: e.message }); }
      return;
    }

    // ── Knowledge: Roles ──

    if (request.method === "GET" && url.pathname === "/api/knowledge/roles") {
      try {
        writeJson(response, 200, { ok: true, roles: knowledgeHandlers.listRoles() });
      } catch (e) { writeJson(response, 500, { error: e.message }); }
      return;
    }

    if (request.method === "POST" && url.pathname === "/api/knowledge/roles") {
      const body = await readJsonBody(request);
      try {
        writeJson(response, 200, knowledgeHandlers.createRole(body));
      } catch (e) { writeJson(response, 400, { error: e.message }); }
      return;
    }

    const roleMatch = url.pathname.match(/^\/api\/knowledge\/roles\/([a-z0-9_-]+)$/);
    if (request.method === "PUT" && roleMatch) {
      const body = await readJsonBody(request);
      try {
        writeJson(response, 200, knowledgeHandlers.updateRole(roleMatch[1], body));
      } catch (e) { writeJson(response, 400, { error: e.message }); }
      return;
    }
    if (request.method === "DELETE" && roleMatch) {
      try {
        writeJson(response, 200, knowledgeHandlers.deleteRole(roleMatch[1]));
      } catch (e) { writeJson(response, 400, { error: e.message }); }
      return;
    }

    // ── Knowledge: Bases ──

    if (request.method === "GET" && url.pathname === "/api/knowledge/bases") {
      try {
        writeJson(response, 200, { ok: true, bases: knowledgeHandlers.listBases() });
      } catch (e) { writeJson(response, 500, { error: e.message }); }
      return;
    }

    if (request.method === "POST" && url.pathname === "/api/knowledge/bases") {
      const body = await readJsonBody(request);
      try {
        writeJson(response, 200, knowledgeHandlers.createBase(body));
      } catch (e) { writeJson(response, 400, { error: e.message }); }
      return;
    }

    const baseMatch = url.pathname.match(/^\/api\/knowledge\/bases\/([a-z0-9_-]+)$/);
    if (request.method === "PUT" && baseMatch) {
      const body = await readJsonBody(request);
      try {
        writeJson(response, 200, knowledgeHandlers.updateBase(baseMatch[1], body));
      } catch (e) { writeJson(response, 400, { error: e.message }); }
      return;
    }
    if (request.method === "DELETE" && baseMatch) {
      try {
        writeJson(response, 200, knowledgeHandlers.deleteBase(baseMatch[1]));
      } catch (e) { writeJson(response, 400, { error: e.message }); }
      return;
    }

    // ── Knowledge: Entries under a base ──

    const baseEntriesMatch = url.pathname.match(/^\/api\/knowledge\/bases\/([a-z0-9_-]+)\/entries$/);
    if (request.method === "GET" && baseEntriesMatch) {
      try {
        const page = Math.max(1, Number(url.searchParams.get("page") || 1));
        const pageSize = Math.min(100, Math.max(1, Number(url.searchParams.get("pageSize") || 20)));
        const categoryId = url.searchParams.get("categoryId") || "";
        writeJson(response, 200, knowledgeHandlers.listEntries(baseEntriesMatch[1], { page, pageSize, categoryId }));
      } catch (e) { writeJson(response, 500, { error: e.message }); }
      return;
    }
    if (request.method === "POST" && baseEntriesMatch) {
      const body = await readJsonBody(request);
      try {
        writeJson(response, 200, knowledgeHandlers.createEntry(baseEntriesMatch[1], body));
      } catch (e) { writeJson(response, 400, { error: e.message }); }
      return;
    }

    const baseImportMatch = url.pathname.match(/^\/api\/knowledge\/bases\/([a-z0-9_-]+)\/import-csv$/);
    if (request.method === "POST" && baseImportMatch) {
      const body = await readJsonBody(request);
      try {
        writeJson(response, 200, knowledgeHandlers.importCsv(baseImportMatch[1], body.csv || ""));
      } catch (e) { writeJson(response, 400, { error: e.message }); }
      return;
    }

    // ── Knowledge: Individual entries ──

    const entryMatch = url.pathname.match(/^\/api\/knowledge\/entries\/([a-z0-9_-]+)$/);
    if (request.method === "PUT" && entryMatch) {
      const body = await readJsonBody(request);
      try {
        writeJson(response, 200, knowledgeHandlers.updateEntry(entryMatch[1], body));
      } catch (e) { writeJson(response, 400, { error: e.message }); }
      return;
    }
    if (request.method === "DELETE" && entryMatch) {
      try {
        writeJson(response, 200, knowledgeHandlers.deleteEntry(entryMatch[1]));
      } catch (e) { writeJson(response, 400, { error: e.message }); }
      return;
    }

    // ── Knowledge: Categories ──

    if (request.method === "GET" && url.pathname === "/api/knowledge/categories") {
      try {
        writeJson(response, 200, { ok: true, categories: knowledgeHandlers.listCategories(url.searchParams.get("baseId") || undefined) });
      } catch (e) { writeJson(response, 500, { error: e.message }); }
      return;
    }

    if (request.method === "POST" && url.pathname === "/api/knowledge/categories") {
      const body = await readJsonBody(request);
      try {
        writeJson(response, 200, knowledgeHandlers.createCategory(body));
      } catch (e) { writeJson(response, 400, { error: e.message }); }
      return;
    }

    const categoryMatch = url.pathname.match(/^\/api\/knowledge\/categories\/([a-z0-9_-]+)$/);
    if (request.method === "DELETE" && categoryMatch) {
      try {
        writeJson(response, 200, knowledgeHandlers.deleteCategory(categoryMatch[1]));
      } catch (e) { writeJson(response, 400, { error: e.message }); }
      return;
    }

    // ── Models ──

    if (request.method === "GET" && url.pathname === "/api/models") {
      try {
        writeJson(response, 200, modelHandlers.get());
      } catch (e) { writeJson(response, 500, { error: e.message }); }
      return;
    }

    if (request.method === "POST" && url.pathname === "/api/models") {
      const body = await readJsonBody(request);
      try {
        writeJson(response, 200, modelHandlers.save(body));
      } catch (e) { writeJson(response, 400, { error: e.message }); }
      return;
    }

    // ── Messages ──

    if (request.method === "GET" && url.pathname === "/api/messages/export") {
      try {
        const csv = messageHandlers.exportCsv({
          accountKey: url.searchParams.get("accountKey") || "",
          query: url.searchParams.get("query") || ""
        });
        response.writeHead(200, {
          "content-type": "text/csv; charset=utf-8",
          "content-disposition": "attachment; filename=conversations.csv"
        });
        response.end(csv);
      } catch (e) { writeJson(response, 500, { error: e.message }); }
      return;
    }

    if (request.method === "GET" && url.pathname === "/api/messages") {
      try {
        writeJson(response, 200, messageHandlers.list({
          accountKey: url.searchParams.get("accountKey") || "",
          query: url.searchParams.get("query") || "",
          page: Math.max(1, Number(url.searchParams.get("page") || 1)),
          pageSize: Math.min(100, Math.max(1, Number(url.searchParams.get("pageSize") || 20)))
        }));
      } catch (e) { writeJson(response, 500, { error: e.message }); }
      return;
    }

    // ── Customers ──

    if (request.method === "GET" && url.pathname === "/api/customers") {
      try {
        writeJson(response, 200, customerHandlers.list({
          accountKey: url.searchParams.get("accountKey") || "",
          page: Math.max(1, Number(url.searchParams.get("page") || 1)),
          pageSize: Math.min(100, Math.max(1, Number(url.searchParams.get("pageSize") || 20)))
        }));
      } catch (e) { writeJson(response, 500, { error: e.message }); }
      return;
    }

    // ── Anti-Ban ──

    if (request.method === "GET" && url.pathname === "/api/antiban") {
      try {
        writeJson(response, 200, antibanHandlers.get());
      } catch (e) { writeJson(response, 500, { error: e.message }); }
      return;
    }

    if (request.method === "POST" && url.pathname === "/api/antiban") {
      const body = await readJsonBody(request);
      try {
        writeJson(response, 200, antibanHandlers.save(body));
      } catch (e) { writeJson(response, 400, { error: e.message }); }
      return;
    }

    // ── Settings ──

    if (request.method === "GET" && url.pathname === "/api/settings") {
      try {
        writeJson(response, 200, settingsHandlers.get());
      } catch (e) { writeJson(response, 500, { error: e.message }); }
      return;
    }

    if (request.method === "POST" && url.pathname === "/api/settings") {
      const body = await readJsonBody(request);
      try {
        writeJson(response, 200, settingsHandlers.save(body));
      } catch (e) { writeJson(response, 400, { error: e.message }); }
      return;
    }

    // ═══════════════════════════════════════════════════════════════
    // LEGACY ROUTES (/admin-api/*) — file-based, not yet migrated
    // ═══════════════════════════════════════════════════════════════

    if (request.method === "POST" && url.pathname === "/admin-api/accounts/clear-all-sessions") {
      const accounts = readAccountsConfig();
      const accountKeys = Object.keys(accounts);
      if (accountKeys.length === 0) {
        writeJson(response, 200, { ok: true, cleared: 0, total: 0 });
        return;
      }

      // 1. Logout each account via OpenClaw CLI (parallel)
      const results = await Promise.allSettled(
        accountKeys.map((k) => openclaw.logoutAccount(k).catch(() => openclaw.logoutAccountViaCli(k)))
      );
      const cleared = results.filter((r) => r.status === "fulfilled").length;

      // 2. Cancel all active login jobs
      accountLoginJobs.clear();

      // 3. Mark restart needed (user triggers manually)
      openclaw.markRestartNeeded();

      appendAuditLog({ action: "account.clear-all-sessions", detail: `Cleared ${cleared} of ${accountKeys.length} sessions via OpenClaw CLI`, ip: clientIp(request) });
      writeJson(response, 200, { ok: true, cleared, total: accountKeys.length });
      return;
    }

    if (request.method === "POST" && url.pathname === "/admin-api/export/conversations") {
      const body = await readJsonBody(request);
      const accountKey = String(body.accountKey || "");
      const query = String(body.query || "");
      const rows = buildConversationRows({ accountKey, query, page: 1, pageSize: 10000 }).rows;
      const headers = ["客户", "账号", "客户提问", "客服回复", "提问时间", "回复时间"];
      const csvRows = [headers.map(h => `"${h}"`).join(",")];
      for (const row of rows) {
        csvRows.push([
          `"${String(row.customer||"").replace(/"/g,'""')}"`,
          `"${String(row.accountKey||"").replace(/"/g,'""')}"`,
          `"${String(row.inboundText||"").replace(/"/g,'""')}"`,
          `"${String(row.outboundText||"").replace(/"/g,'""')}"`,
          `"${String(row.inboundAt||"")}"`,
          `"${String(row.outboundAt||"")}"`
        ].join(","));
      }
      const csv = "﻿" + csvRows.join("\n");
      response.writeHead(200, {
        "content-type": "text/csv; charset=utf-8",
        "content-disposition": "attachment; filename=conversations.csv"
      });
      response.end(csv);
      return;
    }

    if (request.method === "GET" && url.pathname === "/admin-api/stats/overview") {
      const allMessages = readAllMessages(10000);
      const now = new Date();
      const volumeTrend = [];
      for (let i = 6; i >= 0; i--) {
        const d = new Date(now);
        d.setDate(d.getDate() - i);
        const dateStr = d.toISOString().slice(0, 10);
        const count = allMessages.filter(m => String(m.at||"").startsWith(dateStr)).length;
        volumeTrend.push({ date: dateStr.slice(5), count });
      }
      const knowledge = readKnowledgeConfig();
      const roleNames = Object.fromEntries(knowledge.roles.map(r => [r.id, r.name]));
      const productCounts = {};
      for (const msg of allMessages) {
        if (msg.direction !== "inbound") continue;
        const text = String(msg.text||"").toLowerCase();
        for (const role of knowledge.roles) {
          for (const kw of (role.keywords||[])) {
            if (text.includes(kw.toLowerCase())) {
              productCounts[role.id] = (productCounts[role.id]||0) + 1;
              break;
            }
          }
        }
      }
      const topProducts = Object.entries(productCounts).sort((a,b) => b[1]-a[1]).slice(0,5)
        .map(([id,count]) => ({ name: roleNames[id]||id, count }));
      const inboundMsgs = allMessages.filter(m => m.direction === "inbound");
      const responseMetrics = {
        totalConversations: inboundMsgs.length,
        avgReplyTimeSec: 0,
        handoffCount: allMessages.filter(m => String(m.text||"").includes("转人工")).length
      };
      const alerts = readAlerts();
      const unreadAlerts = alerts.filter(a => !a.read).length;
      const status = buildAccountStatus();
      const accountsList = Object.values(status);
      const onlineAccounts = accountsList.filter(a => a.live?.healthy).length;
      const errorRate = accountsList.length > 0 ? Math.round(accountsList.filter(a => !a.live?.healthy && a.enabled).length / accountsList.length * 100) : 0;
      writeJson(response, 200, { ok: true, volumeTrend, topProducts, responseMetrics, accountHealth: { online: onlineAccounts, total: accountsList.length, errorRate }, activeAlerts: unreadAlerts });
      return;
    }

    if (request.method === "GET" && url.pathname === "/admin-api/messages/live") {
      const since = String(url.searchParams.get("since") || "");
      const accountKey = String(url.searchParams.get("accountKey") || "");
      let messages = readAllMessages(500).filter(m => {
        if (since && String(m.at||"") <= since) return false;
        if (accountKey) {
          const ak = m.accountKey || "";
          if (ak && ak !== accountKey) return false;
        }
        return true;
      }).slice(-50);
      messages = messages.map(m => ({
        direction: m.direction || "other",
        text: String(m.text||"").slice(0, 200),
        customer: String(m.customer||m.from||""),
        accountKey: m.accountKey || "",
        at: m.at || new Date().toISOString()
      }));
      writeJson(response, 200, { ok: true, messages, now: new Date().toISOString() });
      return;
    }

    if (request.method === "POST" && url.pathname === "/admin-api/sandbox/test") {
      const body = await readJsonBody(request);
      const accountKey = String(body.accountKey || "");
      const message = String(body.message || "");
      if (!accountKey || !message) {
        writeJson(response, 400, { error: "accountKey and message required" });
        return;
      }
      const accounts = readAccountsConfig();
      const account = accounts[accountKey];
      if (!account) {
        writeJson(response, 404, { error: `unknown account: ${accountKey}` });
        return;
      }
      if (openclaw.isGatewayRestarting()) {
        writeJson(response, 503, { error: "gateway is restarting, please wait" });
        return;
      }
      const agentId = account.agentId || "main";
      try {
        const result = await openclaw.runAgentOnce(agentId, message);
        appendSandboxMessage({ accountKey, agentId, message, reply: result.reply });
        appendAuditLog({ action: "sandbox.test", detail: `${accountKey}: ${message.slice(0, 30)}`, ip: clientIp(request) });
        writeJson(response, 200, { ok: true, reply: result.reply, agentId });
      } catch (e) {
        writeJson(response, 500, { error: `sandbox test failed: ${e.message}` });
      }
      return;
    }

    if (request.method === "GET" && url.pathname === "/admin-api/sandbox/history") {
      const accountKey = String(url.searchParams.get("accountKey") || "");
      writeJson(response, 200, { ok: true, messages: readSandboxMessages(accountKey, 20) });
      return;
    }

    if (request.method === "GET" && url.pathname === "/admin-api/alerts") {
      const alerts = readAlerts().slice(0, 100);
      writeJson(response, 200, { ok: true, alerts, unread: alerts.filter(a => !a.read).length });
      return;
    }

    if (request.method === "POST" && url.pathname === "/admin-api/alerts/mark-read") {
      const body = await readJsonBody(request);
      const ids = body.ids || (body.all ? "all" : null);
      if (!ids) { writeJson(response, 400, { error: "ids or all required" }); return; }
      markAlertsRead(ids);
      writeJson(response, 200, { ok: true });
      return;
    }

    if (request.method === "GET" && url.pathname === "/admin-api/health/gateway") {
      const cs = openclaw.getChannelStatus();
      const healthy = cs.refreshedAt && !cs.error;
      writeJson(response, healthy ? 200 : 503, {
        ok: healthy,
        gateway: healthy ? "running" : "error",
        lastRefresh: cs.refreshedAt,
        error: cs.error || null
      });
      return;
    }

    if (request.method === "GET" && url.pathname === "/admin-api/audit-log") {
      const page = Number(url.searchParams.get("page") || 1);
      const pageSize = Number(url.searchParams.get("pageSize") || 30);
      writeJson(response, 200, { ok: true, ...readAuditLog(page, pageSize) });
      return;
    }

    writeJson(response, 404, { error: "not found" });
  } catch (error) {
    writeJson(response, 500, {
      error: error instanceof Error ? error.message : String(error)
    });
  }
});

server.listen(env.port, () => {
  console.log(`WhatsApp AI POC listening on http://localhost:${env.port}`);
});

openclaw.refreshChannelStatus();
setInterval(openclaw.refreshChannelStatus, CHANNEL_STATUS_REFRESH_MS);

setInterval(checkAlerts, 60_000);
