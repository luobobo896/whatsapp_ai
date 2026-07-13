---
parent: map.md
type: task
status: open
assignee: 
blocked_by: ticket-02-session-context.md, ticket-03-credential-architecture.md
---
## Question

What changes are needed in `server.js` and `openclaw-bridge.js` to support session-aware context management and clean credential separation?

**Scope**: Implementation ticket for the server-side changes:
1. Add session tracking — track active customer sessions (customer ID + account key → session state)
2. Session lifecycle API — endpoints or logic for session start, active, timeout, explicit end
3. Refactor `openclaw-bridge.js` to be a pure pass-through — use OpenClaw CLI/API instead of direct file manipulation for credential operations
4. Remove or isolate credential directory management (`removeAccountDirs`, `wipeAllCredentials`, `clearAllRouterSessions`)
5. Update the message reading logic to associate messages with sessions

**Constraints**:
- Admin backend's own data stays in `data/` directory
- OpenClaw state stays in `~/.openclaw-whatsapp-poc/`
- Bridge functions must use OpenClaw's APIs/CLI, not direct filesystem access to OpenClaw state
