# Reviewer run 33791391499

- Status: **succeeded**
- Started: `2026-09-03T18:36:01Z`
- Completed: `2026-09-03T18:45:02Z`
- Reviewer commit: `4f5ceba4f22f9e0f94a36fd9d0f9ab0d6774fb09`
- Bootstrapped: 0
- Reviewed: 3
- Published: 2
- Deferred: 350
- Skipped: 400
- Regression cases: 0
- Unresolved regression fixes: 0

## Reviewed PRs

### [#334191 — Polish pinned session chat hierarchy connectors](https://github.com/microsoft/vscode/pull/334191)

- Head: `ba7e4f9c0f383c4eff7bccbc997d1d4d0a0be4e2`
- Findings: 0
- Published: false
- Reports: [pr-334191-ba7e4f9c0f38.json](pr-334191-ba7e4f9c0f38.json), [pr-334191-ba7e4f9c0f38.md](pr-334191-ba7e4f9c0f38.md)

### [#334255 — sessions: add Agents window layout telemetry](https://github.com/microsoft/vscode/pull/334255)

- Head: `c09239548ceacd7694e0bcc766bbba65a2784711`
- Findings: 1
- Published: true
- Reports: [pr-334255-c09239548cea.json](pr-334255-c09239548cea.json), [pr-334255-c09239548cea.md](pr-334255-c09239548cea.md)

#### Published comment at `src/vs/sessions/browser/workbench.ts:618`

**[Experimental performance review bot]**

**Severity: low**

The added line synchronously resolves the delayed ITelemetryService and logs an event immediately after Ready-phase contributions, before renderWorkbench, createWorkbenchLayout, layout, and restore.

Opening an Agents window, especially a cold web window with telemetry enabled, now begins telemetry initialization, a health-check request, and telemetry SDK loading before the UI is rendered. This adds synchronous startup work and lets telemetry traffic/module parsing contend with first-render work for a constant one-event benefit.

**Suggested fix:** Record the fixed layout from an AfterRestored/idle contribution (for example, the existing SessionsTelemetryContribution using IAgentWorkbenchLayoutService), or otherwise schedule this log after first render, so the event is preserved without forcing the delayed telemetry stack onto the startup critical path.

(Written by Copilot)

### [#334303 — Improve customization message UI and migration behavior](https://github.com/microsoft/vscode/pull/334303)

- Head: `81b114c55f544144a89ac8943b2640ce949d6b26`
- Findings: 1
- Published: true
- Reports: [pr-334303-81b114c55f54.json](pr-334303-81b114c55f54.json), [pr-334303-81b114c55f54.md](pr-334303-81b114c55f54.md)

#### Published comment at `src/vs/workbench/contrib/chat/browser/aiCustomization/customizationMigrationServiceImpl.ts:74`

**[Experimental performance review bot]**

**Severity: medium**

Summary mode now calls provideChatSessionCustomizations from computeMigrationHint. ChatService awaits that hint together with hooks/instructions before building and dispatching the agent request.

With Summary enabled and a cold cache—for example, the first chat after startup with several remote agent-host plugins—the user's first send waits for directory resolution/listing and per-file metadata reads across all plugins before the agent starts responding.

**Suggested fix:** Do not await full item-provider enumeration before dispatching the chat request. Reuse/coalesce an invalidatable provider-item snapshot that is warmed outside the send path, or publish the summary notification after agent dispatch; also pass the request cancellation token so abandoned sends stop outstanding scans.

(Written by Copilot)
