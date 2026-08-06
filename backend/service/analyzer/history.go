package analyzer

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/openai/openai-go/v3"
)

const (
	analyzeDirectoryToolName = "analyze_directory"
	compatContextToolName    = "compat_context"
)

type clearAnalyzeHistoryTool struct{}

type clearAnalyzeHistoryParameters struct {
	Paths []string `json:"paths"`
}

func newClearAnalyzeHistoryTool() *clearAnalyzeHistoryTool {
	return &clearAnalyzeHistoryTool{}
}

func (tool *clearAnalyzeHistoryTool) Name() string {
	return compatContextToolName
}

func (tool *clearAnalyzeHistoryTool) Description() string {
	return "整理上下文"
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
	targets, err := normalizeAnalyzeHistoryTargets(arguments.Paths)
	if err != nil {
		return "", fmt.Errorf("normalize analyze history targets: %w", err)
	}
	return clearAnalyzeHistory(agent, targets)
}

func (tool *clearAnalyzeHistoryTool) ParameterSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"paths": map[string]any{
				"type":        "array",
				"description": "要压缩扫描历史的具体非根子目录。",
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
			toolResponses = make([]openai.ChatCompletionMessageParamUnion, 0, len(headAssistantMessage.OfAssistant.ToolCalls))
			continue
		}
		if message.OfTool != nil {
			toolResponses = append(toolResponses, message)
		}
	}
	return result
}
func isCsvHeader(content string) bool {
	return strings.Index(content, "path,totalSize,type") >= 0
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
			if toolCall.OfFunction.Function.Name == analyzeDirectoryToolName && isCsvHeader(group.toolResponses[i].OfTool.Content.OfString.String()) {
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
			builder.WriteString("path,totalSize,type\n")
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
					continue
				}
				newCalls = append(newCalls, toolCall)
				newTools = append(newTools, group.toolResponses[i])
			}
			newTools[firstAnalyzeRespIndex].OfTool.Content.OfString = openai.String(builder.String())
			group.assistant.OfAssistant.ToolCalls = newCalls
			group.toolResponses = newTools
		}
		// do clear
		err := doClear(paths, group.assistant.OfAssistant.ToolCalls, group.toolResponses, analyzeToolLen > 1)
		if err != nil {
			return "", err
		}

		result = append(result, group.assistant)
		result = append(result, group.toolResponses...)
	}
	if agent.messages[len(agent.messages)-1].OfAssistant != nil {
		// clear_analyze_history tool call
		result = append(result, agent.messages[len(agent.messages)-1])
	}
	agent.messages = result
	return "Ok", nil
}

// / 替换分析结果
func doClear(
	targets []string,
	toolCalls []openai.ChatCompletionMessageToolCallUnionParam,
	toolMessages []openai.ChatCompletionMessageParamUnion,
	needSort bool,
) error {
	type replacement struct {
		tool    *openai.ChatCompletionToolMessageParam
		content string
	}
	replacements := make([]replacement, 0)

	for index := range toolMessages {
		if index >= len(toolCalls) || toolCalls[index].OfFunction == nil ||
			toolCalls[index].OfFunction.Function.Name != analyzeDirectoryToolName {
			continue
		}
		toolMessage := toolMessages[index].OfTool
		if toolMessage == nil || !toolMessage.Content.OfString.Valid() {
			continue
		}

		content := toolMessage.Content.OfString.Value
		if !isCsvHeader(content) {
			continue
		}

		reader := csv.NewReader(strings.NewReader(content))
		header, err := reader.Read()
		if err != nil || len(header) != 3 ||
			header[0] != "path" || header[1] != "totalSize" || header[2] != "type" {
			continue
		}
		records, err := reader.ReadAll()
		if err != nil {
			return fmt.Errorf("decode analyze_directory CSV at tool message %d: %w", index, err)
		}

		kept := records[:0]
		changed := false
		for _, record := range records {
			candidate, err := normalizeAnalyzeHistoryPath(record[0])
			if err == nil && isStrictAnalyzeHistoryDescendant(candidate, targets) {
				changed = true
				continue
			}
			kept = append(kept, record)
		}

		if needSort {
			type sortableRecord struct {
				fields    []string
				totalSize int64
			}
			sortable := make([]sortableRecord, len(kept))
			for recordIndex, record := range kept {
				totalSize, err := strconv.ParseInt(record[1], 10, 64)
				if err != nil {
					return fmt.Errorf(
						"decode totalSize in analyze_directory CSV at tool message %d row %d: %w",
						index,
						recordIndex+2,
						err,
					)
				}
				sortable[recordIndex] = sortableRecord{fields: record, totalSize: totalSize}
			}
			sort.Slice(sortable, func(i, j int) bool {
				if sortable[i].totalSize != sortable[j].totalSize {
					return sortable[i].totalSize > sortable[j].totalSize
				}
				return sortable[i].fields[0] < sortable[j].fields[0]
			})
			for recordIndex := range sortable {
				kept[recordIndex] = sortable[recordIndex].fields
			}
		}

		if !changed && !needSort {
			continue
		}
		var buffer bytes.Buffer
		writer := csv.NewWriter(&buffer)
		if err := writer.Write(header); err != nil {
			return fmt.Errorf("encode analyze_directory CSV header at tool message %d: %w", index, err)
		}
		if err := writer.WriteAll(kept); err != nil {
			return fmt.Errorf("encode analyze_directory CSV at tool message %d: %w", index, err)
		}
		replacements = append(replacements, replacement{tool: toolMessage, content: buffer.String()})
	}

	for _, item := range replacements {
		item.tool.Content.OfString = openai.String(item.content)
	}
	return nil
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
