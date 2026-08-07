## Context

`analyze_directory` 的扫描结果会持续扩大 Agent 消息历史。与其在旧消息中筛选和重组结果，模型现在负责提供一份完整的扫描交接总结，运行时直接开启全新上下文。

## Goals / Non-Goals

**Goals:**

- 让 LLM 通过 `compress_context(summary)` 开启全新上下文。
- summary 明确列出当前已经搜索的目录，并声明后续禁止再次扫描。
- summary 明确列出剩余未扫描的目录，供新上下文继续工作。
- 新上下文只包含基础 system prompt 和承载 summary 的 user prompt。
- 重置当前上下文 token 状态并重新开放 `analyze_directory`，同时保留累计 token 用量和分析结果。

**Non-Goals:**

- 不解析、合并或保留旧 `analyze_directory` CSV。
- 不构造替代性的 assistant/tool 消息。
- 不重置累计 token 用量、TrashFiles 或 TopUsages。
- 不改变前端或 Wails API。

## Decisions

### 1. Agent 持有并替换消息历史

Agent 继续作为消息、文件树和分析结果的单一状态容器。工具调用成功后，将 `agent.messages` 原子替换为基础 system prompt 与新的 summary user prompt。

### 2. summary 是唯一参数

`compress_context` 使用严格 JSON Schema，只允许非空 `summary`。说明要求 summary 包含两部分：

1. 当前已经搜索的目录，这些目录后续禁止再次扫描；
2. 剩余未扫描的目录。

运行时使用拒绝未知字段的 JSON 解码，旧的 `paths` 参数会直接报错且不改变 Agent。

### 3. 不追加当前工具响应

`compress_context` 成功时已经替换了历史。Agent 运行循环识别这一工具并跳过常规 tool result 追加，因此新上下文中不会出现无对应 assistant call 的 tool message，也不会主动构造替代消息对。

### 4. 重置当前上下文状态

上下文重建后将 `state` 和 `totalTokens` 重置为 Low/0，使下一轮重新提供 `analyze_directory`。累计费用统计 `usedTokens` 保持不变。

### 5. 工具按上下文状态动态披露

`compress_context` 在 Low 状态隐藏，在 Medium/High 状态可用；`analyze_directory` 在 High 状态隐藏。压缩成功回到 Low 后，下一轮重新开放扫描工具。

## Risks / Trade-offs

- [summary 遗漏扫描信息] → 工具描述和运行时提示明确要求已搜索与未扫描目录两部分。
- [模型重复扫描] → 新 user prompt 明确禁止再次扫描 summary 中的已搜索目录。
- [新历史被旧 tool result 污染] → 运行循环对成功的 `compress_context` 跳过常规 tool message 追加。
- [累计预算被意外重置] → 仅重置当前上下文 token 数，不修改 `usedTokens`。

## Migration Plan

1. 将工具参数收敛为 summary。
2. 用 system/user 两条消息替换旧历史。
3. 在运行循环中跳过成功压缩调用的 tool result。
4. 更新工具提示、测试与 OpenSpec。

## Open Questions

无。
