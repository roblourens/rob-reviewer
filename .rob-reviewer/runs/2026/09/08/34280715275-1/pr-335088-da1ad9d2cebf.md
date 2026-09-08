# Performance review: Make chatSessionResource available in grep search and read file tool

[PR #335088](https://github.com/microsoft/vscode/pull/335088) at `da1ad9d2cebf03a1d7ee38c04d61af33e7a16cdd`

## Findings

No high-confidence performance findings.

<details>
<summary>Reviewer scenario analysis</summary>

**Relevant issue families:** boundary-fanout, serialization-allocation

### Scenario 1: Panel chat executes model-generated tool calls, including grep\_search and read\_file

- Critical path: Input parsing, pre-tool hook execution, the existing extension-host/main-thread tool invocation, and the selected tool's work must complete before its result can be returned to the model. The added URI property is only copied into the invocation-options object and does not add an awaited step.
- Scaling input: Number of model-emitted tool calls and, within each tool, the existing workspace/file size. The added cost is a constant-size property assignment per call.
- Effective concurrency: Concurrency remains controlled by the existing prompt/tool-call rendering path; the diff adds no Promise, queue, limiter, or fan-out.
- Cache behavior: Existing toolCallResult reuse skips invocation, and existing schema/tokenizer/tool caches are unchanged. Cold and warm paths differ exactly as before.
- Mechanism confidence: High confidence: the changed line only attaches an already-created sessionResource to the existing options object; traced invocation cardinality and filesystem boundaries are unchanged.
- Magnitude uncertainty: Tool durations vary with workspace and file size, but none of that scale is introduced or amplified by this diff.
- Expensive boundaries:
  - IPC at <code>extensions/copilot/src/extension/prompts/node/panel/toolCalling.tsx:314 and extensions/copilot/src/extension/tools/vscode-node/toolsService.ts:211</code>: Invoke the selected language-model tool through vscode.lm.invokeTool and return its result across the extension-host/main-thread boundary.; cardinality: One invocation per model-emitted tool call that lacks an existing toolCallResult; unchanged effective cardinality.
    - Introduced by diff: false; previous behavior: The same invokeToolWithEndpoint/vscode.lm.invokeTool call occurred once per uncached tool call. ChatRequest.toolInvocationToken already carries the sessionResource in production.; critical-path effect: The existing IPC/tool execution remains on the critical path; copying one existing URI reference does not add another round trip.
  - filesystem at <code>extensions/copilot/src/extension/tools/node/findTextInFilesTool.tsx:123 and extensions/copilot/src/extension/tools/node/readFileTool.tsx:146</code>: grep\_search scans workspace text, while read\_file opens and snapshots the requested file.; cardinality: At most one selected tool execution per emitted call; grep result limits and read ranges are unchanged.
    - Introduced by diff: false; previous behavior: Identical filesystem search/read operations were performed before the added invocation option.; critical-path effect: Tool-specific filesystem work remains required for the tool result, but the diff neither adds nor broadens it.
- Verdict: No reportable regression. The change carries context through an existing invocation without adding work, increasing boundary count, or changing concurrency.

### Scenario 2: Panel chat resolves explicit \#tool references/attachments

- Critical path: For an uncached reference, argument generation (when the tool lacks provideInput), then the existing tool invocation, must complete sequentially before prompt rendering continues. The new field adds no awaited operation.
- Scaling input: Effective cardinality is the number of explicit tool references after toolCallResults reuse. The new work remains one constant-size URI property assignment per miss.
- Effective concurrency: The for-of loop remains sequential; the change neither increases parallelism nor adds fan-out.
- Cache behavior: toolCallResults is the authoritative warm-path cache and bypasses both argument generation and invocation. The added property is only set on the uncached path and does not alter cache keys or retention.
- Mechanism confidence: High confidence from the sequential loop, cache branch, and unchanged fetch/invoke calls around the added line.
- Magnitude uncertainty: Users may attach varying numbers of tools, but the diff does not change per-reference boundary work.
- Expensive boundaries:
  - network at <code>extensions/copilot/src/extension/prompts/node/panel/chatVariables.tsx:501</code>: Generate arguments with the utility chat endpoint when the referenced tool has an input schema and no locally provided input.; cardinality: Up to one model request per uncached tool reference requiring generated arguments; references are processed sequentially.
    - Introduced by diff: false; previous behavior: The same fetchToolArgs model request occurred before the diff.; critical-path effect: The request is already on the prompt-render critical path and is unaffected by the new field.
  - IPC at <code>extensions/copilot/src/extension/prompts/node/panel/chatVariables.tsx:445</code>: Invoke each referenced tool through invokeToolWithEndpoint/vscode.lm.invokeTool.; cardinality: One call per tool reference without a toolCallResults entry, sequential across references.
    - Introduced by diff: false; previous behavior: Exactly one invocation occurred per uncached reference with the same tool input and token.; critical-path effect: Each existing invocation blocks completion of the attachment rendering; no extra invocation is introduced.
- Verdict: No reportable regression. Boundary cardinality, ordering, caching, and critical-path work are unchanged.

### Scenario 3: Inline chat executes editing/search tool calls emitted by the model

- Critical path: The model request emits tool calls; each existing tool invocation must settle before Promise.allSettled allows inline chat to finish. Adding sessionResource is a synchronous property copy before the same invocation.
- Scaling input: Number of model-emitted tool calls and each selected tool's existing input size. Added overhead is constant per call.
- Effective concurrency: Tool promises are still launched for each emitted call and joined with Promise.allSettled; no limiter or concurrency count changed.
- Cache behavior: No new cache or key is introduced. Existing tool/service caches and cancellation behavior remain unchanged on cold and warm invocations.
- Mechanism confidence: High confidence from the unchanged toolExecutions loop and Promise.allSettled join around the added field.
- Magnitude uncertainty: The model can emit multiple calls and filesystem costs vary, but neither multiplicity nor cost is changed by this PR.
- Expensive boundaries:
  - network at <code>extensions/copilot/src/extension/inlineChat2/node/inlineChatIntent.ts:450</code>: Run the inline-chat language-model request that emits tool calls.; cardinality: One model request for the inline-chat turn; unchanged.
    - Introduced by diff: false; previous behavior: The same model request ran before the diff.; critical-path effect: The model request remains the source of tool calls and is unaffected by the option addition.
  - IPC at <code>extensions/copilot/src/extension/inlineChat2/node/inlineChatIntent.ts:506</code>: Invoke each emitted tool through the language-model tool service.; cardinality: One invocation per validated emitted tool call; all accumulated executions are awaited together.
    - Introduced by diff: false; previous behavior: The same one-per-call invocations ran with the same Promise.allSettled concurrency.; critical-path effect: All existing invocations remain on the completion path; no additional IPC is created.
  - filesystem at <code>extensions/copilot/src/extension/inlineChat2/node/inlineChatIntent.ts:506</code>: Selected edit/search/read tools may access workspace files.; cardinality: Tool-dependent, one tool implementation per emitted call; unchanged by the diff.
    - Introduced by diff: false; previous behavior: The selected tools performed the same workspace operations before this change.; critical-path effect: Existing tool I/O must settle before completion, but the added session URI does not trigger an additional operation in the traced grep/read implementations.
- Verdict: No reportable regression. The diff propagates existing request metadata without changing expensive work or scheduling.

**Summary:** Reviewed all four changed files and traced the three invocation paths through the production tools service, extension-host/main-thread IPC, and grep/read filesystem implementations. The added chatSessionResource is an existing URI reference; normal ChatRequest.toolInvocationToken already contains the same session resource, and the diff does not add boundary calls, alter cache behavior, increase concurrency, or retain growing state. No performance findings were submitted.

</details>

<details>
<summary>Review statistics</summary>

- Model: `gpt-5.6-sol`
- Actual model calls: `gpt-5.6-sol`
- Reasoning effort: `high`
- Actual reasoning effort: `high`
- Model API endpoint: `ws:/responses`
- Wall-clock time: 2m14.085s
- Model calls: 17
- Tokens: 706180 input, 7157 output, 713337 total; 2155 reasoning; 634880 cache read, 0 cache write
- Aggregate model API time: 1m59.836s
- Model-returned tool calls: 85
- Copilot usage: 68229200000.000 nano-AI units; model billing multiplier: 1.000
- USD cost: unavailable; the Copilot SDK does not expose a dollar conversion.

</details>
