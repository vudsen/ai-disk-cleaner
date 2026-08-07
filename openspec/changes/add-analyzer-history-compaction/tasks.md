## 1. Agent 状态与工具接口迁移

- [x] 1.1 在 Agent 中增加并初始化消息历史字段，使 run 的 completion 请求、assistant 消息和 tool result 全部直接读写该字段
- [x] 1.2 将 tool.invoke 和 toolsManager.Invoke 的上下文参数统一改为 *Agent，并更新所有调用点和测试替身
- [x] 1.3 将现有工具迁移为从 Agent 读取文件树并直接更新 TrashFiles、TopUsages，移除 diskCleanerContext 等重复状态

## 2. 历史压缩核心逻辑

- [x] 2.1 使用基础 system prompt 和 summary user prompt 原子替换旧消息历史
- [x] 2.2 成功压缩后不追加当前 tool result，也不构造 assistant/tool 消息
- [x] 2.3 将上下文状态和当前 token 数重置，保留累计 token 与分析结果

## 3. 工具注册与契约

- [x] 3.1 实现 compress_context 工具及严格 JSON Schema，仅要求 summary
- [x] 3.2 将 compress_context 注册到 toolsManager，并完善工具描述
- [x] 3.3 为 tool 增加 IsSupport(*Agent)，并按 Low/Medium/High 状态在每轮 completion 前动态过滤工具列表
- [x] 3.4 要求 summary 列出已搜索且禁止重复扫描的目录，以及剩余未扫描目录
- [x] 3.5 修正 analyze_directory 深度语义，使 depth=1 返回目标节点及直接子项，并同步工具说明和回归测试
- [x] 3.6 将历史压缩改为直接开启全新 system/user 上下文

## 4. 测试与验证

- [ ] 4.1 增加 Agent 消息所有权和现有工具迁移测试，确认最终分析结果仍来自 Agent 状态
- [x] 4.2 增加 summary-only 契约、未知参数拒绝及非法输入原子性测试
- [x] 4.3 增加全新上下文、无 assistant/tool 消息及状态重置测试
- [ ] 4.4 运行 analyzer 测试、Go 全量测试和项目构建，并修复所有回归
