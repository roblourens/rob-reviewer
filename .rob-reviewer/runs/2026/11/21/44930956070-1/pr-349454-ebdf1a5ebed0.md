# Performance review: Tab moves focus into the empty editor hint

[PR #349454](https://github.com/microsoft/vscode/pull/349454) at `ebdf1a5ebed09b6887052576e5f6299daea84a41`

## Findings

No high-confidence performance findings.

<details>
<summary>Reviewer scenario analysis</summary>

**Relevant issue families:** synchronous-ui-work, cleanup-lifecycle, repeated-work, cache-lifecycle, serialization-allocation, eager-work, boundary-fanout, unbounded-retention

### Scenario 1: Open or recreate a classic hint in an empty text editor

- Critical path: The existing eager contribution determines eligibility and constructs the content widget before the hint renders. The diff adds one context key, a cached keybinding lookup, and one disposable keyboard listener per existing link; DOM construction remains memoized by getDomNode().
- Scaling input: One widget per eligible editor, containing 2–4 fixed links. Existing eligibility and activated-agent checks can scale with decorations and agents, but their frequency is unchanged.
- Effective concurrency: Synchronous widget creation and listeners; no new task fan-out. Variant changes dispose the old widget before creating its replacement.
- Cache behavior: getDomNode() retains the rendered node for the widget lifetime. The global keybinding resolver is lazy and cached until bindings change; existing hint command lookups already create it before the new focus-command lookup. No new cache keys grow with documents.
- Mechanism confidence: High: constructor, memoized DOM creation, bounded link generation, keybinding resolver and disposeHint ownership were traced.
- Magnitude uncertainty: No profiling was performed; the number of simultaneously eligible empty editors varies.
- Expensive boundaries:
  - serialization at <code>src/vs/workbench/contrib/codeEditor/browser/emptyTextEditorHint/emptyTextEditorHintController.ts:logHintEvent -&gt; emptyTextEditorHintTelemetry.ts:logEmptyEditorHintEvent -&gt; platform/telemetry/common/telemetryService.ts:\_doLog</code>: Clean and forward the existing small 'shown' telemetry record to configured appenders.; cardinality: One record per widget creation, not per link or focus movement; telemetry opt-out, metered-connection gating and bounded initialization buffering still apply.
    - Introduced by diff: false; previous behavior: The same widget constructor already emitted the same shown record.; critical-path effect: Existing synchronous record preparation; no new awaited delivery is added.
- Verdict: No reportable regression. Added work is small and bounded; listeners and context reset are registered for disposal, and the contribution drops the widget reference.

### Scenario 2: Open, resize or refresh the experimental action-list hint

- Critical path: Existing row construction and layout must finish before display. layout() still performs its pre-existing width write and offsetHeight read. The new stored ActionBar reference and extra focus-command label lookup do not add layout calls or row rebuild events.
- Scaling input: 2–4 fixed action entries plus one separator per widget; existing agent/editor registry scans scale with those registries, not one boundary call per registered editor.
- Effective concurrency: Synchronous row refresh and layout on the existing configuration/agent/editor events. The context-change listener filters for chat.enabled, so changes to the new visibility key do not trigger additional row rebuilds.
- Cache behavior: Row refresh clears the DisposableStore and replaces the ActionBar; the new field retains only the current instance. The keybinding resolver is shared and cached. Alternate-editor storage reads use an initialized memory cache, including absent values.
- Mechanism confidence: High: constructor subscriptions, fixed group generation, row replacement/disposal, context filtering, layout and storage cache implementation were inspected.
- Magnitude uncertainty: Registry sizes and resize frequency are unmeasured; expensive pre-existing layout behavior is unchanged.
- Expensive boundaries:
  - database at <code>src/vs/workbench/contrib/codeEditor/browser/emptyTextEditorHint/emptyEditorHintReopen.ts:getPrimaryEditor -&gt; platform/storage/common/storage.ts:get -&gt; base/parts/storage/common/storage.ts:get</code>: Read the remembered alternate-editor identifier from profile storage.; cardinality: One in-memory Map lookup per row rebuild; zero database reads per rebuild after storage initialization.
    - Introduced by diff: false; previous behavior: Every row rebuild already called getPrimaryEditor and read the same cached preference.; critical-path effect: Synchronous cached read only; database initialization is not newly requested or awaited here.
  - serialization at <code>src/vs/workbench/contrib/codeEditor/browser/emptyTextEditorHint/emptyEditorHintActions.ts:constructor/\_logEvent</code>: Emit the existing shown telemetry record when the list fits.; cardinality: One record per fitting widget creation; row refreshes and keyboard focus do not newly emit shown records.
    - Introduced by diff: false; previous behavior: The same fitting-widget constructor emitted the same event.; critical-path effect: Existing telemetry preparation, without a new awaited boundary.
- Verdict: No reportable regression. No additional registry fan-out, hidden heavyweight allocation, forced-layout cycle or retained ActionBar history is introduced.

### Scenario 3: Tab into the hint, navigate links or rows, and return to the editor

- Critical path: Keybinding resolution selects the new command, which synchronously focuses the first existing link or ActionBar item. Arrow navigation changes focus among existing controls. Escape/Shift+Tab focuses the editor. If the standard contribution cannot focus a hint, the fallback triggers the existing tab editor command.
- Scaling input: One active editor per gesture; classic indexOf searches at most four links. ActionBar focus(0) targets one existing row rather than rebuilding the list.
- Effective concurrency: Normal single-threaded browser keyboard dispatch; no promises, subprocesses, filesystem, database, network or IPC operations are added to navigation. DOM focus can incur normal browser focus work but does not invoke the existing action handlers.
- Cache behavior: Navigation reuses the rendered DOM and current ActionBar and the cached keybinding resolver. No document/model load or external service read is initiated by the focus command.
- Mechanism confidence: High: command handler, classic keyboard handler, ActionBar focus/updateFocus, editor focus and notebook contribution fallback were inspected.
- Magnitude uncertainty: Browser focus costs depend on the workbench DOM, but the change does not establish repeated layout or collection-wide reconstruction.
- Expensive boundaries: none found
- Verdict: No actionable performance regression. The new synchronous work is the bounded work required for the requested focus transition.

### Scenario 4: Type from a focused hint, dismiss it, or switch editor models

- Critical path: The new hint-node keydown handler classifies the key and focuses the editor for character/dead-key/composition input. Existing model-content updates then determine hint eligibility and dispose it. Model/variant replacement still follows the existing disposal path.
- Scaling input: One classification and at most one editor-focus call per qualifying hint keydown; the listeners belong to one widget, not all documents. No input text is copied or serialized by the helper.
- Effective concurrency: Synchronous browser input and existing editor update events. The debounce listener and all newly added DOM listeners are registered in the widget DisposableStore.
- Cache behavior: No input cache or history is introduced. Widget DOM, link references, ActionBar and listeners are bounded by widget ownership; disposed widgets are removed from editor widget maps and the contribution clears its reference.
- Mechanism confidence: High: helper, caller registration, content/model subscriptions, widget disposal and CodeEditorWidget removal were traced.
- Magnitude uncertainty: IME/browser event timing is not profiled; correctness of character delivery is outside this performance-only pass.
- Expensive boundaries: none found
- Verdict: No reportable memory or repeated-work regression. There is no new unbounded retention, duplicated model update loop or expensive boundary on the changed input path.

### Scenario 5: Announce the classic hint when editor text regains focus

- Critical path: The existing editor-focus debounce waits 500 ms and verifies text focus, a nonempty label and the verbosity setting. Removing the permanently false visibility guard restores the intended status announcement; the announcement is not awaited before typing or focusing.
- Scaling input: At most one surviving callback per widget per 500 ms debounce window, gated by current text focus. The message is a fixed hint and keybinding label, not document content.
- Effective concurrency: One debounced callback per widget; no queued announcement history or concurrent external calls.
- Cache behavior: The hint label is constructed once per classic widget. status reuses two shared containers rather than accumulating nodes, and the debounce subscription is disposed with the widget.
- Mechanism confidence: High: removed guard, debounced subscription and concrete status DOM implementation were inspected.
- Magnitude uncertainty: Screen-reader and browser announcement costs vary, but no scaling or blocking mechanism is established.
- Expensive boundaries:
  - other at <code>src/vs/base/browser/ui/aria/aria.ts:status/insertMessage</code>: Replace text in one of two shared ARIA status DOM nodes and toggle its visibility.; cardinality: One bounded status insertion per qualifying debounced focus callback; two shared nodes are reused and message insertion is capped at 20,000 characters.
    - Introduced by diff: true; previous behavior: The unchanged isVisible field remained false, so the classic hint's focus callback never called status.; critical-path effect: Runs after the focus debounce, off the initial keyboard-focus critical path; no external I/O or synchronous geometry read is performed.
- Verdict: No reportable regression. Restoring a bounded, delayed announcement is intentional behavior, not evidence of excessive rendering or retained state.

### Scenario 6: Activate a hint action with Enter or Space instead of clicking

- Critical path: The classic keyboard handler calls the same link.click() handler once, and the action list uses its existing ActionBar execution. Only explicit activation invokes commands, language selection, editor reopening or configuration changes; focusing/navigation does not eagerly run them.
- Scaling input: One selected action and one editor per gesture. Reopen narrows tab selection to this editor before the existing grouped command, so its effective resource cardinality is one, not all selected tabs or registered editors.
- Effective concurrency: One action is dispatched per keyboard activation. Existing extension activation may parallelize over host managers, settings writes remain serialized, reopen narrows to one editor, and no new fan-out across links/rows/resources is added.
- Cache behavior: Extension activation uses cached per-event promises and resolved-event flags. Detection reuses its worker. Storage reads/writes retain existing cache and coalescing. Selected feature initialization remains on explicit activation rather than hint construction.
- Mechanism confidence: High for before/after equivalence: handlers, click dispatch, command activation caches, language action, single-editor reopen, configuration queue and storage coalescing were inspected.
- Magnitude uncertainty: Feature-specific extension initialization, remote host latency and telemetry transport differ by environment and were not executed; those costs and invocation counts are unchanged.
- Expensive boundaries:
  - IPC at <code>src/vs/workbench/services/commands/common/commandService.ts:executeCommand -&gt; extensions/common/abstractExtensionService.ts:\_activateByEvent -&gt; extensionHostManager.ts:\_activateByEvent</code>: Activate extensions interested in the selected command via extension-host proxies when required.; cardinality: At most one activation RPC per interested extension-host manager per cold activation event; manager promises coalesce repeated event requests and completed/uninteresting events avoid RPC. Not per action-list entry.
    - Introduced by diff: false; previous behavior: Mouse activation executed the same command with the same extension activation/cache behavior.; critical-path effect: Existing registered-command behavior may await cold activation once hosts are ready; warm execution bypasses it.
  - IPC at <code>src/vs/workbench/browser/parts/editor/editorStatus.ts:ChangeLanguageAction.run -&gt; services/languageDetection/browser/languageDetectionWorkerServiceImpl.ts:detectLanguage</code>: After a language is selected, optionally synchronize the resource to the language-detection worker and send one detection request when URI guessing does not suffice.; cardinality: At most one worker detection request for the selected resource; worker creation is lazy and reused. Auto-detect is not offered for the empty document.
    - Introduced by diff: false; previous behavior: The same language-picker command already performed this work after mouse selection.; critical-path effect: The existing post-selection detection is fire-and-forget and does not delay the language change.
  - filesystem at <code>src/vs/workbench/contrib/codeEditor/browser/emptyTextEditorHint/emptyTextEditorHintController.ts:disableHint and emptyEditorHintActions.ts:\_renderFooter -&gt; configuration/common/configurationEditing.ts:queueWriteConfiguration/save</code>: Persist the hidden-hint setting through the existing configuration editor and text-file save.; cardinality: One configuration update per explicit Don't Show Again activation; actual save occurs only when the edit changes content.
    - Introduced by diff: false; previous behavior: Mouse activation initiated the identical setting update and save path.; critical-path effect: The existing update promise is not awaited by the hint handler; configuration writes are serialized through its existing Queue.
  - database at <code>src/vs/workbench/contrib/codeEditor/browser/emptyTextEditorHint/emptyEditorHintReopen.ts:\_reopenAndRemember -&gt; base/parts/storage/common/storage.ts:set</code>: Remember the successful alternate-editor ID and eventually persist pending storage changes.; cardinality: One constant-key store attempt after a successful single-editor reopen; identical values return early and pending writes coalesce by key before a delayed flush.
    - Introduced by diff: false; previous behavior: The same successful mouse-triggered reopen already remembered the editor ID with the same storage lifecycle.; critical-path effect: Occurs after the existing reopen completes; persistence is deferred rather than newly awaited by the hint.
  - serialization at <code>src/vs/workbench/contrib/codeEditor/browser/emptyTextEditorHint/emptyTextEditorHintTelemetry.ts:logEmptyEditorHintEvent and emptyEditorHintActions.ts:\_runCommand</code>: Prepare the same small action/command telemetry records for existing appenders.; cardinality: One hint-action event per activation and the existing command event where applicable; no event is added per navigation key or list entry.
    - Introduced by diff: false; previous behavior: The corresponding mouse action emitted the same records.; critical-path effect: Existing record preparation; external appender delivery behavior is unchanged.
  - other at <code>src/vs/workbench/browser/parts/editor/editorCommands.ts:reopenEditorWith and contrib/inlineChat/browser/inlineChatController.ts:run</code>: Run the existing selected-editor resolution/replacement or inline-chat session creation, which can invoke feature-specific extension-backed initialization.; cardinality: One selected editor or one inline-chat session per explicit action, with unchanged downstream initialization. No list-wide parallel invocation is added.
    - Introduced by diff: false; previous behavior: Clicking the same hint action entered these same resolver/session paths.; critical-path effect: The same action-specific promises and ordering remain; the diff only adds a keyboard route to them.
- Verdict: No introduced performance regression. Newly accessible activation reuses existing expensive paths without increasing their per-action cardinality or moving them onto focus, render or ordinary typing.

**Summary:** Completed the performance-review pass for PR \#349454 (base 794308978b6d99a6268d7675f216867a78501e3f, head ebdf1a5ebed09b6887052576e5f6299daea84a41). Inspected the complete five-file inventory and every diff page, then traced focus handling, widget ownership, render invalidation, caches and action boundaries. No high-confidence diff-introduced performance regression or concrete performance-regression fix was established. Static read-only review; no runtime durations were measured. Supporting guide was requested but the tool truncated its oversized output; learned-regression guidance was read.

</details>

<details>
<summary>Review statistics</summary>

- Model: `gpt-6.1-sol`
- Actual model calls: `gpt-6.1-sol`
- Reasoning effort: `high`
- Actual reasoning effort: `high`
- Model API endpoint: `ws:/responses`
- Wall-clock time: 2m51.035s
- Model calls: 12
- Tokens: 515558 input, 7833 output, 523391 total; 1949 reasoning; 437443 cache read, 78079 cache write
- Aggregate model API time: 2m29.15s
- Model-returned tool calls: 60
- Copilot usage: 31734380000.000 nano-AI units; model billing multiplier: 1.000
- USD cost: unavailable; the Copilot SDK does not expose a dollar conversion.

</details>
