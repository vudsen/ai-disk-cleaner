package analyzer

import (
	"encoding/json"
	"testing"

	"ai-disk-cleanner/backend/data/models/cleaningrecord"
	modelscanner "ai-disk-cleanner/backend/model/scanner"
)

func TestCodexCandidatePathsResolveAgainstScannedTreeAtomically(t *testing.T) {
	tree := &modelscanner.FileTree{RootPath: "/home/alice", Root: &modelscanner.FileNode{
		Type: modelscanner.NodeTypeDirectory,
		ChildrenByName: map[string]*modelscanner.FileNode{
			"Downloads": {Name: "Downloads", RelativePath: "Downloads", Type: modelscanner.NodeTypeDirectory, ChildrenByName: map[string]*modelscanner.FileNode{
				"a.zip": {Name: "a.zip", RelativePath: "Downloads/a.zip", Type: modelscanner.NodeTypeFile, DiskSize: 123},
			}},
			`literal\name`: {Name: `literal\name`, RelativePath: `literal\name`, Type: modelscanner.NodeTypeFile, DiskSize: 7},
		},
	}}
	state := &codexTaskState{tree: tree}
	arguments := json.RawMessage(`{"files":[{"name":"archive","reason":"old","path":"/Downloads/a.zip","level":1}]}`)
	if _, err := state.callTool("add_trash_file", arguments); err != nil {
		t.Fatalf("valid candidate: %v", err)
	}
	if len(state.trashFiles) != 1 || state.trashFiles[0].Path != "Downloads/a.zip" || state.trashFiles[0].Size != 123 {
		t.Fatalf("candidate was not mapped to scan data: %#v", state.trashFiles)
	}
	before := append([]cleaningrecord.TrashFile(nil), state.trashFiles...)
	for _, invalid := range []string{
		`{"files":[{"name":"bad","reason":"traversal","path":"/../etc/passwd","level":0}]}`,
		`{"files":[{"name":"bad","reason":"glob","path":"/Downloads/*.zip","level":0}]}`,
		`{"files":[{"name":"bad","reason":"root","path":"/","level":0}]}`,
		`{"files":[{"name":"bad","reason":"unknown","path":"/missing","level":0}]}`,
	} {
		if _, err := state.callTool("add_trash_file", json.RawMessage(invalid)); err == nil {
			t.Errorf("invalid candidate accepted: %s", invalid)
		}
		if len(state.trashFiles) != len(before) || state.trashFiles[0] != before[0] {
			t.Fatalf("invalid batch mutated candidates: %#v", state.trashFiles)
		}
	}
	resolved, err := state.resolveTreePath(`/literal\name`, false)
	if err != nil || resolved != `/literal\name` {
		t.Fatalf("POSIX backslash filename resolved as %q, %v", resolved, err)
	}
}

func TestCodexToolSchemasExposeOnlyThreeApplicationTools(t *testing.T) {
	tools := codexDynamicTools()
	if len(tools) != 3 {
		t.Fatalf("codexDynamicTools() returned %d tools", len(tools))
	}
	for _, tool := range tools {
		if tool["type"] != "function" || tool["inputSchema"] == nil {
			t.Errorf("invalid dynamic tool: %#v", tool)
		}
	}
}

func TestCodexTopUsagesRejectNestedPathsAtomically(t *testing.T) {
	tree := &modelscanner.FileTree{RootPath: "/home/alice", Root: &modelscanner.FileNode{Type: modelscanner.NodeTypeDirectory, ChildrenByName: map[string]*modelscanner.FileNode{
		"Downloads": {Name: "Downloads", RelativePath: "Downloads", Type: modelscanner.NodeTypeDirectory, ChildrenByName: map[string]*modelscanner.FileNode{
			"a.zip": {Name: "a.zip", RelativePath: "Downloads/a.zip", Type: modelscanner.NodeTypeFile, DiskSize: 123},
		}},
	}}}
	state := &codexTaskState{tree: tree}
	_, err := state.callTool("add_top_usages", json.RawMessage(`{"usages":[{"path":"/Downloads","description":"directory"},{"path":"/Downloads/a.zip","description":"file"}]}`))
	if err == nil {
		t.Fatal("nested top usages were accepted")
	}
	if len(state.topUsages) != 0 {
		t.Fatalf("invalid usages mutated state: %#v", state.topUsages)
	}
}
