# Performance review guide

Use this guide to identify concrete performance bugs introduced by a change. It is not a requirement to comment on every potentially inefficient construct. Report only when the changed code creates a plausible regression mechanism on a realistic execution path and the proposed correction is compatible with the surrounding behavior.

Performance is the interaction of:

- **cost per operation**;
- **how often the operation runs**;
- **how the cost grows with input or retained history**;
- **which thread, process, or resource pays the cost**;
- **how long the cost or retained state persists**.

A small operation can be a serious bug when it runs per frame, row, message, token, listener, or file. An expensive operation may be harmless when it is rare, bounded, asynchronous, and outside a user-critical path. Establish both cost and multiplicity before reporting.

## Review method

### 1. Find performance-sensitive paths

Prioritize changed code that can execute during:

- application or extension startup;
- input, scrolling, dragging, resizing, animation, or rendering;
- list/tree row creation, update, recycling, or measurement;
- streaming responses and high-frequency events;
- file watching, logging, telemetry, IPC, RPC, or serialization;
- repeated background polling or indexing;
- creation and disposal of long-lived objects;
- processing of unbounded user, workspace, history, or protocol data.

Trace callers rather than judging a function in isolation. Determine whether a changed helper moved onto a hotter path, whether an existing loop now calls more expensive work, or whether a callback is registered once versus once per item.

Start from the changed hunks and classify relevant issue families before expanding context. Large files and unrelated repository context can dilute the signal. Read callers and neighboring implementations selectively to prove a candidate rather than eagerly loading whole files.

### 2. Build a cost model

For each candidate, answer:

1. What triggers it?
2. How often can it run?
3. What input or collection controls its size?
4. Is the result awaited on the critical path of an important product scenario?
5. What work or memory is multiplied?
6. How many distinct cache keys or resource groups exist on a cold load?
7. Is the cost parallel, bounded-concurrent, or secretly serialized downstream?
8. Is there a hard bound, cache, negative cache, or cleanup point?
9. What product or user-visible behavior degrades?

Prefer concrete descriptions such as "one IPC request and buffer allocation per streamed token" over vague claims such as "this might be slow."

For collection work, write an explicit upper-bound model when the evidence supports it:

`wall time ~= fixed work + number of uncached resource groups × serialized boundary cost`

Adjust the cardinality for real deduplication: N sessions may map to fewer repositories, but a cache with one key per repository still has one cold miss per distinct repository. A `Promise.all` at the caller does not imply parallel wall time when a helper uses a global sequencer, mutex, limiter of one, rate limiter, or single-consumer queue.

This is static critical-path reasoning, not a claim that the reviewer measured wall time. The reviewer should identify what must complete before the scenario can progress, then explain which changed operations contribute to that path. Actual duration remains a measurement question.

### 3. Prove the change introduces the problem

The finding must be caused by the reviewed diff. Check the previous behavior, existing guards, call-site frequency, ownership, cancellation, and cleanup. Do not report a pre-existing hot path merely because the diff made it easier to notice.

### 4. Preserve semantics in the fix direction

Performance changes often affect ordering, invalidation, freshness, cancellation, backpressure, error propagation, and lifecycle. Recommend batching only when flush and ordering semantics can be preserved. Recommend caching only when the key, invalidation, size bound, and ownership are understood. Recommend deferral only when the work is not required before first use.

### 5. Use budgets as evidence, not syntax rules

For Chromium renderer work, useful diagnostic thresholds are:

- a task of 50 ms or more is a long task;
- a 60 Hz frame is 16.7 ms, with roughly 10 ms typically available after browser overhead;
- web experiences target Interaction to Next Paint of 200 ms or less at the 75th percentile.

These numbers help translate mechanism into impact, but static code cannot prove a threshold was exceeded. Electron/VS Code does not report web Core Vitals exactly like a website. Require a realistic workload or measurement plan rather than declaring any 50 ms estimate a confirmed regression.

Interaction latency has three parts: input delay before the callback runs, callback processing time, and presentation delay before the next painted frame. Identify which part the changed code affects.

## Common issue families

### Repeated work and scaling mistakes

Look for work whose frequency or complexity grows unexpectedly:

- a loop nested inside another loop over related data;
- rescanning a complete collection after each incremental update;
- rebuilding state from all history for every new item;
- sorting, filtering, parsing, or formatting the same data repeatedly;
- doing work for hidden, collapsed, offscreen, disposed, or already-terminal items;
- repeated linear membership checks where a keyed structure already exists or is warranted;
- an operation advertised as incremental that copies or traverses the full data set;
- recursion or graph traversal without visited-state protection;
- retry paths with no bound, delay, or progress condition.
- a collection mapping that introduces one subprocess, IPC call, filesystem probe, or network request per item or per distinct resource;
- a cache that improves warm refreshes but leaves first load with one miss per high-cardinality key;

Pay special attention to virtualized UI code. Row creation and recycling should normally depend on the visible row and a small amount of local state, not the total history attached to that row.

Do not infer a bug from Big-O notation alone. Establish realistic collection size, invocation frequency, and the cost of the loop body.

### Work on latency-sensitive threads

Look for synchronous or long-running work on the UI thread, extension host, main process, request handler, or other shared event loop:

- synchronous file or process APIs;
- loading or initializing a large module during startup;
- parsing large payloads synchronously;
- compression, hashing, or serialization on an interaction path;
- a long promise chain that is technically asynchronous but performs substantial synchronous work before yielding;
- awaiting unrelated work serially when it could safely overlap;
- blocking one shared host for feature-specific initialization.

Cold-path behavior matters. Module loads, disk access, and initialization that appear fast on a warm developer machine can block on remote, networked, virtualized, or resource-constrained environments.

### Rendering, style, and layout

Look for browser work that multiplies across frames or large DOM populations:

- layout/style reads such as `getComputedStyle`, `getBoundingClientRect`, `offset*`, or `client*` interleaved with DOM writes;
- layout-triggering reads in scroll, pointer-move, resize, animation, or repeated row-render callbacks;
- repeated DOM insertion instead of fragmenting or batching;
- unnecessary rerenders caused by unstable identities, broad state updates, or recreated inputs;
- selector changes that make a large repeated population a matching subject, especially complex relational selectors;
- global CSS variables or dimensions applied after dependent widgets have already been created;
- observers, timers, or reactive subscriptions created per row and not removed on recycling;
- expensive rendering for invisible or collapsed content;
- asynchronous row-size changes that are suppressed, deduplicated, or committed before the virtualizer receives them.

Watch for layout thrashing: a write invalidates layout, a read forces recalculation, and the pattern repeats inside a loop or event stream. A single layout read is not automatically a bug.

Multiple layout reads are not automatically multiple forced layouts. If two reads occur in the same synchronous call stack with no intervening DOM, style, class, or geometry-affecting write, the browser can normally reuse the same computed layout. Trace the exact read/write ordering before claiming an additional reflow.

#### Relational selector review

Treat `:has()` as a reason to inspect selector scope, not as an automatic finding. Its cost depends heavily on which elements are candidate subjects, how large and dynamic the subtree is, and how often mutations force matching to be reconsidered.

Look for:

- a selector whose subject is every row in a large list or tree;
- a broad ancestor subject that must be reconsidered when many descendants change;
- relational selectors in virtualized or rapidly mutating DOM;
- several variants of the same expensive selector spread across CSS files or component modes;
- a partial fix that replaces one usage but leaves equivalent selectors elsewhere;
- a selector used only to reflect state that the renderer already knows and could expose with a class or data attribute.

When a relational selector is suspected, isolate the representative DOM shape and compare style-recalculation time before and after an explicit state class. Audit all matching usages rather than assuming one replacement removes the hotspot. A class toggle is appropriate only when its update lifecycle remains correct.

#### DOM size, containment, and offscreen work

Large DOMs increase style, layout, query, and retained-memory costs. Look for:

- append-only "infinite" lists that never recycle or remove nodes;
- `Array.map`-style rendering of an unbounded result set;
- hidden panels that continue to render or update complete subtrees;
- broad DOM queries repeated after every incremental mutation;
- new subtree-wide invalidation without CSS containment;
- offscreen content whose full layout and paint are unnecessary.

True virtualization keeps rendered node count bounded. `content-visibility: auto` and CSS containment can skip offscreen work, but they change sizing and layout contracts. `contain-intrinsic-size` may be required to avoid scroll jumps; containment can conflict with sticky positioning, overflow, measurement, and accessibility expectations. Do not recommend it generically without checking those constraints.

#### Animation and compositing

Animations of `transform` and `opacity` can often remain in the compositor. Animating geometry or paint-heavy properties such as width, height, top, margin, shadows, or filters can trigger layout or paint every frame.

Also look for layer overuse:

- `will-change` applied broadly or permanently;
- one promoted layer per row in a large list;
- layers retained after animation completes;
- large textures repeatedly uploaded to the GPU.

Compositor promotion is a resource tradeoff, not a universal optimization.

### Allocation and garbage-collection pressure

Look for high-frequency allocation of:

- buffers, arrays, promises, closures, or wrapper objects;
- temporary transformed object graphs before serialization;
- repeated string concatenation or formatting;
- per-message deep clones;
- short-lived observers, cancellation objects, or disposable stores;
- diagnostic objects attached to every instance of a high-cardinality type.

Allocation matters when multiplied by frequency or payload size. Explain why objects become garbage quickly, why they survive long enough to promote, or why the allocation volume affects a shared process. Prefer reuse or fewer transformations only when ownership remains clear and mutable state cannot leak between operations.

### Retained memory and lifecycle leaks

Trace both registration and removal for:

- event listeners, observers, timers, intervals, and callbacks;
- maps, sets, arrays, caches, queues, and history collections;
- closures that capture models, DOM nodes, services, or large payloads;
- per-resource state after the resource closes, is replaced, or is disposed;
- diagnostic metadata such as stack traces and counters;
- pending promises, requests, cancellation tokens, and retry state.

Common warning patterns include:

- decrementing a counter to zero without deleting the key;
- an append-only collection keyed by every value ever seen;
- a cache with no size bound, expiration, invalidation, or owner;
- listeners removed only on a success path;
- a self-cleaning listener whose callback runs but whose disposable wrapper remains in an owner map or array;
- a disposable store or backing array that retains disposed wrappers when a model is recreated;
- replacement that installs a new listener without disposing the old one;
- queue entries retained after cancellation or failure;
- weakly bounded "recent" history that is never actually trimmed;
- storing both a source object and a full derived copy indefinitely.

Distinguish retained memory from temporary allocation. A large collection with a clear, bounded owner may be intentional.

When a listener exists solely to react to its owner's disposal, the disposal callback should normally remove and dispose its own entry from any containing map. Verify both the underlying listener registration and every strong container that can retain its disposable wrapper. Repeated open/close or create/dispose cycles are useful leak tests because counts should return to a stable baseline.

### Caches and memoization

Caches can improve speed or introduce correctness and memory regressions. Check:

- whether the key captures every input that affects the result;
- whether entries are invalidated when source state changes;
- whether the cache is bounded by count, bytes, time, or owner lifetime;
- whether errors, partial results, or cancelled operations are cached;
- whether concurrent misses duplicate expensive work;
- whether cached values retain objects beyond their intended lifetime;
- whether a cache moved per-instance work into global process lifetime;
- whether lookup overhead exceeds the avoided work for the actual usage pattern.

Do not suggest "add a cache" without specifying key, invalidation, bound, and ownership.

### Batching, queues, and backpressure

High-frequency operations often need coalescing, but batching must remain bounded and observable. Look for:

- one file write, IPC call, network request, or state publication per item;
- a queue drained by creating one task or microtask per entry;
- repeated cancellation and recreation of timers;
- producers that can outrun a serialized consumer indefinitely;
- unbounded batches that create a large pause or allocation spike;
- flush that waits for the current batch but misses items queued concurrently;
- errors that drop the remainder of a batch or leave the drain permanently stalled;
- retry that re-enqueues faster than failures resolve.

Check order, maximum batch size, latency requirements, flush behavior, cancellation, rotation/transaction boundaries, and error isolation. "Batch this" is incomplete advice without those constraints.

### I/O, IPC, RPC, and process boundaries

Crossing a boundary usually has fixed overhead in addition to payload cost. Look for:

- calls inside tight loops that could use an existing bulk API;
- chatty request/response patterns where one side already has enough information to batch;
- duplicate reads or stats of the same resource in one operation;
- writing an entire file or state snapshot for a small incremental change;
- protocol messages that include large redundant fields;
- logging that serializes and writes every high-frequency event;
- reading unbounded files or command output into memory;
- starting subprocesses for operations available in-process;
- starting one subprocess per item while building a list or restoring state;
- independently querying the same external system when one bulk query could serve the complete collection;
- missing backpressure between producer and consumer.

Consider remote development, high-latency links, and process serialization. A local method call and an IPC proxy with the same signature have very different costs.

#### Expensive boundary calls on critical paths

For changes that add subprocess, IPC, filesystem, database, or network calls:

1. Identify the important product scenario that reaches the code.
2. Trace whether the scenario waits for those calls before it can complete or make progress.
3. Find boundary calls hidden in helpers, not only on changed lines.
4. Determine how call count scales with realistic input: items, files, sessions, repositories, messages, providers, or another collection.
5. Account for caching and coalescing on both cold and warm paths.
6. Inspect effective concurrency. Apparent parallelism can become serialization through a sequencer, mutex, limiter, rate limit, or single consumer.
7. Check whether a bulk call, shared result, bounded concurrency, or work moved off the critical path would preserve semantics.

External process and IPC calls have fixed overhead even when they succeed quickly. Many calls can dominate an important scenario without any single call being exceptionally slow or timing out. Conversely, do not multiply cost blindly when one call populates a cache or serves many items.

When proposing validation, use a representative high-cardinality input and record boundary-call count plus end-to-end scenario duration. Initial UI population is one example; the same reasoning applies to search, save, refresh, restore, navigation, streaming, or any other important workflow.

Moving CPU work to a Web Worker, worker thread, extension host, or main process can protect one event loop but adds message, serialization, and scheduling costs. Check:

- whether payloads are cloned for every transfer;
- whether `ArrayBuffer` or other transferable ownership could avoid copying;
- whether messages are smaller and less frequent than the work they offload;
- whether the destination is another shared latency-sensitive process;
- cancellation and stale-result behavior;
- duplicate state retained on both sides of the boundary.

### Serialization and data copying

Look for avoidable full-data transformations:

- deep cloning immediately before serialization;
- parse-stringify-parse sequences;
- converting between buffers, strings, and byte arrays multiple times;
- repeated encoding of unchanged data;
- spreading or concatenating a growing array in a loop;
- returning a full snapshot when the consumer needs a small projection;
- retaining source and transformed forms simultaneously without need.

Prefer one traversal or streaming when it materially reduces work and preserves semantics. Do not recommend a custom serializer without evidence that serialization is on the relevant path.

### Async work, concurrency, and cancellation

Look for:

- independent operations awaited sequentially;
- accidental unbounded concurrency from mapping to promises;
- duplicate in-flight work for the same key;
- work continuing after its result can no longer be used;
- queued work that reads a model before checking whether it was disposed;
- cancellation checked only after expensive work finishes;
- stale async results overwriting newer state;
- polling or retry loops that overlap;
- locks held across I/O or model calls;
- queues with head-of-line blocking between unrelated resources.

Concurrency is not automatically faster. Consider resource limits, ordering, rate limits, cancellation, and memory held by in-flight work. A safe finding identifies the existing independence or the missing bound.

Async lifecycle checks belong at every point where ownership may have changed: before queued work first touches a resource and after each suspension that permits disposal or replacement. Cleanup and `finally` blocks can also outlive their original dependency-injection scope. Avoid resolving a new service from a disposed editor/view-scoped container at completion time; create a lightweight dependency while the scope is valid or use a longer-lived owner when that matches the service's lifetime.

### Startup and eager initialization

Review newly introduced startup work for:

- eager module or SDK loading;
- database, filesystem, session, or network enumeration before the feature is used;
- contribution phases that run after restoration and trigger a second global layout;
- creation of services, monitors, caches, or workers for users who never use the feature;
- startup work duplicated across windows, hosts, or processes;
- migration or cleanup that scans unbounded historical state synchronously.
- awaited metadata repair or enrichment added to an important startup or restore path;
- cold-cache process, IPC, filesystem, or network fan-out proportional to user history or workspace size;

Ask whether the work is required for correctness before the UI becomes usable. If it can be deferred, ensure first use still has a clear error path and does not create a worse interaction-time stall.

For Electron code, examine the transitive cost of a new module, not only its call site. A single `require`/import can parse large data files and initialize dependency trees. Main-process blocking affects the entire application; renderer and extension-host blocking affects every feature sharing that process.

### Event, observer, and reactive fan-out

Look for changes that increase how much work one state change triggers:

- firing one event per item instead of one aggregate event;
- broad invalidation when only one resource changed;
- listeners that each rescan shared state;
- derived state recomputed independently by many consumers;
- feedback loops where handling an event causes the same event to fire again;
- subscriptions installed inside frequently called update functions;
- transient observers created for state that is already final.
- touch, wheel, or scroll listeners that never call `preventDefault` but are not registered passive;
- scroll handlers that repeatedly call `getBoundingClientRect` for visibility instead of using an existing observer abstraction;
- `MutationObserver` on a broad subtree with layout reads or expensive queries in its callback;
- `ResizeObserver` callbacks that write back to the observed geometry and create repeated observation cycles;
- observer `.observe()` calls without matching `unobserve()` or `disconnect()` at owner disposal.

Trace the fan-out count and whether callbacks execute synchronously. A cheap callback can become expensive when one event reaches thousands of listeners.

Do not report every re-entrant event as a performance loop. Determine whether it is self-terminating, how many follow-up tasks are actually queued, what each task costs, and the realistic collection bound. One extra serialized no-op scan over a small editor list is not a performance bug without evidence of meaningful scale or latency.

### Timers, scheduling, and idle work

Look for:

- one timer per object when a shared scheduler would suffice;
- high-frequency polling where an event already exists;
- timers repeatedly cancelled and recreated during bursts;
- zero-delay timers or microtasks that produce long chains without yielding meaningfully;
- idle work that is rescheduled immediately when it cannot make progress;
- timeouts retained after completion or disposal;
- periodic work whose cost grows with accumulated history.

Scheduling defers cost; it does not remove it. Verify that delayed work is coalesced, cancellable, bounded, and still necessary when it runs.

When a task can exceed the interaction budget, split non-critical work and yield between chunks. `scheduler.yield()` can preserve continuation priority better than repeatedly appending zero-delay timers. Yielding after every tiny operation adds overhead; choose a time or work budget and ensure partial progress, cancellation, and ordering are explicit. `requestIdleCallback` is for genuinely optional background work and must tolerate delayed or absent idle periods.

### Webview and browser-resource loading

For webviews, browser views, previews, and remote HTML, look for:

- below-fold images loaded eagerly;
- likely first-visible/LCP content incorrectly marked lazy;
- images without reserved dimensions or aspect ratio;
- heavy diagram, syntax, or media libraries loaded when the document does not use them;
- fonts or critical resources discovered only after another stylesheet/script round trip;
- broad `preload`/`preconnect` use for resources that may never be needed;
- font/image/resource caches without versioning, bounds, or invalidation.

These browser-network signals usually do not apply to packaged local workbench assets. Establish whether the surface actually loads remote or web-served content before reporting them.

### Error and fallback paths

Performance regressions often hide outside the happy path:

- a fallback repeats expensive work on every streaming chunk or retry;
- a failure bypasses cache cleanup or queue progress;
- rejected work is retried immediately without changed inputs;
- diagnostic logging becomes much more expensive during an incident;
- partial initialization is repeated after each failed attempt;
- a broad catch misclassifies errors and starts a more expensive fallback;
- timeout recovery leaves the original work running.

Estimate how frequently the failure can recur and whether it amplifies the original incident.

### State suppression and deduplication

Optimizations that suppress repeated updates can create both correctness and performance problems. Check whether code:

- marks a value as delivered before the consumer accepts it;
- updates deduplication state before a guarded or suppressed notification;
- drops an update during rendering, batching, or reentrancy without scheduling reconciliation;
- coalesces states that are not actually equivalent;
- prevents a later authoritative measurement from being delivered;
- uses a stale key or version when deciding that work is redundant.

The correction should preserve the optimization while ensuring the authoritative consumer eventually receives the latest state.

## Evidence and severity

A reportable issue should include:

- the changed line that introduces the mechanism;
- the caller or event that reaches it;
- realistic frequency, scale, or lifetime;
- the multiplied work or retained resource;
- the user or system impact;
- a bounded correction that respects semantics.

Separate two questions:

1. **Mechanism confidence:** Is the changed call path, expensive operation, multiplicity, and blocking relationship established?
2. **Magnitude uncertainty:** How often do users hit the largest input, and how much wall time or memory does it consume in practice?

When mechanism confidence is high and a realistically large input exists, missing field measurements should not automatically suppress the finding. Use a lower severity and state the uncertainty. Omit the finding only when reachability, multiplicity, or the expensive operation itself remains speculative.

Useful evidence includes existing tests, call graphs, data-flow invariants, lifecycle ownership, known repository patterns, benchmarks, traces, heap snapshots, and telemetry. Runtime measurements strengthen a finding but are not mandatory when the static mechanism and scale are clear.

Match measurement to the claim:

- UI interaction or frame claim: performance trace, long tasks, and input/processing/presentation breakdown;
- layout/style claim: recalculation duration, invalidation scope, affected-node count, and representative DOM shape;
- memory claim: repeat the lifecycle, compare heap retainers, and verify counts return to a stable baseline;
- IPC/I/O claim: request rate, payload bytes, queue depth, process CPU, and cancellation behavior;
- startup claim: cold starts on representative platforms, not only warm local runs;
- cache claim: hit rate, miss duplication, retained bytes, and invalidation correctness.

Lab data explains mechanism. Field telemetry explains prevalence. A synthetic benchmark can isolate a hotspot but may exaggerate its share of total application time. Developer acceptance, code changes, and wontFix labels are useful feedback signals but are not objective proof that a finding is correct or incorrect.

Severity depends on reach and impact:

- **Critical:** likely process failure, unbounded resource exhaustion, or severe degradation for a broad/default path.
- **High:** substantial UI stalls, startup blockage, sustained CPU/GC pressure, or unbounded retention on a common path.
- **Medium:** meaningful regression under realistic but narrower scale or feature usage.
- **Low:** bounded degradation with a clear user impact; do not use this category for speculative cleanup.

## Avoid weak findings

Do not report:

- an expensive-looking API without tracing its call frequency;
- a micro-optimization with no plausible user-visible effect;
- an allocation that is rare and promptly released;
- a collection that is intentionally bounded by a clear owner;
- asynchronous code merely because it could be parallelized;
- a cache suggestion without invalidation and ownership;
- batching without flush, ordering, and failure semantics;
- an issue that exists entirely in unchanged code;
- a benchmark request without a concrete suspected mechanism;
- inefficiency confined to test fixtures, component explorers, benchmarks, or developer tooling unless the changed code affects shipped behavior or makes that infrastructure materially unusable;
- multiple layout reads as multiple forced layouts when no invalidating write occurs between them;
- bounded, self-terminating event re-entry whose follow-up work is trivial at realistic collection sizes;
- general advice to "optimize," "memoize," or "make this lazy."

When evidence is incomplete, investigate further or omit the finding.
