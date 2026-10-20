# Local Codex provider validation

## Preflight and protocol baseline (2026-10-20)

- Repository: `add-local-codex-provider` started with 30 unchecked tasks. Existing `.vscode/` and the unfinished `openspec/changes/add-linux-support/` work are preserved. No RTK.md was found in the repository or its checked ancestors. Root and applicable frontend/backend/data-model instructions were read.
- Tools: Go 1.26.2 (module minimum 1.25.0), Node 22.22.1, pnpm 10.32.1, GTK 3.24.41, WebKitGTK 4.1 2.52.6, Wails CLI 2.12.0.
- Codex CLI: `codex-cli 0.162.1`. `codex app-server --help`, `codex features list`, and `codex app-server generate-json-schema --experimental --out /tmp/ai-disk-cleaner-codex-protocol` completed. The schema directory is temporary; no authentication file was read or copied.
- Relevant protocol evidence from the generated schema: `initialize` carries `capabilities.experimentalApi`; `thread/start` supports `ephemeral`, `sandbox`, `environments`, and `dynamicTools`; `turn/start` supplies text input; `item/tool/call` has a separate server request/`callId`; token snapshots arrive through `thread/tokenUsage/updated` with `tokenUsage.total`; `turn/interrupt` is available.
- The schema names the runtime configuration field `mcp_servers` (snake case). `config/read` on an isolated synthetic CODEX_HOME returned `mcp_servers` as a map; the probe emitted only the key `synthetic_probe`, not values. It did not start the fake server before a thread was opened.

## Tool-isolation gate (descriptor-only evidence; corrected below)

All probes used a temporary CODEX_HOME/cwd and a local fake Responses API endpoint. The endpoint only captured the outbound tool definitions and returned a synthetic error; it did not contact a model service. The app-server thread was ephemeral, used `sandbox: "read-only"`, `environments: []`, and three dynamic tools.

- With explicit feature disables for `shell_tool`, `unified_exec`, `apps`, `plugins`, `multi_agent`, `multi_agent_v2`, `browser_use`, `browser_use_external`, `computer_use`, and `code_mode_host`, the captured request contained no shell or execution tool and no plugin/app tools. `shell_tool`, `apps`, `plugins`, `multi_agent`, browser, computer and code-mode feature entries reported false. `unified_exec` itself remains a stable/true infrastructure feature in `codex features list`, but no shell/exec tool was sent to the provider.
- The request contained the three application tools (`analyze_directory`, `add_trash_file`, `add_top_usages`), plus Codex's built-in `request_user_input` and a `web_search` tool with `external_web_access: false`. The app must reject every unrecognized `item/tool/call`; the built-in user-input surface does not read or modify the tree. No external web access is enabled by this tool descriptor. This is the exact model-visible result for 0.162.1; it must not be described as literally a three-entry total tool list.
- Synthetic MCP experiment: an empty `mcp_servers={}` override and a `thread/start.config.mcp_servers` override did not stop the synthetic configured server from starting. The working control was process-level `--config mcp_servers.synthetic_probe.enabled=false`; after that override the synthetic server had no runtime connection, no tools, and no startup error. Therefore the implementation must first obtain configured MCP names with `config/read`, discard config values in memory, then relaunch the owned app-server with one explicit `enabled=false` override per name. If config parsing, name validation, or any disable override fails, analysis must fail closed before a turn starts.
- `codex --disable unified_exec` did not make the stable `unified_exec` feature report false. The isolation decision is based on the fake provider's actual tool list (no shell/exec tool), not on the feature table alone. Keep this discrepancy documented and recheck if the compatible CLI baseline changes.

This probe satisfies the pre-implementation isolation gate for the supported 0.162.1 baseline without a real account or model. It does not establish behavior for other Codex versions; unknown versions must be rejected until their protocol and tool surface are verified.

## Baseline checks

- `GOMODCACHE=/tmp/ai-disk-cleaner-modcache GOCACHE=/tmp/ai-disk-cleaner-go-cache GOPROXY=off go test -tags webkit2_41 ./backend/service/analyzer ./backend/service/setting ./backend/data/...` passed before implementation.
- `cd frontend && pnpm check` passed before implementation.
- `wails version` reported v2.12.0. GTK/WebKit versions are recorded above.
- Windows-native CLI process-tree behavior and a full authenticated Codex analysis remain unverified. No desktop interaction or user file access was performed for this change.

## Test isolation incident

One regression run accidentally resolved the real local Codex CLI because its test configuration left `codex.executable` empty. The connection-test path started a minimal turn without dynamic tools and received streamed assistant text, then the test process panicked while handling the completed message. This may have consumed a small amount of model usage. It did not expose a scan tree, run a disk scan, or offer filesystem/application tools. The test now points to an explicit nonexistent executable, and no further authenticated model request was made. This was an unintended exception to the planned fake-only validation.

## Test-to-requirement evidence

| Test | Requirement behavior | Evidence |
| --- | --- | --- |
| `TestRenderDirectoryUsageCSVSortsAndBoundsEntries` | Existing bounded, descending tree view with 200-entry cap | First run failed at compile time because `renderDirectoryUsageCSV` was absent; after extracting the helper, the test passed. |
| `TestDecodeBusinessToolArguments`, `TestAPIBusinessToolsUseSharedOperations` | Shared parsing/formatting operations retain the API tools' inputs and outputs | Passed in `go test -tags webkit2_41 ./backend/service/analyzer -count=1`. |
| `TestMergeTrashFilesDeduplicatesAndDropsNestedCandidates` | Existing duplicate and parent/child candidate handling | First run failed because `mergeTrashFiles` was absent; passed after extraction. |
| `TestParseProviderConfigDefaultsLegacySevenKeySettingsToAPI`, `TestParseProviderConfigCodexIgnoresInactiveAPIValues`, `TestParseProviderConfigRejectsInvalidProviderAndTimeout` | API default, active provider parsing, inactive API fields, and Codex timeout/path validation | Initial run failed because provider types/parser were absent; passed in the analyzer package. |
| `TestTestConnectionPreservesAPIRequest`, `TestTestConnectionPreservesAPIError`, `TestTestConnectionDoesNotFallBackFromCodexToAPI` | API request/error compatibility and explicit provider routing | Passed in `go test -tags webkit2_41 ./backend/service/analyzer -count=1`; the Codex case verified no API request was issued. |
| `TestSettingDefaultsAndSave` | Four additive defaults and GORM `OnConflict DoNothing` preservation | First run failed because only seven defaults existed; passed in `go test -tags webkit2_41 ./backend/data`. |
| `TestSaveLegacySevenPreservesProviderSettings`, `TestSaveCodexElevenSettingsIgnoresInactiveAPIValues`, `TestSaveRejectsInvalidActiveProviderSettings`, `TestSaveRejectsUnknownAndDuplicateKeys` | Legacy/new save shapes, provider-specific validation and strict keys | Passed in `go test -tags webkit2_41 ./backend/service/setting`. |
| `TestGetStatusReportsAPIConfigurationReadiness`, `TestGetStatusUsesReadinessProbeWithoutTestingModelAccess` | Readiness reports configuration state, and a successful local probe is distinct from model access | Passed with a private Codex probe fake; no model request was sent. |
| `TestParseCodexVersionRequiresReviewedBaseline`, `TestCodexExecutableResolutionSupportsSpacesAndMissingCLI`, `TestCodexWindowsScriptWrappersAreRecognized`, `TestCodexIsolationArgsUseFixedArgvAndDisableMCPs`, `TestOwnedCodexProcessKeepsStdoutAndStderrSeparate` | Fixed version, executable path, Windows wrapper rejection, isolation argv, private cwd and stream separation | First run failed because the process helpers were absent; passed after implementation. |
| `TestCodexProtocolRoutesResponsesAndConcurrentServerRequests`, `TestCodexProtocolRoutesUnknownNotificationsAndRejectsUnknownRequests`, `TestCodexProtocolRejectsOversizedFramesAndWrites`, `TestCodexProtocolEOFFailsPendingRequest` | JSON-line framing, pending response ids, server request dispatch, notifications, bounded frames and EOF cleanup | First run failed because the transport was absent; passed after implementation. The oversized-frame test exposed and fixed a missing-newline boundary bug. |
| `TestCodexCandidatePathsResolveAgainstScannedTreeAtomically`, `TestCodexTopUsagesRejectNestedPathsAtomically`, `TestCodexToolSchemasExposeOnlyThreeApplicationTools`, `TestCodexRunnerDuplicateToolCallIdReturnsCachedResult`, `TestCodexRunnerDeduplicatesStreamAndFinalMessage`, `TestCodexRunnerKeepsLatestCumulativeUsageAndRejectsInterruptedTurn` | Exact candidate mapping, non-nested usage paths, strict tool list, idempotency, output de-duplication, cumulative usage snapshots and failure status | Passed in the analyzer package; no disk reads or writes are used by these operations. |
| `TestCodexProviderFullPathSmokeWithFakeCLI` | Provider connection test, analyze path, all three app tools, tool request/response, scan-path size correction, streaming, local readiness, and missing-usage behavior | Passed with a temporary fake executable and the Go test binary as its app-server; no real model service was used by this smoke test. |
| `TestCodexTurnCancellationSendsInterrupt`, `TestCodexRunnerBoundsAccumulatedOutput`, `TestOwnedCodexProcessTerminatesOnlyItsTreeAndWaitsOnce` | Interrupt delivery, bounded output, process termination and one-time Wait behavior | Passed on Linux; Windows code was cross-compiled, but native process-tree execution remains unverified. |

The analyzer race suite and full backend package suite pass. Windows analyzer cross-compilation passes. Frontend `pnpm lint`, `pnpm check`, and `pnpm build` pass. The production build reports the existing large-chunk warning. The actual app UI, Windows-native process behavior, and authenticated Codex full analysis remain unverified.

Fake subprocess/protocol tests are evidence for deterministic transport behavior only; they do not replace real account/model acceptance.


## Dynamic-tool execution correction (2026-10-20)

The original descriptor-only gate did not prove dynamic tool execution. Disabling `code_mode_host` was incorrect: in CLI 0.162.1 the host is required to route dynamic calls, despite the tools remaining present in provider requests. This caused `code-mode host is disabled` on real directory reads. The production argv now explicitly enables the host and continues disabling shell tools, plugins, apps and configured MCP servers.

The installed-CLI integration regression `TestCodexInstalledCLIDynamicToolsRoundTrip` passed with a localhost-only fake Responses endpoint, temporary CODEX_HOME, an initially disabled host and a disabled synthetic MCP entry. It verifies actual `item/tool/call` dispatch, directory result delivery in the next Responses request, a subsequent usage-tool call, and turn completion through production analyzer code. It rejects any model-visible shell/exec function. No account credentials or real model endpoint are used.

A completed analysis with no successful directory read now fails and cannot be persisted as a successful empty report. See `docs/follow-up-fixes.md` for snapshot persistence, retry semantics and legacy-data limitations. Descriptor visibility, runtime tool execution and authenticated-model acceptance are distinct checks; the last remains unverified.
