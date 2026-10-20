package scanner

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	modelscanner "ai-disk-cleanner/backend/model/scanner"
)

func TestParseGDUContextHonoursCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := NewService().ParseGDUContext(ctx, t.TempDir(), nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("ParseGDUContext() error = %v, want context.Canceled", err)
	}
}

func TestPOSIXTreePreservesBackslashFilename(t *testing.T) {
	const report = `[1,2,{"progname":"gdu","progver":"v5","timestamp":0},[{"name":"/tmp/中文 root"},{"name":"a\\b.txt","asize":7,"dsize":7}]]`
	tree, err := parseGDUJSON(strings.NewReader(report))
	if err != nil {
		t.Fatalf("parseGDUJSON() error = %v", err)
	}
	node, err := tree.FindNode(`a\b.txt`)
	if err != nil {
		t.Fatalf("FindNode() error = %v", err)
	}
	if node.Name != `a\b.txt` || node.FullPath != `/tmp/中文 root/a\b.txt` {
		t.Fatalf("node path = name %q, full %q", node.Name, node.FullPath)
	}
}

const testReport = `[1,2,{"progname":"gdu","progver":"v5.36.1","timestamp":1784024360},
[{"name":"C:\\Users\\tester\\AppData\\Local\\arthas-ui","mtime":1750929535},
{"name":"tc_version.txt","asize":1,"dsize":2,"mtime":1750929532,"unknown":"ignored"},
[{"name":"pkg","mtime":1750929535},
 [{"name":"arthas"},
  {"name":"arthas-agent.jar","asize":8445,"dsize":9000},
  {"name":"first-link.jar","asize":100,"dsize":128,"ino":9007199254740993,"hlnkc":true}],
 [{"name":"downloads"},
  {"name":"arthas-bin.zip","asize":14402310,"dsize":14403000},
  {"name":"second-link.jar","asize":100,"dsize":128,"ino":9007199254740993,"hlnkc":true},
  {"name":"socket","notreg":true}]]]]`

func TestParseGDUJSONCreatesCompleteTree(t *testing.T) {
	tree, err := parseGDUJSON(strings.NewReader(testReport))
	if err != nil {
		t.Fatalf("ParseGDU() error = %v", err)
	}

	if tree.Metadata.MajorVersion != 1 || tree.Metadata.MinorVersion != 2 {
		t.Fatalf("version = %d.%d, want 1.2", tree.Metadata.MajorVersion, tree.Metadata.MinorVersion)
	}
	if tree.Metadata.ProgramName != "gdu" || tree.Metadata.ProgramVersion != "v5.36.1" {
		t.Fatalf("program metadata = %#v", tree.Metadata)
	}
	if got := tree.Metadata.Timestamp; !got.Equal(time.Unix(1784024360, 0)) {
		t.Fatalf("timestamp = %v", got)
	}
	if tree.RootPath != `C:\Users\tester\AppData\Local\arthas-ui` {
		t.Fatalf("RootPath = %q", tree.RootPath)
	}
	if tree.Root.Name != "arthas-ui" || tree.Root.RelativePath != "." || tree.Root.FullPath != tree.RootPath {
		t.Fatalf("root = %#v", tree.Root)
	}
	if len(tree.Root.Children) != 2 {
		t.Fatalf("root children = %d, want 2", len(tree.Root.Children))
	}

	agent, err := tree.FindNode(`pkg\arthas\arthas-agent.jar`)
	if err != nil {
		t.Fatalf("Get(agent) error = %v", err)
	}
	if agent.Type != modelscanner.NodeTypeFile || agent.ApparentSize != 8445 || agent.DiskSize != 9000 {
		t.Fatalf("agent = %#v", agent)
	}
	if agent.RelativePath != "pkg/arthas/arthas-agent.jar" {
		t.Fatalf("agent.RelativePath = %q", agent.RelativePath)
	}
	if agent.FullPath != `C:\Users\tester\AppData\Local\arthas-ui\pkg\arthas\arthas-agent.jar` {
		t.Fatalf("agent.FullPath = %q", agent.FullPath)
	}

	socket, err := tree.FindNode("pkg/downloads/socket")
	if err != nil {
		t.Fatalf("Get(socket) error = %v", err)
	}
	if !socket.NotRegular {
		t.Fatal("socket.NotRegular = false, want true")
	}
}

func TestParseGDUScansDirectoryInMemory(t *testing.T) {
	tempDir := t.TempDir()
	sourceDir := filepath.Join(tempDir, "source")
	if err := os.MkdirAll(filepath.Join(sourceDir, "nested"), 0o700); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "nested", "file.txt"), []byte("content"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	tree, err := NewService().ParseGDU(sourceDir)
	if err != nil {
		t.Fatalf("ParseGDU() error = %v", err)
	}
	absoluteSourceDir, err := filepath.Abs(sourceDir)
	if err != nil {
		t.Fatalf("Abs() error = %v", err)
	}
	if tree.RootPath != absoluteSourceDir {
		t.Fatalf("RootPath = %q, want %q", tree.RootPath, absoluteSourceDir)
	}
	entries, err := tree.Get("nested")
	if err != nil {
		t.Fatalf("Get(nested) error = %v", err)
	}
	if len(entries) != 1 || entries[0].Name != "file.txt" || entries[0].Type != modelscanner.NodeTypeFile {
		t.Fatalf("Get(nested) = %#v", entries)
	}

	if _, err := os.Stat(filepath.Join(tempDir, "report.json")); !os.IsNotExist(err) {
		t.Fatalf("ParseGDU() created an unexpected report file: %v", err)
	}
}

func TestParseGDUFileScansDirectory(t *testing.T) {
	sourceDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(sourceDir, "file.txt"), []byte("content"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	tree, err := NewService().ParseGDUFile(sourceDir)
	if err != nil {
		t.Fatalf("ParseGDUFile() error = %v", err)
	}
	if len(tree.Root.Children) != 1 || tree.Root.Children[0].Name != "file.txt" {
		t.Fatalf("root children = %#v", tree.Root.Children)
	}
}

func TestParseGDURejectsInvalidPaths(t *testing.T) {
	if _, err := NewService().ParseGDU(""); err == nil {
		t.Fatal("ParseGDU() error = nil for empty directory path")
	}

	filePath := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(filePath, nil, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if _, err := NewService().ParseGDU(filePath); err == nil {
		t.Fatal("ParseGDU() error = nil for file path")
	}
}

func TestDirectoryStatsAndHardLinkDeduplication(t *testing.T) {
	tree, err := parseGDUJSON(strings.NewReader(testReport))
	if err != nil {
		t.Fatalf("ParseGDU() error = %v", err)
	}

	arthas, _ := tree.FindNode("pkg/arthas")
	downloads, _ := tree.FindNode("pkg/downloads")
	pkg, _ := tree.FindNode("pkg")

	if arthas.ItemCount != 3 || arthas.ApparentSize != 8545 || arthas.DiskSize != 9128 {
		t.Fatalf("arthas stats = count:%d apparent:%d disk:%d", arthas.ItemCount, arthas.ApparentSize, arthas.DiskSize)
	}
	if downloads.ItemCount != 4 || downloads.ApparentSize != 14402310 || downloads.DiskSize != 14403000 {
		t.Fatalf("downloads stats = count:%d apparent:%d disk:%d", downloads.ItemCount, downloads.ApparentSize, downloads.DiskSize)
	}
	if pkg.ItemCount != 8 || pkg.ApparentSize != 14410855 || pkg.DiskSize != 14412128 {
		t.Fatalf("pkg stats = count:%d apparent:%d disk:%d", pkg.ItemCount, pkg.ApparentSize, pkg.DiskSize)
	}
	if tree.Root.ItemCount != 10 || tree.Root.ApparentSize != 14410856 || tree.Root.DiskSize != 14412130 {
		t.Fatalf("root stats = count:%d apparent:%d disk:%d", tree.Root.ItemCount, tree.Root.ApparentSize, tree.Root.DiskSize)
	}

	secondLink, _ := tree.FindNode("pkg/downloads/second-link.jar")
	if secondLink.ApparentSize != 100 || secondLink.DiskSize != 128 || secondLink.Inode != 9007199254740993 {
		t.Fatalf("hard-link node fields were not preserved: %#v", secondLink)
	}
}

func TestGetNormalizesTreePaths(t *testing.T) {
	tree, err := parseGDUJSON(strings.NewReader(testReport))
	if err != nil {
		t.Fatalf("ParseGDU() error = %v", err)
	}

	for _, treePath := range []string{"", ".", "./", "/"} {
		entries, err := tree.Get(treePath)
		if err != nil {
			t.Errorf("Get(%q) error = %v", treePath, err)
			continue
		}
		if len(entries) != 2 || entries[0].Name != "pkg" || entries[1].Name != "tc_version.txt" {
			t.Errorf("Get(%q) = %#v, want sorted root children", treePath, entries)
		}
	}

	for _, treePath := range []string{"pkg", "./pkg", "/pkg"} {
		entries, err := tree.Get(treePath)
		if err != nil {
			t.Errorf("Get(%q) error = %v", treePath, err)
			continue
		}
		if len(entries) != 2 || entries[0].Name != "downloads" || entries[1].Name != "arthas" {
			t.Errorf("Get(%q) = %#v, want sorted pkg children", treePath, entries)
		}
	}

	for _, treePath := range []string{`pkg\arthas\..\downloads`, "pkg/arthas/../downloads", "/pkg/arthas/../downloads"} {
		entries, err := tree.Get(treePath)
		if err != nil {
			t.Errorf("Get(%q) error = %v", treePath, err)
			continue
		}
		if len(entries) != 3 || entries[0].Name != "arthas-bin.zip" {
			t.Errorf("Get(%q) = %#v, want downloads children", treePath, entries)
		}
	}

	for input, want := range map[string]string{
		"pkg":                      "/pkg",
		"./pkg":                    "/pkg",
		"/pkg":                     "/pkg",
		"/pkg/arthas/../downloads": "/pkg/downloads",
		"../../pkg":                "/pkg",
	} {
		if got := normalizeTreePath(input); got != want || strings.Contains(got, "..") {
			t.Errorf("normalizeTreePath(%q) = %q, want %q without '..'", input, got, want)
		}
	}

	_, err = tree.Get("pkg/missing")
	if !errors.Is(err, modelscanner.ErrNodeNotFound) {
		t.Fatalf("Get(missing) error = %v, want modelscanner.ErrNodeNotFound", err)
	}
}

func TestGetReturnsSortedDirectoryEntriesAndSingleFile(t *testing.T) {
	tree, err := parseGDUJSON(strings.NewReader(testReport))
	if err != nil {
		t.Fatalf("ParseGDU() error = %v", err)
	}

	entries, err := tree.Get("pkg/downloads")
	if err != nil {
		t.Fatalf("Get(directory) error = %v", err)
	}
	want := []modelscanner.FileEntry{
		{Name: "arthas-bin.zip", Size: 14403000, Type: modelscanner.NodeTypeFile},
		{Name: "second-link.jar", Size: 128, Type: modelscanner.NodeTypeFile},
		{Name: "socket", Size: 0, Type: modelscanner.NodeTypeFile},
	}
	if len(entries) != len(want) {
		t.Fatalf("Get(directory) = %#v, want %#v", entries, want)
	}
	for index := range want {
		if entries[index] != want[index] {
			t.Errorf("Get(directory)[%d] = %#v, want %#v", index, entries[index], want[index])
		}
	}

	entries, err = tree.Get("pkg/arthas/arthas-agent.jar")
	if err != nil {
		t.Fatalf("Get(file) error = %v", err)
	}
	if len(entries) != 1 || entries[0] != (modelscanner.FileEntry{Name: "arthas-agent.jar", Size: 9000, Type: modelscanner.NodeTypeFile}) {
		t.Fatalf("Get(file) = %#v", entries)
	}
}

func TestParseGDUJSONErrors(t *testing.T) {
	tests := []struct {
		name   string
		report string
		match  string
	}{
		{name: "broken JSON", report: `[1,2,`, match: "metadata"},
		{name: "not top-level array", report: `{}`, match: "top level"},
		{name: "root is not directory", report: `[1,2,{},{}]`, match: "expected directory array"},
		{name: "empty directory", report: `[1,2,{},[]]`, match: "directory array is empty"},
		{name: "missing name", report: `[1,2,{},[{}]]`, match: "name is required"},
		{name: "wrong name type", report: `[1,2,{},[{"name":3}]]`, match: "name must be a string"},
		{name: "duplicate child", report: `[1,2,{},[{"name":"root"},{"name":"x"},{"name":"x"}]]`, match: "duplicate child"},
		{name: "inode overflow", report: `[1,2,{},[{"name":"root"},{"name":"x","ino":18446744073709551616}]]`, match: "unsigned integer"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := parseGDUJSON(strings.NewReader(test.report))
			if err == nil || !strings.Contains(err.Error(), test.match) {
				t.Fatalf("ParseGDU() error = %v, want containing %q", err, test.match)
			}
		})
	}
}
