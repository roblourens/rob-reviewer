# Reviewer run 34243720924

- Status: **succeeded**
- Started: `2026-09-08T15:17:43Z`
- Completed: `2026-09-08T15:37:07Z`
- Reviewer commit: `22d2f3ec72332773d6fe6490753140d4f3d9418c`
- Bootstrapped: 0
- Reviewed: 2
- Published: 1
- Deferred: 354
- Skipped: 391
- Regression cases: 1
- Unresolved regression fixes: 0

## Reviewed PRs

### [#330955 — Add transient UI for /btw side questions](https://github.com/microsoft/vscode/pull/330955)

- Head: `f48bd0e5704b38491d1995553b652aeb10ed8f8d`
- Findings: 0
- Published: false
- Reports: [pr-330955-f48bd0e5704b.json](pr-330955-f48bd0e5704b.json), [pr-330955-f48bd0e5704b.md](pr-330955-f48bd0e5704b.md)

### [#332410 — agentHost: centralize session and chat catalog metadata](https://github.com/microsoft/vscode/pull/332410)

- Head: `224066406daa8e1bbd66c00e82b012c742c952a7`
- Findings: 1
- Published: true
- Reports: [pr-332410-224066406daa.json](pr-332410-224066406daa.json), [pr-332410-224066406daa.md](pr-332410-224066406daa.md)

#### Published comment at `src/vs/platform/agentHost/node/agentService.ts:776`

**[Experimental performance review bot]**

**Severity: medium**

Every emitted summary delta now queues `_persistListVisibleSessionStateNow`. That opens the session database, reads all catalog metadata and per-chat title keys, validates/stable-serializes/hashes the full payload, writes the local catalog snapshot, reads/upserts/acknowledges `sessions_v2`, and marks the central payload dirty before and after. This also runs for `activity` changes and transient status-bit changes, although `AgentHostCatalogData` stores no activity and the resolver only projects the IsRead/IsArchived status bits.

During worktree setup, tool/status progress, or multiple active automation sessions, activity/status summary updates can repeatedly rewrite or replay unchanged catalog state. This adds avoidable disk traffic and globally serialized host-database work, delaying list and persistence operations even though the list-visible payload is unchanged.

**Suggested fix:** Gate the queue on fields that can alter `AgentHostCatalogData`; specifically ignore activity-only deltas and transient status changes when the IsRead/IsArchived projection is unchanged. A cached projected signature or comparison of the projected status bits can preserve required title, recency, project, changes, working-directory, metadata, read, and archive synchronization while avoiding no-op full writes.

(Written by Copilot)
