package analyzer

import (
	"encoding/csv"
	"fmt"
	"strings"
	"testing"

	"ai-disk-cleanner/backend/data/models/cleaningrecord"
	modelscanner "ai-disk-cleanner/backend/model/scanner"
)

func TestRenderDirectoryUsageCSVSortsAndBoundsEntries(t *testing.T) {
	children := make([]*modelscanner.FileNode, 205)
	childrenByName := make(map[string]*modelscanner.FileNode, len(children))
	for index := range children {
		name := fmt.Sprintf("file-%03d", index)
		children[index] = &modelscanner.FileNode{
			Name:     name,
			Type:     modelscanner.NodeTypeFile,
			DiskSize: int64(index + 1),
		}
		childrenByName[name] = children[index]
	}
	root := &modelscanner.FileNode{
		Name:           "root",
		Type:           modelscanner.NodeTypeDirectory,
		DiskSize:       1000,
		Children:       children,
		ChildrenByName: childrenByName,
	}
	tree := &modelscanner.FileTree{RootPath: "/scan", Root: root}

	got, err := renderDirectoryUsageCSV(tree, "/")
	if err != nil {
		t.Fatalf("renderDirectoryUsageCSV() error = %v", err)
	}
	rows, err := csv.NewReader(strings.NewReader(got)).ReadAll()
	if err != nil {
		t.Fatalf("parse CSV: %v", err)
	}
	if len(rows) != maxDirectoryEntries+1 {
		t.Fatalf("row count = %d, want header plus %d entries", len(rows), maxDirectoryEntries)
	}
	if strings.Join(rows[0], ",") != "path,totalSize,type" {
		t.Fatalf("CSV header = %v", rows[0])
	}
	if rows[1][0] != "/" || rows[1][1] != "1000" {
		t.Fatalf("largest entry = %v, want scan root", rows[1])
	}
	if rows[len(rows)-1][0] != "/file-006" || rows[len(rows)-1][1] != "7" {
		t.Fatalf("last retained entry = %v, want smallest retained child", rows[len(rows)-1])
	}
}

func TestDecodeBusinessToolArguments(t *testing.T) {
	usages, err := decodeTopUsages(`{"usages":[{"path":"/large","size":42,"description":"cache"}]}`)
	if err != nil {
		t.Fatalf("decodeTopUsages() error = %v", err)
	}
	if len(usages) != 1 || usages[0].Path != "/large" || usages[0].Size != 42 {
		t.Fatalf("decoded usages = %#v", usages)
	}

	files, err := decodeTrashFiles(`{"files":[{"name":"archive","reason":"old","path":"/archive.zip","size":99,"level":1}]}`)
	if err != nil {
		t.Fatalf("decodeTrashFiles() error = %v", err)
	}
	if len(files) != 1 || files[0].Path != "/archive.zip" || files[0].Size != 99 {
		t.Fatalf("decoded trash files = %#v", files)
	}
}

func TestAPIBusinessToolsUseSharedOperations(t *testing.T) {
	file := &modelscanner.FileNode{Name: "item.txt", RelativePath: "item.txt", FullPath: "/scan/item.txt", Type: modelscanner.NodeTypeFile, DiskSize: 12}
	root := &modelscanner.FileNode{
		Name:           "scan",
		RelativePath:   ".",
		FullPath:       "/scan",
		Type:           modelscanner.NodeTypeDirectory,
		DiskSize:       20,
		Children:       []*modelscanner.FileNode{file},
		ChildrenByName: map[string]*modelscanner.FileNode{file.Name: file},
	}
	agent := &Agent{
		tree:   &modelscanner.FileTree{RootPath: "/scan", Root: root},
		config: &llmConfig{},
		state:  agentContextStateLow,
	}

	if _, err := newAddUsageTool().invoke(agent, `{"usages":[{"path":"/item.txt","size":12,"description":"file"}]}`); err != nil {
		t.Fatalf("add_top_usages invoke error = %v", err)
	}
	if len(agent.TopUsages) != 1 || agent.TopUsages[0].Path != "/item.txt" {
		t.Fatalf("top usages = %#v", agent.TopUsages)
	}
	if _, err := newAddTrashFileTool().invoke(agent, `{"files":[{"name":"item","reason":"old","path":"/item.txt","size":999,"level":1}]}`); err != nil {
		t.Fatalf("add_trash_file invoke error = %v", err)
	}
	if len(agent.TrashFiles) != 1 || agent.TrashFiles[0].Size != 999 {
		t.Fatalf("trash files = %#v", agent.TrashFiles)
	}
	result, err := newAnalyzeDirectoryTool().invoke(agent, `{"path":"/item.txt"}`)
	if err != nil {
		t.Fatalf("analyze_directory invoke error = %v", err)
	}
	if !strings.Contains(result, "/item.txt,12,1") {
		t.Fatalf("analyze_directory result = %q", result)
	}
}

func TestMergeTrashFilesDeduplicatesAndDropsNestedCandidates(t *testing.T) {
	parent := cleaningrecord.TrashFile{Name: "parent", Path: "/cache", Size: 80}
	duplicate := cleaningrecord.TrashFile{Name: "duplicate", Path: "/cache", Size: 5}
	child := cleaningrecord.TrashFile{Name: "child", Path: "/cache/item", Size: 5}
	other := cleaningrecord.TrashFile{Name: "other", Path: "/other", Size: 5}

	got := mergeTrashFiles([]cleaningrecord.TrashFile{parent}, []cleaningrecord.TrashFile{duplicate, child, other})
	if len(got) != 2 || got[0].Name != "parent" || got[1].Name != "other" {
		t.Fatalf("merged candidates = %#v", got)
	}
}
