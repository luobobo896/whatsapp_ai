#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
GATEWAY_PORT="${GATEWAY_PORT:-18789}"

command -v openclaw >/dev/null 2>&1 || { echo "openclaw not found. Install OpenClaw CLI first."; exit 1; }

# Verify OpenClaw config exists
OPENCLAW_STATE_DIR="${OPENCLAW_STATE_DIR:-$HOME/.openclaw-whatsapp-poc}"
if [[ ! -f "$OPENCLAW_STATE_DIR/openclaw.json" ]]; then
  echo "OpenClaw config not found at $OPENCLAW_STATE_DIR/openclaw.json"
  echo "Make sure OpenClaw is set up with: openclaw --profile whatsapp-poc configure"
  exit 1
fi

# Clean up old gateway process
pids="$(lsof -ti tcp:"$GATEWAY_PORT" 2>/dev/null || true)"
if [[ -n "$pids" ]]; then
  kill -9 $pids || true
fi

mkdir -p "$PROJECT_ROOT/logs"

# Start OpenClaw Gateway
(cd "$PROJECT_ROOT" && nohup openclaw --profile whatsapp-poc gateway run --force --auth none > logs/openclaw-gateway.out.log 2> logs/openclaw-gateway.err.log < /dev/null &)

echo "Gateway: http://127.0.0.1:$GATEWAY_PORT"
echo "Check:   openclaw --profile whatsapp-poc channels status --deep"
echo "Note:    Admin backend must be started separately: bash scripts/start-admin.sh"
