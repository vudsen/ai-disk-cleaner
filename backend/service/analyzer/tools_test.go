package analyzer

import (
	"encoding/csv"
	"slices"
	"strings"
	"testing"

	modelscanner "ai-disk-cleanner/backend/model/scanner"
)

func TestAnalyzeDirectoryDepthOneExpandsDirectChildren(t *testing.T) {
	root := analyzeDirectoryTestTree()
	agent := &Agent{
		state: agentContextStateLow,
		tree:  &modelscanner.FileTree{Root: root},
	}

	output, err := newAnalyzeDirectoryTool().invoke(agent, `{"path":"/","depth":1}`)
	if err != nil {
		t.Fatalf("invoke analyze_directory: %v", err)
	}

	paths := readAnalyzeDirectoryPaths(t, output)
	for _, expected := range []string{"/", "/foo", "/bar.bin"} {
		if !slices.Contains(paths, expected) {
			t.Errorf("depth=1 result does not contain %q: %v", expected, paths)
		}
	}
	if slices.Contains(paths, "/foo/nested.log") {
		t.Fatalf("depth=1 unexpectedly contains grandchild: %v", paths)
	}
}

func TestAnalyzeDirectoryDepthTwoExpandsGrandchildren(t *testing.T) {
	root := analyzeDirectoryTestTree()
	agent := &Agent{
		state: agentContextStateLow,
		tree:  &modelscanner.FileTree{Root: root},
	}

	output, err := newAnalyzeDirectoryTool().invoke(agent, `{"path":"/","depth":2}`)
	if err != nil {
		t.Fatalf("invoke analyze_directory: %v", err)
	}

	paths := readAnalyzeDirectoryPaths(t, output)
	if !slices.Contains(paths, "/foo/nested.log") {
		t.Fatalf("depth=2 result does not contain grandchild: %v", paths)
	}
}

func analyzeDirectoryTestTree() *modelscanner.FileNode {
	grandchild := &modelscanner.FileNode{
		Name:     "nested.log",
		Type:     modelscanner.NodeTypeFile,
		DiskSize: 10,
	}
	directory := &modelscanner.FileNode{
		Name:           "foo",
		Type:           modelscanner.NodeTypeDirectory,
		DiskSize:       80,
		Children:       []*modelscanner.FileNode{grandchild},
		ChildrenByName: map[string]*modelscanner.FileNode{"nested.log": grandchild},
	}
	file := &modelscanner.FileNode{
		Name:     "bar.bin",
		Type:     modelscanner.NodeTypeFile,
		DiskSize: 20,
	}
	return &modelscanner.FileNode{
		Name:           "/",
		Type:           modelscanner.NodeTypeDirectory,
		DiskSize:       100,
		Children:       []*modelscanner.FileNode{directory, file},
		ChildrenByName: map[string]*modelscanner.FileNode{"foo": directory, "bar.bin": file},
	}
}

func readAnalyzeDirectoryPaths(t *testing.T, output string) []string {
	t.Helper()
	records, err := csv.NewReader(strings.NewReader(output)).ReadAll()
	if err != nil {
		t.Fatalf("parse analyze_directory CSV: %v", err)
	}
	if len(records) == 0 {
		t.Fatal("analyze_directory returned empty CSV")
	}

	paths := make([]string, 0, len(records)-1)
	for _, record := range records[1:] {
		paths = append(paths, record[0])
	}
	return paths
}
