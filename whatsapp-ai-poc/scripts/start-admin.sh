#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
ADMIN_PORT="${ADMIN_PORT:-8790}"

command -v node >/dev/null 2>&1 || { echo "node not found. Install Node.js 20+ first."; exit 1; }

if [[ ! -d "$PROJECT_ROOT/node_modules" ]]; then
  (cd "$PROJECT_ROOT" && npm install)
fi

# Clean up old admin process
pids="$(lsof -ti tcp:"$ADMIN_PORT" 2>/dev/null || true)"
if [[ -n "$pids" ]]; then
  kill -9 $pids || true
fi

mkdir -p "$PROJECT_ROOT/logs"

# Generate AGENTS.md from latest config
(cd "$PROJECT_ROOT" && node src/build-agent-prompt.js)

# Start admin server
(cd "$PROJECT_ROOT" && env PORT="$ADMIN_PORT" nohup node src/server.js > logs/admin-server-8790.out.log 2> logs/admin-server-8790.err.log < /dev/null &)

echo "Admin:   http://127.0.0.1:$ADMIN_PORT"
echo "Note:    OpenClaw Gateway must be started separately: bash scripts/start-gateway.sh"
