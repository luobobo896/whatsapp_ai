#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
TARGET_DIR="${OPENCLAW_STATE_DIR:-$HOME/.openclaw-whatsapp-poc}"

# Rewrite workspace/agent paths for the current machine
node - "$PROJECT_ROOT" "$TARGET_DIR" <<'NODE'
const fs = require('node:fs');
const path = require('node:path');
const [projectRoot, targetDir] = process.argv.slice(2);
const configPath = path.join(targetDir, 'openclaw.json');
if (!fs.existsSync(configPath)) {
  throw new Error(`OpenClaw config not found: ${configPath}`);
}
const config = JSON.parse(fs.readFileSync(configPath, 'utf8'));
config.agents ||= {};
config.agents.defaults ||= {};
config.agents.defaults.workspace = projectRoot;
config.agents.list ||= [];
for (const agent of config.agents.list) {
  if (!agent || agent.id === 'main') continue;
  agent.workspace = projectRoot;
  agent.agentDir = path.join(targetDir, 'agents', agent.id, 'agent');
}
fs.writeFileSync(configPath, `${JSON.stringify(config, null, 2)}\n`);
NODE

echo "Migration paths updated."
echo "Project root: $PROJECT_ROOT"
echo "OpenClaw state: $TARGET_DIR"
