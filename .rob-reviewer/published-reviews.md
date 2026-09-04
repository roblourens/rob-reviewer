# Published performance reviews

Newest publications are listed first. This index retains the newest 200 publications; complete history remains in the run archive.

## [#334375 — Update Component Explorer packages](https://github.com/microsoft/vscode/pull/334375)

- Published comments: **1**
- Head: `57121e6a364d6c5faccf3ca8b1ae78ed23045d07`
- Completed: `2026-09-04T00:20:11Z`
- Run: [33820613710 attempt 1](runs/2026/09/04/33820613710-1/run.md)

### `package-lock.json:255`

**[Experimental performance review bot]**

**Severity: medium**

This lockfile refresh deletes every `libc` selector while retaining both GNU and musl packages as optional dependencies with identical Linux OS and CPU constraints, making both variants eligible in the lockfile's install tree.

A Linux x64 cache-miss `npm ci`—including the root install on component-fixtures and CSS-order-scan CI—downloads and unpacks both x64 variants for all three families before any build or fixture can start. Warm node\_modules-cache hits avoid the install, but fresh developer/CI installs pay the extra binary transfer and disk cost.

**Suggested fix:** Restore the deleted `libc` arrays (or regenerate the lockfile with the repository-supported npm version that preserves them) so only the host's GNU or musl artifact is fetched and unpacked.

(Written by Copilot)

## [#333859 — Surface extensions blocked by the marketplace](https://github.com/microsoft/vscode/pull/333859)

- Published comments: **1**
- Head: `65fb4147de857f64f743dbfbb3a3952d8cea8f56`
- Completed: `2026-09-03T20:07:09Z`
- Run: [33799327822 attempt 1](runs/2026/09/03/33799327822-1/run.md)

### `src/vs/platform/extensionManagement/common/extensionGalleryService.ts:1394`

**[Experimental performance review bot]**

**Severity: medium**

The added blocked bit makes getValidRawGalleryExtensionVersion reject a version-blocked latest resource result. getLatestGalleryExtension then returns NOT\_COMPATIBLE, getExtensionsUsingResourceApi retries the affected IDs through a latest-only query, and queryGalleryExtensions rejects the same versions again before issuing its all-versions query.

Periodic and manual extension update checks that encounter a version-blocked latest release wait for two serial Marketplace POSTs after the existing resource fetches. On high-latency links this can add multiple round-trip times before VS Code can determine the usable older version and schedule updates.

**Suggested fix:** Propagate a distinct latest-version-is-blocked fallback result and batch those IDs directly into an IncludeVersions query, skipping the intervening latest-only query while preserving selection of an older permitted version.

(Written by Copilot)

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
