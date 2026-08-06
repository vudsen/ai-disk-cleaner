## Context

当前 Agent.run 用局部 messages 切片维护完整对话，工具只接收 *diskCleanerContext，因此工具无法访问或压缩对话历史。analyze_directory 的 CSV 返回通常是历史中体积最大的内容；LLM 在完成某个目录分支的判断后仍会反复携带这些结果进入后续请求。

本变更位于单一 analyzer 服务内，但会同时改变 Agent 状态所有权、工具调用接口及消息历史的不变量。路径采用文件树工具已经公开的、以 / 开头且使用 / 分隔的逻辑路径。

## Goals / Non-Goals

**Goals:**

- 让 LLM 通过 clear_analyze_history(paths) 主动删除指定路径严格后代的历史扫描上下文。
- 始终保留目标路径自身的扫描结果，并保留祖先、无关路径以及前缀相似但不属于后代的路径。
- 删除一个扫描记录时，同时删除对应的 analyze_directory tool call 和由其 tool-call ID 关联的 tool result，保持消息序列有效。
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

### 2. 通过 tool-call ID 成对识别和删除扫描记录

压缩逻辑遍历历史 assistant 消息中的 tool calls，只检查函数名为 analyze_directory 的调用，并解析其参数中的 path。满足删除条件的调用 ID 组成集合，然后：

1. 从 assistant 消息中移除这些特定 tool call；
2. 从历史中移除具有对应 tool-call ID 的 tool result；
3. assistant 消息若仍含文本或其他 tool calls 则保留；若清除后为空则删除；
4. 不完整、参数无法解析或 ID 无法配对的记录保持不变，避免误删。

当前正在执行的 clear_analyze_history 调用不属于 analyze_directory，因此会保留；它的结果在 Invoke 返回后照常追加。该设计也能保留同一 assistant 消息中的其他工具调用。

选择调用参数而非解析 CSV 结果，是因为请求路径是结构化且稳定的来源，tool-call ID 则是请求与结果的明确关联键。

### 3. 使用规范化逻辑路径执行严格后代匹配

工具参数为必填 paths: string[]，每项必须是以 / 开头、只使用 / 分隔的逻辑路径。Invoke 在修改历史前先解析并验证全部参数；任一非法项使调用返回错误且历史不变。

每项通过与文件树一致的逻辑路径规范化处理，去除冗余分隔符、点段和尾随斜杠。删除条件是扫描路径位于目标路径下且不等于目标路径。比较使用路径分段边界，而不是裸字符串前缀；因此 /foo/bar2 不是 /foo/bar 的后代。根路径 / 的严格后代包括所有非根路径。

多个路径按集合并集处理。重复路径和被另一目标包含的路径不会改变结果，保证幂等性。

### 4. 工具结果返回可观察的压缩摘要

成功时返回 JSON 摘要，至少包含被移除的扫描调用数量，便于 LLM 判断压缩是否生效；没有匹配项时成功返回零。错误沿用 manager 的现有错误传递路径，由运行循环转成 tool result。

相较始终返回 true，计数摘要不会重新引入大量上下文，却能减少 LLM 为确认效果而重复调用。

## Risks / Trade-offs

- [SDK 的 message union 修改较繁琐，错误重建可能丢失字段] → 用小型纯函数完成筛选，并为含文本、混合 tool calls 和配对 result 的消息添加单元测试；重建时保留未修改字段。
- [历史中存在畸形或未配对 tool call] → 仅删除能够按调用 ID 安全关联的完整扫描记录，无法确定的消息保持原样。
- [路径规范化与文件树语义不一致导致误匹配] → 复用 modelscanner.NormalizeTreePath 或等价 POSIX 逻辑，并覆盖根路径、尾斜杠、重叠路径和相似前缀测试。
- [清除历史后累计 token 告警仍可能升高] → 明确累计用量是费用/预算统计而非当前上下文长度，本变更不回退计数。
- [工具可删除模型之后可能想引用的信息] → 删除动作只能由 LLM 显式调用，且目标节点自身作为导航锚点保留。

## Migration Plan

1. 先迁移 Agent 消息与分析结果状态，并统一工具 Invoke 签名。
2. 实现历史筛选纯函数及单元测试。
3. 注册并描述 clear_analyze_history 工具，补充端到端工具调用测试。
4. 运行 analyzer 测试及项目构建。若需回滚，可删除新工具并恢复局部 messages 与旧 Invoke 上下文；无持久化数据需要迁移。

## Open Questions

无。实现以“仅清除严格后代的完整 analyze_directory 调用/结果对”为固定语义。
