import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const ROOT_DIR = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");

const envFile = path.join(ROOT_DIR, ".env");
if (fs.existsSync(envFile)) {
  for (const line of fs.readFileSync(envFile, "utf8").split(/\r?\n/)) {
    const trimmed = line.trim();
    if (!trimmed || trimmed.startsWith("#") || !trimmed.includes("=")) continue;
    const [key, ...valueParts] = trimmed.split("=");
    if (!process.env[key]) process.env[key] = valueParts.join("=").trim();
  }
}

import os from "node:os";

export const rootDir = ROOT_DIR;
export const openClawStateDir = process.env.OPENCLAW_STATE_DIR || path.join(os.homedir(), ".openclaw-whatsapp-poc");
export const openClawConfigPath = process.env.OPENCLAW_CONFIG_PATH || path.join(openClawStateDir, "openclaw.json");

export const env = {
  port: Number(process.env.PORT || 8790),
};

export const DB_PATH = process.env.DB_PATH || path.join(rootDir, "data", "app.db");
