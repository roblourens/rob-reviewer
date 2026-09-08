# Reviewer run 34171267390

- Status: **succeeded**
- Started: `2026-09-07T23:49:24Z`
- Completed: `2026-09-08T00:06:10Z`
- Reviewer commit: `add9ed06110e74a6f47e8d38c1cb1acbbe1e60ab`
- Bootstrapped: 0
- Reviewed: 5
- Published: 3
- Deferred: 351
- Skipped: 404
- Regression cases: 0
- Unresolved regression fixes: 0

## Reviewed PRs

### [#334952 — layout: backport primary side bar width fix to 1.137](https://github.com/microsoft/vscode/pull/334952)

- Head: `3e96ff70f6db7e81f2193309c4575a67b859efb0`
- Findings: 0
- Published: false
- Reports: [pr-334952-3e96ff70f6db.json](pr-334952-3e96ff70f6db.json), [pr-334952-3e96ff70f6db.md](pr-334952-3e96ff70f6db.md)

### [#334962 — Agents - Open new sessions to the side with Alt-click](https://github.com/microsoft/vscode/pull/334962)

- Head: `9dde363734b7e9330ae221d6a2c5e8d748715674`
- Findings: 0
- Published: false
- Reports: [pr-334962-9dde363734b7.json](pr-334962-9dde363734b7.json), [pr-334962-9dde363734b7.md](pr-334962-9dde363734b7.md)

### [#332410 — agentHost: centralize session and chat catalog metadata](https://github.com/microsoft/vscode/pull/332410)

- Head: `12f279f126ae279b3a69347c4e1dec15300e012f`
- Findings: 1
- Published: true
- Reports: [pr-332410-12f279f126ae.json](pr-332410-12f279f126ae.json), [pr-332410-12f279f126ae.md](pr-332410-12f279f126ae.md)

#### Published comment at `src/vs/platform/agentHost/node/agentHostDatabase.ts:1281`

**[Experimental performance review bot]**

**Severity: medium**

AgentHostPeerChatStore now sends every single-chat upsert/remove through replaceSessionChatCatalog; that method deletes all rows and awaits one SQLite INSERT for every surviving/current chat, then the compatibility path still writes the legacy blob.

Creating or removing a peer chat, or changing one chat's model/provider data, can issue about 1,000 serialized INSERTs for a large session. Repeated chat creation accumulates quadratic writes and stalls the operation and other work sharing the orchestrator database.

**Suggested fix:** Keep the revision/CAS and legacy-mirror semantics, but add mutation-specific database operations: UPDATE one row for provider-data changes, INSERT one appended row for creation, and DELETE plus a set-based order adjustment for removal. Reserve full replacement for migration/reconciliation; alternatively execute a true bulk insert rather than one awaited statement per row.

(Written by Copilot)

### [#333196 — chat: add customization migration assessment telemetry](https://github.com/microsoft/vscode/pull/333196)

- Head: `8a35417c67d8dfce97b4a61ef90ed0d7df397b80`
- Findings: 2
- Published: true
- Reports: [pr-333196-8a35417c67d8.json](pr-333196-8a35417c67d8.json), [pr-333196-8a35417c67d8.md](pr-333196-8a35417c67d8.md)

#### Published comment at `src/vs/sessions/services/sessions/browser/sessionsService.ts:789`

**[Experimental performance review bot]**

**Severity: medium**

Every `openChat` now starts `computeMigrations` for the owning session, even when the session is already active and only its selected child chat changes.

For sessions with multiple chats, normal tab switching or reopening a side chat repeatedly recomputes the same session-wide assessment. Users with many MCP servers/customizations can see avoidable background CPU and extension-host traffic competing with chat navigation.

**Suggested fix:** Trigger the assessment only when navigation actually changes the active session (for example, compare the previously active session before `_activate`), while keeping the session-open telemetry path for genuine session opens; do not run it for child-chat selection within the same session.

(Written by Copilot)

#### Published comment at `src/vs/sessions/services/sessions/browser/sessionsService.ts:1614`

**[Experimental performance review bot]**

**Severity: medium**

After `restoreGrid`, all resolved sessions simultaneously start `computeMigrations`; late sessions do the same from `place`, with no queue or concurrency bound.

Restoring a window with several pinned sessions, especially across different workspaces, can produce an avoidable startup CPU/IPC spike and GC pressure while the restored UI is becoming interactive.

**Suggested fix:** Feed restore telemetry assessments through a small concurrency limiter or idle queue with restore-token cancellation; coalesce only assessments proven to share the same session/resource key, while still emitting the required per-session aggregate telemetry.

(Written by Copilot)

### [#334369 — Add MCP customization migration](https://github.com/microsoft/vscode/pull/334369)

- Head: `b65e332fb2ba19b92bdd889dc326215274392e47`
- Findings: 1
- Published: true
- Reports: [pr-334369-b65e332fb2ba.json](pr-334369-b65e332fb2ba.json), [pr-334369-b65e332fb2ba.md](pr-334369-b65e332fb2ba.md)

#### Published comment at `src/vs/workbench/contrib/chat/browser/aiCustomization/mcpServerCustomizationMigration.ts:224`

**[Experimental performance review bot]**

**Severity: medium**

The new execution loop invokes `findCrossRootConflicts` for every migration group; that helper serially reads both MCP configuration locations in every other workspace root, and the same full-root scan is invoked again after the target write and after verification.

Confirming migration for servers spread across a 10-root workspace causes roughly 540 cross-root conflict reads before counting source/target verification and write guards, and the nested loops await them serially. On remote filesystems this can turn a one-click migration into a long-running operation.

**Suggested fix:** Build a conflict index by reading each root's source and target once per validation phase, then check all groups against that index. Preserve concurrency safety by refreshing only roots whose files may have changed after each write (or track etag/mtime and re-read changed documents), rather than rescanning every file for every group.

(Written by Copilot)
