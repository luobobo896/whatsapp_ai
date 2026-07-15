#!/bin/zsh

set -euo pipefail

project_root="${0:A:h:h}"
dbx_store="$HOME/Library/Application Support/com.dbx.app/dbx.db"
openclaw_config="$HOME/.openclaw/openclaw.json"
internal_token_file="$HOME/.openclaw/whatsapp-ai.internal-token"
python_bin="/opt/homebrew/bin/python3"

db_password="$(/usr/bin/sqlite3 "$dbx_store" "SELECT secret FROM connection_secrets WHERE connection_id = 'bf776d63-aeae-4503-b909-694e85028a68' AND key = 'password';")"
if [[ -z "$db_password" ]]; then
  print -u2 'PostgreSQL password is not available from the local DBX connection.'
  exit 1
fi

export DB_PASSWORD="$db_password"
export DATABASE_URL="$($python_bin -c 'import os; from urllib.parse import quote; print("postgres://admin:" + quote(os.environ["DB_PASSWORD"], safe="") + "@new.hsddns.com:5432/whatsapp_ai?sslmode=disable")')"
unset DB_PASSWORD db_password

if [[ -z "${INTERNAL_API_TOKEN:-}" ]]; then
  if [[ -r "$internal_token_file" ]]; then
    export INTERNAL_API_TOKEN="$(<"$internal_token_file")"
  else
    umask 077
    export INTERNAL_API_TOKEN="$(/usr/bin/openssl rand -hex 32)"
    print -r -- "$INTERNAL_API_TOKEN" > "$internal_token_file"
  fi
fi
export HTTP_HOST="127.0.0.1"
export PORT="8790"
export COOKIE_SECURE="false"

exec "$project_root/server"
