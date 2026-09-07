# Reviewer run 34101442004

- Status: **succeeded**
- Started: `2026-09-07T08:36:47Z`
- Completed: `2026-09-07T08:43:26Z`
- Reviewer commit: `add9ed06110e74a6f47e8d38c1cb1acbbe1e60ab`
- Bootstrapped: 0
- Reviewed: 1
- Published: 1
- Deferred: 346
- Skipped: 407
- Regression cases: 0
- Unresolved regression fixes: 0

## Reviewed PRs

### [#334833 — ci: Parallelize integration and smoke test shards](https://github.com/microsoft/vscode/pull/334833)

- Head: `fcf9b6955c85eecfa69338e8492f3d51d84aa06c`
- Findings: 2
- Published: true
- Reports: [pr-334833-fcf9b6955c85.json](pr-334833-fcf9b6955c85.json), [pr-334833-fcf9b6955c85.md](pr-334833-fcf9b6955c85.md)

#### Published comment at `.github/workflows/pr.yml:153`

**[Experimental performance review bot]**

**Severity: medium**

The integration jobs now always skip Agent Host E2E, while each affected platform starts a separate full reusable workflow whose only enabled test is Agent Host E2E, repeating all unconditional preparation solely to move the suite beside the integration job.

A realistic platform or build change now causes three extra full repository preparations before the same three platform suites run. Parallel execution can shorten that PR's longest check, but the duplicated CPU, cache traffic, and Electron downloads consume additional shared CI capacity and reduce throughput for concurrent PRs.

**Suggested fix:** Keep Agent Host E2E parallel, but fan the integration and E2E test jobs out from one prepared build artifact per platform, or otherwise make the dedicated E2E workflow consume existing compiled outputs instead of rerunning the full platform bootstrap.

(Written by Copilot)

#### Published comment at `.github/workflows/pr.yml:174`

**[Experimental performance review bot]**

**Severity: medium**

The added chat job invokes the same reusable Linux workflow as an independent job; analogous added macOS and Windows chat jobs and the macOS proxy job each independently repeat all unconditional preparation before running only their shard.

On every PR, smoke coverage now launches two complete prepared environments per platform and three on macOS instead of one, even though each added environment runs only a complementary subset. This trades test wall-clock latency for a fixed four-job burst of duplicated CPU and I/O, which consumes shared/self-hosted capacity and lowers repository-wide CI throughput under realistic concurrent PR traffic.

**Suggested fix:** Preserve the shards but prepare the smoke build once per platform and have core/chat/proxy jobs consume that prepared artifact; if a shared artifact is not viable, keep the suites in one workflow until shard-specific jobs can skip the unrelated checkout/cache/build work.

(Written by Copilot)
