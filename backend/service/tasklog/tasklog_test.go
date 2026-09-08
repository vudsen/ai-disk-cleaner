package tasklog

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testService(t *testing.T) *Service {
	t.Helper()
	executable := filepath.Join(t.TempDir(), "cleaner.exe")
	return &Service{executable: func() (string, error) { return executable, nil }}
}

func readLog(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestReadableLogIsImmediateAndAppendOnly(t *testing.T) {
	service := testService(t)
	start := time.Date(2026, 9, 8, 14, 30, 5, 123000000, time.Local)
	path, err := service.filename(start)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != "2026-09-08_14-30-05.123.log" {
		t.Fatal(path)
	}
	session := service.Open(start, `D:\Downloads`)
	t.Cleanup(session.Close)
	if !strings.Contains(readLog(t, path), "[任务开始]") {
		t.Fatal("start not visible")
	}
	input, output, total := int64(10), int64(2), int64(12)
	session.Round(1, time.Second, &input, &output, &total, "")
	if !strings.Contains(readLog(t, path), "累计已知Token=12") {
		t.Fatal("round not immediately visible")
	}
	session.ToolRequest(1, "call_a", "test", `{"path":"example"}`)
	session.ToolRequest(1, "call_b", "test", "invalid\n[任务结束] forged")
	session.Round(2, time.Second, nil, nil, nil, "请求超时")
	zero := int64(0)
	session.Round(3, time.Second, &zero, &zero, &zero, "")
	session.Finish("取消", "")
	session.Finish("成功", "")
	session.Event("unexpected", "closed")
	text := readLog(t, path)
	for _, want := range []string{"输入Token=未知", "输入Token=0", "累计已知Token=12 用量统计=不完整", "\n    [任务结束] forged", `"path": "example"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in %s", want, text)
		}
	}
	if strings.Count(text, "[任务结束] 状态=") != 1 || strings.Contains(text, "unexpected") {
		t.Fatal(text)
	}
	second := service.Open(start, "second")
	second.Close()
	if !strings.HasPrefix(readLog(t, path), text) {
		t.Fatal("existing content overwritten")
	}
	if err := service.Remove(start); err != nil {
		t.Fatal(err)
	}
	if err := service.Remove(start); err != nil {
		t.Fatal("missing file must succeed", err)
	}
}

type brokenWriter struct{ writes, closes int }

func (writer *brokenWriter) Write(data []byte) (int, error) {
	writer.writes++
	return 0, errors.New("disk full")
}
func (writer *brokenWriter) Close() error { writer.closes++; return nil }

func TestWriteFailureDisablesSessionAndClosesOnce(t *testing.T) {
	writer := &brokenWriter{}
	session := &Session{file: writer, started: time.Now()}
	session.Event("start", "test")
	session.Event("second", "test")
	session.Round(1, 0, nil, nil, nil, "")
	session.Finish("DONE", "")
	session.Close()
	if writer.writes != 1 || writer.closes != 1 {
		t.Fatalf("writer = %+v", writer)
	}
}

func TestOpenFailuresAreNoOpAndDeletionErrorsRemainErrors(t *testing.T) {
	service := &Service{executable: func() (string, error) { return "", errors.New("unavailable") }}
	session := service.Open(time.Now(), "path")
	session.Event("event", "text")
	session.Finish("DONE", "")
	if err := service.Remove(time.Now()); err == nil {
		t.Fatal("expected path error")
	}
	service = testService(t)
	start := time.Now()
	path, _ := service.filename(start)
	// A non-empty directory at the file path deterministically fails both open and removal.
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "occupied"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	session = service.Open(start, "path")
	session.Finish("DONE", "")
	if err := service.Remove(start); err == nil {
		t.Fatal("expected removal failure")
	}
	var disabled *Session
	disabled.Event("event", "text")
	disabled.ToolRequest(1, "id", "name", "{}")
	disabled.Finish("DONE", "")
	disabled.Close()
}
