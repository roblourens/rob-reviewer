# Reviewer run 34443076352

- Status: **succeeded**
- Started: `2026-09-10T05:57:10Z`
- Completed: `2026-09-10T06:10:06Z`
- Reviewer commit: `88002dfb88d8ae1a3ca3d405307ccd163af1eae3`
- Bootstrapped: 0
- Reviewed: 5
- Published: 2
- Deferred: 337
- Skipped: 42
- Regression cases: 0
- Unresolved regression fixes: 0

## Reviewed PRs

### [#331983 — agentHost: measure time to first edit](https://github.com/microsoft/vscode/pull/331983)

- Head: `9c913bd177c4aa377c9ff1f8601cc3227974cc9f`
- Findings: 1
- Published: true
- Reports: [pr-331983-9c913bd177c4.json](pr-331983-9c913bd177c4.json), [pr-331983-9c913bd177c4.md](pr-331983-9c913bd177c4.md)

#### Published comment at `src/vs/platform/agentHost/node/copilot/copilotAgentSession.ts:6587`

**[Experimental performance review bot]**

**Severity: low**

Every root `assistant.turn_start` with an interaction ID now retains the interaction-to-host-turn pair, and distinct SDK turn IDs are retained as well, even after the protocol turn completes.

A long-lived Copilot chat accumulates unreachable historical correlation keys and host-turn UUID values, increasing agent-host heap retention and eventual GC pressure until the chat/session is released.

**Suggested fix:** Expire correlations when their host protocol turn completes, aborts, or is discarded. Maintain reverse keys per host turn (or a bounded active/recent correlation set if late-event rejection requires it) so cleanup is O(entries for that turn) rather than scanning the full maps.

(Written by Copilot)

### [#335384 — sessions: keep new chat pills opaque over backgrounds](https://github.com/microsoft/vscode/pull/335384)

- Head: `9c0030608f7b037104ba718e59a6640d5aeb8229`
- Findings: 0
- Published: false
- Reports: [pr-335384-9c0030608f7b.json](pr-335384-9c0030608f7b.json), [pr-335384-9c0030608f7b.md](pr-335384-9c0030608f7b.md)

### [#335385 — Auto mode: support server-owned Hydra-RL multi-turn routing](https://github.com/microsoft/vscode/pull/335385)

- Head: `9e4416e05b2472ad298b4ac5f8e8fcea9601f23b`
- Findings: 0
- Published: false
- Reports: [pr-335385-9e4416e05b24.json](pr-335385-9e4416e05b24.json), [pr-335385-9e4416e05b24.md](pr-335385-9e4416e05b24.md)

### [#335390 — mcp: Await workspace .mcp.json discovery before autostart](https://github.com/microsoft/vscode/pull/335390)

- Head: `8996c34a2c020caab36bb6c1ea7a1d5f22ec6fbb`
- Findings: 0
- Published: false
- Reports: [pr-335390-8996c34a2c02.json](pr-335390-8996c34a2c02.json), [pr-335390-8996c34a2c02.md](pr-335390-8996c34a2c02.md)

### [#335391 — chat: expand customization migration assessment telemetry](https://github.com/microsoft/vscode/pull/335391)

- Head: `82d60ae60a881e4eeec9277ef438089f7382600f`
- Findings: 1
- Published: true
- Reports: [pr-335391-82d60ae60a88.json](pr-335391-82d60ae60a88.json), [pr-335391-82d60ae60a88.md](pr-335391-82d60ae60a88.md)

#### Published comment at `src/vs/workbench/contrib/chat/browser/aiCustomization/customizationMigrationServiceImpl.ts:224`

**[Experimental performance review bot]**

**Severity: high**

The added `Promise.all` invokes `listPromptFiles` for all five customization types regardless of which migration categories are enabled, then derives source-folder target types from the entire inventory. This performs discovery solely for telemetry assessment and can call `provideSourceFolders` for otherwise-disabled agents/instructions/skills/hooks.

With migration hints enabled by the experiment and the default migration-category settings (prompt migration enabled, the other file migrations disabled), a user who has native agent/instruction/skill/hook customizations but no prompt migration candidate now pays all-category filesystem discovery and may wait up to 2 seconds for remote session state before their message is sent. In `always` mode this occurs every turn; in `once` mode it also repeats while no hint is returned.

**Suggested fix:** Preserve the full assessment telemetry, but collect/cache it outside chat request dispatch (for example once per target/inventory generation after the request is sent), or at minimum keep hint computation enablement-gated and schedule the disabled-category assessment separately with invalidation on customization changes.

(Written by Copilot)
