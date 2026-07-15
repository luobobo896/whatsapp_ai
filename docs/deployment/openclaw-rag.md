# OpenClaw RAG Routing

Set these variables for an installation that runs OpenClaw in Docker:

```sh
OPENCLAW_DOCKER_CONTAINER=openclaw
WHATSAPP_AI_RAG_API_URL=https://your-whatsapp-ai-host
INTERNAL_API_TOKEN=replace-with-a-secret
```

When a WhatsApp QR login succeeds, WhatsApp AI creates one MCP server and one
OpenClaw route for that account. The route is scoped by the OpenClaw WhatsApp
account key, while the MCP server uses the application account ID to retrieve
only that account's bound knowledge bases.

The service also synchronizes all non-disabled accounts at startup. The RAG MCP
source is discovered from `cmd/rag-mcp-server` in the working tree, or from
`WHATSAPP_AI_RAG_MCP_SOURCE_DIR` when that location is customized.

For local `tools/launch-server.sh` usage, the script persists its generated
internal token in `~/.openclaw/whatsapp-ai.internal-token`. Set
`INTERNAL_API_TOKEN` before launch to use a managed token instead.

The backend also writes the token to `~/.openclaw/.env` with mode `0600`. Each
managed MCP definition references `${INTERNAL_API_TOKEN}` instead of storing the
secret directly in `openclaw.json`.

For a public customer-service number, configure both WhatsApp direct-message
settings before restarting the gateway:

```sh
openclaw config set channels.whatsapp.dmPolicy open
openclaw config set channels.whatsapp.allowFrom '["*"]'
```

Keep `pairing` instead when every new customer must be approved manually.
