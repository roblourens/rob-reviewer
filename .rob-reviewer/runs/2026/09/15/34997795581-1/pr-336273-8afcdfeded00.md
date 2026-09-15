# Performance review: feat: adds the default openapi file match for the JSON extension

[PR #336273](https://github.com/microsoft/vscode/pull/336273) at `8afcdfeded0068de26d9eeb69a6c6544907007b4`

## Findings

No high-confidence performance findings.

<details>
<summary>Reviewer scenario analysis</summary>

**Relevant issue families:** eager-work, boundary-fanout, cache-lifecycle, repeated-work

### Scenario 1: Activate the built-in JSON language feature and publish schema associations to the language server

- Critical path: Extension activation computes associations before sending the schema-association notification. The three added declarations are only enumerated and serialized; no added schema download occurs on this path.
- Scaling input: Number of installed extensions and declared JSON validation associations; this diff adds three constant-size associations.
- Effective concurrency: Association computation is represented by one cached promise. Registry files are read sequentially, then one notification is sent.
- Cache behavior: The association list is cached for the client lifetime and recomputed only after an extension/registry refresh. The added static package entries do not alter cache keys or negative-cache behavior.
- Mechanism confidence: High: package contributions are synchronously enumerated, the association promise is cached, and the only activation boundaries are directly traced.
- Magnitude uncertainty: Exact installed-extension association count varies, but the diff's increment is fixed and negligible relative to the existing full-list operation.
- Expensive boundaries:
  - filesystem at <code>extensions/json-language-features/client/src/jsonClient.ts:getSchemaRegistryAssociations</code>: Read each configured JSON validation registry with workspace.fs.readFile.; cardinality: One read per validation registry; the built-in configuration has one registry, independent of the three added direct associations.
    - Introduced by diff: false; previous behavior: The same registry was read before this diff; adding direct package associations does not add registry reads.; critical-path effect: Activation awaits the existing registry read before publishing associations.
  - IPC at <code>extensions/json-language-features/client/src/jsonClient.ts:startClientWithParticipants</code>: Send the complete schema-association list to the JSON language server.; cardinality: One notification on activation, plus existing debounced refreshes when extensions or registries change; payload grows by three entries.
    - Introduced by diff: false; previous behavior: The notification and full-list serialization already occurred; this diff only adds three constant-size records.; critical-path effect: The initial notification must be delivered before the server can use extension schema associations.
- Verdict: No reportable regression: the diff adds constant-size in-memory and IPC payload work without adding an activation-time network request or scaling loop.

### Scenario 2: Open or edit arazzo.json, openapi.json/\*.openapi.json, or overlay.json/\*.overlay.json and request diagnostics/completions

- Critical path: After document-open/change diagnostics are requested (push diagnostics debounce changes by 500 ms), the language service matches the new association, requests unresolved schema content through LSP IPC, awaits trust checking and cache/network resolution, parses the schema, then validates the document. This diff moves schema resolution and schema-based validation onto this path for the newly matched filenames.
- Scaling input: Effective schema-fetch cardinality is grouped by the three schema URLs, not by every file once the language-service and desktop caches are warm. Validation CPU remains per changed/open document and scales with document and schema complexity.
- Effective concurrency: Push diagnostics coalesce pending validation separately per document; pull diagnostics follow editor requests. There is no changed concurrency scheduler in the diff. Shared-schema request coalescing is owned by the external JSON language-service cache and is not established by repository code.
- Cache behavior: On desktop, SchemaStore responses with ETags are persisted by exact URL and served without network for 48 hours; afterward they are conditionally revalidated. In browser workspaces there is no persistent client cache, though the language service owns its in-memory resolved-schema cache. Cold failures and transitive-reference caching depend on the fetched remote schema and service implementation.
- Mechanism confidence: High that the diff intentionally adds first-use schema IPC/network/cache work and per-document schema validation; low that it creates avoidable fan-out or repeated downloads because cache grouping and remote reference contents prevent proving such a mechanism.
- Magnitude uncertainty: Remote schema size, reference graph, browser cache headers, validation duration on very large OpenAPI documents, and prevalence of concurrent matching files are not established by repository evidence.
- Expensive boundaries:
  - IPC at <code>extensions/json-language-features/client/src/jsonClient.ts:VSCodeContentRequest handler</code>: Request HTTP(S) schema content from the extension-host client and return schema text to the language server.; cardinality: At least one request for the matched top-level schema on a cold language-service cache, grouped by one of three schema URLs; transitive $ref cardinality cannot be established from repository-owned content.
    - Introduced by diff: true; previous behavior: Without an explicit user/other-extension association, these filenames had no built-in OpenAPI schema and therefore did not make this schema-content IPC request.; critical-path effect: Schema-based diagnostics/completion wait for unresolved schema content.
  - filesystem at <code>extensions/json-language-features/client/src/node/schemaCache.ts</code>: Read or write the persistent JSON schema cache and update extension global state.; cardinality: One cache lookup/read for a warm matched schema URL; a successful cold response with ETag adds one file write per URL.
    - Introduced by diff: true; previous behavior: These cache operations were not triggered by the newly matched filenames absent another association.; critical-path effect: Warm desktop validation waits for a local cache read instead of network; cold successful resolution may await cache persistence before returning content.
  - network at <code>extensions/json-language-features/client/src/node/jsonClientMain.ts:getSchemaRequestService and client/src/browser/jsonClientMain.ts</code>: Download or conditionally revalidate the associated SchemaStore schema, following up to five redirects; trusted spec.openapis.org requests are also allowed when schema resolution reaches that origin.; cardinality: One top-level URL per effective matched schema group on a cold cache; desktop suppresses repeat requests for 48 hours when an ETag-backed entry exists. Additional remote-reference requests are possible but not provable because remote schema bodies are outside the repository.
    - Introduced by diff: true; previous behavior: The built-in association did not previously initiate these URLs for the matched filenames, and spec.openapis.org was not trusted by the default-domain setting.; critical-path effect: Cold schema-based diagnostics and completion wait for the download; the editor itself can open before delayed/pull diagnostics complete.
- Verdict: No reportable performance bug: the added cold resolution and schema-validation cost is the direct intended behavior of supplying default validation, is deferred until a matching document is processed, and repository evidence does not show avoidable repeated boundary work, unbounded retention, or a concurrency burst.

**Summary:** Reviewed the sole changed file and traced activation, association IPC, diagnostics scheduling, trust checks, desktop/browser schema requests, persistent cache behavior, and validation. No high-confidence actionable performance regression was established.

</details>

<details>
<summary>Review statistics</summary>

- Model: `gpt-5.6-sol`
- Actual model calls: `gpt-5.6-sol`
- Reasoning effort: `high`
- Actual reasoning effort: `high`
- Model API endpoint: `ws:/responses`
- Wall-clock time: 1m30.632s
- Model calls: 9
- Tokens: 205936 input, 4818 output, 210754 total; 1714 reasoning; 167957 cache read, 37952 cache write
- Aggregate model API time: 1m17.282s
- Model-returned tool calls: 42
- Copilot usage: 35341080000.000 nano-AI units; model billing multiplier: 1.000
- USD cost: unavailable; the Copilot SDK does not expose a dollar conversion.

</details>
