package analyzer

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"runtime"
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
	invoke(ctx *diskCleanerContext, parameter string) (any, error)
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
		newGetDirectoryUsageTool(),
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
	invoke, err := tool.invoke(ctx, parameter)
	if err != nil {
		return "", err
	}
	marshal, err := json.Marshal(invoke)
	if err != nil {
		return "", err
	}
	return string(marshal), nil
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

func (tool *addTopUsagesTool) invoke(ctx *diskCleanerContext, parameter string) (any, error) {
	var v addTopUsagesParameters
	err := json.Unmarshal([]byte(parameter), &v)
	if err != nil {
		return nil, err
	}
	ctx.TopUsages = v.Usages
	return true, nil
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

func (tool *addTrashFileTool) invoke(ctx *diskCleanerContext, parameter string) (any, error) {
	var v addTrashFileParameters
	if err := json.Unmarshal([]byte(parameter), &v); err != nil {
		return nil, err
	}
	ctx.TrashFiles = removeNestedTrashFiles(append(ctx.TrashFiles, v.Files...))
	return true, nil
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

type getDirectoryUsageTool struct{}

func newGetDirectoryUsageTool() *getDirectoryUsageTool {
	return &getDirectoryUsageTool{}
}

func (g *getDirectoryUsageTool) Name() string {
	return "get_directory_usage"
}

func (g *getDirectoryUsageTool) Description() string {
	return "获取指定目录下的磁盘占用，使用该方法可以读取沙盒环境下的文件占用信息. 一个目录下最多返回 200 个子文件，超过的部分将被截断"
}

func (g *getDirectoryUsageTool) invoke(ctx *diskCleanerContext, parameter string) (any, error) {
	var args map[string]any
	err := json.Unmarshal([]byte(parameter), &args)
	if err != nil {
		panic(err)
	}
	treePath := args["path"].(string)
	files, err := ctx.FileTree.Get(treePath)
	if err != nil {
		return nil, err
	}
	if len(files) > 200 {
		files = files[:200]
	}
	result, err := json.Marshal(files)
	if err != nil {
		panic(err)
	}
	return string(result), nil
}

func (g *getDirectoryUsageTool) ParameterSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]string{
				"type":        "string",
				"description": "文件树中的相对或绝对路径，例如 `pkg`、`./pkg` 或 `/pkg`；使用 `/` 访问根路径",
			},
		},
		"required":             []string{"path"},
		"additionalProperties": false,
	}
}
