# Performance review: Simplify state indicators in MCP customization page

[PR #337402](https://github.com/microsoft/vscode/pull/337402) at `e9bbeefa1a1ee32112d0bca652a4c3b1d0547330`

## Findings

No high-confidence performance findings.

<details>
<summary>Reviewer scenario analysis</summary>

**Relevant issue families:** synchronous-ui-work, repeated-work, eager-work, boundary-fanout, cleanup-lifecycle, concurrency-burst

### Scenario 1: Open or scroll the MCP customization tree and render installed server rows

- Critical path: The widget groups in-memory installed/gallery entries, updates the WorkbenchObjectTree, and synchronously renders only viewport-backed row templates. Added compatibility text/icons and state controls are on this UI critical path; the alternate card-row helper is not called by production code in this file.
- Scaling input: Total installed and marketplace entries determine tree model size, while DOM/control creation is bounded by the virtualized viewport. Compatibility parsing/link creation applies only to visible rows with a non-supported compatibility presentation.
- Effective concurrency: Single-threaded synchronous DOM rendering; tree virtualization recycles a bounded template set. No async fan-out is started by row rendering.
- Cache behavior: Compatibility state is read from an in-memory observable map. Tree templates, hovers, and disposable stores are recycled; no cold/warm external cache changes the path.
- Mechanism confidence: High: the renderer lifecycle, signature guard, template disposal, and production tree setup were traced directly.
- Magnitude uncertainty: Exact installed-server counts and compatibility-row prevalence vary, but active DOM cardinality remains viewport-bounded.
- Expensive boundaries: none found
- Verdict: No reportable regression. The added DOM work is bounded to visible rows and disposed on recycling; no subprocess, IPC, filesystem, database, or network boundary was added to rendering.

### Scenario 2: Update MCP status, enablement, compatibility, or active-session customization while the list is visible

- Critical path: Observable status changes and the customization event synchronously update visible row presentation. Action DOM is rebuilt only when the status signature changes, while compatibility message content is refreshed for visible templates.
- Scaling input: Work scales with the virtualized visible template count for broad customization events, not the full installed collection; formatted compatibility content is a short fixed string and only creates a link for affected rows.
- Effective concurrency: Updates run synchronously on the UI thread and are serialized by the event/observable delivery path; no background task or external operation is launched.
- Cache behavior: The observable compatibility map is reused. Status signatures suppress action reconstruction when state is unchanged; compatibility message listeners are replaced through bounded DisposableStores.
- Mechanism confidence: High: both local autorun and global customization-event paths were inspected through updateStatus and their disposal ownership.
- Magnitude uncertainty: Event frequency depends on active MCP state transitions, but normal transitions are sparse and template count is viewport-bounded.
- Expensive boundaries: none found
- Verdict: No actionable performance finding. The richer compatibility refresh adds small repeated DOM work but remains bounded and does not establish realistic jank, unbounded retention, or boundary fan-out.

### Scenario 3: Start a stopped MCP server from the new inline Start button

- Critical path: A click resolves the current row entry and awaits exactly one server start operation. The clicked button is disabled until completion; server startup itself is the critical path.
- Scaling input: One backend start per user click and selected server. Users can initiate different rows independently, but there is no collection loop or automatic burst.
- Effective concurrency: At most one in-flight start through a given button because it is disabled around the awaited promise; there is no new shared limiter across different servers, matching existing lifecycle actions.
- Cache behavior: No UI cache is involved. Local server start is already idempotent in the MCP service path; failed or completed starts are reflected by authoritative status updates.
- Mechanism confidence: High: the click handler, awaited call, per-button disabling, and local/active-session branches were traced.
- Magnitude uncertainty: Startup duration varies by server command and transport, but the diff does not add extra starts or collection-wide work.
- Expensive boundaries:
  - subprocess/network at <code>src/vs/workbench/contrib/chat/browser/aiCustomization/mcpListWidget.ts:startMcpEntry</code>: Start the selected local MCP server, spawning its command or connecting its configured transport.; cardinality: Exactly one operation for the clicked server; no grouping or collection fan-out.
    - Introduced by diff: true; previous behavior: The same localServer.start operation was available through existing MCP lifecycle/context-menu actions; the PR adds an inline route without increasing automatic call count.; critical-path effect: Awaited by the inline button action and required for that server to progress from Stopped.
  - IPC at <code>src/vs/workbench/contrib/chat/browser/agentSessions/agentHost/agentHostCustomizationService.ts:getMcpServers start callback</code>: Request the active agent host to start the selected session MCP server via target.startMcpServer.; cardinality: Exactly one request for the clicked active-session server.
    - Introduced by diff: true; previous behavior: Existing lifecycle actions already exposed a one-server start request; the diff changes discoverability, not backend multiplicity.; critical-path effect: Awaited by the inline Start action.
- Verdict: No performance regression. This is intentional user-triggered work with one boundary call and existing lifecycle semantics.

### Scenario 4: Open MCP output from an error row using the new inline Show Output button

- Critical path: A click resolves the current server and invokes one existing output handler. Local servers reveal their output; active-session servers register/reuse diagnostic output state and reveal the selected channel.
- Scaling input: One selected server per click. Active-session first use records the current session's server states in memory, but the same handler and session-wide recording already backed the context-menu Show Output action.
- Effective concurrency: User-triggered promise per click with no collection-wide async fan-out. Output-channel reveal is handled by the existing output service/registry.
- Cache behavior: The active-session log registry tracks sessions and deduplicates recorded server state signatures; subsequent shows reuse registered channels. Local output uses the existing server output channel.
- Mechanism confidence: High: inline routing was compared with getMcpServerActions and both converge on getMcpServerOutputHandler.
- Magnitude uncertainty: First-time diagnostics setup cost depends on session server count, but the mechanism and cardinality are unchanged from the existing action.
- Expensive boundaries:
  - IPC at <code>src/vs/workbench/contrib/chat/browser/agentSessions/agentHost/agentHostCustomizationService.ts:showMcpServerLog</code>: Attach/reveal active agent-host MCP diagnostics for the selected server's output channel.; cardinality: One selected output request per click; session diagnostics registration iterates that session's in-memory MCP server states and uses deduplicated registry records.
    - Introduced by diff: true; previous behavior: The context-menu Show Output action already called the same handler with the same selected-server cardinality; the PR adds an inline entry point.; critical-path effect: Required to reveal active-session output after the user clicks Show Output.
  - other at <code>src/vs/workbench/contrib/chat/browser/aiCustomization/mcpListWidget.ts:getMcpServerOutputHandler</code>: Reveal the selected local server or output-service channel.; cardinality: One channel reveal per click.
    - Introduced by diff: true; previous behavior: Identical output routing existed in the row context menu before the diff.; critical-path effect: Directly completes the requested output navigation.
- Verdict: No reportable regression. The new button does not duplicate output work or amplify boundary cardinality.

### Scenario 5: Open MCP Migrations from a compatibility message

- Critical path: The compatibility link fires one editor event and opens the already-enabled MCP migration category. Because a category ID is supplied, this path renders the migration page directly rather than refreshing migration discovery first.
- Scaling input: One event and one migration-page render per explicit click; no per-server loop or automatic navigation.
- Effective concurrency: Synchronous editor state/DOM update followed by a requestAnimationFrame focus; no external operation is launched on this category-specific route.
- Cache behavior: Uses current editor migration state; no cold/warm network, filesystem, or database cache participates in opening the category.
- Mechanism confidence: High: the event wiring and category-specific startCustomizationMigration/showCustomizationMigrationPage branch were traced.
- Magnitude uncertainty: Migration page DOM size depends on existing migration data, but the link neither recomputes discovery nor introduces an external boundary.
- Expensive boundaries: none found
- Verdict: No performance regression. The action is single-shot, user-triggered UI navigation with no new expensive boundary.

**Summary:** Completed the performance pass over all eight changed files and all diff pages. Traced virtualized rendering, status-update lifecycle/disposal, and the Start, Show Output, and Migrations action boundaries. No high-confidence diff-introduced performance issue or concrete regression fix was found.

</details>

<details>
<summary>Review statistics</summary>

- Model: `gpt-5.6-sol`
- Actual model calls: `gpt-5.6-sol`
- Reasoning effort: `high`
- Actual reasoning effort: `high`
- Model API endpoint: `ws:/responses`
- Wall-clock time: 2m20.512s
- Model calls: 10
- Tokens: 497236 input, 7196 output, 504432 total; 2808 reasoning; 418329 cache read, 78877 cache write
- Aggregate model API time: 2m3.851s
- Model-returned tool calls: 55
- Copilot usage: 70575660000.000 nano-AI units; model billing multiplier: 1.000
- USD cost: unavailable; the Copilot SDK does not expose a dollar conversion.

</details>
