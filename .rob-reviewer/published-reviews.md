# Published performance reviews

Newest publications are listed first. This index retains the newest 200 publications; complete history remains in the run archive.

## [#335329 — chore: bump foundry-local-sdk to 2.x](https://github.com/microsoft/vscode/pull/335329)

- Published comments: **3**
- Head: `7b1addd2da741949551bd89aa5fb898281f504aa`
- Completed: `2026-09-09T23:30:33Z`
- Run: [34416179184 attempt 1](runs/2026/09/09/34416179184-1/run.md)

### `src/vs/platform/localTranscription/node/localTranscriptionService.ts:563`

**[Experimental performance review bot]**

**Severity: high**

The added SDK 2.x call invokes `model.download` with only the progress callback, while `cancel()` still merely cancels `_modelPrepareCts`; that token is checked only after the download resolves and can no longer stop the active model transfer.

If a user cancels first-use dictation while the model is downloading, cancellation returns but the utility process continues fetching and writing the large model until completion or network failure.

**Suggested fix:** Restore an abortable SDK 2.x download path and wire `_modelPrepareCts` to it; if the new API offers no per-download signal, cancel by tearing down/recreating the downloader or utility worker and await that cleanup rather than leaving the transfer detached.

(Written by Copilot)

### `build/gulpfile.vscode.ts:263`

**[Experimental performance review bot]**

**Severity: medium**

The added `prebuilds/**/foundry_local_node.node` include uses an unrestricted target wildcard, and the adjacent preload include does the same, so `packageTask(platform, arch, ...)` retains both addons from every SDK prebuild directory instead of just the product's loadable target.

Each packaged desktop build carries up to ten native addon binaries although its support gate can use only two. Every installation and update transfers and stores the other targets' binaries.

**Suggested fix:** Parameterize the filter with the package platform/arch and re-include only the two addon files for the single runtime target that build can load (mapping the universal macOS build to darwin-arm64).

(Written by Copilot)

### `build/dictation-runtime/package.ts:132`

**[Experimental performance review bot]**

**Severity: low**

The product now deliberately bundles both SDK addons and configures `libraryPath` only for shared libraries, but `requiredSdkNativeFileNames` newly forces both `.node` files into the CDN tarball and runtime validation requires them in the cache as well.

On the first dictation after each runtime-version change, users must download and extract cached copies of both addons even though the SDK loads the packaged copies.

**Suggested fix:** Build and validate the CDN payload from only the shared libraries used through `libraryPath`; omit the two `.node` files from staging and from runtime cache completeness checks.

(Written by Copilot)

## [#335303 — sessions: add Dev Container workspace menu option](https://github.com/microsoft/vscode/pull/335303)

- Published comments: **2**
- Head: `83ff3d6b8c1627eb1f6ddc8063c7d3dc399ad5e6`
- Completed: `2026-09-09T23:30:33Z`
- Run: [34416179184 attempt 1](runs/2026/09/09/34416179184-1/run.md)

### `src/vs/sessions/contrib/chat/browser/sessionWorkspacePicker.ts:1896`

**[Experimental performance review bot]**

**Severity: medium**

Each successful availability promise now immediately force-reopens the visible picker instead of batching results or updating the affected list row.

Users with several Dev Container projects in recent workspaces can incur a burst of full list allocations, DOM teardown/recreation, focus work, and layout just after opening the menu, producing visible jank and unnecessary CPU/GC pressure.

**Suggested fix:** Aggregate or debounce availability completions and refresh the visible list once, preferably using updateItems/refreshActiveList (or patching the resolved row) rather than hiding and recreating the popup.

(Written by Copilot)

### `src/vs/sessions/contrib/chat/browser/sessionWorkspacePicker.ts:1470`

**[Experimental performance review bot]**

**Severity: medium**

The added call inside the recent-workspace loop probes Dev Container availability for every local row, and showPicker clears the probe cache whenever the popup is freshly opened. Selecting a row then still creates a draft whose existing \_resolveDevContainerAvailability path probes the selected workspace again.

Opening the picker with a populated local history now launches up to 40 concurrent filesystem stats plus IPC roundtrips; slow disks or file-scheme network folders can generate an I/O burst on a routine UI interaction, and the selected folder is immediately re-statted during draft creation.

**Suggested fix:** Keep availability results across picker opens and invalidate them from bounded provider/configuration/file-change signals, or defer the check to the row/submenu the user interacts with. Also share/coalesce the selected workspace's result with draft creation so the required final validation does not duplicate the same stats.

(Written by Copilot)

## [#335259 — agentHost: change default settings for Codex harness](https://github.com/microsoft/vscode/pull/335259)

- Published comments: **1**
- Head: `3c57460a5e57b3fa4a8788d8c73c0a81204600e0`
- Completed: `2026-09-09T23:30:33Z`
- Run: [34416179184 attempt 1](runs/2026/09/09/34416179184-1/run.md)

### `src/vs/platform/agentHost/common/agentHostStarter.config.contribution.ts:295`

**[Experimental performance review bot]**

**Severity: medium**

The new non-Stable default is forwarded by the starter, causes agentHostMain to instantiate CodexAgent during agent-host prewarm, and the constructor immediately queues \_probeAccountAtStartup.

On an Insiders startup where AI features are enabled and the Codex SDK is already cached, BlockRestore prewarming now launches a native Codex process even if the user never selects Codex. \_probeAccountAtStartup resolves the SDK, creates proxy/temp resources, spawns the binary, performs the initialize/account handshake (and signed-in account metadata requests), then tears it down, potentially contending with editor restoration for up to the 30-second timeout.

**Suggested fix:** Keep the provider/routing defaults if desired, but make registration passive: do not run \_probeAccountAtStartup merely because the provider is enabled. Trigger the one-off account probe lazily when a Codex account surface or Codex session is first requested, and coalesce that first-use probe with persistent connection startup.

(Written by Copilot)

## [#332410 — agentHost: centralize session and chat catalog metadata](https://github.com/microsoft/vscode/pull/332410)

- Published comments: **1**
- Head: `fd5d3014f165531587976426e03479c6653599af`
- Completed: `2026-09-09T15:31:51Z`
- Run: [34369527186 attempt 1](runs/2026/09/09/34369527186-1/run.md)

### `src/vs/platform/agentHost/node/agentService.ts:821`

**[Experimental performance review bot]**

**Severity: medium**

The constructor now schedules reconciliation on a fixed 1-second timer independently of the startup-settled barrier. On a fresh version marker, the pass dirties the whole catalog and reprojects the first 50 sessions.

After upgrading to this schema/version, users with a sizable session history can have up to 50 session databases statted/opened/read and catalog rows verified or rewritten beginning one second after Agent Host construction, while startup or the first session list/load is still in progress. On slower or busy disks this can delay the first useful interaction.

**Suggested fix:** Queue the initial reconciliation through `_runWhenStartupSettled` (or otherwise gate `start`/`schedule` on the same barrier), then begin its periodic schedule after that deferred pass so repair remains automatic without competing with startup.

(Written by Copilot)

## [#332410 — agentHost: centralize session and chat catalog metadata](https://github.com/microsoft/vscode/pull/332410)

- Published comments: **1**
- Head: `b095443a4fb1395b984619437dcf2b371d114e8d`
- Completed: `2026-09-09T01:40:11Z`
- Run: [34299341451 attempt 1](runs/2026/09/09/34299341451-1/run.md)

### `src/vs/platform/agentHost/node/copilot/copilotAgentSession.ts:5342`

**[Experimental performance review bot]**

**Severity: medium**

Tool completion callbacks are now tracked in one set for the lifetime of the session wrapper, and every normal idle waits for all entries in that set, regardless of which turn created them.

After aborting an edit and immediately sending a replacement prompt, the replacement turn can appear stuck at completion until unrelated cleanup from the cancelled turn finishes, increasing end-of-turn latency in proportion to the slowest outstanding prior edit.

**Suggested fix:** Track completion promises by their captured turn and await only the current turn's set. On abort, detach that turn's entries from the idle barrier while still allowing their cleanup/persistence promises to settle.

(Written by Copilot)

## [#334341 — Add opt-in auto-archive/delete inactive sessions with merged pull requests](https://github.com/microsoft/vscode/pull/334341)

- Published comments: **2**
- Head: `5859c8badc910de8359b9f5bc8037f5fc83e55dd`
- Completed: `2026-09-08T23:42:38Z`
- Run: [34291162783 attempt 1](runs/2026/09/08/34291162783-1/run.md)

### `src/vs/platform/agentHost/node/agentHostSessionLifecycle.ts:164`

**[Experimental performance review bot]**

**Severity: medium**

The new cleanup-only path first refreshes all related PRs at the top of \_evaluateCandidate, performs only local settings/summary checks, and then refreshes the same URLs authoritatively again immediately before cleanup.

With the default-on standalone worktree cleanup, Agent Host startup and every hourly pass issue two GitHub core reads per related PR for each merged/closed cleanup candidate. Users retaining many completed agent worktrees consume twice the required API quota and wait twice as long for the serialized cleanup sweep.

**Suggested fix:** Perform cleanup's local settings/summary revalidation first and make a single \_arePullRequestsComplete call immediately before cleanup; keep the pre-restore check only for archive/delete candidates where restoration creates a meaningful intervening boundary.

(Written by Copilot)

### `src/vs/platform/agentHost/node/agentService.ts:2129`

**[Experimental performance review bot]**

**Severity: medium**

The default-enabled cleanup pass enumerates all internal registered sessions. For an unloaded session `archived` is undefined, making this condition true unconditionally, so `tryOpenDatabase` runs before the code can reject sessions without related PR metadata or a worktree.

Users with large retained agent-session histories now get a catalogue-wide burst of filesystem and SQLite work after provider startup and on every hourly pass, including the common majority of sessions that have no lifecycle-relevant PR/worktree. This can contend with normal Agent Host startup and creates ongoing disk/CPU overhead.

**Suggested fix:** Keep the minimal lifecycle eligibility facts in the existing durable session registry (or an equivalent invalidated candidate index) so the pass can select sessions with related PR/worktree metadata before opening per-session databases; retain the current per-session read as final authoritative validation only for selected candidates.

(Written by Copilot)

## [#335070 — Add word wrap control to Agents editors](https://github.com/microsoft/vscode/pull/335070)

- Published comments: **1**
- Head: `841383d8071492f0ba7b7e04b123d0da02574eb1`
- Completed: `2026-09-08T19:06:37Z`
- Run: [34265039228 attempt 1](runs/2026/09/08/34265039228-1/run.md)

### `src/vs/editor/browser/widget/multiDiffEditor/multiDiffEditorWidget.ts:133`

**[Experimental performance review bot]**

**Severity: medium**

setDiffWordWrap always writes a second fresh \_diffLayoutOptions object. Both SessionsDiffEditorLayoutContribution and SessionChangesEditor call it immediately after setViewMode, so even when neither effective value changed, the same bound entries process another full options update.

With a multi-diff/Changes editor showing many compact entries, switching active editors, changing visible editors, or changing either layout preference synchronously traverses every bound diff twice and updates both inner editors twice, adding avoidable UI-thread work and potential interaction jank.

**Suggested fix:** Expose one setter that applies view mode and word wrap in a single \_diffLayoutOptions.set (and skip the write when the resulting values are unchanged), then use it from both callers; alternatively add equality guards and batch the two fields in one observable transaction.

(Written by Copilot)

## [#335053 — agentHost: surface Codex skill validation errors](https://github.com/microsoft/vscode/pull/335053)

- Published comments: **1**
- Head: `3dc67c0c882fdd00f005130ec907fec2325bebe2`
- Completed: `2026-09-08T19:06:37Z`
- Run: [34265039228 attempt 1](runs/2026/09/08/34265039228-1/run.md)

### `src/vs/platform/agentHost/node/codex/codexCustomizations.ts:362`

**[Experimental performance review bot]**

**Severity: medium**

Invalid skill paths are now projected as E separate top-level containers, which the existing refresh publisher emits as E independent state updates.

When a workspace contains many invalid or legacy skills (for example, a collection whose SKILL.md files all omit the newly required description), every coalesced `skills/changed` refresh now publishes one `SessionCustomizationUpdated` action per invalid file. Those actions are processed synchronously and individually, so repairing or editing skills can cause noticeable Customizations UI churn and agent-host IPC/CPU spikes.

**Suggested fix:** Preserve the visible per-skill diagnostics but batch their state publication—for example, add a directory-customization batch replacement action—or group children that share the same diagnostic into one error container so a common schema failure does not create one top-level update per file.

(Written by Copilot)

## [#334513 — feat: implementing word wrap indicators](https://github.com/microsoft/vscode/pull/334513)

- Published comments: **1**
- Head: `1bc3356bea0674bd4c6a65f6a703ddb4481afd8d`
- Completed: `2026-09-08T19:06:37Z`
- Run: [34265039228 attempt 1](runs/2026/09/08/34265039228-1/run.md)

### `src/vs/editor/browser/viewParts/wordWrapIndicator/wordWrapIndicator.ts:108`

**[Experimental performance review bot]**

**Severity: medium**

The new overlay calls ctx.viewportData.getViewLineRenderingData for every visible line on each invalidation even though it reads only continuesWithWrappedLine.

With wordWrapIndicator enabled, a tall wrapped editor containing many visible decorations (for example search matches, diagnostics, and inline decorations) repeats the viewport-decoration scan once per visible line whenever scrolling or decoration changes trigger rendering; hundreds of visible lines and decorations can therefore produce tens of thousands of checks on a frame and cause dropped frames.

**Suggested fix:** Read continuesWithWrappedLine from this.\_context.viewModel.getViewLineData(lineNumber) (or add an equally narrow accessor) so the pass remains O(V) and avoids allocating full rendering data.

(Written by Copilot)

## [#332410 — agentHost: centralize session and chat catalog metadata](https://github.com/microsoft/vscode/pull/332410)

- Published comments: **1**
- Head: `224066406daa8e1bbd66c00e82b012c742c952a7`
- Completed: `2026-09-08T15:37:07Z`
- Run: [34243720924 attempt 1](runs/2026/09/08/34243720924-1/run.md)

### `src/vs/platform/agentHost/node/agentService.ts:776`

**[Experimental performance review bot]**

**Severity: medium**

Every emitted summary delta now queues `_persistListVisibleSessionStateNow`. That opens the session database, reads all catalog metadata and per-chat title keys, validates/stable-serializes/hashes the full payload, writes the local catalog snapshot, reads/upserts/acknowledges `sessions_v2`, and marks the central payload dirty before and after. This also runs for `activity` changes and transient status-bit changes, although `AgentHostCatalogData` stores no activity and the resolver only projects the IsRead/IsArchived status bits.

During worktree setup, tool/status progress, or multiple active automation sessions, activity/status summary updates can repeatedly rewrite or replay unchanged catalog state. This adds avoidable disk traffic and globally serialized host-database work, delaying list and persistence operations even though the list-visible payload is unchanged.

**Suggested fix:** Gate the queue on fields that can alter `AgentHostCatalogData`; specifically ignore activity-only deltas and transient status changes when the IsRead/IsArchived projection is unchanged. A cached projected signature or comparison of the projected status bits can preserve required title, recency, project, changes, working-directory, metadata, read, and archive synchronization while avoiding no-op full writes.

(Written by Copilot)

## [#333196 — chat: add customization migration assessment telemetry](https://github.com/microsoft/vscode/pull/333196)

- Published comments: **1**
- Head: `cd4d9a1f199af5462d0e97190012a87f28eca618`
- Completed: `2026-09-08T11:50:47Z`
- Run: [34221279571 attempt 1](runs/2026/09/08/34221279571-1/run.md)

### `src/vs/sessions/services/sessions/browser/sessionsService.ts:939`

**[Experimental performance review bot]**

**Severity: medium**

Startup restore now queues a full customization migration assessment for every restored session. Although each enqueue receives the restore cancellation token, queued and running factories discard it once admitted by the limiter.

If the user opens a session while startup is restoring several persisted sessions—especially while agent/customization state is cold—all stale restore assessments continue in the background. They consume filesystem/extension-provider I/O and MCP assessment CPU after the restore was abandoned, contending with the newly opened session and prolonging post-navigation activity.

**Suggested fix:** Recheck the restore token inside the limiter factory and pass it to `reportCustomizationMigrationTelemetry`; make the MCP assessment wait cancellation-aware as well so already-running work releases its scope promptly. This preserves the concurrency limit and telemetry for restores that remain active while dropping only superseded work.

(Written by Copilot)

## [#334369 — Add MCP customization migration](https://github.com/microsoft/vscode/pull/334369)

- Published comments: **1**
- Head: `b65e332fb2ba19b92bdd889dc326215274392e47`
- Completed: `2026-09-08T00:06:10Z`
- Run: [34171267390 attempt 1](runs/2026/09/08/34171267390-1/run.md)

### `src/vs/workbench/contrib/chat/browser/aiCustomization/mcpServerCustomizationMigration.ts:224`

**[Experimental performance review bot]**

**Severity: medium**

The new execution loop invokes `findCrossRootConflicts` for every migration group; that helper serially reads both MCP configuration locations in every other workspace root, and the same full-root scan is invoked again after the target write and after verification.

Confirming migration for servers spread across a 10-root workspace causes roughly 540 cross-root conflict reads before counting source/target verification and write guards, and the nested loops await them serially. On remote filesystems this can turn a one-click migration into a long-running operation.

**Suggested fix:** Build a conflict index by reading each root's source and target once per validation phase, then check all groups against that index. Preserve concurrency safety by refreshing only roots whose files may have changed after each write (or track etag/mtime and re-read changed documents), rather than rescanning every file for every group.

(Written by Copilot)

## [#333196 — chat: add customization migration assessment telemetry](https://github.com/microsoft/vscode/pull/333196)

- Published comments: **2**
- Head: `8a35417c67d8dfce97b4a61ef90ed0d7df397b80`
- Completed: `2026-09-08T00:06:10Z`
- Run: [34171267390 attempt 1](runs/2026/09/08/34171267390-1/run.md)

### `src/vs/sessions/services/sessions/browser/sessionsService.ts:789`

**[Experimental performance review bot]**

**Severity: medium**

Every `openChat` now starts `computeMigrations` for the owning session, even when the session is already active and only its selected child chat changes.

For sessions with multiple chats, normal tab switching or reopening a side chat repeatedly recomputes the same session-wide assessment. Users with many MCP servers/customizations can see avoidable background CPU and extension-host traffic competing with chat navigation.

**Suggested fix:** Trigger the assessment only when navigation actually changes the active session (for example, compare the previously active session before `_activate`), while keeping the session-open telemetry path for genuine session opens; do not run it for child-chat selection within the same session.

(Written by Copilot)

### `src/vs/sessions/services/sessions/browser/sessionsService.ts:1614`

**[Experimental performance review bot]**

**Severity: medium**

After `restoreGrid`, all resolved sessions simultaneously start `computeMigrations`; late sessions do the same from `place`, with no queue or concurrency bound.

Restoring a window with several pinned sessions, especially across different workspaces, can produce an avoidable startup CPU/IPC spike and GC pressure while the restored UI is becoming interactive.

**Suggested fix:** Feed restore telemetry assessments through a small concurrency limiter or idle queue with restore-token cancellation; coalesce only assessments proven to share the same session/resource key, while still emitting the required per-session aggregate telemetry.

(Written by Copilot)

## [#332410 — agentHost: centralize session and chat catalog metadata](https://github.com/microsoft/vscode/pull/332410)

- Published comments: **1**
- Head: `12f279f126ae279b3a69347c4e1dec15300e012f`
- Completed: `2026-09-08T00:06:10Z`
- Run: [34171267390 attempt 1](runs/2026/09/08/34171267390-1/run.md)

### `src/vs/platform/agentHost/node/agentHostDatabase.ts:1281`

**[Experimental performance review bot]**

**Severity: medium**

AgentHostPeerChatStore now sends every single-chat upsert/remove through replaceSessionChatCatalog; that method deletes all rows and awaits one SQLite INSERT for every surviving/current chat, then the compatibility path still writes the legacy blob.

Creating or removing a peer chat, or changing one chat's model/provider data, can issue about 1,000 serialized INSERTs for a large session. Repeated chat creation accumulates quadratic writes and stalls the operation and other work sharing the orchestrator database.

**Suggested fix:** Keep the revision/CAS and legacy-mirror semantics, but add mutation-specific database operations: UPDATE one row for provider-data changes, INSERT one appended row for creation, and DELETE plus a set-based order adjustment for removal. Reserve full replacement for migration/reconciliation; alternatively execute a true bulk insert rather than one awaited statement per row.

(Written by Copilot)

## [#334889 — Sort chat pill dropdown entries newest first](https://github.com/microsoft/vscode/pull/334889)

- Published comments: **1**
- Head: `3e5d8f7d9fdfe9645aa53cc9fde4e78fccd1d149`
- Completed: `2026-09-07T19:05:27Z`
- Run: [34153072178 attempt 1](runs/2026/09/07/34153072178-1/run.md)

### `src/vs/workbench/contrib/browserView/common/browserView.ts:113`

**[Experimental performance review bot]**

**Severity: medium**

The shared newest-first helper inserts every match at index zero, shifting the accumulated array for each matching browser on every recomputation.

When an agent session accumulates many browser pages, each page load/navigation emits several label events; both pill surfaces can repeatedly perform quadratic array copying before updating the UI, delaying rendering and increasing GC pressure.

**Suggested fix:** Append matching inputs with `push` during the map walk and call `views.reverse()` once afterward; this preserves newest-first ordering and restores O(n) construction.

(Written by Copilot)

## [#334369 — Add MCP customization migration](https://github.com/microsoft/vscode/pull/334369)

- Published comments: **2**
- Head: `7f2ff6b9918273ec374046e7ee8e359e2861ff62`
- Completed: `2026-09-07T14:40:43Z`
- Run: [34132873477 attempt 1](runs/2026/09/07/34132873477-1/run.md)

### `src/vs/workbench/contrib/chat/browser/aiCustomization/mcpServerCustomizationMigration.ts:243`

**[Experimental performance review bot]**

**Severity: medium**

The new execution loop calls findCrossRootConflicts once for each source/target group, while findCrossRootConflicts sequentially reads both MCP configuration files in every other root; migrateGroup repeats that full cross-root scan twice more for revalidation.

Migrating eligible servers spread across a multi-root workspace, especially through a remote filesystem provider, blocks completion on hundreds or thousands of sequential file operations; e.g. 20 roots produce roughly 2,280 cross-root reads.

**Suggested fix:** Snapshot each root's source and target once per required validation phase, build a name-to-root conflict index, and reuse it for all groups; preserve optimistic concurrency by revalidating each file once at the phase boundary (or only affected names/files), rather than rescanning every root for every group.

(Written by Copilot)

### `src/vs/workbench/contrib/chat/browser/aiCustomization/aiCustomizationManagementEditor.ts:1175`

**[Experimental performance review bot]**

**Severity: medium**

The PR adds an IMcpWorkbenchService onChange listener and a separate autorun over the same IMcpService server list and enablement observables. Both immediately call refreshCustomizationMigrationInfo, which only uses a sequence to discard stale results and does not cancel or coalesce the work already started.

While the Customizations editor is open, toggling/installing/updating an MCP server can launch duplicate full refreshes. In an enabled multi-root migration setup this doubles per-root configuration reads; even with MCP migration disabled by default, it unnecessarily reruns the default-enabled prompt migration and loading render for unrelated MCP changes.

**Suggested fix:** Route all MCP signals through one RunOnceScheduler/delayer (or subscribe to one authoritative source), gate it on the MCP migration setting, and cancel/suppress superseded refresh computations so one logical MCP change produces one assessment, one per-root planning pass, and one UI update.

(Written by Copilot)

## [#334833 — ci: Parallelize integration and smoke test shards](https://github.com/microsoft/vscode/pull/334833)

- Published comments: **2**
- Head: `fcf9b6955c85eecfa69338e8492f3d51d84aa06c`
- Completed: `2026-09-07T08:43:26Z`
- Run: [34101442004 attempt 1](runs/2026/09/07/34101442004-1/run.md)

### `.github/workflows/pr.yml:153`

**[Experimental performance review bot]**

**Severity: medium**

The integration jobs now always skip Agent Host E2E, while each affected platform starts a separate full reusable workflow whose only enabled test is Agent Host E2E, repeating all unconditional preparation solely to move the suite beside the integration job.

A realistic platform or build change now causes three extra full repository preparations before the same three platform suites run. Parallel execution can shorten that PR's longest check, but the duplicated CPU, cache traffic, and Electron downloads consume additional shared CI capacity and reduce throughput for concurrent PRs.

**Suggested fix:** Keep Agent Host E2E parallel, but fan the integration and E2E test jobs out from one prepared build artifact per platform, or otherwise make the dedicated E2E workflow consume existing compiled outputs instead of rerunning the full platform bootstrap.

(Written by Copilot)

### `.github/workflows/pr.yml:174`

**[Experimental performance review bot]**

**Severity: medium**

The added chat job invokes the same reusable Linux workflow as an independent job; analogous added macOS and Windows chat jobs and the macOS proxy job each independently repeat all unconditional preparation before running only their shard.

On every PR, smoke coverage now launches two complete prepared environments per platform and three on macOS instead of one, even though each added environment runs only a complementary subset. This trades test wall-clock latency for a fixed four-job burst of duplicated CPU and I/O, which consumes shared/self-hosted capacity and lowers repository-wide CI throughput under realistic concurrent PR traffic.

**Suggested fix:** Preserve the shards but prepare the smoke build once per platform and have core/chat/proxy jobs consume that prepared artifact; if a shared artifact is not viable, keep the suites in one workflow until shard-specific jobs can skip the unrelated checkout/cache/build work.

(Written by Copilot)

## [#332410 — agentHost: centralize session and chat catalog metadata](https://github.com/microsoft/vscode/pull/332410)

- Published comments: **2**
- Head: `9c0669cb71f4516b788f6943b870ab8c602af505`
- Completed: `2026-09-06T20:44:10Z`
- Run: [34058138108 attempt 1](runs/2026/09/06/34058138108-1/run.md)

### `src/vs/platform/agentHost/node/agentService.ts:2320`

**[Experimental performance review bot]**

**Severity: medium**

After the nominally batched registry update, the added line invokes \_markCatalogPayloadDirty once for every advanced session. Each invocation runs BEGIN, SELECT, INSERT/UPDATE, SELECT, and COMMIT through the same transaction sequencer; the new updateSessionModifiedTimes implementation also issues two awaited UPDATE calls per item.

When discovery returns many existing sessions with newer timestamps, completion is blocked on roughly N serialized transactions plus 2N sequential UPDATE bridge calls instead of one bulk transaction. Large provider catalogs therefore make discovery and list invalidation noticeably slower.

**Suggested fix:** Add a bulk database operation that updates both registry tables and advances all affected payload-dirty markers in one sequenced transaction (using a batched statement/script or reused prepared statements), then schedule reconciliation once.

(Written by Copilot)

### `src/vs/platform/agentHost/node/agentHostCatalogReconciliationService.ts:557`

**[Experimental performance review bot]**

**Severity: medium**

The new service initializes \_initialPayloadDirtyMarkPending to true and this line marks all payloads dirty on its first pass. AgentService schedules that pass during construction with the default 1-second delay; each dirty row enters reconciliation, opens its session database, resolves provider metadata, rebuilds/hashes the payload, and queries the central database even when already synchronized.

Profiles with dozens or hundreds of sessions incur up to 50 local database/provider validations shortly after every startup, followed by another batch every five minutes until complete; the hourly full sweep repeats this work for unchanged catalogs. This can cause startup-time disk contention and sustained background CPU/I/O.

**Suggested fix:** Gate the full compatibility scan on an upgrade/persisted verification epoch and defer it until genuine idle, or keep a persisted rotating verification cursor that samples a bounded number of clean rows without marking the entire catalog dirty. Preserve event-driven dirtying for known writes.

(Written by Copilot)

## [#334521 — automations: refactor: make provider session templates canonical](https://github.com/microsoft/vscode/pull/334521)

- Published comments: **1**
- Head: `2dec72b1c8c47606885910917b7457e5fa08858e`
- Completed: `2026-09-06T18:26:13Z`
- Run: [34051196811 attempt 1](runs/2026/09/06/34051196811-1/run.md)

### `src/vs/sessions/contrib/automations/browser/automationDialog.ts:365`

**[Experimental performance review bot]**

**Severity: medium**

Retargeting now synchronously captures the previous provider draft before creating the requested draft; stale retarget iterations still pay that capture and leave the old draft installed, causing the next iteration to capture it again.

When an Agent Host/provider configuration request is slow or disconnected, quickly switching folder/provider/session type leaves the session controls unavailable for roughly two seconds for a common A→B→C correction, and further switches can add another timeout while also spawning additional never-settling capture promises.

**Suggested fix:** Coalesce capture by applied session: start at most one capture attempt for the old draft, share its result/deadline across queued target updates, and after it settles or times out create only the latest requested target. Also cancel or otherwise release the underlying provider capture when abandoning the draft.

(Written by Copilot)

## [#334705 — Make action widget menu items directly clickable by Playwright](https://github.com/microsoft/vscode/pull/334705)

- Published comments: **1**
- Head: `2373481a96253a7623f1df8234169b1b000cb224`
- Completed: `2026-09-06T13:18:23Z`
- Run: [34034967193 attempt 1](runs/2026/09/06/34034967193-1/run.md)

### `src/vs/platform/actionWidget/browser/actionList.ts:961`

**[Experimental performance review bot]**

**Severity: low**

The diff registers List.onMouseMove for the full widget lifetime. Although the callback body becomes a no-op once \_ignoreInitialHover is false, ListView still maps every DOM mousemove through toMouseEvent before invoking it.

After the first movement over an open code-action, dropdown, or model-picker list, all later movements still perform DOM target-to-row resolution and allocate a mapped list event. This scales with mouse polling rate and can add avoidable CPU/GC pressure while the popup is being interacted with.

**Suggested fix:** Use a raw disposable `mousemove` listener on the list element and dispose/clear that listener immediately after the first non-zero movement (while retaining mousedown activation), so row hover continues via the existing `onMouseOver` handler without permanent per-move mapping.

(Written by Copilot)

## [#332410 — agentHost: centralize session and chat catalog metadata](https://github.com/microsoft/vscode/pull/332410)

- Published comments: **1**
- Head: `3392bf9d61d3b5c848eb09bfd8d7d7de5e404713`
- Completed: `2026-09-05T15:52:44Z`
- Run: [33975248149 attempt 1](runs/2026/09/05/33975248149-1/run.md)

### `src/vs/platform/agentHost/node/agentService.ts:5313`

**[Experimental performance review bot]**

**Severity: medium**

The passive toggle now awaits `_synchronizePassiveSessionMetadata` before updating surfaced state. That synchronization reads the local snapshot and central row, writes a local pending snapshot, upserts the central catalog, and writes a local acknowledgement, after which this method also marks the payload dirty.

Marking an unopened session read or archived can now remain pending while several SQLite operations complete, and any subsequent action from that client waits behind the same dispatch promise. This is especially visible while startup migration or reconciliation is using the central database sequencer.

**Suggested fix:** After the original metadata write, update/dispatch the surfaced state immediately and enqueue the catalog projection on the existing per-session sequencer/background-write tracker. Preserve deletion ordering by draining that tracked work, and invalidate/reconcile the list when the projection finishes.

(Written by Copilot)

## [#334696 — sessions: measure and unblock V3 onboarding GitHub personalization](https://github.com/microsoft/vscode/pull/334696)

- Published comments: **1**
- Head: `fda8f5b56fbe33ab077346719a380e643ffe3b49`
- Completed: `2026-09-05T12:58:15Z`
- Run: [33966858284 attempt 1](runs/2026/09/05/33966858284-1/run.md)

### `src/vs/sessions/contrib/chat/browser/newChatInput.ts:685`

**[Experimental performance review bot]**

**Severity: medium**

The PR makes prompt options selectable immediately while repository discovery and GitHub personalization are still in flight, then only suppresses later renders after selection instead of cancelling that work.

A user who immediately chooses one of the newly available standard or partial options can still trigger the rest of a 10-second personalization run in the background. That consumes filesystem and GitHub network capacity after its result can no longer be rendered, potentially contending with session startup and wasting up to eight GraphQL requests per impression.

**Suggested fix:** When option selection begins, cancel the active prompt-options refresh/token in addition to setting the selection guard (without clearing the selected/generated input). Preserve the existing guard for focus-only render suppression, since focus alone should not abort personalization.

(Written by Copilot)

## [#334694 — \[cherry-pick\] agentHost: Preserve workspace transition boundaries](https://github.com/microsoft/vscode/pull/334694)

- Published comments: **1**
- Head: `4b4be225afb770efd6718857998959ecc9d1f6ce`
- Completed: `2026-09-05T12:58:15Z`
- Run: [33966858284 attempt 1](runs/2026/09/05/33966858284-1/run.md)

### `src/vs/platform/agentHost/node/chatContributions/sessionWorkspaceConversion/sessionWorkspaceConversionContribution.ts:76`

**[Experimental performance review bot]**

**Severity: low**

Hydrating a converted transcript now performs a fresh storage existence check/database acquisition and a full workspace-transition query after provider history has returned.

Reopening a converted agent session is delayed by an additional filesystem/database round trip after provider history has already completed; sessions with many lazily opened peer or subagent transcripts repeat the delay per transcript.

**Suggested fix:** Read the transition map while the existing restore database reference is open and start that read alongside provider history, then pass the resolved map into hydration. For peer/subagent chats, propagate a storage-specific marker/map so a parent session transition does not trigger empty child-database probes.

(Written by Copilot)

## [#334341 — Add opt-in auto-archive inactive sessions with merged pull requests](https://github.com/microsoft/vscode/pull/334341)

- Published comments: **1**
- Head: `7e6d14157062b843b8a4ca0efc58f6b2157f306b`
- Completed: `2026-09-05T09:24:43Z`
- Run: [33957353041 attempt 1](runs/2026/09/05/33957353041-1/run.md)

### `src/vs/platform/agentHost/node/agentHostSessionLifecycle.ts:144`

**[Experimental performance review bot]**

**Severity: medium**

The new hourly lifecycle restores every due deletion candidate before checking whether its retained worktree makes automatic deletion impossible; rejected candidates remain eligible and repeat the sequence next hour.

Automatically archived sessions whose worktree cleanup was skipped or failed repeatedly consume GitHub API budget and provider/database I/O every hour despite being unable to make deletion progress.

**Suggested fix:** For delete candidates, run a cheap `canDeleteSession` preflight before the GitHub refresh and restore, while retaining the current in-disposal recheck for race safety.

(Written by Copilot)

## [#333196 — chat: add customization migration assessment telemetry](https://github.com/microsoft/vscode/pull/333196)

- Published comments: **2**
- Head: `37e5ee7b1a9747057e9f3d6b4797ce90f27ba6e9`
- Completed: `2026-09-05T09:24:43Z`
- Run: [33957353041 attempt 1](runs/2026/09/05/33957353041-1/run.md)

### `src/vs/sessions/services/sessions/browser/sessionsService.ts:789`

**[Experimental performance review bot]**

**Severity: medium**

The added fire-and-forget call invokes `reportCustomizationMigrationTelemetry` after every `_openChat`, including opens of side chats and subagent chats in a session that is already active.

Opening or switching among chats in an already-open Agents session calls `_openChat` each time, so normal tab navigation repeatedly launches the complete customization assessment even though the session lifecycle has not changed.

**Suggested fix:** Move this report to the actual session-entry transition (or guard/deduplicate it by session resource for the current open lifecycle) and leave chat-only navigation free of migration recomputation.

(Written by Copilot)

### `src/vs/sessions/services/sessions/browser/sessionsService.ts:1614`

**[Experimental performance review bot]**

**Severity: medium**

The new loop fire-and-forgets one `_reportCustomizationMigrationTelemetry` call for every already-resolved restored session without a limiter, idle deferral, or grouping by equivalent assessment scope.

Reloading an Agents window with several persisted visible sessions immediately launches a full customization assessment for every restored session just as the workbench is restoring. Different workspace roots prevent MCP-scope coalescing, so a realistic multi-session grid fans out concurrent work.

**Suggested fix:** Queue these reports with bounded concurrency or defer them to idle, and coalesce assessments that share the same session type/workspace scope while preserving the required per-session telemetry semantics.

(Written by Copilot)

## [#334594 — chat: fix GitHub context repository selection](https://github.com/microsoft/vscode/pull/334594)

- Published comments: **1**
- Head: `9501b080fe0786fad7aae9020d87f5e472e22148`
- Completed: `2026-09-04T23:36:18Z`
- Run: [33928652298 attempt 1](runs/2026/09/04/33928652298-1/run.md)

### `extensions/copilot/src/platform/git/vscode-node/gitServiceImpl.ts:201`

**[Experimental performance review bot]**

**Severity: medium**

Every file-scheme URI now performs and awaits workspace.fs.stat before joining discovery, even when the URI is a file and therefore cannot take the new direct .git/config branch.

While repository discovery is pending, content exclusion calls this API for each file and does not cache unsettled verdicts; collection-based prompt/search/review paths can therefore launch a burst of one extra stat per file on their critical path. Warm repository-root and verdict caches reduce later calls, but do not protect this startup window.

**Suggested fix:** Restrict the direct .git/config fast path to callers that know they are passing a selected repository/workspace root (for example via a root-specific method or option), leaving generic file-based getRepositoryFetchUrls calls on the Git API/discovery path without this preflight stat.

(Written by Copilot)

## [#334591 — sessions: redesign unified workspace picker](https://github.com/microsoft/vscode/pull/334591)

- Published comments: **1**
- Head: `654e64b0d74b497aa99a56aaaa11657fe92ffa14`
- Completed: `2026-09-04T23:36:18Z`
- Run: [33928652298 attempt 1](runs/2026/09/04/33928652298-1/run.md)

### `src/vs/sessions/contrib/chat/browser/sessionWorkspacePicker.ts:1416`

**[Experimental performance review bot]**

**Severity: low**

With tabs removed, `activeGroup` is normally undefined, and this added condition includes every remote provider while constructing the unified picker. The subsequent loop calls `getRemoteHostStatusDescription`, which calls `provider.getSessions()` and filters every session to compute active counts before the Remote submenu is opened.

Opening the workspace selector for any new session can pause longer as remote session history grows, even if the user selects Open Folder or a GitHub action and never opens Remote.

**Suggested fix:** Gate remote-provider status/count construction behind opening the Remote flyout (or otherwise compute those rows lazily), while keeping only the cheap top-level Remote action in the initial unified list.

(Written by Copilot)

## [#334521 — automations: refactor: make provider session templates canonical](https://github.com/microsoft/vscode/pull/334521)

- Published comments: **1**
- Head: `e20b2e8b093a1abe2b282225d49140e1f268f5f4`
- Completed: `2026-09-04T23:36:18Z`
- Run: [33928652298 attempt 1](runs/2026/09/04/33928652298-1/run.md)

### `src/vs/sessions/contrib/providers/copilotChatSessions/browser/copilotChatSessionsProvider.ts:1919`

**[Experimental performance review bot]**

**Severity: medium**

Initial Automation configuration now launches `_resolveAutomationSessionMode` with `void`; that method owns a ChatModes instance but disposes it only after `waitForPendingUpdates()` settles, with no timeout or cancellation tied to deletion, replacement, send completion, or provider disposal.

If custom-agent discovery is hung or delayed by an unavailable/restarting extension host, each older-host/browser Automation run using an unresolved provider/custom mode leaves another ChatModes object, subscriptions, refresh token, and session closure alive. Recurring Automations then cause renderer memory and listener counts to grow for the lifetime of the stalled discovery.

**Suggested fix:** Register the resolver/ChatModes instance with the draft or provider lifecycle and race pending discovery with that cancellation (and a bounded timeout). Dispose it immediately when the draft is replaced, deleted, committed, or the provider shuts down; also coalesce discovery where multiple runs resolve the same session scope.

(Written by Copilot)

## [#334341 — Add opt-in auto-archive inactive sessions with merged pull requests](https://github.com/microsoft/vscode/pull/334341)

- Published comments: **3**
- Head: `597a753b6fa15d563544dcaacd743bc4b5520fd9`
- Completed: `2026-09-04T23:36:18Z`
- Run: [33928652298 attempt 1](runs/2026/09/04/33928652298-1/run.md)

### `src/vs/platform/agentHost/node/agentHostSessionLifecycle.ts:151`

**[Experimental performance review bot]**

**Severity: medium**

Every cleanup candidate is fully restored before the lifecycle service performs the authoritative core GitHub refresh that determines whether any archive/delete action is possible.

With cleanup enabled, old sessions whose pull requests are still open are cold-restored every lifecycle pass merely to discover that no action is needed. AgentService restoration activates provider metadata, materializes the default provider chat, calls provider.chats.getMessages for the whole transcript, hydrates contributions, and reads session databases. Restores touch residency and reconcile against a default limit of 10, so a catalogue with many candidates churns restored sessions; archived sessions are immediately release-eligible and can repeat this work each pass. Users can see periodic CPU/disk spikes and memory/GC pressure proportional to accumulated histories.

**Suggested fix:** Let the new lifecycle resolver accept the designated PR identity/branch from IAgentSessionMetadata and perform the core refresh without live session hydration; restore (and hold) the session only after a merged result, immediately before revalidation and the archive/delete action.

(Written by Copilot)

### `src/vs/platform/agentHost/node/agentHostSessionLifecycle.ts:64`

**[Experimental performance review bot]**

**Severity: medium**

The new listener schedules an immediate cleanup pass for every onDidRootConfigChange event, although that event is unkeyed and explicitly fires for every root-config key, not only the two lifecycle thresholds.

Once either cleanup threshold is enabled, changing settings such as proxy, sandbox, MCP, model, migration, or provider setup values can trigger a full catalogue pass and authoritative refreshes for all stale PR sessions. For users with many inactive sessions this converts a single unrelated setting update into N GitHub requests and session restores, consuming rate limit and causing avoidable background latency.

**Suggested fix:** Cache the last validated archive/delete threshold pair and schedule an immediate pass only when one of those two values actually changes; ignore root-config events whose effective pair is unchanged.

(Written by Copilot)

### `src/vs/platform/agentHost/node/agentHostSessionLifecycle.ts:117`

**[Experimental performance review bot]**

**Severity: medium**

The new scheduled lifecycle pass calls AgentService.listSessions before applying any cutoff or pull-request filtering, at startup, hourly, and after scheduled configuration/provider events.

Users who opt into cleanup and retain a large session history now pay a complete catalogue rebuild every hour even if there are no cleanup candidates. AgentService.\_computeSessions prewarms each involved provider, reads metadata for every registered session, then stats/opens every per-session database for overlays; its own comments identify those operations as the dominant cost on large catalogues. This can create recurring disk/provider activity and delay a concurrent user-triggered listing that joins the shared computation.

**Suggested fix:** Add a cutoff-aware lifecycle candidate enumeration that first filters registry entries by external/modified state and reads only the archived/provenance and GitHub metadata needed for survivors (folding provenance into the existing metadata batch where applicable), rather than invoking the presentation-oriented full listSessions computation on every tick.

(Written by Copilot)

## [#334022 — Center full-width characters in two monospace cells](https://github.com/microsoft/vscode/pull/334022)

- Published comments: **1**
- Head: `b9b8ede3afdb676101dccab70bb672660762b57f`
- Completed: `2026-09-04T21:42:03Z`
- Run: [33921409256 attempt 1](runs/2026/09/04/33921409256-1/run.md)

### `src/vs/editor/browser/view/domLineBreaksComputer.ts:280`

**[Experimental performance review bot]**

**Severity: high**

Advanced wrapping now materializes each full-width character as its own styled DOM element whenever the new two-cell option is active, instead of keeping ordinary text in large shared spans.

Opening, reconfiguring, resizing/re-wrapping, or flushing a large CJK-heavy editor with `fullwidthCharacterWidth: 'twoCells'` and advanced wrapping can block the UI while the browser parses, styles, lays out, and measures hundreds of thousands of temporary elements; peak DOM memory and GC pressure grow at the same time.

**Suggested fix:** Keep measurement semantics but bound the fan-out: process queued lines/characters in capped batches so only a limited temporary subtree is live at once, and where possible compute fixed two-cell runs outside the browser instead of emitting one measurement element per character.

(Written by Copilot)

## [#332410 — agentHost: centralize session and chat catalog metadata](https://github.com/microsoft/vscode/pull/332410)

- Published comments: **1**
- Head: `c607b06d6dbec2888b4ff59bd366acfd883919e3`
- Completed: `2026-09-04T21:42:03Z`
- Run: [33921409256 attempt 1](runs/2026/09/04/33921409256-1/run.md)

### `src/vs/platform/agentHost/node/agentService.ts:749`

**[Experimental performance review bot]**

**Severity: high**

The new unconditional callback sends every `onDidChangeSessionSummary` event to `_queueCatalogSync`. That method immediately creates and retains a distinct `_persistListVisibleSessionStateNow` promise; it does not merge an update into an existing pending write.

A tool-rich agent turn produces repeated summary status/activity/timestamp changes (the notifier can flush every 100 ms), and concurrent active sessions multiply them. Every change opens `session.db`, reads all catalog metadata, canonicalizes/hashes the full payload, writes the local pending snapshot and compatibility metadata, upserts `agent-host.db`, acknowledges the local snapshot, and marks the central row dirty. These writes are serialized per session and host-wide, so bursts can build a disk-I/O backlog and delay list/registry work or deletion, which explicitly waits for all background catalog writes.

**Suggested fix:** Keep at most one in-flight catalog write and one merged trailing update per session. Merge metadata overrides and rebuild the trailing projection from the latest state when the in-flight write settles; preserve the existing ordered/teardown flush paths by awaiting that coalesced drain.

(Written by Copilot)

## [#332410 — agentHost: centralize session and chat catalog metadata](https://github.com/microsoft/vscode/pull/332410)

- Published comments: **1**
- Head: `0f605424902269dc6acd45bcbfc679dbcb014126`
- Completed: `2026-09-04T17:05:45Z`
- Run: [33897628556 attempt 1](runs/2026/09/04/33897628556-1/run.md)

### `src/vs/platform/agentHost/node/agentHostPeerChatStore.ts:339`

**[Experimental performance review bot]**

**Severity: medium**

\_publishCompatibilityState now maps the complete current entry list to \_writeChatMetadata before writing the parent legacy mirror, and createChat/disposeChat synchronously await peerChatStore.upsert/remove.

In a long-lived session with many peer chats, adding or deleting one chat blocks on potentially hundreds of unrelated SQLite opens/writes (250 four-wide waves at the 1,000-chat limit). Repeated additions accumulate roughly N(N+1)/2 writes, causing increasingly slow chat operations and disk churn.

**Suggested fix:** Diff the previous and updated central catalogs and write chat-local metadata only for added or actually changed entries, then write the parent peerChats mirror once. Reserve full per-chat repair sweeps for migration/background reconciliation rather than the interactive mutation path.

(Written by Copilot)

## [#334022 — Center full-width characters in two monospace cells](https://github.com/microsoft/vscode/pull/334022)

- Published comments: **1**
- Head: `4c0552f4f08c492b3ca7e817d1ed9870b78f55e3`
- Completed: `2026-09-04T13:27:30Z`
- Run: [33877485041 attempt 1](runs/2026/09/04/33877485041-1/run.md)

### `src/vs/editor/common/viewModel/monospaceLineBreaksComputer.ts:46`

**[Experimental performance review bot]**

**Severity: medium**

With forceFullwidthCharacterWidth enabled, the added condition rejects prior projection data unconditionally and sends every wrapped line through createLineBreaks, which scans the complete line and allocates fresh break-offset arrays and a new ModelLineProjectionData.

In a large CJK/Markdown document with viewport word wrap and this new option enabled, dragging a panel or resizing the editor synchronously reconstructs all model lines on every wrapping-column update, causing avoidable UI stalls and GC pressure.

**Suggested fix:** Retain the previous-data branch and call createLineBreaksFromPreviousLineBreaks with `forceFullwidthCharacterWidth ? 2 : columnsForFullWidthChar`; add a resize/previousLineBreakData test with the option enabled to preserve the exact two-column semantics.

(Written by Copilot)

## [#334375 — Update Component Explorer packages](https://github.com/microsoft/vscode/pull/334375)

- Published comments: **1**
- Head: `57121e6a364d6c5faccf3ca8b1ae78ed23045d07`
- Completed: `2026-09-04T00:20:11Z`
- Run: [33820613710 attempt 1](runs/2026/09/04/33820613710-1/run.md)

### `package-lock.json:255`

**[Experimental performance review bot]**

**Severity: medium**

This lockfile refresh deletes every `libc` selector while retaining both GNU and musl packages as optional dependencies with identical Linux OS and CPU constraints, making both variants eligible in the lockfile's install tree.

A Linux x64 cache-miss `npm ci`—including the root install on component-fixtures and CSS-order-scan CI—downloads and unpacks both x64 variants for all three families before any build or fixture can start. Warm node\_modules-cache hits avoid the install, but fresh developer/CI installs pay the extra binary transfer and disk cost.

**Suggested fix:** Restore the deleted `libc` arrays (or regenerate the lockfile with the repository-supported npm version that preserves them) so only the host's GNU or musl artifact is fetched and unpacked.

(Written by Copilot)

## [#333859 — Surface extensions blocked by the marketplace](https://github.com/microsoft/vscode/pull/333859)

- Published comments: **1**
- Head: `65fb4147de857f64f743dbfbb3a3952d8cea8f56`
- Completed: `2026-09-03T20:07:09Z`
- Run: [33799327822 attempt 1](runs/2026/09/03/33799327822-1/run.md)

### `src/vs/platform/extensionManagement/common/extensionGalleryService.ts:1394`

**[Experimental performance review bot]**

**Severity: medium**

The added blocked bit makes getValidRawGalleryExtensionVersion reject a version-blocked latest resource result. getLatestGalleryExtension then returns NOT\_COMPATIBLE, getExtensionsUsingResourceApi retries the affected IDs through a latest-only query, and queryGalleryExtensions rejects the same versions again before issuing its all-versions query.

Periodic and manual extension update checks that encounter a version-blocked latest release wait for two serial Marketplace POSTs after the existing resource fetches. On high-latency links this can add multiple round-trip times before VS Code can determine the usable older version and schedule updates.

**Suggested fix:** Propagate a distinct latest-version-is-blocked fallback result and batch those IDs directly into an IncludeVersions query, skipping the intervening latest-only query while preserving selection of an older permitted version.

(Written by Copilot)

## [#334303 — Improve customization message UI and migration behavior](https://github.com/microsoft/vscode/pull/334303)

- Published comments: **1**
- Head: `81b114c55f544144a89ac8943b2640ce949d6b26`
- Completed: `2026-09-03T18:45:02Z`
- Run: [33791391499 attempt 1](runs/2026/09/03/33791391499-1/run.md)

### `src/vs/workbench/contrib/chat/browser/aiCustomization/customizationMigrationServiceImpl.ts:74`

**[Experimental performance review bot]**

**Severity: medium**

Summary mode now calls provideChatSessionCustomizations from computeMigrationHint. ChatService awaits that hint together with hooks/instructions before building and dispatching the agent request.

With Summary enabled and a cold cache—for example, the first chat after startup with several remote agent-host plugins—the user's first send waits for directory resolution/listing and per-file metadata reads across all plugins before the agent starts responding.

**Suggested fix:** Do not await full item-provider enumeration before dispatching the chat request. Reuse/coalesce an invalidatable provider-item snapshot that is warmed outside the send path, or publish the summary notification after agent dispatch; also pass the request cancellation token so abandoned sends stop outstanding scans.

(Written by Copilot)

## [#334255 — sessions: add Agents window layout telemetry](https://github.com/microsoft/vscode/pull/334255)

- Published comments: **1**
- Head: `c09239548ceacd7694e0bcc766bbba65a2784711`
- Completed: `2026-09-03T18:45:02Z`
- Run: [33791391499 attempt 1](runs/2026/09/03/33791391499-1/run.md)

### `src/vs/sessions/browser/workbench.ts:618`

**[Experimental performance review bot]**

**Severity: low**

The added line synchronously resolves the delayed ITelemetryService and logs an event immediately after Ready-phase contributions, before renderWorkbench, createWorkbenchLayout, layout, and restore.

Opening an Agents window, especially a cold web window with telemetry enabled, now begins telemetry initialization, a health-check request, and telemetry SDK loading before the UI is rendered. This adds synchronous startup work and lets telemetry traffic/module parsing contend with first-render work for a constant one-event benefit.

**Suggested fix:** Record the fixed layout from an AfterRestored/idle contribution (for example, the existing SessionsTelemetryContribution using IAgentWorkbenchLayoutService), or otherwise schedule this log after first render, so the event is preserved without forcing the delayed telemetry stack onto the startup critical path.

(Written by Copilot)

## [#332410 — agentHost: centralize session and chat catalog metadata](https://github.com/microsoft/vscode/pull/332410)

- Published comments: **1**
- Head: `d3fc54d2a1c84ab24af96fca972159e0b75a9b9b`
- Completed: `2026-09-03T17:30:02Z`
- Run: [33783823809 attempt 1](runs/2026/09/03/33783823809-1/run.md)

### `src/vs/platform/agentHost/node/chatContributions/sessionTitle/sessionTitleContribution.ts:77`

**[Experimental performance review bot]**

**Severity: high**

Every title hydration now stats and potentially opens/queries the chat-local SQLite database before accepting an already-restored title.

Opening a session with many restored peer chats now launches one filesystem probe and SQLite metadata read per chat concurrently, delaying restore and creating an I/O/open-handle burst; up to the catalog limit of 999 peers can be affected.

**Suggested fix:** Restore the cached-title fast path before chat-local database access, and import chat-local title changes through the catalog reconciliation path. If local verification must remain on restore, batch/limit it and only probe chats whose central title is missing or known stale.

(Written by Copilot)
