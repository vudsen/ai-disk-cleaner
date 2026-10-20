package analyzer

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	modelscanner "ai-disk-cleanner/backend/model/scanner"
)

func TestCodexRunnerDeduplicatesStreamAndFinalMessage(t *testing.T) {
	var deltas string
	runner := &codexRunner{thread: "thread", turn: "turn", active: true, items: map[string]string{}, onDelta: func(delta string) { deltas += delta }}
	consume := func(method, params string) {
		t.Helper()
		if _, _, err := runner.consume(codexEvent{method: method, params: json.RawMessage(params)}); err != nil {
			t.Fatal(err)
		}
	}
	consume("item/agentMessage/delta", `{"threadId":"thread","turnId":"turn","itemId":"one","delta":"Hello"}`)
	consume("item/completed", `{"threadId":"thread","turnId":"turn","item":{"id":"one","type":"agentMessage","text":"Hello"}}`)
	consume("item/completed", `{"threadId":"thread","turnId":"turn","item":{"id":"two","type":"agentMessage","text":" world"}}`)
	if got := runner.output.String(); got != "Hello world" {
		t.Fatalf("output = %q", got)
	}
	if deltas != "Hello world" {
		t.Fatalf("deltas = %q", deltas)
	}
}

func TestCodexRunnerKeepsLatestCumulativeUsageAndRejectsInterruptedTurn(t *testing.T) {
	runner := &codexRunner{thread: "thread", turn: "turn", items: map[string]string{}}
	for _, total := range []int{100, 100, 150} {
		if _, _, err := runner.consume(codexEvent{method: "thread/tokenUsage/updated", params: json.RawMessage(`{"threadId":"thread","turnId":"turn","tokenUsage":{"total":{"totalTokens":` + jsonNumber(total) + `}}}`)}); err != nil {
			t.Fatal(err)
		}
	}
	if !runner.hasUsage || runner.usage != 150 {
		t.Fatalf("usage = %d known=%t", runner.usage, runner.hasUsage)
	}
	_, done, err := runner.consume(codexEvent{method: "turn/completed", params: json.RawMessage(`{"threadId":"thread","turnId":"turn","turn":{"status":"interrupted"}}`)})
	if !done || err == nil {
		t.Fatalf("interrupted turn done=%t err=%v", done, err)
	}
}

func TestCodexRunnerConsumesActualCompletedTurnShape(t *testing.T) {
	runner := &codexRunner{thread: "thread", turn: "turn", items: map[string]string{}}
	status, done, err := runner.consume(codexEvent{
		method: "turn/completed",
		params: json.RawMessage(`{"threadId":"thread","turn":{"id":"turn","status":"completed"}}`),
	})
	if err != nil || !done || status != "completed" {
		t.Fatalf("actual turn/completed notification = %q, %t, %v", status, done, err)
	}
}

func TestCodexRunnerDuplicateToolCallIdReturnsCachedResult(t *testing.T) {
	file := &modelscanner.FileNode{Name: "a.zip", RelativePath: "a.zip", Type: modelscanner.NodeTypeFile, DiskSize: 42}
	tree := &modelscanner.FileTree{RootPath: "/home/alice", Root: &modelscanner.FileNode{Type: modelscanner.NodeTypeDirectory, ChildrenByName: map[string]*modelscanner.FileNode{"a.zip": file}}}
	runner := &codexRunner{thread: "thread", turn: "turn", active: true, allowTools: true, state: &codexTaskState{tree: tree, callCache: map[string]json.RawMessage{}}}
	first := json.RawMessage(`{"threadId":"thread","turnId":"turn","callId":"same","tool":"add_trash_file","arguments":{"files":[{"name":"archive","reason":"old","path":"/a.zip","level":1}]}}`)
	second := json.RawMessage(`{"threadId":"thread","turnId":"turn","callId":"same","tool":"add_trash_file","arguments":{"files":[{"name":"different","reason":"different","path":"/missing","level":1}]}}`)
	for _, request := range []json.RawMessage{first, second} {
		if _, err := runner.handleServerRequest(context.Background(), "item/tool/call", request); err != nil {
			t.Fatal(err)
		}
	}
	if len(runner.state.trashFiles) != 1 || runner.state.trashFiles[0].Name != "archive" {
		t.Fatalf("repeated call mutated candidates: %#v", runner.state.trashFiles)
	}
}

func TestCodexRunnerFailsUnexpectedServerRequests(t *testing.T) {
	runner := &codexRunner{fatal: make(chan error, 1)}
	if _, err := runner.handleServerRequest(context.Background(), "exec/commandApproval", json.RawMessage(`{}`)); err == nil {
		t.Fatal("unsupported server request returned nil error")
	}
	select {
	case err := <-runner.fatal:
		if err == nil {
			t.Fatal("fatal error is nil")
		}
	default:
		t.Fatal("unsupported server request did not fail the task")
	}
}

func TestCodexRunnerBoundsAccumulatedOutput(t *testing.T) {
	runner := &codexRunner{thread: "thread", turn: "turn", items: map[string]string{}}
	event, err := json.Marshal(map[string]any{"threadId": "thread", "turnId": "turn", "itemId": "large", "delta": strings.Repeat("x", maxCodexOutputBytes+1)})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = runner.consume(codexEvent{method: "item/agentMessage/delta", params: event})
	if !errors.Is(err, ErrCodexOutputTooLarge) {
		t.Fatalf("consume() error = %v", err)
	}
}

func jsonNumber(value int) string { return strconv.Itoa(value) }

func TestCodexTurnCancellationSendsInterrupt(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("fake CLI fixture uses a POSIX executable")
	}
	cliPath := writeFakeCodexCLI(t)
	marker := filepath.Join(t.TempDir(), "interrupt-received")
	t.Setenv("AI_DISK_CODEX_APPSERVER_HANG", "1")
	t.Setenv("AI_DISK_CODEX_INTERRUPT_MARKER", marker)
	session, _, err := isolatedCodexSession(context.Background(), &codexConfig{executable: cliPath})
	if err != nil {
		t.Fatalf("isolatedCodexSession() error = %v", err)
	}
	runner := &codexRunner{session: session, events: make(chan codexEvent, maxCodexNotificationQueue), items: map[string]string{}, fatal: make(chan error, 1)}
	session.protocol.requestHandler = runner.handleServerRequest
	session.protocol.notificationHandler = runner.handleNotification
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	_, err = newService(nil).runCodexTurn(ctx, runner, "", "", false)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("runCodexTurn() error = %v", err)
	}
	session.close()
	if contents, readErr := os.ReadFile(marker); readErr != nil || string(contents) != "interrupted" {
		t.Fatalf("interrupt marker = %q, %v", contents, readErr)
	}
}
