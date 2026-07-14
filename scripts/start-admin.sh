#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
ADMIN_PORT="${ADMIN_PORT:-8790}"

command -v go >/dev/null 2>&1 || { echo "go not found. Install Go 1.26 first."; exit 1; }

# Clean up old admin process
pids="$(lsof -ti tcp:"$ADMIN_PORT" 2>/dev/null || true)"
if [[ -n "$pids" ]]; then
  kill -9 $pids || true
fi

mkdir -p "$PROJECT_ROOT/logs"

# Start admin server
(cd "$PROJECT_ROOT" && env HTTP_HOST="${HTTP_HOST:-127.0.0.1}" PORT="$ADMIN_PORT" APP_ORIGIN="${APP_ORIGIN:-http://127.0.0.1:$ADMIN_PORT}" nohup go run ./cmd/server > logs/admin-server-8790.out.log 2> logs/admin-server-8790.err.log < /dev/null &)

echo "Admin:   http://127.0.0.1:$ADMIN_PORT"
echo "Admin backend started."
