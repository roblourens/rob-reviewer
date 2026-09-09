# Published comments: 6

## [#335259 — agentHost: change default settings for Codex harness](https://github.com/microsoft/vscode/pull/335259) at `src/vs/platform/agentHost/common/agentHostStarter.config.contribution.ts:295`

**[Experimental performance review bot]**

**Severity: medium**

The new non-Stable default is forwarded by the starter, causes agentHostMain to instantiate CodexAgent during agent-host prewarm, and the constructor immediately queues \_probeAccountAtStartup.

On an Insiders startup where AI features are enabled and the Codex SDK is already cached, BlockRestore prewarming now launches a native Codex process even if the user never selects Codex. \_probeAccountAtStartup resolves the SDK, creates proxy/temp resources, spawns the binary, performs the initialize/account handshake (and signed-in account metadata requests), then tears it down, potentially contending with editor restoration for up to the 30-second timeout.

**Suggested fix:** Keep the provider/routing defaults if desired, but make registration passive: do not run \_probeAccountAtStartup merely because the provider is enabled. Trigger the one-off account probe lazily when a Codex account surface or Codex session is first requested, and coalesce that first-use probe with persistent connection startup.

(Written by Copilot)

## [#335303 — sessions: add Dev Container workspace menu option](https://github.com/microsoft/vscode/pull/335303) at `src/vs/sessions/contrib/chat/browser/sessionWorkspacePicker.ts:1896`

**[Experimental performance review bot]**

**Severity: medium**

Each successful availability promise now immediately force-reopens the visible picker instead of batching results or updating the affected list row.

Users with several Dev Container projects in recent workspaces can incur a burst of full list allocations, DOM teardown/recreation, focus work, and layout just after opening the menu, producing visible jank and unnecessary CPU/GC pressure.

**Suggested fix:** Aggregate or debounce availability completions and refresh the visible list once, preferably using updateItems/refreshActiveList (or patching the resolved row) rather than hiding and recreating the popup.

(Written by Copilot)

## [#335303 — sessions: add Dev Container workspace menu option](https://github.com/microsoft/vscode/pull/335303) at `src/vs/sessions/contrib/chat/browser/sessionWorkspacePicker.ts:1470`

**[Experimental performance review bot]**

**Severity: medium**

The added call inside the recent-workspace loop probes Dev Container availability for every local row, and showPicker clears the probe cache whenever the popup is freshly opened. Selecting a row then still creates a draft whose existing \_resolveDevContainerAvailability path probes the selected workspace again.

Opening the picker with a populated local history now launches up to 40 concurrent filesystem stats plus IPC roundtrips; slow disks or file-scheme network folders can generate an I/O burst on a routine UI interaction, and the selected folder is immediately re-statted during draft creation.

**Suggested fix:** Keep availability results across picker opens and invalidate them from bounded provider/configuration/file-change signals, or defer the check to the row/submenu the user interacts with. Also share/coalesce the selected workspace's result with draft creation so the required final validation does not duplicate the same stats.

(Written by Copilot)

## [#335329 — chore: bump foundry-local-sdk to 2.x](https://github.com/microsoft/vscode/pull/335329) at `src/vs/platform/localTranscription/node/localTranscriptionService.ts:563`

**[Experimental performance review bot]**

**Severity: high**

The added SDK 2.x call invokes `model.download` with only the progress callback, while `cancel()` still merely cancels `_modelPrepareCts`; that token is checked only after the download resolves and can no longer stop the active model transfer.

If a user cancels first-use dictation while the model is downloading, cancellation returns but the utility process continues fetching and writing the large model until completion or network failure.

**Suggested fix:** Restore an abortable SDK 2.x download path and wire `_modelPrepareCts` to it; if the new API offers no per-download signal, cancel by tearing down/recreating the downloader or utility worker and await that cleanup rather than leaving the transfer detached.

(Written by Copilot)

## [#335329 — chore: bump foundry-local-sdk to 2.x](https://github.com/microsoft/vscode/pull/335329) at `build/gulpfile.vscode.ts:263`

**[Experimental performance review bot]**

**Severity: medium**

The added `prebuilds/**/foundry_local_node.node` include uses an unrestricted target wildcard, and the adjacent preload include does the same, so `packageTask(platform, arch, ...)` retains both addons from every SDK prebuild directory instead of just the product's loadable target.

Each packaged desktop build carries up to ten native addon binaries although its support gate can use only two. Every installation and update transfers and stores the other targets' binaries.

**Suggested fix:** Parameterize the filter with the package platform/arch and re-include only the two addon files for the single runtime target that build can load (mapping the universal macOS build to darwin-arm64).

(Written by Copilot)

## [#335329 — chore: bump foundry-local-sdk to 2.x](https://github.com/microsoft/vscode/pull/335329) at `build/dictation-runtime/package.ts:132`

**[Experimental performance review bot]**

**Severity: low**

The product now deliberately bundles both SDK addons and configures `libraryPath` only for shared libraries, but `requiredSdkNativeFileNames` newly forces both `.node` files into the CDN tarball and runtime validation requires them in the cache as well.

On the first dictation after each runtime-version change, users must download and extract cached copies of both addons even though the SDK loads the packaged copies.

**Suggested fix:** Build and validate the CDN payload from only the shared libraries used through `libraryPath`; omit the two `.node` files from staging and from runtime cache completeness checks.

(Written by Copilot)

