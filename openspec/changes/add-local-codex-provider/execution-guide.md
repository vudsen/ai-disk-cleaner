# Luna execution guide

当前只生成方案；所有任务尚未实施。先读 proposal、design、specs，再按 tasks 顺序执行。本方案已有技术选择，不需要 Luna 重新选扫描库、SDK 或 UI 框架。

## Copyable instruction

```text
Implement openspec/changes/add-local-codex-provider.

Read the applicable AGENTS.md files and any available RTK.md, followed by
this change's proposal.md, design.md, specs, tasks.md and execution-guide.md.
Preserve all existing work, especially the ongoing Linux support change.

First obtain the focused test approval described below unless that scope
is already approved in this session. While approval is pending, perform
read-only investigation and existing baseline checks. Do not treat silence
or the planning document as approval to write tests or implementation.

Verify the locally installed Codex version and generate its experimental
app-server schema. The reviewed baseline is 0.162.1. Verify the effective
tool restrictions in design D5 before building the remaining integration.
If they cannot be enforced, report the exact gap and stop dependent work;
do not silently switch to unrestricted shell execution or another design.

Implement one functional unit at a time using approved TDD. Keep the
existing API provider, compression behavior, cleaner contract and stored
API values compatible. Use a Go stdio app-server client with only the
application's three business tools. Do not read authentication tokens,
install Codex automatically, or modify the user's global Codex config.

Reuse private business helpers. Use separate Codex task state and resolve
tree paths through FileTree before storing relative candidate paths.
Do not expose deletion or arbitrary command execution to Codex.

Add provider-aware settings and home checks, generate Wails bindings, and
provide English and Chinese UI text. Keep all new system instructions in
English. Do not introduce a Node/Python sidecar or a frontend test framework.

Test transport and process cancellation with controlled fake subprocesses.
Keep real authenticated Codex acceptance separate and run it only with
the user's allowed configuration and temporary fixtures. Record evidence
in docs/local-codex-validation.md. Do not mark unverified tasks complete.

Do not commit, publish, or archive changes unless separately requested.
Report completed tasks, exact validation, remaining gaps and real-Codex
acceptance status in Chinese.
```

## Test approval

实施时按用户“写单元测试前先询问”的规则，一次性询问：

> 是否同意为本地 Codex 接入增加必要 Go 测试：配置兼容、协议/工具桥接、进程取消与超时、输出/用量，以及现有 API 回归？前端复用现有检查和手工验收，不新增测试框架。

批准后无需每个用例反复询问。用户若明确选择其它验证方案，按其最新指示记录范围；没有批准不能擅自开始测试及依赖实现。当前只写文档，无需为文档新增单元测试。

## Initial checks

根目录引用了 RTK.md，但本次仓库文件检索未找到。实施时检查仓库和祖先路径，找到则读取，找不到如实记录，不创建占位文件。

```bash
git status --short
go version
node --version
pnpm --version
codex --version
codex app-server --help
codex app-server generate-json-schema --experimental --out /tmp/ai-disk-cleaner-codex-protocol
```

Go 由 gvm、Node 由 nvm 管理，使用 pnpm；CLI 路径来自当前环境，不能复制方案生成机器的绝对路径。不要运行模型请求、读取 auth.json 或把全局 config.toml 输出到诊断文档。旧 Linux 方案或未跟踪文件不是“可以覆盖”的授权。

## Required validation

按功能单元先跑相关测试，全局检查在最后完成。Linux 根据当前项目环境使用 webkit2_41 标签；Windows 不携带 Linux 标签。不要为了这份计划更新依赖。

仓库根目录，Linux：

```bash
git diff --check
gofmt -l backend app.go
go test -tags webkit2_41 ./backend/service/analyzer ./backend/service/setting ./backend/data/...
go test -race -tags webkit2_41 ./backend/service/analyzer ./backend/service/cleaner
go test -tags webkit2_41 ./...
wails build -platform linux/amd64 -tags webkit2_41 -trimpath
```

frontend 目录：

```bash
pnpm install --frozen-lockfile
pnpm lint
pnpm check
pnpm build
```

根包嵌入 frontend/dist，需要先构建前端再运行全量根包检查；绑定发生变化时先生成再检查前端。gofmt 输出新增问题必须清零，lint 自动修复后的无关变化不要夹带。

Windows 原生环境执行不带 webkit2_41 的相关 Go 测试/race（支持时）、前端检查和现有 Windows Wails 构建，特别检查 codex.exe 启动与进程树回收。交叉编译不等于原生进程验收。真实 Codex 不作为普通 CI 测试依赖，不在 CI 自动安装或登录。

## Coverage map

可用同一 table-driven 测试覆盖多行，优先复用现有测试。测试辅助进程使用当前 Go 测试二进制和特定模式，不依赖 shell 脚本、Python、真实登录或外网；跨平台执行。每个新增业务行为先写能证明缺陷的失败测试，再实现。

| 用例 | 必需断言 |
| --- | --- |
| 七键旧配置/十一键新配置 | 默认 api，保留新增值，不覆盖 secret，不接受未知或重复 key |
| 活动模式校验 | Codex 无 API key 可启动；inactive extra-body 不阻塞；切回 API 恢复验证 |
| provider 路由/API 回归 | API 原工具/压缩/请求参数保留；Codex 不自动回退 API |
| 路径与 CLI 版本 | 空格路径、缺失 CLI、Windows cmd/bat、非兼容版本可解释错误 |
| 初始化和认证 | initialized 必须在 initialize 成功后；缺登录、无需 OpenAI 登录的 provider 区分 |
| 协议数据流 | 分片/多帧、乱序 id、双向请求、未知通知/请求、超大帧、EOF、stderr 大量输出 |
| 动态工具 | 正确 inputSchema/arguments/result；callId 幂等；错误批量不部分写入 |
| 树路径映射 | `/Downloads/a.zip` 存成正确 RelativePath；中文/空格/反斜杠/大小写不改写 |
| 候选边界 | 拒绝 ../、未知节点、glob、根候选、非普通文件；宿主大小为准 |
| 有效工具隔离 | 无通用执行、外部 MCP/插件和代理工具；意外批准请求被拒绝，不产生副作用 |
| 流式输出/终态 | delta+最终消息无重复；failed/interrupted/早退不是 DONE；无候选允许成功 |
| usage | 累计 100/100/150 最终 150；未知不造零用量；不同任务隔离 |
| 取消/超时/关闭 | 各阻塞点能退出；不晚写状态、不留子进程、不杀其它 Codex；可启动下个任务 |
| 未保存连接测试 | 使用新路径/模型、不保存、不扫盘；仅实际 completed 成功 |

完成后用 `rg -n '^func Test' backend -g '*_test.go'` 建立真实测试名到规格的映射。根规则的 Python 测试审计示例不适用此 Go 项目，不能新建 tests/test_core.py 应付审计。

## Real acceptance

只有用户已有可用 CLI 登录且允许测试请求时执行；没有登录不擅自登录。若无使用额度或外网则记录未验证，不声称 mock 已证明真实可用。

1. 在设置页保留已有 API 配置，选择 Codex，不填 API key，设置有效本机 CLI 路径；刷新后模式和隐藏 API 值均保留。
2. 用未保存的可选模型执行连接测试，验证成功后保存值没有变化；无效模型/CLI 路径分别给出错误。CLI 模型默认继承和显式指定都验证。
3. 准备临时树，含大小不同的目录、中文/空格名及适用平台的大小写/反斜杠样本；由 gdu 正常扫描再分析。
4. 核查实际三个工具往返、渐进读取、界面文本、候选路径/大小、日志及历史；不出现自由 shell 目录遍历，不调用删除工具。
5. 在模型运行时停止任务，观察 CANCELLED 和进程退出，再开始下一次；另验证超时和退出应用清理。
6. 如需验证“确认后删除”链路，仅选择专用临时 fixture 中准确候选，仍由现有 UI 发起；不得让 Codex 代为删除。
7. 切回 API，确认原 URL/密钥/模型以及原压缩选择存在；按获准方式验证原 API 行为。
8. Windows/Linux 分别记录本机版本、启动/取消结果；未能验证的平台保持未完成，不能根据 Linux 结果推定 Windows 正常。

## Deliverables

- 功能实现、获准的必要测试、生成的绑定、双语 UI/README。
- docs/local-codex-validation.md：Git 基线、CLI 版本/协议证据、D5 有效工具隔离证据、实际测试名/命令/结果、双平台状态、真实登录验收及限制。
- 仅凭证据更新 tasks.md；OpenSpec 工件完整不代表实现完整。不得自动提交、发布或归档。

若需要改变协议选择、放开通用执行、安装 CLI、升级依赖或扩展到其它 provider，先给出具体原因及最小变更范围；常规私有函数拆分无需反复确认。
