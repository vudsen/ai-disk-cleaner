## ADDED Requirements

### Requirement: Provider choice preserves API compatibility

The application SHALL support api and codex providers, default missing provider settings to api, and preserve inactive-provider values. Saving the legacy seven-key configuration MUST preserve new settings. Runtime validation MUST apply to the selected provider.

#### Scenario: Upgrade an existing API installation
- **WHEN** default settings are installed into an existing database
- **THEN** the existing API values remain unchanged and the provider defaults to api

#### Scenario: Use Codex without API credentials
- **WHEN** codex is selected with a valid executable and settings while API fields are empty or contain stale inactive values
- **THEN** starting analysis and connection testing do not require API URL, key or model

#### Scenario: Switch back to API
- **WHEN** the user switches from codex to api
- **THEN** saved API values are restored and API-specific validation resumes without an automatic fallback from failed Codex analysis

### Requirement: Local executable and authentication

The application SHALL launch the configured local Codex executable using argument arrays and existing CLI-managed authentication. It MUST NOT read authentication tokens, automatically log in, or invoke a user-supplied command string.

#### Scenario: Executable path contains spaces
- **WHEN** a valid executable with spaces in its absolute path is configured
- **THEN** the exact executable starts without shell interpolation

#### Scenario: Windows resolves a command shim
- **WHEN** PATH resolves only a cmd or bat wrapper unsupported by the launcher
- **THEN** the application requests a supported native executable path rather than silently invoking a shell command string

#### Scenario: Login is required
- **WHEN** the configured CLI requires authentication but reports no authenticated account
- **THEN** the application explains how to log in externally and leaves existing authentication files untouched

### Requirement: Versioned bidirectional protocol

The backend SHALL initialize a version-compatible app-server session, process responses, notifications and server requests independently, and provide bounded framing and diagnostics. Experimental capabilities MUST be explicitly negotiated.

#### Scenario: Dynamic tool requires a client response
- **WHEN** item/tool/call arrives with a server request id, callId and JSON arguments
- **THEN** the correct result is returned under the request id without confusing it with a pending client response

#### Scenario: Unsupported CLI or malformed framing
- **WHEN** the CLI cannot support required fields, exits early or emits malformed or oversized protocol data
- **THEN** the operation returns a controlled failure and reaps its owned process

### Requirement: Application-owned tools remain the source of disk data

Codex SHALL use only analyze_directory, add_trash_file and add_top_usages for this workflow. Tools MUST operate on the existing FileTree and per-task results, and MUST NOT rescan or modify disk contents. External tools and general environment execution MUST be disabled as specified in design D5.

#### Scenario: Read a large directory
- **WHEN** Codex invokes analyze_directory
- **THEN** the host returns the existing bounded sorted tree view without reading the real directory again

#### Scenario: Candidate uses a tree path
- **WHEN** the scan root is /home/alice and Codex marks /Downloads/a.zip
- **THEN** the host resolves the scanned node and stores its relative path and actual scanned size rather than treating /Downloads as an operating-system root

#### Scenario: Invalid or repeated marking
- **WHEN** a request includes traversal, unknown nodes, globs, protected non-regular candidates or a repeated callId
- **THEN** invalid batches leave state unchanged and repeated successful calls do not duplicate results

#### Scenario: Codex requests a general command
- **WHEN** an unexpected execution or filesystem permission request occurs
- **THEN** the request is rejected and the task fails clearly without granting extra permissions

### Requirement: Provider-specific context management

Codex SHALL manage its own conversation context and MUST NOT receive the API Agent's compress_context tool or message-reset behavior. API context compression and tool behavior MUST remain compatible.

#### Scenario: API auto-context-compress is enabled
- **WHEN** the user selects Codex while the inactive API compression setting remains enabled
- **THEN** Codex does not receive the application compression tool and the saved API setting remains available for a later switch

### Requirement: Streamed results and truthful completion

Codex messages SHALL feed existing task output events without duplication, and candidates SHALL come from successful tool operations. Only completed turns MUST produce a successful AnalysisResult; failure, interruption and early EOF MUST NOT produce DONE.

#### Scenario: Final text repeats streamed content
- **WHEN** agent-message deltas are followed by a completed item containing the full message
- **THEN** the UI and persisted LLMOutput contain the message once

#### Scenario: Completed turn has no candidates
- **WHEN** a successful analysis legitimately finds no cleanup candidates
- **THEN** it returns empty candidate lists without inventing results from natural-language text

### Requirement: Cancellation and bounded process lifetime

User cancellation, configured deadline and application shutdown SHALL stop tool mutations and output callbacks, interrupt the active turn and terminate only owned processes when necessary.

#### Scenario: User stops analysis during a tool call
- **WHEN** cancellation is requested while tool work or protocol I/O is pending
- **THEN** the task exits as CANCELLED, no late callback changes its state, and subsequent analysis can start

#### Scenario: Analysis deadline expires
- **WHEN** the configured analysis deadline is reached
- **THEN** the operation reports a timeout error, cleans up its process tree and does not label the timeout as user cancellation

### Requirement: Accurate usage and safe diagnostics

Token usage SHALL use the latest cumulative snapshot without summing repeated totals. Missing usage MUST remain unknown in logs. Authentication responses, raw configuration and unrelated stderr contents MUST NOT be exposed as diagnostics.

#### Scenario: Cumulative usage repeats
- **WHEN** total token notifications report 100, 100 and 150
- **THEN** the final known usage is 150 rather than 350

### Requirement: Provider-aware setup and testing

Settings and home-page checks SHALL use the selected provider. Lightweight readiness MUST NOT imply a successful model request. Connection testing MUST use the unsaved form and complete a real minimal turn without altering saved settings.

#### Scenario: Probe passes but model access fails
- **WHEN** CLI discovery and authentication succeed but the actual model request fails
- **THEN** connection testing reports failure and does not claim model access from readiness alone

#### Scenario: User tests an unsaved local configuration
- **WHEN** the Codex model or executable differs from the saved value
- **THEN** the test uses the form value and the saved configuration remains unchanged

### Requirement: Windows and Linux evidence

The change SHALL provide deterministic fake-process tests, existing API regression checks and documented real local-Codex acceptance. Platform-specific or authenticated checks that cannot run MUST be reported as unverified.

#### Scenario: CI has no Codex login
- **WHEN** automated tests run without a local authenticated Codex installation
- **THEN** protocol and provider tests use controlled fake processes and real model acceptance remains separately identified
