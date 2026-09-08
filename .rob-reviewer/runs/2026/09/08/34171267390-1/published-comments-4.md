# Published comments: 4

## [#332410 — agentHost: centralize session and chat catalog metadata](https://github.com/microsoft/vscode/pull/332410) at `src/vs/platform/agentHost/node/agentHostDatabase.ts:1281`

**[Experimental performance review bot]**

**Severity: medium**

AgentHostPeerChatStore now sends every single-chat upsert/remove through replaceSessionChatCatalog; that method deletes all rows and awaits one SQLite INSERT for every surviving/current chat, then the compatibility path still writes the legacy blob.

Creating or removing a peer chat, or changing one chat's model/provider data, can issue about 1,000 serialized INSERTs for a large session. Repeated chat creation accumulates quadratic writes and stalls the operation and other work sharing the orchestrator database.

**Suggested fix:** Keep the revision/CAS and legacy-mirror semantics, but add mutation-specific database operations: UPDATE one row for provider-data changes, INSERT one appended row for creation, and DELETE plus a set-based order adjustment for removal. Reserve full replacement for migration/reconciliation; alternatively execute a true bulk insert rather than one awaited statement per row.

(Written by Copilot)

## [#333196 — chat: add customization migration assessment telemetry](https://github.com/microsoft/vscode/pull/333196) at `src/vs/sessions/services/sessions/browser/sessionsService.ts:789`

**[Experimental performance review bot]**

**Severity: medium**

Every `openChat` now starts `computeMigrations` for the owning session, even when the session is already active and only its selected child chat changes.

For sessions with multiple chats, normal tab switching or reopening a side chat repeatedly recomputes the same session-wide assessment. Users with many MCP servers/customizations can see avoidable background CPU and extension-host traffic competing with chat navigation.

**Suggested fix:** Trigger the assessment only when navigation actually changes the active session (for example, compare the previously active session before `_activate`), while keeping the session-open telemetry path for genuine session opens; do not run it for child-chat selection within the same session.

(Written by Copilot)

## [#333196 — chat: add customization migration assessment telemetry](https://github.com/microsoft/vscode/pull/333196) at `src/vs/sessions/services/sessions/browser/sessionsService.ts:1614`

**[Experimental performance review bot]**

**Severity: medium**

After `restoreGrid`, all resolved sessions simultaneously start `computeMigrations`; late sessions do the same from `place`, with no queue or concurrency bound.

Restoring a window with several pinned sessions, especially across different workspaces, can produce an avoidable startup CPU/IPC spike and GC pressure while the restored UI is becoming interactive.

**Suggested fix:** Feed restore telemetry assessments through a small concurrency limiter or idle queue with restore-token cancellation; coalesce only assessments proven to share the same session/resource key, while still emitting the required per-session aggregate telemetry.

(Written by Copilot)

## [#334369 — Add MCP customization migration](https://github.com/microsoft/vscode/pull/334369) at `src/vs/workbench/contrib/chat/browser/aiCustomization/mcpServerCustomizationMigration.ts:224`

**[Experimental performance review bot]**

**Severity: medium**

The new execution loop invokes `findCrossRootConflicts` for every migration group; that helper serially reads both MCP configuration locations in every other workspace root, and the same full-root scan is invoked again after the target write and after verification.

Confirming migration for servers spread across a 10-root workspace causes roughly 540 cross-root conflict reads before counting source/target verification and write guards, and the nested loops await them serially. On remote filesystems this can turn a one-click migration into a long-running operation.

**Suggested fix:** Build a conflict index by reading each root's source and target once per validation phase, then check all groups against that index. Preserve concurrency safety by refreshing only roots whose files may have changed after each write (or track etag/mtime and re-read changed documents), rather than rescanning every file for every group.

(Written by Copilot)

