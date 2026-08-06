## Why

磁盘分析 Agent 会把每次 `analyze_directory` 的大段扫描结果持续保留在消息历史中，随着渐进式探索深入，会快速消耗上下文窗口并削弱后续判断质量。需要让 LLM 能主动丢弃已经提炼完成的子目录扫描上下文，同时保留目标目录本身的结果作为导航锚点。

## What Changes

- 新增 `clear_analyze_history` 工具，接收字符串数组 `paths`，并在调用时立即压缩当前 Agent 的消息历史。
- 对每个目标路径，仅移除该路径后代目录或文件对应的历史 `analyze_directory` 调用及其配对工具结果；目标路径自身、祖先路径和无关路径的扫描历史保持不变。
- 对多个目标路径按并集处理，并使用规范化后的路径边界判断，避免前缀相似路径被误删。
- 将对话消息历史从 `Agent.run` 的局部变量迁移到 `Agent` 状态，使工具能够安全地原地更新历史。
- **BREAKING** 将 analyzer 内部 `tool.invoke` 及 `toolsManager.Invoke` 的上下文参数由 `*diskCleanerContext` 改为 `*Agent`；现有工具改为从 Agent 读取文件树并写入分析结果。

## Capabilities

### New Capabilities

- `analyzer-history-compaction`: 定义 Agent 消息历史的持有方式，以及按路径清除后代扫描调用和结果的工具行为。

### Modified Capabilities

无。项目当前没有已发布的 OpenSpec capability。

## Impact

- 主要影响 `backend/service/analyzer/agnet.go`、`backend/service/analyzer/tools.go` 及 analyzer 测试。
- analyzer 内部工具接口发生签名变更，但不改变 Wails 前端 API。
- 不新增外部依赖；需要补充路径匹配、消息配对删除、多路径和边界条件测试。
