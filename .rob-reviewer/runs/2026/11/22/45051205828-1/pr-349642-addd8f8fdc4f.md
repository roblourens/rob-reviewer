# Performance review: show slight color when side chat is not focused

[PR #349642](https://github.com/microsoft/vscode/pull/349642) at `addd8f8fdc4fd20a00b27dba602e22b3fd87c3fc`

## Findings

No high-confidence performance findings.

<details>
<summary>Reviewer scenario analysis</summary>

**Relevant issue families:** synchronous-ui-work, repeated-work, serialization-allocation

### Scenario 1: Hovering or keyboard-focusing an inactive session or split chat, then leaving it

- Critical path: Existing hover/focus selectors and active-group class changes cause browser style resolution and paint. The request fill and border now join the existing 100 ms reveal animation; no asynchronous operation must finish before interaction can proceed.
- Scaling input: Rendered descendant DOM in the affected chat lists, including request markdown and origin chips. ListView normally renders the viewport range rather than the entire history; a current text selection can extend that range.
- Effective concurrency: Browser-managed transitions can run simultaneously on affected lists. No new task queue, request fan-out, or persistent animation loop; unchanged motion gating disables transitions under reduced motion.
- Cache behavior: No application cache is added or invalidated. Cold and warm interaction use the same selectors; existing render-row reuse is unchanged.
- Mechanism confidence: High confidence in the added transition and paint operations from the CSS and endpoint/intermediate-state assertions. No evidence establishes a substantial new rendering bottleneck beyond the existing animated subtree work.
- Magnitude uncertainty: Actual rendering time depends on rendered subtree complexity and hardware; no runtime profiling was available.
- Expensive boundaries:
  - other at <code>src/vs/workbench/contrib/sessions/browser/parts/media/sessionView.css:41-42,124-163</code>: Browser style interpolation and painting of request background colors and zero-blur inset outlines; cardinality: Two additional custom-property transition tracks per affected list for 100 ms; outline painting applies to matching rendered request bubbles and origin chips. Existing request background and response opacity interpolation already ran over the same subtree.
    - Introduced by diff: true; previous behavior: The list already animated opacity and an inherited bubble-background property for 100 ms. Request background colors already changed each animation frame; request outlines were absent.; critical-path effect: Visual reveal occurs during browser rendering, without new script-driven geometry reads or changes to row dimensions.
- Verdict: No reportable high-confidence regression. The change adds bounded visual work to an already animated path without new layout feedback, blurred effects, or lifetime growth.

### Scenario 2: Displaying and scrolling unfocused chat transcripts, including partially visible sticky requests and request-origin chips

- Critical path: Existing virtualized tree rendering inserts and removes rows while scrolling. The CSS adds a size-preserving outline to requests and origins, with three nonblurred inset strokes on a partially visible sticky request. Optional origin-quote loading remains asynchronous and off the transcript's initial display path.
- Scaling input: Request bubbles and origin chips in the current render range; stickyScrollMaxItemCount is 1. Source loading is attempted only for origins lacking a supplied selection and an already loaded source model.
- Effective concurrency: Existing scroll rendering stays on the UI thread. Local source reads are serialized by storeQueue; remote loads coalesce per key, not globally. All asynchronous source-loading scheduling is unchanged.
- Cache behavior: Viewport row reuse is unchanged. Origins use a supplied selection or existing model before loading. Remote loads coalesce by resource and settled entries are removed; local reads use the existing store queue. The diff changes no positive/negative cache behavior or retention.
- Mechanism confidence: High confidence from ListView render-range insertion/removal, sticky-row limits, and the traced origin-resolution path that the diff neither increases session-loading cardinality nor changes row geometry.
- Magnitude uncertainty: Paint magnitude depends on visible request complexity and selection-expanded render ranges. Provider-specific load duration is irrelevant to causality because loading is unchanged.
- Expensive boundaries:
  - other at <code>src/vs/workbench/contrib/sessions/browser/parts/media/sessionView.css:153-163; src/vs/workbench/contrib/chat/browser/widget/chatListWidget.ts:650-667</code>: Paint request outlines in normal and sticky tree rows; cardinality: One outline per matching rendered bubble/origin, with three strokes on at most one sticky row per chat tree; not one operation per historical turn.
    - Introduced by diff: true; previous behavior: The same virtualized rows and sticky request were already rendered with request backgrounds but without the new outline.; critical-path effect: Adds paint, but not geometry changes that would independently trigger dynamic-height relayout.
  - filesystem at <code>src/vs/workbench/contrib/chat/browser/widget/chatContentParts/chatRequestOriginPart.ts:130-145; src/vs/workbench/contrib/chat/common/chatService/chatServiceImpl.ts:787-802; src/vs/workbench/contrib/chat/common/model/chatSessionStore.ts:635-680</code>: Existing optional local source-session restoration reads a log file, with flat-file or previous-location fallback; cardinality: Zero reads for a supplied selection or loaded source model. Each eligible local cache-miss restoration reads the log or flat representation; fallback can add reads. The PR does not change the number of restorations.
    - Introduced by diff: false; previous behavior: Identical optional restoration, file reads, fallback behavior, and source-quote rendering.; critical-path effect: Quote resolution is started with void after rendering the origin title; it does not block scrolling or initial transcript display.
  - serialization at <code>src/vs/workbench/contrib/chat/common/model/chatSessionStore.ts:685-702</code>: Decode and revive a restored source-session log or JSON history; cardinality: Once per successful local cache-miss restoration, scaling with source-session data size; unchanged by the new styling.
    - Introduced by diff: false; previous behavior: The same full source-session decode occurred on the same cache-miss path.; critical-path effect: Part of optional asynchronous quote resolution, not added to hover or outline rendering.
  - other at <code>src/vs/workbench/contrib/chat/common/chatService/chatServiceImpl.ts:843-880,977-998</code>: Existing remote source-session provider resolution through canResolveChatSession and getOrCreateChatSession; cardinality: Loaded model hits avoid provider resolution. Concurrent remote misses join the in-flight load keyed by session resource; distinct source resources can load independently.
    - Introduced by diff: false; previous behavior: Identical remote model reuse, per-resource in-flight coalescing, and provider requests.; critical-path effect: Optional quote resolution remains off the initial transcript display path. Provider-specific downstream IPC/network implementation is unchanged and was not expanded into unrelated code.
- Verdict: No reportable regression. Bounded nonblurred outline painting does not establish scroll jank; existing filesystem, provider, and serialization work is not caused or amplified by this CSS-only production change.

### Scenario 3: Using active chats, comparison-grid mode, high-contrast themes, or reduced motion

- Critical path: Browser selector evaluation determines styling when mode, theme, or active-group state changes. Active chats, comparison grids, and high-contrast modes are excluded from the new outline rules; reduced motion excludes the transition rule.
- Scaling input: Mounted session lists and their rendered descendants when application classes change; no new collection traversal in application code.
- Effective concurrency: Existing synchronous class/style updates followed by browser rendering; no new asynchronous tasks or boundary calls.
- Cache behavior: No changed cache or model lifecycle. Theme colors still come from the existing in-memory theme lookup and CSS variables.
- Mechanism confidence: High confidence from explicit selector exclusions and tests asserting active/comparison/high-contrast absence of outlines, unchanged bubble dimensions, and zero reduced-motion animations.
- Magnitude uncertainty: No measured theme-switch or activation latency; no concrete added scaling mechanism was found for the excluded modes.
- Expensive boundaries: none found
- Verdict: No performance regression established. Tests and fixture descriptions cover visual behavior, and the stylelint variable inventory is build-time metadata rather than added product-path work.

**Summary:** Completed the performance-review pass over all four changed files and every diff page against base 733a99842b0e0e14c3593dfab31fff2035478ba0. Traced CSS animation/paint, virtualized scrolling and sticky rows, active-state callers, and unchanged optional source-session I/O/serialization. No high-confidence actionable performance regression or concrete regression fix found. Static read-only review; the supporting guide tool output was truncated, while the learned-mechanism document and supplied focus instructions were available.

</details>

<details>
<summary>Review statistics</summary>

- Model: `gpt-6.1-sol`
- Actual model calls: `gpt-6.1-sol`
- Reasoning effort: `high`
- Actual reasoning effort: `high`
- Model API endpoint: `ws:/responses`
- Wall-clock time: 1m58.082s
- Model calls: 13
- Tokens: 406068 input, 4363 output, 410431 total; 748 reasoning; 359781 cache read, 46248 cache write
- Aggregate model API time: 1m35.454s
- Model-returned tool calls: 42
- Copilot usage: 19530610000.000 nano-AI units; model billing multiplier: 1.000
- USD cost: unavailable; the Copilot SDK does not expose a dollar conversion.

</details>
