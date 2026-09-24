# Performance review: Persist full Claude shell output in chat sessions

[PR #337389](https://github.com/microsoft/vscode/pull/337389) at `a4e6ef9ceaab585d195bb3f3685152fa3d634265`

## Findings (1)

### 1. [medium] Filter and batch retained-output checks during restore

**Finding ID:** `PERF-92560852BE68`  
**Location:** <code>src/vs/platform/agentHost/node/claude/claudeTerminalOutput.ts:125</code> (<code>RIGHT</code>)  
**Performance category:** <code>io</code>  
**Mechanism family:** <code>boundary-fanout</code>  
**Resource:** serialized SQLite queries and JS/native callback crossings  
**Scaling:** On each Claude chat history load, the number of awaited database queries grows linearly with the count of completed top-level Bash calls in the transcript, even when all of those calls kept their output inline and terminal\_outputs is empty. The session database's \_queueTurnData and mutation sequencers make the effective query concurrency one.  
**Outcome:** increased chat restore latency on shell-heavy sessions  
**PR causality:** <code>introduced</code>  
**Previous behavior:** Claude history reconstruction fetched and mapped the SDK transcript and then returned the turns; it did not stat/open the session database or perform per-tool SQLite lookups.  
**Changed behavior:** After reconstructing turns, restore collects every completed Bash tool call, opens the session database, and awaits getTerminalOutputSize separately inside a loop before the history can be returned.  
**Confidence:** 0.98 — The added critical-path await, per-call SQL implementation, serialization behavior, and unfiltered Bash-call cardinality are directly visible in the changed and transitive code. Exact milliseconds vary by session length and disk/cache state, which affects severity rather than mechanism confidence.

**Causal diff evidence:** Added line 125 places \`await database.object.getTerminalOutputSize(toolCall.toolCallId)\` inside the newly added loop over every completed Bash tool call, and added claudeAgent.ts line 1964 awaits the whole restore operation on every history read.

Reopening a shell-heavy Claude session with dozens or hundreds of ordinary Bash calls now adds dozens or hundreds of serialized SQLite SELECTs to the chat-load critical path. Warm filesystem/SQLite caches reduce each lookup's duration but do not remove the linear sequence of native database callbacks.

**Evidence:** ClaudeAgent.\_readChatMessages now awaits \_terminalOutputs.restore before returning history (claudeAgent.ts:1963-1964). restore gathers all completed Bash calls without first checking for Claude's &lt;persisted-output&gt; notice, then line 125 awaits getTerminalOutputSize for each one. SessionDatabase.getTerminalOutputSize executes one SELECT per call through \_queueTurnData (sessionDatabase.ts:945-949), and \_queueTurnData serializes work through \_turnUsageSequencer and \_mutationSequencer (sessionDatabase.ts:430-432). Capture only creates rows for messages with persistedOutputPath and a matching persisted-output terminal, so inline Bash calls can be rejected before touching SQLite.

**Suggested direction:** Before querying, restrict candidates to results containing the persisted-output notice, then resolve candidate IDs with one batched terminal\_outputs query (or expose a single API returning stored IDs) and attach resources from that set. This preserves restore semantics while removing per-Bash serialized database fan-out.


<details>
<summary>Reviewer scenario analysis</summary>

**Relevant issue families:** boundary-fanout, serialization-allocation, cleanup-lifecycle

### Scenario 1: Complete a top-level Claude Bash/PowerShell command whose result was spilled to a persisted-output file

- Critical path: The SDK user result is not mapped or published until capture reads the saved file, creates the turn row, and stores the output blob. This newly added filesystem and SQLite work is intentionally on the completion critical path so the attached resource is available when published.
- Scaling input: Number and byte size of foreground top-level shell results that Claude spills; each result is bounded to 10 MiB and handled once
- Effective concurrency: The SDK pipeline awaits router.handle per message; file read and database writes for a result are sequential, while the session database also sequences turn-data writes
- Cache behavior: There is no application cache or coalescing; each qualifying result is copied once. OS filesystem/SQLite caches may make warm access faster. Nonqualifying results are rejected before boundary work.
- Mechanism confidence: High: the awaited read and writes and 10 MiB bound are explicit in the changed call path.
- Magnitude uncertainty: Large spilled shell output is workload-dependent, and exact read/write latency depends on disk state. Work is bounded and required by the feature's publish-after-store contract.
- Expensive boundaries:
  - filesystem at <code>src/vs/platform/agentHost/node/claude/claudeTerminalOutput.ts:71</code>: Read Claude's saved output file with a 10 MiB size limit; cardinality: One read for each qualifying foreground top-level Bash result with persistedOutputPath; inline, background, subagent, and non-Bash results perform zero reads
    - Introduced by diff: true; previous behavior: Claude returned/published the model-facing result without copying the saved file; critical-path effect: The tool completion waits for the bounded read
  - database at <code>src/vs/platform/agentHost/node/claude/claudeTerminalOutput.ts:76-79</code>: Create the owning turn and insert/update its terminal output BLOB; cardinality: Two awaited database operations per qualifying spilled result, storing at most 10 MiB
    - Introduced by diff: true; previous behavior: No terminal-output database row was created for Claude shell results; critical-path effect: The completion waits until durable storage succeeds
- Verdict: No additional finding: this adds bounded I/O and allocation, but it is the core persistence operation and preserves the stated availability ordering; no avoidable multiplicity was found.

### Scenario 2: Open or restore a Claude chat with a shell-heavy history

- Critical path: ClaudeAgent fetches/maps the SDK transcript and now awaits ClaudeTerminalOutputs.restore before returning turns to AgentService, so database discovery and all existence checks block chat restoration.
- Scaling input: Count of completed Bash calls across all restored turns; realistic long-lived coding chats can contain dozens or hundreds, while effective retained-output cardinality is usually much smaller
- Effective concurrency: The restore loop awaits each lookup and SessionDatabase queues turn-data operations through sequencers, yielding effective query concurrency of one
- Cache behavior: getMessages reconstructs and restores on every call with no memoization. Warm OS/SQLite caches reduce individual query cost but do not coalesce or eliminate the per-call SQL callbacks. A missing database is negatively resolved by one stat for that invocation only.
- Mechanism confidence: Very high: the new await chain, unfiltered candidate collection, per-ID SELECT, and database sequencers are directly traced.
- Magnitude uncertainty: Exact added milliseconds depend on history length and storage speed; the linear serialized boundary fan-out is certain.
- Expensive boundaries:
  - filesystem at <code>src/vs/platform/agentHost/node/sessionDataService.ts:114-126, reached from claudeTerminalOutput.ts:119</code>: Stat the per-session database path before opening it; cardinality: One stat per getMessages restore when at least one completed Bash call exists
    - Introduced by diff: true; previous behavior: Claude getMessages returned after SDK transcript mapping without consulting the session database; critical-path effect: Chat history cannot be returned until the stat completes
  - database at <code>src/vs/platform/agentHost/node/claude/claudeTerminalOutput.ts:125 -&gt; src/vs/platform/agentHost/node/sessionDatabase.ts:945-949</code>: SELECT length(output) for a tool\_call\_id; cardinality: One query for every completed top-level Bash call in the reconstructed transcript, including inline-output calls with no possible retained row
    - Introduced by diff: true; previous behavior: Zero terminal-output existence queries on Claude history restore; critical-path effect: All lookups are awaited before history is returned
  - other expensive boundary at <code>src/vs/platform/agentHost/node/claude/claudeAgent.ts:2015</code>: Fetch the complete SDK transcript with includeSystemMessages; cardinality: One existing SDK fetch per getMessages call
    - Introduced by diff: false; previous behavior: The same SDK fetch and mapping occurred before the diff; critical-path effect: Existing prerequisite to reconstruct history; the diff adds database work after it
- Verdict: Reported: unfiltered per-Bash serialized SQLite fan-out adds avoidable history-load latency.

### Scenario 3: Open Full Output from a live or restored retained shell-output card

- Critical path: Resolving the non-PTY resource ensures session/chat state exists, then reads the one stored output BLOB before returning terminal state.
- Scaling input: Size of the single selected output, capped at 10 MiB, and number of explicit opens
- Effective concurrency: Each resource resolution performs its own awaited database read; no changed burst or collection fan-out was introduced here
- Cache behavior: No application blob cache; repeated opens reread the database, with only underlying OS/SQLite warm-cache benefit
- Mechanism confidence: High: the changed resource construction reaches an established one-resource resolver path.
- Magnitude uncertainty: Decode/allocation cost varies with output size, but is bounded and user-triggered.
- Expensive boundaries:
  - filesystem at <code>src/vs/platform/agentHost/node/agentService.ts:5925</code>: Stat/open the session database; cardinality: One database discovery per requested retained terminal resource
    - Introduced by diff: false; previous behavior: The resolver already used this path for retained non-PTY terminal resources; the diff makes Claude cards point to it; critical-path effect: The read-only full-output view waits for database discovery
  - database at <code>src/vs/platform/agentHost/node/agentService.ts:5927 and sessionDatabase.ts:952-964</code>: Read and decode one terminal output BLOB, bounded to 10 MiB; cardinality: One logical output read per user open; internally size and BLOB are read in the existing database implementation
    - Introduced by diff: false; previous behavior: Existing retained-terminal resources used the same resolver; Claude results did not previously expose such a resource; critical-path effect: The editor cannot show full output until the blob is loaded
- Verdict: No finding: the diff reuses the existing on-demand retained-terminal path with one bounded read per explicit open.

### Scenario 4: Cancel a turn while retained shell output is being captured

- Critical path: Cancellation is checked after the file read, after database storage, and again before synchronous publication. If cancellation wins after storage, cleanup deletes the row before returning.
- Scaling input: One currently completing spilled shell result; output read is capped at 10 MiB
- Effective concurrency: Capture and cleanup are awaited serially in the per-message router
- Cache behavior: No cache; cancellation removes staged mapper state and any stored row rather than retaining a negative entry
- Mechanism confidence: High: all abort gates and cleanup boundaries are explicit.
- Magnitude uncertainty: A cancellation during a slow maximum-size read can be delayed by that read, but the operation is bounded and one-at-a-time.
- Expensive boundaries:
  - filesystem at <code>src/vs/platform/agentHost/node/claude/claudeTerminalOutput.ts:71-73</code>: Potentially finish the in-flight saved-output read before observing abort; cardinality: At most one bounded read for the cancelling qualifying result
    - Introduced by diff: true; previous behavior: No saved-output read existed on cancellation; critical-path effect: Cancellation completion may wait for an already-started read because no cancellation token is passed to readFile
  - database at <code>src/vs/platform/agentHost/node/claude/claudeTerminalOutput.ts:81-83 and 99-100</code>: Delete a just-stored terminal output when cancellation wins; cardinality: At most one delete for the qualifying cancelled tool result; the second router check covers the post-capture microtask gap
    - Introduced by diff: true; previous behavior: No terminal-output row required cleanup; critical-path effect: The cancelled handler waits for cleanup, preventing retained database growth
- Verdict: No finding: cancellation can wait for bounded in-flight I/O, but cleanup ownership is complete and prevents memory/database retention.

**Summary:** Completed the performance pass over all 10 changed files. One medium-confidence-severity history-restore boundary-fanout regression was submitted; live capture, resource opening, bounded allocation, and cancellation cleanup did not establish additional actionable regressions.

</details>

<details>
<summary>Review statistics</summary>

- Model: `gpt-5.6-sol`
- Actual model calls: `gpt-5.6-sol`
- Reasoning effort: `high`
- Actual reasoning effort: `high`
- Model API endpoint: `ws:/responses`
- Wall-clock time: 1m53.96s
- Model calls: 9
- Tokens: 351112 input, 6948 output, 358060 total; 2282 reasoning; 288196 cache read, 62889 cache write
- Aggregate model API time: 1m39.427s
- Model-returned tool calls: 48
- Copilot usage: 56879140000.000 nano-AI units; model billing multiplier: 1.000
- USD cost: unavailable; the Copilot SDK does not expose a dollar conversion.

</details>
