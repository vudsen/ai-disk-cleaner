## ADDED Requirements

### Requirement: Native Linux platform entrypoints

The application SHALL compile natively for Linux amd64 using the existing Go, Wails and gdu dependencies. Platform bridge methods MUST delegate to the centrally initialized platform service. Windows public bridge signatures MUST remain compatible.

#### Scenario: Linux platform methods are available
- **WHEN** the Linux application initializes services and invokes GetDisks, GetPlatformInfo and IsRunningAsAdministrator
- **THEN** each method resolves to a correctly exported Linux implementation and reports the actual platform, home directory and effective-UID privilege state

#### Scenario: Windows behavior remains available
- **WHEN** the Windows test and build jobs run
- **THEN** drive enumeration and privilege checks retain their existing behavior without duplicate build-tag implementations

### Requirement: Mounted filesystem enumeration

Linux disk enumeration SHALL parse mountinfo with escaped fields and compute capacity with Statfs using the formulas and filtering policy in design D3. It MUST preserve valid root mounts and return stable unique mount paths.

#### Scenario: Mount paths contain escaped characters
- **WHEN** mountinfo contains optional fields and a mount path with escaped spaces or backslashes
- **THEN** the correct decoded path is returned once without splitting it into multiple paths

#### Scenario: One mount becomes inaccessible
- **WHEN** Statfs fails for one mount while other valid mounts remain
- **THEN** enumeration returns the remaining mounts without failing the entire list

#### Scenario: Reserved filesystem blocks exist
- **WHEN** Bavail differs from Bfree
- **THEN** free reports bytes available to the user and used reports allocated bytes without forcing free plus used to equal total

### Requirement: Valid platform defaults and privilege experience

The home page SHALL default to the Linux user's home directory, preserve Windows system-drive defaults, and display capacity for the selected target's containing mount. Linux normal-user startup MUST NOT trigger a Windows administrator warning.

#### Scenario: Ordinary Linux user starts the application
- **WHEN** platform information is loaded
- **THEN** custom-directory mode selects the valid home directory and the application does not request elevation

#### Scenario: User changes selection during loading
- **WHEN** asynchronous platform or disk information arrives after the user changed the scan target
- **THEN** initialization does not overwrite that choice

#### Scenario: Mount list is empty
- **WHEN** no eligible mounts are returned
- **THEN** the user can still choose a custom directory and no C-drive target is fabricated

### Requirement: Bounded Linux scans

Linux scans SHALL remain within the selected mount, exclude the virtual system paths in design D5, avoid following symbolic links and preserve the distinction between apparent and allocated size. Nested bind mounts MUST be excluded even when the device number matches the root.

#### Scenario: Scan the filesystem root
- **WHEN** the user explicitly scans `/`
- **THEN** virtual paths and nested mounts are excluded and their contents are not traversed

#### Scenario: Scan an explicit mounted home directory
- **WHEN** `/home` is a separate mount and is explicitly selected
- **THEN** its ordinary contents are scanned while its nested mounts remain excluded

#### Scenario: Ordinary directory is named proc
- **WHEN** the selected tree contains `/home/alice/project/proc`
- **THEN** it is not excluded merely because its basename is proc

#### Scenario: Links and sparse files occur
- **WHEN** the tree contains a directory symlink, two hard links and a sparse file
- **THEN** the symlink target is not traversed, hard-linked storage is counted once in the single filesystem, and sparse-file logical size differs from allocated size when supported

### Requirement: Partial scan disclosure and cancellation

The scanner SHALL distinguish fatal root failures, policy exclusions and recoverable child read failures. It MUST carry a bounded summary through task snapshots, logs, persisted history and the details page, and MUST stop project-owned progress callbacks before returning.

#### Scenario: Child directory cannot be read
- **WHEN** an ordinary user scans a readable root containing an EACCES child
- **THEN** accessible contents remain available and the read-error count and incomplete-scan message remain visible after reopening history

#### Scenario: Root cannot be read
- **WHEN** the selected root is unreadable or belongs to the forbidden system paths
- **THEN** the task fails clearly rather than presenting an empty successful scan

#### Scenario: User cancels an active scan
- **WHEN** cancellation is requested during scanning or a later pipeline phase
- **THEN** the operation exits with cancellation, never advances to LLM analysis and emits no progress callback after the scan returns

#### Scenario: Candidate contains an excluded subtree
- **WHEN** a recursive cleanup candidate contains a protected mount, excluded subtree or unsupported symlink path component
- **THEN** cleanup rejects that candidate using the existing failure response instead of traversing the boundary

### Requirement: Linux path identity

Native Linux path identity SHALL preserve case. POSIX tree paths MUST preserve backslashes inside filenames, and Windows-style reports MUST retain their existing interpretation even when parsed on Linux. Cleanup, migration and history matching MUST use the same platform identity rules.

#### Scenario: Case-distinct siblings exist
- **WHEN** a Linux directory contains both Cache and cache
- **THEN** querying, marking, migrating or updating one never aliases the other

#### Scenario: POSIX filename contains a backslash
- **WHEN** a POSIX tree contains the filename `a\b.txt`
- **THEN** scanning, analyze_directory, result display and file operations all refer to that exact filename

#### Scenario: Windows fixture is parsed on Linux
- **WHEN** existing Windows-rooted JSON fixtures are parsed by Linux tests
- **THEN** their full paths and normalized lookup behavior remain compatible

### Requirement: Writable user data and logs

Linux persistent paths SHALL follow design D7, ignore relative XDG overrides and be independent of cwd and executable-directory writability. Windows locations MUST remain unchanged. Legacy databases MUST survive additive schema migration without automatic file relocation.

#### Scenario: Start from a read-only installation directory
- **WHEN** a normal user starts the installed Linux binary from a directory they cannot write
- **THEN** database and task logs are written to the user's configured data and state paths

#### Scenario: Relative XDG override is present
- **WHEN** XDG_DATA_HOME or XDG_STATE_HOME is relative
- **THEN** the corresponding absolute home fallback is used

#### Scenario: Existing record lacks scan summary
- **WHEN** a pre-change database copy is opened and migrated
- **THEN** its records remain readable and missing scan summaries are handled as empty

### Requirement: Native Linux build and documented distribution

CI SHALL test Windows and Linux, build a native Ubuntu 24.04 amd64 Wails artifact with WebKitGTK 4.1, and prepare a versioned Linux tarball with checksum. Documentation MUST distinguish a successful build from completed desktop acceptance.

#### Scenario: Linux build job runs
- **WHEN** frontend checks and tagged Go tests succeed with the documented dependencies
- **THEN** Wails v2.12.0 produces the Linux artifact without changing the existing Windows release job

#### Scenario: Only headless validation is available
- **WHEN** no graphical desktop is available for manual acceptance
- **THEN** the report lists desktop verification as pending and does not claim full Linux acceptance
