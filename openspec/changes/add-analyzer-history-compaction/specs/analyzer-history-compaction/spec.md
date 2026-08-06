## ADDED Requirements

### Requirement: Agent owns mutable analysis history
The analyzer Agent MUST own the message history used for every model completion, and every registered analyzer tool MUST receive the active Agent instance when invoked so that tools can directly read or mutate the active analysis state.

#### Scenario: Completion uses Agent history
- **WHEN** the analyzer starts and performs one or more model completions
- **THEN** the system builds each completion request from the message history stored on the active Agent
- **AND** assistant messages and tool results are appended to that same history

#### Scenario: Existing tools use Agent state
- **WHEN** an existing analyzer tool is invoked
- **THEN** it reads required inputs such as the file tree from the active Agent
- **AND** it writes top-usage or trash-file results directly to the active Agent

### Requirement: LLM can request analysis-history compaction
The analyzer MUST expose a strict function tool named clear_analyze_history with a required paths array of logical path strings. The tool MUST validate every path before mutating history and MUST reject invalid input without partially changing history.

#### Scenario: Tool is advertised
- **WHEN** the analyzer builds the tool definitions for a model completion
- **THEN** clear_analyze_history is included with a schema that requires paths
- **AND** every path is described as starting with / and using / separators

#### Scenario: Invalid path is atomic
- **WHEN** any supplied path is empty, does not start with /, or contains a backslash
- **THEN** the tool returns an error
- **AND** no message is removed from Agent history

#### Scenario: No matching scan
- **WHEN** all supplied paths are valid but no removable scan record matches
- **THEN** the invocation succeeds
- **AND** the result reports that zero scan calls were removed

### Requirement: Compaction removes only strict descendant CSV rows
For each normalized target path, the tool MUST scan historical tool response content that uses the analyze_directory CSV format and remove rows whose path is a strict descendant of that target. It MUST retain rows for the target itself, its ancestors, unrelated paths, and paths that merely share a textual prefix.

#### Scenario: Preserve target and remove descendants
- **GIVEN** a historical tool response CSV contains rows for /foo/bar, /foo/bar/child, and /foo/bar/child/file
- **WHEN** clear_analyze_history is invoked with paths containing /foo/bar
- **THEN** the rows for /foo/bar/child and /foo/bar/child/file are removed
- **AND** the row for /foo/bar remains

#### Scenario: Preserve similar prefix
- **GIVEN** a historical tool response CSV contains rows for /foo/bar/child and /foo/bar2/child
- **WHEN** clear_analyze_history is invoked for /foo/bar
- **THEN** the row for /foo/bar/child is removed
- **AND** the row for /foo/bar2/child remains

#### Scenario: Normalize paths
- **GIVEN** a historical tool response CSV contains a row for /foo/bar/child
- **WHEN** the tool is invoked with an equivalent target containing redundant separators, dot segments, or a trailing slash
- **THEN** descendant matching uses the normalized logical path
- **AND** the child row is removed

#### Scenario: Root target
- **GIVEN** a historical tool response CSV contains rows for / and /foo
- **WHEN** the tool is invoked for /
- **THEN** the row for /foo is removed
- **AND** the row for / remains

#### Scenario: Multiple overlapping targets
- **GIVEN** historical tool response CSV rows exist below multiple target paths
- **WHEN** the tool is invoked with duplicate or overlapping target paths
- **THEN** matching CSV rows are removed according to the union of all targets
- **AND** each matching row is counted at most once

### Requirement: Compaction preserves message-history integrity
The tool MUST filter matching rows directly from parseable analyze_directory CSV tool responses. It MUST NOT parse or modify assistant messages, and it MUST retain every tool response message and tool-call ID.

#### Scenario: Filter response without changing message pairing
- **GIVEN** a tool response contains matching and non-matching analyze_directory CSV rows
- **WHEN** history compaction runs
- **THEN** only matching CSV rows are removed from the response content
- **AND** the assistant tool call, tool response message, and tool-call ID remain

#### Scenario: Ignore assistant arguments
- **GIVEN** an assistant tool call has malformed or unrelated arguments while its tool response contains a valid analyze_directory CSV
- **WHEN** history compaction runs
- **THEN** matching is determined only from paths in the tool response CSV
- **AND** the assistant message remains byte-for-byte equivalent

#### Scenario: Preserve non-CSV response
- **GIVEN** a tool response is malformed, non-CSV, or does not have the analyze_directory CSV header
- **WHEN** history compaction runs
- **THEN** the response remains unchanged

#### Scenario: Repeat compaction
- **WHEN** the same valid compaction request is invoked more than once without adding new matching CSV rows
- **THEN** subsequent invocations succeed with zero additional removals
- **AND** the remaining history is unchanged

### Requirement: Tool availability follows Agent context state
Every analyzer tool MUST declare whether it supports the active Agent, and the analyzer MUST rebuild the advertised tool definitions for every model completion using that support decision.

#### Scenario: Low context exposes scanning but not compaction
- **WHEN** the Agent state is low
- **THEN** analyze_directory is included in the advertised tools
- **AND** clear_analyze_history is not included

#### Scenario: Medium context exposes scanning and compaction
- **WHEN** the Agent state is medium
- **THEN** both analyze_directory and clear_analyze_history are included in the advertised tools

#### Scenario: High context exposes compaction but not scanning
- **WHEN** the Agent state is high
- **THEN** clear_analyze_history is included in the advertised tools
- **AND** analyze_directory is not included

#### Scenario: State change affects the next completion
- **WHEN** the Agent context state changes between model completions
- **THEN** the next completion request uses tool definitions filtered for the new state

### Requirement: Model guidance favors narrow completed branches
Whenever clear_analyze_history is presented or recommended to the model, the guidance MUST instruct the model to select only specific non-root directories whose descendant scan results have already been read, whose analysis is complete, and which will no longer be referenced. The guidance MUST explicitly tell the model not to pass /.

#### Scenario: Tool definition discourages root-wide clearing
- **WHEN** clear_analyze_history is included in the advertised tools
- **THEN** its description and paths parameter guidance explicitly prohibit the / root path
- **AND** they direct the model toward completed, no-longer-needed child directory branches

#### Scenario: Runtime context warning recommends safe compaction
- **WHEN** the Agent emits a medium or high context-usage instruction
- **THEN** the instruction recommends clearing only previously read and no-longer-needed non-root child directories
- **AND** it explicitly says never to pass /

### Requirement: Directory analysis depth expands descendant levels
The analyzer MUST interpret analyze_directory depth as the number of descendant levels to expand while also including the requested node in its CSV result.

#### Scenario: Root depth one exposes direct children
- **GIVEN** the file-tree root contains direct children and deeper descendants
- **WHEN** analyze_directory is invoked with path / and depth 1
- **THEN** the CSV result includes the root and its direct children
- **AND** it does not include grandchildren

#### Scenario: Depth two exposes grandchildren
- **GIVEN** a requested directory contains children and grandchildren
- **WHEN** analyze_directory is invoked with depth 2
- **THEN** the CSV result includes the requested node, its children, and its grandchildren
