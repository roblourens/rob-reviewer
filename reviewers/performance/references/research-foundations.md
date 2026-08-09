# External research foundations

This document records external sources that informed the generic performance guide and reviewer workflow. It is provenance, not a checklist. Apply the principles only after tracing the reviewed code's actual path, scale, and lifecycle.

## LLM code-review systems

No strong source found a mature LLM reviewer dedicated specifically to performance regressions. The closest evidence comes from general and security-focused review agents, whose problems of context selection, grounding, false positives, calibration, and evaluation closely match performance review.

### Use bounded, change-centered context

- [Does AI Code Review Lead to Code Changes?](https://arxiv.org/abs/2508.18771) analyzed more than 22,000 comments across 178 repositories. Hunk-level review tools, concise comments, and comments containing code snippets were more likely to lead to code changes.
- [An Insight into Security Code Review with LLMs](https://arxiv.org/abs/2401.16310) reports that model performance degrades with larger files/token counts and changes with prompt design.
- [PRWeaver](https://arxiv.org/abs/2608.02693) found substantial detection degradation when many benign PRs shared one context window.

The reviewer should start from changed hunks and fetch only the surrounding code needed to prove reachability, ownership, scale, and cleanup. Full-repository access is valuable as a tool, not as eagerly loaded prompt content.

### Route review effort to relevant issue families

[iCodeReviewer](https://arxiv.org/abs/2510.12186) uses a mixture of prompt experts and activates only experts relevant to features found in the input. Its authors report that routing reduces hallucination-driven false positives and improves coverage.

For performance review, changed-file types and code features should determine emphasis:

- CSS/DOM/rendering changes: layout, selector scope, animation, DOM growth;
- logging/protocol/storage changes: batching, serialization, IPC, backpressure;
- constructors/disposal changes: listeners, observers, caches, retained ownership;
- async/scheduling changes: cancellation, duplicate in-flight work, stale results;
- startup/contribution changes: eager module load, synchronous work, late global invalidation.

This is prioritization, not exclusion. Cross-cutting bugs still require following the actual call chain.

### Generate structured findings, then filter

- [RovoDev Code Reviewer](https://arxiv.org/abs/2601.01129) describes a review-guided, context-aware, quality-checked pipeline and reports that 38.7% of generated comments triggered subsequent code changes in its deployment.
- [Sphinx](https://arxiv.org/abs/2601.04252) evaluates review quality through structured verification checklists rather than prose similarity.
- Google's [ML code-review comment resolution](https://research.google/blog/resolving-code-review-comments-with-ml/) traded quantity for quality through confidence filtering and curated feedback.

The framework therefore uses a typed finding tool, changed-line validation, a confidence threshold, deduplication, a finding cap, and host-side publication. Confidence also needs a rationale so human adjudicators can distinguish traced facts from assumptions.

### Evaluate with replay and human adjudication

- [Understanding the Limits of Automated Evaluation for Code Review Bots](https://arxiv.org/abs/2604.24525) found only moderate alignment (approximately 44% to 62%) between LLM evaluators and developer fixed/wontFix labels. Workflow pressure and prioritization make developer action an imperfect quality label.
- [Does AI Code Review Lead to Code Changes?](https://arxiv.org/abs/2508.18771) uses subsequent edits as a useful behavioral proxy, but not proof of correctness.
- [Watts This Smell](https://arxiv.org/abs/2604.04809) builds a large inefficiency taxonomy from profiled, functionally equivalent code pairs and finds that multiple smells commonly co-occur.

Maintain a labeled replay corpus containing:

- confirmed regression-inducing PRs;
- nearby clean PRs in the same subsystems;
- previously rejected false positives;
- findings validated through traces, profiles, or later fixes.

Run replays multiple times because model output is stochastic. Track precision and recall by issue family, not only aggregate acceptance. Human technical adjudication remains the final quality signal.

### Treat PR narrative as untrusted

[SEVRA-BENCH](https://arxiv.org/abs/2606.13757) shows that social-engineering framing in PR text can reduce review-agent detection. Titles, descriptions, comments, and repository documents must remain untrusted technical context and cannot override reviewer policy.

## Browser and Electron performance

### Interaction budgets and long tasks

- web.dev's [Optimize INP](https://web.dev/articles/optimize-inp) defines good Interaction to Next Paint as 200 ms or less at the 75th percentile and separates interaction latency into input delay, processing duration, and presentation delay.
- web.dev's [Optimize long tasks](https://web.dev/articles/optimize-long-tasks) uses 50 ms as the long-task threshold and discusses yielding long work.
- web.dev's [Rendering performance](https://web.dev/articles/rendering-performance) notes that a 60 Hz frame nominally has 16.7 ms, with roughly 10 ms available after browser overhead.

These are diagnostic budgets, not automatic static-analysis findings. VS Code/Electron does not publish Core Web Vitals in the same way as a website, but renderer input and frame deadlines still follow Chromium's main-thread model.

### Layout, DOM scope, and containment

- [Avoid large, complex layouts and layout thrashing](https://web.dev/articles/avoid-large-complex-layouts-and-layout-thrashing) explains that layout cost grows with affected DOM size and that style reads after invalidating writes force synchronous layout.
- [content-visibility](https://web.dev/articles/content-visibility) can skip style/layout/paint work for offscreen subtrees; `contain-intrinsic-size` is needed to avoid scroll-size jumps.
- [Virtualize large lists](https://web.dev/articles/virtualize-long-lists-react-window) recommends bounding rendered DOM rather than append-only infinite lists.

Containment and virtualization alter measurement, accessibility, sticky positioning, and lifecycle behavior. Recommend them only when those contracts are understood.

### Events, observers, and scheduling

- Chrome's [passive scrolling intervention](https://developer.chrome.com/blog/scrolling-intervention) explains why touch/wheel listeners that never call `preventDefault` should be passive.
- [ResizeObserver](https://web.dev/articles/resize-observer) and [IntersectionObserver](https://web.dev/articles/intersectionobserver-v2) replace broad polling or scroll-based geometry checks, but callbacks still need bounded work and teardown.
- [Optimize long tasks](https://web.dev/articles/optimize-long-tasks) recommends yielding non-critical work; modern `scheduler.yield()` preserves continuation priority better than an arbitrary zero-delay timeout.

### Animation and compositing

[Stick to compositor-only properties and manage layer count](https://web.dev/articles/stick-to-compositor-only-properties-and-manage-layer-count) recommends `transform` and `opacity` for animation while warning that excessive layer promotion consumes GPU memory and can reduce performance.

### Resource loading

- [Browser-level image lazy loading](https://web.dev/articles/browser-level-image-lazy-loading) recommends lazy loading below-fold images but not likely-LCP images, and reserving dimensions to avoid layout shifts.
- [Fetch Priority](https://web.dev/articles/fetch-priority) explains priority hints for critical versus deferred resources.
- [Font best practices](https://web.dev/articles/font-best-practices) covers WOFF2, font discovery, preconnect, and `font-display` tradeoffs.

These signals matter most inside webviews, browser views, documentation previews, extension UIs, and remote content. They are usually irrelevant to packaged local workbench assets.

### Off-main-thread work and Electron boundaries

- [Off-main-thread architectures](https://web.dev/articles/off-main-thread) recommends workers for CPU-heavy work without DOM dependencies and notes that structured cloning can itself be expensive; transferable buffers avoid copies.
- Electron's [Performance guide](https://www.electronjs.org/docs/latest/tutorial/performance) emphasizes measurement, avoiding unnecessary module load, deferring code until needed, and not blocking the main or renderer process.

Moving work to another process is not free. Review message frequency, serialization size, transfer semantics, cancellation, and whether the destination process is itself shared and latency-sensitive.

## Measurement cautions

Static review should identify a concrete mechanism and realistic multiplier, then recommend measurement appropriate to the claim:

- interaction or frame claim: Chromium performance trace and long-task attribution;
- style/layout claim: style recalculation, layout duration, invalidation scope, affected nodes;
- memory claim: repeated lifecycle scenario and heap-retainer comparison;
- IPC/I/O claim: request rate, payload bytes, queue depth, and process CPU;
- startup claim: cold-start samples across representative platforms;
- cache claim: hit rate, retained bytes, invalidation correctness, and miss duplication.

Lab measurements reveal mechanism; field telemetry reveals prevalence. Neither alone proves the other.
