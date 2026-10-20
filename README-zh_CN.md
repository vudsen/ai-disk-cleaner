# ai-disk-cleaner

一款 AI 驱动的智能磁盘清理助手。

![overview](./images/overview.png)

![trash-table](./images/trash-table.png)

![llm-output](./images/llm.png)

![migrations](./images/migrations.png)

支持的功能:

- 🤖 LLM 智能分析: LLM 智能分析磁盘占用，扫描可以删除的垃圾，并汇总结果展示
- 📦 迁移管理: 提供了软链接的统一管理，支持一键迁移文件到其它盘中


## 快速开始

1. 从 [Release](https://github.com/vudsen/ai-disk-cleaner/releases) 下载 Windows `.exe` 或 Linux `amd64` `.tar.gz`。当前 Linux 构建在 Ubuntu 24.04 amd64 上验证。
2. Windows 版放在单独目录中，数据仍保存在 exe 旁边。Linux 版解压后在解压目录运行 `./ai-disk-cleaner`；运行时需要 GTK 3 和 WebKit2GTK 4.1。
3. Linux 数据保存在 `$XDG_DATA_HOME/ai-disk-cleaner`（默认 `$HOME/.local/share/ai-disk-cleaner`），日志保存在 `$XDG_STATE_HOME/ai-disk-cleaner/logs`（默认 `$HOME/.local/state/ai-disk-cleaner/logs`）。旧测试版数据不会自动迁移。关闭应用后，先备份旧 `data.db`，再移动到新数据目录；不要覆盖新版本已创建的数据库。
4. 打开设置页面，配置大模型参数。
5. 回到首页，开始扫描。Windows 推荐以管理员权限运行；Linux 请使用普通桌面用户运行。

## 运作原理

整个架构分为三步:

1. 使用 [gdu](https://github.com/dundee/gdu) 扫描指定位置，并将文件树加载到内存中
2. 暴露读取文件树的工具，让 LLM 分析文件树，找出占用最大的文件。在找到垃圾文件后 LLM 会调用 [add_trash_file](backend/service/analyzer/tools.go) 工具来标记文件
3. 系统汇总 LLM 分析结果，展示给用户 
