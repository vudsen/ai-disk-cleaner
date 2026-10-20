# Luna execution guide

本文件是执行入口。当前交付仅为方案，所有 tasks.md 复选框保持未完成；OpenSpec artifacts ready 不代表功能已实现。

## Copyable instruction

下面是可直接交给 Luna 的英文执行指令。使用前无需另行生成方案；执行过程中用中文报告进度即可。

```text
Implement the OpenSpec change at openspec/changes/add-linux-support.

Read the root AGENTS.md, applicable directory AGENTS.md files, and any
resolvable RTK.md first. Then read proposal.md, design.md, both capability
specs, tasks.md, and execution-guide.md in this change directory.

Follow tasks.md in dependency order. Before writing new tests or their
corresponding implementation, obtain the user's approval for the focused
test package described in execution-guide.md unless already approved in
this session. While awaiting approval, perform read-only investigation
and existing baseline checks. Do not interpret silence as approval.

Keep gdu v5.34.0 and Wails v2.12.0. Verify the pinned gdu source before
using filtering, error or cancellation APIs. Preserve the JSON pipeline.
Do not implement direct tree conversion or replace the scan engine.

Work on one functional unit at a time. For each approved code change:
write a focused failing test, capture the expected failure, implement the
smallest complete change, and run the relevant checks. Reuse existing
tests instead of duplicating them. Do not add a frontend test framework
without approval. Preserve unrelated work and other OpenSpec changes.

Keep app.go as thin service delegation. Register the platform service
centrally. Follow native Linux path semantics and preserve Windows
fixtures. Use temporary data and private dependency injection for tests.
Never use real user files for deletion or migration experiments.

Update tasks.md only when the task's implementation and required checks
are complete. Record real commands and results in docs/linux-validation.md
and docs/scan-performance-baseline.md. Distinguish source inspection,
automated tests, native compilation, desktop acceptance, and real-data
benchmarks. Missing access or dependencies are unverified work, not passes.

Do not commit, publish, upload release assets, or archive the change
unless the user separately requests that action. Report completed units,
validation evidence, remaining failures, and the performance decision.
```

## Test approval gate

用户的 AGENTS.md 明确要求写测试前询问；本次只生成文档，不在此时询问，也不把本文当成批准。实施开始时一次性询问这组有价值的测试，不要每个测试都重复确认：

> 本次 Linux 适配涉及挂载点解析、路径大小写、扫描边界与取消、用户数据目录及历史兼容。是否同意新增这些必要 Go 回归测试和扫描基准，复用现有测试并按 TDD 执行？前端默认使用现有静态检查与手工验收，不新增测试框架。

用户同意后，授权覆盖上述任务中的对应必要测试。若用户明确选择其它验证方式，遵循其最新指示并记录该决定；不能暗中添加测试，也不能把手工验证描述为自动覆盖。文档、翻译和工作流配置不需要为了形式新增镜像单元测试。

## Preflight

先读各级指令；`RTK.md` 是根规则中的引用，但方案生成时在仓库及已检查的常用位置没有找到。实施时检查当前目录及祖先目录；存在就阅读，不存在如实记录，不创建该文件。不要把不相关外部文档当成用户命令。

先确认 cwd、git status 和工具版本。Node 从 nvm 获取，Go 从 gvm 获取，依赖用 pnpm；不得硬编码方案生成机器的用户路径。go.mod 的 Go 版本是最低依据，已有更高且兼容版本可先用于调查，最终 CI 以 go.mod 为准。

```bash
git status --short
go version
go env GOMODCACHE GOPATH
node --version
pnpm --version
pkg-config --modversion gtk+-3.0 webkit2gtk-4.1
```

依赖安装涉及当前环境是否已有权限，按执行环境规则处理。Ubuntu 24.04 所需包：`build-essential pkg-config libgtk-3-dev libwebkit2gtk-4.1-dev`。固定 Wails CLI 安装命令：

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@v2.12.0
wails doctor
```

若 PATH 找不到 wails，检查实际 GOBIN 或 GOPATH/bin，不安装另一个版本。网络请求重试超过两次时按用户规则在新 shell 检查并启用实际系统代理；不得猜代理地址。依赖无法获取时记录阻塞，不修改依赖版本绕开。

## Validation commands

以下命令在实施后运行；方案生成阶段无需为文档执行全部应用测试。先按功能单元跑相关包，最后再运行全量。`pnpm lint` 会修复文件，检查 diff 以防无关格式变化。

前端目录：

```bash
pnpm install --frozen-lockfile
pnpm lint
pnpm check
pnpm build
```

仓库根目录，Linux 原生环境：

```bash
git diff --check
gofmt -l backend app.go
go test -tags webkit2_41 ./backend/...
go test -tags webkit2_41 ./...
go test -race -tags webkit2_41 ./backend/service/scanner ./backend/service/cleaner
wails build -platform linux/amd64 -tags webkit2_41 -trimpath
```

`gofmt -l` 必须无新增未格式化文件；已有格式问题记录基线，不能顺手重写全仓库。Race 使用真实支持的 CGO 工具链，尤其验证新增摘要和进度同步；有实际新失败再扩大排查。全量检查不要设置真实 LLM 密钥，涉及真实服务的验收单独执行并记录。

Windows 验收应在 Windows 原生 CI/主机执行 `pnpm check`、`pnpm build`、`go test ./...` 和现有 Wails 构建；Linux 的 GOOS=windows 交叉编译不等价于 Windows 测试通过。`go test -tags webkit2_41` 的标签不得照搬到 Windows。

基准在仓库根目录：

```bash
go test -tags webkit2_41 ./backend/service/scanner -run '^$' -bench '^BenchmarkGDU' -benchmem -benchtime=3x -count=5
go test -c -tags webkit2_41 -o /tmp/ai-disk-cleaner-scanner.test ./backend/service/scanner
/usr/bin/time -v /tmp/ai-disk-cleaner-scanner.test -test.run '^$' -test.bench '^BenchmarkGDUScanEndToEnd$' -test.benchtime=1x -test.count=1
```

RSS 命令应重复至少五次再取中位数，环境、fixture 大小和样本集合必须一致。time 的测量对象为已编译测试进程；不要测 `go test` 编译进程后声称是应用峰值。测试 fixture 的生成也会影响整个测试进程 RSS，报告必须说明；需要隔离扫描峰值时使用预生成只读 fixture 并记录独立采样范围。微基准与 RSS 结果不能混成同一个数据口径。

## Focused coverage map

下表是必须验证的行为，不要求每行创建独立函数；复用 table-driven 测试和现有 TestParseGDUContextHonoursCancellation、TestParseGDUScansDirectoryInMemory、TestDirectoryStatsAndHardLinkDeduplication 等。新增行为先产生失败证据，已有通过的用例不刻意制造失败。

| 行为 | 建议测试位置 | 验证重点 |
| --- | --- | --- |
| 平台入口/集中初始化 | backend/util/*_test.go、backend/service/platform/*_test.go、service_test.go | Linux 有导出实现，manager 持有唯一服务，Windows 不重定义 |
| XDG/用户目录 | backend/util/apppaths_test.go、data/store_test.go、tasklog/tasklog_test.go | 绝对与相对变量、不同 cwd、Windows 兼容、日志删除位置 |
| mountinfo/容量 | backend/util/*linux_test.go | 转义、可选字段、bind 信息、坏行、Bavail/Bfree、失效单项 |
| 大小写/候选匹配 | cleaner、migration、cleaningrecord 对应 *_test.go | Cache/cache 不混淆，迁移 key 不合并，旧行为兼容 |
| 原生/树路径 | model/scanner 与 scanner/scanner_test.go、analyzer/*_test.go | POSIX 反斜杠往返、Windows 夹具、路径前缀及越界 |
| 扫描策略/取消 | scanner/*_test.go | 不误排除普通 proc、同设备 bind、无链接递归、返回后无回调 |
| 统计/部分结果 | scanner、cleaner、data 对应 *_test.go | 硬链接、稀疏文件、EACCES、有限警告及历史重读 |
| 删除边界 | cleaner/cleaner_test.go | 根/越界/排除子树被拒绝，普通临时目标可正常删除 |
| 前端选择/容量/提示 | 现有检查 + 下表手工场景 | Linux 与 Windows 默认值、最长挂载前缀、空列表、双语、摘要 |
| 构建/发布配置 | 原生 CI + 构建产物检查 | 双平台、固定版本、动态库、产物名与校验 |
| 性能 | scanner/benchmark_test.go | 各阶段计时边界、可变输入重建、受控只读样本 |

用户规则给出的 `grep -n "def test_" tests/test_core.py` 是 Python 示例；此仓库为 Go/React，不创建占位 Python 测试。审计时执行等价 Go 检索并人工映射规格：

```bash
rg -n '^func (Test|Benchmark)' backend -g '*_test.go'
```

docs/linux-validation.md 必须列出每个实际测试名、对应规格、命令、结果或未验证原因；文档/CI 条目列静态或集成证据，不编造单元覆盖。

## Manual acceptance matrix

| 场景 | 预期结果 | 环境 |
| --- | --- | --- |
| 普通用户启动 | 不要求 root；默认用户家目录；目录选择可用 | Ubuntu 24.04 amd64 桌面 |
| 从只读目录及不同 cwd 启动 | 同一用户库和日志目录，设置/历史重启后可见 | 临时只读安装目录 |
| 切换目标与目录 | 容量卡对应有效最长挂载前缀，无 C 盘残留 | 含独立 /home 或挂载表夹具 |
| 扫描并立即停止 | 进度停止，终态 CANCELLED，不继续调用 LLM，可重新发起任务 | 受控较大临时树 |
| 权限不足子树 | 普通可读内容保留，警告及计数在历史中可见 | 普通用户 + chmod 000 临时子目录，退出后恢复权限 |
| 链接/硬链接/稀疏文件 | 不递归链接；磁盘占用与逻辑大小口径正确 | 临时树；对比 stat/du 时采用相同边界及单位 |
| 完整 AI 分析 | 工具查询正确路径，候选仅来自扫描结果，错误正常展示 | 用户已有且允许使用的 LLM 配置，勿向日志输出密钥 |
| 用户确认清理 | 只删除明确勾选的临时目标，失败项可见，不越过扫描边界 | 专用临时 fixture |
| 迁移与还原 | 创建链接后可访问，恢复后原数据正确；大小写不同目标互不覆盖 | 专用临时 fixture |
| Windows 回归 | 系统盘、权限提醒、扫描和历史行为正常 | Windows 原生 CI/桌面 |

真实 bind mount 的集成验证仅在已有获准的隔离环境进行；没有 root 时使用 mountinfo 单元夹具并明确注明真实挂载验证未做。禁止为演示直接改变宿主机挂载表。

## Deliverables and stop conditions

- 实现代码、必要测试、Wails 绑定、双语文案、构建配置及双语 README。
- `docs/linux-validation.md`：环境、固定版本源码核查、基线失败、功能/用例对应表、实际检查结果、桌面验收、Windows 状态及剩余限制。
- `docs/scan-performance-baseline.md`：方法、数据集、五次结果及中位数、阶段占比、分配/RSS/缓存口径、是否值得后续优化。
- 所有必需验证完成才把对应 tasks 勾选；缺真实桌面/Windows/LLM 时报告具体未完成项，不把 Linux 整体验收标记为完成。
- 不进行自动提交。若以后明确要求提交，按完整功能单元组织相关代码、测试、绑定和文档；提交信息不含 AI 归属。

遇到需要替换扫描库、升级依赖、fork gdu、扩大平台范围或改变用户数据迁移策略时，先记录证据并提出具体变更，不在当前任务里自行扩大实现。常规私有函数拆分、错误处理和格式修复无需反复确认。
