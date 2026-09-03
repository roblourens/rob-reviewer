# Reviewer run 33799327822

- Status: **succeeded**
- Started: `2026-09-03T19:57:23Z`
- Completed: `2026-09-03T20:07:09Z`
- Reviewer commit: `4f5ceba4f22f9e0f94a36fd9d0f9ab0d6774fb09`
- Bootstrapped: 0
- Reviewed: 2
- Published: 1
- Deferred: 342
- Skipped: 400
- Regression cases: 0
- Unresolved regression fixes: 0

## Reviewed PRs

### [#333859 — Surface extensions blocked by the marketplace](https://github.com/microsoft/vscode/pull/333859)

- Head: `65fb4147de857f64f743dbfbb3a3952d8cea8f56`
- Findings: 1
- Published: true
- Reports: [pr-333859-65fb4147de85.json](pr-333859-65fb4147de85.json), [pr-333859-65fb4147de85.md](pr-333859-65fb4147de85.md)

#### Published comment at `src/vs/platform/extensionManagement/common/extensionGalleryService.ts:1394`

**[Experimental performance review bot]**

**Severity: medium**

The added blocked bit makes getValidRawGalleryExtensionVersion reject a version-blocked latest resource result. getLatestGalleryExtension then returns NOT\_COMPATIBLE, getExtensionsUsingResourceApi retries the affected IDs through a latest-only query, and queryGalleryExtensions rejects the same versions again before issuing its all-versions query.

Periodic and manual extension update checks that encounter a version-blocked latest release wait for two serial Marketplace POSTs after the existing resource fetches. On high-latency links this can add multiple round-trip times before VS Code can determine the usable older version and schedule updates.

**Suggested fix:** Propagate a distinct latest-version-is-blocked fallback result and batch those IDs directly into an IncludeVersions query, skipping the intervening latest-only query while preserving selection of an older permitted version.

(Written by Copilot)

### [#334260 — Implement queueing for busy target chats in send\_message tool](https://github.com/microsoft/vscode/pull/334260)

- Head: `8b01184faa5031a85e3736b228ab318f290013ec`
- Findings: 0
- Published: false
- Reports: [pr-334260-8b01184faa50.json](pr-334260-8b01184faa50.json), [pr-334260-8b01184faa50.md](pr-334260-8b01184faa50.md)
