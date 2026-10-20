package analyzer

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"ai-disk-cleanner/backend/data/models/cleaningrecord"
	modelscanner "ai-disk-cleanner/backend/model/scanner"
)

const (
	maxCodexToolPayloadBytes = 1 << 20
	maxCodexCandidates       = 30
)

func codexDynamicTools() []map[string]any {
	return []map[string]any{
		{"type": "function", "name": "analyze_directory", "description": "Read one directory from the already scanned file tree. The path is relative to the scan root and starts with /; use / for the scan root. Results are sorted by size and limited to 200 entries.", "inputSchema": objectSchema(map[string]any{"path": map[string]any{"type": "string"}}, "path")},
		{"type": "function", "name": "add_trash_file", "description": "Mark up to 30 scanned files or directories as cleanup candidates. Paths must identify exact scanned tree nodes; never use globs. level is 0 (low), 1 (medium), or 2 (high). Do not mark the scan root or links.", "inputSchema": objectSchema(map[string]any{"files": map[string]any{"type": "array", "maxItems": maxCodexCandidates, "items": map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"name": map[string]any{"type": "string"}, "reason": map[string]any{"type": "string"}, "path": map[string]any{"type": "string"}, "level": map[string]any{"type": "integer", "minimum": 0, "maximum": 2}}, "required": []string{"name", "reason", "path", "level"}}}}, "files")},
		{"type": "function", "name": "add_top_usages", "description": "Set up to 30 notable locations from the scanned tree as disk usage highlights. Paths must resolve to scanned nodes.", "inputSchema": objectSchema(map[string]any{"usages": map[string]any{"type": "array", "maxItems": maxCodexCandidates, "items": map[string]any{"type": "object", "additionalProperties": false, "properties": map[string]any{"path": map[string]any{"type": "string"}, "description": map[string]any{"type": "string"}}, "required": []string{"path", "description"}}}}, "usages")},
	}
}

func objectSchema(properties map[string]any, required string) map[string]any {
	return map[string]any{"type": "object", "additionalProperties": false, "properties": properties, "required": []string{required}}
}

type codexTaskState struct {
	directoryReads int
	tree           *modelscanner.FileTree
	trashFiles     []cleaningrecord.TrashFile
	topUsages      []cleaningrecord.DiskUsage
	callCache      map[string]json.RawMessage
}

func (state *codexTaskState) callTool(name string, arguments json.RawMessage) (string, error) {
	if len(arguments) == 0 || len(arguments) > maxCodexToolPayloadBytes {
		return "", errors.New("tool arguments exceed the 1 MiB limit")
	}
	if state == nil || state.tree == nil || state.tree.Root == nil {
		return "", errors.New("scanned tree is unavailable")
	}
	switch name {
	case "analyze_directory":
		var input struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(arguments, &input); err != nil {
			return "", err
		}
		path, err := state.resolveTreePath(input.Path, true)
		if err != nil {
			return "", err
		}
		output, err := renderDirectoryUsageCSV(state.tree, path)
		if err != nil {
			return "", err
		}
		if len(output) > maxCodexToolPayloadBytes {
			return "", errors.New("tool result exceeds the 1 MiB limit")
		}
		state.directoryReads++
		return output, nil
	case "add_trash_file":
		var input addTrashFileParameters
		if err := json.Unmarshal(arguments, &input); err != nil {
			return "", err
		}
		if len(input.Files) > maxCodexCandidates {
			return "", errors.New("at most 30 candidates may be added")
		}
		validated := make([]cleaningrecord.TrashFile, len(input.Files))
		for i, candidate := range input.Files {
			path, err := state.resolveTreePath(candidate.Path, false)
			if err != nil {
				return "", fmt.Errorf("candidate %d: %w", i+1, err)
			}
			node, err := state.tree.FindNode(path)
			if err != nil {
				return "", err
			}
			if node.NotRegular || node.HardLink {
				return "", fmt.Errorf("candidate %q is not a regular scanned node", candidate.Path)
			}
			if strings.TrimSpace(candidate.Name) == "" || strings.TrimSpace(candidate.Reason) == "" || candidate.Level < cleaningrecord.LOW || candidate.Level > cleaningrecord.HIGH || candidate.IsDeleted {
				return "", fmt.Errorf("candidate %q has invalid metadata", candidate.Path)
			}
			candidate.Path = node.RelativePath
			candidate.Size = node.DiskSize
			candidate.IsDeleted = false
			validated[i] = candidate
		}
		if len(state.trashFiles)+len(validated) > maxCodexCandidates {
			return "", errors.New("at most 30 candidates may be retained")
		}
		state.trashFiles = mergeTrashFiles(state.trashFiles, validated)
		return "Candidates saved from scanned tree data.", nil
	case "add_top_usages":
		var input struct {
			Usages []cleaningrecord.DiskUsage `json:"usages"`
		}
		if err := json.Unmarshal(arguments, &input); err != nil {
			return "", err
		}
		if len(input.Usages) > maxCodexCandidates {
			return "", errors.New("at most 30 usage locations may be added")
		}
		validated := make([]cleaningrecord.DiskUsage, len(input.Usages))
		for i, usage := range input.Usages {
			path, err := state.resolveTreePath(usage.Path, true)
			if err != nil {
				return "", fmt.Errorf("usage %d: %w", i+1, err)
			}
			node, err := state.tree.FindNode(path)
			if err != nil {
				return "", err
			}
			usage.Path, usage.Size = node.RelativePath, node.DiskSize
			if usage.Path == "" {
				usage.Path = "/"
			}
			validated[i] = usage
		}
		for i := range validated {
			for j := i + 1; j < len(validated); j++ {
				if codexTreePathsOverlap(validated[i].Path, validated[j].Path) {
					return "", errors.New("top usage paths must not duplicate or contain one another")
				}
			}
		}
		state.topUsages = validated
		return "Usage highlights saved from scanned tree data.", nil
	default:
		return "", fmt.Errorf("unknown application tool %q", name)
	}
}

func codexTreePathsOverlap(left, right string) bool {
	left = strings.Trim(left, "/")
	right = strings.Trim(right, "/")
	if left == right || left == "" || right == "" {
		return true
	}
	return strings.HasPrefix(left, right+"/") || strings.HasPrefix(right, left+"/")
}

func (state *codexTaskState) resolveTreePath(raw string, allowRoot bool) (string, error) {
	if strings.TrimSpace(raw) != raw || raw == "" || strings.ContainsAny(raw, "*?[]{}") {
		return "", errors.New("path must identify one exact scanned node")
	}
	separator := "/"
	if isWindowsCodexTree(state.tree.RootPath) {
		parts := strings.FieldsFunc(raw, func(r rune) bool { return r == '/' || r == '\\' })
		for _, part := range parts {
			if part == ".." {
				return "", errors.New("parent traversal is not allowed")
			}
		}
	} else {
		for _, part := range strings.Split(raw, separator) {
			if part == ".." {
				return "", errors.New("parent traversal is not allowed")
			}
		}
	}
	normalized := modelscanner.NormalizeTreePathForRoot(state.tree.RootPath, raw)
	if normalized == "/" {
		if allowRoot {
			return normalized, nil
		}
		return "", errors.New("scan root cannot be a cleanup candidate")
	}
	if _, err := state.tree.FindNode(normalized); err != nil {
		return "", err
	}
	return normalized, nil
}

func isWindowsCodexTree(root string) bool {
	return strings.HasPrefix(root, `\\`) ||
		len(root) >= 3 && ((root[0] >= 'a' && root[0] <= 'z') || (root[0] >= 'A' && root[0] <= 'Z')) && root[1] == ':' && (root[2] == '/' || root[2] == '\\')
}

func codexToolResponse(success bool, message string) map[string]any {
	return map[string]any{"success": success, "contentItems": []map[string]string{{"type": "inputText", "text": message}}}
}

func parseCodexToolArguments(raw json.RawMessage) (json.RawMessage, error) {
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	if _, ok := value.(map[string]any); !ok {
		return nil, errors.New("tool arguments must be a JSON object")
	}
	return append(json.RawMessage(nil), raw...), nil
}
