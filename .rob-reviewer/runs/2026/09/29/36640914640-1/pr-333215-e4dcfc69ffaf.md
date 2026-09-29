# Performance review: launch: improve cross-platform setup and cleanup

[PR #333215](https://github.com/microsoft/vscode/pull/333215) at `e4dcfc69ffafaa5ade25f89bbc106ecbb5543d9e`

## Findings (2)

### 1. [medium] Defer process-table polling until the user waits for exit

**Finding ID:** `PERF-6FB080A2445D`  
**Location:** <code>.agents/skills/launch/scripts/bootstrap/bootstrap-profile.sh:71</code> (<code>RIGHT</code>)  
**Performance category:** <code>subprocess</code>  
**Mechanism family:** <code>repeated-work</code>  
**Resource:** A new \`ps\` subprocess plus full process-table parsing, arrays, and sets on every 250 ms timer tick  
**Scaling:** The monitor launches four \`ps\` processes per second for the entire bootstrap-window lifetime (240 per minute); each invocation and the descendant-closure scan grow with the machine's process count and process-tree depth.  
**Outcome:** Persistent subprocess churn, CPU usage, scheduler wakeups, and battery drain while the user signs in or whenever the bootstrap Code OSS window is left open.  
**PR causality:** <code>introduced</code>  
**Previous behavior:** The pre-diff documented bootstrap path launched Code OSS directly against the source profile and did not create a lifetime process-table monitor or recurring subprocesses.  
**Changed behavior:** Every successful macOS/Linux profile bootstrap now starts a detached Node monitor that synchronously spawns \`ps\`, parses the complete process table, and recomputes descendant/running sets four times per second after bootstrap readiness.  
**Confidence:** 0.99 — The subprocess frequency, full-table scan, detached lifetime, and human-controlled duration are directly established by the changed script and bootstrap documentation. Exact per-scan CPU cost varies by OS and process count, which limits severity but not mechanism confidence.

**Causal diff evidence:** Added line 71 places synchronous \`execFileSync("ps", ...)\` inside \`check\`, and added line 109 schedules \`check\` every 250 ms until every tracked Code OSS descendant exits.

Choosing profile bootstrap and taking several minutes to complete GitHub/Copilot sign-in produces hundreds to thousands of extra \`ps\` launches; leaving the persistent window open makes that overhead unbounded in time.

**Evidence:** \`bootstrap-profile.sh\` starts this detached monitor before waiting for CDP and returns JSON as soon as CDP is ready, while SKILL.md instructs the caller to wait for a human to sign in and close the window before invoking \`--wait-for-exit\`. Thus the 250 ms loop is active throughout a realistically long human interaction, not just a short shutdown check. The loop's expensive boundary and process-table cardinality are explicit in lines 66-75 and its lifetime condition is explicit in lines 100-111.

**Suggested direction:** Avoid polling for the whole interactive session. Preserve the release guarantee with an OS lifecycle primitive/process-group owner, or perform a bounded, much lower-frequency targeted/profile-lock check only when \`--wait-for-exit\` is invoked after user acknowledgement.

### 2. [medium] Stop querying CIM four times per second for the window lifetime

**Finding ID:** `PERF-47D7852BD944`  
**Location:** <code>.agents/skills/launch/scripts/bootstrap/windows-process-tree-monitor.ps1:19</code> (<code>RIGHT</code>)  
**Performance category:** <code>ipc</code>  
**Mechanism family:** <code>repeated-work</code>  
**Resource:** Repeated CIM/WMI IPC queries that materialize every \`Win32\_Process\`, followed by full-table arrays, hash sets, and descendant scans  
**Scaling:** The added monitor issues four machine-wide CIM queries per second for as long as the bootstrap Code OSS process tree remains alive; work grows with both process count and the human-controlled window lifetime.  
**Outcome:** Ongoing CIM provider load, PowerShell CPU/allocation pressure, scheduler wakeups, and battery drain during profile sign-in or while the bootstrap window is left open.  
**PR causality:** <code>introduced</code>  
**Previous behavior:** Before this PR, users were instructed to launch \`code.bat\` directly against the persistent profile; that path had no hidden lifetime monitor and no recurring CIM process enumeration.  
**Changed behavior:** Every successful Windows profile bootstrap now launches a hidden PowerShell monitor that retrieves and materializes the complete system process table over CIM, rebuilds process sets, and rescans descendants four times per second until the entire Code OSS tree exits.  
**Confidence:** 0.99 — The changed query, 250 ms cadence, detached ownership, and human-controlled lifetime are explicit. CIM query duration differs across Windows systems, so magnitude is uncertain, but repeated machine-wide IPC and allocation are certain.

**Causal diff evidence:** Added line 19 executes \`Get-CimInstance Win32\_Process\` at the top of an unconditional loop, and added line 55 sleeps only 250 ms before repeating while any tracked process remains.

A several-minute GitHub/Copilot sign-in causes hundreds or thousands of full \`Win32\_Process\` queries, and an accidentally open bootstrap window sustains the machine-wide polling indefinitely.

**Evidence:** \`bootstrap-profile.ps1\` starts \`windows-process-tree-monitor.ps1\` as a detached hidden process and returns once \`waitForCdp.ts\` reports readiness. SKILL.md then waits on a user sign-in/close acknowledgement before checking the exit marker, proving that the unconditional CIM loop runs across the interactive interval rather than only during cleanup.

**Suggested direction:** Replace steady-state CIM polling with a Windows lifecycle mechanism such as a job/process-exit notification, or defer a bounded, lower-frequency targeted/profile-use check to \`-WaitForExit\`; retain the exact-profile/process-tree release guarantee.


<details>
<summary>Reviewer scenario analysis</summary>

**Relevant issue families:** repeated-work, cleanup-lifecycle, boundary-fanout, eager-work, serialization-allocation

### Scenario 1: Normal isolated launch from an existing authenticated profile on macOS/Linux or Windows

- Critical path: Path validation, profile/shared-data copying, settings rewrite, pre-launch, Code OSS process start, and CDP readiness remain serial launch prerequisites. The added Unix \`node -e\` path normalization is one extra process on the critical path but is negligible relative to the unchanged launcher pipeline.
- Scaling input: Number and total bytes of files in the source profile/shared storage; the diff does not increase effective copy cardinality when the profile exists.
- Effective concurrency: Profile preparation and pre-launch are serialized; the app and readiness probe overlap only after process start, unchanged from before.
- Cache behavior: Slim/full profile copies remain per-run with no shared cache. Pre-launch can still be skipped explicitly or benefit from existing build outputs; no cache key, hit, miss, or coalescing behavior changes.
- Mechanism confidence: High confidence that successful existing-profile launches retain the prior expensive boundaries and gain only one fixed-cost Unix helper process.
- Magnitude uncertainty: The one added Node startup varies by platform but is dwarfed by profile copy and application startup.
- Expensive boundaries:
  - filesystem at <code>.agents/skills/launch/scripts/launch.sh:85 and .agents/skills/launch/scripts/launch.ps1:428</code>: Stat/resolve the source profile and recursively copy its selected files and shared data; cardinality: One source check and one profile copy per launch, scaling with retained profile files; unchanged for an existing source profile
    - Introduced by diff: false; previous behavior: The launcher already validated and copied the source profile before starting Code OSS.; critical-path effect: Must finish before settings update and app startup
  - subprocess at <code>.agents/skills/launch/scripts/launch.sh:91</code>: Start Node to resolve the source path; cardinality: Exactly one Node process per Unix launch
    - Introduced by diff: true; previous behavior: The shell previously tested the path directly without this Node invocation.; critical-path effect: Adds one Node startup before profile preparation
  - subprocess at <code>.agents/skills/launch/scripts/launch.sh and .agents/skills/launch/scripts/launch.ps1</code>: Run settings updater, optional pre-launch build preparation, Code OSS, and readiness helper; cardinality: One settings process, one optional pre-launch process, one app launch, and one readiness helper per launch; unchanged
    - Introduced by diff: false; previous behavior: The same subprocess chain ran for every successful launch.; critical-path effect: These dominate launch readiness but their frequency and placement are unchanged
- Verdict: No reportable regression: the only added work is one bounded path-normalization process, while the scaling profile-copy and startup paths are unchanged.

### Scenario 2: Launch when the persistent source profile does not exist

- Critical path: The launcher now creates an empty isolated profile, writes settings, runs pre-launch, starts Code OSS, and probes CDP; all must complete before the user can reach sign-in. Previously this scenario stopped at validation and could not make product progress.
- Scaling input: Build preparation state controls pre-launch duration; no source-profile collection is copied in this branch.
- Effective concurrency: Directory/settings setup and pre-launch are serial; after app start, one readiness helper issues non-overlapping localhost probes.
- Cache behavior: Warm compiled outputs reduce pre-launch work and \`--skip-prelaunch\` remains available. The empty profile is intentionally fresh per run, with no negative-cache loop.
- Mechanism confidence: High confidence that the diff intentionally replaces immediate failure with one complete launch.
- Magnitude uncertainty: Startup duration depends on build state, but the work is required to enable a previously blocked sign-in workflow.
- Expensive boundaries:
  - filesystem at <code>.agents/skills/launch/scripts/launch.sh:165 and .agents/skills/launch/scripts/launch.ps1:481</code>: Create the isolated profile/settings directories and write settings instead of copying a missing source; cardinality: A fixed set of directories/files per launch; no source-tree copy
    - Introduced by diff: true; previous behavior: The launcher exited before creating a runnable isolated profile.; critical-path effect: Required before Code OSS starts
  - subprocess at <code>.agents/skills/launch/scripts/launch.sh and .agents/skills/launch/scripts/launch.ps1</code>: Run pre-launch, start Code OSS, and poll its local CDP endpoint; cardinality: One pre-launch unless skipped, one app tree, one readiness helper, and serial localhost probes until ready or 90 seconds
    - Introduced by diff: true; previous behavior: No app was launched because the missing profile caused immediate failure.; critical-path effect: Defines time to the sign-in UI
- Verdict: No performance finding: the added launch cost is the requested functionality and there was no previously progressing scenario whose latency regressed.

### Scenario 3: Bootstrap a persistent profile, sign in interactively, close Code OSS, then wait for profile release

- Critical path: Profile directory creation, Code OSS startup, and serial localhost CDP probes are required before the bootstrap command returns. The new process-tree monitors run concurrently and are off the readiness critical path, but continue consuming resources throughout the human sign-in interval. The final wait polls only an exit-marker file for up to 30 seconds.
- Scaling input: Machine process count and human-controlled bootstrap-window lifetime; monitor cost is O(processes × lifetime), with additional descendant-depth rescans.
- Effective concurrency: One monitor per bootstrap. Each Unix \`ps\` or Windows CIM query is synchronous so ticks do not overlap, but monitoring overlaps the Code OSS workload and continues until every tracked descendant exits.
- Cache behavior: No process snapshot is cached or incrementally updated; every 250 ms tick refetches and rebuilds the complete process set. Exit-marker checks are uncached but bounded.
- Mechanism confidence: Very high: cadence, full-table boundary, detached lifetime, and interactive duration are all explicit in changed code and documentation.
- Magnitude uncertainty: Per-scan cost varies with OS and process count, and users may sign in quickly, but several minutes is realistic and an unclosed window makes duration unbounded.
- Expensive boundaries:
  - subprocess at <code>.agents/skills/launch/scripts/bootstrap/bootstrap-profile.sh:54 and .agents/skills/launch/scripts/bootstrap/bootstrap-profile.ps1:69</code>: Launch Code OSS against the persistent profile; cardinality: One application process tree per explicit bootstrap
    - Introduced by diff: true; previous behavior: Documentation had the user launch Code OSS directly, so the same application boundary existed manually.; critical-path effect: Required to present sign-in UI; readiness waits for its CDP endpoint
  - network at <code>.agents/skills/launch/scripts/waitForCdp.ts:34</code>: Probe \`127.0.0.1:&lt;cdpPort&gt;/json/version\` until ready; cardinality: One non-overlapping HTTP request approximately every 100-300 ms until readiness, bounded by 90 seconds
    - Introduced by diff: true; previous behavior: The manual setup instruction did not provide readiness synchronization.; critical-path effect: Bootstrap output waits for the first successful response
  - subprocess at <code>.agents/skills/launch/scripts/bootstrap/bootstrap-profile.sh:71</code>: Spawn \`ps\` and parse the full process table every 250 ms; cardinality: Four subprocesses per second for the entire app lifetime; each scan scales with all machine processes
    - Introduced by diff: true; previous behavior: Direct manual launch had no detached monitor or recurring \`ps\` subprocesses.; critical-path effect: Off the readiness critical path but concurrent with the app and user sign-in
  - IPC at <code>.agents/skills/launch/scripts/bootstrap/windows-process-tree-monitor.ps1:19</code>: Query and materialize all \`Win32\_Process\` instances over CIM every 250 ms; cardinality: Four machine-wide CIM queries per second for the entire app lifetime
    - Introduced by diff: true; previous behavior: Direct \`code.bat\` launch had no hidden PowerShell/CIM monitor.; critical-path effect: Off the readiness critical path but competes for CPU/IPC resources during sign-in
  - filesystem at <code>.agents/skills/launch/scripts/bootstrap/bootstrap-profile.sh:25 and .agents/skills/launch/scripts/bootstrap/bootstrap-profile.ps1:20</code>: Test the exit marker during final wait; cardinality: Up to 120 file existence checks at 250 ms intervals, normally one after a completed close
    - Introduced by diff: true; previous behavior: There was no scripted release acknowledgement.; critical-path effect: Blocks only the explicit post-close release check
- Verdict: Two medium findings submitted: recurring Unix \`ps\` subprocess churn and recurring Windows CIM process-table polling.

### Scenario 4: Clean up an isolated Windows launch

- Critical path: Optionally close one Playwright session, enumerate exact-profile Code OSS processes, stop matches with bounded retries, verify no matches remain, then recursively delete the run directory.
- Scaling input: Number of matching Code OSS processes and files in one isolated run directory; process scans are capped and exact-profile filtered.
- Effective concurrency: All cleanup actions and retries are serialized; no burst or unbounded queue is introduced.
- Cache behavior: No cache is relevant. Process state is intentionally refreshed between bounded termination attempts; package resolution uses the repository's installed Playwright dependency.
- Mechanism confidence: High confidence that all new expensive boundaries are bounded and scoped to one cleanup operation.
- Magnitude uncertainty: CIM and deletion latency vary with machine load/profile size, but retry counts are fixed and cleanup is an explicit teardown path.
- Expensive boundaries:
  - subprocess at <code>.agents/skills/launch/scripts/cleanup/cleanup.ps1:26</code>: Invoke the local Playwright CLI to close a named session; cardinality: Zero or one CLI process per cleanup
    - Introduced by diff: true; previous behavior: No Windows cleanup helper existed.; critical-path effect: Runs before process termination but failure does not prevent cleanup
  - IPC at <code>.agents/skills/launch/scripts/cleanup/cleanup.ps1:40 and :58</code>: Enumerate \`Win32\_Process\` and parse command lines to find exact user-data arguments; cardinality: Two scans when no process remains; at most six scans across five stop attempts plus verification
    - Introduced by diff: true; previous behavior: Users manually killed the returned wrapper PID and removed the directory, which was unreliable on Windows.; critical-path effect: Serial and required to avoid deleting a profile still in use
  - filesystem at <code>.agents/skills/launch/scripts/cleanup/cleanup.ps1:69</code>: Recursively delete the isolated run directory; cardinality: One successful recursive delete, with at most five serialized attempts
    - Introduced by diff: true; previous behavior: Deletion was manual.; critical-path effect: Cleanup completes only after deletion
- Verdict: No reportable regression: the work is bounded teardown necessary for reliable Windows cleanup, not a hot or lifetime path.

### Scenario 5: Paste text into a chat Monaco editor through Playwright

- Critical path: Focus the chat input, optionally send select-all and backspace, write/run one temporary paste script, parse its result, and remove the temporary directory. These operations are serialized before the caller can submit the prompt.
- Scaling input: Prompt bytes and rendered view-line count; verification remains capped at 20 attempts and uses only a 40-character expected prefix.
- Effective concurrency: CLI operations are strictly serial with no new fan-out. One helper process handles argument parsing and result parsing.
- Cache behavior: The helper resolves the installed CLI module once per paste process and reuses that path for all calls. There is no cross-invocation cache, matching the old process-per-invocation behavior.
- Mechanism confidence: High confidence that effective CLI cardinality and DOM work are preserved while package-launch overhead is reduced.
- Magnitude uncertainty: Temporary-file latency varies by filesystem, but it replaces shell/inline-command and jq overhead and is fixed at one small file.
- Expensive boundaries:
  - subprocess at <code>.agents/skills/launch/scripts/monaco-paste.ts:91</code>: Run Playwright CLI commands through direct Node entrypoint; cardinality: Two calls when appending (focus and paste), four in default replace mode (focus, two keypresses, paste)
    - Introduced by diff: false; previous behavior: The Bash implementation made the same two/four CLI calls through \`npx\`, plus separate Node and jq processes.; critical-path effect: Every call is serial and required before paste completion
  - filesystem at <code>.agents/skills/launch/scripts/monaco-paste.ts:185</code>: Create a temporary directory, write one generated JavaScript file, and recursively remove it; cardinality: One create/write/remove sequence per paste
    - Introduced by diff: true; previous behavior: The old helper passed generated JavaScript inline and did not create this temporary file.; critical-path effect: The file is required by \`run-code\`; cleanup happens before result parsing
  - IPC at <code>.agents/skills/launch/scripts/monaco-paste.ts:108-181</code>: Execute focus, keypress, DOM paste, and verification through the existing Playwright/CDP session; cardinality: Same command count as before; verification is capped at 20 attempts and scans rendered view lines
    - Introduced by diff: false; previous behavior: Equivalent focus, keypress, paste event, and capped rendered-line verification ran through the Bash helper.; critical-path effect: Defines paste latency, unchanged in effective cardinality
- Verdict: No performance finding: the rewrite does not amplify subprocess, IPC, DOM, or verification cardinality, and the new temporary-file work is bounded.

**Summary:** Completed the performance pass over all 10 changed files and all diff pages. Submitted two high-confidence medium findings for unbounded-lifetime process-table polling in the new Unix and Windows bootstrap monitors; other changed launch, cleanup, and paste paths are bounded, intentional, or preserve prior expensive-boundary cardinality.

</details>

<details>
<summary>Review statistics</summary>

- Model: `gpt-5.6-sol`
- Actual model calls: `gpt-5.6-sol`
- Reasoning effort: `high`
- Actual reasoning effort: `high`
- Model API endpoint: `ws:/responses`
- Wall-clock time: 2m26.408s
- Model calls: 7
- Tokens: 236592 input, 9577 output, 246169 total; 4202 reasoning; 181830 cache read, 54741 cache write
- Aggregate model API time: 2m9.713s
- Model-returned tool calls: 32
- Copilot usage: 53806100000.000 nano-AI units; model billing multiplier: 1.000
- USD cost: unavailable; the Copilot SDK does not expose a dollar conversion.

</details>
