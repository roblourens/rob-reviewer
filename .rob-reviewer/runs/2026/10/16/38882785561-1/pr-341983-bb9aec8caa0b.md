# Performance review: chat: fix flaky Draw atlas tests leaking into later suites

[PR #341983](https://github.com/microsoft/vscode/pull/341983) at `bb9aec8caa0b132236e6d85bf48bf71f8ae39b61`

## Findings

No high-confidence performance findings.

## Performance regression fixes detected (1)

### 1. repeated-work

**Fix location:** `src/vs/workbench/contrib/chat/test/browser/widget/chatWorkingLogoAtlas.test.ts:220` (`RIGHT`)  
**Blamed base line:** `src/vs/workbench/contrib/chat/test/browser/widget/chatWorkingLogoAtlas.test.ts:192`  
**Introducing path:** `src/vs/workbench/contrib/chat/test/browser/widget/chatWorkingLogoAtlas.test.ts`  
**Introducing commit:** `e142434610b3bae70cfcbac5d9404909f94cad16`  
**Mechanism family:** `repeated-work`  
**Performance category:** `cpu`  
**Confidence:** 0.96

**Mechanism:** The live-atlas remount test released the production shared cache at zero references and then reacquired the same atlas key, forcing serial SVG decode, canvas rasterization, and blob encoding repeatedly in one test.

**Symptom:** Repeated full-atlas generation made the browser test slow enough to exceed its 15-second timeout on throttled or slow CI agents.

**Fixed behavior:** The added test-local cache survives zero-reference intervals for the test lifetime, so hide/remount cycles reuse the first decoded atlas rather than repeating the 320-frame rasterization for each reacquire.

**Evidence:** The base test constructed ChatWorkingLogo without an acquireAtlas override, so each remount used acquireChatWorkingLogoAtlas. That shared wrapper disposes its per-window cache when the last reference is released (chatWorkingLogoAtlas.ts:119-151), while hiding/removing the logo clears its atlas request; the same test then hides and remounts the logo repeatedly. Each resulting cold acquire calls createChatWorkingLogoAtlas, whose serial loop decodes and rasterizes 320 frames (chatWorkingLogoAtlas.ts:203-275). The changed test instead owns one ChatWorkingLogoAtlasCache and routes every reacquire through it, whose released completed entries remain reusable until cache teardown.


<details>
<summary>Reviewer scenario analysis</summary>

**Relevant issue families:** repeated-work, cleanup-lifecycle, cache-lifecycle

### Scenario 1: Real atlas generation and native-tile validation in the browser test

- Critical path: The test cannot assert tile pixels until full atlas generation, atlas image loading, and frame-by-frame expected-image decoding complete.
- Scaling input: Fixed 320 atlas frames plus 320 validation frames; physical tile size controls pixel work.
- Effective concurrency: Generation and validation are serial: the generator loops over 320 frames with periodic yields, then the test validates frames one at a time. No limiter is needed because there is no fan-out.
- Cache behavior: No atlas cache is used for the direct generator call; every run is cold. Blob URLs and canvases are disposed at test teardown or per-frame finally blocks.
- Mechanism confidence: High: the 320-frame loops, awaits, and canvas/image operations are directly visible.
- Magnitude uncertainty: Frame decode duration varies substantially by browser host and CPU speed, but cardinality and serial critical-path placement are explicit.
- Expensive boundaries:
  - other expensive boundary at <code>src/vs/workbench/contrib/chat/browser/widget/chatWorkingLogoAtlas.ts:createChatWorkingLogoAtlas and src/vs/workbench/contrib/chat/test/browser/widget/chatWorkingLogoAtlas.test.ts:139-207</code>: Serialize SVG frames, decode them through HTMLImageElement, rasterize to canvas, encode an atlas blob, and read pixels for comparison.; cardinality: Up to 320 SVG image decodes during generation and another 320 serial SVG image decodes during validation for one test run.
    - Introduced by diff: false; previous behavior: The same real generation and validation work already occurred; the diff only supplies a teardown-cancelled token and raises the Mocha timeout.; critical-path effect: All decoding and canvas pixel reads must finish before the test completes.
- Verdict: No regression finding: the diff adds cancellation without increasing successful-path work and bounds abandoned work at teardown.

### Scenario 2: Live Draw logo hide, resume, remove, and remount test

- Critical path: The test waits for the first atlas to be generated and decoded before checking animation state; later remounts can proceed from the retained cached atlas.
- Scaling input: Number of hide/remount reacquisitions; effective atlas-key cardinality is one.
- Effective concurrency: One atlas generation runs at a time. Reacquisitions for the same key share or reuse the single cache entry rather than creating a burst.
- Cache behavior: The new per-test ChatWorkingLogoAtlasCache has one cold generation for the size/color key, then warm reuse across zero-reference hide/remount intervals; it is disposed after the test. Before the diff, the production wrapper disposed its cache whenever references reached zero, making each reacquire cold.
- Mechanism confidence: High: cache reference teardown and completed-entry retention are explicit, and the test performs repeated remove/remount transitions.
- Magnitude uncertainty: Exact elapsed savings vary by CI CPU and browser throttling, but the removed multiplicity is established.
- Expensive boundaries:
  - other expensive boundary at <code>src/vs/workbench/contrib/chat/browser/widget/chatWorkingLogoAtlas.ts:createChatWorkingLogoAtlas</code>: Decode and rasterize all atlas frames and encode the resulting canvas to a blob.; cardinality: After the diff, one generation with up to 320 serial SVG decodes and one canvas blob encoding serves all remounts; before the diff, the same full generation could repeat for each of roughly three acquisitions.
    - Introduced by diff: false; previous behavior: Each zero-reference release disposed the production shared cache, so later remounts regenerated the same atlas.; critical-path effect: The first generation blocks initial readiness; warm remounts avoid repeated generation on the test critical path.
- Verdict: This is a performance fix, not a regression; it was recorded through report\_regression\_fix.

### Scenario 3: DPR and theme changes replace a Draw atlas in browser tests

- Critical path: Each test waits for the initial atlas and one replacement atlas before reading dimensions or pixels. Polling observes completion but does not initiate additional generations.
- Scaling input: Two distinct cache keys per test and a fixed 320 frames per key.
- Effective concurrency: Replacement is sequenced by logo request cancellation; polling uses one outstanding 10 ms timer at a time and does not add atlas concurrency.
- Cache behavior: Each distinct DPR or color creates a different key. The production shared cache handles the requests; there is no new cache behavior in this diff. Cancellation tokens created by polling are released at test teardown.
- Mechanism confidence: High: changed lines replace only polling/cancellation and timeout controls around unchanged product operations.
- Magnitude uncertainty: Browser decode time varies; no changed generation cardinality or concurrency was found.
- Expensive boundaries:
  - other expensive boundary at <code>src/vs/workbench/contrib/chat/browser/widget/chatWorkingLogoAtlas.ts:createChatWorkingLogoAtlas</code>: Rasterize and encode an atlas for each distinct size/color key.; cardinality: Two distinct atlas generations per test, each with up to 320 serial SVG decodes and one blob encode; effective key cardinality is two.
    - Introduced by diff: false; previous behavior: The same two atlas keys and generation calls were awaited through the bounded retry helper.; critical-path effect: Both generations are required before assertions can complete.
- Verdict: No performance regression: successful-path expensive work is unchanged, while teardown cancellation prevents timed-out polling and generation from spilling into later suites.

### Scenario 4: Polling and cancellation during test failure or teardown

- Critical path: On success, conditions are checked every 10 ms. If a condition never becomes true, the Mocha timeout triggers teardown, which cancels the outstanding timer and atlas work.
- Scaling input: Number of sequential wait conditions in a test; each has one timer and one cancellation source.
- Effective concurrency: Each awaited waitFor loop has at most one timer outstanding; calls are sequential in every changed test.
- Cache behavior: Each wait allocates a cancellation source retained only until that test's teardown; there is no cross-test cache or retained failure state.
- Mechanism confidence: High for timer lifecycle and bounds: the loop, cancellation source, teardown disposal, and test timeouts are explicit.
- Magnitude uncertainty: A failing assertion can now be detected at the enclosing 30 or 60 second Mocha timeout rather than the prior 1-3 second retry cap, but this is a bounded failure-only CI latency tradeoff and does not affect product execution.
- Expensive boundaries: none found
- Verdict: No reportable product performance regression. The bounded failure-path delay is paired with cancellation that eliminates the prior cross-suite disposable/timer spillover.

**Summary:** Reviewed the sole changed test file and traced atlas generation, image decoding, canvas encoding, cache ownership, polling, cancellation, and teardown. No diff-introduced performance finding met the reporting bar. The PR removes repeated full-atlas generation in the remount test and bounds abandoned asynchronous work; the repeated-work fix was recorded with its introducing commit.

</details>

<details>
<summary>Review statistics</summary>

- Model: `gpt-5.6-sol`
- Actual model calls: `gpt-5.6-sol`
- Reasoning effort: `high`
- Actual reasoning effort: `high`
- Model API endpoint: `ws:/responses`
- Wall-clock time: 1m25.2s
- Model calls: 8
- Tokens: 205532 input, 5334 output, 210866 total; 2423 reasoning; 168905 cache read, 36603 cache write
- Aggregate model API time: 1m9.098s
- Model-returned tool calls: 22
- Copilot usage: 35735300000.000 nano-AI units; model billing multiplier: 1.000
- USD cost: unavailable; the Copilot SDK does not expose a dollar conversion.

</details>
