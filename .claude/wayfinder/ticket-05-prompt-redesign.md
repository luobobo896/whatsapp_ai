---
parent: map.md
type: task
status: open
assignee: 
blocked_by: ticket-01-language-detection.md, ticket-02-session-context.md
---
## Question

How should the AGENTS.md prompt generation (`build-agent-prompt.js`) be updated to incorporate language detection and session context rules?

**Current state**: `build-agent-prompt.js` generates a static AGENTS.md with:
- Account→role mapping table
- 7 core rules (identity, permissions, knowledge boundaries, style, security, identity secrecy)
- Rule #5 mentions language but is weak: "用客户的语言回复（中文问→中文答，英文问→英文答）"
- No session/context awareness rules

**Scope**: This ticket depends on the outcomes of ticket-01 (language detection) and ticket-02 (session context). Once those decisions are made, this ticket implements the prompt changes:
1. Add explicit language detection and response rules
2. Add session context awareness rules
3. Ensure rules don't conflict with existing permission isolation

**Constraints**:
- Must not break existing permission isolation (rule #2)
- Must not break identity secrecy (rule #7)
- The prompt must remain effective within token limits
