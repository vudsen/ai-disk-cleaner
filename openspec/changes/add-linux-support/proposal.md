## Why

项目已通过 gdu 的 Go API 扫描目录，但平台适配、路径处理、数据存储和发布流程仍带有 Windows 假设。需要在保留 Windows 行为的基础上，让普通用户可以在 Linux 桌面完成扫描、AI 分析及确认后的清理，并用分阶段测量判断扫描性能瓶颈。

## What Changes

- 继续使用 `github.com/dundee/gdu/v5 v5.34.0`；先修复 Linux 编译入口及平台适配，不更换扫描引擎。
- 增加 Linux 挂载点与容量枚举，首页根据平台选择有效扫描目标，Linux 默认使用用户家目录。
- 实现 Linux 扫描边界：排除虚拟系统目录、不递归符号链接、不跨入嵌套挂载点，保留取消和进度功能，并展示扫描不完整的警告。
- 修复 Linux 大小写敏感、反斜杠文件名、硬链接身份等语义；覆盖清理、迁移及历史结果匹配的相关调用点。
- Linux 数据库和日志写入用户数据目录；保留 Windows 现有位置。
- 扩展 Linux 原生构建和验证，新增 Ubuntu 24.04 amd64 的构建产物，保留现有 Windows 任务。
- 分别测量目录扫描、统计汇总、JSON 编码、FileTree 解析的耗时与分配；本变更只提交测量和决策，不默认实施直接树转换。

## Capabilities

### New Capabilities

- `linux-platform-support`: 平台入口、挂载点、首页默认值、普通用户运行、用户数据目录、路径及扫描边界、构建和兼容性。
- `scan-performance-baseline`: gdu 现有流水线的分阶段测量、可复现基准及是否需要后续优化的决策。

### Modified Capabilities

无。当前没有可复用的主规格；已有 `add-analyzer-history-compaction` 变更不属于本次范围，不得修改。

## Impact

- 后端：`backend/util`、`backend/model/scanner`、`backend/service/{scanner,cleaner,migration,analyzer,tasklog}`、`backend/data` 及 cleaningrecord 路径匹配；新增 `backend/service/platform/platform.go` 并由服务管理器集中装配。
- 桥接：`app.go` 的相关方法改为薄服务委托；新增平台信息方法；重生成对应 Wails 绑定。
- 前端：首页、权限提醒、结果路径显示以及中英文翻译，复用现有 HeroUI 组件。
- 工程：`.github/workflows/{build,release}.yml`、双语 README、新增 Linux 验收与性能报告。
- 优先使用现有 `golang.org/x/sys/unix` 和标准库，不新增 gopsutil、fastwalk 或外部 gdu 可执行文件依赖，不升级 Go/Wails/gdu。
- 目标为 Linux amd64 桌面，参考构建环境为 Ubuntu 24.04 + GTK3 + WebKitGTK 4.1；不承诺所有发行版、ARM64、macOS 或沙盒包支持。
- 这是开发方案，尚未授权编写测试或实施代码。执行时先询问用户是否同意方案中的必要测试包，再按获准范围执行 TDD；本次文档生成无需新增单元测试。
