#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Run migration preparation (rewrite paths for current machine)
"$SCRIPT_DIR/prepare-migration.sh"

# Start each service independently
echo "=== Starting Admin Backend ==="
bash "$SCRIPT_DIR/start-admin.sh"

echo ""
echo "=== Starting OpenClaw Gateway ==="
bash "$SCRIPT_DIR/start-gateway.sh"

echo ""
echo "Both services started."
echo "Admin:   http://127.0.0.1:${ADMIN_PORT:-8790}"
echo "Gateway: http://127.0.0.1:${GATEWAY_PORT:-18789}"
