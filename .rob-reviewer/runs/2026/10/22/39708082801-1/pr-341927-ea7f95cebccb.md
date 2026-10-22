# Performance review: automations: group disabled automations in a collapsed section

[PR #341927](https://github.com/microsoft/vscode/pull/341927) at `ea7f95cebccb797af928f9705bb3029553aac298`

## Findings

No high-confidence performance findings.

<details>
<summary>Reviewer scenario analysis</summary>

**Relevant issue families:** synchronous-ui-work, repeated-work, eager-work, cleanup-lifecycle

### Scenario 1: Open the Automations view and receive initial or periodic catalogue refreshes

- Critical path: View construction synchronously creates the existing card UI plus one small hidden details section and reads one cached profile boolean. The pre-existing asynchronous automationService.refresh must complete before fresh provider data appears, after which the autorun performs the local DOM render; the remote refresh is not introduced or moved by this diff.
- Scaling input: n catalogue automations and t templates. The changed render adds two linear filters/arrays, then still updates or creates each automation card once. persistentCards reuses cards by automation ID on warm renders; cold renders create one card per automation. Effective cardinality remains n rather than multiplying by groups.
- Effective concurrency: One autorun render executes synchronously per observable transaction. The existing refresh scheduler requests one background refresh on open and schedules the next after completion; the diff adds no parallel work, limiter bypass, or burst.
- Cache behavior: Cold view creation renders all cards and reads the expansion preference from initialized storage. Warm catalogue updates reuse persistentCards and latestAutomations; there is no new cache miss path or negative-cache behavior. Collapsing disabled cards does not lazily avoid card construction, but those cards were already constructed before the PR, so this is not a regression.
- Mechanism confidence: High: the autorun, refresh scheduling, storage API, persistent card map, and changed loops are explicit in the traced code.
- Magnitude uncertainty: Exact automation counts and provider refresh latency vary, but neither affects causality: remote refresh cost is unchanged and new local work is bounded linear filtering plus constant section setup.
- Expensive boundaries:
  - network/IPC at <code>src/vs/sessions/contrib/sessions/browser/views/automationsView.ts:2680 refreshAutomations</code>: Refresh automation providers through automationService.refresh(options).; cardinality: One logical refresh on initial view construction and then one scheduled refresh after each 60-second interval/completion; provider internals may fan out, but the diff does not change that grouping.
    - Introduced by diff: false; previous behavior: The same initial and periodic refresh call existed before the PR with the same scheduling and call count.; critical-path effect: Initial/fresh catalogue data waits for the asynchronous provider refresh, while the view can render current observable state beforehand.
  - database at <code>src/vs/sessions/contrib/sessions/browser/views/automationsView.ts:419 AutomationCardsSection constructor</code>: Read the disabled-section expansion boolean from profile storage.; cardinality: One cached scalar read per widget construction, plus one cached read if an external change event arrives.
    - Introduced by diff: true; previous behavior: No disabled-section preference was read because the section did not exist.; critical-path effect: The in-memory storage lookup occurs synchronously during local DOM construction; it does not initiate a per-automation boundary call.
- Verdict: No reportable regression. The diff adds small O(n) array filtering and constant DOM/storage setup while preserving one card update per automation and all pre-existing provider-boundary cardinality.

### Scenario 2: Create or edit an automation so it moves into or out of the disabled section

- Critical path: The existing dialog commit awaits automationService.createAutomation or updateAutomationIfUnchanged. Once the service publishes the changed catalogue, the render autorun partitions the list, reuses the persistent card, reparents it locally, and repairs focus. The diff does not add a service, filesystem, or network operation to the commit path.
- Scaling input: One user-selected automation changes state; the subsequent catalogue render scans n automations and may shift/reparent DOM siblings after the moved card. The service call cardinality remains one per committed create/update.
- Effective concurrency: A dialog issues one awaited commit. The resulting autorun is synchronous and serial; there is no new Promise fan-out or concurrent provider burst.
- Cache behavior: The card is reused from persistentCards when editing an existing automation; creation has the same cold card construction as before. No new cache keys, retries, or miss amplification are added.
- Mechanism confidence: High: the dialog commit paths, catalogue autorun, persistentCards reuse, and new renderCardsInto placement are directly visible.
- Magnitude uncertainty: The number of automations controls the local O(n) rerender, but enable/disable commits are user-driven and the PR does not increase service calls or card-update cardinality.
- Expensive boundaries:
  - network/IPC at <code>src/vs/sessions/contrib/sessions/browser/views/automationsView.ts:1227 openCreateDialog and :1324 openEditDialog</code>: Persist one automation create or update through IAutomationService.; cardinality: One service operation per user commit.
    - Introduced by diff: false; previous behavior: Create/update already awaited the same automation service operation; the diff only changes the subsequent local placement and focus handling.; critical-path effect: The dialog commit waits for persistence before completion and catalogue publication, as it did before.
- Verdict: No reportable regression. Moving a card between grids adds bounded local DOM reparenting on an infrequent user commit without adding expensive-boundary work or concurrency.

### Scenario 3: Expand or collapse the Disabled automations disclosure

- Critical path: The browser toggles the native details element immediately. The handler compares one cached boolean and stores one scalar profile value when it changed; persistence is buffered by the storage layer and is not multiplied by automation count.
- Scaling input: One operation per explicit user toggle, independent of the number of disabled automations.
- Effective concurrency: User toggles are handled serially on the UI thread. Storage notifications set the same details.open state and do not create a write loop; no fan-out or parallel boundary burst is introduced.
- Cache behavior: getBoolean reads initialized in-memory storage. store updates the in-memory Storage object and the browser implementation batches underlying IndexedDB persistence on its flush interval; same-value feedback is suppressed by the comparison.
- Mechanism confidence: High: the toggle comparison, single store call, storage implementation, and event feedback path are explicit.
- Magnitude uncertainty: Desktop and web storage backends differ, but the operation remains one tiny user-triggered value rather than a scaling or hot-path write.
- Expensive boundaries:
  - database at <code>src/vs/sessions/contrib/sessions/browser/views/automationsView.ts:424 toggle listener</code>: Persist the boolean disabled-section expansion preference in profile storage.; cardinality: At most one logical scalar storage update per state-changing user toggle, independent of catalogue size; underlying browser writes are buffered/coalesced by storage flushing.
    - Introduced by diff: true; previous behavior: There was no disabled disclosure and therefore no expansion-state write.; critical-path effect: Only the in-memory comparison/update is in the toggle handler; durable persistence is buffered and does not block section rendering on an awaited operation.
- Verdict: No reportable regression. The introduced persistence has constant cardinality, is user-triggered, and is buffered; it does not establish meaningful latency, I/O, or throughput degradation.

### Scenario 4: Open Automations accessibility help

- Critical path: The existing accessibility-help builder creates its content array synchronously; the diff adds one localized constant string. No external work must complete.
- Scaling input: Constant work per accessibility-help invocation, independent of automation count.
- Effective concurrency: Single synchronous invocation; no queue, parallelism, or backpressure behavior changes.
- Cache behavior: No changed cache or retained collection is involved.
- Mechanism confidence: High: the entire changed hunk is a constant localize call appended to the existing array.
- Magnitude uncertainty: None material; the added allocation is a single bounded string entry.
- Expensive boundaries: none found
- Verdict: No reportable regression. The change adds only constant, bounded local string content.

**Summary:** Completed the performance pass over all four changed files and all diff pages. Traced view construction/refresh, catalogue rendering, enable-disable transitions, disclosure persistence, focus repair, accessibility help, cleanup ownership, and all changed or transitively reached storage and automation-service boundaries. No high-confidence actionable performance regression or qualifying regression fix was found.

</details>

<details>
<summary>Review statistics</summary>

- Model: `gpt-5.6-sol`
- Actual model calls: `gpt-5.6-sol`
- Reasoning effort: `high`
- Actual reasoning effort: `high`
- Model API endpoint: `ws:/responses`
- Wall-clock time: 1m17.504s
- Model calls: 8
- Tokens: 215149 input, 5419 output, 220568 total; 2162 reasoning; 170211 cache read, 44914 cache write
- Aggregate model API time: 1m2.788s
- Model-returned tool calls: 35
- Copilot usage: 40113040000.000 nano-AI units; model billing multiplier: 1.000
- USD cost: unavailable; the Copilot SDK does not expose a dollar conversion.

</details>
