# Published comments: 2

## [#334369 — Add MCP customization migration](https://github.com/microsoft/vscode/pull/334369) at `src/vs/workbench/contrib/chat/browser/aiCustomization/mcpServerCustomizationMigration.ts:243`

**[Experimental performance review bot]**

**Severity: medium**

The new execution loop calls findCrossRootConflicts once for each source/target group, while findCrossRootConflicts sequentially reads both MCP configuration files in every other root; migrateGroup repeats that full cross-root scan twice more for revalidation.

Migrating eligible servers spread across a multi-root workspace, especially through a remote filesystem provider, blocks completion on hundreds or thousands of sequential file operations; e.g. 20 roots produce roughly 2,280 cross-root reads.

**Suggested fix:** Snapshot each root's source and target once per required validation phase, build a name-to-root conflict index, and reuse it for all groups; preserve optimistic concurrency by revalidating each file once at the phase boundary (or only affected names/files), rather than rescanning every root for every group.

(Written by Copilot)

## [#334369 — Add MCP customization migration](https://github.com/microsoft/vscode/pull/334369) at `src/vs/workbench/contrib/chat/browser/aiCustomization/aiCustomizationManagementEditor.ts:1175`

**[Experimental performance review bot]**

**Severity: medium**

The PR adds an IMcpWorkbenchService onChange listener and a separate autorun over the same IMcpService server list and enablement observables. Both immediately call refreshCustomizationMigrationInfo, which only uses a sequence to discard stale results and does not cancel or coalesce the work already started.

While the Customizations editor is open, toggling/installing/updating an MCP server can launch duplicate full refreshes. In an enabled multi-root migration setup this doubles per-root configuration reads; even with MCP migration disabled by default, it unnecessarily reruns the default-enabled prompt migration and loading render for unrelated MCP changes.

**Suggested fix:** Route all MCP signals through one RunOnceScheduler/delayer (or subscribe to one authoritative source), gate it on the MCP migration setting, and cancel/suppress superseded refresh computations so one logical MCP change produces one assessment, one per-root planning pass, and one UI update.

(Written by Copilot)

