# Linux validation record

## Initial environment and baseline (2026-10-19)

- Go: `go1.26.2 linux/amd64` (the module declares Go 1.25.0); Go cache redirected to `/tmp/ai-disk-cleaner-go-cache` because the default cache is read-only.
- Node: `v22.22.1`; pnpm: `10.32.1`; the frontend `node_modules` directory is absent.
- GTK 3: `3.24.41`; WebKitGTK 4.1: `2.52.6`.
- Wails CLI: unavailable (`wails: command not found`).
- `RTK.md`: not found in the repository or checked ancestor directories. Root and applicable directory `AGENTS.md` files were read.
- Initial `git status --short`: existing untracked `.vscode/` and `openspec/changes/add-linux-support/`; both are preserved. No other OpenSpec change is modified.
- Baseline `go test ./backend/...` first failed because the default Go build cache was read-only. With `GOCACHE=/tmp/ai-disk-cleaner-go-cache`, Linux packages ran and exposed the existing `backend/util/disk_other.go: undefined: model` compile error. Remaining packages could not load uncached modules because the configured module cache is read-only.
- Baseline `cd frontend && pnpm lint` and `pnpm check` could not start: `eslint: not found` (no `node_modules`). Neither command changed tracked files.
- `wails doctor` could not run because Wails CLI is not installed.

## Pinned gdu source inspection

Source: `github.com/dundee/gdu/v5@v5.34.0`, downloaded at the exact `go.mod` version; module origin tag resolves to commit `0337d3003677a0d5822296149aa38557aba5ad3c`.

- `pkg/analyze/parallel.go:92` defines `(*ParallelAnalyzer).AnalyzeDir(path, ignore common.ShouldDirBeIgnored, fileTypeFilter common.ShouldFileBeIgnored)`. Directory filtering receives `(name, path)` and `true` skips descent; file filtering receives `name`. The sequential and stable-order analyzers expose the same contract.
- Symlink following is opt-in (`SetFollowSymlinks(true)`); the analyzer's zero value leaves it disabled. Linux `os.ReadDir` entries do not classify symlinks as directories, and `getFlag` marks symlink/socket entries with `@`.
- Directory read failures and `FileInfo` failures are logged by gdu. Directory failures are represented by `getDirFlag(err, items) == '!'`; individual metadata failures set the containing directory flag to `!` and skip that file. `AnalyzeDir` returns a tree and no error value, so it does not expose structured error paths. The project can read flags for partial-scan indication but cannot reconstruct every error without changing gdu or doing another traversal.
- `pkg/analyze/dir_linux-openbsd.go` extracts inode from `syscall.Stat_t.Ino` when link count exceeds one and allocated bytes from `Stat_t.Blocks * 512`; apparent size comes from `FileInfo.Size()`. The library hard-link key is inode only, not `(device, inode)`, so single-filesystem scan boundaries remain required.
- `AnalyzeDir` starts a progress worker and broadcasts completion, but has no context parameter. Project cancellation can be observed in filter callbacks and before/after phases, not interrupt a blocked `ReadDir`. `GetProgressChan` remains open; consumer shutdown must be project-owned.
- `UpdateStats` recursively aggregates using `fs.HardLinkedItems`; `EncodeJSON(io.Writer, bool)` writes JSON to the supplied writer and returns encoding/write errors.

The source inspection above is not, by itself, native Wails compilation, desktop acceptance, or real-data performance evidence.

## TDD coverage and implementation evidence

| Test | Specification behavior | Command/result |
| --- | --- | --- |
| `TestLinuxPlatformExports` | Linux exports platform functions and reports effective-UID privilege | First run failed as expected because the functions were undefined (also exposed the existing missing `model` import). The Linux platform split and mount enumerator now pass `go test ./backend/util`. |
| `TestGetInfoUsesNativePlatformAndHome`, `TestGetInfoReportsHomeLookupError`, `TestIsRunningAsAdministratorUsesNativeCheck` | Platform information and effective UID | Included in `go test ./backend/service/platform`; passed. |
| `TestGettersPanicBeforeInitialization`, `TestInitializePublishesEveryService` | Central service lifecycle | Included in `go test ./backend/service`; passed with platform service registration. |
| `TestLinuxDataAndStatePathsRespectAbsoluteXDGOverrides`, `TestLinuxDataAndStatePathsIgnoreRelativeXDGOverrides`, `TestLinuxDataAndStatePathsFailWithoutHome` | XDG absolute/fallback/error behavior | Included in `go test ./backend/util`; passed. |
| `TestOpenDefaultSQLiteUsesSameDatabaseAcrossWorkingDirectories` | Linux data path ignores cwd | First run failed because the two cwd values opened separate DBs; after `openDefaultSQLite` used `DataFilePath`, `go test ./backend/data` passed. |
| `TestOpenUsesConfiguredLogDirectoryAndPrivatePermissions` | Linux task log directory and file permissions | Included in `go test ./backend/service/tasklog`; passed. Existing failure degradation and remove tests passed in the same package. |
| `TestParseMountInfoDecodesMountPathsAndPreservesBindEntries`, `TestParseMountInfoReturnsScannerReadError` | mountinfo parsing, escapes, malformed lines, repeated bind paths, reader failure | First run failed because parser was undefined; then `go test ./backend/util -run TestParseMountInfo` passed. |
| `TestBuildLinuxDiskListFiltersDeduplicatesAndSortsMounts`, `TestBuildLinuxDiskListUsesAvailableAndAllocatedBlocks`, `TestBuildLinuxDiskListReturnsEmptyWhenNoMountCanBeRead` | Linux mount filtering, duplicate selection, Statfs formulas, failed/empty mounts | Included in `go test ./backend/util`; passed. |
| `TestSamePathPreservesLinuxCase`, `TestMigrationPathIdentityPreservesLinuxCase`, `TestTrashFileSourceMatchPreservesLinuxCase` | Case-distinct Linux cleanup, migration, and history identity | Each first failed against unconditional case folding; passed after shared host-aware path helpers were added. Native Windows tests are still pending. |
| `TestPOSIXTreePreservesBackslashFilename` | POSIX FileTree lookup and display preserve a literal backslash | First run exposed `FindNode` converting the filename into a separator; passed after root-aware normalization. Existing scanner tests with Windows-rooted fixtures also passed on Linux. |
| `TestAnalyzeDirectoryRoundTripsPOSIXBackslashFilename` | Analyzer tool query round trips a POSIX literal-backslash filename | First run failed at the unconditional separator check; passed after validation used the tree root's path style. |
| `TestLinuxScanPolicySkipsVirtualAndNestedMountsByPathBoundary`, `TestLinuxScanPolicyAllowsExplicitMountRootAndExcludesItsChildren`, `TestLinuxScanPolicyRejectsVirtualSystemRoots` | Virtual path and nested mount boundaries, including same-device bind entries | Mountinfo fixture tests passed; no root mount operation was required. |
| `TestLinuxScanUsesFilesystemMetadataForLinksSparseFilesAndFIFO` | Real Linux temp tree behavior for symlinks, hard links, sparse files, and FIFO | Passed against gdu v5.34.0; unsupported filesystem operations explicitly skip. |
| `TestProgressConsumerStopWaitsForCallbackAndPreventsLaterCallbacks`, `TestLinuxScanObservesCancellationDuringDirectoryWalk` | Scan cancellation and joined progress consumer | Cancellation during the real temporary directory walk returned `context.Canceled`; no callback occurred after return. |
| `TestLinuxScanRejectsUnreadableRoot`, `TestLinuxScanSummarizesUnreadableChildDirectory` | Fatal root read errors and partial child read errors | Both passed as the ordinary user (`uid=1000`) against real mode-000 temporary directories. gdu v5.34.0 exposes child errors only at directory granularity; the summary uses the flagged directory path and cannot report the underlying errno. |
| `TestScanSummaryBoundsWarningDetailsButKeepsFullCounts` | Bounded warning details with full counters | Passed in `go test ./backend/model/scanner`. |
| `TestCleaningRecordScanSummaryRoundTripAndCompletionPreservesIt`, `TestOpeningDatabaseAddsNullableScanSummaryColumn` | Nullable history summary, additive schema upgrade, and completion update safety | Passed in `go test ./backend/data`; summary remained after `CompleteCleaningRecord`, and old rows read with nil summary. |
| `TestLinuxDeletionBoundaryRejectsNestedMountAndSymlinkComponent`, `TestDeleteTrashFilesRejectsSymlinkPathComponent`, `TestServiceDeletesOnlyRecordedTrashFilesInsideScanRoot` | Linux cleanup mount/symlink boundaries and ordinary target deletion | Passed with synthetic mountinfo and isolated temporary files; no real user path was used. |

## Current validation (2026-10-20)

- Frontend dependencies were installed with `pnpm install --frozen-lockfile`. `pnpm check` and `pnpm build` passed. The build reports its existing large JavaScript chunk advisory (1.17 MB before gzip); no frontend test framework exists in this repository, and none was added.
- Wails CLI v2.12.0 generated `GetPlatformInfo`, `model.PlatformInfo`, `scanner.ScanSummary`, and `scanner.ScanWarning` bindings. Native build passed with `GOMODCACHE=/tmp/ai-disk-cleaner-modcache GOCACHE=/tmp/ai-disk-cleaner-go-cache GOPROXY=off wails build -platform linux/amd64 -tags webkit2_41 -o ai-disk-cleaner -trimpath -m`.
- The resulting `build/bin/ai-disk-cleaner` is an x86-64 dynamically linked ELF. `ldd` resolved all listed dependencies, including `libwebkit2gtk-4.1.so.0`, `libgtk-3.so.0`, `libsoup-3.0.so.0`, `libjavascriptcoregtk-4.1.so.0`, and `libglib-2.0.so.0`; no `not found` entries were reported.
- `GOMODCACHE=/tmp/ai-disk-cleaner-modcache GOCACHE=/tmp/ai-disk-cleaner-go-cache GOPROXY=off go test -tags webkit2_41 ./...` passed across all Go packages. An earlier concurrent run exposed an extra duplicate `SCANNING` event after summary capture; the summary is now included in the existing `ANALYZING` event, and the clean full-suite rerun passed.
- `GOMODCACHE=/tmp/ai-disk-cleaner-modcache GOCACHE=/tmp/ai-disk-cleaner-go-cache GOPROXY=off go test -race -tags webkit2_41 ./...` passed across all Go packages. The async cleaner test's completion wait is 10 seconds so race-instrumented SQLite work can finish.
- `GOOS=windows GOARCH=amd64 go test -exec=/bin/true ./...` cross-compiled all Windows test packages successfully. `/bin/true` replaces test execution, so this is compile evidence only; Windows-native test behavior and CI results remain unverified.
- Scanner stage benchmarks and the one-pass phase sample are recorded in [scan-performance-baseline.md](scan-performance-baseline.md). The fixture is synthetic; no real user directory or LLM was used.
- Final non-GUI smoke check passed: `GOMODCACHE=/tmp/ai-disk-cleaner-modcache GOCACHE=/tmp/ai-disk-cleaner-go-cache GOPROXY=off go test -tags webkit2_41 ./backend/service/cleaner ./backend/service/scanner -run '^(TestServiceRunsTaskAndPersistsResult|TestParseGDUScansDirectoryInMemory|TestLinuxScanUsesFilesystemMetadataForLinksSparseFilesAndFIFO)$' -count=1`. This exercises an isolated gdu filesystem scan and the cleaner task-to-analysis-to-persistence lifecycle; the cleaner test injects its scanner and analyzer dependencies.
- Windows-native execution and Windows CI results remain unverified in this local Linux environment. The unreadable root and child `EACCES` tests both ran under the ordinary `uid=1000` user; the root test explicitly skips if later run with effective UID 0.
- Full desktop acceptance remains incomplete. A limited window inspection had checked the Linux home default, disk card, missing-LLM prompt, and native directory picker using isolated temporary data, but it did not execute a scan or establish acceptance for GUI cancellation, real LLM analysis, deletion confirmation, migration/restore, or restart/history behavior.

The Linux build and automated checks establish compilation and package behavior on Ubuntu 24.04 amd64. They do not establish cross-distribution support, Linux ARM support, GUI behavior in a desktop session, or real-data safety beyond the isolated temporary-tree tests above.
