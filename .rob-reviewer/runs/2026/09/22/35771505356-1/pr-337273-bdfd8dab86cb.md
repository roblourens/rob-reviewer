# Performance review: agentHost: avoid recomputing unchanged turn diffs

[PR #337273](https://github.com/microsoft/vscode/pull/337273) at `bdfd8dab86cb6eadf543b54db0a280d493a5cf42`

## Findings

No high-confidence performance findings.

## Performance regression fixes detected (1)

### 1. repeated-work

**Fix location:** `src/vs/workbench/contrib/chat/browser/agentSessions/agentHost/agentHostResponseFileChanges.ts:317` (`RIGHT`)  
**Blamed base line:** `src/vs/workbench/contrib/chat/browser/agentSessions/agentHost/agentHostResponseFileChanges.ts:355`  
**Introducing path:** `src/vs/workbench/contrib/chat/browser/agentSessions/agentHost/agentHostResponseFileChanges.ts`  
**Introducing commit:** `4c0e33c44aa57eb46c795e44eca940308d49e3cf`  
**Mechanism family:** `repeated-work`  
**Performance category:** `cpu`  
**Confidence:** 0.98

**Mechanism:** Per-request observables depended on the full chat state, so every streamed token update caused each historical request observer to scan chats/history and potentially reparse response edits. Synchronous work scaled with streamed updates times observed historical requests times history lookup length.

**Symptom:** Long Agent Host chats could spend seconds to tens of seconds synchronously processing token streams, causing severe CPU usage and UI responsiveness degradation as history and update count grew.

**Fixed behavior:** The provider now shares one lazy session/chat source, projects completed turns and the active turn into separate observables at the subscription event boundary, and indexes completed turns by ID only when the history array changes. Active token updates therefore notify only the active-turn path; historical response summaries retain identity and perform no history lookup or edit parsing.

**Evidence:** Before this PR, every rendered request created its own chat-state observable and a derived callback that reacted to every chat subscription change, then walked the chat list/history to locate that request and parsed its response edits. Chat response summaries and timeline entries observe one provider result per request, while streamed token/reasoning actions update the same chat subscription. Thus each token update invalidated all historical request observers and repeated history lookup/parsing; the added regression test at lines 402-450 proves the fixed path now keeps five historical observers at one run across 100 streamed updates, with no additional response/edit reads. The changed projection at lines 301-318 separates completed turns from active-turn updates, and line 317 indexes history once only when the turns array changes.


<details>
<summary>Reviewer scenario analysis</summary>

**Relevant issue families:** repeated-work, cache-lifecycle, cleanup-lifecycle, boundary-fanout, concurrency-burst, serialization-allocation

### Scenario 1: A long Agent Host chat/timeline renders many completed turn summaries while the active turn streams text, reasoning, and occasional file edits.

- Critical path: Each chat subscription event is processed synchronously before observers and UI summaries can update. The diff moves completed-turn lookup behind a turns-array projection, so ordinary active-turn token updates no longer invalidate historical summary observers; only the active turn's relevant response-edit changes reach normalization and aggregation.
- Scaling input: Let H be completed turns, R observed historical summaries, C discovered chats, and U streamed updates. Before the fix, update work included repeated per-request history lookup/parsing and scaled roughly with U × R × H in the common single-chat case. After the fix, token updates affect the active projection only; completed-turn indexing is O(H) only when history changes, while historical lookup is O(1) per request after indexing.
- Effective concurrency: One shared session subscription and one shared subscription per discovered chat are observed concurrently, plus one existing per-turn changeset subscription for each rendered request. Stream processing itself is synchronous; there is no newly introduced task fan-out or queue.
- Cache behavior: Per-request, session-source, and chat-source caches are bounded to 1,000 entries. Cold observation builds the shared projection and completed-turn map; warm requests reuse sources and indexed turns. Empty/error states remain distinct, and failed shared subscriptions retry only when a new or restarted request observation begins rather than polling.
- Mechanism confidence: High: the old and new observable dependencies are explicit in the diff, UI consumers observe one result per rendered request, reducer-driven chat events are synchronous, and the added instrumented tests show no historical observer reruns or edit reads across streamed updates.
- Magnitude uncertainty: Exact end-to-end frame impact varies with rendered history, token cadence, changeset readiness, and hardware, but the removed multiplicative synchronous work is directly established.
- Expensive boundaries:
  - network at <code>src/vs/workbench/contrib/chat/browser/agentSessions/agentHost/agentHostResponseFileChanges.ts:\_createSharedSource</code>: Acquire the Agent Host session and chat protocol subscriptions used to project session metadata and turns.; cardinality: Cold observation acquires one session resource and one subscription per effective chat URI; the subscription manager coalesces all request observers for the same resource. Warm observers reuse the shared source and do not issue another server subscribe.
    - Introduced by diff: false; previous behavior: The same session/chat resources were subscribed for each per-request observable, but AgentSubscriptionManager coalesced identical resource subscriptions at the wire boundary; the PR reduces local references rather than increasing network calls.; critical-path effect: Initial snapshots are needed before cold summaries can resolve; streamed updates then arrive over the existing subscriptions and do not add boundary calls.
  - network at <code>src/vs/workbench/contrib/chat/browser/agentSessions/agentHost/agentHostResponseFileChanges.ts:\_subscribeChangeset</code>: Subscribe to the authoritative per-turn changeset, and for the migration-boundary turn optionally the branch changeset.; cardinality: One unique turn changeset resource per observed rendered request; the branch resource is only used for the single recorded migrated turn. Identical resource references are coalesced by AgentSubscriptionManager.
    - Introduced by diff: false; previous behavior: The same per-turn and conditional branch subscriptions existed before the diff with the same effective resource cardinality.; critical-path effect: A cold turn summary waits for its changeset snapshot; streaming uses existing subscriptions and does not resubscribe.
- Verdict: No reportable regression. This scenario is the concrete repeated-work regression fixed by the PR; the introducing commit was recorded separately.

### Scenario 2: A response summary or prompt timeline is first opened, reobserved after disposal/reconnect, or resolves turns across a default and peer chat.

- Critical path: The UI must acquire session/chat snapshots, discover effective chat URIs, locate the requested turn, and resolve its authoritative changeset. On a warm observation, cached source definitions reacquire lazily and current snapshots are indexed; unobserved lookups acquire no subscription.
- Scaling input: Cold work scales with C effective chat resources and R visible/observed turn changesets, not with duplicate summary consumers. Turn lookup now indexes H history entries once per history-array change rather than linearly scanning H for every request observer.
- Effective concurrency: Unique session/chat resources are shared across all request observers. Different turn changesets can hydrate concurrently as before; no new global burst is introduced, and the manager coalesces duplicate resource acquisitions.
- Cache behavior: The four LRUs cap observable/source definitions at 1,000 each. Eviction does not cancel live observables held by UI owners; inactive entries hold no server subscription. Completed-turn maps are rebuilt from the latest snapshot after reacquisition, so no stale positive or negative cache is pinned.
- Mechanism confidence: High: lazy acquisition and last-observer disposal are explicit, manager refcounting/coalescing was inspected, and tests cover sharing, disposal, cache bounds, peer discovery, pending hydration, replacement snapshots, and reobservation.
- Magnitude uncertainty: The number of simultaneously visible summaries and peer chats varies by UI state, but resource cardinality and lifecycle are established.
- Expensive boundaries:
  - network at <code>src/vs/platform/agentHost/common/state/agentSubscription.ts:AgentSubscriptionManager.getSubscription (called by agentHostResponseFileChanges.ts)</code>: Request initial snapshots and maintain Agent Host subscriptions for session, chat, turn changeset, and the narrowly gated branch fallback.; cardinality: For a session with C effective chats and R rendered turns: one session, up to C chats, and up to R unique turn changesets; duplicate consumers share resources. Reobservation after the last observer leaves reacquires those resources, matching the lazy lifecycle.
    - Introduced by diff: false; previous behavior: Before the PR the same unique protocol resources were required. Local duplicate references were more numerous, but wire subscriptions were already coalesced.; critical-path effect: Cold/reobserved summaries require snapshots; cached source closures alone perform no network I/O. Branch fallback is only critical for the one migrated boundary turn.
- Verdict: No reportable regression. New caches are bounded and lazy, network cardinality is not increased, and the changed indexing reduces CPU work.

### Scenario 3: A session or chat subscription fails, then another request starts observing and triggers recovery.

- Critical path: The first failed source yields no result; a later observation checks the shared subscription error once, triggers source replacement, and waits for a fresh snapshot. Existing observers follow the replaced shared source.
- Scaling input: Retry cost scales with explicit new/restarted observations during an error interval, not streamed tokens or cache lookups. Tests cover repeated failure, external replacement, pending hydration, and cleanup.
- Effective concurrency: One retry is initiated per new/restarted observation while the current shared subscription is errored. The source is shared and AgentSubscriptionManager evicts the errored resource before reacquisition; there is no timer, polling loop, or unbounded retry fan-out.
- Cache behavior: Errors are not treated as successful cached snapshots. The cache retains the source wrapper, while acquisition detects the current error and replaces the underlying subscription only on demand; successful and pending subscriptions are shared.
- Mechanism confidence: High: trigger placement, absence of polling, manager eviction semantics, and recovery lifecycle tests directly establish the mechanism.
- Magnitude uncertainty: Failure prevalence and server retry latency are environment-dependent.
- Expensive boundaries:
  - network at <code>src/vs/workbench/contrib/chat/browser/agentSessions/agentHost/agentHostResponseFileChanges.ts:\_createSharedSource</code>: Retry an errored session or chat subscription when a new observation begins.; cardinality: One server subscribe attempt for the triggering observation/source replacement; unobserved cache lookups do not retry, and successful replacement is shared by existing and new observers.
    - Introduced by diff: true; previous behavior: The old failed observable could remain poisoned; it did not provide this explicit demand-driven recovery path.; critical-path effect: Recovery waits for the replacement snapshot, but retries are intentionally demand-driven and bounded by observation starts.
- Verdict: No reportable regression. The added boundary call is a bounded, user-triggered recovery operation rather than an uncontrolled retry or concurrency burst.

**Summary:** Completed the performance pass over the full two-file diff. No diff-introduced performance findings met the reporting threshold. The PR removes a proven synchronous repeated-work regression by separating active-turn updates from completed history, sharing lazy subscriptions, selectively mapping edits, and bounding all new caches; the regression fix and introducing commit were recorded.

</details>

<details>
<summary>Review statistics</summary>

- Model: `gpt-5.6-sol`
- Actual model calls: `gpt-5.6-sol`
- Reasoning effort: `high`
- Actual reasoning effort: `high`
- Model API endpoint: `ws:/responses`
- Wall-clock time: 2m19.731s
- Model calls: 11
- Tokens: 489738 input, 8092 output, 497830 total; 3243 reasoning; 420651 cache read, 69054 cache write
- Aggregate model API time: 1m58.35s
- Model-returned tool calls: 56
- Copilot usage: 67550240000.000 nano-AI units; model billing multiplier: 1.000
- USD cost: unavailable; the Copilot SDK does not expose a dollar conversion.

</details>
