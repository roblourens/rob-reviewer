# Reviewer run 33897628556

- Status: **succeeded**
- Started: `2026-09-04T16:54:31Z`
- Completed: `2026-09-04T17:05:45Z`
- Reviewer commit: `38a5c3528405796e5d5882a3c37571199b7181e0`
- Bootstrapped: 0
- Reviewed: 3
- Published: 1
- Deferred: 343
- Skipped: 393
- Regression cases: 0
- Unresolved regression fixes: 0

## Reviewed PRs

### [#332410 — agentHost: centralize session and chat catalog metadata](https://github.com/microsoft/vscode/pull/332410)

- Head: `0f605424902269dc6acd45bcbfc679dbcb014126`
- Findings: 1
- Published: true
- Reports: [pr-332410-0f6054249022.json](pr-332410-0f6054249022.json), [pr-332410-0f6054249022.md](pr-332410-0f6054249022.md)

#### Published comment at `src/vs/platform/agentHost/node/agentHostPeerChatStore.ts:339`

**[Experimental performance review bot]**

**Severity: medium**

\_publishCompatibilityState now maps the complete current entry list to \_writeChatMetadata before writing the parent legacy mirror, and createChat/disposeChat synchronously await peerChatStore.upsert/remove.

In a long-lived session with many peer chats, adding or deleting one chat blocks on potentially hundreds of unrelated SQLite opens/writes (250 four-wide waves at the 1,000-chat limit). Repeated additions accumulate roughly N(N+1)/2 writes, causing increasingly slow chat operations and disk churn.

**Suggested fix:** Diff the previous and updated central catalogs and write chat-local metadata only for added or actually changed entries, then write the parent peerChats mirror once. Reserve full per-chat repair sweeps for migration/background reconciliation rather than the interactive mutation path.

(Written by Copilot)

### [#334411 — mcp: Activate lazy providers in Agent Customizations](https://github.com/microsoft/vscode/pull/334411)

- Head: `88b083ee11209e3606c511f688ba28a768a9dacf`
- Findings: 0
- Published: false
- Reports: [pr-334411-88b083ee1120.json](pr-334411-88b083ee1120.json), [pr-334411-88b083ee1120.md](pr-334411-88b083ee1120.md)

### [#334412 — agentHost: Resolve published MCP server lifecycle IDs](https://github.com/microsoft/vscode/pull/334412)

- Head: `1e85eb8b75a46e07e0adcb1cc9eda2d9f1454523`
- Findings: 0
- Published: false
- Reports: [pr-334412-1e85eb8b75a4.json](pr-334412-1e85eb8b75a4.json), [pr-334412-1e85eb8b75a4.md](pr-334412-1e85eb8b75a4.md)
