# Performance review: Center full-width characters in two monospace cells

[PR #334022](https://github.com/microsoft/vscode/pull/334022) at `b9b8ede3afdb676101dccab70bb672660762b57f`

## Findings (1)

### 1. [high] Bound the per-character advanced-wrap measurement DOM

**Finding ID:** `PERF-2EA2FF1141C2`  
**Location:** <code>src/vs/editor/browser/view/domLineBreaksComputer.ts:280</code> (<code>RIGHT</code>)  
**Performance category:** <code>layout</code>  
**Mechanism family:** <code>boundary-fanout</code>  
**Resource:** temporary DOM nodes, HTML bytes, style/layout work, and Range geometry reads  
**Scaling:** Cost and peak retained DOM grow linearly with the total number of full-width characters in the entire model, not with the viewport; all lines are placed into one container and processed serially in one finalize call.  
**Outcome:** blocked critical path and severe CPU/GC pressure during synchronous wrapping  
**PR causality:** <code>materially-amplified</code>  
**Previous behavior:** Advanced wrapping still measured the whole model synchronously, but ordinary full-width text stayed inside shared spans, normally yielding about one child span per model line rather than one per character.  
**Changed behavior:** Advanced wrapping now materializes each full-width character as its own styled DOM element whenever the new two-cell option is active, instead of keeping ordinary text in large shared spans.  
**Confidence:** 0.98 — The per-character span is explicit in the changed loop, and the complete synchronous call path is established: \_constructLines queues every model line, finalize builds one HTML string/container, assigns innerHTML, appends it to document.body, and reads layout before returning. The only magnitude uncertainty is how often users combine the opt-in width mode with advanced wrapping and very large CJK files; advanced wrapping is also selected automatically with accessibility support.

**Causal diff evidence:** The added full-width branch at src/vs/editor/browser/view/domLineBreaksComputer.ts:280 emits a separate inline-block span for every classified full-width character and closes it immediately after that character. Before this hunk, those characters remained in the normal shared span, which was split only at fixed-width injections or the 16,384-character safety interval.

Opening, reconfiguring, resizing/re-wrapping, or flushing a large CJK-heavy editor with \`fullwidthCharacterWidth: 'twoCells'\` and advanced wrapping can block the UI while the browser parses, styles, lays out, and measures hundreds of thousands of temporary elements; peak DOM memory and GC pressure grow at the same time.

**Evidence:** ViewModelLinesFromProjectedModel.\_constructLines queues all model lines and calls finalize synchronously (viewModelLines.ts:125-139). DOMLineBreaksComputer.createLineBreaks renders all queued lines into one StringBuilder, assigns the result to one container's innerHTML, appends that container to document.body, and synchronously calls Range.getClientRects while discovering breaks (domLineBreaksComputer.ts:75-195, 386-438). The changed branch at lines 273-297 creates and closes an inline-block span around every full-width code unit. Thus a 200k-character CJK model produces roughly 200k child spans in one temporary live DOM subtree, whereas the previous path generally used one span per line (with only 16,384-character splits).

**Suggested direction:** Keep measurement semantics but bound the fan-out: process queued lines/characters in capped batches so only a limited temporary subtree is live at once, and where possible compute fixed two-cell runs outside the browser instead of emitting one measurement element per character.


<details>
<summary>Reviewer scenario analysis</summary>

**Relevant issue families:** boundary-fanout, synchronous-ui-work, serialization-allocation, repeated-work

### Scenario 1: Open or rewrap a large CJK-heavy editor using two-cell character width and advanced wrapping.

- Critical path: Editor construction, a wrapping-affecting configuration change, model flush, or advanced rewrap synchronously completes HTML generation, innerHTML parsing, DOM attachment, style/layout, and Range geometry reads before the view model can finish and render.
- Scaling input: Total classified full-width characters in the entire model, with no viewport reduction, grouping, cache hit, or batching on full reconstruction.
- Effective concurrency: All model lines and their geometry reads execute serially on the UI thread in one finalize call; there is no chunking, yielding, queue, or backpressure.
- Cache behavior: No viewport or line-break cache bounds the configuration/open path: construction queues every model line into one finalize call. Incremental edits request only changed/inserted lines, but changing font/wrapping/two-cell policy reconstructs all projections.
- Mechanism confidence: Very high: the added loop emits and closes one span per classified character, while \_constructLines demonstrably queues every model line into the one live measurement container.
- Magnitude uncertainty: The exact affected population depends on users enabling the new two-cell option and selecting advanced wrapping; accessibility mode selects advanced automatically. Browser-specific duration is unmeasured, but whole-model cardinality and synchronous reachability are explicit.
- Expensive boundaries:
  - other expensive boundary at <code>src/vs/editor/browser/view/domLineBreaksComputer.ts:createLineBreaks/renderLine</code>: Serialize all lines to HTML, materialize a live measurement DOM subtree, and synchronously read browser geometry.; cardinality: One temporary element per full-width character across all queued model lines after the diff; realistic CJK-heavy models can produce hundreds of thousands of elements in one subtree, followed by multiple Range.getClientRects reads per wrapped line.
    - Introduced by diff: true; previous behavior: The same advanced-wrap DOM boundary existed, but ordinary full-width text was grouped in shared spans (normally one child span per line, split only around injections or every 16,384 characters) rather than one child per character.; critical-path effect: DOM parsing, attachment, layout, and geometry measurement must finish synchronously before line projections and editor layout can proceed.
- Verdict: Reportable high-severity regression: the PR materially amplifies an already slow synchronous boundary from roughly line-scaled DOM children to character-scaled children and peak live DOM.

### Scenario 2: Type, scroll, and render dense CJK text with two-cell mode enabled.

- Critical path: For changed or newly visible lines, token splitting and HTML serialization occur before DOM line rendering; browser layout is subsequently needed for widths and caret/range geometry.
- Scaling input: Classified full-width characters among visible or changed rendered lines, after the 10,000-character line cap.
- Effective concurrency: Visible line rendering is synchronous and serial on the UI thread, but cardinality is viewport-bounded and long-line output is capped by stopRenderingLineAfter (10,000 by default).
- Cache behavior: ViewLine input equality avoids rebuilding unchanged visible lines, and pixel offsets are cached per rendered line where the existing cache is enabled. Scrolling or edits still render newly visible/changed lines.
- Mechanism confidence: High for added allocation and DOM fan-out; lower that it independently causes a user-visible regression at normal viewport sizes.
- Magnitude uncertainty: Dense CJK viewports can create thousands of nodes, but viewport bounds and the opt-in setting make this less severe than the whole-model advanced-wrap path, and individual boxes implement the requested centering semantics.
- Expensive boundaries:
  - serialization at <code>src/vs/editor/common/viewLayout/viewLineRenderer.ts:splitFullWidthCharacters/\_renderLine</code>: Split line parts around every full-width character and serialize each as a fixed-width inline-block span.; cardinality: One rendered span per classified full-width character in visible/changed text, plus one character-classification pass; bounded to rendered lines and the configured per-line render cap.
    - Introduced by diff: true; previous behavior: Full-width characters shared syntax/decorations spans; DOM cardinality tracked token/decorations parts rather than characters.; critical-path effect: Adds HTML bytes, DOM nodes, and style/layout work before the frame completes.
- Verdict: No separate finding: this is the direct visible rendering implementation, is viewport/cap bounded, and evidence does not establish an independent high-confidence regression beyond the reported unbounded whole-model measurement path.

### Scenario 3: Render CJK text with experimental GPU acceleration and two-cell mode enabled.

- Critical path: Each visible line must pass eligibility before GPU buffer population. Lines containing a classified full-width character fall back to the existing DOM renderer so the requested two-cell styling can be applied.
- Scaling input: Visible line count times rendered content length, capped per line; effective fallback count is only lines containing classified full-width characters.
- Effective concurrency: Eligibility checks and GPU/DOM routing are serial per visible line on the render thread; no external concurrency or queue is involved.
- Cache behavior: GPU eligibility is recomputed per visible line during rendering. There is no memoized full-width-presence flag, but scanning is limited by stopRenderingLineAfter; unchanged full-file GPU lines may later be skipped only after eligibility is checked.
- Mechanism confidence: High for the scan and fallback routing; uncertain that the bounded extra scan alone is material.
- Magnitude uncertainty: GPU usage and CJK density vary, and the option documentation explicitly warns of rendering cost. No measured frame regression or avoidable semantics-preserving GPU implementation is established.
- Expensive boundaries:
  - other expensive boundary at <code>src/vs/editor/browser/gpu/viewGpuContext.ts:canRender/requiresDomFullwidthCharacterRendering</code>: Scan rendered content for full-width characters and reject GPU rendering when specialized DOM boxes are required.; cardinality: At most one bounded content scan per visible line per GPU render pass; lines with full-width content are routed to one DOM line render rather than duplicated across GPU and DOM.
    - Introduced by diff: true; previous behavior: Eligible CJK lines could remain on the GPU path; no specialized content scan was performed.; critical-path effect: The scan is on the frame path, and fallback lines incur DOM rendering, but both are viewport-bounded.
- Verdict: No finding: fallback is required by the feature's unsupported GPU representation and remains viewport-bounded; the avoidable scan overhead is not proven material.

### Scenario 4: Move or reveal the caret and render selections adjacent to a two-cell full-width character.

- Critical path: Caret placement, reveals, and collapsed selection geometry may synchronously query an element box; this replaces the existing collapsed Range geometry query for those positions rather than adding a second successful layout read.
- Scaling input: Uncached caret/range positions queried near full-width characters, generally one position per caret/reveal operation.
- Effective concurrency: Geometry reads are serial on the UI thread and occur per uncached requested position; there is no IPC, filesystem, network, or subprocess work.
- Cache behavior: RenderedViewLine caches pixel offsets per column when its existing cache is active; cache lifetime is the rendered line. The new branch runs only next to a classified full-width character.
- Mechanism confidence: High that the changed read is critical-path geometry; high that it substitutes for an existing layout read rather than introducing repeated layout.
- Magnitude uncertainty: Browser costs of element versus Range rect reads may differ, but call cardinality does not increase on the successful path and cached offsets prevent repeated reads.
- Expensive boundaries:
  - other expensive boundary at <code>src/vs/editor/browser/viewParts/viewLines/rangeUtil.ts:readHorizontalRangeForElement and viewLine.ts:\_actualReadPixelOffset</code>: Read the fixed-width child element's client rect to obtain its cell edge.; cardinality: One getClientRects call for an uncached queried position adjacent to a full-width character; fallback to the old Range read only if the element lookup/read returns no usable rect.
    - Introduced by diff: true; previous behavior: The same scenario synchronously read a collapsed DOM Range's client rect to locate the glyph edge.; critical-path effect: Geometry is needed before caret/range placement can complete, as was already true for non-basic text.
- Verdict: No finding: the changed geometry operation replaces an already-required synchronous geometry read and is cached under the same lifecycle.

**Summary:** Completed the performance pass over all 24 changed files and every diff hunk. Traced editor construction/configuration, advanced whole-model wrapping, visible DOM rendering, GPU eligibility/fallback, and caret geometry. Submitted one high-confidence whole-model DOM fan-out finding; no subprocess, IPC, filesystem, database, or network boundaries are reached by these paths.

</details>

<details>
<summary>Review statistics</summary>

- Model: `gpt-5.6-sol`
- Actual model calls: `gpt-5.6-sol`
- Reasoning effort: `high`
- Actual reasoning effort: `high`
- Model API endpoint: `ws:/responses`
- Wall-clock time: 2m27.693s
- Model calls: 10
- Tokens: 594395 input, 9446 output, 603841 total; 4192 reasoning; 510747 cache read, 83618 cache write
- Aggregate model API time: 2m18.403s
- Model-returned tool calls: 67
- Copilot usage: 81142880000.000 nano-AI units; model billing multiplier: 1.000
- USD cost: unavailable; the Copilot SDK does not expose a dollar conversion.

</details>
