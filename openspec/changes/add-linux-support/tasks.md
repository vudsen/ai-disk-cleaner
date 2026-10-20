## 0. Execution gates and baseline

- [x] 0.1 阅读根和目录级 AGENTS.md、可找到的 RTK.md 及本变更所有文档；记录 git status，保留用户的 `.vscode/` 等无关文件；确认无须修改其它 OpenSpec 变更。
- [x] 0.2 按 execution-guide.md 询问必要测试包；记录批准范围。未获答复前只做只读调查及基线，不写测试或对应实现；已明确批准则不重复询问。
- [x] 0.3 下载并阅读 go.mod 固定的 gdu v5.34.0 源码，逐项核实 design D1 的过滤、错误、符号链接、硬链接和取消 API；将实际符号及限制记录到 docs/linux-validation.md，不使用推测 API。
- [x] 0.4 核查 gvm/nvm/pnpm、Go 版本、GTK/WebKit 与 Wails v2.12.0；运行可用基线检查，记录现有失败和环境缺失，禁止记为本次已通过。

## 1. Platform service and Linux compilation

- [x] 1.1 测试先行修复 util 平台导出函数/import/build tags：新增 disk_linux.go、administrator_linux.go，收窄 other 标签，保证 Windows 无重定义；先可返回明确的未实现枚举错误，不能伪造磁盘。
- [x] 1.2 测试先行新增 model.PlatformInfo 和 platform 服务的 GetInfo/ListDisks/IsRunningAsAdministrator；由 service.go 集中构造、发布、提供 getter；补充初始化及未初始化 getter 用例。
- [x] 1.3 app.go 的相关入口改为薄服务委托，增加 GetPlatformInfo；不新增根目录 Go 文件。完成本组后运行相关包检查，记录枚举实现仍待第 3 组完成。

## 2. Writable Linux persistence

- [x] 2.1 测试先行实现 design D7 的 apppaths 解析：绝对/相对/缺省 XDG、HOME 失败、Windows 原路径；测试使用 t.Setenv 与临时 HOME，不访问真实用户数据。
- [x] 2.2 测试先行接入 data/store.go：创建私有 Linux 数据目录，从不同 cwd 打开同一测试数据库；更新与实现不符的注释，Windows 数据位置保持兼容。
- [x] 2.3 测试先行接入 tasklog Open/Remove 及 cleaninghistory 日志保留清理；验证日志降级与 Linux 路径、权限；不移动旧数据库或日志。

## 3. Linux mount enumeration

- [x] 3.1 测试先行编写 mountinfo 纯解析器，覆盖可选字段、转义空格/反斜杠、坏行、重复挂载路径和 bind mount 信息；保留完整列表供扫描策略使用。
- [x] 3.2 测试先行实现 design D3 的首页过滤、稳定排序和 Statfs 容量换算；用私有注入模拟失效挂载、保留块和空列表；接通 Linux ListDisks，移除第 1 组的临时未实现错误。

## 4. Native and tree path semantics

- [x] 4.1 测试先行修复 cleaner、migration.samePath、migrationStepKey、cleaningrecord.trashFileMatchesSource 的 Linux 大小写语义；覆盖 Cache/cache 不互相匹配及 Windows 行为兼容。
- [x] 4.2 测试先行让 FileTree 查询、parser 路径拼接按树根区分 POSIX/Windows；新增 a\b.txt、中文/空格、根路径样本，同时保留既有 Windows JSON 夹具全部通过。
- [x] 4.3 测试先行修复 analyze_directory 对 POSIX 字面反斜杠的处理；覆盖工具返回路径再次查询的往返一致性。不得修改 LLM prompt 或上下文压缩策略。

## 5. Linux scan policy and lifecycle

- [x] 5.1 测试先行实现虚拟路径边界及嵌套挂载排除：`/proc` 与项目内 proc 不混淆，bind mount 同设备也被排除；用 mountinfo 夹具测试，无需 root 挂载。
- [x] 5.2 真实临时目录验证 gdu 的目录符号链接、硬链接、稀疏文件及特殊文件处理；根错误与子节点读取失败按 design D5 分流，不支持的平台显式 skip，不以 mock 替代 Linux 实测。
- [x] 5.3 测试先行保证扫描及后处理观察取消，进度消费者在返回前退出；覆盖已关闭通道/返回后无回调。复用现有预取消测试，再补“扫描中取消”，避免重复测试。
- [x] 5.4 测试先行实现有上限的 ScanSummary，加入 FileTree 与快照，持久化到 cleaningrecord 可空 JSON 列并写入日志；用旧数据库副本验证升级、历史重读及后续完成任务不覆盖摘要。
- [x] 5.5 测试先行复核 cleaner 删除边界，拒绝跨嵌套挂载、策略排除子树或不支持的符号链接分量；保留普通临时文件的确认删除行为。不得对真实用户目录执行递归删除验收。

## 6. Frontend integration

- [x] 6.1 通过固定版本 Wails 的实际绑定生成流程生成新平台 API 和 ScanSummary 类型；检查生成 diff，不手写桥接定义。
- [x] 6.2 首页按 design D4 初始化 Linux 家目录/Windows 系统盘，处理异步初始化不覆盖选择、空磁盘列表、加载失败及所选路径的最长挂载前缀容量匹配。
- [x] 6.3 Layout 只在 Windows 显示现有管理员提醒；结果路径显示按树根风格保留 POSIX 字面反斜杠；详情页对活动/历史扫描显示摘要。
- [x] 6.4 补齐 en/zh_CN 翻译，执行 pnpm lint、pnpm check、pnpm build；按获准的测试方案验证前端行为，不为本次任务擅自引入新前端测试框架。

## 7. Native build and delivery preparation

- [x] 7.1 build.yml 保留 Windows 检查并新增 Ubuntu 24.04 原生依赖、带 webkit2_41 的 Go 测试和 Wails 构建任务；Linux job 不依赖真实 LLM 密钥。
- [x] 7.2 release.yml 保留 Windows job，增加 Linux tar.gz、sha256 及版本/tag 校验；仅准备工作流，不创建 release、不上传真实发布资产。
- [x] 7.3 更新中英文 README 的平台、依赖、启动、用户数据位置及迁移说明；执行 Linux 原生 Wails 构建并保留产物及实际动态依赖信息。

## 8. Performance baseline without engine changes

- [x] 8.1 新增 benchmark_test.go，按 design D9 实现五个阶段基准、受控 fixture 和显式只读真实样本入口；可变对象每次重新准备，生成时间不计入基准。
- [x] 8.2 运行至少五组相同条件暖缓存样本，取得同次端到端阶段时长、JSON 大小、分配及独立测试进程峰值 RSS；记录硬件/文件系统/版本/并发/数据集，不能测 LLM 耗时充当扫描耗时。
- [x] 8.3 写 docs/scan-performance-baseline.md，应用 20%/内存证据阈值并作出“后续单独优化或先调查遍历”的结论；本变更不实施直接转换或更换扫描库。

## 9. Acceptance and audit

- [x] 9.1 运行 execution-guide.md 的格式、前端、Linux Go、race 和 Wails 检查；核查 Windows CI 结果，无法执行的检查列为未验证，不能默认为通过。
- [ ] 9.2 在普通用户 Linux 桌面按 execution-guide.md 场景验收；记录选择目录、进度/取消、部分扫描、AI 分析、临时树确认删除、迁移/还原和历史重启结果。缺 GUI/LLM 凭据时保留对应项未完成。
- [x] 9.3 将实际用例名映射到规格行为，记录覆盖与手工验证证据到 docs/linux-validation.md；复用现有测试，补齐获准范围内的缺口；Go 项目不机械创建 tests/test_core.py。
- [x] 9.4 检查范围、依赖版本、绑定、双语文案、数据兼容、未修改无关文件；更新任务状态并报告剩余限制。不自动提交、发布或归档此变更。
