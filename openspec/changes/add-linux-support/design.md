## Context

本文面向执行模型 Luna。先读 `proposal.md`，再读本文、两个 `specs/*/spec.md` 和 `tasks.md`；操作顺序见 `execution-guide.md`。所有路径相对仓库根目录。

2026-10-19 的源码核查结果如下；这是静态核查，不代表已完成 Linux 编译或真实目录验收。

| 位置 | 当前事实 | 执行要求 |
| --- | --- | --- |
| `go.mod` | Go 1.25.0；gdu v5.34.0；Wails v2.12.0；已有 x/sys | 保持版本，禁止用升级依赖代替修复 |
| `backend/util/disk_other.go` | `!windows` 分支只有小写 `listDisks`；引用 `model` 却未导入 | 为 Linux 提供与 Windows 一致的导出函数，修复其它平台 fallback |
| `backend/util/administrator_other.go` | 只有小写函数且固定返回 true | Linux 用真实有效 UID；其它平台显式 fallback |
| `app.go` | `GetDisks`、权限方法直接调用 util | 相关方法改为调用 platform 服务，遵守根目录规则 |
| `frontend/src/pages/home/index.tsx` | 默认 target 与 systemDisk 写死 `C:\\` | Linux 默认用户家目录；容量卡使用所选目标所在挂载点 |
| `backend/service/scanner/scan.go` | AnalyzeDir → UpdateStats → EncodeJSON → parseGDUJSON | 首期保留这条生产流水线，并独立测量各阶段 |
| `backend/model/scanner/scanner.go` | NormalizeTreePath 总是把反斜杠转为 `/` | POSIX 树保留合法反斜杠文件名；Windows 旧格式兼容 |
| `backend/service/scanner/parser.go` | 仅以 inode 去重硬链接；路径风格由字符串猜测 | 默认单挂载点扫描；路径风格按树根确定 |
| cleaner/path.go、migration/{migration,steps}.go、data/models/cleaningrecord/cleaningrecord.go | 部分路径比较无条件 EqualFold/ToLower | Linux 保留大小写；Windows 保留现有大小写不敏感行为 |
| `backend/service/analyzer/tools.go` | 已按 GOOS 处理大小写，但 analyze_directory 无条件拒绝反斜杠 | 只修补树路径风格的入口；复用已有正确逻辑 |
| `frontend/src/pages/cleanup/[id]/TrashFileTable.tsx` | 路径显示将两类斜杠都视为分隔符 | POSIX 根下保留文件名中的反斜杠 |
| `backend/data/store.go` | 实际使用 `./data.db`，注释与实现不符 | Linux 改为用户数据路径，禁止依赖 cwd |
| `backend/service/tasklog/file.go` | 日志在 exe 旁边 | Linux 使用用户状态路径，并同步日志删除逻辑 |
| `.github/workflows/{build,release}.yml` | 测试和发布主流程为 Windows | 增加 Linux 原生任务，保留 Windows 任务 |

本机未缓存可直接检查的 gdu v5.34.0 模块源码，版本文档获取也未成功；本文不假定任何尚未核实的 gdu 扩展 API。执行阶段的 0.3 是必须完成的依赖核查门槛。

## Goals / Non-Goals

**Goals:**

- 普通 Linux 用户在 Ubuntu 24.04 amd64 桌面上，可以启动、选择目录、扫描、取消、分析、确认清理、迁移和查看历史。
- Linux 文件名和路径语义正确；Windows 现有功能和 JSON 夹具继续通过。
- 提供可重现的扫描阶段耗时、分配和端到端内存报告，明确下一步优化是否有依据。

**Non-Goals:**

- 本次不引入新扫描库、不 fork gdu、不升级 Wails、不重做 UI、不修改 LLM 分析策略或已有上下文压缩变更。
- 不实现直接读取 ext4/XFS 元数据、后台索引守护进程、增量扫描、全系统权限提升或自动删除。
- 不制作 deb/rpm/AppImage，不支持 macOS/ARM64，不承诺通用 Linux 静态单文件运行。
- 不实施 JSON 到直接树转换优化；性能结果只能成为后续变更的依据。

## Decisions

### D1. 复用 gdu，先核实固定版本接口

首期继续嵌入 gdu v5.34.0，不调用外部 `gdu` 命令。先用 `go mod download github.com/dundee/gdu/v5` 取得固定版本，查看模块缓存中的 `pkg/analyze`、`pkg/fs`、Linux stat 和 JSON 编码实现，记录：

1. `AnalyzeDir` 两个过滤回调的参数含义、调用时机和返回值语义。
2. 默认符号链接行为、读取错误的暴露方式、进度通道关闭与发送行为。
3. stat 信息是否包含设备号、inode、分配块数；硬链接与稀疏文件如何计量。
4. `UpdateStats` 与 JSON 导出实际字段，特别是错误、排除、硬链接标记。

将实际符号名及证据位置记录到 `docs/linux-validation.md`。生产代码不得引用仅在新版出现的 API。若现有 API 无法表达必要边界，先描述最小适配缺口；不得擅自 fork、替换库或谎称已支持。

### D2. 平台服务与桥接契约

新增 `backend/service/platform/platform.go`，只在该文件公开 `Service`、构造器和服务方法；内部实现在同目录其它文件。服务由 `backend/service/service.go` 初始化、持有并暴露 `GetPlatformService()`；同步 service_test。

公开服务方法固定为：

- `ListDisks() ([]model.DiskInfo, error)`：委托 util。
- `IsRunningAsAdministrator() bool`：Windows 沿用现有实现；Linux 为 `os.Geteuid() == 0`。
- `GetInfo() (model.PlatformInfo, error)`：返回 `{os, homeDirectory, defaultScanPath}`；Linux 的 defaultScanPath 为有效的用户家目录，Windows 为系统卷根路径，其他平台返回明确不支持错误。

新增 `backend/model/platform.go` 定义 PlatformInfo。`app.go` 的 `GetDisks`、`IsRunningAsAdministrator` 改为薄委托，新增 `GetPlatformInfo() (model.PlatformInfo, error)`；返回 error 的方法先执行已有 `ready()`。bool 方法不改签名，遵循服务管理器未初始化时 panic 的既有约定。禁止新建根目录 Go 文件。

util 拆分为 `disk_windows.go`、`disk_linux.go`、`disk_other.go`；other 标签改为 `!windows && !linux`，导出名称和 import 正确。权限文件同样拆分，确保 build tags 不重叠。测试依赖通过包内私有帮助函数注入，不新增生产全局 fake。

### D3. Linux 挂载点与容量

- 读取 `/proc/self/mountinfo`；解析时先按 ` - ` 分左右字段，再解码挂载路径的 `\040`、`\011`、`\012`、`\134`，不得先对整行解码再按空白拆分。
- 独立纯解析函数接受 `io.Reader`，覆盖可选字段、空格、反斜杠和坏行。单条坏行跳过；整个读取失败返回 error；无需执行 `lsblk`、`df` 或 shell。
- 首页列表以挂载路径去重，按路径稳定排序，`/` 置首。保留有效根挂载，即使文件系统是 overlay。排除其它 overlay、proc、sysfs、devtmpfs、devpts、tmpfs、cgroup/cgroup2、securityfs、debugfs、tracefs、configfs、pstore、mqueue、hugetlbfs、autofs、bpf、rpc_pipefs、fusectl、binfmt_misc、nsfs 和 squashfs；网络盘不纳入首期容量列表，用户仍可显式选目录且无本地性能承诺。
- 通过 `unix.Statfs` 获取容量，`total = Blocks * Bsize`，`free = Bavail * Bsize`，`used = (Blocks - Bfree) * Bsize`，所有操作校验符号与下溢。保留块存在时 `total != free + used` 是允许的。磁盘名使用 `source (mountpoint)`，path 必须为真实可访问挂载路径。
- 某个挂载点在枚举过程中消失或 statfs 失败时跳过该项；仍返回其它有效项。没有有效项时返回空数组，首页仍可选择自定义目录。
- 挂载表解析供扫描策略复用；扫描策略使用完整挂载表，不能用已过滤的首页列表代替。

### D4. 首页及权限提醒

- GetPlatformInfo 成功后，Linux 初始化 `target='directory'`、`directory=homeDirectory`；Windows 初始化 defaultScanPath 对应的磁盘，有效时优先，否则第一块有效磁盘，否则自定义目录。
- 加载期间不提交扫描；异步加载只初始化一次，不能覆盖用户已修改的选择。平台信息失败显示 toast，保持自定义目录可用，不能回退到猜测的 `C:\\`。
- 首页容量卡根据当前目录路径的最长挂载点前缀匹配；前缀必须在路径分隔符处结束，`/home` 不能匹配 `/home2`。Windows 使用平台路径语义。无匹配时显示无容量信息，而非 0% 或 C 盘。
- Layout 仅在 `os == 'windows'` 时使用原有管理员提醒。Linux 普通用户不因 UID 非零而弹出提权提示，也不自动启动 sudo/pkexec。
- 复用 HeroUI/ControlledNextUIFormWrapper/react-hook-form/Tailwind；这是现有 UI，通用“新 React 项目默认 Ant Design”不构成迁移要求。新增文字同步 en、zh_CN；操作错误使用 toastError。

### D5. Linux 扫描边界与部分结果

- 扫描 `/` 时跳过绝对路径 `/proc`、`/sys`、`/dev`、`/run` 及其后代；不能将用户目录内名为 `proc` 的普通目录误排除。直接选择这些系统路径时返回清晰错误。
- 默认扫描一个挂载点。在完整 mountinfo 中找到当前根的所属挂载，再排除根目录下所有嵌套挂载点，包含同设备 bind mount；不能只比较 st_dev。用户显式选择 `/home` 等另一个挂载点时，该路径可作为新的扫描根。
- 不跟随目录符号链接；链接项保留非普通文件标记，FIFO/socket/设备项不读取内容。硬链接节点保留，容量汇总按单文件系统 inode 去重；稀疏文件区分 apparentSize 与 diskSize。
- 无效根、根无权限、mountinfo 读取失败属于整次扫描失败；子目录 EACCES 或扫描中消失的条目产生部分结果和警告，不让整个应用崩溃。策略排除与读取错误分别计数。
- 增加 `modelscanner.ScanSummary`：`excludedCount`、`readErrorCount`、`warnings`；warning 为 `{path, code}`，code 固定为 `permission_denied`、`entry_disappeared`、`read_error`，最多保存 100 条明细，计数保存全部。策略排除只计数，不逐条刷警告。
- FileTree、CleaningTaskSnapshot 和 CleaningRecord 增加 scanSummary；历史采用可空 JSON 列及 serializer，旧记录 nil 按无警告处理。模型方法单独保存扫描摘要，不让后续 CompleteCleaningRecord 覆盖该字段。前端详情页在活动与历史记录中显示“不完整扫描”及跳过数量；日志记录同一摘要，禁止把部分扫描描述为全盘完整统计。
- 采用现有 gdu 错误标记/项目侧轻量适配取得摘要，不能再完整遍历一遍目录收集错误。摘要与扫描策略在并发回调下必须安全。
- 取消必须贯穿扫描、汇总、编码、解析；扫描返回前等待项目启动的进度消费者结束，保证返回后无回调。确认通道是否关闭，避免读取关闭通道导致空进度循环。无法中断的底层调用需在前后检查 context 并记录限制，不承诺固定毫秒级取消。
- 对包含排除子树、嵌套挂载或符号链接路径分量的候选，不允许通过父目录递归清理绕过扫描边界：在 cleaner 删除前复核候选及目录边界，拒绝不受支持的项并返回现有 DeleteFailure，要求选择准确子项。测试仅在临时树中执行。

### D6. 路径风格和受影响调用方

原生文件操作统一用 filepath；LLM 树路径仍以 `/` 表示层级。Windows 树根识别限定为盘符绝对路径或 UNC；POSIX 树根以 `/` 开头，其名字中的 `\` 是普通字符。不能根据某个文件名是否含反斜杠判断整个树风格。

- `FileTree.FindNode` 按 RootPath 的树风格规范化；保留旧 NormalizeTreePath 导出函数兼容现有调用，生产树查询改为根感知入口。Windows JSON 夹具在 Linux 上仍须通过。
- parser 的 baseName/joinDisplayPath、analyzer 的反斜杠校验、前端 pathName/resolvePath 同步采用根风格。POSIX 名字 `a\b.txt` 必须能扫描、工具查询、显示并定位同一文件。
- cleaner.samePath、migration.samePath、migrationStepKey、cleaningrecord.trashFileMatchesSource：Linux 不 EqualFold/ToLower；Windows 保留现状。若抽取公共帮助函数，只放 backend/util 内，禁止引入 service/data 循环依赖。
- 回归样本包括 `Cache` 与 `cache`、中文与含空格路径、反斜杠文件名、`/home` 与 `/home2`、`../` 越界、扫描根自身、目录符号链接和嵌套挂载；比较以真实目录和结果为准。
- 扫描单挂载点是本次硬链接 inode 去重的前提。如果未来允许跨文件系统，必须先将身份改为 `(device, inode)`；本次不得悄悄允许跨挂载扫描。

### D7. Linux 数据和日志位置

新增 `backend/util/apppaths.go` 及 Linux/Windows/other 平台实现，所有路径先解析再显式创建，不能 chdir 或依赖当前工作目录。

- Linux 数据库：`$XDG_DATA_HOME/ai-disk-cleaner/data.db`；变量为空或非绝对路径时用 `$HOME/.local/share/ai-disk-cleaner/data.db`。
- Linux 日志：`$XDG_STATE_HOME/ai-disk-cleaner/logs/`；变量为空或非绝对路径时用 `$HOME/.local/state/ai-disk-cleaner/logs/`。目录用 0700，日志用 0600；创建数据库前保证应用数据目录私有，避免暴露 LLM 设置。
- HOME 无效且无法取得用户家目录时返回错误，不回退 cwd。数据库创建失败使启动失败；日志打开失败维持现有降级行为。
- Windows 保留 `./data.db` 与 exe 旁 logs，确保旧数据可见。tasklog.Open/Remove、cleaninghistory 清理统一使用新解析器，不能只改写入。
- Linux 不自动移动或删除既有 cwd 数据。README 解释旧试用版用户可在关闭应用后手动备份/迁移；新位置存在时不覆盖。

### D8. 构建和交付

- Linux CI 使用固定 `ubuntu-24.04`，安装 `build-essential pkg-config libgtk-3-dev libwebkit2gtk-4.1-dev`，保持 CGO_ENABLED=1；Wails CLI 固定 v2.12.0，使用 `-tags webkit2_41`。先运行 wails doctor，缺依赖按实际结果修正，不能通过关掉 CGO 规避。
- 测试、完整 Go 构建与 Wails 构建使用一致的 webkit2_41 标签。后端测试不需要启动 GUI；真实桌面验收单独记录。
- CI 保留 Windows 测试，新增 Linux 后端测试、前端检查及 Linux Wails 构建产物。改 public API 后通过 Wails v2.12.0 的绑定生成命令/构建生成绑定，不能手工伪造 JS/TS 签名。
- 发布工作流保留 Windows job，增加独立 Linux job；产物命名 `ai-disk-cleaner-v<version>-linux-amd64.tar.gz`，内含可执行文件、Linux 启动说明，附 sha256；沿用 project.json 与 tag 一致性检查。仅修改工作流，不创建或发布 Release。
- README 清楚区分动态依赖与已验证发行版；采用用户目录解包运行，不宣称任意 Linux 下载即可运行。日志/数据库不会写入只读安装目录。

### D9. 先测量，再决定性能优化

增加 `backend/service/scanner/benchmark_test.go`，只提供包内基准与 fixture，不增加 app.go 生产接口或线上开关。

| 基准名称 | 计时范围 | 固定条件 |
| --- | --- | --- |
| BenchmarkGDUAnalyze | AnalyzeDir | 相同目录、边界策略、并发配置；不含数据生成 |
| BenchmarkGDUUpdateStats | UpdateStats | 使用每次独立的未汇总树；准备阶段不计时，不能重复汇总同一可变对象 |
| BenchmarkGDUEncodeJSON | 现有完整 JSON 报告写入 bytes.Buffer | 输入是已汇总树；输出不能换成 io.Discard |
| BenchmarkGDUParseFileTree | parseGDUJSON 含内部 aggregateDirectoryStats | 固定合法报告；不能把解析内部汇总遗漏掉 |
| BenchmarkGDUScanEndToEnd | 生产 scanGDUContext | 与上面同一语义，不调用 LLM |

另在一次端到端样本中采集真实阶段时长，避免把不同运行的 benchmark 均值强行相加。报告同时给出 ns/op、B/op、allocs/op、条目数、输入 JSON 大小、单进程峰值 RSS；heap profile 与进程峰值 RSS 分开说明。

默认合成样本：200 个目录、每个 100 个 1 KiB 文件，加少量深层目录、硬链接、符号链接和稀疏文件，总量受控；生成不进入计时，输出到临时目录。可选真实样本通过 `AI_DISK_BENCH_ROOT` 显式传入，访问只读，不扫描 `/`、不调用删除或真实 LLM。各微基准使用等价 fixture，端到端可另测真实根。

常规运行 `-benchmem -count=5`，对同一数据至少一次预热；记录 CPU/内存、SSD/HDD、文件系统、Go/gdu 版本、并发数和缓存状态。默认测暖缓存，不执行全机 drop_caches，也不把暖缓存结果称作冷扫描。真实数据缺失时如实说明，禁止编造吞吐量。

输出 `docs/scan-performance-baseline.md`。决策规则：同一受控样本下 JSON 编码加解析占端到端中位时长至少 20%，或内存剖析表明中间 JSON/双树显著主导峰值内存，才建议独立创建直接树转换变更；否则优先调查遍历/存储瓶颈。20% 是项目内部决策阈值，不是性能保证。任何结果均不能推导其它扫描库一定更快。

## Risks / Trade-offs

- [固定 gdu API 与预期不符] → 先完成 D1 核查与最小原型；不能完成边界则保持相关任务未完成并说明缺口。
- [bind mount 或动态挂载变化] → 用完整 mountinfo 快照过滤并在删除前复核；不承诺抵御并发恶意文件系统修改，实测中记录限制。
- [权限错误测试用 root 执行而失真] → 真实 EACCES 验收使用普通用户；root 下相应测试显式 skip 并给原因，不能当通过。
- [Wails 动态依赖及 headless 环境] → 区分原生编译成功、GUI 启动成功和真实目录验收成功。
- [路径修复影响 Windows 夹具] → 每个功能单元跑现有用例和 Linux 新用例；CI 同时保留 Windows。
- [全量文件树内存随条目数增长] → 记录峰值，不在无证据时更换引擎；后续优化另开变更。

## Migration Plan

按 tasks.md 顺序完成平台入口、用户数据路径、挂载点、路径语义、扫描策略、UI、构建及测量。新增持久化字段为可空附加列，先用旧数据库副本验证升级；不得在真实数据库试验回滚。Linux 新存储位置不自动搬迁旧文件。出现回归时按功能单元回退，保留用户数据库与日志。

## Open Questions

- 实施前必须确认必要测试包是否获准；这是用户 AGENTS.md 的明确要求，文档生成不视为已经批准。
- gdu v5.34.0 的错误暴露及取消细节需要在任务 0.3 现场核实；允许据证据调整私有实现，但不改变本文行为契约。
- 本期默认目标和发布范围已明确，无须再询问发行版、扫描库或 UI 框架。

## References

- [gdu 官方仓库](https://github.com/dundee/gdu)：Linux 支持与并行扫描背景；具体接口以 go.mod 固定版本源码为准。
- [Wails v2 安装文档](https://wails.io/docs/gettingstarted/installation/)：Linux GTK/WebKit 依赖与 webkit2_41 构建标签。该页面是滚动文档，实际构建仍须核验 v2.12.0。
