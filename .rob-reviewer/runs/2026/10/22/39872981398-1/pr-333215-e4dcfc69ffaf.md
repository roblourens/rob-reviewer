# Performance review: launch: improve cross-platform setup and cleanup

[PR #333215](https://github.com/microsoft/vscode/pull/333215) at `e4dcfc69ffafaa5ade25f89bbc106ecbb5543d9e`

## Findings (2)

### 1. [medium] Stop spawning \`ps\` four times per second for the bootstrap lifetime

**Finding ID:** `PERF-65D4DD845F45`  
**Location:** <code>.agents/skills/launch/scripts/bootstrap/bootstrap-profile.sh:110</code> (<code>RIGHT</code>)  
**Performance category:** <code>subprocess</code>  
**Mechanism family:** <code>repeated-work</code>  
**Resource:** \`ps\` subprocesses plus CPU and allocations used to parse and rescan the complete process table  
**Scaling:** Cost grows as 4 full scans per second × system process count × time the bootstrap window remains open; concurrent bootstrap windows multiply the rate.  
**Outcome:** Sustained avoidable CPU/subprocess activity and reduced battery life while waiting for bootstrap sign-in  
**PR causality:** <code>introduced</code>  
**Previous behavior:** Before this PR there was no bootstrap helper or detached process-tree monitor, so leaving a persistent Code OSS profile open did not create recurring \`ps\` subprocesses or full process-table parsing.  
**Changed behavior:** The new detached bootstrap monitor schedules \`check\` every 250 ms; every check synchronously spawns \`ps\`, parses every system process, rebuilds PID sets, and walks the process list to extend the tracked tree.  
**Confidence:** 0.99 — The interval, synchronous subprocess call, all-process arguments (\`ps -ax\`/\`ps -e\`), and post-readiness lifecycle are explicit. Only the exact host process count and how long a user leaves the sign-in window open are uncertain, which affects magnitude rather than mechanism.

**Causal diff evidence:** RIGHT line 110 newly installs a 250 ms interval around \`check\`; the same added block shows each invocation synchronously runs whole-system \`ps\`.

During the documented flow the bootstrap script returns at CDP readiness and the user may spend minutes signing in before closing Code OSS. The monitor therefore launches about 240 \`ps\` processes per minute for each bootstrap window, causing avoidable CPU, subprocess, and battery load for the entire interaction.

**Evidence:** Added lines 57-110 launch a detached Node monitor. \`check\` at lines 68-103 calls \`execFileSync("ps", psArgs)\`, parses all rows, and scans them for descendants; line 110 repeats that work every 250 ms. SKILL.md lines 101-108 explicitly direct the caller to wait while the user signs in and only then close the application and consume the exit marker, proving the monitor remains live beyond startup.

**Suggested direction:** Keep fast discovery only through initial CDP readiness, then switch to event-driven process tracking or a much slower idle cadence, returning to a short cadence only after a stop request. Preserve the exit marker and tree-cleanup semantics.

### 2. [medium] Avoid polling WMI for every process four times per second

**Finding ID:** `PERF-1585E2AD5F51`  
**Location:** <code>.agents/skills/launch/scripts/bootstrap/windows-process-tree-monitor.ps1:55</code> (<code>RIGHT</code>)  
**Performance category:** <code>ipc</code>  
**Mechanism family:** <code>repeated-work</code>  
**Resource:** WMI/CIM IPC calls, full process objects, and CPU used to enumerate and rescan the system process table  
**Scaling:** Cost grows as 4 full WMI enumerations per second × system process count × duration of the open bootstrap window; multiple bootstraps multiply it.  
**Outcome:** Sustained avoidable CPU/IPC activity and reduced responsiveness or battery life during bootstrap sign-in  
**PR causality:** <code>introduced</code>  
**Previous behavior:** Before this PR the Windows bootstrap and its monitor did not exist, so an open persistent Code OSS sign-in window incurred no recurring WMI/CIM process-table queries from this skill.  
**Changed behavior:** The newly spawned hidden PowerShell monitor loops every 250 ms and executes an unfiltered \`Get-CimInstance Win32\_Process\`, materializes the complete process list, and repeatedly scans it to discover descendants and liveness.  
**Confidence:** 0.99 — The unfiltered CIM query, full-list scans, 250 ms loop, detached ownership, and post-readiness user-wait lifecycle are explicit. Machine process count and user sign-in duration only affect the size of the regression.

**Causal diff evidence:** RIGHT line 55 newly sets the loop cadence to 250 ms; the added loop body at lines 18-33 performs an unfiltered whole-system CIM query and repeated process-list scans each iteration.

The documented bootstrap flow leaves this hidden monitor running after readiness while a user signs in and closes the window. This causes about 240 full WMI process enumerations per minute, adding sustained CPU and WMI/IPC pressure throughout a potentially minutes-long interactive wait.

**Evidence:** bootstrap-profile.ps1 lines 73-75 starts this monitor detached before waiting for CDP and never stops it on success. In this file, lines 18-33 query and materialize all \`Win32\_Process\` instances and scan them, while added line 55 repeats the loop after only 250 ms. SKILL.md lines 101-108 establish that the process remains open while awaiting user acknowledgement.

**Suggested direction:** Track descendants with OS process/job or event mechanisms where possible. At minimum, use the 250 ms cadence only during initial startup/active shutdown and poll much less frequently after CDP readiness while preserving the exit-marker contract.


<details>
<summary>Reviewer scenario analysis</summary>

**Relevant issue families:** repeated-work, boundary-fanout, cleanup-lifecycle, serialization-allocation, concurrency-burst

### Scenario 1: Normal isolated launch with an existing or missing source profile

- Critical path: Path resolution and source-profile checks precede profile preparation; when a profile exists, the existing copy/settings/prelaunch/app-start/CDP-ready chain remains. When it is missing, the diff skips source copy and auth preflight, creates an empty destination, then newly continues through Code OSS startup and waits for CDP instead of aborting.
- Scaling input: One source profile and one destination per launch. Existing-profile cost scales with profile bytes as before; missing-profile launches copy no source files. The added Unix path normalization starts one Node process per launch.
- Effective concurrency: Preparation and startup are serialized within each launcher. Separate isolated launches can run concurrently, as before; there is no added shared queue or burst beyond the intentionally newly reachable app launch for a missing profile.
- Cache behavior: No new cache is added. Existing source-profile contents act as the seed; a missing profile is represented explicitly and skips auth/preflight and copy work. Repeated launches each prepare a fresh isolated directory.
- Mechanism confidence: High: branches and boundary placement are explicit in both launchers.
- Magnitude uncertainty: The startup cost newly reached by users without a profile varies by build and machine, but it is the requested workflow rather than avoidable duplicate work.
- Expensive boundaries:
  - subprocess at <code>.agents/skills/launch/scripts/launch.sh:90</code>: Resolve the source path with \`node -e\` on macOS/Linux; cardinality: Exactly once per launch
    - Introduced by diff: true; previous behavior: The shell validated and canonicalized only an existing source directory without starting Node here.; critical-path effect: Adds one Node startup before profile preparation, but is constant and small relative to launching Electron.
  - filesystem at <code>.agents/skills/launch/scripts/launch.sh:87-183 and .agents/skills/launch/scripts/launch.ps1:429-500</code>: Check/create/copy source and destination profile directories and update settings; cardinality: One source/destination group per launch; copy cardinality remains one grouped profile copy when a source exists and zero when absent.
    - Introduced by diff: false; previous behavior: Existing-profile launches already copied the profile and updated settings; missing profiles aborted before this boundary.; critical-path effect: Profile preparation must finish before Code OSS starts. Missing-profile launches now create only the empty destination rather than failing.
  - subprocess at <code>.agents/skills/launch/scripts/launch.sh and .agents/skills/launch/scripts/launch.ps1</code>: Start Code OSS and wait for its local CDP endpoint; cardinality: One application process tree and one bounded readiness loop per launch
    - Introduced by diff: true; previous behavior: A missing profile caused immediate usage failure; existing profiles already incurred application startup and readiness waiting.; critical-path effect: Launch output waits for readiness. This is newly reachable for a missing source profile, intentionally enabling sign-in; unchanged for an existing profile.
- Verdict: No reportable regression. Existing-profile work is not amplified; missing-profile app startup is the intentional behavior, and source copy/preflight work is skipped.

### Scenario 2: Bootstrap a persistent profile and wait while the user signs in on macOS, Linux, or Windows

- Critical path: Bootstrap creates profile/log state, starts Code OSS, starts a detached tree monitor, and synchronously probes local CDP until ready. User sign-in occurs after bootstrap returns; the detached monitor remains off the immediate readiness critical path but consumes resources until Code OSS exits. After acknowledgement, \`--wait-for-exit\` polls the marker and blocks relaunch until cleanup completes.
- Scaling input: One Code OSS process tree per bootstrap. The reported monitor cost scales with system process count, four scans per second, sign-in/window lifetime, and number of concurrent bootstrap windows.
- Effective concurrency: Each bootstrap creates one independent detached monitor. Unix synchronously serializes \`ps\` calls inside each monitor; Windows serializes CIM queries inside each monitor. Concurrent bootstraps multiply aggregate scan rate without a shared limiter.
- Cache behavior: There is no cache or coalescing for process-table snapshots. Every 250 ms scan refetches and rematerializes the complete table even when no process state changed. Persistent profile data may be warm across launches but does not reduce monitor work.
- Mechanism confidence: Very high: detached ownership, 250 ms cadence, all-process queries, and documented post-readiness user wait are explicit.
- Magnitude uncertainty: Exact CPU/IPC cost depends on host process count and the time a user leaves the sign-in window open; the 4 Hz whole-system scan mechanism and unbounded-by-readiness lifetime are certain.
- Expensive boundaries:
  - subprocess at <code>.agents/skills/launch/scripts/bootstrap/bootstrap-profile.sh:53 and .agents/skills/launch/scripts/bootstrap/bootstrap-profile.ps1:69</code>: Start Code OSS through \`code.sh\`/\`code.bat\`; cardinality: One application process tree per bootstrap
    - Introduced by diff: true; previous behavior: No dedicated bootstrap workflow existed.; critical-path effect: Required before readiness and user sign-in can proceed.
  - network at <code>.agents/skills/launch/scripts/waitForCdp.ts:34-66</code>: Probe the loopback CDP \`/json/version\` endpoint; cardinality: At most roughly one local request per 100-300 ms for up to 90 seconds, stopping at readiness
    - Introduced by diff: true; previous behavior: No bootstrap path invoked the existing readiness helper.; critical-path effect: Bootstrap JSON is withheld until readiness; this helper existed already but is newly reused by bootstrap.
  - subprocess at <code>.agents/skills/launch/scripts/bootstrap/bootstrap-profile.sh:68-110</code>: Run whole-system \`ps\`, parse all rows, and rescan descendants; cardinality: Four subprocesses per second for the full bootstrap-window lifetime, about 240 per minute per bootstrap
    - Introduced by diff: true; previous behavior: No bootstrap monitor or recurring \`ps\` process existed.; critical-path effect: Off the post-readiness interaction critical path, but remains as sustained background load and supplies the exit marker.
  - IPC at <code>.agents/skills/launch/scripts/bootstrap/windows-process-tree-monitor.ps1:18-55</code>: Query all \`Win32\_Process\` instances over CIM/WMI and rescan descendants; cardinality: Four full CIM enumerations per second for the full bootstrap-window lifetime, about 240 per minute per bootstrap
    - Introduced by diff: true; previous behavior: No Windows bootstrap monitor or recurring CIM enumeration existed.; critical-path effect: Off the post-readiness interaction critical path, but remains as sustained background load and supplies cleanup/exit tracking.
  - filesystem at <code>.agents/skills/launch/scripts/bootstrap/bootstrap-profile.sh and bootstrap-profile.ps1</code>: Create and poll stop/exit marker files and append logs; cardinality: One marker pair and log per bootstrap; wait mode checks the exit marker four times per second for at most 30 seconds
    - Introduced by diff: true; previous behavior: No bootstrap marker protocol existed.; critical-path effect: Marker polling blocks only the explicit post-acknowledgement wait; otherwise it is small compared with process scans.
- Verdict: Reportable on both implementations: two medium repeated-work findings were submitted for sustained whole-system process polling.

### Scenario 3: Paste text into a Monaco chat input through Playwright

- Critical path: The helper reads input, resolves the local Playwright CLI, sequentially focuses the editor, optionally sends select-all/delete, writes one temporary run-code file, executes browser-side paste/verification, parses output, and removes the temporary directory before returning.
- Scaling input: One invocation per paste. Subprocess cardinality is three CLI calls in append mode or four otherwise, matching the old helper. Serialization and browser verification scale with text size and rendered view-line count, with the same 20-attempt cap as before.
- Effective concurrency: CLI operations inside one invocation are synchronous and ordered. Separate named sessions can run concurrently as before; no new shared limiter or aggregate fan-out is introduced.
- Cache behavior: \`require.resolve\` is memoized only within the short-lived helper process. There is no cross-invocation cache, but direct CLI resolution avoids the old \`npx\` wrapper startup on each CLI command. Verification has no cache and intentionally samples current DOM state.
- Mechanism confidence: High: old and new command cardinality and verification loops are visible in the diff.
- Magnitude uncertainty: Temporary-file latency varies by filesystem, but it is one bounded file per user/agent paste and is offset by removing repeated \`npx\` wrapper resolution; no realistic regression mechanism is established.
- Expensive boundaries:
  - subprocess at <code>.agents/skills/launch/scripts/monaco-paste.ts:88-116 and 190</code>: Invoke the Playwright CLI for focus, key presses, and paste run-code; cardinality: Three calls with \`--append\`, four without; one logical paste per invocation
    - Introduced by diff: false; previous behavior: The shell helper performed the same focus, optional two key presses, and one eval through \`npx\`; the diff replaces the wrapper with direct Node CLI launches rather than increasing call count.; critical-path effect: All calls are sequential and must complete before paste success is returned.
  - filesystem at <code>.agents/skills/launch/scripts/monaco-paste.ts:185-195</code>: Create, write, and recursively remove one temporary directory containing \`paste.js\`; cardinality: One small directory and file per paste
    - Introduced by diff: true; previous behavior: The old helper passed generated JavaScript directly to CLI eval and created no temporary file.; critical-path effect: Synchronous and on the paste critical path, but bounded to one generated script; replaces passing the generated script through command arguments.
  - serialization at <code>.agents/skills/launch/scripts/monaco-paste.ts:118-218</code>: Embed input text into generated JavaScript and parse CLI result JSON; cardinality: One serialization and result parse per paste; output is bounded to verification metadata
    - Introduced by diff: false; previous behavior: The shell implementation also JSON-escaped input and parsed a JSON result, using Node and jq.; critical-path effect: Required for paste execution and status reporting.
- Verdict: No finding. The helper preserves browser-operation cardinality and replaces heavier wrapper/JSON tooling; its new single temporary file is bounded.

### Scenario 4: Clean up an isolated Windows launch

- Critical path: Optionally close one Playwright session, enumerate matching Code OSS processes, force-stop them with bounded retries, verify none remain, then recursively delete one run directory with bounded retries.
- Scaling input: One run directory and one optional session. Process enumeration is at least two and at most six whole-system CIM queries; directory deletion scales with files in that isolated run. All retry loops cap at five attempts.
- Effective concurrency: Cleanup operations are serialized. It targets all matching Code OSS processes in each attempt but issues stops sequentially; there is no unbounded parallelism.
- Cache behavior: No cache is used; process state is intentionally refreshed between termination attempts. Command-line parsing compiles the helper type once per PowerShell process and examines only Code OSS entries after the WMI query.
- Mechanism confidence: High: query/deletion counts and retry bounds are explicit.
- Magnitude uncertainty: CIM and recursive deletion duration depend on machine state and run-directory size, but invocation and retries are bounded and occur only during explicit cleanup.
- Expensive boundaries:
  - subprocess at <code>.agents/skills/launch/scripts/cleanup/cleanup.ps1:24-33</code>: Close one named Playwright CLI session through \`npx\`; cardinality: Zero or one invocation per cleanup
    - Introduced by diff: true; previous behavior: No Windows cleanup helper existed.; critical-path effect: Runs first so browser automation releases resources; failure is allowed to proceed to process cleanup.
  - IPC at <code>.agents/skills/launch/scripts/cleanup/cleanup.ps1:38-69</code>: Enumerate \`Win32\_Process\`, parse matching command lines, and stop matching Code OSS processes; cardinality: Two queries when no process remains; up to six with termination retries
    - Introduced by diff: true; previous behavior: Windows documentation had no exact-run cleanup helper; users cleaned up manually.; critical-path effect: Cleanup waits for process release before deleting the run directory.
  - filesystem at <code>.agents/skills/launch/scripts/cleanup/cleanup.ps1:75-88</code>: Recursively remove the isolated run directory; cardinality: One deletion, retried at most five times on failure
    - Introduced by diff: true; previous behavior: No scripted Windows run-directory deletion path existed.; critical-path effect: Cleanup does not complete until deletion succeeds or retries are exhausted.
- Verdict: No finding. Although it performs whole-process enumeration, this is an explicit, bounded cleanup path rather than sustained or input-amplified work.

**Summary:** Completed the performance pass over all 10 changed files and relevant call paths. Submitted two medium repeated-work findings for the new macOS/Linux and Windows bootstrap monitors, each of which scans the complete system process table four times per second for the entire user sign-in lifetime. Other changed launch, paste, and cleanup boundaries are intentional, bounded, or preserve prior operation cardinality.

</details>

<details>
<summary>Review statistics</summary>

- Model: `gpt-5.6-sol`
- Actual model calls: `gpt-5.6-sol`
- Reasoning effort: `high`
- Actual reasoning effort: `high`
- Model API endpoint: `ws:/responses`
- Wall-clock time: 1m57.768s
- Model calls: 7
- Tokens: 201530 input, 8012 output, 209542 total; 2814 reasoning; 156190 cache read, 45319 cache write
- Aggregate model API time: 1m46.131s
- Model-returned tool calls: 34
- Copilot usage: 44939500000.000 nano-AI units; model billing multiplier: 1.000
- USD cost: unavailable; the Copilot SDK does not expose a dollar conversion.

</details>
