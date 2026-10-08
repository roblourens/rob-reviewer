# Performance review: sessions: wire cloud automations behind an experiment gate

[PR #339419](https://github.com/microsoft/vscode/pull/339419) at `fa55dd0afbedf9bf0673d74b7eac83d8b9668585`

## Findings (1)

### 1. [medium] Keep history hydration out of the mutation FIFO

**Finding ID:** `PERF-5C340CE43E83`  
**Location:** <code>src/vs/sessions/contrib/providers/copilotChatSessions/browser/githubCloudAutomationStore.ts:219</code> (<code>RIGHT</code>)  
**Performance category:** <code>latency</code>  
**Mechanism family:** <code>boundary-fanout</code>  
**Resource:** GitHub network requests queued ahead of user-triggered mutation requests  
**Scaling:** The blocking work grows with the effective cloud catalogue: one listRuns request for every loaded definition, plus one tasks.get request for each active task among up to 50 returned runs per definition; only four definitions progress concurrently.  
**Outcome:** Run/Create/Update/Delete can remain queued for seconds or longer after the UI already reports the catalogue ready, making user actions appear hung and reducing mutation throughput.  
**PR causality:** <code>introduced</code>  
**Previous behavior:** Before this change the provider-local store only refreshed definitions and had neither automatic history hydration nor a shared FIFO that could place history reads ahead of mutations.  
**Changed behavior:** CloudAutomationStore now starts refreshHistory immediately after definition refresh, while refreshHistory and every mutation both queue on the same Sequencer. Definition refresh publishes \`ready\` before history starts, so controls become enabled even though the full history fan-out is already ahead of subsequent user work.  
**Confidence:** 0.98 — The queue identity, ordering, ready-state transition, automatic initial call path, network cardinality, and four-way limiter are all explicit in changed code and tests. Exact request duration and typical catalogue size vary, which affects severity but not the blocking mechanism.

**Causal diff evidence:** Added line 219 places the new history refresh on \`this.operations\`; added line 257 places mutations on the identical sequencer, and added lines 228-235 perform the scaling network fan-out before that queue item completes.

With a realistic account containing multiple cloud automations, enabling the feature or rotating the account/client enables the cards and then issues per-definition history reads. A user clicking Run or editing a card during that period waits behind all of those reads; the added test demonstrates 10 definitions with four long-lived detail reads occupying the limiter.

**Evidence:** \`CloudAutomationStore.refresh\` awaits definition refresh and then calls \`store.refreshHistory()\` (cloudAutomationStore.ts:95-109). \`refreshRepositories\` sets catalogue state to \`ready\` before returning (githubCloudAutomationStore.ts:338-344). This changed line queues history on \`operations\`; lines 225-235 fan out \`listRuns\` and active-task \`tasks.get\` calls, while \`mutate\` at lines 257-273 queues Run/Create/Update/Delete on that same FIFO. The test at githubCloudAutomationStore.test.ts:304-319 creates 10 definitions and verifies four blocked detail requests, establishing both reachability and concurrency.

**Suggested direction:** Use a separate cancellable/coalesced history-read lane, or otherwise prioritize admitted mutations, while publishing history only if its captured catalogue/account generation is still current. Keep definition refresh and writes serialized where ordering requires it, but do not make user mutations wait for the whole history window.


<details>
<summary>Reviewer scenario analysis</summary>

**Relevant issue families:** boundary-fanout, eager-work, missing-coalescing, concurrency-burst, cache-lifecycle, cleanup-lifecycle

### Scenario 1: Enable cloud automations, or rotate the default account/GitHub client, and load the cloud catalogue/history

- Critical path: The adapter starts refresh asynchronously, so application startup itself does not await it. Definition discovery is on the cloud-card readiness path; history is on the history-section completion path and is started immediately after definitions publish ready.
- Scaling input: Effective unique remembered/recent repositories R, loaded definitions D, and active tasks T; network work is approximately repository discovery plus D history-list calls plus T detail calls.
- Effective concurrency: Recent resolution and repositories are serial. Definition details use batches of 5 per repository. History limits active definition jobs to 4, with task details serial inside each job. All refreshes, history reads, and mutations share one Sequencer.
- Cache behavior: Repository references persist per account and are deduplicated case-insensitively; definitions/history are memory-only and cold after account/client/gate transitions. A client lease is reused within one store lifetime. Concurrent definition refreshes coalesce via refreshPromise; history refreshes have no analogous coalescing promise.
- Mechanism confidence: High: automatic invocation, exact request loops, cache lifetime, and concurrency are explicit in changed code and tests.
- Magnitude uncertainty: The feature defaults off and exact repository/definition/task counts and network latency vary. Remembered repositories can outlive the 10-item recent list, so larger long-lived catalogues remain realistic.
- Expensive boundaries:
  - filesystem at <code>src/vs/sessions/contrib/providers/copilotChatSessions/browser/copilotChatSessionsProvider.ts:1135</code>: Resolve each local recent workspace through its Git configuration to a GitHub repository URI; cardinality: At most 10 Agents-owned recent workspaces, processed sequentially; remote GitHub workspace URIs skip this boundary.
    - Introduced by diff: true; previous behavior: The resolver existed for other provider work but was not wired into an automatically refreshed cloud automation adapter.; critical-path effect: Blocks definition discovery for later repositories because recent resolution is sequential, but does not block overall application startup.
  - network at <code>src/vs/sessions/contrib/providers/copilotChatSessions/browser/githubCloudAutomationStore.ts:315-327,349-369</code>: Check repository visibility, list definition pages, and hydrate each definition detail; cardinality: Per effective unique repository: one visibility request, up to 10 list pages of 100 summaries, and one detail request per definition in batches of 5. Repositories are the deduplicated union of up to 10 recents and all remembered eligible repositories.
    - Introduced by diff: true; previous behavior: The low-level explicit refresh path pre-existed, but this PR's adapter invokes it automatically when the enabled account/client lifetime is created.; critical-path effect: All repositories are processed serially before cloud catalogue refresh completes and state settles ready/error.
  - network at <code>src/vs/sessions/contrib/providers/copilotChatSessions/browser/githubCloudAutomationStore.ts:228-235</code>: List up to 50 runs per definition and fetch authoritative detail for every active run; cardinality: One listRuns request per loaded definition plus one tasks.get per queued/running/waiting task in each 50-run window; at most four definitions execute concurrently.
    - Introduced by diff: true; previous behavior: No cloud run-history read existed.; critical-path effect: Does not delay definition state becoming ready, but occupies the shared operations FIFO before subsequent user mutations and delays history publication until all definitions finish.
- Verdict: Reportable because the introduced history fan-out uses the mutation FIFO after catalogue readiness, delaying interactive operations.

### Scenario 2: Run, create, update, delete, or stop a cloud automation after the catalogue reports ready

- Critical path: The user operation must enter the store's global Sequencer, revalidate repository visibility, then perform the requested remote mutation. During initial/account refresh it first waits for the already-queued complete history hydration.
- Scaling input: Mutation latency scales with the queued history window D + T, then with its own two or three network round trips.
- Effective concurrency: Global serialization: one definition refresh, history refresh, or mutation executes at a time at the outer Sequencer level; history internally fans out four definition jobs.
- Cache behavior: The GitHub client/credential lease is reused, but repository eligibility is deliberately re-read for every mutation. Indeterminate writes are negatively gated until a successful definition refresh reconciles state.
- Mechanism confidence: High: ready-state ordering and shared FIFO identity are explicit, and the test demonstrates long-lived history detail requests at the configured concurrency.
- Magnitude uncertainty: Users may not act immediately after enablement, but the UI publishes ready before history completes, making the overlap reachable; network duration varies.
- Expensive boundaries:
  - network at <code>src/vs/sessions/contrib/providers/copilotChatSessions/browser/githubCloudAutomationStore.ts:257-273</code>: Revalidate private repository eligibility and dispatch the requested automation/task mutation; cardinality: Normally two network operations per mutation: one getRepository eligibility read and one create/get+update/delete/dispatch/abort path; update uses preflight get and may therefore use three.
    - Introduced by diff: true; previous behavior: Cloud mutations were not implemented, and no history read could queue ahead of them.; critical-path effect: Directly blocks the user's operation; additionally waits behind every earlier history request because both use the same FIFO.
- Verdict: Reportable interactive latency regression; history should use a separate generation-checked/coalesced read lane or permit mutation priority.

### Scenario 3: Render and observe the combined automation cards and run history

- Critical path: Observable projections map cloud entries/runs, the provider aggregate flattens and sorts them, and the Automations view synchronously renders cards/history after each published snapshot.
- Scaling input: Loaded definitions A and projected runs R, with cloud history capped at 50 runs per definition for one refresh window.
- Effective concurrency: Synchronous observable recomputation/rendering on snapshot publication; no subprocess, IPC, filesystem, database, or network boundary occurs in this rendering path.
- Cache behavior: Derived observables cache while observed; cloud snapshots replace arrays atomically. Unsupported states are filtered during projection.
- Mechanism confidence: High for the synchronous mapping/sorting path; low for material user-visible cost at realistic cardinality.
- Magnitude uncertainty: Very large remembered catalogues could produce substantial arrays, but no changed per-frame/event loop or measured rendering mechanism establishes a separate high-confidence regression.
- Expensive boundaries: none found
- Verdict: No additional finding: the changed projection work is snapshot-driven and the diff does not prove a distinct hot rendering regression.

**Summary:** Completed the performance pass over all 26 changed files and paginated large diffs. One medium-confidence-severity finding was submitted for per-definition history network fan-out blocking interactive cloud mutations through a shared FIFO. Automatic discovery, cache lifetimes, concurrency bounds, cancellation/disposal, aggregate rendering, and all transitive filesystem/network boundaries were also checked; no regression-fix learning signal was warranted.

</details>

<details>
<summary>Review statistics</summary>

- Model: `gpt-5.6-sol`
- Actual model calls: `gpt-5.6-sol`
- Reasoning effort: `high`
- Actual reasoning effort: `high`
- Model API endpoint: `ws:/responses`
- Wall-clock time: 2m51.648s
- Model calls: 15
- Tokens: 1032937 input, 10114 output, 1043051 total; 4389 reasoning; 925990 cache read, 106902 cache write
- Aggregate model API time: 2m31.096s
- Model-returned tool calls: 79
- Copilot usage: 110736600000.000 nano-AI units; model billing multiplier: 1.000
- USD cost: unavailable; the Copilot SDK does not expose a dollar conversion.

</details>
