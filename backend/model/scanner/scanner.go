package scanner

import (
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"
	"time"
)

var ErrNodeNotFound = errors.New("file tree node not found")

type NodeType string

const (
	NodeTypeFile      NodeType = "file"
	NodeTypeDirectory NodeType = "directory"
)

type GDUReportMetadata struct {
	MajorVersion   int       `json:"majorVersion"`
	MinorVersion   int       `json:"minorVersion"`
	ProgramName    string    `json:"programName"`
	ProgramVersion string    `json:"programVersion"`
	Timestamp      time.Time `json:"timestamp"`
}

type FileNode struct {
	Name           string               `json:"name"`
	RelativePath   string               `json:"relativePath"`
	FullPath       string               `json:"fullPath"`
	Type           NodeType             `json:"type"`
	ApparentSize   int64                `json:"apparentSize"`
	DiskSize       int64                `json:"diskSize"`
	ModifiedAt     time.Time            `json:"modifiedAt"`
	NotRegular     bool                 `json:"notRegular"`
	HardLink       bool                 `json:"hardLink"`
	Inode          uint64               `json:"inode"`
	ItemCount      int64                `json:"itemCount"`
	Children       []*FileNode          `json:"children,omitempty"`
	ChildrenByName map[string]*FileNode `json:"-"`
}

func (n *FileNode) IsDir() bool {
	return n != nil && n.Type == NodeTypeDirectory
}

type FileTree struct {
	Metadata    GDUReportMetadata `json:"metadata"`
	RootPath    string            `json:"rootPath"`
	Root        *FileNode         `json:"root"`
	ScanSummary ScanSummary       `json:"scanSummary"`
}

type ScanWarning struct {
	Path string `json:"path"`
	Code string `json:"code"`
}

type ScanSummary struct {
	ExcludedCount  int64         `json:"excludedCount"`
	ReadErrorCount int64         `json:"readErrorCount"`
	Warnings       []ScanWarning `json:"warnings"`
}

const MaxScanWarnings = 100

func (summary *ScanSummary) AddWarning(path, code string) {
	if summary == nil {
		return
	}
	if len(summary.Warnings) < MaxScanWarnings {
		summary.Warnings = append(summary.Warnings, ScanWarning{Path: path, Code: code})
	}
}

type FileEntry struct {
	Name string   `json:"name"`
	Size int64    `json:"size"`
	Type NodeType `json:"type"`
}

type ScanProgress struct {
	CurrentPath string `json:"currentPath"`
	ItemCount   int64  `json:"itemCount"`
	ScannedSize int64  `json:"scannedSize"`
}

func (t *FileTree) Get(treePath string) ([]FileEntry, error) {
	node, err := t.FindNode(treePath)
	if err != nil {
		return nil, err
	}
	if !node.IsDir() {
		return []FileEntry{entryFromNode(node)}, nil
	}
	entries := make([]FileEntry, 0, len(node.Children))
	for _, child := range node.Children {
		entries = append(entries, entryFromNode(child))
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Size != entries[j].Size {
			return entries[i].Size > entries[j].Size
		}
		return entries[i].Name < entries[j].Name
	})
	return entries, nil
}

func (t *FileTree) FindNode(treePath string) (*FileNode, error) {
	if t == nil || t.Root == nil {
		return nil, fmt.Errorf("%w: tree has no root", ErrNodeNotFound)
	}
	normalizedTreePath := NormalizeTreePathForRoot(t.RootPath, treePath)
	current := t.Root
	for _, part := range strings.Split(strings.TrimPrefix(normalizedTreePath, "/"), "/") {
		if part == "" {
			continue
		}
		if !current.IsDir() {
			return nil, fmt.Errorf("%w: %q", ErrNodeNotFound, normalizedTreePath)
		}
		next, exists := current.ChildrenByName[part]
		if !exists {
			return nil, fmt.Errorf("%w: %q", ErrNodeNotFound, normalizedTreePath)
		}
		current = next
	}
	return current, nil
}

func NormalizeTreePath(treePath string) string {
	normalized := strings.ReplaceAll(treePath, `\`, "/")
	return path.Clean("/" + normalized)
}

// NormalizeTreePathForRoot applies the path rules implied by a tree's root.
func NormalizeTreePathForRoot(rootPath, treePath string) string {
	if isWindowsTreePath(rootPath) {
		return NormalizeTreePath(treePath)
	}
	return path.Clean("/" + treePath)
}

// TreePathBaseName returns a root name without treating POSIX backslashes as separators.
func TreePathBaseName(treePath string) string {
	separators := "/"
	if isWindowsTreePath(treePath) {
		separators = `/\`
	}
	trimmed := strings.TrimRight(treePath, separators)
	if trimmed == "" {
		return treePath
	}
	if index := strings.LastIndexAny(trimmed, separators); index >= 0 {
		return trimmed[index+1:]
	}
	return trimmed
}

func isWindowsTreePath(treePath string) bool {
	if strings.HasPrefix(treePath, `\\`) || strings.HasPrefix(treePath, "//") {
		return true
	}
	return len(treePath) >= 3 &&
		((treePath[0] >= 'a' && treePath[0] <= 'z') || (treePath[0] >= 'A' && treePath[0] <= 'Z')) &&
		treePath[1] == ':' && (treePath[2] == '/' || treePath[2] == '\\')
}

func entryFromNode(node *FileNode) FileEntry {
	return FileEntry{Name: node.Name, Size: node.DiskSize, Type: node.Type}
}
