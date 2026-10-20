# ai-disk-cleaner

[简体中文](./README-zh_CN.md)

An AI-powered intelligent disk cleanup assistant.

![overview](./images/overview-en.png)

![trash-table](./images/trash-table-en.png)

![llm-output](./images/llm-en.png)

![migrations](./images/migrations-en.png)

Features:

- 🤖 Intelligent LLM analysis: Uses an LLM to analyze disk usage, identify removable junk files, and present a summary of the results
- 📦 Migration management: Provides centralized management of symbolic links and supports one-click file migration to other drives

## Quick Start

1. Download the Windows `.exe` or Linux `amd64` `.tar.gz` from the [Releases](https://github.com/vudsen/ai-disk-cleaner/releases) page. Linux builds are currently validated on Ubuntu 24.04 amd64.
2. On Windows, place the `.exe` in a dedicated folder; its data remains beside the executable. On Linux, extract the archive and run `./ai-disk-cleaner` from the extracted folder. Linux requires GTK 3 and WebKit2GTK 4.1 at runtime.
3. Linux data is stored in `$XDG_DATA_HOME/ai-disk-cleaner` (default: `$HOME/.local/share/ai-disk-cleaner`) and logs in `$XDG_STATE_HOME/ai-disk-cleaner/logs` (default: `$HOME/.local/state/ai-disk-cleaner/logs`). Existing beta data is not migrated automatically. With the application closed, back up the old `data.db` before moving it to the new data directory; do not overwrite a database created by the new version.
4. Open the Settings page and configure the LLM parameters.
5. Return to the home page and start scanning. On Windows, running as administrator is recommended; on Linux, use a normal desktop user account.

### Choose an LLM provider

In Settings, choose either an OpenAI-compatible API or Local Codex CLI. The API option keeps its own URL, secret, model, token budget, and extra request body. Local Codex uses a separately installed Codex CLI and its existing terminal login; install it and run `codex login` in a terminal before selecting it. You can leave the executable path empty to resolve `codex` from the application's PATH, or enter one absolute executable path. On Windows, select a native executable instead of a `.cmd` or `.bat` wrapper.

The Codex model override is optional and inherits the model configured in Codex CLI when blank. Its analysis timeout can be set from 30 to 3600 seconds. Codex still needs network access to the configured model provider; account access, quotas, and usage depend on that provider. The app does not install Codex, log in, or copy authentication tokens. A successful readiness check only means the CLI and login prerequisites are available; use Test Connection to check an actual model response.

## How It Works

The entire architecture are three steps:

1. Use [gdu](https://github.com/dundee/gdu) to scan the specific location and load the file tree into memory.
2. Expose a file tree reading tool to the LLM and let it find the trash file. When the trash file is found, the LLM invokes the [add_trash_file](backend/service/analyzer/tools.go) tool to mark the file.
3. Combine the LLM analysis results and display them to the user.
