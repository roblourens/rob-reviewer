# Reviewer run 34132873477

- Status: **succeeded**
- Started: `2026-09-07T14:26:21Z`
- Completed: `2026-09-07T14:40:43Z`
- Reviewer commit: `add9ed06110e74a6f47e8d38c1cb1acbbe1e60ab`
- Bootstrapped: 0
- Reviewed: 4
- Published: 1
- Deferred: 356
- Skipped: 406
- Regression cases: 0
- Unresolved regression fixes: 0

## Reviewed PRs

### [#334369 — Add MCP customization migration](https://github.com/microsoft/vscode/pull/334369)

- Head: `7f2ff6b9918273ec374046e7ee8e359e2861ff62`
- Findings: 2
- Published: true
- Reports: [pr-334369-7f2ff6b99182.json](pr-334369-7f2ff6b99182.json), [pr-334369-7f2ff6b99182.md](pr-334369-7f2ff6b99182.md)

#### Published comment at `src/vs/workbench/contrib/chat/browser/aiCustomization/mcpServerCustomizationMigration.ts:243`

**[Experimental performance review bot]**

**Severity: medium**

The new execution loop calls findCrossRootConflicts once for each source/target group, while findCrossRootConflicts sequentially reads both MCP configuration files in every other root; migrateGroup repeats that full cross-root scan twice more for revalidation.

Migrating eligible servers spread across a multi-root workspace, especially through a remote filesystem provider, blocks completion on hundreds or thousands of sequential file operations; e.g. 20 roots produce roughly 2,280 cross-root reads.

**Suggested fix:** Snapshot each root's source and target once per required validation phase, build a name-to-root conflict index, and reuse it for all groups; preserve optimistic concurrency by revalidating each file once at the phase boundary (or only affected names/files), rather than rescanning every root for every group.

(Written by Copilot)

#### Published comment at `src/vs/workbench/contrib/chat/browser/aiCustomization/aiCustomizationManagementEditor.ts:1175`

**[Experimental performance review bot]**

**Severity: medium**

The PR adds an IMcpWorkbenchService onChange listener and a separate autorun over the same IMcpService server list and enablement observables. Both immediately call refreshCustomizationMigrationInfo, which only uses a sequence to discard stale results and does not cancel or coalesce the work already started.

While the Customizations editor is open, toggling/installing/updating an MCP server can launch duplicate full refreshes. In an enabled multi-root migration setup this doubles per-root configuration reads; even with MCP migration disabled by default, it unnecessarily reruns the default-enabled prompt migration and loading render for unrelated MCP changes.

**Suggested fix:** Route all MCP signals through one RunOnceScheduler/delayer (or subscribe to one authoritative source), gate it on the MCP migration setting, and cancel/suppress superseded refresh computations so one logical MCP change produces one assessment, one per-root planning pass, and one UI update.

(Written by Copilot)

### [#334705 — Make action widget menu items directly clickable by Playwright](https://github.com/microsoft/vscode/pull/334705)

- Head: `4e63cce5441c8dc4809813a572a4bad3c7985147`
- Findings: 0
- Published: false
- Reports: [pr-334705-4e63cce5441c.json](pr-334705-4e63cce5441c.json), [pr-334705-4e63cce5441c.md](pr-334705-4e63cce5441c.md)

### [#334849 — agentHost: Support SSH proxy configuration](https://github.com/microsoft/vscode/pull/334849)

- Head: `8de3d6f45dd6f2a0635eb4efdddb7fd96720e9bd`
- Findings: 0
- Published: false
- Reports: [pr-334849-8de3d6f45dd6.json](pr-334849-8de3d6f45dd6.json), [pr-334849-8de3d6f45dd6.md](pr-334849-8de3d6f45dd6.md)

### [#334865 — automations: feat: backport templates and target selection to 1.137](https://github.com/microsoft/vscode/pull/334865)

- Head: `4f81ea73a4eb0e03c2eb20e9eed5cd64bc9e04d7`
- Findings: 0
- Published: false
- Reports: [pr-334865-4f81ea73a4eb.json](pr-334865-4f81ea73a4eb.json), [pr-334865-4f81ea73a4eb.md](pr-334865-4f81ea73a4eb.md)
