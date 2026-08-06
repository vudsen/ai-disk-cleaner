package analyzer

import (
	"encoding/json"
	"fmt"
	"path"
	"strings"

	"github.com/openai/openai-go/v3"
)

const (
	analyzeDirectoryToolName    = "analyze_directory"
	clearAnalyzeHistoryToolName = "clear_analyze_history"
)

type clearAnalyzeHistoryTool struct{}

type clearAnalyzeHistoryParameters struct {
	Paths []string `json:"paths"`
}

func newClearAnalyzeHistoryTool() *clearAnalyzeHistoryTool {
	return &clearAnalyzeHistoryTool{}
}

func (tool *clearAnalyzeHistoryTool) Name() string {
	return clearAnalyzeHistoryToolName
}

func (tool *clearAnalyzeHistoryTool) Description() string {
	return "清除每个目标路径严格后代的历史 analyze_directory 调用及其配对结果，以压缩上下文；目标路径自身、祖先路径和无关路径的扫描历史仍会保留"
}

func (tool *clearAnalyzeHistoryTool) IsSupport(agent *Agent) bool {
	return agent.state != agentContextStateLow
}

func (tool *clearAnalyzeHistoryTool) invoke(agent *Agent, parameter string) (string, error) {
	var arguments clearAnalyzeHistoryParameters
	if err := json.Unmarshal([]byte(parameter), &arguments); err != nil {
		return "", fmt.Errorf("decode clear_analyze_history parameters: %w", err)
	}
	if arguments.Paths == nil {
		return "", fmt.Errorf("paths is required")
	}
	return clearAnalyzeHistory(agent, arguments.Paths)
}

func (tool *clearAnalyzeHistoryTool) ParameterSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"paths": map[string]any{
				"type":        "array",
				"description": "要压缩的文件树逻辑路径；每项必须以 / 开头、使用绝对路径",
				"items": map[string]any{
					"type":    "string",
					"pattern": "^/[^\\\\]*$",
				},
			},
		},
		"required":             []string{"paths"},
		"additionalProperties": false,
	}
}

type clearAnalyzeHistoryResult struct {
	RemovedAnalyzeCalls int `json:"removedAnalyzeCalls"`
}

type historyToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

func clearAnalyzeHistory(agent *Agent, paths []string) (string, error) {
	targets, err := normalizeAnalyzeHistoryTargets(paths)
	if err != nil {
		return "", err
	}
	if agent == nil {
		return "", fmt.Errorf("agent is not available")
	}

	messages, removed, err := compactAnalyzeHistory(agent.messages, targets)
	if err != nil {
		return "", err
	}
	result, err := json.Marshal(clearAnalyzeHistoryResult{RemovedAnalyzeCalls: removed})
	if err != nil {
		return "", fmt.Errorf("encode clear analyze history result: %w", err)
	}
	agent.messages = messages
	return string(result), nil
}

func compactAnalyzeHistory(
	messages []openai.ChatCompletionMessageParamUnion,
	targets []string,
) ([]openai.ChatCompletionMessageParamUnion, int, error) {
	callCounts := make(map[string]int)
	candidateCounts := make(map[string]int)
	resultCounts := make(map[string]int)

	for _, message := range messages {
		fields, role, err := decodeHistoryMessage(message)
		if err != nil {
			return nil, 0, err
		}
		switch role {
		case "assistant":
			calls, err := decodeHistoryToolCalls(fields)
			if err != nil {
				return nil, 0, err
			}
			for _, rawCall := range calls {
				var call historyToolCall
				if err := json.Unmarshal(rawCall, &call); err != nil || call.ID == "" {
					continue
				}
				callCounts[call.ID]++
				if isAnalyzeHistoryRemovalCandidate(call, targets) {
					candidateCounts[call.ID]++
				}
			}
		case "tool":
			if toolCallID := decodeHistoryString(fields["tool_call_id"]); toolCallID != "" {
				resultCounts[toolCallID]++
			}
		}
	}

	removable := make(map[string]struct{})
	for id, count := range candidateCounts {
		if count == 1 && callCounts[id] == 1 && resultCounts[id] == 1 {
			removable[id] = struct{}{}
		}
	}
	if len(removable) == 0 {
		return append([]openai.ChatCompletionMessageParamUnion(nil), messages...), 0, nil
	}

	result := make([]openai.ChatCompletionMessageParamUnion, 0, len(messages))
	for _, message := range messages {
		fields, role, err := decodeHistoryMessage(message)
		if err != nil {
			return nil, 0, err
		}
		switch role {
		case "assistant":
			calls, err := decodeHistoryToolCalls(fields)
			if err != nil {
				return nil, 0, err
			}
			kept := make([]json.RawMessage, 0, len(calls))
			changed := false
			for _, rawCall := range calls {
				var call historyToolCall
				if err := json.Unmarshal(rawCall, &call); err == nil {
					if _, ok := removable[call.ID]; ok {
						changed = true
						continue
					}
				}
				kept = append(kept, rawCall)
			}
			if !changed {
				result = append(result, message)
				continue
			}
			if len(kept) == 0 {
				delete(fields, "tool_calls")
			} else {
				encodedCalls, err := json.Marshal(kept)
				if err != nil {
					return nil, 0, fmt.Errorf("encode assistant tool calls: %w", err)
				}
				fields["tool_calls"] = encodedCalls
			}
			if len(kept) == 0 && !assistantHistoryMessageHasContent(fields) {
				continue
			}
			rebuilt, err := encodeHistoryMessage(fields)
			if err != nil {
				return nil, 0, err
			}
			result = append(result, rebuilt)
		case "tool":
			toolCallID := decodeHistoryString(fields["tool_call_id"])
			if _, ok := removable[toolCallID]; !ok {
				result = append(result, message)
			}
		default:
			result = append(result, message)
		}
	}
	return result, len(removable), nil
}

func isAnalyzeHistoryRemovalCandidate(call historyToolCall, targets []string) bool {
	if call.Function.Name != analyzeDirectoryToolName {
		return false
	}
	var parameters analyzeDirectoryParameters
	if err := json.Unmarshal([]byte(call.Function.Arguments), &parameters); err != nil || parameters.Depth < 1 {
		return false
	}
	candidate, err := normalizeAnalyzeHistoryPath(parameters.Path)
	if err != nil {
		return false
	}
	return isStrictAnalyzeHistoryDescendant(candidate, targets)
}

func decodeHistoryMessage(
	message openai.ChatCompletionMessageParamUnion,
) (map[string]json.RawMessage, string, error) {
	data, err := json.Marshal(message)
	if err != nil {
		return nil, "", fmt.Errorf("encode analysis history message: %w", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, "", fmt.Errorf("decode analysis history message: %w", err)
	}
	return fields, decodeHistoryString(fields["role"]), nil
}

func encodeHistoryMessage(fields map[string]json.RawMessage) (openai.ChatCompletionMessageParamUnion, error) {
	data, err := json.Marshal(fields)
	if err != nil {
		return openai.ChatCompletionMessageParamUnion{}, fmt.Errorf("encode compacted history message: %w", err)
	}
	var result openai.ChatCompletionMessageParamUnion
	if err := json.Unmarshal(data, &result); err != nil {
		return openai.ChatCompletionMessageParamUnion{}, fmt.Errorf("decode compacted history message: %w", err)
	}
	return result, nil
}

func decodeHistoryToolCalls(fields map[string]json.RawMessage) ([]json.RawMessage, error) {
	raw, ok := fields["tool_calls"]
	if !ok || string(raw) == "null" {
		return nil, nil
	}
	var result []json.RawMessage
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("decode assistant tool calls: %w", err)
	}
	return result, nil
}

func decodeHistoryString(raw json.RawMessage) string {
	var result string
	_ = json.Unmarshal(raw, &result)
	return result
}

func assistantHistoryMessageHasContent(fields map[string]json.RawMessage) bool {
	for name, value := range fields {
		switch name {
		case "role", "tool_calls", "name":
			continue
		}
		if historyJSONHasContent(value) {
			return true
		}
	}
	return false
}

func historyJSONHasContent(raw json.RawMessage) bool {
	var value any
	if len(raw) == 0 || json.Unmarshal(raw, &value) != nil {
		return false
	}
	switch value := value.(type) {
	case nil:
		return false
	case string:
		return value != ""
	case []any:
		return len(value) > 0
	case map[string]any:
		return len(value) > 0
	default:
		return true
	}
}

func normalizeAnalyzeHistoryTargets(paths []string) ([]string, error) {
	result := make([]string, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for index, value := range paths {
		normalized, err := normalizeAnalyzeHistoryPath(value)
		myLog.Println("Cleaning history of", value)
		if err != nil {
			return nil, fmt.Errorf("invalid path at index %d: %w", index, err)
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}
	return result, nil
}

func normalizeAnalyzeHistoryPath(value string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("path must not be empty")
	}
	if !strings.HasPrefix(value, "/") {
		return "", fmt.Errorf("path must start with '/': %q", value)
	}
	if strings.Contains(value, `\`) {
		return "", fmt.Errorf("path must use '/' as its separator: %q", value)
	}
	return path.Clean(value), nil
}

func isStrictAnalyzeHistoryDescendant(candidate string, targets []string) bool {
	for _, target := range targets {
		if candidate == target {
			continue
		}
		if target == "/" {
			if strings.HasPrefix(candidate, "/") {
				return true
			}
			continue
		}
		if strings.HasPrefix(candidate, target+"/") {
			return true
		}
	}
	return false
}
