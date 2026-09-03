# Published performance reviews

Newest publications are listed first. This index retains the newest 200 publications; complete history remains in the run archive.

## [#334303 — Improve customization message UI and migration behavior](https://github.com/microsoft/vscode/pull/334303)

- Published comments: **1**
- Head: `81b114c55f544144a89ac8943b2640ce949d6b26`
- Completed: `2026-09-03T18:45:02Z`
- Run: [33791391499 attempt 1](runs/2026/09/03/33791391499-1/run.md)

### `src/vs/workbench/contrib/chat/browser/aiCustomization/customizationMigrationServiceImpl.ts:74`

**[Experimental performance review bot]**

**Severity: medium**

Summary mode now calls provideChatSessionCustomizations from computeMigrationHint. ChatService awaits that hint together with hooks/instructions before building and dispatching the agent request.

With Summary enabled and a cold cache—for example, the first chat after startup with several remote agent-host plugins—the user's first send waits for directory resolution/listing and per-file metadata reads across all plugins before the agent starts responding.

**Suggested fix:** Do not await full item-provider enumeration before dispatching the chat request. Reuse/coalesce an invalidatable provider-item snapshot that is warmed outside the send path, or publish the summary notification after agent dispatch; also pass the request cancellation token so abandoned sends stop outstanding scans.

(Written by Copilot)

## [#334255 — sessions: add Agents window layout telemetry](https://github.com/microsoft/vscode/pull/334255)

- Published comments: **1**
- Head: `c09239548ceacd7694e0bcc766bbba65a2784711`
- Completed: `2026-09-03T18:45:02Z`
- Run: [33791391499 attempt 1](runs/2026/09/03/33791391499-1/run.md)

### `src/vs/sessions/browser/workbench.ts:618`

**[Experimental performance review bot]**

**Severity: low**

The added line synchronously resolves the delayed ITelemetryService and logs an event immediately after Ready-phase contributions, before renderWorkbench, createWorkbenchLayout, layout, and restore.

Opening an Agents window, especially a cold web window with telemetry enabled, now begins telemetry initialization, a health-check request, and telemetry SDK loading before the UI is rendered. This adds synchronous startup work and lets telemetry traffic/module parsing contend with first-render work for a constant one-event benefit.

**Suggested fix:** Record the fixed layout from an AfterRestored/idle contribution (for example, the existing SessionsTelemetryContribution using IAgentWorkbenchLayoutService), or otherwise schedule this log after first render, so the event is preserved without forcing the delayed telemetry stack onto the startup critical path.

(Written by Copilot)

## [#332410 — agentHost: centralize session and chat catalog metadata](https://github.com/microsoft/vscode/pull/332410)

- Published comments: **1**
- Head: `d3fc54d2a1c84ab24af96fca972159e0b75a9b9b`
- Completed: `2026-09-03T17:30:02Z`
- Run: [33783823809 attempt 1](runs/2026/09/03/33783823809-1/run.md)

### `src/vs/platform/agentHost/node/chatContributions/sessionTitle/sessionTitleContribution.ts:77`

**[Experimental performance review bot]**

**Severity: high**

Every title hydration now stats and potentially opens/queries the chat-local SQLite database before accepting an already-restored title.

Opening a session with many restored peer chats now launches one filesystem probe and SQLite metadata read per chat concurrently, delaying restore and creating an I/O/open-handle burst; up to the catalog limit of 999 peers can be affected.

**Suggested fix:** Restore the cached-title fast path before chat-local database access, and import chat-local title changes through the catalog reconciliation path. If local verification must remain on restore, batch/limit it and only probe chats whose central title is missing or known stale.

(Written by Copilot)
