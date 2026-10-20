## 0. Preflight and implementation gates

- [x] 0.1 阅读根/目录 AGENTS.md、存在的 RTK.md、本方案全部文档；记录当前 git status，保留 Linux 适配、工作流及其它变更，不把未跟踪文件当作可覆盖文件。
- [x] 0.2 按 execution-guide.md 一次性确认必要 Go 测试包；获准后才编写测试及对应实现，批准前继续只读核查与既有基线检查。
- [x] 0.3 重新核实本机 Codex 版本/帮助并生成临时实验 schema；记录所需 RPC、动态工具参数、终态和 usage 字段到 docs/local-codex-validation.md，不复制认证数据。
- [x] 0.4 优先完成 design D5 的有效工具隔离验证：临时 cwd、环境禁用、内置 shell/扩展禁用及继承 MCP 禁用。无法达到时保留后续实现未完成并报告具体缺口，不退化为自由 shell 扫盘。
- [x] 0.5 记录现有前后端检查结果与环境缺失；确定当前 Windows/Linux 构建参数，不修改 Go/Wails/gdu 版本来掩盖既有失败。

## 1. Preserve and reuse the existing API behavior

- [x] 1.1 在批准的 TDD 范围内先为目录排序/上限、候选去重及三业务工具补充必要回归，再从 tools.go 抽取私有帮助函数供旧 API 工具调用；不得改变 compress_context、Agent 消息或现有 IsSupport 行为。
- [x] 1.2 引入私有 provider 配置分支，缺省 api；API 分支继续使用现有 run/TestConnection；测试旧七键配置、相同 API 请求参数与错误行为，保证重构前后结果一致。

## 2. Provider settings and readiness

- [x] 2.1 测试先行新增四个默认设置键；验证 OnConflict 不覆盖旧值，旧七键 Save 请求保留新键，新十一键保存无缺失，未知/重复键被拒绝。
- [x] 2.2 测试先行实现活动 provider 配置验证：Codex 不依赖 API 凭据/额外 body，超时 30–3600，路径不是命令字符串；保存不完整 API 配置仍保持旧行为。
- [x] 2.3 在 analyzer.go 定义 ProviderStatus 与 GetStatus，并准备 app.go 的 GetLLMStatus 薄委托；先用私有探测 fake 验证 ready 与实际模型成功的区别，不新增服务单例。

## 3. Process lifecycle and protocol transport

- [x] 3.1 用 Go 测试辅助子进程先覆盖路径空格、找不到 CLI、非兼容版本、Windows 包装器和 stdout/stderr 分离，再实现私有进程工厂、临时 cwd 与固定参数启动。
- [x] 3.2 测试先行实现 JSON 行读写、握手时序、pending id、并发服务器请求分流、帧上限和有界 stderr；覆盖分片/多帧、乱序响应、未知通知、未知请求、EOF 和非零退出。
- [x] 3.3 根据本版本 schema 解析 account/read 与可选模型设置，接通 GetStatus；验证缺登录、不要求 OpenAI 认证的 CLI provider、空模型继承及未知版本受控错误，不读取 auth.json。
- [x] 3.4 测试先行实现 interrupt、2 秒宽限、超时和拥有进程树的回收；覆盖取消时 interrupt 投递、进程树终止及 Wait 仅一次，不按进程名终止外部 Codex。阻塞读写/原生 Windows 运行留作平台验收。

## 4. Codex tools and analysis workflow

- [x] 4.1 新建英文 CODEX_SYSTEM.md；明确三个应用工具、精确候选边界与风险规则，复用现有用户语言要求；不修改原 API 系统 prompt 或压缩变更。
- [x] 4.2 测试先行实现 dynamicTools schema 和 item/tool/call 响应；arguments 按 JSON 值处理，request id/callId 分开，重复 callId 幂等，工具调用串行且参数/结果有界。
- [x] 4.3 测试先行实现 Codex 树路径→FindNode→RelativePath/真实扫描大小的映射；覆盖非根目录扫描、大小写、POSIX 反斜杠、未知路径/../glob/根候选拒绝和批量原子校验。
- [x] 4.4 实现 per-task Codex 状态和三个业务帮助函数调用；验证不重新遍历磁盘、不写真实文件、不暴露 compress_context，不从普通文本提取候选。
- [x] 4.5 测试先行接入 agentMessage delta 去重和最终 AnalysisResult；只有 completed 成功，failed/interrupted/早退失败，无候选成功为空列表；复用 cleaner 的现有状态及输出事件。
- [x] 4.6 测试先行实现累计 usage 快照及脱敏日志；验证 100/100/150 最终为 150、未知用量不虚构、取消后无回调或新候选，API 日志回归通过。
- [x] 4.7 实现 Codex TestConnection，使用未保存配置与独立 30 秒最小实际推理；fake smoke 确认 completed 和非空消息条件，不自动执行真实模型请求。

## 5. Frontend integration

- [x] 5.1 设置页增加 provider/path/model/timeout，复用现有表单组件；只校验活动分支、保留隐藏值、禁止重复检测，新增文字同步 en/zh_CN。
- [x] 5.2 首页统一用 GetLLMStatus 替换两处 API 三字段硬编码检测；区分配置缺失、CLI 不存在、未登录和可开始状态，API 提示保持可用。
- [x] 5.3 通过固定 Wails 2.12.0 绑定生成 API，接通原 TestLLMConnection 的分支和新 GetLLMStatus；不手写桥接或提供通用命令执行 API。
- [ ] 5.4 自动执行 pnpm lint、pnpm check、pnpm build 并检查 diff 均已完成；桌面手工验收未执行（按用户要求不操作桌面）。

## 6. Validation and documentation

- [x] 6.1 跑相关包测试、transport/取消 race、完整后端及 Linux 原生 Wails build；Windows 全应用交叉构建通过，Windows 原生运行未验证并已记录。
- [ ] 6.2 在用户已有且允许使用的 Codex 登录环境，用临时树实际验证渐进工具调用、流式输出、候选路径、取消和历史；不删除真实用户文件，不把 fake 通过当真实 Codex 验收。
- [x] 6.3 更新双语 README 的 CLI 安装前提、终端登录、可执行路径、模型继承、联网属性及 API/Codex 设置差异；不承诺免费或账户权限。
- [x] 6.4 写 docs/local-codex-validation.md：版本/协议、工具隔离证据、用例映射、命令结果、双平台/真实登录状态及限制；对照规格审计获准范围的覆盖，保留未完成项。
- [x] 6.5 检查功能改动与既有 Linux 工作分界、任务证据；不提交、不发布、不归档其它变更。
