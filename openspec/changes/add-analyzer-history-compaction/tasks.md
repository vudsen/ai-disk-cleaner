## 1. Agent 状态与工具接口迁移

- [ ] 1.1 在 Agent 中增加并初始化消息历史字段，使 run 的 completion 请求、assistant 消息和 tool result 全部直接读写该字段
- [ ] 1.2 将 tool.invoke 和 toolsManager.Invoke 的上下文参数统一改为 *Agent，并更新所有调用点和测试替身
- [ ] 1.3 将现有工具迁移为从 Agent 读取文件树并直接更新 TrashFiles、TopUsages，移除 diskCleanerContext 等重复状态

## 2. 历史压缩核心逻辑

- [ ] 2.1 实现逻辑路径的全量预验证、规范化和严格后代匹配，覆盖根路径、尾斜杠、点段、重复/重叠目标及相似前缀
- [ ] 2.2 实现 analyze_directory 调用参数解析和按 tool-call ID 配对的历史筛选，仅删除可安全识别的完整调用/结果对
- [ ] 2.3 重建混合 assistant 消息时保留文本、无关 tool calls 及其结果，并删除清空后不再含有效内容的消息
- [ ] 2.4 返回包含 removed scan-call count 的紧凑 JSON 摘要，并保证重复调用幂等

## 3. 工具注册与契约

- [ ] 3.1 实现 clear_analyze_history 工具及严格 JSON Schema，要求 paths 字符串数组且声明逻辑路径格式
- [ ] 3.2 将 clear_analyze_history 注册到 toolsManager，并完善工具描述以强调仅清除目标路径严格后代的扫描历史

## 4. 测试与验证

- [ ] 4.1 增加 Agent 消息所有权和现有工具迁移测试，确认最终分析结果仍来自 Agent 状态
- [ ] 4.2 增加目标保留、后代删除、相似前缀、根路径、多路径、规范化及非法输入原子性的单元测试
- [ ] 4.3 增加 tool-call/result 成对删除、混合 assistant 消息保留、畸形或未配对记录保留及幂等性测试
- [ ] 4.4 运行 analyzer 测试、Go 全量测试和项目构建，并修复所有回归
