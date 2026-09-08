# Published comments: 2

## [#334341 — Add opt-in auto-archive/delete inactive sessions with merged pull requests](https://github.com/microsoft/vscode/pull/334341) at `src/vs/platform/agentHost/node/agentHostSessionLifecycle.ts:164`

**[Experimental performance review bot]**

**Severity: medium**

The new cleanup-only path first refreshes all related PRs at the top of \_evaluateCandidate, performs only local settings/summary checks, and then refreshes the same URLs authoritatively again immediately before cleanup.

With the default-on standalone worktree cleanup, Agent Host startup and every hourly pass issue two GitHub core reads per related PR for each merged/closed cleanup candidate. Users retaining many completed agent worktrees consume twice the required API quota and wait twice as long for the serialized cleanup sweep.

**Suggested fix:** Perform cleanup's local settings/summary revalidation first and make a single \_arePullRequestsComplete call immediately before cleanup; keep the pre-restore check only for archive/delete candidates where restoration creates a meaningful intervening boundary.

(Written by Copilot)

## [#334341 — Add opt-in auto-archive/delete inactive sessions with merged pull requests](https://github.com/microsoft/vscode/pull/334341) at `src/vs/platform/agentHost/node/agentService.ts:2129`

**[Experimental performance review bot]**

**Severity: medium**

The default-enabled cleanup pass enumerates all internal registered sessions. For an unloaded session `archived` is undefined, making this condition true unconditionally, so `tryOpenDatabase` runs before the code can reject sessions without related PR metadata or a worktree.

Users with large retained agent-session histories now get a catalogue-wide burst of filesystem and SQLite work after provider startup and on every hourly pass, including the common majority of sessions that have no lifecycle-relevant PR/worktree. This can contend with normal Agent Host startup and creates ongoing disk/CPU overhead.

**Suggested fix:** Keep the minimal lifecycle eligibility facts in the existing durable session registry (or an equivalent invalidated candidate index) so the pass can select sessions with related PR/worktree metadata before opening per-session databases; retain the current per-session read as final authoritative validation only for selected candidates.

(Written by Copilot)

