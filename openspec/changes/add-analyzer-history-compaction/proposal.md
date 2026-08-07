## Why

磁盘分析 Agent 会把每次 `analyze_directory` 的大段扫描结果持续保留在消息历史中，随着渐进式探索深入，会快速消耗上下文窗口并削弱后续判断质量。需要让 LLM 能主动丢弃已经提炼完成的子目录扫描上下文，同时保留目标目录本身的结果作为导航锚点。

## What Changes

- 新增 `compress_context` 工具，只接收必填的 `summary`。
- summary 必须列出当前已经搜索的目录（后续禁止再次扫描）和剩余未扫描的目录。
- 调用成功后丢弃完整旧消息历史，直接以基础 system prompt 和 summary user prompt 开启全新上下文，不构造 assistant/tool 消息。
- 将对话消息历史从 `Agent.run` 的局部变量迁移到 `Agent` 状态，使工具能够安全地原地更新历史。
- **BREAKING** 将 analyzer 内部 `tool.invoke` 及 `toolsManager.Invoke` 的上下文参数由 `*diskCleanerContext` 改为 `*Agent`；现有工具改为从 Agent 读取文件树并写入分析结果。
- 工具通过 `IsSupport(*Agent)` 声明状态相关可用性：Low 阶段隐藏 `clear_analyze_history`，High 阶段隐藏 `analyze_directory`，每轮 completion 前重新计算工具列表。
- 模型可见提示要求 summary 清晰区分已经搜索、禁止重复扫描的目录与剩余未扫描目录。
- 修正 `analyze_directory` 的深度语义：`depth=1` 返回目标节点及其直接子项，确保首次展开根目录时模型能看到真实的下一层路径。

## Capabilities

### New Capabilities

- `analyzer-history-compaction`: 定义 Agent 消息历史的持有方式，以及通过扫描交接总结开启全新上下文的工具行为。

### Modified Capabilities

无。项目当前没有已发布的 OpenSpec capability。

## Impact

- 主要影响 `backend/service/analyzer/agnet.go`、`backend/service/analyzer/tools.go` 及 analyzer 测试。
- analyzer 内部工具接口发生签名变更，但不改变 Wails 前端 API。
- 不新增外部依赖；需要补充上下文重置、工具结果跳过和参数契约测试。
