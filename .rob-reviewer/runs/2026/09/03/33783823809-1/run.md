# Reviewer run 33783823809

- Status: **succeeded**
- Started: `2026-09-03T17:19:43Z`
- Completed: `2026-09-03T17:30:02Z`
- Reviewer commit: `c11f6045526f3893d90ec50f681226b811e07028`
- Bootstrapped: 0
- Reviewed: 2
- Published: 1
- Deferred: 347
- Skipped: 402
- Regression cases: 0
- Unresolved regression fixes: 1

## Reviewed PRs

### [#330955 — Add transient UI for /btw side questions](https://github.com/microsoft/vscode/pull/330955)

- Head: `6a4fe0a2a16349d326ea406aa874873d6220b665`
- Findings: 0
- Published: false
- Reports: [pr-330955-6a4fe0a2a163.json](pr-330955-6a4fe0a2a163.json), [pr-330955-6a4fe0a2a163.md](pr-330955-6a4fe0a2a163.md)

### [#332410 — agentHost: centralize session and chat catalog metadata](https://github.com/microsoft/vscode/pull/332410)

- Head: `d3fc54d2a1c84ab24af96fca972159e0b75a9b9b`
- Findings: 1
- Published: true
- Reports: [pr-332410-d3fc54d2a1c8.json](pr-332410-d3fc54d2a1c8.json), [pr-332410-d3fc54d2a1c8.md](pr-332410-d3fc54d2a1c8.md)

#### Published comment at `src/vs/platform/agentHost/node/chatContributions/sessionTitle/sessionTitleContribution.ts:77`

**[Experimental performance review bot]**

**Severity: high**

Every title hydration now stats and potentially opens/queries the chat-local SQLite database before accepting an already-restored title.

Opening a session with many restored peer chats now launches one filesystem probe and SQLite metadata read per chat concurrently, delaying restore and creating an I/O/open-handle burst; up to the catalog limit of 999 peers can be affected.

**Suggested fix:** Restore the cached-title fast path before chat-local database access, and import chat-local title changes through the catalog reconciliation path. If local verification must remain on restore, batch/limit it and only probe chats whose central title is missing or known stale.

(Written by Copilot)
