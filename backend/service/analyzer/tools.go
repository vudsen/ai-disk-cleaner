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

func (g *analyzeDirectoryTool) Description() string {
	return "按指定深度展开文件树中的目录或文件，以 CSV 返回路径、总大小和类型；每个目录最多展示占用最大的 200 个直接子项"
}

func (g *analyzeDirectoryTool) IsSupport(agent *Agent) bool {
	return agent.state != agentStateHigh && !shouldCompress(agent)
}

func (g *analyzeDirectoryTool) invoke(agent *Agent, parameter string) (string, error) {
	if shouldCompress(agent) {
		return "", errors.New("该工具已被禁用，请使用 `compress_context` 工具来压缩上下文，")
	} else if agent.state == agentStateHigh {
		return "", errors.New("上下文即将超出限制，该工具已被禁用，立即进行总结并**一次性**调用所有 `add_trash_file` 来添加垃圾，禁止分多次调用")
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
	if len(entries) > 200 {
		return entries[:200]
	}
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

const (
	compressContextToolName = "compress_context"
)

type compressContextTool struct{}

type clearAnalyzeHistoryParameters struct {
	Summary string `json:"summary"`
}

func newClearAnalyzeHistoryTool() *compressContextTool {
	return &compressContextTool{}
}

func (tool *compressContextTool) Name() string {
	return compressContextToolName
}

func (tool *compressContextTool) Description() string {
	return "开启全新的扫描上下文。summary 必须列出当前和历史已经搜索的目录（这些目录后续禁止再次扫描），以及剩余未扫描的目录。旧对话不会带入新上下文。"
}

func (tool *compressContextTool) IsSupport(agent *Agent) bool {
	return agent.state != agentStateHigh
}

func (tool *compressContextTool) invoke(agent *Agent, parameter string) (string, error) {
	var arguments clearAnalyzeHistoryParameters
	decoder := json.NewDecoder(strings.NewReader(parameter))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&arguments); err != nil {
		return "", fmt.Errorf("decode compress_context parameters: %w", err)
	}
	summary := strings.TrimSpace(arguments.Summary)
	if summary == "" {
		return "", fmt.Errorf("summary is required")
	}

	resetAnalyzeContext(agent, summary)
	return "Ok", nil
}

func (tool *compressContextTool) ParameterSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"summary": map[string]any{
				"type":        "string",
				"description": "扫描交接总结，必须明确列出两部分：1. 当前已经搜索的目录，且这些目录后续禁止再次扫描；2. 剩余未扫描的目录。",
			},
		},
		"required":             []string{"summary"},
		"additionalProperties": false,
	}
}

func shouldSwitchToAgentHighState(agent *Agent) bool {
	return agent.usedTokens >= int64(float64(agent.config.maxTokens)*0.8)
}

func resetAnalyzeContext(agent *Agent, summary string) {
	if shouldSwitchToAgentHighState(agent) {
		myLog.Println("Switch to agent high state")
		agent.state = agentStateHigh
	} else if agent.usedTokens >= int64(float64(agent.config.maxTokens)*0.5) {
		myLog.Println("Switch to agent medium state")
		agent.state = agentStateMedium
	} else {
		agent.state = agentContextStateLow
	}

	userPrompt := strings.Builder{}
	userPrompt.WriteString("请基于以下扫描交接继续分析。禁止再次扫描已经搜索的目录，只从剩余未扫描的目录继续。使用 `")
	userPrompt.WriteString(agent.language)
	userPrompt.WriteString("` 语言输出。\n\n<scan_summary>\n")
	userPrompt.WriteString(summary)
	userPrompt.WriteString("</scan_summary>\n\n")

	switch agent.state {
	case agentStateMedium:
		userPrompt.WriteString(`<context_state>
当前上下文状态：Medium。

上下文空间已经受到限制，需要提高扫描效率：

- 优先处理已经发现的大型、高价值目录和文件。
- 减少低收益的目录探索，不要为了收集更多候选而深度遍历小目录。
- 对用途不明确、风险较高或需要多次下钻才能确认的目录，可以直接放弃。
- 添加候选时优先选择占用空间大、删除边界明确、安全性高的项目。
- 继续执行当前任务，但避免无意义的探索。
</context_state>`)
		break
	case agentStateHigh:
		userPrompt.WriteString(`<context_state>
当前上下文状态：High。

当前上下文空间有限，必须进入收尾模式：

- 立即停止目录探索，使用 'add_trash_file' 和 'add_top_usages' 工具进行总结
</context_state>`)
		break
	}
	agent.messages = []openai.ChatCompletionMessageParamUnion{
		agent.messages[0],
		openai.UserMessage(userPrompt.String()),
	}

	agent.totalTokens = 0
}
