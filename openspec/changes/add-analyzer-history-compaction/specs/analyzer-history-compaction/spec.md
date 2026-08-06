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

### Requirement: Compaction removes only strict descendant scan records
For each normalized target path, the tool MUST remove historical analyze_directory calls whose requested path is a strict descendant of that target. It MUST retain scans of the target itself, its ancestors, unrelated paths, and paths that merely share a textual prefix.

#### Scenario: Preserve target and remove descendants
- **GIVEN** history contains scans for /foo/bar, /foo/bar/child, and /foo/bar/child/file
- **WHEN** clear_analyze_history is invoked with paths containing /foo/bar
- **THEN** the scans for /foo/bar/child and /foo/bar/child/file are removed
- **AND** the scan for /foo/bar remains

#### Scenario: Preserve similar prefix
- **GIVEN** history contains scans for /foo/bar/child and /foo/bar2/child
- **WHEN** clear_analyze_history is invoked for /foo/bar
- **THEN** the scan for /foo/bar/child is removed
- **AND** the scan for /foo/bar2/child remains

#### Scenario: Normalize paths
- **GIVEN** history contains a scan for /foo/bar/child
- **WHEN** the tool is invoked with an equivalent target containing redundant separators, dot segments, or a trailing slash
- **THEN** descendant matching uses the normalized logical path
- **AND** the child scan is removed

#### Scenario: Root target
- **GIVEN** history contains scans for / and /foo
- **WHEN** the tool is invoked for /
- **THEN** the scan for /foo is removed
- **AND** the scan for / remains

#### Scenario: Multiple overlapping targets
- **GIVEN** history contains scans below multiple target paths
- **WHEN** the tool is invoked with duplicate or overlapping target paths
- **THEN** matching scan records are removed according to the union of all targets
- **AND** each matching scan record is counted at most once

### Requirement: Compaction preserves message-history integrity
The tool MUST delete a matched analyze_directory function call together with its tool result identified by the same tool-call ID. It MUST preserve unrelated message content and tool calls, including the clear_analyze_history call and its result.

#### Scenario: Remove paired call and result
- **GIVEN** a completed matching analyze_directory call and tool result share a tool-call ID
- **WHEN** history compaction runs
- **THEN** both the function call and its corresponding result are removed
- **AND** no orphaned result for that call remains

#### Scenario: Preserve mixed assistant message
- **GIVEN** an assistant message contains a matching analyze_directory call and one or more unrelated tool calls or text
- **WHEN** history compaction runs
- **THEN** only the matching call and its paired result are removed
- **AND** the unrelated calls, their results, and the assistant text remain

#### Scenario: Preserve unsafe-to-classify records
- **GIVEN** an analyze_directory call has malformed arguments or cannot be safely paired with a result
- **WHEN** history compaction runs
- **THEN** the call and any uncertain related messages remain unchanged

#### Scenario: Repeat compaction
- **WHEN** the same valid compaction request is invoked more than once without adding new matching scans
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
