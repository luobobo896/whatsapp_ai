import fs from "node:fs";
import path from "node:path";
import { openClawStateDir, rootDir } from "./config.js";

const DATA_DIR = path.join(rootDir, "data");
const USAGE_FILE = path.join(DATA_DIR, "usage.json");
const MESSAGE_LOG = path.join(DATA_DIR, "messages.ndjson");
const ACCOUNT_OVERRIDES_FILE = path.join(DATA_DIR, "account-overrides.json");
const OPENCLAW_STATE_DIR = openClawStateDir;
const AUDIT_LOG = path.join(DATA_DIR, "audit-log.ndjson");

function ensureDataDir() {
  fs.mkdirSync(DATA_DIR, { recursive: true });
}

/**
 * @returns {Record<string, number>}
 */
function readUsage() {
  ensureDataDir();
  if (!fs.existsSync(USAGE_FILE)) {
    return {};
  }
  return JSON.parse(fs.readFileSync(USAGE_FILE, "utf8"));
}

/**
 * @param {Record<string, number>} usage
 */
function writeUsage(usage) {
  ensureDataDir();
  fs.writeFileSync(USAGE_FILE, `${JSON.stringify(usage, null, 2)}\n`);
}

/**
 * @returns {string}
 */
export function todayKey() {
  const now = new Date();
  const local = new Date(now.getTime() - now.getTimezoneOffset() * 60_000);
  return local.toISOString().slice(0, 10);
}

/**
 * @param {string} accountKey
 * @returns {number}
 */
export function getDailyUsage(accountKey) {
  const usage = readUsage();
  return usage[`${accountKey}:${todayKey()}`] || 0;
}

/**
 * @param {string} accountKey
 * @returns {number}
 */
export function incrementDailyUsage(accountKey) {
  const usage = readUsage();
  const key = `${accountKey}:${todayKey()}`;
  usage[key] = (usage[key] || 0) + 1;
  writeUsage(usage);
  return usage[key];
}

/**
 * @param {Record<string, unknown>} event
 */
export function appendMessageLog(event) {
  ensureDataDir();
  fs.appendFileSync(MESSAGE_LOG, `${JSON.stringify({ ...event, at: new Date().toISOString() })}\n`);
}

/**
 * @returns {Record<string, {dailyLimit?: number, enabled?: boolean, note?: string}>}
 */
export function readAccountOverrides() {
  ensureDataDir();
  if (!fs.existsSync(ACCOUNT_OVERRIDES_FILE)) {
    return {};
  }
  return JSON.parse(fs.readFileSync(ACCOUNT_OVERRIDES_FILE, "utf8"));
}

/**
 * @param {Record<string, {dailyLimit?: number, enabled?: boolean, note?: string}>} overrides
 */
export function writeAccountOverrides(overrides) {
  ensureDataDir();
  fs.writeFileSync(ACCOUNT_OVERRIDES_FILE, `${JSON.stringify(overrides, null, 2)}\n`);
}

/**
 * @param {string} accountKey
 * @param {{dailyLimit?: number, enabled?: boolean, note?: string}} patch
 * @returns {Record<string, unknown>}
 */
export function updateAccountOverride(accountKey, patch) {
  const overrides = readAccountOverrides();
  overrides[accountKey] = {
    ...(overrides[accountKey] || {}),
    ...patch
  };
  writeAccountOverrides(overrides);
  return overrides[accountKey];
}

/**
 * @param {number} limit
 * @returns {Array<Record<string, unknown>>}
 */
export function readMessageLogs(limit = 100) {
  ensureDataDir();
  if (!fs.existsSync(MESSAGE_LOG)) {
    return [];
  }
  const lines = fs.readFileSync(MESSAGE_LOG, "utf8").split(/\r?\n/).filter(Boolean);
  return lines.slice(-limit).map((line) => JSON.parse(line));
}

/**
 * @param {unknown} content
 * @returns {string}
 */
function contentToText(content) {
  if (typeof content === "string") {
    return content;
  }
  if (!Array.isArray(content)) {
    return "";
  }
  return content
    .filter((part) => part && typeof part === "object" && part.type === "text")
    .map((part) => String(part.text || ""))
    .join("\n")
    .trim();
}

/**
 * @param {string} agentId
 * @param {string} filePath
 * @returns {Array<Record<string, unknown>>}
 */
function readOpenClawSessionFile(agentId, filePath) {
  const events = [];
  const messages = fs.readFileSync(filePath, "utf8").split(/\r?\n/).filter(Boolean);
  for (const line of messages) {
    const item = JSON.parse(line);
    if (item.type !== "message" || !item.message?.role) {
      continue;
    }
    const text = contentToText(item.message.content);
    if (!text) {
      continue;
    }
    events.push({
      source: "openclaw_session",
      agentId,
      sessionId: path.basename(filePath, ".jsonl"),
      direction: item.message.role === "user" ? "inbound" : "outbound",
      status: "recorded",
      text,
      at: item.timestamp || item.message.timestamp || new Date().toISOString()
    });
  }
  return events;
}

/**
 * @param {number} limit
 * @returns {Array<Record<string, unknown>>}
 */
export function readOpenClawMessageLogs(limit = 100) {
  const agentsDir = path.join(OPENCLAW_STATE_DIR, "agents");
  if (!fs.existsSync(agentsDir)) {
    return [];
  }
  const sessionFiles = [];
  for (const agentId of fs.readdirSync(agentsDir)) {
    const sessionsDir = path.join(agentsDir, agentId, "sessions");
    if (!fs.existsSync(sessionsDir)) {
      continue;
    }
    for (const name of fs.readdirSync(sessionsDir)) {
      if (!name.endsWith(".jsonl")) {
        continue;
      }
      const filePath = path.join(sessionsDir, name);
      sessionFiles.push({ agentId, filePath, mtimeMs: fs.statSync(filePath).mtimeMs });
    }
  }
  return sessionFiles
    .sort((left, right) => right.mtimeMs - left.mtimeMs)
    .slice(0, 50)
    .flatMap((file) => readOpenClawSessionFile(file.agentId, file.filePath))
    .sort((left, right) => new Date(String(left.at)).getTime() - new Date(String(right.at)).getTime())
    .slice(-limit);
}

const SETTINGS_FILE = path.join(DATA_DIR, "settings.json");
const DEFAULT_SETTINGS = {
  workingHours: { start: "09:00", end: "18:00", timezone: "Asia/Shanghai" },
  autoReply: { outOfHours: "当前非工作时间，我们会在工作时间尽快回复您。", greeting: "您好，请问有什么可以帮您的？" },
  globalDefaults: { dailyLimit: 30 }
};

export function readSettings() {
  ensureDataDir();
  if (!fs.existsSync(SETTINGS_FILE)) return { ...DEFAULT_SETTINGS };
  return { ...DEFAULT_SETTINGS, ...JSON.parse(fs.readFileSync(SETTINGS_FILE, "utf8")) };
}

export function writeSettings(settings) {
  ensureDataDir();
  fs.writeFileSync(SETTINGS_FILE, JSON.stringify(settings, null, 2) + "\n");
}

const SANDBOX_LOG = path.join(DATA_DIR, "sandbox-messages.ndjson");

export function appendSandboxMessage(entry) {
  ensureDataDir();
  fs.appendFileSync(SANDBOX_LOG, JSON.stringify({ ...entry, at: new Date().toISOString() }) + "\n");
}

export function readSandboxMessages(accountKey, limit = 20) {
  ensureDataDir();
  if (!fs.existsSync(SANDBOX_LOG)) return [];
  const lines = fs.readFileSync(SANDBOX_LOG, "utf8").split(/\r?\n/).filter(Boolean);
  const msgs = lines.map(l => JSON.parse(l));
  if (accountKey) return msgs.filter(m => m.accountKey === accountKey).slice(-limit);
  return msgs.slice(-limit);
}

export function appendAuditLog(entry) {
  ensureDataDir();
  fs.appendFileSync(AUDIT_LOG, JSON.stringify({ ...entry, at: new Date().toISOString() }) + "\n");
}

export function readAuditLog(page = 1, pageSize = 30) {
  ensureDataDir();
  if (!fs.existsSync(AUDIT_LOG)) return { logs: [], page: 1, total: 0, totalPages: 0 };
  const lines = fs.readFileSync(AUDIT_LOG, "utf8").split(/\r?\n/).filter(Boolean);
  const logs = lines.map(l => JSON.parse(l)).reverse();
  const total = logs.length;
  const totalPages = Math.max(1, Math.ceil(total / pageSize));
  const safePage = Math.min(Math.max(1, page), totalPages);
  const start = (safePage - 1) * pageSize;
  return { logs: logs.slice(start, start + pageSize), page: safePage, total, totalPages };
}

// ── Anti-ban ─────────────────────────────────────────────────────────────

const ANTIBAN_FILE = path.join(DATA_DIR, "antiban.json");
const DEFAULT_ANTIBAN = {
  replyDelay: { min: 1000, max: 5000 },
  rateLimit: { maxPerHour: 30, maxPerDay: 200 },
  warmup: { enabled: true, durationHours: 24 },
  sessionKeepalive: { enabled: true, intervalMinutes: 5 }
};

export function readAntiBanConfig() {
  ensureDataDir();
  if (!fs.existsSync(ANTIBAN_FILE)) return { ...DEFAULT_ANTIBAN };
  return { ...DEFAULT_ANTIBAN, ...JSON.parse(fs.readFileSync(ANTIBAN_FILE, "utf8")) };
}

export function writeAntiBanConfig(config) {
  ensureDataDir();
  fs.writeFileSync(ANTIBAN_FILE, JSON.stringify(config, null, 2) + "\n");
}

const MSG_COUNTS_FILE = path.join(DATA_DIR, "msg-counts.json");
function readMsgCounts() {
  ensureDataDir();
  if (!fs.existsSync(MSG_COUNTS_FILE)) return {};
  return JSON.parse(fs.readFileSync(MSG_COUNTS_FILE, "utf8"));
}
function writeMsgCounts(counts) {
  fs.writeFileSync(MSG_COUNTS_FILE, JSON.stringify(counts, null, 2) + "\n");
}

export function checkRateLimit(accountKey) {
  const config = readAntiBanConfig();
  const counts = readMsgCounts();
  const hourKey = `${accountKey}:${new Date().toISOString().slice(0,13)}`;
  const dayKey = `${accountKey}:${new Date().toISOString().slice(0,10)}`;
  const hourCount = counts[hourKey] || 0;
  const dayCount = counts[dayKey] || 0;
  if (hourCount >= config.rateLimit.maxPerHour) return { blocked: true, reason: "hourly_limit" };
  if (dayCount >= config.rateLimit.maxPerDay) return { blocked: true, reason: "daily_limit" };
  return { blocked: false };
}

export function incrementMsgCount(accountKey) {
  const counts = readMsgCounts();
  const hourKey = `${accountKey}:${new Date().toISOString().slice(0,13)}`;
  const dayKey = `${accountKey}:${new Date().toISOString().slice(0,10)}`;
  counts[hourKey] = (counts[hourKey] || 0) + 1;
  counts[dayKey] = (counts[dayKey] || 0) + 1;
  writeMsgCounts(counts);
}

export function getAccountWarmupHours(accountKey) {
  const warmupFile = path.join(DATA_DIR, "warmup.json");
  ensureDataDir();
  if (!fs.existsSync(warmupFile)) return null;
  const warmup = JSON.parse(fs.readFileSync(warmupFile, "utf8"));
  const start = warmup[accountKey];
  if (!start) return null;
  const elapsed = (Date.now() - new Date(start).getTime()) / 3600000;
  return elapsed;
}

export function markAccountWarmupStart(accountKey) {
  const warmupFile = path.join(DATA_DIR, "warmup.json");
  ensureDataDir();
  const warmup = fs.existsSync(warmupFile) ? JSON.parse(fs.readFileSync(warmupFile, "utf8")) : {};
  if (!warmup[accountKey]) warmup[accountKey] = new Date().toISOString();
  fs.writeFileSync(warmupFile, JSON.stringify(warmup, null, 2) + "\n");
}

export function isWarmingUp(accountKey) {
  const config = readAntiBanConfig();
  if (!config.warmup.enabled) return false;
  const elapsed = getAccountWarmupHours(accountKey);
  if (elapsed === null) return true;
  return elapsed < config.warmup.durationHours;
}

// ── Knowledge backups ────────────────────────────────────────────────────

const KNOWLEDGE_BACKUPS_DIR = path.join(rootDir, "config", "knowledge-backups");

export function backupKnowledge() {
  const knowledgePath = path.join(rootDir, "config", "knowledge.json");
  if (!fs.existsSync(knowledgePath)) return null;
  const dir = KNOWLEDGE_BACKUPS_DIR;
  fs.mkdirSync(dir, { recursive: true });
  const timestamp = new Date().toISOString().replace(/[:.]/g, "-").slice(0, 19);
  const backupPath = path.join(dir, `${timestamp}.json`);
  fs.copyFileSync(knowledgePath, backupPath);
  return { path: backupPath, timestamp };
}

export function listKnowledgeBackups() {
  const dir = KNOWLEDGE_BACKUPS_DIR;
  if (!fs.existsSync(dir)) return [];
  return fs.readdirSync(dir).filter(f => f.endsWith(".json")).sort().reverse().map(f => {
    const stat = fs.statSync(path.join(dir, f));
    return { file: f, size: stat.size, at: stat.mtime.toISOString() };
  });
}

export function restoreKnowledgeBackup(filename) {
  const knowledgePath = path.join(rootDir, "config", "knowledge.json");
  const backupPath = path.join(KNOWLEDGE_BACKUPS_DIR, filename);
  if (!fs.existsSync(backupPath)) throw new Error(`backup not found: ${filename}`);
  fs.copyFileSync(backupPath, knowledgePath);
  return JSON.parse(fs.readFileSync(knowledgePath, "utf8"));
}

export function getKnowledgeAnalytics() {
  const allMessages = [];
  const msgLog = path.join(DATA_DIR, "messages.ndjson");
  if (fs.existsSync(msgLog)) {
    allMessages.push(...fs.readFileSync(msgLog, "utf8").split(/\r?\n/).filter(Boolean).map(l => JSON.parse(l)));
  }
  const knowledgePath = path.join(rootDir, "config", "knowledge.json");
  if (!fs.existsSync(knowledgePath)) return {};
  const knowledge = JSON.parse(fs.readFileSync(knowledgePath, "utf8"));
  const analytics = {};
  for (const role of (knowledge.roles || [])) {
    const keywords = role.keywords || [];
    const replies = allMessages.filter(m => m.direction === "outbound" && keywords.some(kw => String(m.text||"").includes(kw)));
    const handoffs = allMessages.filter(m => String(m.text||"").includes("转人工") && keywords.some(kw => String(m.text||"").includes(kw)));
    analytics[role.id] = {
      name: role.name,
      replyCount: replies.length,
      handoffCount: handoffs.length,
      hitRate: replies.length > 0 ? Math.round((replies.length - handoffs.length) / replies.length * 100) : 0
    };
  }
  return analytics;
}

const ALERTS_FILE = path.join(DATA_DIR, "alerts.ndjson");

export function readAlerts() {
  ensureDataDir();
  if (!fs.existsSync(ALERTS_FILE)) return [];
  return fs.readFileSync(ALERTS_FILE, "utf8").split(/\r?\n/).filter(Boolean).map(l => JSON.parse(l)).reverse();
}

export function appendAlert(alert) {
  ensureDataDir();
  const entry = { ...alert, id: Date.now().toString(36), at: new Date().toISOString(), read: false };
  fs.appendFileSync(ALERTS_FILE, JSON.stringify(entry) + "\n");
  return entry;
}

export function markAlertsRead(ids) {
  ensureDataDir();
  if (!fs.existsSync(ALERTS_FILE)) return;
  const lines = fs.readFileSync(ALERTS_FILE, "utf8").split(/\r?\n/).filter(Boolean).map(l => JSON.parse(l));
  const updated = lines.map(a => ids === "all" || (Array.isArray(ids) && ids.includes(a.id)) ? { ...a, read: true } : a);
  fs.writeFileSync(ALERTS_FILE, updated.map(a => JSON.stringify(a)).join("\n") + "\n");
}
