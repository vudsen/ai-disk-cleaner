package analyzer

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
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

func buildTools(manager *toolsManager) []openai.ChatCompletionToolUnionParam {
	result := make([]openai.ChatCompletionToolUnionParam, 0, len(manager.tools))
	for _, tool := range manager.tools {
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

type diskCleanerContext struct {
	TrashFiles []cleaningrecord.TrashFile
	TopUsages  []cleaningrecord.DiskUsage
	FileTree   *modelscanner.FileTree `json:"-"`
}

type tool interface {
	Name() string
	Description() string
	invoke(ctx *diskCleanerContext, parameter string) (string, error)
	ParameterSchema() map[string]any
}

func newDiskCleanerContext(fileTree *modelscanner.FileTree) *diskCleanerContext {
	return &diskCleanerContext{FileTree: fileTree}
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
	}
	toolMap := make(map[string]tool)
	for _, tool := range tools {
		toolMap[tool.Name()] = tool
	}
	return &toolsManager{toolMap: toolMap, tools: tools}
}

func (manager *toolsManager) Invoke(toolName string, parameter string, ctx *diskCleanerContext) (string, error) {
	tool, ok := manager.toolMap[toolName]
	if !ok {
		return "", fmt.Errorf("tool '%s' not found", toolName)
	}
	return tool.invoke(ctx, parameter)
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

func (tool *addTopUsagesTool) invoke(ctx *diskCleanerContext, parameter string) (string, error) {
	var v addTopUsagesParameters
	err := json.Unmarshal([]byte(parameter), &v)
	if err != nil {
		return "", err
	}
	ctx.TopUsages = v.Usages
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

func (tool *addTrashFileTool) invoke(ctx *diskCleanerContext, parameter string) (string, error) {
	var v addTrashFileParameters
	if err := json.Unmarshal([]byte(parameter), &v); err != nil {
		return "", err
	}
	ctx.TrashFiles = removeNestedTrashFiles(append(ctx.TrashFiles, v.Files...))
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
	Path  string `json:"path"`
	Depth int    `json:"depth"`
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
	return "按指定深度分析文件树中的目录或文件，以 CSV 返回路径、总大小和类型；每个目录最多展示占用最大的 200 个直接子项"
}

func (g *analyzeDirectoryTool) invoke(ctx *diskCleanerContext, parameter string) (string, error) {
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
	if args.Depth < 1 {
		return "", fmt.Errorf("depth must be at least 1")
	}
	if ctx == nil || ctx.FileTree == nil {
		return "", fmt.Errorf("file tree is not available")
	}

	treePath := modelscanner.NormalizeTreePath(args.Path)
	node, err := ctx.FileTree.FindNode(treePath)
	if err != nil {
		return "", err
	}

	entries := make([]directoryUsageEntry, 0)
	traversalDepth := args.Depth
	if traversalDepth > 1 {
		// depth=1 only returns the requested node. For larger values, depth is
		// the number of descendant levels to inspect, matching the tool's
		// depth=2 example (/foo and /foo/xxx.exe are both included from /).
		traversalDepth++
	}
	collectDirectoryUsage(&entries, node, treePath, traversalDepth)
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

func collectDirectoryUsage(entries *[]directoryUsageEntry, node *modelscanner.FileNode, treePath string, depth int) {
	typeID := 1
	if node.IsDir() {
		typeID = 0
	}
	*entries = append(*entries, directoryUsageEntry{
		path:      treePath,
		totalSize: node.DiskSize,
		typeID:    typeID,
	})
	if depth == 1 || !node.IsDir() {
		return
	}

	children := append([]*modelscanner.FileNode(nil), node.Children...)
	sort.Slice(children, func(i, j int) bool {
		if children[i].DiskSize != children[j].DiskSize {
			return children[i].DiskSize > children[j].DiskSize
		}
		return children[i].Name < children[j].Name
	})
	if len(children) > maxDirectoryEntries {
		children = children[:maxDirectoryEntries]
	}
	for _, child := range children {
		collectDirectoryUsage(entries, child, path.Join(treePath, child.Name), depth-1)
	}
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
			"depth": map[string]any{
				"type":        "integer",
				"description": "搜索深度，从 1 开始；1 表示只显示 path 对应的目录或文件",
				"minimum":     1,
			},
		},
		"required":             []string{"path", "depth"},
		"additionalProperties": false,
	}
}
