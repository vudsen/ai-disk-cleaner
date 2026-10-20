## Context

2026-10-20 按当前工作区核查。实施时先重新检查 git status，保留已有 Linux 适配文件和工作流修改；本方案不包含它们的提交或验收。

| 当前位置 | 已核实行为 | 本次影响 |
| --- | --- | --- |
| analyzer/analyzer.go | Analyze 与 TestConnection 只解析 API 配置 | 在现有服务内按 provider 路由 |
| analyzer/agnet.go | OpenAI SDK 驱动多轮工具调用，Agent 持有结果和上下文 | API 循环保留；Codex 使用独立循环/状态 |
| analyzer/tools.go | 三个业务工具 + compress_context；参数/调用依赖 Agent | 抽取业务帮助函数，禁止把 API 上下文压缩工具直接交给 Codex |
| analyzer/config.go | 必填 secret/url/model；解析额外 body、预算、压缩 | 先识别 provider，再校验活动配置 |
| service/setting/validation.go | 严格要求原有七个 key 全部存在 | 新键兼容旧七键请求，验证分支明确 |
| data/models/setting/setting.go | defaults 使用 OnConflict DoNothing | 附加默认键，不覆盖旧值 |
| frontend settings/home | 表单与首页检测均要求 API 三要素 | 同步修改两处，不能只接通后端 |
| cleaner/task.go | 已有 context 取消、onDelta、AnalysisResult 与历史保存 | 保留现有公共契约及状态机 |
| analyzer/log.go、tasklog | 记录工具与可空用量，无返回用量时标为未知 | Codex 日志沿用语义，累计通知不得重复加总 |

本机已只读运行 `codex --version`、`codex app-server --help`、`codex features list`，并生成临时协议 schema；版本为 **0.162.1**。没有读取 auth.json，没有启动分析会话，没有进行真实模型请求。本变更文档不依赖临时文件持续存在，执行前须重生成协议。

## Goals / Non-Goals

**Goals:** Windows/Linux 上，用户能够选择第三方 API 或本地 Codex CLI，使用相同 gdu 文件树、进度界面、候选表与确认清理流程。Codex 模式复用其 CLI 登录及模型配置，支持取消和完整错误反馈。

**Non-Goals:** 离线模型、直接读取 CLI OAuth 令牌调用 HTTP、自动安装/升级/登录 Codex、连接已经运行的桌面聊天、自动回退 API、全盘重新扫描、代理自动删除、重构全部 Agent、实现 Codex 内的项目版压缩工具、Node/Python sidecar 或新 MCP 网络服务。

## Decisions

### D1. 使用本机 app-server，协议版本显式核对

Go 使用参数数组启动 `codex app-server --listen stdio://`。通过有界 JSON 行传输实现双向消息，不用一次性 `codex exec` 文本解析，因为任务需要多次读取应用内存树和响应工具请求。官方 app-server 支持这些交互；动态工具仍为实验协议。

首个可验收兼容基线为 0.162.1，不将所有 `>=0.162.1` 视为兼容。启动前检测 CLI，握手及所需能力失败时返回可解释错误；支持其它版本应补协议夹具与实测记录。不能静默降级成自由 shell 扫盘。

实施前重新执行：

```bash
codex --version
codex app-server --help
codex app-server generate-json-schema --experimental --out /tmp/ai-disk-cleaner-codex-protocol
```

schema 只用于开发核查，运行时不每次生成文件。编写协议 DTO 时只维护实际使用的字段和脱敏夹具，不把整份生成协议复制进项目。

### D2. 配置与旧数据兼容

保留原七个 key，增加以下四个：

| key | 默认值 | 语义 |
| --- | --- | --- |
| llm.provider | api | 只允许 api/codex；旧记录缺失时按 api |
| codex.executable | 空字符串 | 空则从应用进程 PATH 查找；非空必须是一个绝对可执行文件路径，不是命令行 |
| codex.model | 空字符串 | 空则继承 CLI 默认模型；指定时原样传入，不自动换模型 |
| codex.timeout-seconds | 600 | 单次分析总超时，正整数且范围 30–3600 秒 |

`EnsureDefaultSettings` 附加默认行且不覆盖用户值。SaveSettings 仍接受旧七键完整请求：缺少新增键时保留数据库现值，无现值时补默认值；新前端提交全部十一键。拒绝未知键和重复键，不把配置保存改成任意补丁接口。

保存与可运行校验分开：API 模式沿用目前可保存未填凭据的行为，但 Test/Analyze 要求凭据齐全。Codex 模式不要求 llm.secret/url/model，也不校验或转发 API 专用 extra-body、max-token 和 auto-context-compress；保留这些原始值供切回 API。record.max.count 继续通用验证。活动 API 的 JSON/数值验证保持现状，不能误把 API 的原默认值 620000 改为前端 fallback 50000。

`parseProviderConfig` 先识别模式，再调用现有 API 配置解析或新增私有 Codex 配置解析。单任务启动时固定配置快照，任务中保存设置不改变已运行的 provider。错误时不自动从 Codex 切回 API。

### D3. 文件组织与公共边界

在 **现有 analyzer 服务** 中实现，不增加新生产服务：

| 文件 | 职责 |
| --- | --- |
| analyzer.go | Service.Analyze/TestConnection 路由；新增 Service.GetStatus |
| provider.go | 私有 provider 配置分支与统一结果契约 |
| codex.go | Codex 分析流程与 per-task 状态、完成条件 |
| codex_process.go、codex_process_windows.go、codex_process_other.go | 可执行路径、进程/管道及系统差异 |
| codex_protocol.go | 请求匹配、通知和服务器请求分流、最小 DTO |
| codex_tools.go、tool_operations.go | 动态工具适配及从旧 invoke 抽取的共享业务函数 |
| CODEX_SYSTEM.md | 新增英文 Codex 专用系统指令，go:embed |
| 对应 *_test.go 和 testdata/codex/ | 子进程协议夹具、provider 和工具回归 |

所有新增公开服务方法仍放 analyzer.go；其它新文件只包含私有符号。不改 cleaner.Analyzer、AnalysisResult、StartCleaning 和 DeleteTrashFiles 签名，不搬迁 agnet.go 来夹带拼写重命名。测试使用包内私有 process factory，不能增加生产全局 fake。

`app.go` 新增 `GetLLMStatus() (analyzer.ProviderStatus, error)`，只调用 ready 和 analyzer.GetStatus；原 TestLLMConnection 签名不变。ProviderStatus 包含 `{provider, ready, code, version}`，code 为 `ready|config_missing|executable_not_found|cli_unsupported|not_authenticated`。协议或读取异常返回 error；ready 只代表本地启动前提满足，不代表账号具有模型额度。

### D4. 进程与认证

- 每次 Analyze/Test/GetStatus 使用自己拥有的短生命周期子进程；首期不做常驻共享进程或跨任务线程续用。启动和握手上限 10 秒；普通 RPC 默认 15 秒；分析总超时按配置；TestConnection 总超时 30 秒。
- CLI 路径可以包含空格。用 exec.LookPath/明确路径解析后执行，不拼接 shell 命令。不得传入 `sh -c`、PowerShell 表达式或用户自定义参数串。
- Windows 特别处理 `.cmd/.bat` 包装器：不得假设 exec.Command 可以直接执行 npm/pnpm shim；优先解析或要求用户选择包内原生 codex.exe，明确提示。不能为了方便自动启用命令字符串执行。Linux 可运行有效 shebang 包装器。
- 使用全新临时工作目录，cwd 不指向扫描根、源码仓库或用户家目录，避免加载扫描目录中的项目指令。不修改 CODEX_HOME 或全局 config.toml；由 CLI 自己使用其现有认证和 provider 配置。不给 CLI 注入本应用保存的 llm.secret/url。
- GetStatus 用同一 transport 初始化后请求 account/read，不输出账号邮箱、令牌或完整配置；根据 requiresOpenaiAuth 与 account 判断是否缺认证，不把不需要 OpenAI 认证的自定义 CLI provider 一律判为未登录。只有 TestConnection 的实际推理完成才证明当前配置可调用。
- 用户未登录时提示其在终端完成 `codex login` 后重试；不弹浏览器替用户登录，不自动执行 login/logout，不复制认证文件。
- Linux 停止拥有的进程组；Windows 用拥有的 Job Object 或经验证的等价方案终止本任务进程树。不得按进程名杀死其它 codex；确保 stdout/stderr 消费、Wait、临时目录回收只有一次。

### D5. 限制为应用业务工具

本应用的流程是 gdu 扫描后让 LLM 阅读内存树；因此 Codex 不应使用其通用 shell、文件编辑、外部 MCP/插件或子代理代替这个流程。

- thread/start 设只读沙箱，临时 cwd；基线 schema 的字符串为 `sandbox: "read-only"`，不要误用 turn/start 的 `sandboxPolicy.type: "readOnly"`。
- 本机 schema 支持 thread/start 与 turn/start 的 `environments: []` 以禁用环境访问；结合已核实的进程级 features（shell_tool、unified_exec、apps、plugins、multi_agent）关闭内置通用执行与扩展能力，web_search 关闭。
- 对用户配置中启用的外部 MCP，通过会话配置覆盖逐项禁用；空对象是否覆盖/合并必须实际核实，不能假设 `mcp_servers={}` 已禁用所有继承项。不修改原配置、不记录配置中的密钥。
- **实施门槛**：验证有效会话只提供三个应用业务工具，无法写文件或调用用户外部 MCP。若 0.162.1 无法实现所需禁用组合，记录缺口并调整集成设计后再继续；只读沙箱或提示词本身不是外部工具禁用的证明，不以“允许所有权限”绕过。
- 意外收到命令/文件修改/权限批准请求，按该版本 schema 拒绝并终止该次分析，给出明确错误。未知 server request 以受控错误回复，不能悬挂等待；不新增通用批准弹框。

### D6. 协议状态机

本机 0.162.1 生成 schema 已核实下列字段；实现必须有对应协议夹具：

1. initialize 携带稳定的 clientInfo 与 capabilities.experimentalApi=true；收到成功响应后才发 initialized。
2. account/read 检测认证；必要时 model/list 仅用于诊断，不新增模型选择器依赖，也不把列表当成可用额度证明。
3. thread/start 创建 ephemeral=true 的新线程，传入可选 model、英文 baseInstructions/developerInstructions、会话限制和 dynamicTools。每个 dynamic tool 使用 `{type:"function",name,description,inputSchema}`；不复用 OpenAI Chat Completions 的 tools 包装格式。
4. 收到线程 id 后 turn/start，input 为文本 user input，明确用户语言、扫描摘要及“只通过应用工具查看目录”；初始只给根层必要信息，不把整个树一次塞进 prompt。
5. 持续读取 stdout。响应按 id 路由；无 id 为通知；同时有 method 和 id 的消息为服务器请求，不能误判为响应。请求 id 与工具 callId 分开保存。
6. `item/tool/call` 按 threadId/turnId/tool/callId 关联；arguments 是 JSON 值，校验后再编码给业务帮助函数，不假设它是已经编码的字符串。返回同一个请求 id，result 为 `{success,contentItems:[{type:"inputText",text:...}]}`，不是 OpenAI ToolMessage。
7. `item/agentMessage/delta` 追加现有 onDelta 和最终文本，按线程/turn/item 过滤。item/completed 的完整内容只作一致性补齐，不能再重复追加已经流出的全文；不显示推理内部事件。
8. 只有当前 turn 的 turn/completed.status 为 completed 才成功；failed/interrupted、EOF、进程非零退出、未完成即退出均不成功。error 通知结合是否仍会重试和最终终态处理，不能收到一条可恢复错误就错误宣布完成。

实现一个 stdout reader 和一个串行 writer；RPC pending 表保护请求状态，业务工具经串行队列执行，避免结果 slice 并发修改。不能在 reader 上同步等待工具 RPC 导致读取死锁。重复 callId 返回缓存结果而不重复变更候选。未知通知可忽略，未知请求必须回复。

最大单帧 8 MiB，最大单次工具参数/结果各 1 MiB；达到上限明确失败，不静默截断 JSON。stderr 独立持续消费，最多保留 64 KiB 脱敏诊断尾部，不和 stdout 混用；不直接向用户或日志倾倒原始协议/认证响应。传输、工具与输出队列有界，取消能够解除阻塞。

### D7. 复用业务逻辑，隔离上下文管理

Codex 只注册 `analyze_directory`、`add_trash_file`、`add_top_usages`。将旧工具中的目录查询/CSV 格式和候选合并规则抽成私有帮助函数，由 API invoke 和 Codex 适配器复用；原工具接口继续接收 Agent，避免本次重写 API Agent。

- analyze_directory 保留原有最多 200 项、目录大小排序和根感知路径语义；只读 FileTree，不运行 os.ReadDir 或 COM 扫盘。
- Codex 工具的路径明确是文件树路径，如根为 `/home/alice`，`/Downloads/a.zip` 表示树中相对项。宿主用 FindNode 找到节点，将候选持久化为节点 RelativePath，并用节点 DiskSize 校准大小；禁止把它当成真实系统绝对路径 `/Downloads/a.zip`。
- 在规范化之前拒绝 `..` 段；拒绝不存在节点、扫描根自身、glob 和非普通/链接候选；保留大小写与合法 POSIX 反斜杠。顶层占用展示允许树根，但清理候选不允许。Tool schema 对路径和风险等级给出精确限制；不能让 LLM 设置 isDeleted=true。
- 批量调用全部校验成功后才写入本任务结果；复用候选去重、父子路径处理和 30 项上限。工具失败返回 success=false 的可解释结果，不直接改变历史数据库或真实文件。
- 本次严格路径适配限定在 Codex 分支；若核查发现 API 分支也存在树/系统路径混淆，单独记录回归及后续修复，不在保持兼容的重构中悄悄改变它。
- Codex 使用其自身上下文管理，不注册 compress_context，也不套用 API Agent.messages 重置、IsSupport 状态切换或 token 预算。API 的自定义压缩模式和当前 prompt 不变。
- 新建英文 CODEX_SYSTEM.md，保留现有候选风险分级、精确边界、受保护系统/应用数据、最多 30 项、父子去重、虚拟磁盘只展示、文件名为数据等业务规则；明确只标记、不删除及目标语言。不得把现有中文 SYSTEM_FAST.md 当成新英文系统指令直接拼接。

### D8. 输出、用量与取消

Codex 维护独立 per-task 状态，成功时产出同一 AnalysisResult。候选来自成功的业务工具调用，不能从自然语言或模型最后一段自造 JSON 反解析为候选。正常结束且无候选允许返回空列表；异常不能借部分候选伪装 DONE。

Token 用量来自 thread/tokenUsage/updated 的 tokenUsage.total。它是线程累计快照，不把每条 total 相加；按线程保存最新合法值，相同快照忽略。tasklog.Round 若要记录增量，只记录相邻累计快照差值且不重复写入轮次合计。新任务/健康检测不共享统计。没有用量时日志为未知，AnalysisResult.TokenUsage 沿用现有整数零占位，不将其描述为实际零成本；不增加虚构费用字段。

取消或超时后先停止接受新的业务变更，已知 turn id 时请求 turn/interrupt，给 2 秒退出宽限，再终止本次拥有的进程树并 Wait。响应取消和 context.DeadlineExceeded 分别归为原有 CANCELLED/ERROR，不把超时说成用户取消。清理 context 不复用已经取消的 ctx，确保 interrupt 有机会发送。返回前停止输出回调；应用 shutdown 沿现有 cleaner.Close 触发并等待同一流程。

### D9. 设置页、首页与连接检测

- 设置页新增 API/本地 Codex 选择。API 显示现有字段；Codex 显示可执行路径、可选模型、超时和“使用已安装 CLI/已有登录”的简短说明。隐藏字段保留值，react-hook-form trigger 只校验活动分支，错误用 toastError。
- 复用已有 HeroUI/ControlledNextUIFormWrapper 和 Tailwind，不引入 UI 框架或前端测试框架。新增文案及后端错误 key 同时提供 en/zh_CN，用户界面不暴露 JSON-RPC/schema 等实现术语。
- 首页去掉“必须存在 llm.secret/url/model”的固定判断，统一调用 GetLLMStatus；模式未配置/CLI 不存在/未登录分别提示设置或终端操作。GetStatus 不调用模型，不能把保存设置或本地探测记为连接成功。
- TestLLMConnection 使用**当前未保存表单**按 provider 测试。API 保留现状；Codex 用短生命周期线程发最小请求，只有实际 completed 且有非空最终消息才返回现有 success 结构，不修改保存值、不触发磁盘扫描。
- 测试按钮请求期间禁用重复点击；服务端每次探测有独立上下文及超时，结束后无残留进程。切换/保存与分析互不共享可变配置；不把用户已有 API secret 注入 Codex。
- 桥接更改后通过固定 Wails 版本实际生成绑定，再做前端检查。新 public API 只为状态查询，不暴露任意命令执行接口。

## Risks / Trade-offs

- [动态工具协议实验性] → 以本机 schema+夹具+真实任务核对；记录实际支持版本，未知不兼容版本明确失败。
- [CLI 继承通用工具配置] → D5 作为实现门槛，验证有效工具隔离；不能仅靠提示词声明只读。
- [Windows PATH 与包装器差异] → 原生 exe 路径和进程树测试单列，不能用 Linux 单测替代。
- [上下文管理与 API 不同] → UI 标明 API 专用字段；保留原 API 循环，不伪装两者的预算/压缩完全等价。
- [工作区有正在进行的 Linux 变更] → 只改本功能必要调用点，不覆盖未跟踪文件；原有失败单独记录，不顺便推进 Linux 发布。

## Migration Plan

先通过工具帮助函数回归，再增加配置与 Codex transport，最后接入 UI。仅新增设置行，不删除 API 配置、不修改业务结果表结构。回退代码时保持设置数据和历史，新增键不影响老数据库读取；旧前端保存时忽略新增键的方案需有测试证据。

## Open Questions

- 编写测试前需按根规则取得必要 Go 测试包的批准；计划本身不是批准。
- D5 中有效工具隔离必须在实施初期实测；如果运行环境策略不支持，先报告具体缺口，不擅自换成 MCP/exec 架构。
- 真实登录、模型权限及 Windows 验收依赖对应用户环境；未实测的项目保留未完成状态，不要求用户现在提供密钥。

## References

- [官方 app-server 协议](https://learn.chatgpt.com/docs/app-server)：stdio、初始化、工具请求、通知与取消的设计依据。
- [官方非交互模式](https://learn.chatgpt.com/docs/non-interactive-mode)：用于与一次性任务集成方式比较。
- 本机生成 schema：ThreadStartParams、TurnStartParams、DynamicToolCallParams/Response、InitializeParams、ThreadTokenUsageUpdatedNotification，来自 codex-cli 0.162.1；只在临时目录检查，未复制任何认证配置。
