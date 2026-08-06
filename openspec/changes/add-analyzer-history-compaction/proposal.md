## Why

磁盘分析 Agent 会把每次 `analyze_directory` 的大段扫描结果持续保留在消息历史中，随着渐进式探索深入，会快速消耗上下文窗口并削弱后续判断质量。需要让 LLM 能主动丢弃已经提炼完成的子目录扫描上下文，同时保留目标目录本身的结果作为导航锚点。

## What Changes

- 新增 `clear_analyze_history` 工具，接收字符串数组 `paths`，并在调用时立即压缩当前 Agent 的消息历史。
- 对每个目标路径，直接扫描历史 tool response 中的 `analyze_directory` CSV，并仅移除路径位于该目标严格后代的 CSV 行；目标路径自身、祖先路径和无关路径的扫描行保持不变。
- 压缩过程不解析 assistant tool call 参数，也不删除 assistant 或 tool response 消息，仅重写匹配响应中的 CSV 内容。
- 对多个目标路径按并集处理，并使用规范化后的路径边界判断，避免前缀相似路径被误删。
- 将对话消息历史从 `Agent.run` 的局部变量迁移到 `Agent` 状态，使工具能够安全地原地更新历史。
- **BREAKING** 将 analyzer 内部 `tool.invoke` 及 `toolsManager.Invoke` 的上下文参数由 `*diskCleanerContext` 改为 `*Agent`；现有工具改为从 Agent 读取文件树并写入分析结果。
- 工具通过 `IsSupport(*Agent)` 声明状态相关可用性：Low 阶段隐藏 `clear_analyze_history`，High 阶段隐藏 `analyze_directory`，每轮 completion 前重新计算工具列表。
- 模型可见提示明确禁止使用 `/` 作为历史压缩目标，并要求优先选择已经读取、完成判断且后续不再使用的具体子目录。
- 修正 `analyze_directory` 的深度语义：`depth=1` 返回目标节点及其直接子项，确保首次展开根目录时模型能看到真实的下一层路径。

## Capabilities

### New Capabilities

- `analyzer-history-compaction`: 定义 Agent 消息历史的持有方式，以及按路径清除后代扫描调用和结果的工具行为。

### Modified Capabilities

无。项目当前没有已发布的 OpenSpec capability。

## Impact

- 主要影响 `backend/service/analyzer/agnet.go`、`backend/service/analyzer/tools.go` 及 analyzer 测试。
- analyzer 内部工具接口发生签名变更，但不改变 Wails 前端 API。
- 不新增外部依赖；需要补充 CSV 响应过滤、路径匹配、多路径和边界条件测试。
