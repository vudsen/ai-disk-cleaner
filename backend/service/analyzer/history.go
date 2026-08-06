package analyzer

import (
	"bytes"
	"encoding/csv"
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
	return "仅用于清除对话中的扫描历史，不会删除磁盘文件。paths 只能选择已经读取完后代扫描结果、已完成判断且后续不再引用的具体非根子目录；绝对禁止传入 / 根目录，也不要选择仍在分析的目录分支。工具直接过滤历史 tool response CSV 中属于目标严格后代的扫描行，目标路径自身仍会保留，不会修改 assistant 消息"
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
				"description": "要压缩扫描历史的具体非根子目录。仅选择已经读取、完成判断且后续不再使用的目录分支；禁止传入 / 根目录。每项必须以 / 开头并使用绝对路径",
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
	RemovedAnalyzeEntries int `json:"removedAnalyzeEntries"`
}

type messageGroup struct {
	assistant     openai.ChatCompletionMessageParamUnion
	toolResponses []openai.ChatCompletionMessageParamUnion
}

func splitMessageGroup(messages []openai.ChatCompletionMessageParamUnion) []messageGroup {
	result := make([]messageGroup, 0)
	var toolResponses []openai.ChatCompletionMessageParamUnion
	var headAssistantMessage *openai.ChatCompletionMessageParamUnion = nil
	for _, message := range messages {
		if message.OfAssistant != nil {
			if headAssistantMessage != nil {
				result = append(result, messageGroup{
					assistant:     *headAssistantMessage,
					toolResponses: toolResponses,
				})
			}
			headAssistantMessage = &message
			toolResponses = make([]openai.ChatCompletionMessageParamUnion, len(headAssistantMessage.OfAssistant.ToolCalls))
			continue
		}
		if message.OfTool != nil {
			toolResponses = append(toolResponses, message)
		}
	}
	return result
}

func clearAnalyzeHistory(agent *Agent, paths []string) (string, error) {
	groups := splitMessageGroup(agent.messages)
	result := make([]openai.ChatCompletionMessageParamUnion, 0, len(agent.messages))
	result = append(result, agent.messages[0])
	result = append(result, agent.messages[1])

	for _, group := range groups {
		firstAnalyzeRespIndex := -1
		analyzeToolLen := 0
		toolCalls := group.assistant.OfAssistant.ToolCalls
		for i, toolCall := range toolCalls {
			if toolCall.OfFunction.Function.Name == analyzeDirectoryToolName && group.toolResponses[i].OfTool.Content.OfString.String() != analyzeDirectoryRefuseMessage {
				analyzeToolLen++
				if firstAnalyzeRespIndex < 0 {
					firstAnalyzeRespIndex = i
				}
			}
		}
		if firstAnalyzeRespIndex < 0 {
			result = append(result, group.assistant)
			result = append(result, group.toolResponses...)
			continue
		}
		if analyzeToolLen > 1 {
			// do combination.
			builder := strings.Builder{}
			builder.WriteString("path totalSize type\n")
			newCalls := make([]openai.ChatCompletionMessageToolCallUnionParam, 0, len(toolCalls)-analyzeToolLen+1)
			newTools := make([]openai.ChatCompletionMessageParamUnion, 0, len(toolCalls)-analyzeToolLen+1)
			for i, toolCall := range toolCalls {
				function := toolCall.OfFunction.Function
				if function.Name == analyzeDirectoryToolName {
					tool := group.toolResponses[i].OfTool
					content := tool.Content.OfString.String()
					post := strings.Index(content, "\n")
					builder.WriteString(content[post+1:])
					if i == firstAnalyzeRespIndex {
						newCalls = append(newCalls, toolCall)
						newTools = append(newTools, openai.ChatCompletionMessageParamUnion{
							OfTool: &openai.ChatCompletionToolMessageParam{
								Content: openai.ChatCompletionToolMessageParamContentUnion{
									OfString: openai.String(""),
								},
								ToolCallID: tool.ToolCallID,
								Role:       tool.Role,
							},
						})
					}
					break
				}
				newCalls = append(newCalls, toolCall)
				newTools = append(newTools, group.toolResponses[i])
			}
			newTools[firstAnalyzeRespIndex].OfTool.Content.OfString = openai.String(builder.String())
			group.assistant.OfAssistant.ToolCalls = newCalls
		}
		// do clear
		err := doClear(paths, group.toolResponses)
		if err != nil {
			return "", err
		}

		result = append(result, group.assistant)
		result = append(result, group.toolResponses...)
	}
	agent.messages = result
	return "Ok", nil
}

// / 替换分析结果
func doClear(paths []string, toolMessages []openai.ChatCompletionMessageParamUnion) error {

}

func compactAnalyzeHistory(
	messages []openai.ChatCompletionMessageParamUnion,
	targets []string,
) ([]openai.ChatCompletionMessageParamUnion, int, error) {
	result := make([]openai.ChatCompletionMessageParamUnion, 0, len(messages))
	removed := 0
	for _, message := range messages {
		toolResponse := message.OfTool
		if toolResponse == nil || !toolResponse.Content.OfString.Valid() {
			result = append(result, message)
			continue
		}

		filtered, count, ok := filterAnalyzeDirectoryResponseCSV(toolResponse.Content.OfString.Value, targets)
		if !ok || count == 0 {
			result = append(result, message)
			continue
		}
		updatedResponse := *toolResponse
		updatedResponse.Content.OfString.Value = filtered
		message.OfTool = &updatedResponse
		result = append(result, message)
		removed += count
	}
	return result, removed, nil
}

func filterAnalyzeDirectoryResponseCSV(content string, targets []string) (string, int, bool) {
	reader := csv.NewReader(strings.NewReader(content))
	records, err := reader.ReadAll()
	if err != nil || len(records) == 0 {
		return content, 0, false
	}
	header := records[0]
	if len(header) != 3 || header[0] != "path" || header[1] != "totalSize" || header[2] != "type" {
		return content, 0, false
	}

	kept := make([][]string, 0, len(records))
	kept = append(kept, header)
	removed := 0
	for _, record := range records[1:] {
		candidate, err := normalizeAnalyzeHistoryPath(record[0])
		if err == nil && isStrictAnalyzeHistoryDescendant(candidate, targets) {
			removed++
			continue
		}
		kept = append(kept, record)
	}
	if removed == 0 {
		return content, 0, true
	}

	var buffer bytes.Buffer
	writer := csv.NewWriter(&buffer)
	if err := writer.WriteAll(kept); err != nil {
		return content, 0, false
	}
	return buffer.String(), removed, true
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
