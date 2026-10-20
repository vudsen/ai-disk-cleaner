package analyzer

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"ai-disk-cleanner/backend/data/models/setting"
	modelscanner "ai-disk-cleanner/backend/model/scanner"
)

func TestCodexAppServerHelper(t *testing.T) {
	if os.Getenv("AI_DISK_CODEX_APPSERVER_HELPER") != "1" {
		return
	}
	reader := bufio.NewReader(os.Stdin)
	writer := bufio.NewWriter(os.Stdout)
	toolsEnabled := false
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			os.Exit(0)
		}
		var frame struct {
			ID     json.RawMessage            `json:"id"`
			Method string                     `json:"method"`
			Params map[string]json.RawMessage `json:"params"`
		}
		if json.Unmarshal(line, &frame) != nil {
			os.Exit(3)
		}
		switch frame.Method {
		case "initialize":
			if delay := os.Getenv("AI_DISK_CODEX_INIT_DELAY"); delay != "" {
				duration, _ := time.ParseDuration(delay)
				time.Sleep(duration)
			}
			fakeWriteFrame(writer, map[string]any{"id": frame.ID, "result": map[string]any{}})
		case "config/read":
			fakeWriteFrame(writer, map[string]any{"id": frame.ID, "result": map[string]any{"config": map[string]any{"mcp_servers": map[string]any{}}}})
		case "account/read":
			fakeWriteFrame(writer, map[string]any{"id": frame.ID, "result": map[string]any{"requiresOpenaiAuth": false, "account": nil}})
		case "thread/start":
			_, toolsEnabled = frame.Params["dynamicTools"]
			fakeWriteFrame(writer, map[string]any{"id": frame.ID, "result": map[string]any{"thread": map[string]any{"id": "thread-smoke"}}})
		case "turn/start":
			fakeWriteFrame(writer, map[string]any{"id": frame.ID, "result": map[string]any{"turn": map[string]any{"id": "turn-smoke", "status": "inProgress"}}})
			if os.Getenv("AI_DISK_CODEX_APPSERVER_HANG") == "1" {
				continue
			}
			if toolsEnabled && os.Getenv("AI_DISK_CODEX_NO_TOOL_CALLS") != "1" {
				fakeToolCall(writer, reader, "analyze_directory", map[string]any{"path": "/"})
				fakeToolCall(writer, reader, "add_trash_file", map[string]any{"files": []any{map[string]any{"name": "Old archive", "reason": "A test fixture", "path": "/Downloads/a.zip", "level": 1}}})
				fakeToolCall(writer, reader, "add_top_usages", map[string]any{"usages": []any{map[string]any{"path": "/Downloads/a.zip", "description": "A scanned fixture"}}})
			}
			fakeWriteFrame(writer, map[string]any{"method": "item/agentMessage/delta", "params": map[string]any{"threadId": "thread-smoke", "turnId": "turn-smoke", "itemId": "message-smoke", "delta": "Reviewed scanned data."}})
			fakeWriteFrame(writer, map[string]any{"method": "item/completed", "params": map[string]any{"threadId": "thread-smoke", "turnId": "turn-smoke", "item": map[string]any{"id": "message-smoke", "type": "agentMessage", "text": "Reviewed scanned data."}}})
			fakeWriteFrame(writer, map[string]any{"method": "turn/completed", "params": map[string]any{"threadId": "thread-smoke", "turn": map[string]any{"id": "turn-smoke", "status": "completed"}}})
		case "turn/interrupt":
			fakeWriteFrame(writer, map[string]any{"id": frame.ID, "result": map[string]any{}})
			if marker := os.Getenv("AI_DISK_CODEX_INTERRUPT_MARKER"); marker != "" {
				_ = os.WriteFile(marker, []byte("interrupted"), 0o600)
			}
		default:
			fakeWriteFrame(writer, map[string]any{"id": frame.ID, "error": map[string]any{"code": -32601, "message": "unknown fake method"}})
		}
	}
}

func TestCodexAppServerAllowsColdStartup(t *testing.T) {
	t.Setenv("AI_DISK_CODEX_APPSERVER_HELPER", "1")
	t.Setenv("AI_DISK_CODEX_INIT_DELAY", "11s")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	session, err := startCodexAppServer(ctx, executable, []string{"-test.run=^TestCodexAppServerHelper$"})
	if err != nil {
		t.Fatalf("cold app-server startup failed: %v", err)
	}
	session.close()
}

// This optional probe exercises the installed CLI without starting a model turn.
func TestCodexLocalStartupProbe(t *testing.T) {
	if os.Getenv("AI_DISK_CODEX_STARTUP_PROBE") != "1" {
		t.Skip("set AI_DISK_CODEX_STARTUP_PROBE=1 to check the installed CLI without inference")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	session, version, err := isolatedCodexSession(ctx, &codexConfig{timeout: defaultCodexTimeoutSeconds})
	if err != nil {
		t.Fatal(err)
	}
	defer session.close()
	response, err := codexTimedRequest(ctx, session.protocol, "account/read", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	status, err := parseCodexAccountRead(response)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Codex %s initialized; readiness=%s (no model turn started)", version, status.Code)
}

func fakeWriteFrame(writer *bufio.Writer, frame any) {
	if err := json.NewEncoder(writer).Encode(frame); err != nil {
		os.Exit(4)
	}
	if err := writer.Flush(); err != nil {
		os.Exit(5)
	}
}

func fakeToolCall(writer *bufio.Writer, reader *bufio.Reader, tool string, arguments any) {
	if err := json.NewEncoder(writer).Encode(map[string]any{"id": "server-call", "method": "item/tool/call", "params": map[string]any{"threadId": "thread-smoke", "turnId": "turn-smoke", "callId": "call-" + tool, "tool": tool, "arguments": arguments}}); err != nil {
		os.Exit(6)
	}
	if err := writer.Flush(); err != nil {
		os.Exit(7)
	}
	_, _ = reader.ReadBytes('\n')
}

func writeFakeCodexCLI(t *testing.T) string {
	t.Helper()
	testBinary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cliPath := filepath.Join(t.TempDir(), "fake codex")
	script := fmt.Sprintf("#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then printf 'codex-cli 0.162.1\\n'; exit 0; fi\nexec '%s' -test.run='^TestCodexAppServerHelper$'\n", strings.ReplaceAll(testBinary, "'", "'\\''"))
	if err := os.WriteFile(cliPath, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AI_DISK_CODEX_TEST_BINARY", testBinary)
	t.Setenv("AI_DISK_CODEX_APPSERVER_HELPER", "1")
	return cliPath
}

func TestCodexProviderFullPathSmokeWithFakeCLI(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("fake CLI smoke fixture uses a POSIX executable")
	}
	cliPath := writeFakeCodexCLI(t)
	settings := []setting.Setting{{Key: "llm.provider", Value: "codex"}, {Key: "codex.executable", Value: cliPath}, {Key: "codex.model", Value: ""}, {Key: "codex.timeout-seconds", Value: "30"}, {Key: "record.max.count", Value: "10"}}
	service := newService(statusSettingsStore{settings: settings})
	connection, err := service.TestConnection(context.Background(), settings)
	if err != nil || connection.Type != "success" {
		t.Fatalf("TestConnection() = %#v, %v", connection, err)
	}
	tree := &modelscanner.FileTree{RootPath: "/home/alice", Root: &modelscanner.FileNode{Type: modelscanner.NodeTypeDirectory, ChildrenByName: map[string]*modelscanner.FileNode{"Downloads": {Name: "Downloads", RelativePath: "Downloads", Type: modelscanner.NodeTypeDirectory, ChildrenByName: map[string]*modelscanner.FileNode{"a.zip": {Name: "a.zip", RelativePath: "Downloads/a.zip", Type: modelscanner.NodeTypeFile, DiskSize: 123}}}}}}
	var streamed strings.Builder
	result, err := service.Analyze(context.Background(), tree, "en", func(delta string) { streamed.WriteString(delta) }, nil)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if result.LLMOutput != "Reviewed scanned data." || streamed.String() != result.LLMOutput {
		t.Fatalf("output=%q streamed=%q", result.LLMOutput, streamed.String())
	}
	if len(result.TrashFiles) != 1 || result.TrashFiles[0].Path != "Downloads/a.zip" || result.TrashFiles[0].Size != 123 {
		t.Fatalf("candidate mapping = %#v", result.TrashFiles)
	}
	if len(result.TopUsages) != 1 || result.TopUsages[0].Path != "Downloads/a.zip" || result.TopUsages[0].Size != 123 {
		t.Fatalf("top usage mapping = %#v", result.TopUsages)
	}
	status, err := service.GetStatus(context.Background())
	if err != nil || !status.Ready || status.Code != "ready" {
		t.Fatalf("GetStatus() = %#v, %v", status, err)
	}
	if result.TokenUsage != 0 {
		t.Fatalf("missing fake usage was fabricated as %d", result.TokenUsage)
	}
}

func TestCodexRejectsCompletedAnalysisWithoutDirectoryEvidence(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("POSIX fixture")
	}
	cli := writeFakeCodexCLI(t)
	t.Setenv("AI_DISK_CODEX_NO_TOOL_CALLS", "1")
	service := newService(statusSettingsStore{})
	tree := &modelscanner.FileTree{RootPath: "/offline", Root: &modelscanner.FileNode{Type: modelscanner.NodeTypeDirectory}}
	result, err := service.analyzeWithCodex(context.Background(), tree, "en", nil, nil, &codexConfig{executable: cli, timeout: 30})
	if err == nil || result != nil {
		t.Fatalf("analysis without evidence must fail: %#v, %v", result, err)
	}
}
