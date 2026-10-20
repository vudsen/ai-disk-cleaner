package scanner

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"
	"time"

	modelscanner "ai-disk-cleanner/backend/model/scanner"
)

// parseGDUJSON parses a gdu/ncdu-style JSON export and builds an in-memory tree.
func parseGDUJSON(reader io.Reader) (*modelscanner.FileTree, error) {
	if reader == nil {
		return nil, errors.New("parse gdu report: reader is nil")
	}

	decoder := json.NewDecoder(reader)
	decoder.UseNumber()

	if err := expectDelimiter(decoder, '['); err != nil {
		return nil, fmt.Errorf("top level: %w", err)
	}

	major, err := decodeInt(decoder, "major version")
	if err != nil {
		return nil, fmt.Errorf("top level: %w", err)
	}
	minor, err := decodeInt(decoder, "minor version")
	if err != nil {
		return nil, fmt.Errorf("top level: %w", err)
	}

	metadata, err := decodeReportMetadata(decoder)
	if err != nil {
		return nil, fmt.Errorf("top level metadata: %w", err)
	}
	metadata.MajorVersion = major
	metadata.MinorVersion = minor

	token, err := decoder.Token()
	if err != nil {
		return nil, fmt.Errorf("root node: %w", err)
	}
	delimiter, ok := token.(json.Delim)
	if !ok || delimiter != '[' {
		return nil, fmt.Errorf("root node: expected directory array, got %v", token)
	}

	root, rootPath, err := decodeDirectoryBody(decoder, nil, true)
	if err != nil {
		return nil, fmt.Errorf("root node: %w", err)
	}

	// The current format has four entries. Ignore future top-level additions.
	for decoder.More() {
		var ignored json.RawMessage
		if err := decoder.Decode(&ignored); err != nil {
			return nil, fmt.Errorf("top level extra entry: %w", err)
		}
	}
	if err := expectDelimiter(decoder, ']'); err != nil {
		return nil, fmt.Errorf("top level: %w", err)
	}

	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, errors.New("top level: unexpected data after report")
		}
		return nil, fmt.Errorf("top level trailing data: %w", err)
	}

	aggregateDirectoryStats(root, make(map[uint64]struct{}))
	return &modelscanner.FileTree{Metadata: metadata, RootPath: rootPath, Root: root}, nil
}

func decodeReportMetadata(decoder *json.Decoder) (modelscanner.GDUReportMetadata, error) {
	var raw struct {
		ProgramName    string      `json:"progname"`
		ProgramVersion string      `json:"progver"`
		Timestamp      json.Number `json:"timestamp"`
	}
	if err := decoder.Decode(&raw); err != nil {
		return modelscanner.GDUReportMetadata{}, err
	}

	metadata := modelscanner.GDUReportMetadata{
		ProgramName:    raw.ProgramName,
		ProgramVersion: raw.ProgramVersion,
	}
	if raw.Timestamp != "" {
		seconds, err := raw.Timestamp.Int64()
		if err != nil {
			return modelscanner.GDUReportMetadata{}, fmt.Errorf("timestamp must be an integer: %w", err)
		}
		metadata.Timestamp = time.Unix(seconds, 0)
	}
	return metadata, nil
}

func decodeDirectoryBody(decoder *json.Decoder, parent *modelscanner.FileNode, root bool) (*modelscanner.FileNode, string, error) {
	if !decoder.More() {
		return nil, "", errors.New("directory array is empty")
	}
	if err := expectDelimiter(decoder, '{'); err != nil {
		return nil, "", fmt.Errorf("directory metadata: %w", err)
	}
	metadata, err := decodeNodeObjectBody(decoder)
	if err != nil {
		return nil, "", fmt.Errorf("directory metadata: %w", err)
	}

	rawName := metadata.Name
	rootPath := ""
	if root {
		rootPath = rawName
		metadata.Name = modelscanner.TreePathBaseName(rawName)
	}
	if metadata.Name == "" {
		return nil, "", errors.New("directory name must not be empty")
	}

	node := metadata.toNode(modelscanner.NodeTypeDirectory)
	node.ChildrenByName = make(map[string]*modelscanner.FileNode)
	setNodePaths(node, parent, rootPath)

	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, "", fmt.Errorf("directory %q child: %w", rawName, err)
		}
		delimiter, ok := token.(json.Delim)
		if !ok {
			return nil, "", fmt.Errorf("directory %q child: expected object or array, got %v", rawName, token)
		}

		var child *modelscanner.FileNode
		switch delimiter {
		case '{':
			childMetadata, err := decodeNodeObjectBody(decoder)
			if err != nil {
				return nil, "", fmt.Errorf("directory %q file: %w", rawName, err)
			}
			child = childMetadata.toNode(modelscanner.NodeTypeFile)
			setNodePaths(child, node, "")
		case '[':
			child, _, err = decodeDirectoryBody(decoder, node, false)
			if err != nil {
				return nil, "", fmt.Errorf("directory %q child directory: %w", rawName, err)
			}
		default:
			return nil, "", fmt.Errorf("directory %q child: unexpected delimiter %q", rawName, delimiter)
		}

		if _, duplicate := node.ChildrenByName[child.Name]; duplicate {
			return nil, "", fmt.Errorf("directory %q contains duplicate child %q", rawName, child.Name)
		}
		node.Children = append(node.Children, child)
		node.ChildrenByName[child.Name] = child
	}

	if err := expectDelimiter(decoder, ']'); err != nil {
		return nil, "", fmt.Errorf("directory %q: %w", rawName, err)
	}
	return node, rootPath, nil
}

type nodeMetadata struct {
	Name         string
	ApparentSize int64
	DiskSize     int64
	ModifiedAt   time.Time
	NotRegular   bool
	HardLink     bool
	Inode        uint64
}

func (m nodeMetadata) toNode(nodeType modelscanner.NodeType) *modelscanner.FileNode {
	return &modelscanner.FileNode{
		Name:         m.Name,
		Type:         nodeType,
		ApparentSize: m.ApparentSize,
		DiskSize:     m.DiskSize,
		ModifiedAt:   m.ModifiedAt,
		NotRegular:   m.NotRegular,
		HardLink:     m.HardLink,
		Inode:        m.Inode,
		ItemCount:    1,
	}
}

func decodeNodeObjectBody(decoder *json.Decoder) (nodeMetadata, error) {
	var metadata nodeMetadata
	foundName := false

	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return nodeMetadata{}, err
		}
		key, ok := keyToken.(string)
		if !ok {
			return nodeMetadata{}, fmt.Errorf("object key must be a string, got %v", keyToken)
		}

		switch key {
		case "name":
			if err := decoder.Decode(&metadata.Name); err != nil {
				return nodeMetadata{}, fmt.Errorf("name must be a string: %w", err)
			}
			foundName = true
		case "asize":
			value, err := decodeInt64(decoder, "asize")
			if err != nil {
				return nodeMetadata{}, err
			}
			metadata.ApparentSize = value
		case "dsize":
			value, err := decodeInt64(decoder, "dsize")
			if err != nil {
				return nodeMetadata{}, err
			}
			metadata.DiskSize = value
		case "mtime":
			value, err := decodeInt64(decoder, "mtime")
			if err != nil {
				return nodeMetadata{}, err
			}
			metadata.ModifiedAt = time.Unix(value, 0)
		case "notreg":
			if err := decoder.Decode(&metadata.NotRegular); err != nil {
				return nodeMetadata{}, fmt.Errorf("notreg must be a boolean: %w", err)
			}
		case "hlnkc":
			if err := decoder.Decode(&metadata.HardLink); err != nil {
				return nodeMetadata{}, fmt.Errorf("hlnkc must be a boolean: %w", err)
			}
		case "ino":
			var number json.Number
			if err := decoder.Decode(&number); err != nil {
				return nodeMetadata{}, fmt.Errorf("ino must be an unsigned integer: %w", err)
			}
			value, err := parseUint64(number, "ino")
			if err != nil {
				return nodeMetadata{}, err
			}
			metadata.Inode = value
		default:
			var ignored json.RawMessage
			if err := decoder.Decode(&ignored); err != nil {
				return nodeMetadata{}, fmt.Errorf("field %q: %w", key, err)
			}
		}
	}
	if err := expectDelimiter(decoder, '}'); err != nil {
		return nodeMetadata{}, err
	}
	if !foundName {
		return nodeMetadata{}, errors.New("name is required")
	}
	if metadata.Name == "" {
		return nodeMetadata{}, errors.New("name must not be empty")
	}
	return metadata, nil
}

func setNodePaths(node, parent *modelscanner.FileNode, rootPath string) {
	if parent == nil {
		node.RelativePath = "."
		node.FullPath = rootPath
		return
	}
	node.RelativePath = node.Name
	if parent.RelativePath != "." {
		node.RelativePath = path.Join(parent.RelativePath, node.Name)
	}
	node.FullPath = joinDisplayPath(parent.FullPath, node.Name)
}

func aggregateDirectoryStats(node *modelscanner.FileNode, seenInodes map[uint64]struct{}) (int64, int64, int64) {
	if !node.IsDir() {
		if node.HardLink && node.Inode != 0 {
			if _, seen := seenInodes[node.Inode]; seen {
				return 1, 0, 0
			}
			seenInodes[node.Inode] = struct{}{}
		}
		return 1, node.ApparentSize, node.DiskSize
	}

	count := int64(1)
	var apparentSize, diskSize int64
	for _, child := range node.Children {
		childCount, childApparentSize, childDiskSize := aggregateDirectoryStats(child, seenInodes)
		count += childCount
		apparentSize += childApparentSize
		diskSize += childDiskSize
	}
	node.ItemCount = count
	node.ApparentSize = apparentSize
	node.DiskSize = diskSize
	return count, apparentSize, diskSize
}

func normalizeTreePath(treePath string) string {
	return modelscanner.NormalizeTreePath(treePath)
}

func baseName(value string) string {
	return modelscanner.TreePathBaseName(value)
}

func joinDisplayPath(parent, name string) string {
	if parent == "" {
		return name
	}
	if strings.HasSuffix(parent, "/") || strings.HasSuffix(parent, `\`) {
		return parent + name
	}
	separator := "/"
	if strings.LastIndex(parent, `\`) > strings.LastIndex(parent, "/") {
		separator = `\`
	}
	return parent + separator + name
}

func decodeInt(decoder *json.Decoder, field string) (int, error) {
	value, err := decodeInt64(decoder, field)
	if err != nil {
		return 0, err
	}
	converted := int(value)
	if int64(converted) != value {
		return 0, fmt.Errorf("%s is out of range", field)
	}
	return converted, nil
}

func decodeInt64(decoder *json.Decoder, field string) (int64, error) {
	var number json.Number
	if err := decoder.Decode(&number); err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", field, err)
	}
	value, err := number.Int64()
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", field, err)
	}
	return value, nil
}

func parseUint64(number json.Number, field string) (uint64, error) {
	value := string(number)
	if value == "" || strings.HasPrefix(value, "-") || strings.ContainsAny(value, ".eE") {
		return 0, fmt.Errorf("%s must be an unsigned integer", field)
	}
	result, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be an unsigned integer: %w", field, err)
	}
	return result, nil
}

func expectDelimiter(decoder *json.Decoder, expected json.Delim) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok || delimiter != expected {
		return fmt.Errorf("expected %q, got %v", expected, token)
	}
	return nil
}
