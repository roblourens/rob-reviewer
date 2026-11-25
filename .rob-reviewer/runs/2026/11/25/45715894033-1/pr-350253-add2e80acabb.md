# Performance review: agentHost: stop canvas extensions that outlive their folder's trust

[PR #350253](https://github.com/microsoft/vscode/pull/350253) at `add2e80acabb5da7a11a12547046205464e0fa18`

## Findings (3)

### 1. [medium] Accept a complete stopped inventory before retrying the reload

**Finding ID:** `PERF-9BD742E9E2E0`  
**Location:** <code>src/vs/platform/agentHost/node/copilot/copilotAgentSession.ts:7515</code> (<code>RIGHT</code>)  
**Performance category:** <code>latency</code>  
**Mechanism family:** <code>repeated-work</code>  
**Resource:** Redundant extensions.reload IPC plus avoidable session-disconnect and subsequent session-resume work.  
**Scaling:** One redundant reload per affected session per reconciliation, followed by one avoidable session teardown/resume if both reload attempts reject. The global pass processes sessions concurrently; this is not a per-extension reload count.  
**Outcome:** Extra runtime lifecycle work and increased latency on the next chat or canvas operation.  
**PR causality:** <code>introduced</code>  
**Previous behavior:** A rejected reload ended that reconciliation attempt; for an already live session the agent logged the error and left the warm session available. There was no second reload or trust-revocation-driven session restart.  
**Changed behavior:** After a reload rejects, confirmation cannot succeed if the original revoked extension had no PID or an unreadable row, even when the new complete inventory contains no active extensions. The code unnecessarily reloads again and can request a whole-session restart.  
**Confidence:** 0.99 — The added test demonstrates the exact stopped-inventory state being rejected and the two reloads. The production error handler and restart call path establish the additional lifecycle cost. No timing measurements are claimed.

**Causal diff evidence:** The newly added condition (reloaded \|\| \!hasUnidentified) at line 7515 rejects an otherwise complete, stopped inventory. The surrounding new two-attempt loop then repeats rpc.extensions.reload and throws CopilotProjectExtensionRevocationError after another rejection.

A reload that reports failure after successfully stopping a PID-less starting extension causes an unnecessary second reload. If it reports failure again, an otherwise usable warm chat is disconnected and the next send/canvas operation pays SDK-session resume costs despite a complete inventory already proving there is nothing active to stop.

**Evidence:** hasUnidentified is set when a revoked starting extension lacks a PID (line 7482). If reload rejects but all extensions are now failed/absent, inventory.complete is true and stillRunning is empty, yet the added guard forbids returning. The new test explicitly captures this case: reloadFailedForOneStartingWithoutProcess lists revoked('starting') before reload and only revoked('failed') afterwards (copilotAgent.test.ts:13379), but expects two reloads and an unconfirmed outcome (13414). The agent handles that error by marking and disposing the session when idle (copilotAgent.ts:2788-2790, 2593-2601), or restarting it on the next send/control operation (9027, 9114-9123).

**Suggested direction:** Before requiring a successful reload for unidentified old processes, accept a complete inventory that has no active extensions. Preserve the conservative retry/restart behavior when the inventory is incomplete or an active extension could still be the unidentified process.

### 2. [medium] Use an elapsed-time deadline for extension settling

**Finding ID:** `PERF-0418F639CF2B`  
**Location:** <code>src/vs/platform/agentHost/node/copilot/copilotAgentSession.ts:7530</code> (<code>RIGHT</code>)  
**Performance category:** <code>latency</code>  
**Mechanism family:** <code>repeated-work</code>  
**Resource:** Full-inventory IPC round trips, inventory serialization/allocation, and time occupying the reconciliation/initialization critical path.  
**Scaling:** Up to ten serialized inventory calls per reload attempt, twenty across two attempts, per affected session. Sessions run concurrently via Promise.all, but the next global reconciliation pass waits for the slowest. Ten successful five-second responses, for example, require about fifty seconds per attempt; this is a code-derived example, not a measurement.  
**Outcome:** Amplified reconciliation and chat-initialization latency on slow runtimes, plus repeated full-inventory IPC work.  
**PR causality:** <code>introduced</code>  
**Previous behavior:** A contraction listed the session's extensions once and awaited one reload; it did not poll inventories after the reload. A combined expansion could perform one additional list.  
**Changed behavior:** Revocation confirmation now polls the full extension inventory up to ten times sequentially per reload attempt, with a fresh control-plane RPC timeout for every poll. The second reload attempt can repeat the entire polling sequence.  
**Confidence:** 0.98 — The fixed poll count, per-call timeout, two attempts, Promise.all barrier, and initialization await are all directly traced. Actual runtime inventory latency and the prevalence of PID-less starting rows are unknown, but do not change the multiplicative wait.

**Causal diff evidence:** The added \_settledRuntimeExtensions loop awaits \_listRuntimeExtensions at line 7530 before checking poll &gt;= PROJECT\_EXTENSION\_SETTLE\_POLLS and sleeping 100ms. \_stopRevokedProjectExtensions calls this new loop inside its two-attempt reload loop.

When a same-named extension is still starting without a PID and the runtime is slow to answer inventories, this ostensibly brief settling wait can last tens of seconds or minutes, delaying trust reconciliation for other sessions and blocking a starting/resuming chat when the change occurs during initialization.

**Evidence:** PROJECT\_EXTENSION\_SETTLE\_POLLS is 10 and the interval is 100ms (lines 722-724), but inventory RPC durations are not included in any elapsed-time budget. \_listRuntimeExtensions invokes \_awaitControlPlaneRpc('rpc.extensions.list', ...) at line 5553; each call has its own default 30,000ms timeout (lines 681, 2591, 9410-9420). Slow successful inventories that continue to show a revoked name as starting without a PID therefore incur ten RPC round trips per attempt, not a roughly one-second bound. copilotAgent.ts:2776-2794 awaits all sessions before its global trust Throttler can run the next pass; initialization also directly awaits this reconciliation at 11668/11692.

**Suggested direction:** Start an elapsed-time settling deadline after the first inventory and bound subsequent waits by its remaining budget. Stop polling once that deadline expires and use the existing bounded retry/restart decision.

### 3. [medium] Bound revocation teardown before awaiting it during initialization

**Finding ID:** `PERF-F9A6F5385F6D`  
**Location:** <code>src/vs/platform/agentHost/node/copilot/copilotAgent.ts:11674</code> (<code>RIGHT</code>)  
**Performance category:** <code>latency</code>  
**Mechanism family:** <code>cleanup-lifecycle</code>  
**Resource:** The pending session-disconnect IPC, initialization promise, and serialized chat queue slot.  
**Scaling:** The delay is unbounded for each affected chat when neither the disconnect response nor shutdown arrives; queueing further operations cannot make progress. Other chat keys can still run.  
**Outcome:** Potentially indefinite chat initialization latency and head-of-line blocking of subsequent chat operations.  
**PR causality:** <code>introduced</code>  
**Previous behavior:** The reconciliation error propagated to callers such as \_resumeSessionOnce, which called agentSession.dispose() and rethrew. Wrapper disposal started disconnect as best-effort asynchronous cleanup, without keeping the initializing operation pending on that response.  
**Changed behavior:** A failed trust revocation during initialization now awaits \_destroyLiveSession before propagating the initialization error, moving session disconnect onto the initializing chat's serialized critical path.  
**Confidence:** 0.97 — The newly awaited cleanup, its uncapped disconnect/shutdown race, and the per-chat sequencer are explicit in the traced code. Pending disconnect behavior is also modeled in copilotAgentSession.test.ts:7207-7233 and 7500-7522. The frequency of a runtime stalling during this failure path is not measured.

**Causal diff evidence:** The added await this.\_destroyLiveSession(session, true) at line 11674 precedes throw error; the added post-registration branch at line 11699 does the same. Neither added await has a deadline or cancellation race.

If a chat starts or resumes while trust contracts and the unhealthy runtime fails revocation and then stalls disconnect without emitting shutdown, initialization and all subsequent sends/control operations queued for that chat remain blocked, even after cancellation. A single affected chat is sufficient.

**Evidence:** \_destroyLiveSession awaits chatSession.destroySession() (copilotAgent.ts:11619-11631); destroySession awaits \_wrapper.disconnect() (copilotAgentSession.ts:8705-8717). CopilotSessionWrapper.disconnect races only the disconnect promise against session.shutdown, and \_disconnect directly awaits session.disconnect() (copilotSessionWrapper.ts:144-186). If neither arrives, this wait never settles. \_queueChat awaits its task inside SequencerByKey and its stall timer only logs (copilotAgent.ts:10609-10627). \_resumeSessionOnce's catch/dispose at 11855-11857 cannot execute until the newly added await finishes.

**Suggested direction:** Keep teardown ordered, but give this awaited teardown a deadline and escalate through shutdown of the owning runtime if disconnect does not settle; do not hold initialization and the chat queue indefinitely.


<details>
<summary>Reviewer scenario analysis</summary>

**Relevant issue families:** repeated-work, cleanup-lifecycle, boundary-fanout, missing-coalescing, cache-lifecycle, concurrency-burst, unbounded-retention, serialization-allocation

### Scenario 1: Create/resume a canvas-enabled chat and allow or refuse its project-extension launches.

- Critical path: Admission and SDK-session initialization precede use of the chat. Each project-extension launch must resolve its canonical module and discovery root before the launch provider permits the extension subprocess. Existing SDK initialization and sandbox preparation remain on the startup path.
- Scaling input: D admitted directories, E project-extension launch requests across sessions, and T distinct trusted URI strings. Launch records deduplicate by requested path plus canonical module; different sessions requesting the same path still perform launch-time canonicalization.
- Effective concurrency: Admission directory reads remain sequential. The module and discovery-root reads for one launch now run in Promise.all. Trusted URI reads run in parallel and share in-flight promises. Independent runtime launch requests are not globally limited here.
- Cache behavior: Successful trusted-root promises are cached/coalesced per trusted URI and cleared on workspace-trust changes. Missing roots are removed and retried. Scratch exemptions bypass trusted-root reads. The module and newly introduced discovery-root realpaths are not cached.
- Mechanism confidence: High: launch callback wiring, filesystem calls, cache ownership, and startup call path are explicit.
- Magnitude uncertainty: No filesystem timings or real-world extension counts are available. The added discovery-root canonicalization is bounded per launch and necessary for the new ownership tracking; no separate high-confidence regression is reported for it.
- Expensive boundaries:
  - filesystem at <code>src/vs/platform/agentHost/node/copilot/copilotProjectExtensionLaunchPolicy.ts:175-280 acquireAdmission and exemption canonicalization</code>: Canonicalize admitted directories and, where applicable, scratch/worktree metadata using fs.realpath.; cardinality: D admission-root calls; scratch validation adds two parallel canonicalizations, or worktree validation three. These counts are unchanged by this PR.
    - Introduced by diff: false; previous behavior: The same directory and exemption canonicalization calls already ran during admission.; critical-path effect: Admission must finish before session initialization proceeds.
  - filesystem at <code>src/vs/platform/agentHost/node/copilot/copilotProjectExtensionLaunchPolicy.ts:289-293 resolveLaunch; 412-420 \_canonicalDiscoveryRoot</code>: Resolve the module path and newly resolve the folder above .github/extensions for project launch bookkeeping.; cardinality: One existing module realpath plus one new discovery-root realpath for each normally shaped project-extension launch request; no discovery-root call for other path shapes.
    - Introduced by diff: true; previous behavior: Only the module canonicalization was awaited here; discovery-root canonicalization did not run.; critical-path effect: Both results are awaited before the corresponding extension may launch.
  - filesystem at <code>src/vs/platform/agentHost/node/copilot/copilotProjectExtensionLaunchPolicy.ts:352-374 \_trustedRoots/\_canonicalTrustedRoot</code>: Canonicalize configured trusted roots.; cardinality: Up to T distinct successful cold reads shared across launches; zero additional successful-root filesystem reads when warm. Unresolved roots may be retried on subsequent decisions.
    - Introduced by diff: false; previous behavior: The same successful/in-flight cache and retry-on-missing behavior existed.; critical-path effect: Nonexempt trust decisions await these roots.
  - IPC at <code>src/vs/platform/agentHost/node/copilot/copilotSessionLauncher.ts:1203, 1351 createSession/resumeSession; copilotAgent.ts:5413 launchProvider</code>: Create/resume an SDK session and answer runtime extension-launch requests; permitted extensions use the existing Node/bootstrap launch configuration.; cardinality: One normal SDK create/resume per chat initialization, with launch-provider callbacks for extensions the runtime starts.
    - Introduced by diff: false; previous behavior: The same SDK launch and extension subprocess path existed.; critical-path effect: Existing startup and individual extension-launch dependencies; no new direct network request is introduced by this bookkeeping.
- Verdict: Normal launch bookkeeping introduces one additional filesystem canonicalization per project launch, but no independently actionable performance regression was established on this normal path.

### Scenario 2: Apply a workspace-trust expansion or contraction to already live canvas sessions.

- Critical path: The policy computes affected extension names before the agent lists live sessions. The global trust Throttler cannot advance to its next pass until Promise.all of the current canvas sessions settles. This normally runs outside send/control queues, except for reconciliation performed during initialization.
- Scaling input: N distinct recorded allowed launches, D distinct admitted roots, S live canvas-enabled sessions, K sessions with an active revoked extension or an incomplete inventory, and R sessions with failed project extensions after an expansion.
- Effective concurrency: Policy launch checks are awaited sequentially, with shared trusted-root promises. One global Throttler serializes passes and coalesces changes into at most one queued successor. Within a pass all S session reconciliations run concurrently; multiple revoked names in one session share a reload.
- Cache behavior: Workspace-trust changes clear canonical trusted-root promises. Successful trusted roots are resolved at most once per URI per generation, not once per launch. Failed roots can be retried. Inventory snapshots are not cached between calls. Launch bookkeeping is pruned conservatively by discovery root.
- Mechanism confidence: High: grouping is per session and the global/per-session ordering is directly visible.
- Magnitude uncertainty: Session counts, provider startup costs, inventory sizes, and runtime RPC durations are not measured. The normal extra confirmation is intentional; the avoidable repeated-work cases are reported separately.
- Expensive boundaries:
  - filesystem at <code>src/vs/platform/agentHost/node/copilot/copilotProjectExtensionLaunchPolicy.ts:144-166 reconciliationFor</code>: Recheck allowed launch trust using canonical trusted roots.; cardinality: Up to T successful cold trusted-root realpaths shared across all N records; warm successes perform no filesystem read. Missing roots may be retried on later record checks.
    - Introduced by diff: false; previous behavior: Contractions already scanned allowed launches and checked the same cached trust roots; the diff retains some records longer to cover borrowed extensions.; critical-path effect: Policy reconciliation finishes before session RPC fan-out.
  - IPC at <code>src/vs/platform/agentHost/node/copilot/copilotAgent.ts:2776-2794; copilotAgentSession.ts:7428, 7492, 7530</code>: List extension inventories; reload affected sessions; confirm stops by additional inventories.; cardinality: One initial inventory per S session. Normal successful revocation adds one confirmation inventory per affected session and one grouped reload per K session. Failure/starting cases can perform two reloads and up to twenty confirmation inventories per affected session.
    - Introduced by diff: true; previous behavior: One initial inventory per session and one reload per affected session, with at most an additional list for a combined expansion; no post-reload confirmation polling.; critical-path effect: The slowest session holds the current global reconciliation pass. Additional confirmation calls are new.
  - subprocess at <code>src/vs/platform/agentHost/node/copilot/copilotAgentSession.ts:7492 rpc.extensions.reload; test/node/copilotSdkCanvas.integrationTest.ts:181-182</code>: Runtime reload stops/restarts enabled extension providers, consulting the existing launch provider.; cardinality: Grouped once per affected session in the successful case, up to twice in the new retry path, not once per extension. Each reload can affect that session's other enabled providers.
    - Introduced by diff: true; previous behavior: One grouped reload was already used; the retry introduces a possible second provider lifecycle cycle.; critical-path effect: Reload completion is awaited before confirmation; provider readiness can continue afterwards.
  - IPC at <code>src/vs/platform/agentHost/node/copilot/copilotAgentSession.ts:7456-7467</code>: Reconcile failed/refused project extensions after trust expands.; cardinality: At most one reconcile call for each of R sessions with failed project extensions in a pass.
    - Introduced by diff: false; previous behavior: The same expansion retry already existed.; critical-path effect: Awaited by the same per-pass Promise.all barrier.
- Verdict: The confirmation design intentionally adds bounded normal work. Reported issues concern its wall-clock-unbounded polling budget and unnecessary retry/restart when a complete inventory already shows everything stopped.

### Scenario 3: Open or close a scratch/worktree exemption that another chat's extension borrows.

- Critical path: Admission acquisition/release updates reference counts and scans recorded launches synchronously; relevant exemption changes schedule background reconciliation. The policy's trust counters also make initializing sessions catch up before returning to their caller.
- Scaling input: Up to two exemption roots changed by an admission, N recorded allowed/refused launches keyed by requested/canonical path, and S live canvas sessions. A supplier closing with no remaining chat under a discovery root allows pruning before any session RPC.
- Effective concurrency: Change events reuse the global Throttler, OR accumulated expansion/contraction flags, and coalesce changes arriving during a running pass. Session RPCs within the resulting pass run concurrently, not under a global per-session limiter.
- Cache behavior: Exemption changes advance the trust generation without clearing successful canonical workspace-trust roots. Shared exemptions use reference counts, so only first admission/last release can signal a change. Refused records are removed after success or pruned on reconciliation; unknown discovery roots remain conservatively retained.
- Mechanism confidence: High: event gating, reference counts, pruning, and coalescing are explicit in source and new policy tests.
- Magnitude uncertainty: Borrowed scratch/worktree extensions are specialized; launch-history sizes are unknown. No large retained-object graph or avoidable boundary burst was established beyond the separately reported retry behavior.
- Expensive boundaries:
  - filesystem at <code>src/vs/platform/agentHost/node/copilot/copilotProjectExtensionLaunchPolicy.ts:175-280 acquireAdmission</code>: Resolve admitted scratch/worktree roots and their repository relationships.; cardinality: Same admission and scratch/worktree canonicalization counts as before; no filesystem work is added by the synchronous event emission itself.
    - Introduced by diff: false; previous behavior: Canonicalization and reference counting already occurred, but did not schedule reconciliation for exemptions.; critical-path effect: Existing admission startup dependency.
  - IPC at <code>src/vs/platform/agentHost/node/copilot/copilotAgent.ts:1705-1711 onDidChangeExemptions; 2776-2794 reconciliation pass</code>: Schedule existing inventory/reload/reconcile RPCs when an exemption changes under a recorded launch.; cardinality: One coalesced pass serving accumulated changes; one initial list per eligible live canvas session when affected names/expansion remain, then grouped operations only for relevant sessions. No inventory RPC if pruning produces no affected names and there is no expansion.
    - Introduced by diff: true; previous behavior: Exemption changes alone did not schedule these runtime operations, leaving borrowed extensions unreconciled.; critical-path effect: Normally background; initializing sessions separately catch up on their startup path.
- Verdict: New reconciliation is required for the intended lifecycle semantics and is coalesced. No additional finding for exemption events or conservative launch retention.

### Scenario 4: Confirm revocation while a same-named provider remains starting without a PID and inventory reads are slow.

- Critical path: Each reload attempt awaits the settling loop; the global pass and any initialization-time reconciliation await that loop. Ordinary sends are not universally blocked by the background pass, but newly initializing/resuming chats that are catching up are.
- Scaling input: K affected sessions, each potentially making ten successful inventory RPCs per attempt and two attempts; each response's duration adds serially within its session.
- Effective concurrency: Polls are sequential per session with 100ms sleeps. A new 30-second RPC timeout starts for every call. K sessions can poll concurrently, while the global trust Throttler waits for the slowest.
- Cache behavior: Every poll makes a fresh full inventory RPC. Trusted-root caching does not coalesce or shorten inventory polling.
- Mechanism confidence: High and independent of magnitude: call count, serial waits, individual timeout, and critical-path callers are directly proven.
- Magnitude uncertainty: No observed inventory latency distribution is available; slow successful replies are the trigger, not ten consecutive timeouts. Numeric examples in the finding are explicitly derived rather than measured.
- Expensive boundaries:
  - IPC at <code>src/vs/platform/agentHost/node/copilot/copilotAgentSession.ts:7528-7543 \_settledRuntimeExtensions; 5553 \_listRuntimeExtensions; 9410 \_awaitControlPlaneRpc</code>: Repeatedly fetch and decode the complete extension inventory.; cardinality: Up to ten successful list calls per attempt, twenty across retries, per affected session, plus its initial pre-reload list. A timed-out list aborts that attempt rather than being repeated ten times within it.
    - Introduced by diff: true; previous behavior: No contraction-only post-reload polling existed; combined expansion could list once more.; critical-path effect: Successful slow calls can stretch the nominal short settle period to tens of seconds or minutes and delay the pass/initialization.
  - serialization at <code>src/vs/platform/agentHost/node/copilot/copilotAgentSession.ts:5552-5590 \_listRuntimeExtensions</code>: Receive and map each complete inventory, including PID/status normalization.; cardinality: Linear in inventory rows for every issued poll; no per-row IPC fan-out.
    - Introduced by diff: true; previous behavior: Inventory decoding existed, but did not repeat through this new polling loop.; critical-path effect: Part of each awaited inventory operation.
- Verdict: Reported: use an elapsed-time deadline instead of treating ten individually timed RPCs as a short settling bound.

### Scenario 5: A reload rejects after stopping a PID-less extension, versus a genuinely unconfirmed revocation requiring idle/busy-session fallback.

- Critical path: After a failed confirmation the agent marks restart required. Idle teardown runs through the chat sequencer; busy sessions wait for their own turn/subagents to finish, or restart before a subsequent send/control operation. The next use of a disposed session must resume it before proceeding.
- Scaling input: Affected sessions, not extension names: at most two reload attempts per session, one coalesced wrapper disconnect, and one later SDK-session resume when used again.
- Effective concurrency: The global pass handles independent sessions concurrently. Teardown and subsequent work are serialized per chat. Wrapper disconnect requests share \_disconnectPromise; there is no per-extension disconnect fan-out.
- Cache behavior: A complete inventory is authoritative but the new hasUnidentified gate prevents using even an entirely inactive inventory after a failed reload. Disposing drops the warm SDK wrapper; retained metadata/customization anchors do not eliminate the later SDK resume.
- Mechanism confidence: Very high: the new table test explicitly expects two reloads and failure for starting-without-PID followed by failed-only inventory; production handling traces directly to disposal/resume.
- Magnitude uncertainty: Real reload-rejection rates and resume costs are unknown. The finding is limited to complete inactive inventories and does not object to restarting when a process could still run.
- Expensive boundaries:
  - IPC at <code>src/vs/platform/agentHost/node/copilot/copilotAgentSession.ts:7482-7523 \_stopRevokedProjectExtensions</code>: Retry extensions.reload and list again despite a complete inventory with only failed/absent extensions.; cardinality: One avoidable second reload and another confirmation sequence per affected session; two rejected reloads trigger the restart fallback.
    - Introduced by diff: true; previous behavior: A rejected reload ended reconciliation with a logged failure for live sessions, without a second reload or trust-specific restart.; critical-path effect: Adds work before the pass finishes and may unnecessarily invalidate the warm chat session.
  - IPC at <code>src/vs/platform/agentHost/node/copilot/copilotAgent.ts:2573-2604; copilotAgentSession.ts:8705-8717; copilotSessionWrapper.ts:144-186</code>: Flush/release session state and disconnect the SDK session.; cardinality: One disconnect per retiring wrapper, shared across concurrent disconnect requests; pre-disconnect sampling-interest release is one existing bounded RPC.
    - Introduced by diff: true; previous behavior: A non-timeout reload rejection alone did not schedule this trust-specific live-session teardown.; critical-path effect: Occupies the chat queue until teardown completes. This is intended for genuine unconfirmed revocations, avoidable for the stopped-inventory case.
  - database at <code>src/vs/platform/agentHost/node/copilot/copilotAgent.ts:11756-11854 \_resumeSessionOnce</code>: Read stored session metadata and customization state to rebuild the resumed session.; cardinality: Existing resume-path reads once per subsequent cold resume; newly reached when this false-negative confirmation disposes a previously warm session.
    - Introduced by diff: true; previous behavior: The already live session could be reused after a non-timeout reload failure; no forced cold-resume reads were required by this path.; critical-path effect: The next send or canvas/control operation waits for restoration.
  - IPC at <code>src/vs/platform/agentHost/node/copilot/copilotSessionLauncher.ts:1351; copilotAgentSession.ts:5260-5395 initialization</code>: Resume the SDK session and perform its existing registration/control-plane setup.; cardinality: One resume plus the normal initialization RPCs per replacement wrapper; enabled extension providers can relaunch through the normal launch path.
    - Introduced by diff: true; previous behavior: These boundaries existed for ordinary cold resume, but were not reached solely because this stopped-inventory reload had rejected.; critical-path effect: Required before the next operation can use a disposed chat's runtime.
- Verdict: Reported: accept a complete inventory with no active extensions before retrying or forcing session restart.

### Scenario 6: Trust contracts while a chat initializes or resumes, reconciliation fails, and teardown does not return a response or shutdown event.

- Critical path: Initialization already holds the chat queue. The added error branches now await \_destroyLiveSession before propagating failure; the caller's dispose/rethrow and queue release cannot occur first. This wait has no cancellation/deadline.
- Scaling input: One affected initialization suffices. Further operations on that chat accumulate behind its sequencer; independent chat keys can progress.
- Effective concurrency: One chat sequencer slot is held through cleanup. CopilotSessionWrapper waits on either disconnect response or shutdown. The chat stall timer only logs and does not release the task.
- Cache behavior: Coalesced default/peer resume promises remain pending until initialization completes. Disconnect calls share one promise, which avoids duplicate IPC but supplies no time bound; ordinary caches cannot release the queue.
- Mechanism confidence: High: the added awaited cleanup, caller disposal catch, promise races, queue ordering, and logging-only stall warning are established in code. Existing tests explicitly model pending disconnects and shutdown as the alternative completion.
- Magnitude uncertainty: No production rate for a stalled runtime failing both revocation and teardown is established. The indefinite queue-blocking mechanism is distinct from that prevalence uncertainty.
- Expensive boundaries:
  - IPC at <code>src/vs/platform/agentHost/node/copilot/copilotAgent.ts:11674, 11699; copilotAgentSession.ts:8716; copilotSessionWrapper.ts:144-186</code>: Await SDK-session disconnect after initialization-time revocation failure.; cardinality: One shared disconnect operation per wrapper. Neither repeated calls nor cancellation create a completion deadline.
    - Introduced by diff: true; previous behavior: For a failure during initialization, the error reached the caller's dispose/rethrow; wrapper disposal initiated best-effort disconnect without waiting for it before the operation failed.; critical-path effect: Newly moves potentially nonterminating disconnect onto initialization and the chat's serialized critical path.
  - IPC at <code>src/vs/platform/agentHost/node/copilot/copilotAgentSession.ts:5360-5367 setBeforeDisconnect</code>: Release sampling-event interest before session disconnect.; cardinality: One existing releaseInterest RPC per wrapper, bounded by the control-plane timeout, after pending output captures settle.
    - Introduced by diff: true; previous behavior: The same cleanup occurred during wrapper teardown, but initialization failure did not await that best-effort disposal path.; critical-path effect: Existing disconnect preparation now also lies in the added awaited initialization cleanup; unlike the final disconnect RPC, this individual interest-release RPC is bounded.
- Verdict: Reported: preserve teardown ordering but bound it and escalate through the owning runtime rather than wedging chat initialization indefinitely.

**Summary:** Completed the performance-review pass over all seven changed files and all diff pages against base beaa31d361bfe0fc386285370819b343941a9c0c. Submitted three high-confidence findings: newly awaited unbounded initialization teardown, poll-count rather than elapsed-time settling, and redundant retry/session restart despite a complete stopped inventory. Traced startup, trust changes, exemption changes, polling, fallback restart, caches, grouping, and per-chat/global sequencing. No separate findings for necessary discovery-root I/O, conservative launch records, metadata projection, or intentional fail-closed handling. Investigated provenance of retained borrowed-extension processes, but did not record a performance-regression fix because blame of the trust-policy change alone does not establish a prior performance worsening.

</details>

<details>
<summary>Review statistics</summary>

- Model: `gpt-6.1-sol`
- Actual model calls: `gpt-6.1-sol`
- Reasoning effort: `high`
- Actual reasoning effort: `high`
- Model API endpoint: `ws:/responses`
- Wall-clock time: 5m29.046s
- Model calls: 13
- Tokens: 944911 input, 14356 output, 959267 total; 5249 reasoning; 831679 cache read, 113193 cache write
- Aggregate model API time: 5m2.891s
- Model-returned tool calls: 62
- Copilot usage: 50978840000.000 nano-AI units; model billing multiplier: 1.000
- USD cost: unavailable; the Copilot SDK does not expose a dollar conversion.

</details>
