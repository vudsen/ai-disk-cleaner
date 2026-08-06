## Context

当前 Agent.run 用局部 messages 切片维护完整对话，工具只接收 *diskCleanerContext，因此工具无法访问或压缩对话历史。analyze_directory 的 CSV 返回通常是历史中体积最大的内容；LLM 在完成某个目录分支的判断后仍会反复携带这些结果进入后续请求。

本变更位于单一 analyzer 服务内，但会同时改变 Agent 状态所有权、工具调用接口及消息历史的不变量。路径采用文件树工具已经公开的、以 / 开头且使用 / 分隔的逻辑路径。

## Goals / Non-Goals

**Goals:**

- 让 LLM 通过 clear_analyze_history(paths) 主动删除指定路径严格后代的历史扫描上下文。
- 始终保留目标路径自身的扫描结果，并保留祖先、无关路径以及前缀相似但不属于后代的路径。
- 直接过滤 tool response CSV 中匹配的扫描行，同时保留 assistant tool call 和 tool response 消息，保持消息序列有效。
- 由 Agent 统一持有运行中的消息、文件树和分析结果，并让所有工具直接操作该实例。
- 对多路径、重复/重叠路径及异常参数提供确定且可测试的行为。

**Non-Goals:**

- 不压缩普通 assistant/user 文本，也不总结被删除的扫描结果。
- 不删除目标路径自身的 analyze_directory 记录。
- 不修改累计 token 用量、上下文阈值策略或模型配置。
- 不清理磁盘文件，也不改变最终清理建议的用户确认流程。
- 不改变前端或 Wails 暴露 API。

## Decisions

### 1. Agent 成为单一运行状态容器

在 Agent 增加消息切片字段，并在 newAgent 或 run 开始时初始化 system/user 消息。run 的每次请求、assistant 消息追加和 tool result 追加都直接读写该字段，不再保留独立局部副本。

tool.invoke 与 toolsManager.Invoke 接收 *Agent。现有工具从 agent.tree 读取文件树，并直接更新 agent.TrashFiles、agent.TopUsages。旧 diskCleanerContext 及未使用的间接状态应被移除，避免形成两份事实来源。

选择此方案是因为历史压缩本身属于一次 Agent 状态变更；若仅给特定工具额外传入消息指针，会让工具接口分叉并增加切片被替换后调用方仍持有旧值的风险。

### 2. 直接扫描并过滤 tool response CSV

压缩逻辑只检查历史中的 tool response。若响应内容能解析为 `analyze_directory` 的 `path,totalSize,type` CSV，则逐行规范化 path 并执行严格后代匹配：

1. 匹配目标严格后代的 CSV 行从响应内容中移除；
2. 目标路径自身、祖先、无关路径和相似前缀行保持不变；
3. assistant 消息完全不解析、不修改，tool response 消息及其 tool-call ID 也保持不变；
4. 非 CSV、表头不匹配或无法安全解析的响应保持原样。

选择直接过滤 CSV 是因为真正消耗上下文的是响应中的扫描明细。保留消息结构并只删除不再需要的行，可以避免解析或重建 assistant tool calls，也不会产生缺失 tool response 的无效消息序列。

### 3. 使用规范化逻辑路径执行严格后代匹配

工具参数为必填 paths: string[]，每项必须是以 / 开头、只使用 / 分隔的逻辑路径。Invoke 在修改历史前先解析并验证全部参数；任一非法项使调用返回错误且历史不变。

每项通过与文件树一致的逻辑路径规范化处理，去除冗余分隔符、点段和尾随斜杠。删除条件是扫描路径位于目标路径下且不等于目标路径。比较使用路径分段边界，而不是裸字符串前缀；因此 /foo/bar2 不是 /foo/bar 的后代。根路径 / 的严格后代包括所有非根路径。

多个路径按集合并集处理。重复路径和被另一目标包含的路径不会改变结果，保证幂等性。

### 4. 工具结果返回可观察的压缩摘要

成功时返回 JSON 摘要，至少包含被移除的 CSV 扫描行数量，便于 LLM 判断压缩是否生效；没有匹配项时成功返回零。错误沿用 manager 的现有错误传递路径，由运行循环转成 tool result。

相较始终返回 true，计数摘要不会重新引入大量上下文，却能减少 LLM 为确认效果而重复调用。

### 5. 工具按 Agent 上下文状态动态披露

`tool` 接口增加 `IsSupport(*Agent) bool`，`buildTools` 仅生成当前 Agent 支持的工具定义，并在每次 completion 请求前重新执行过滤，而不是在运行循环外缓存结果。

`analyze_directory` 在 High 状态返回不支持，阻止模型继续扩大扫描上下文；`clear_analyze_history` 在 Low 状态返回不支持，避免上下文尚小时增加无意义操作。Medium 状态同时提供二者，其余结果类工具始终提供。过滤只控制模型可见的工具定义，不在 Invoke 阶段二次拒绝，以免一次 completion 返回后状态变化导致已经获准的调用被拒绝。

### 6. 提示模型执行窄范围历史压缩

`clear_analyze_history` 的工具描述、参数说明以及 Medium/High 运行时提示必须一致强调：只选择已读取、已完成判断且后续不再使用的具体非根子目录，禁止传入 `/`，并避免清除仍在分析或后续推理中需要引用的目录分支。底层路径语义继续支持根路径，以保持内部能力通用；本决策只约束 LLM 的正常工具选择。

### 7. analyze_directory 深度表示展开的子层数

`analyze_directory` 始终返回请求的目标节点，并将 `depth` 解释为需要展开的后代层数。因此 `depth=1` 返回目标节点及其直接子项，`depth=2` 再包含孙级节点。这样模型首次以 `path="/"`、`depth=1` 探索文件树时即可获得真实的下一层路径，不需要猜测根目录的子项。

## Risks / Trade-offs

- [CSV 响应格式异常导致误删] → 仅处理表头严格匹配 `path,totalSize,type` 且能完整解析的响应，其他响应保持原样。
- [重写 tool response 时破坏消息配对] → 不删除任何消息或 tool-call ID，只替换匹配响应的 content 字段。
- [路径规范化与文件树语义不一致导致误匹配] → 复用 modelscanner.NormalizeTreePath 或等价 POSIX 逻辑，并覆盖根路径、尾斜杠、重叠路径和相似前缀测试。
- [清除历史后累计 token 告警仍可能升高] → 明确累计用量是费用/预算统计而非当前上下文长度，本变更不回退计数。
- [工具可删除模型之后可能想引用的信息] → 删除动作只能由 LLM 显式调用，且目标节点自身作为导航锚点保留。

## Migration Plan

1. 先迁移 Agent 消息与分析结果状态，并统一工具 Invoke 签名。
2. 实现历史筛选纯函数及单元测试。
3. 注册并描述 clear_analyze_history 工具，补充端到端工具调用测试。
4. 运行 analyzer 测试及项目构建。若需回滚，可删除新工具并恢复局部 messages 与旧 Invoke 上下文；无持久化数据需要迁移。

## Open Questions

无。实现以“仅清除 tool response CSV 中严格后代路径的扫描行，不解析 assistant 消息”为固定语义。
