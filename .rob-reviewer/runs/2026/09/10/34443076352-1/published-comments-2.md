# Published comments: 2

## [#331983 — agentHost: measure time to first edit](https://github.com/microsoft/vscode/pull/331983) at `src/vs/platform/agentHost/node/copilot/copilotAgentSession.ts:6587`

**[Experimental performance review bot]**

**Severity: low**

Every root `assistant.turn_start` with an interaction ID now retains the interaction-to-host-turn pair, and distinct SDK turn IDs are retained as well, even after the protocol turn completes.

A long-lived Copilot chat accumulates unreachable historical correlation keys and host-turn UUID values, increasing agent-host heap retention and eventual GC pressure until the chat/session is released.

**Suggested fix:** Expire correlations when their host protocol turn completes, aborts, or is discarded. Maintain reverse keys per host turn (or a bounded active/recent correlation set if late-event rejection requires it) so cleanup is O(entries for that turn) rather than scanning the full maps.

(Written by Copilot)

## [#335391 — chat: expand customization migration assessment telemetry](https://github.com/microsoft/vscode/pull/335391) at `src/vs/workbench/contrib/chat/browser/aiCustomization/customizationMigrationServiceImpl.ts:224`

**[Experimental performance review bot]**

**Severity: high**

The added `Promise.all` invokes `listPromptFiles` for all five customization types regardless of which migration categories are enabled, then derives source-folder target types from the entire inventory. This performs discovery solely for telemetry assessment and can call `provideSourceFolders` for otherwise-disabled agents/instructions/skills/hooks.

With migration hints enabled by the experiment and the default migration-category settings (prompt migration enabled, the other file migrations disabled), a user who has native agent/instruction/skill/hook customizations but no prompt migration candidate now pays all-category filesystem discovery and may wait up to 2 seconds for remote session state before their message is sent. In `always` mode this occurs every turn; in `once` mode it also repeats while no hint is returned.

**Suggested fix:** Preserve the full assessment telemetry, but collect/cache it outside chat request dispatch (for example once per target/inventory generation after the request is sent), or at minimum keep hint computation enablement-gated and schedule the disabled-category assessment separately with invalidation on customization changes.

(Written by Copilot)

