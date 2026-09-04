# Reviewer run 33928652298

- Status: **succeeded**
- Started: `2026-09-04T23:14:03Z`
- Completed: `2026-09-04T23:36:18Z`
- Reviewer commit: `38a5c3528405796e5d5882a3c37571199b7181e0`
- Bootstrapped: 0
- Reviewed: 4
- Published: 4
- Deferred: 350
- Skipped: 394
- Regression cases: 1
- Unresolved regression fixes: 0

## Reviewed PRs

### [#334341 — Add opt-in auto-archive inactive sessions with merged pull requests](https://github.com/microsoft/vscode/pull/334341)

- Head: `597a753b6fa15d563544dcaacd743bc4b5520fd9`
- Findings: 3
- Published: true
- Reports: [pr-334341-597a753b6fa1.json](pr-334341-597a753b6fa1.json), [pr-334341-597a753b6fa1.md](pr-334341-597a753b6fa1.md)

#### Published comment at `src/vs/platform/agentHost/node/agentHostSessionLifecycle.ts:151`

**[Experimental performance review bot]**

**Severity: medium**

Every cleanup candidate is fully restored before the lifecycle service performs the authoritative core GitHub refresh that determines whether any archive/delete action is possible.

With cleanup enabled, old sessions whose pull requests are still open are cold-restored every lifecycle pass merely to discover that no action is needed. AgentService restoration activates provider metadata, materializes the default provider chat, calls provider.chats.getMessages for the whole transcript, hydrates contributions, and reads session databases. Restores touch residency and reconcile against a default limit of 10, so a catalogue with many candidates churns restored sessions; archived sessions are immediately release-eligible and can repeat this work each pass. Users can see periodic CPU/disk spikes and memory/GC pressure proportional to accumulated histories.

**Suggested fix:** Let the new lifecycle resolver accept the designated PR identity/branch from IAgentSessionMetadata and perform the core refresh without live session hydration; restore (and hold) the session only after a merged result, immediately before revalidation and the archive/delete action.

(Written by Copilot)

#### Published comment at `src/vs/platform/agentHost/node/agentHostSessionLifecycle.ts:64`

**[Experimental performance review bot]**

**Severity: medium**

The new listener schedules an immediate cleanup pass for every onDidRootConfigChange event, although that event is unkeyed and explicitly fires for every root-config key, not only the two lifecycle thresholds.

Once either cleanup threshold is enabled, changing settings such as proxy, sandbox, MCP, model, migration, or provider setup values can trigger a full catalogue pass and authoritative refreshes for all stale PR sessions. For users with many inactive sessions this converts a single unrelated setting update into N GitHub requests and session restores, consuming rate limit and causing avoidable background latency.

**Suggested fix:** Cache the last validated archive/delete threshold pair and schedule an immediate pass only when one of those two values actually changes; ignore root-config events whose effective pair is unchanged.

(Written by Copilot)

#### Published comment at `src/vs/platform/agentHost/node/agentHostSessionLifecycle.ts:117`

**[Experimental performance review bot]**

**Severity: medium**

The new scheduled lifecycle pass calls AgentService.listSessions before applying any cutoff or pull-request filtering, at startup, hourly, and after scheduled configuration/provider events.

Users who opt into cleanup and retain a large session history now pay a complete catalogue rebuild every hour even if there are no cleanup candidates. AgentService.\_computeSessions prewarms each involved provider, reads metadata for every registered session, then stats/opens every per-session database for overlays; its own comments identify those operations as the dominant cost on large catalogues. This can create recurring disk/provider activity and delay a concurrent user-triggered listing that joins the shared computation.

**Suggested fix:** Add a cutoff-aware lifecycle candidate enumeration that first filters registry entries by external/modified state and reads only the archived/provenance and GitHub metadata needed for survivors (folding provenance into the existing metadata batch where applicable), rather than invoking the presentation-oriented full listSessions computation on every tick.

(Written by Copilot)

### [#334521 — automations: refactor: make provider session templates canonical](https://github.com/microsoft/vscode/pull/334521)

- Head: `e20b2e8b093a1abe2b282225d49140e1f268f5f4`
- Findings: 1
- Published: true
- Reports: [pr-334521-e20b2e8b093a.json](pr-334521-e20b2e8b093a.json), [pr-334521-e20b2e8b093a.md](pr-334521-e20b2e8b093a.md)

#### Published comment at `src/vs/sessions/contrib/providers/copilotChatSessions/browser/copilotChatSessionsProvider.ts:1919`

**[Experimental performance review bot]**

**Severity: medium**

Initial Automation configuration now launches `_resolveAutomationSessionMode` with `void`; that method owns a ChatModes instance but disposes it only after `waitForPendingUpdates()` settles, with no timeout or cancellation tied to deletion, replacement, send completion, or provider disposal.

If custom-agent discovery is hung or delayed by an unavailable/restarting extension host, each older-host/browser Automation run using an unresolved provider/custom mode leaves another ChatModes object, subscriptions, refresh token, and session closure alive. Recurring Automations then cause renderer memory and listener counts to grow for the lifetime of the stalled discovery.

**Suggested fix:** Register the resolver/ChatModes instance with the draft or provider lifecycle and race pending discovery with that cancellation (and a bounded timeout). Dispose it immediately when the draft is replaced, deleted, committed, or the provider shuts down; also coalesce discovery where multiple runs resolve the same session scope.

(Written by Copilot)

### [#334591 — sessions: redesign unified workspace picker](https://github.com/microsoft/vscode/pull/334591)

- Head: `654e64b0d74b497aa99a56aaaa11657fe92ffa14`
- Findings: 1
- Published: true
- Reports: [pr-334591-654e64b0d74b.json](pr-334591-654e64b0d74b.json), [pr-334591-654e64b0d74b.md](pr-334591-654e64b0d74b.md)

#### Published comment at `src/vs/sessions/contrib/chat/browser/sessionWorkspacePicker.ts:1416`

**[Experimental performance review bot]**

**Severity: low**

With tabs removed, `activeGroup` is normally undefined, and this added condition includes every remote provider while constructing the unified picker. The subsequent loop calls `getRemoteHostStatusDescription`, which calls `provider.getSessions()` and filters every session to compute active counts before the Remote submenu is opened.

Opening the workspace selector for any new session can pause longer as remote session history grows, even if the user selects Open Folder or a GitHub action and never opens Remote.

**Suggested fix:** Gate remote-provider status/count construction behind opening the Remote flyout (or otherwise compute those rows lazily), while keeping only the cheap top-level Remote action in the initial unified list.

(Written by Copilot)

### [#334594 — chat: fix GitHub context repository selection](https://github.com/microsoft/vscode/pull/334594)

- Head: `9501b080fe0786fad7aae9020d87f5e472e22148`
- Findings: 1
- Published: true
- Reports: [pr-334594-9501b080fe07.json](pr-334594-9501b080fe07.json), [pr-334594-9501b080fe07.md](pr-334594-9501b080fe07.md)

#### Published comment at `extensions/copilot/src/platform/git/vscode-node/gitServiceImpl.ts:201`

**[Experimental performance review bot]**

**Severity: medium**

Every file-scheme URI now performs and awaits workspace.fs.stat before joining discovery, even when the URI is a file and therefore cannot take the new direct .git/config branch.

While repository discovery is pending, content exclusion calls this API for each file and does not cache unsettled verdicts; collection-based prompt/search/review paths can therefore launch a burst of one extra stat per file on their critical path. Warm repository-root and verdict caches reduce later calls, but do not protect this startup window.

**Suggested fix:** Restrict the direct .git/config fast path to callers that know they are passing a selected repository/workspace root (for example via a root-specific method or option), leaving generic file-based getRepositoryFetchUrls calls on the Git API/discovery path without this preflight stat.

(Written by Copilot)
