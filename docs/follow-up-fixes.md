# Codex and desktop UI follow-up fixes

## Scope and evidence

These changes address five reported failures while preserving the existing Linux and local-Codex work. No commits or release uploads were made. The user approved focused regression tests; frontend verification uses the existing checks and isolated browser/native acceptance, without a new project test framework.

| Report | Change | Evidence |
| --- | --- | --- |
| Codex initialize deadline | Allow 30 seconds for cold startup, still bounded by the caller's context | `TestCodexAppServerAllowsColdStartup` originally failed after the fixed ten-second timeout with an eleven-second startup fixture; it now passes |
| Codex analysis never completes | Read completion identity from `turn.id`, matching the actual protocol rather than requiring a top-level turnId | `TestCodexRunnerConsumesActualCompletedTurnShape` originally ignored the notification; it now completes, and the fake CLI full-path test now uses the real completion shape |
| White page after saving | Application-owned HeroUI notification queue updates only its region, avoiding whole-document View Transitions | Chromium and WebKit checked successful and rejected saves, zero whole-page transition calls, visible settings and no uncaught page errors; the actual production Linux application also saved successfully without losing its page |
| Historical analysis retry | Reanalyze the same record from its saved tree using current settings; never scan disk, including after restart. ERROR, CANCELLED and DONE records can be reanalyzed | `TestRetryHistoryUsesSavedTreeWithoutScanning`, `TestRetryMissingSnapshotNeverScans`, `TestRetryCanPersistExistingRAMTreeWithoutScanning`, `TestRetryCorruptSnapshotFailsWithoutScanning` |
| Black navigation selection | Explicit active-page colors and aria-current; remove forced transform/will-change layers from buttons | Browser computed styles verified transform:none and will-change:auto; Chromium, WebKit and the production GTK application displayed the selected navigation item normally |
| Slow scrolling/rendering | Remove large blur/backdrop filters from scan pages, batch live text at 120 ms, avoid unused text updates on overview pages, render plain text while streaming and memoized Markdown after completion | Both browser engines received fragmented output, then an authoritative completion snapshot; final Markdown rendered once with no duplicated text. Production app scrolling and saving were checked on an isolated virtual display |

The original device-specific white frame was not reproduced in headless browsers before the change. The fix removes the full-page snapshot dependency implicated by the notification path; native save acceptance then passed. No hardware-specific FPS improvement is claimed.

## Validation commands and results

- `go test -tags webkit2_41 ./backend/...`: passed.
- `go test -race -tags webkit2_41 ./backend/service/analyzer ./backend/service/cleaner ./backend/service/setting`: passed.
- `go test -tags webkit2_41 ./... -skip '^TestLLMAnalysisFindsExpectedTrashFiles$'`: passed. The root integration test was deliberately skipped to avoid using real model credentials or changing real application settings.
- `AI_DISK_CODEX_STARTUP_PROBE=1 go test -tags webkit2_41 ./backend/service/analyzer -run '^TestCodexLocalStartupProbe$' -v -count=1`: passed against installed Codex 0.162.1. Both configuration-discovery and isolated app-server initialization completed; account readiness was ready. No model turn was started.
- Frontend: `pnpm lint`, `pnpm check`, `pnpm build`: passed. The existing large-bundle advisory remains.
- Wails bindings regenerated through `wails generate module -tags webkit2_41`; RetryCleaning is generated, not hand-written in JS/TS.
- Production application: `wails build -m -nosyncgomod -platform linux/amd64 -tags webkit2_41 -trimpath -o ai-disk-cleaner-fixes`: passed. `-m` avoids fetching unrelated dependency-test modules through mod tidy; go.mod/go.sum were not changed.
- Chromium and WebKit ran the same isolated Wails-fixture checks against both Vite development resources and the built production resources: passed. The fixtures covered save success/error, active navigation styles, retry reusing the same record, and streamed output followed by completion.
- The actual Linux binary was launched under a separate Xvfb display with temporary XDG data/state directories and a synthetic API configuration. Navigation, scrolling to the save controls, save notification and database access passed. No scan or model request was started, and the owned test application was closed afterwards.

Regression names were audited against the changed behaviors. Existing settings/API tests remain in the full backend suite. The optional installed-CLI startup probe skips unless explicitly enabled. No Python placeholder unit tests or frontend framework dependencies were added.

## Artifact and limits

Linux binary: `build/bin/ai-disk-cleaner-fixes`.

Windows-native UI/process behavior and the user's physical graphics-driver scrolling performance were not measured in this session. The real CLI checks cover startup/readiness and dynamic-tool round trips through a localhost-only fake Responses provider; authenticated model analysis remains unverified. Existing unrelated workflow CRLF whitespace and generated model whitespace were left intact.

## Persisted scans and dynamic-tool execution follow-up

- The supported CLI 0.162.1 requires `code_mode_host` for dynamic application tool execution. Disabling it left tool definitions visible but made actual calls fail with `code-mode host is disabled`. The isolation argv now explicitly enables that host while retaining shell/plugin/MCP restrictions. Analysis without a successful `analyze_directory` call is an error, rather than a successful empty report.
- `TestCodexIsolationRetainsDynamicToolHost` and `TestCodexRejectsCompletedAnalysisWithoutDirectoryEvidence` failed before the fix and pass after it. `AI_DISK_CODEX_TOOL_PROBE=1 go test ./backend/service/analyzer -run '^TestCodexInstalledCLIDynamicToolsRoundTrip$' -v -count=1` passed against the installed real CLI using a private CODEX_HOME and fake local endpoint. The fixture starts with the host disabled in user configuration, proving the process override restores execution. It verifies directory output reaches the provider and the next usage tool produces a size from the scanned tree. No real model request is made.
- Scans are saved before LLM analysis to the independent `scan_snapshot` SQLite table as versioned gzip/gob data. A flat parent-index representation avoids duplicating absolute/relative paths and lookup maps. Restoration rebuilds those values and preserves inode precision, node flags, timestamps and scan coverage. History listings select only small snapshot metadata, never tree blobs; the frontend receives only counts and sizes.
- Reanalysis keeps the record ID, scan time and root. It uses a matching RAM tree when available, otherwise restores the saved snapshot asynchronously. It neither checks that the original directory is still mounted nor invokes the disk scanner. Completed scans remain available after an LLM error or cancellation. Missing/corrupt snapshots fail explicitly, without scan fallback. Only StartCleaning, called by the homepage scan button, starts a fresh scan.
- `TestCompletedScanPersistsTreeBeforeAnalyzerFailure`, `TestSnapshotSurvivesDatabaseReopenAndRestoresLookup` and `TestHistoryRetentionRemovesSnapshotsAtomically` cover save-before-analysis, SQLite reopen and tree lookup, Linux literal backslashes/Windows/UNC paths, corruption, cancellation and transactional snapshot cleanup with history retention.
- Legacy records saved only scan coverage and analysis results, not the full directory tree. A read-only check found zero saved snapshots and no running disk-cleaner process for the reported scan. Those old trees cannot be recovered from stored statistics. They remain intact and will not be silently rescanned.
- Reanalysis labels now say “重新分析” / “Analyze again”. Historical counts and sizes survive restart. Byte formatting clamps sub-byte chart ticks to the B unit, preventing the displayed `undefined` suffix.
- The data-store singleton test now sets temporary XDG_DATA_HOME/XDG_STATE_HOME as well as XDG_CONFIG_HOME; its prior Linux setup resolved the user's default database during the suite. No user scan or analysis was started, and no historical record was removed by that test.
