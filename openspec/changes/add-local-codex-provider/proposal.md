## Why

当前分析器仅支持 OpenAI-compatible 第三方 API，已安装并登录 Codex CLI 的用户仍需配置另一套 URL、密钥和模型。需要增加可切换的本地 Codex 调用方式，继续使用项目的内存文件树、渐进式工具和确认清理流程。

## What Changes

- 增加 `llm.provider=api|codex`，缺省为 api；旧配置与第三方 API 行为保持兼容，不自动降级或切换供应商。
- Go 后端启动用户安装的 `codex app-server`，通过 stdio 协议传输消息、应用工具、流式输出及取消请求；复用 Codex 自己管理的本机认证，不读取或复制认证令牌。
- Codex 通过项目暴露的三个业务工具读取 gdu 文件树和记录候选，不重新扫描真实磁盘、不执行删除命令。
- 设置页提供提供方、CLI 路径、可选模型及连接检测；首页配置检查按提供方分支，不再强制 Codex 用户填写第三方 API 密钥。
- 复用现有 AnalysisResult、任务事件、错误和历史存储；区分进程可用、已登录、模型调用成功三个状态。
- 补充 Windows/Linux 进程生命周期、协议和 API 回归测试；准备真实 Codex 验收说明。

## Capabilities

### New Capabilities

- `local-codex-provider`: 配置与 UI、app-server 子进程和协议、动态工具桥接、结果、取消、用量及兼容行为。

### Modified Capabilities

无。本次不修改未归档的 Linux 适配和 analyzer-history-compaction 变更，也不把它们的任务视为已完成。

## Impact

- `backend/service/analyzer` 新增私有 provider 路由、Codex transport 和工具适配；保留 Service.Analyze 与 cleaner.Analyzer 契约，不新增根目录 Go 文件或独立服务单例。
- 设置默认值、保存校验、首页/设置页及中英文翻译；新增只读能力检测接口时必须由 app.go 薄委托 analyzer 服务，并生成 Wails 绑定。
- 继续使用 Go 标准库和现有依赖；不引入 Node/Python sidecar、MCP 网络服务或第三方 Codex SDK。
- 首期参考兼容版本为已核查的 `codex-cli 0.162.1`；动态工具属于实验协议，其他版本须经能力探测和测试，不能简单声称任意更高版本兼容。
- “本地 Codex”表示本机 CLI 调用，仍使用其配置的联网模型服务；不承诺离线、免费或某账号模型权限。
- 仅生成开发方案。Luna 实施前按用户要求确认必要测试包，再以获准范围执行 TDD；本次不编写应用代码或真实调用模型。
