//go:build linux

package analyzer

import (
	"strings"
	"testing"

	modelscanner "ai-disk-cleanner/backend/model/scanner"
)

func TestAnalyzeDirectoryRoundTripsPOSIXBackslashFilename(t *testing.T) {
	file := &modelscanner.FileNode{Name: `a\b.txt`, Type: modelscanner.NodeTypeFile, DiskSize: 7}
	directory := &modelscanner.FileNode{
		Name:           "dir",
		Type:           modelscanner.NodeTypeDirectory,
		FullPath:       "/tmp/root/dir",
		Children:       []*modelscanner.FileNode{file},
		ChildrenByName: map[string]*modelscanner.FileNode{file.Name: file},
	}
	root := &modelscanner.FileNode{
		Name:           "root",
		Type:           modelscanner.NodeTypeDirectory,
		Children:       []*modelscanner.FileNode{directory},
		ChildrenByName: map[string]*modelscanner.FileNode{"dir": directory},
	}
	tree := &modelscanner.FileTree{RootPath: "/tmp/root", Root: root}
	agent := &Agent{tree: tree, config: &llmConfig{}, state: agentContextStateLow}

	got, err := newAnalyzeDirectoryTool().invoke(agent, `{"path":"/dir/a\\b.txt"}`)
	if err != nil {
		t.Fatalf("analyze_directory error = %v", err)
	}
	if !strings.Contains(got, `/dir/a\b.txt,7`) {
		t.Fatalf("analyze_directory output lost the literal backslash: %q", got)
	}
}
