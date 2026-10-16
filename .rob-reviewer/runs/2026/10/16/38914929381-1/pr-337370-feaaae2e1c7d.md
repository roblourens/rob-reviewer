# Performance review: agentHost: apply pending configuration before MCP startup

[PR #337370](https://github.com/microsoft/vscode/pull/337370) at `feaaae2e1c7d4ec74f79db3aa9f7eb7a4212ea98`

## Findings (1)

### 1. [medium] Retry failed plugins without reprocessing healthy siblings

**Finding ID:** `PERF-18132C7AB659`  
**Location:** <code>src/vs/platform/agentHost/node/copilot/copilotAgent.ts:6845</code> (<code>RIGHT</code>)  
**Performance category:** <code>io</code>  
**Mechanism family:** <code>repeated-work</code>  
**Resource:** filesystem exists/copy/cache-write operations and plugin parsing CPU/allocations for healthy plugin trees  
**Scaling:** Cost grows with the total number and size of a client's plugins (N), multiplied by every send or MCP Start while any one plugin remains errored, rather than with only the errored subset (E).  
**Outcome:** increased critical-path latency and repeated filesystem I/O/CPU parsing  
**PR causality:** <code>materially-amplified</code>  
**Previous behavior:** The retry constructed inputs only from errored customizations, so a client with one failed plugin and many healthy siblings retried and reparsed only that failed subset at each message boundary.  
**Changed behavior:** When one customization is in Error, every message boundary and MCP Start now re-syncs the client's entire customization contribution, including all healthy plugins, and reparses every returned plugin directory.  
**Confidence:** 0.99 — The send/Start critical path calls \`retryFailedClientSyncIfNeeded\`, the changed method explicitly supplies all client inputs, and the added tests assert that both the failed and healthy plugin are supplied on retry. \`AgentPluginManager.syncCustomizations\` performs per-input filesystem work and \`resolveSyncedCustomization\` calls \`parsePlugin\`, which reads plugin manifests and component directories. Only the typical number/size of installed plugins is uncertain, affecting severity rather than mechanism.

**Causal diff evidence:** RIGHT line 6845 changes the retry input from \`errored.map(item =&gt; item.input\!)\` to the client's complete \`client.inputs\`; RIGHT line 6723 simultaneously bypasses the identical-input fast path whenever any item is errored, so the full list reaches \`syncCustomizations\` rather than being deduplicated.

With N client plugins and one persistent/transient failure, each affected prompt or MCP Start incurs O(N) filesystem synchronization and full plugin parsing on its critical path instead of O(number of failed plugins), increasing prompt/start latency and filesystem/CPU load; malformed or unavailable plugins make this recur at every message boundary.

**Evidence:** \`\_refreshSessionConfiguration\` awaits \`retryFailedClientSyncIfNeeded()\` before a send or Start can proceed. The changed retry uses all \`client.inputs\`; \`AgentPluginManager.\_syncPlugin\` performs copy work on uncached/no-nonce inputs, or at minimum an existence check plus cache-file persistence on nonce hits, and every result then runs \`parsePlugin\`, which reads the manifest, hooks, MCP definitions, skills, agents, and instructions. The new test \`Send retries failed sync without dropping healthy plugins\` confirms the retry call includes \`\[pluginDirectory, healthyDirectory\]\`, not only the failed directory.

**Suggested direction:** Preserve the existing healthy client slice while retrying only errored inputs—for example, add a targeted retry/merge path that replaces results by URI instead of making \`sync\` replace the whole client contribution. This retains healthy siblings without sending them back through plugin synchronization and parsing.


<details>
<summary>Reviewer scenario analysis</summary>

**Relevant issue families:** repeated-work, boundary-fanout, missing-coalescing, cleanup-lifecycle, concurrency-burst

### Scenario 1: Send a chat message while one client plugin is in an error state

- Critical path: \`sendMessage\` enters the per-chat queue and awaits \`retryFailedClientSyncIfNeeded\`, plugin synchronization, resolution/parsing, snapshot comparison, and any required session refresh before calling the SDK send. All of this preparation is on prompt latency's critical path.
- Scaling input: Total client-contributed plugins and their manifest/component tree sizes, multiplied by prompts while any customization remains errored.
- Effective concurrency: Messages are serialized per chat. Within one client sync, all N plugin inputs are synchronized in parallel (serialized only by plugin URI); clients containing errors are retried sequentially. The parallel fan-out does not reduce aggregate filesystem/parsing work.
- Cache behavior: Successful healthy plugins may hit the nonce materialization cache, but each retry still checks the directory, persists the shared cache metadata, and reparses the plugin tree; inputs without a nonce can be recopied. Error states are deliberately retried at every message boundary, so persistent failures receive no negative-cache/backoff behavior.
- Mechanism confidence: High: the changed full-input array, send call path, filesystem implementation, parser, and new test assertions establish the mechanism and O(N) cardinality directly.
- Magnitude uncertainty: The installed plugin count and tree sizes vary by user, but a single unavailable or malformed plugin is a realistic persistent trigger; exact added milliseconds are unknown.
- Expensive boundaries:
  - filesystem at <code>src/vs/platform/agentHost/node/copilot/copilotAgent.ts:6845 -&gt; AgentPluginManager.syncCustomizations/resolveSyncedCustomization</code>: Materialize or validate each plugin cache entry, persist cache metadata, and parse plugin manifests/components.; cardinality: After the diff, N sync operations for every plugin contributed by a client whenever at least one is errored; previously E operations for only the errored subset, where E is commonly 1 and N may include many healthy plugins.
    - Introduced by diff: true; previous behavior: Only errored inputs were re-synchronized and parsed on a message-boundary retry.; critical-path effect: All syncs and subsequent plugin parses must complete before the prompt reaches the SDK.
  - IPC at <code>src/vs/platform/agentHost/node/copilot/copilotAgent.ts:\_sendMessageOnce -&gt; CopilotAgentSession.send</code>: Send the user prompt to the Copilot SDK session.; cardinality: One SDK send per prompt after preparation.
    - Introduced by diff: false; previous behavior: The same SDK send occurred, but healthy sibling plugins were not reprocessed during failed-plugin retry.; critical-path effect: Starts the actual turn only after configuration preparation completes.
- Verdict: Reportable medium-severity repeated filesystem and parsing work: the diff expands each retry from the failed subset to every healthy sibling on the prompt critical path.

### Scenario 2: Start an MCP server with pending plugin configuration

- Critical path: MCP Start is first prepared in the per-chat queue. It awaits failed-plugin retry, snapshot/enablement comparison, and—when configuration changed—SDK disconnect and session resume; after preparation leaves the chat queue, the requested server's Start RPC must complete. During an active turn, refresh waits for idle and cancellation can end the caller wait.
- Scaling input: Number/size of client plugins for retry work; one session restart and one Start RPC per request that observes pending configuration.
- Effective concurrency: Preparation is serialized per chat. The MCP Start RPC runs outside the chat queue and is sequenced per server name, so different server RPCs can proceed concurrently; AgentSideEffects supersedes and cancels duplicate starts for the same session/server.
- Cache behavior: Preparation uses the same plugin cache behavior as sends. Snapshot data is rebuilt rather than memoized, while customization enablement session initialization is internally reusable. A pending configuration change causes one refreshed live SDK runtime; unchanged configuration avoids disconnect/resume.
- Mechanism confidence: High: queue placement, idle handling, refresh boundaries, per-server sequencing, and test gates explicitly establish ordering and concurrency.
- Magnitude uncertainty: Session restart and plugin synchronization duration depend on runtime and plugin size. The refresh itself is intentional; the reported regression is specifically reprocessing healthy siblings.
- Expensive boundaries:
  - filesystem at <code>src/vs/platform/agentHost/node/copilot/copilotAgent.ts:4317,6845</code>: Retry plugin materialization and parse all contributed plugin trees.; cardinality: N plugin sync/parse operations if any client plugin is errored, versus E failed-plugin operations before this diff's full-input retry.
    - Introduced by diff: true; previous behavior: MCP Start did not prepare session configuration; the pre-existing message retry path processed only errored inputs.; critical-path effect: Blocks configuration preparation and therefore delays server Start.
  - IPC at <code>src/vs/platform/agentHost/node/copilot/copilotAgent.ts:\_refreshSessionConfiguration</code>: Disconnect the old Copilot SDK session and resume/create the configured replacement.; cardinality: Zero when no refresh reason exists; otherwise one SDK disconnect and one resume/create operation for the addressed session.
    - Introduced by diff: true; previous behavior: Start directly targeted the current runtime and did not refresh pending configuration.; critical-path effect: Required to apply pending configuration before Start; Start waits for replacement runtime registration.
  - IPC at <code>src/vs/platform/agentHost/node/copilot/copilotAgentSession.ts:startMcpServer</code>: Invoke \`session.rpc.mcp.startServer\` for the resolved server name.; cardinality: One SDK MCP Start RPC per effective Start request; duplicate same-server requests cancel the older host request, although an already-running SDK operation may finish.
    - Introduced by diff: false; previous behavior: One Start RPC was issued directly against the current session without configuration preparation.; critical-path effect: The Start action completes only when this RPC settles; it is intentionally outside the chat queue.
- Verdict: The preparation/restart behavior is intentional and bounded, but its full-client failed-plugin retry shares the reported O(N) regression.

### Scenario 3: Cancel or stop an MCP Start during configuration preparation or SDK startup

- Critical path: A Stop first waits for already-queued chat preparation for that session, then invokes the current runtime's Stop operation. If a same-server Start RPC is already running, the per-server lifecycle sequencer orders Stop after it.
- Scaling input: One user Stop request and at most the preceding preparation/replacement operation for that chat; no collection fan-out.
- Effective concurrency: The chat barrier serializes Stop behind configuration preparation, but MCP RPCs for different server names remain independent. Same-session/server Start cancellation is coalesced by the AgentSideEffects map.
- Cache behavior: Stop performs no plugin refresh. It resolves the live/backing session from in-memory maps. Cancellation suppresses superseded Start failure publication; no retained cancellation entry remains after settlement or disposal.
- Mechanism confidence: High: the empty chat-queue barrier and per-server sequencer are explicit and covered by disconnect/resume tests.
- Magnitude uncertainty: A barrier can inherit latency from an in-progress configuration refresh, but that ordering is required to avoid targeting the retiring runtime and does not introduce additional boundary calls.
- Expensive boundaries:
  - IPC at <code>src/vs/platform/agentHost/node/copilot/copilotAgentSession.ts:stopMcpServer</code>: Invoke the SDK MCP server Stop RPC.; cardinality: One SDK MCP Stop RPC per Stop request when a live control chat exists.
    - Introduced by diff: false; previous behavior: The same Stop RPC occurred immediately against the currently found runtime; the diff adds only an in-process queue barrier so it reaches a replacement runtime after refresh.; critical-path effect: Stop completion waits for the RPC and for any preceding same-server lifecycle operation.
- Verdict: No separate performance finding: cancellation sources are cleaned up, queue/RPC ordering is bounded, and cross-server/chat work remains independent.

### Scenario 4: Re-publish an unchanged client customization list containing one failed plugin

- Critical path: A client re-publishing customizations (for example after navigating away and back) awaits the controller sync and its filesystem/parser work before the refreshed customization result settles.
- Scaling input: Total plugins in the republished client contribution and number of identical re-publications while an error persists.
- Effective concurrency: All plugin inputs for that client fan out concurrently through the plugin manager, with per-URI sequencing; there is no grouping that avoids healthy inputs.
- Cache behavior: Identical healthy customization publications still hit the in-memory equality fast path. If any one customization is errored, the diff intentionally bypasses that fast path and performs a full sync; nonce hits avoid copying but not existence checks, cache persistence, or reparsing.
- Mechanism confidence: High: the changed equality guard directly changes an identical errored input from no-op to full synchronization.
- Magnitude uncertainty: Navigation/republication frequency and plugin counts vary; this is the same root cause and fix as the submitted full-client retry finding, so it is not reported separately.
- Expensive boundaries:
  - filesystem at <code>src/vs/platform/agentHost/node/copilot/copilotAgent.ts:6723</code>: Revalidate/materialize and reparse the entire client customization list.; cardinality: N plugin syncs/parses for an identical publication when one item is errored; previously zero because identical inputs always returned the existing settled sync.
    - Introduced by diff: true; previous behavior: Identical input lists were deduplicated regardless of an existing error state.; critical-path effect: Delays customization publication/navigation-associated refresh and adds filesystem load.
- Verdict: Confirms the submitted repeated-work mechanism but is not a distinct finding.

**Summary:** Completed the performance pass over all seven changed files. One medium-severity repeated-work regression was submitted: failed-plugin recovery now re-synchronizes and reparses every healthy sibling on send/Start and identical re-publication paths. MCP refresh, idle waiting, cancellation ownership, failure publication, and Stop ordering were also traced; their boundary counts and lifecycles are bounded and did not yield additional high-confidence performance issues.

</details>

<details>
<summary>Review statistics</summary>

- Model: `gpt-5.6-sol`
- Actual model calls: `gpt-5.6-sol`
- Reasoning effort: `high`
- Actual reasoning effort: `high`
- Model API endpoint: `ws:/responses`
- Wall-clock time: 2m1.697s
- Model calls: 12
- Tokens: 549477 input, 8113 output, 557590 total; 3010 reasoning; 482533 cache read, 66908 cache write
- Aggregate model API time: 1m48.781s
- Model-returned tool calls: 53
- Copilot usage: 68995720000.000 nano-AI units; model billing multiplier: 1.000
- USD cost: unavailable; the Copilot SDK does not expose a dollar conversion.

</details>
