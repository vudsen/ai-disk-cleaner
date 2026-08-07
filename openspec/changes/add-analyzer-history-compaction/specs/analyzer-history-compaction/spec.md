## ADDED Requirements

### Requirement: Agent owns mutable analysis history
The analyzer Agent MUST own the message history used for every model completion, and every analyzer tool MUST receive the active Agent instance.

#### Scenario: Completion uses Agent history
- **WHEN** the analyzer performs a model completion
- **THEN** the request uses the active Agent's current message history

### Requirement: LLM can request a fresh context
The analyzer MUST expose a strict function tool named compress_context whose only parameter is a required non-empty summary string.

#### Scenario: Tool contract
- **WHEN** compress_context is advertised
- **THEN** its schema requires summary
- **AND** it does not define paths or any other property

#### Scenario: Invalid input is atomic
- **WHEN** summary is missing or empty, or an unknown property is supplied
- **THEN** the tool returns an error
- **AND** Agent state and history remain unchanged

### Requirement: Summary hands off scan coverage
The summary guidance MUST require the directories already searched and the directories remaining unsearched. Already searched directories MUST be marked as forbidden from subsequent scans.

#### Scenario: Summary guidance
- **WHEN** the model sees the tool definition or runtime warning
- **THEN** it is instructed to list already searched directories that must not be scanned again
- **AND** it is instructed to list remaining unsearched directories

### Requirement: Compaction starts a clean conversation
Successful compaction MUST replace the old history with exactly one base system message and one user message containing the summary. It MUST NOT preserve or synthesize assistant or tool messages.

#### Scenario: Fresh history
- **GIVEN** history contains prior user, assistant, and tool messages
- **WHEN** compress_context succeeds
- **THEN** all old messages are discarded
- **AND** the new history contains only the base system prompt and summary user prompt

#### Scenario: Current tool result is skipped
- **WHEN** compress_context succeeds inside the Agent run loop
- **THEN** its normal tool result is not appended to the new history

### Requirement: Fresh context re-enables scanning
Successful compaction MUST reset the current context state to Low and current-context token usage to zero while retaining cumulative token usage and accumulated analysis results.

#### Scenario: Continue scanning
- **GIVEN** the Agent is in High state
- **WHEN** compress_context succeeds
- **THEN** the next completion advertises analyze_directory again
- **AND** cumulative usedTokens, TrashFiles, and TopUsages remain unchanged

### Requirement: Tool availability follows Agent context state
Every analyzer tool MUST declare whether it supports the active Agent, and advertised definitions MUST be rebuilt for every completion.

#### Scenario: Low context
- **WHEN** the Agent state is Low
- **THEN** analyze_directory is included and compress_context is excluded

#### Scenario: Medium context
- **WHEN** the Agent state is Medium
- **THEN** analyze_directory and compress_context are included

#### Scenario: High context
- **WHEN** the Agent state is High
- **THEN** compress_context is included and analyze_directory is excluded

### Requirement: Directory analysis depth expands descendant levels
The analyzer MUST interpret analyze_directory depth as the number of descendant levels to expand while including the requested node.

#### Scenario: Root depth one exposes direct children
- **WHEN** analyze_directory is invoked with path / and depth 1
- **THEN** the result includes the root and its direct children but not grandchildren

#### Scenario: Depth two exposes grandchildren
- **WHEN** analyze_directory is invoked with depth 2
- **THEN** the result includes the requested node, children, and grandchildren
