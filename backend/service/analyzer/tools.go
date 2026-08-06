package analyzer

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"ai-disk-cleanner/backend/data/models/cleaningrecord"
	modelscanner "ai-disk-cleanner/backend/model/scanner"

	"github.com/invopop/jsonschema"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/shared"
)

func buildTools(manager *toolsManager, agent *Agent) []openai.ChatCompletionToolUnionParam {
	result := make([]openai.ChatCompletionToolUnionParam, 0, len(manager.tools))
	for _, tool := range manager.tools {
		if !tool.IsSupport(agent) {
			continue
		}
		result = append(result, openai.ChatCompletionToolUnionParam{
			OfFunction: &openai.ChatCompletionFunctionToolParam{
				Function: shared.FunctionDefinitionParam{
					Name:        tool.Name(),
					Strict:      openai.Bool(true),
					Description: openai.String(tool.Description()),
					Parameters:  tool.ParameterSchema(),
				},
			},
		})
	}
	return result
}

type tool interface {
	Name() string
	Description() string
	IsSupport(agent *Agent) bool
	invoke(agent *Agent, parameter string) (string, error)
	ParameterSchema() map[string]any
}

type toolsManager struct {
	toolMap map[string]tool
	tools   []tool
}

func newManager() *toolsManager {
	tools := []tool{
		newAddTrashFileTool(),
		newAddUsageTool(),
		newAnalyzeDirectoryTool(),
		newClearAnalyzeHistoryTool(),
	}
	toolMap := make(map[string]tool)
	for _, tool := range tools {
		toolMap[tool.Name()] = tool
	}
	return &toolsManager{toolMap: toolMap, tools: tools}
}

func (manager *toolsManager) Invoke(toolName string, parameter string, agent *Agent) (string, error) {
	tool, ok := manager.toolMap[toolName]
	if !ok {
		return "", fmt.Errorf("tool '%s' not found", toolName)
	}
	return tool.invoke(agent, parameter)
}

type addTopUsagesTool struct {
	mySchema map[string]any
}

type addTopUsagesParameters struct {
	Usages []cleaningrecord.DiskUsage `json:"usages"`
}

func newAddUsageTool() *addTopUsagesTool {
	reflector := jsonschema.Reflector{
		AllowAdditionalProperties: false,
		DoNotReference:            true,
	}
	var v addTopUsagesParameters
	schema := reflector.Reflect(v)
	data, err := json.Marshal(schema)
	if err != nil {
		panic(err)
	}
	var result map[string]any
	err = json.Unmarshal(data, &result)
	if err != nil {
		panic(err)
	}
	return &addTopUsagesTool{result}
}

func (tool *addTopUsagesTool) ParameterSchema() map[string]any {
	return tool.mySchema
}

func (tool *addTopUsagesTool) Name() string {
	return "add_top_usages"
}

func (tool *addTopUsagesTool) Description() string {
	return "设置当前磁盘占用最高的地方，每次调用都将覆盖之前的结果"
}

func (tool *addTopUsagesTool) IsSupport(agent *Agent) bool {
	return true
}

func (tool *addTopUsagesTool) invoke(agent *Agent, parameter string) (string, error) {
	var v addTopUsagesParameters
	err := json.Unmarshal([]byte(parameter), &v)
	if err != nil {
		return "", err
	}
	agent.TopUsages = v.Usages
	return "true", nil
}

type addTrashFileTool struct {
	mySchema map[string]any
}

type addTrashFileParameters struct {
	Files []cleaningrecord.TrashFile `json:"files"`
}

func newAddTrashFileTool() *addTrashFileTool {
	reflector := jsonschema.Reflector{
		AllowAdditionalProperties: false,
		DoNotReference:            true,
	}
	var v addTrashFileParameters
	schema := reflector.Reflect(v)
	data, err := json.Marshal(schema)
	if err != nil {
		panic(err)
	}
	var result map[string]any
	err = json.Unmarshal(data, &result)
	if err != nil {
		panic(err)
	}
	return &addTrashFileTool{result}
}

func (tool *addTrashFileTool) ParameterSchema() map[string]any {
	return tool.mySchema
}

func (tool *addTrashFileTool) Name() string {
	return "add_trash_file"
}

func (tool *addTrashFileTool) Description() string {
	return "添加建议删除、移动、跳过或需要用户进一步确认的文件和目录；每项必须提供说明标题、建议或原因、路径和风险等级，多次调用会合并结果，并移除已被父候选路径包含的子项"
}

func (tool *addTrashFileTool) IsSupport(agent *Agent) bool {
	return true
}

func (tool *addTrashFileTool) invoke(agent *Agent, parameter string) (string, error) {
	var v addTrashFileParameters
	if err := json.Unmarshal([]byte(parameter), &v); err != nil {
		return "", err
	}
	agent.TrashFiles = removeNestedTrashFiles(append(agent.TrashFiles, v.Files...))
	return "true", nil
}

func removeNestedTrashFiles(files []cleaningrecord.TrashFile) []cleaningrecord.TrashFile {
	result := make([]cleaningrecord.TrashFile, 0, len(files))
	for i, file := range files {
		duplicate := false
		contained := false
		for j, other := range files {
			if i == j {
				continue
			}
			if samePath(file.Path, other.Path) {
				duplicate = j < i
				continue
			}
			if isPathInside(file.Path, other.Path) {
				contained = true
				break
			}
		}
		if !duplicate && !contained {
			result = append(result, file)
		}
	}
	return result
}

func samePath(left string, right string) bool {
	left = normalizePath(left)
	right = normalizePath(right)
	return left != "" && left == right
}

func isPathInside(child string, parent string) bool {
	child = normalizePath(child)
	parent = normalizePath(parent)
	if child == "" || parent == "" || child == parent {
		return false
	}
	relative, err := filepath.Rel(parent, child)
	if err != nil || relative == ".." || filepath.IsAbs(relative) {
		return false
	}
	return !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func normalizePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	path = filepath.Clean(filepath.FromSlash(path))
	if runtime.GOOS == "windows" {
		path = strings.ToLower(path)
	}
	return path
}

const maxDirectoryEntries = 200

type analyzeDirectoryTool struct{}

type analyzeDirectoryParameters struct {
	Path string `json:"path"`
}

type directoryUsageEntry struct {
	path      string
	totalSize int64
	typeID    int
}

func newAnalyzeDirectoryTool() *analyzeDirectoryTool {
	return &analyzeDirectoryTool{}
}

func (g *analyzeDirectoryTool) Name() string {
	return "analyze_directory"
}

const analyzeDirectoryRefuseMessage = "this tool is disabled due to context limitation! Either stop scanning and summarise the final result or use 'clear_analyze_history' tool to reduce the context size"

func (g *analyzeDirectoryTool) Description() string {
	return "按指定深度展开文件树中的目录或文件，以 CSV 返回路径、总大小和类型；每个目录最多展示占用最大的 200 个直接子项"
}

func (g *analyzeDirectoryTool) IsSupport(agent *Agent) bool {
	return agent.state != agentStateHigh
}

func (g *analyzeDirectoryTool) invoke(agent *Agent, parameter string) (string, error) {
	if agent.state == agentStateHigh {
		return "", errors.New(analyzeDirectoryRefuseMessage)
	}
	var args analyzeDirectoryParameters
	if err := json.Unmarshal([]byte(parameter), &args); err != nil {
		return "", fmt.Errorf("decode analyze_directory parameters: %w", err)
	}
	if !strings.HasPrefix(args.Path, "/") {
		return "", fmt.Errorf("path must start with '/': %q", args.Path)
	}
	if strings.Contains(args.Path, `\`) {
		return "", fmt.Errorf("path must use '/' as its separator: %q", args.Path)
	}

	treePath := modelscanner.NormalizeTreePath(args.Path)
	node, err := agent.tree.FindNode(treePath)
	if err != nil {
		return "", err
	}

	entries := collectDirectoryUsage(node, treePath)

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].totalSize != entries[j].totalSize {
			return entries[i].totalSize > entries[j].totalSize
		}
		return entries[i].path < entries[j].path
	})

	var buffer bytes.Buffer
	writer := csv.NewWriter(&buffer)
	if err := writer.Write([]string{"path", "totalSize", "type"}); err != nil {
		return "", err
	}
	for _, entry := range entries {
		if err := writer.Write([]string{
			entry.path,
			strconv.FormatInt(entry.totalSize, 10),
			strconv.Itoa(entry.typeID),
		}); err != nil {
			return "", err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return "", err
	}
	return buffer.String(), nil
}

func collectDirectoryUsage(node *modelscanner.FileNode, base string) []directoryUsageEntry {
	typeID := 1
	if node.IsDir() {
		typeID = 0
	}
	entries := make([]directoryUsageEntry, 0)
	entries = append(entries, directoryUsageEntry{
		path:      base,
		totalSize: node.DiskSize,
		typeID:    typeID,
	})
	for _, child := range node.Children {
		myTypeID := 1
		if child.IsDir() {
			myTypeID = 0
		}
		entries = append(entries, directoryUsageEntry{
			path:      path.Join(base, child.Name),
			totalSize: child.DiskSize,
			typeID:    myTypeID,
		})
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].totalSize > entries[j].totalSize
	})
	return entries
}

func (g *analyzeDirectoryTool) ParameterSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "文件树路径，必须以 / 开头并使用 / 作为路径分隔符",
				"pattern":     "^/",
			},
		},
		"required":             []string{"path"},
		"additionalProperties": false,
	}
}
