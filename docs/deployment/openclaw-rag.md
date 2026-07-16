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

## OpenClaw access policy

WhatsApp AI manages the OpenClaw access policy on every account sync:

- `gateway.auth.mode=token`; keep `OPENCLAW_GATEWAY_TOKEN` in the OpenClaw
  service environment.
- WhatsApp DMs use `dmPolicy=open` and `allowFrom=["*"]`, so customers do not
  need pairing approval.
- Agent sandboxing is off and the tool/exec profile is `full`; per-agent and
  MCP tool allowlists are removed.

The equivalent OpenClaw commands are:

```sh
openclaw config set gateway.auth.mode token
openclaw config set channels.whatsapp.dmPolicy open
openclaw config set channels.whatsapp.allowFrom '["*"]'
openclaw config set agents.defaults.sandbox.mode off
openclaw config set tools.profile full
openclaw config set tools.exec.mode full
```

## Model authentication

Do not put provider API keys in the WhatsApp AI `.env` file. OpenClaw isolates
model authentication per agent. Configure each managed agent through the
official OpenClaw auth command, which writes a static API-key profile to that
agent's auth store:

```sh
openclaw models auth --agent <agent-id> paste-api-key \
  --provider deepseek --profile-id deepseek:whatsapp-ai
openclaw models auth list --agent <agent-id> --provider deepseek
openclaw models status --agent <agent-id> --probe --probe-provider deepseek
```

Repeat this for every `whatsapp-rag-<account-key>` agent returned by
`openclaw agents list`. The key is entered on standard input and must not be
passed as a command-line argument. OpenClaw's multi-agent documentation allows
secondary agents to read through to the configured default agent's portable
static profile, but keeping a local static profile on every WhatsApp agent
avoids failures if the default agent changes.
