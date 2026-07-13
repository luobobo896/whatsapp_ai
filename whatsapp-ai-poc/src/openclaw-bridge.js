/**
 * OpenClaw Bridge — all OpenClaw interactions through one seam.
 * server.js calls this module; never touches openclaw.json, state dirs, or the CLI directly.
 */
import { execFile, execFileSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { pathToFileURL } from "node:url";
import { rootDir, openClawStateDir, openClawConfigPath } from "./config.js";
import { readAntiBanConfig } from "./store.js";
const IS_WINDOWS = process.platform === "win32";
const OPENCLAW_CLI = IS_WINDOWS ? "openclaw.cmd" : "openclaw";
const WHATSAPP_RUNTIME_API_URL = pathToFileURL(
  path.join(openClawStateDir, "extensions", "whatsapp", "dist", "runtime-api.js")
).href;

let whatsAppRuntimeApiPromise = null;
let gatewayRestarting = false;
let restartNeeded = false;

// ── Config helpers ──────────────────────────────────────────────────────

export function readOpenClawConfig() {
  return JSON.parse(fs.readFileSync(openClawConfigPath, "utf8"));
}

export function writeOpenClawConfig(config) {
  fs.writeFileSync(openClawConfigPath, `${JSON.stringify(config, null, 2)}\n`, "utf8");
}

// ── Agent helpers ───────────────────────────────────────────────────────

export function isGatewayRestarting() {
  return gatewayRestarting;
}

export function getReplyDelay() {
  const config = readAntiBanConfig();
  const { min, max } = config.replyDelay;
  return min + Math.random() * (max - min);
}

// ── Channel status ──────────────────────────────────────────────────────

const channelStatusCache = { status: {}, refreshedAt: null, refreshing: false, error: null };

function parseOpenClawChannelStatus(output) {
  const status = {};
  for (const line of output.split(/\r?\n/)) {
    const match = line.match(/^- WhatsApp (.+?)\): (.+)$/);
    if (!match) continue;
    const accountKey = match[1].trim().replace(/\s+\(.+$/, "");
    const tokens = new Set(match[2].split(",").map((p) => p.trim()));
    status[accountKey] = {
      raw: match[2],
      linked: tokens.has("linked"),
      running: tokens.has("running"),
      connected: tokens.has("connected"),
      healthy: match[2].includes("health:healthy"),
      disabled: tokens.has("disabled")
    };
  }
  return status;
}

export function refreshChannelStatus() {
  if (channelStatusCache.refreshing) return;
  channelStatusCache.refreshing = true;
  execFile(OPENCLAW_CLI, ["--profile", "whatsapp-poc", "channels", "status", "--deep"], {
    encoding: "utf8", timeout: 30_000, windowsHide: IS_WINDOWS
  }, (error, stdout) => {
    channelStatusCache.refreshing = false;
    if (error) { channelStatusCache.error = error.message; return; }
    channelStatusCache.status = parseOpenClawChannelStatus(stdout);
    channelStatusCache.refreshedAt = new Date().toISOString();
    channelStatusCache.error = null;
  });
}

export function getChannelStatus() {
  return { ...channelStatusCache };
}

export function readChannelStatus() {
  if (!channelStatusCache.refreshedAt && !channelStatusCache.refreshing) {
    refreshChannelStatus();
  }
  return channelStatusCache.status;
}

// ── Gateway lifecycle ───────────────────────────────────────────────────

function gatewayRestartCommand() {
  const port = "18789";
  return [
    `pids="$(lsof -ti tcp:${port} 2>/dev/null || true)"`,
    'if [ -n "$pids" ]; then kill -TERM $pids 2>/dev/null || true; fi',
    "for i in 1 2 3 4 5; do",
    '  pids="$(lsof -ti tcp:' + port + ' 2>/dev/null || true)"',
    '  [ -z "$pids" ] && break',
    "  sleep 2",
    "done",
    'pids="$(lsof -ti tcp:' + port + ' 2>/dev/null || true)"',
    'if [ -n "$pids" ]; then kill -9 $pids 2>/dev/null || true; fi',
    "sleep 1",
    `nohup ${OPENCLAW_CLI} --profile whatsapp-poc gateway run --force --auth none > logs/openclaw-gateway.out.log 2> logs/openclaw-gateway.err.log < /dev/null &`
  ].join("; ");
}

export function markRestartNeeded() {
  restartNeeded = true;
}

export function isRestartNeeded() {
  return restartNeeded;
}

/**
 * Manual gateway restart — only called when user explicitly triggers it.
 * The admin backend no longer auto-restarts the gateway.
 */
export function performGatewayRestart() {
  if (gatewayRestarting) return;
  gatewayRestarting = true;
  restartNeeded = false;
  execFile(IS_WINDOWS ? "powershell.exe" : "bash",
    IS_WINDOWS ? ["-NoProfile", "-Command", gatewayRestartCommand()] : ["-lc", gatewayRestartCommand()],
    { windowsHide: IS_WINDOWS, cwd: rootDir, timeout: 90_000 },
    (error) => {
      gatewayRestarting = false;
      if (error) { console.error("gateway restart failed", error); return; }
      refreshChannelStatus();
    }
  );
}

/**
 * @deprecated Use markRestartNeeded() + performGatewayRestart() instead.
 * Kept for backward compatibility with existing callers that need an immediate restart.
 */
export function scheduleGatewayRestart() {
  markRestartNeeded();
  performGatewayRestart();
}

// ── WhatsApp runtime API ────────────────────────────────────────────────

/**
 * OpenClaw's internal WhatsApp runtime API module.
 * This IS the intended programmatic API for WhatsApp operations.
 * QR login/logout cannot be done via CLI because `channels login` is
 * interactive TUI-only with no --json output. The runtime API is the
 * correct way to drive these operations programmatically.
 */
export async function loadWhatsAppRuntimeApi() {
  process.env.OPENCLAW_PROFILE = "whatsapp-poc";
  process.env.OPENCLAW_STATE_DIR = openClawStateDir;
  process.env.OPENCLAW_HOME = openClawStateDir;
  process.env.OPENCLAW_CONFIG_PATH = openClawConfigPath;
  whatsAppRuntimeApiPromise ||= import(WHATSAPP_RUNTIME_API_URL);
  return whatsAppRuntimeApiPromise;
}

// ── Account lifecycle via OpenClaw CLI ──────────────────────────────────

/**
 * Create agent + channel + binding via OpenClaw CLI (pure pass-through).
 * Replaces the old ensureAgentAndRoute() which wrote openclaw.json directly.
 */
export async function createAgentViaCli(agentId, accountKey, label) {
  return new Promise((resolve, reject) => {
    execFile(OPENCLAW_CLI, [
      "--profile", "whatsapp-poc",
      "agents", "add", agentId,
      "--workspace", rootDir,
      "--bind", `whatsapp:${accountKey}`,
      "--non-interactive",
      "--json"
    ], {
      encoding: "utf8", timeout: 30_000, windowsHide: IS_WINDOWS
    }, (error, stdout) => {
      if (error) {
        // If agent already exists, try bind-only
        execFile(OPENCLAW_CLI, [
          "--profile", "whatsapp-poc",
          "agents", "bind",
          "--agent", agentId,
          "--bind", `whatsapp:${accountKey}`,
          "--json"
        ], {
          encoding: "utf8", timeout: 15_000, windowsHide: IS_WINDOWS
        }, (bindError, bindStdout) => {
          if (bindError) {
            console.error(`agent bind failed for ${agentId}:`, bindError.message);
            resolve({ success: false, error: bindError.message });
            return;
          }
          addChannelAccountViaCli(accountKey, label).then(resolve).catch(resolve);
        });
        return;
      }
      // Agent created — now add channel account
      addChannelAccountViaCli(accountKey, label).then(resolve).catch(resolve);
    });
  });
}

/**
 * Add or update a WhatsApp channel account via OpenClaw CLI.
 */
export async function addChannelAccountViaCli(accountKey, label) {
  return new Promise((resolve, reject) => {
    execFile(OPENCLAW_CLI, [
      "--profile", "whatsapp-poc",
      "channels", "add",
      "--channel", "whatsapp",
      "--account", accountKey,
      "--name", label
    ], {
      encoding: "utf8", timeout: 15_000, windowsHide: IS_WINDOWS
    }, (error, stdout) => {
      if (error) {
        console.error(`channel add failed for ${accountKey}:`, error.message);
        resolve({ success: false, error: error.message });
        return;
      }
      resolve({ success: true, output: stdout.trim() });
    });
  });
}

/**
 * Enable or disable a WhatsApp channel account.
 * Note: `channels add` with updated config is the CLI way to toggle.
 * Falls back to direct config write for atomic toggles.
 */
export function setAccountEnabled(accountKey, enabled) {
  const config = readOpenClawConfig();
  const account = config.channels?.whatsapp?.accounts?.[accountKey];
  if (!account) throw new Error(`OpenClaw WhatsApp account not found: ${accountKey}`);
  account.enabled = enabled;
  writeOpenClawConfig(config);
  markRestartNeeded();
}

/** @deprecated — use createAgentViaCli() instead */
export function ensureAgentAndRoute(config, accountKey, label, agentId) {
  config.agents ||= {};
  config.agents.list ||= [];
  const workspace = rootDir;
  const agentDir = path.join(openClawStateDir, "agents", agentId, "agent");
  if (!config.agents.list.some((a) => a.id === agentId)) {
    config.agents.list.push({ id: agentId, name: agentId, workspace, agentDir });
  }
  config.channels ||= {};
  config.channels.whatsapp ||= { enabled: true, accounts: {} };
  config.channels.whatsapp.enabled = true;
  config.channels.whatsapp.accounts ||= {};
  config.channels.whatsapp.accounts[accountKey] = {
    ...(config.channels.whatsapp.accounts[accountKey] || {}),
    name: label, enabled: true, allowFrom: ["*"], dmPolicy: "open", debounceMs: 3500
  };
  config.bindings ||= [];
  if (!config.bindings.some((b) =>
    b.type === "route" && b.match?.channel === "whatsapp" && b.match?.accountId === accountKey
  )) {
    config.bindings.push({ type: "route", agentId, match: { channel: "whatsapp", accountId: accountKey } });
  }
}

/** @deprecated — use removeAccountViaCli() + deleteAgentViaCli() instead */
export function removeAgentAndRoute(config, accountKey, agentId) {
  if (config.channels?.whatsapp?.accounts?.[accountKey]) {
    delete config.channels.whatsapp.accounts[accountKey];
  }
  if (config.bindings) {
    config.bindings = config.bindings.filter((b) =>
      !(b.type === "route" && b.match?.channel === "whatsapp" && b.match?.accountId === accountKey)
    );
  }
  if (config.agents?.list) {
    config.agents.list = config.agents.list.filter((a) => a.id !== agentId);
  }
}

// ── WhatsApp login / logout ─────────────────────────────────────────────

export async function startWebLogin(accountKey) {
  const runtime = await loadWhatsAppRuntimeApi();
  return runtime.startWebLoginWithQr({
    accountId: accountKey, timeoutMs: 30_000, force: true, verbose: false
  });
}

export async function waitForWebLogin(accountKey, currentQrDataUrl) {
  const runtime = await loadWhatsAppRuntimeApi();
  return runtime.waitForWebLogin({
    accountId: accountKey, timeoutMs: 1000, currentQrDataUrl
  });
}

export async function logoutAccount(accountKey) {
  const runtime = await loadWhatsAppRuntimeApi();
  return runtime.logoutWeb({ accountId: accountKey });
}

// ── Credential / session cleanup ────────────────────────────────────────

/**
 * Logout a single WhatsApp account via OpenClaw CLI.
 * This is the proper way — uses OpenClaw's native credential management.
 */
export async function logoutAccountViaCli(accountKey) {
  return new Promise((resolve, reject) => {
    execFile(OPENCLAW_CLI, [
      "--profile", "whatsapp-poc",
      "channels", "logout",
      "--channel", "whatsapp",
      "--account", accountKey
    ], {
      encoding: "utf8", timeout: 30_000, windowsHide: IS_WINDOWS
    }, (error, stdout) => {
      if (error) {
        console.error(`logout failed for ${accountKey}:`, error.message);
        resolve({ success: false, error: error.message });
        return;
      }
      resolve({ success: true, output: stdout.trim() });
    });
  });
}

/**
 * Remove a WhatsApp account config and state via OpenClaw CLI.
 * Uses `channels remove --delete` which handles both config and credential cleanup.
 */
export async function removeAccountViaCli(accountKey) {
  return new Promise((resolve, reject) => {
    execFile(OPENCLAW_CLI, [
      "--profile", "whatsapp-poc",
      "channels", "remove",
      "--channel", "whatsapp",
      "--account", accountKey,
      "--delete"
    ], {
      encoding: "utf8", timeout: 30_000, windowsHide: IS_WINDOWS
    }, (error, stdout) => {
      if (error) {
        console.error(`remove failed for ${accountKey}:`, error.message);
        resolve({ success: false, error: error.message });
        return;
      }
      resolve({ success: true, output: stdout.trim() });
    });
  });
}

/**
 * Delete an agent via OpenClaw CLI (auto-prunes sessions and state).
 */
export async function deleteAgentViaCli(agentId) {
  return new Promise((resolve, reject) => {
    execFile(OPENCLAW_CLI, [
      "--profile", "whatsapp-poc",
      "agents", "delete", agentId
    ], {
      encoding: "utf8", timeout: 30_000, windowsHide: IS_WINDOWS
    }, (error, stdout) => {
      if (error) {
        console.error(`agent delete failed for ${agentId}:`, error.message);
        resolve({ success: false, error: error.message });
        return;
      }
      resolve({ success: true, output: stdout.trim() });
    });
  });
}

/**
 * @deprecated Use logoutAccountViaCli() instead.
 * Direct filesystem credential deletion bypasses OpenClaw's native management.
 */
export function removeAccountDirs(accountKey, agentId) {
  // Use CLI-based approach instead of direct fs operations.
  // The caller (server.js deleteAccount) now uses removeAccountViaCli.
  console.warn("removeAccountDirs is deprecated. Use removeAccountViaCli instead.");
}

/**
 * @deprecated Use logoutAccountViaCli() per-account instead.
 * Direct filesystem credential wipe bypasses OpenClaw's native management.
 */
export function wipeAllCredentials() {
  // Use CLI-based logout per account instead of direct fs operations.
  // The caller (server.js clearAllSessions) now uses logoutAccountViaCli.
  console.warn("wipeAllCredentials is deprecated. Use logoutAccountViaCli per account instead.");
}

/**
 * @deprecated Use deleteAgentViaCli() instead.
 * Direct filesystem session deletion bypasses OpenClaw's native session management.
 */
export function clearAllRouterSessions() {
  // Use CLI-based agent deletion instead of direct fs operations.
  // The caller (server.js clearAllSessions) should use deleteAgentViaCli per agent.
  console.warn("clearAllRouterSessions is deprecated. Use deleteAgentViaCli instead.");
}

export async function runAgentOnce(agentId, message) {
  return new Promise((resolve) => {
    execFile(OPENCLAW_CLI, [
      "--profile", "whatsapp-poc",
      "agent",
      "--agent", agentId,
      "--message", message,
      "--json",
      "--timeout", "30"
    ], {
      encoding: "utf8",
      timeout: 35_000,
      windowsHide: IS_WINDOWS,
      cwd: rootDir
    }, (error, stdout) => {
      if (error) {
        resolve({ reply: "快捷测试功能需要 OpenClaw gateway 运行中。请确认 gateway 已启动。" });
        return;
      }
      try {
        const result = JSON.parse(stdout);
        // OpenClaw --json returns: { payloads: [{ text: "..." }] }
        const reply = result.payloads?.[0]?.text
          || result.reply
          || result.text
          || result.message
          || stdout.trim();
        resolve({ reply: typeof reply === "string" ? reply : JSON.stringify(reply) });
      } catch {
        resolve({ reply: stdout.trim() || "(empty reply)" });
      }
    });
  });
}
