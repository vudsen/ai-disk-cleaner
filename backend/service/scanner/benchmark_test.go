package scanner

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	modelscanner "ai-disk-cleanner/backend/model/scanner"
	"github.com/dundee/gdu/v5/pkg/analyze"
	"github.com/dundee/gdu/v5/pkg/fs"
)

const (
	benchmarkDirectoryCount = 200
	benchmarkFilesPerDir    = 100
	benchmarkFileSize       = 1024
)

var benchmarkFixtureRoot string
var benchmarkFixtureTempRoot string

type benchmarkLogger interface {
	Helper()
	Fatal(...any)
	Fatalf(string, ...any)
	Logf(string, ...any)
}

func TestMain(m *testing.M) {
	code := m.Run()
	if benchmarkFixtureTempRoot != "" {
		_ = os.RemoveAll(benchmarkFixtureTempRoot)
	}
	os.Exit(code)
}

func BenchmarkGDUAnalyze(b *testing.B) {
	root := benchmarkFixture(b)
	b.ReportAllocs()
	b.SetBytes(benchmarkFixtureBytes())
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := analyzeBenchmarkRoot(root); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGDUUpdateStats(b *testing.B) {
	root := benchmarkFixture(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		directory, err := analyzeBenchmarkRoot(root)
		if err != nil {
			b.Fatal(err)
		}
		b.StartTimer()
		directory.UpdateStats(make(fs.HardLinkedItems, 10))
	}
}

func BenchmarkGDUEncodeJSON(b *testing.B) {
	root := benchmarkFixture(b)
	directory, err := analyzeBenchmarkRoot(root)
	if err != nil {
		b.Fatal(err)
	}
	directory.UpdateStats(make(fs.HardLinkedItems, 10))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var output bytes.Buffer
		if err := directory.EncodeJSON(&output, true); err != nil {
			b.Fatal(err)
		}
		if output.Len() == 0 {
			b.Fatal("EncodeJSON produced an empty report")
		}
	}
}

func BenchmarkGDUParseFileTree(b *testing.B) {
	report := benchmarkReport(b)
	b.SetBytes(int64(len(report)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := parseGDUJSON(bytes.NewReader(report)); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGDUScanEndToEnd(b *testing.B) {
	root := benchmarkFixture(b)
	b.ReportAllocs()
	b.SetBytes(benchmarkFixtureBytes())
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := scanGDUContext(context.Background(), root, nil); err != nil {
			b.Fatal(err)
		}
	}
}

func TestScanPerformancePhaseSample(t *testing.T) {
	if os.Getenv("AI_DISK_PERF_SAMPLE") != "1" {
		t.Skip("set AI_DISK_PERF_SAMPLE=1 to collect one phase timing sample")
	}
	root := benchmarkFixture(t)
	var total time.Duration
	started := time.Now()
	directory, err := analyzeBenchmarkRoot(root)
	analyzeDuration := time.Since(started)
	if err != nil {
		t.Fatal(err)
	}
	started = time.Now()
	directory.UpdateStats(make(fs.HardLinkedItems, 10))
	updateStatsDuration := time.Since(started)
	var report bytes.Buffer
	if _, err := fmt.Fprintf(&report, `[1,2,{"progname":"gdu","progver":"benchmark","timestamp":0},`); err != nil {
		t.Fatal(err)
	}
	started = time.Now()
	if err := directory.EncodeJSON(&report, true); err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(&report, "]\n"); err != nil {
		t.Fatal(err)
	}
	encodeDuration := time.Since(started)
	started = time.Now()
	tree, err := parseGDUJSON(bytes.NewReader(report.Bytes()))
	parseDuration := time.Since(started)
	if err != nil {
		t.Fatal(err)
	}
	started = time.Now()
	if _, err := scanGDUContext(context.Background(), root, nil); err != nil {
		t.Fatal(err)
	}
	endToEndDuration := time.Since(started)
	total = analyzeDuration + updateStatsDuration + encodeDuration + parseDuration
	t.Logf("root=%s entries=%d JSON_bytes=%d", root, countTreeEntries(tree.Root), report.Len())
	t.Logf("one-pass stages: analyze=%s update_stats=%s encode_json=%s parse_file_tree=%s; their measured sum=%s", analyzeDuration, updateStatsDuration, encodeDuration, parseDuration, total)
	t.Logf("production scanGDUContext end_to_end=%s (separate pass; not inferred by summing benchmark averages)", endToEndDuration)
}

func countTreeEntries(root *modelscanner.FileNode) int64 {
	if root == nil {
		return 0
	}
	count := int64(1)
	for _, child := range root.Children {
		count += countTreeEntries(child)
	}
	return count
}

// benchmarkFixture supports an explicit read-only corpus. The default fixture
// is generated once, outside benchmark timing, and is safe for repeated scans.
func benchmarkFixture(b benchmarkLogger) string {
	b.Helper()
	if root := os.Getenv("AI_DISK_BENCH_ROOT"); root != "" {
		absolute, err := filepath.Abs(root)
		if err != nil {
			b.Fatal(err)
		}
		absolute, err = filepath.EvalSymlinks(absolute)
		if err != nil {
			b.Fatalf("resolve AI_DISK_BENCH_ROOT: %v", err)
		}
		if absolute == string(filepath.Separator) {
			b.Fatal("AI_DISK_BENCH_ROOT must not be /")
		}
		info, err := os.Stat(absolute)
		if err != nil || !info.IsDir() {
			b.Fatalf("AI_DISK_BENCH_ROOT must name a readable directory: %v", err)
		}
		return absolute
	}
	if benchmarkFixtureRoot != "" {
		return benchmarkFixtureRoot
	}
	root, err := os.MkdirTemp("", "ai-disk-cleaner-scan-bench-")
	if err != nil {
		b.Fatal(err)
	}
	benchmarkFixtureTempRoot = root
	content := bytes.Repeat([]byte("x"), benchmarkFileSize)
	for dirIndex := 0; dirIndex < benchmarkDirectoryCount; dirIndex++ {
		dir := filepath.Join(root, fmt.Sprintf("directory-%03d", dirIndex))
		if err := os.Mkdir(dir, 0o700); err != nil {
			b.Fatal(err)
		}
		for fileIndex := 0; fileIndex < benchmarkFilesPerDir; fileIndex++ {
			name := filepath.Join(dir, fmt.Sprintf("file-%03d.dat", fileIndex))
			if err := os.WriteFile(name, content, 0o600); err != nil {
				b.Fatal(err)
			}
		}
	}
	deep := filepath.Join(root, "deep", "nested", "tree")
	if err := os.MkdirAll(deep, 0o700); err != nil {
		b.Fatal(err)
	}
	regular := filepath.Join(root, "directory-000", "file-000.dat")
	if err := os.Link(regular, filepath.Join(root, "directory-000", "hardlink.dat")); err != nil {
		b.Logf("hardlink fixture unavailable: %v", err)
	}
	if err := os.Symlink(regular, filepath.Join(root, "directory-000", "symlink.dat")); err != nil {
		b.Logf("symlink fixture unavailable: %v", err)
	}
	sparse, err := os.Create(filepath.Join(deep, "sparse.dat"))
	if err == nil {
		if err = sparse.Truncate(8 * 1024 * 1024); err != nil {
			b.Fatal(err)
		}
		if err = sparse.Close(); err != nil {
			b.Fatal(err)
		}
	} else {
		b.Logf("sparse fixture unavailable: %v", err)
	}
	benchmarkFixtureRoot = root
	b.Logf("fixture: %d directories, %d regular files, %d bytes of payload; root=%s", benchmarkDirectoryCount+3, benchmarkDirectoryCount*benchmarkFilesPerDir+3, benchmarkFixtureBytes(), root)
	return root
}

func benchmarkFixtureBytes() int64 {
	return int64(benchmarkDirectoryCount*benchmarkFilesPerDir*benchmarkFileSize + 8*1024*1024)
}

func analyzeBenchmarkRoot(root string) (*analyze.Dir, error) {
	policy, err := buildScanPolicy(context.Background(), root)
	if err != nil {
		return nil, err
	}
	directory, ok := analyze.CreateAnalyzer().AnalyzeDir(root, policy.ignoreDirectory, policy.ignoreFile).(*analyze.Dir)
	if !ok {
		return nil, fmt.Errorf("gdu did not return an analyze.Dir")
	}
	return directory, nil
}

func benchmarkReport(b benchmarkLogger) []byte {
	b.Helper()
	root := benchmarkFixture(b)
	directory, err := analyzeBenchmarkRoot(root)
	if err != nil {
		b.Fatal(err)
	}
	directory.UpdateStats(make(fs.HardLinkedItems, 10))
	var report bytes.Buffer
	if _, err := fmt.Fprintf(&report, `[1,2,{"progname":"gdu","progver":"benchmark","timestamp":0},`); err != nil {
		b.Fatal(err)
	}
	if err := directory.EncodeJSON(&report, true); err != nil {
		b.Fatal(err)
	}
	if _, err := io.WriteString(&report, "]\n"); err != nil {
		b.Fatal(err)
	}
	b.Logf("JSON input: %d bytes", report.Len())
	return report.Bytes()
}
