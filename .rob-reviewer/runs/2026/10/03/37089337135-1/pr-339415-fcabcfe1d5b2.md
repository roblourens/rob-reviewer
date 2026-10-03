# Performance review: Scope content exclusion path rules to their repository

[PR #339415](https://github.com/microsoft/vscode/pull/339415) at `fcabcfe1d5b21fd82053b9953df4f3b210ebaa9d`

## Findings (1)

### 1. [medium] Keep the retry bounded instead of enumerating every match

**Finding ID:** `PERF-08994F2426EA`  
**Location:** <code>extensions/copilot/src/platform/search/vscode-node/searchServiceImpl.ts:37</code> (<code>RIGHT</code>)  
**Performance category:** <code>latency</code>  
**Mechanism family:** <code>eager-work</code>  
**Resource:** workspace search-provider/filesystem enumeration, URI result allocations, and per-result ignore checks  
**Scaling:** When a full first page contains any excluded URI, cost jumps from the caller's maxResults M to all N workspace matches: the provider materializes N URIs and filterIngoredResources runs up to N exclusion checks (20 concurrently) before slice keeps only M.  
**Outcome:** increased latency, blocked feature initialization or tool response, and avoidable CPU/allocation pressure  
**PR causality:** <code>introduced</code>  
**Previous behavior:** The same scenario issued one workspace.findFiles2 call capped at maxResults and checked at most that bounded result page. It could return fewer allowed results, but it did not enumerate the rest of the workspace.  
**Changed behavior:** After filtering a full limited page, the added retry clears maxResults, waits for workspace.findFiles2 to return every match, filters every returned URI, and only then slices back to the caller's small limit.  
**Confidence:** 0.99 — The exact boundary call, trigger, caller limits, materialization, and N exclusion checks are directly traced in repository code and tests. Only the absolute duration varies with provider and workspace size; the change in cardinality is certain.

**Causal diff evidence:** RIGHT line 37 adds a second super.findFiles call with maxResults explicitly set to undefined only when the limited page was full and filtering removed at least one result; before this diff there was no second search.

A repository-scoped exclusion in the first page now makes bounded workflows such as maxResults: 1 test-file discovery, maxResults: 1000 dev-container discovery, or the 100,000-file workspace index scan traverse every matching file. In a large monorepo this can turn a quick lookup into a multi-second search or timeout, even when the second result would satisfy the request.

**Evidence:** BaseSearchServiceImpl delegates each call to vscode.workspace.findFiles2, which returns a materialized URI array. The added line invokes it with maxResults: undefined; the next line passes the entire array to filterIngoredResources, whose workers explicitly visit every resource before returning. Real callers use limits of 1 in extension/prompt/node/testFiles.ts, 1000 in devContainerConfigurationServiceImpl.ts, and 100,000 by default in workspaceFileIndex.ts. The new test also pins the \[1, undefined\] call sequence when the first hit is excluded.

**Suggested direction:** Retry with a progressively larger finite maxResults and stop as soon as M allowed results are found or the provider returns fewer than requested; reuse already checked results. This preserves the quota semantics without immediately expanding a one-result lookup into an unbounded workspace scan.


<details>
<summary>Reviewer scenario analysis</summary>

**Relevant issue families:** eager-work, boundary-fanout, repeated-work, cache-lifecycle, concurrency-burst

### Scenario 1: Bounded file discovery used by test/source lookup, dev-container generation, and workspace indexing

- Critical path: The caller waits for ignore-pattern initialization, one capped workspace.findFiles2 call, and filtering of that page. With an excluded URI in a full page, the diff adds a serial second workspace.findFiles2 call for all matches and waits for every exclusion check before slicing to the requested quota.
- Scaling input: Caller quota M versus total matching files N. Real M values are 1 for test/source lookup, 1,000 for dev-container scans, and 100,000 by default for workspace indexing; the regression changes fallback work from O(M) to O(N).
- Effective concurrency: The two provider searches and their filtering phases are serial. filterIngoredResources uses 20 workers; content-regex file reads are further limited to 10. Repository-rule network batches contain 10 repos with at most 5 batches concurrently.
- Cache behavior: Cold ignore initialization discovers repositories, fetches their rules, and populates repository metadata. Rules remain warm for 30 minutes and concurrent misses are coalesced. Warm rules avoid CAPI calls, and verdict memoization can reduce repeated rule evaluation, but the unbounded fallback still materializes all N URIs and dispatches an ignore check for each.
- Mechanism confidence: Very high: the added undefined limit, materialized result array, all-result filtering, real finite-limit callers, and trigger are directly visible in code and tests.
- Magnitude uncertainty: Absolute time depends on provider indexing, match selectivity, regex-rule presence, and workspace size, but monorepos with thousands to hundreds of thousands of matches are explicitly supported and tested in this subsystem.
- Expensive boundaries:
  - filesystem at <code>extensions/copilot/src/platform/search/vscode-node/searchServiceImpl.ts:29</code>: VS Code workspace.findFiles2 provider search with the caller's finite maxResults; cardinality: One capped provider call returning at most M URIs.
    - Introduced by diff: false; previous behavior: This was the only file-search boundary and remained capped at M.; critical-path effect: Must complete before the bounded lookup can return.
  - filesystem at <code>extensions/copilot/src/platform/search/vscode-node/searchServiceImpl.ts:37</code>: Second VS Code workspace.findFiles2 provider search with maxResults removed; cardinality: One additional provider call that enumerates and materializes all N matches whenever the first M results are full and at least one is excluded.
    - Introduced by diff: true; previous behavior: No retry occurred, so files beyond the initial M were not enumerated or checked.; critical-path effect: Fully blocks the lookup; all returned URIs are filtered before any M-result slice is returned.
  - filesystem at <code>extensions/copilot/src/platform/ignore/node/remoteContentExclusion.ts:259</code>: Read up to 1 KiB for content-regex exclusion checks; cardinality: Zero when no regex rules apply; otherwise up to one read per uncached returned file, bounded to 10 concurrent reads. The retry can expand this from M to N files.
    - Introduced by diff: false; previous behavior: At most the capped first-page URIs reached these checks.; critical-path effect: Each applicable read blocks that URI's ignore verdict; the filter waits for all verdicts.
  - IPC at <code>extensions/copilot/src/platform/ignore/node/remoteContentExclusion.ts:190</code>: Resolve repository fetch URLs through the git service on a repository-metadata miss; cardinality: Normally avoided for discovered repository files by the populated root cache; misses can occur per unmatched URI.
    - Introduced by diff: false; previous behavior: Only URIs in the original capped result page could encounter misses.; critical-path effect: A miss blocks that URI's exclusion verdict.
- Verdict: Reportable medium-severity eager-work regression: one excluded first-page hit can convert a bounded request into an unbounded critical-path scan and N exclusion checks.

### Scenario 2: Workspace-wide searches after repository glob rules become correctly repository-scoped

- Critical path: Ignore initialization supplies only organization-wide globs to the provider; repository globs are evaluated per returned URI by isIgnored. Broad callers therefore wait for provider enumeration and post-filtering of files that repository rules may exclude.
- Scaling input: Number of files matching the include pattern across all workspace repositories, number of repositories, and applicable per-repository patterns.
- Effective concurrency: One provider search per call; post-filtering has 20 workers. Rule fetches are coalesced and batched 10 repositories per request with at most 5 network batches active.
- Cache behavior: Compiled globs are rebuilt per fetch URL. Repository metadata, rules, and file verdicts are cached; organization and repository rules expire after 30 minutes. Warm caches avoid network and usually git IPC, while every returned URI still incurs cache lookup and scoped matcher evaluation.
- Mechanism confidence: High that provider-side filtering is reduced and per-result work increases; uncertain how common large repository-rule match sets are.
- Magnitude uncertainty: The extra post-filter cardinality depends on how many repository-scoped rules exist and how many files they match. No separate finding was submitted because this cost is inherent to correcting the prior cross-repository over-exclusion with the current global exclude API, and no independently bounded replacement path was established in the diff.
- Expensive boundaries:
  - filesystem at <code>extensions/copilot/src/platform/ignore/node/ignoreServiceImpl.ts:131</code>: Discover .git/HEAD files before building search excludes; cardinality: One workspace search per asMinimatchPattern invocation; results are repository roots.
    - Introduced by diff: false; previous behavior: The same repository discovery occurred before returning the flattened glob set.; critical-path effect: Completes before the requested file search starts.
  - IPC at <code>extensions/copilot/src/platform/ignore/node/remoteContentExclusion.ts:322</code>: Resolve fetch URLs for discovered repositories; cardinality: One git-service lookup per discovered repository during loadRepos.
    - Introduced by diff: false; previous behavior: The same preload path populated repository rules.; critical-path effect: Blocks rule preloading and therefore search-pattern construction.
  - network at <code>extensions/copilot/src/platform/ignore/node/remoteContentExclusion.ts:462</code>: Fetch content-exclusion rules from CAPI; cardinality: Cold or expired repositories only, grouped 10 per request and coalesced; at most 5 batches concurrently.
    - Introduced by diff: false; previous behavior: The same CAPI requests loaded the flattened rules.; critical-path effect: Cold rule loads complete before the search receives its exclusion pattern; warm cache removes this boundary.
  - filesystem at <code>extensions/copilot/src/platform/search/vscode-node/searchServiceImpl.ts:29</code>: Enumerate broad search matches that are later rejected by repository-scoped rules; cardinality: Proportional to matches not removable by organization-wide excludes; repository-excluded files are now returned for correct per-repository evaluation.
    - Introduced by diff: true; previous behavior: Flattened repository globs narrowed the provider search globally, but incorrectly removed matching files from unrelated repositories.; critical-path effect: Provider enumeration and per-result filtering are on the search critical path.
- Verdict: No additional finding: the scoped-rule change fixes correctness and uses compiled matchers plus warm metadata/rule caches; the separately reported unbounded retry is the avoidable amplification.

**Summary:** Completed the performance pass over all four changed files and traced search-provider, git IPC, CAPI, and conditional file-read boundaries. Submitted one medium-severity finding for the newly unbounded retry; no regression-fix learning record was warranted.

</details>

<details>
<summary>Review statistics</summary>

- Model: `gpt-5.6-sol`
- Actual model calls: `gpt-5.6-sol`
- Reasoning effort: `high`
- Actual reasoning effort: `high`
- Model API endpoint: `ws:/responses`
- Wall-clock time: 1m33.458s
- Model calls: 9
- Tokens: 258510 input, 7010 output, 265520 total; 3066 reasoning; 213691 cache read, 44792 cache write
- Aggregate model API time: 1m20.025s
- Model-returned tool calls: 38
- Copilot usage: 44974440000.000 nano-AI units; model billing multiplier: 1.000
- USD cost: unavailable; the Copilot SDK does not expose a dollar conversion.

</details>
