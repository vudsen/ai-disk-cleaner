package analyzer

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"

	"ai-disk-cleanner/backend/data/models/cleaningrecord"
	modelscanner "ai-disk-cleanner/backend/model/scanner"
)

//go:embed CODEX_SYSTEM.md
var codexSystemPrompt string

const (
	codexRPCTimeout      = 15 * time.Second
	codexTestTimeout     = 30 * time.Second
	codexInterruptGrace  = 2 * time.Second
	maxCodexToolResponse = 1 << 20
	maxCodexOutputBytes  = 8 << 20
)

var ErrCodexOutputTooLarge = errors.New("Codex output exceeds 8 MiB")

type codexEvent struct {
	method string
	params json.RawMessage
}

type codexRunner struct {
	session    *codexAppServerSession
	state      *codexTaskState
	mu         sync.Mutex
	ctx        context.Context
	thread     string
	turn       string
	active     bool
	turnReady  chan struct{}
	fatal      chan error
	allowTools bool
	eventMu    sync.Mutex
	eventBytes int
	items      map[string]string
	output     strings.Builder
	usage      int64
	hasUsage   bool
	onDelta    func(string)
	events     chan codexEvent
}

func codexTimedRequest(ctx context.Context, protocol *codexProtocol, method string, params any) (json.RawMessage, error) {
	rpcCtx, cancel := context.WithTimeout(ctx, codexRPCTimeout)
	defer cancel()
	return protocol.request(rpcCtx, method, params)
}

func isolatedCodexSession(ctx context.Context, config *codexConfig) (*codexAppServerSession, string, error) {
	executable, err := resolveCodexExecutable(config.executable)
	if err != nil {
		return nil, "", err
	}
	versionCtx, cancel := context.WithTimeout(ctx, codexHandshakeTimeout)
	defer cancel()
	command := exec.CommandContext(versionCtx, executable, "--version")
	versionOutput, commandErr := command.Output()
	if commandErr != nil {
		return nil, "", fmt.Errorf("run Codex version check: %w", commandErr)
	}
	version, err := parseCodexVersion(string(versionOutput))
	if err != nil {
		return nil, "", err
	}
	names, err := readCodexMCPServerNames(ctx, executable)
	if err != nil {
		return nil, version, err
	}
	args, err := codexIsolationArgs(names)
	if err != nil {
		return nil, version, err
	}
	session, err := startCodexAppServer(ctx, executable, args)
	return session, version, err
}

func (analyzer *Service) analyzeWithCodex(ctx context.Context, tree *modelscanner.FileTree, language string, onDelta func(string), logger analysisLogger, config *codexConfig) (*cleaningrecord.AnalysisResult, error) {
	startedAt := time.Now()
	deadlineCtx, cancel := context.WithTimeout(ctx, time.Duration(config.timeout)*time.Second)
	defer cancel()
	session, _, err := isolatedCodexSession(deadlineCtx, config)
	if err != nil {
		return nil, fmt.Errorf("start Codex analysis: %w", err)
	}
	defer session.close()
	if onDelta == nil {
		onDelta = func(string) {}
	}
	runner := &codexRunner{session: session, state: &codexTaskState{tree: tree, callCache: make(map[string]json.RawMessage)}, items: make(map[string]string), onDelta: onDelta, events: make(chan codexEvent, maxCodexNotificationQueue), turnReady: make(chan struct{}), fatal: make(chan error, 1)}
	runner.ctx = deadlineCtx
	session.protocol.requestHandler = runner.handleServerRequest
	session.protocol.notificationHandler = runner.handleNotification
	status, err := analyzer.runCodexTurn(deadlineCtx, runner, language, config.model, true)
	if err != nil {
		if logger != nil {
			logger.Round(1, time.Since(startedAt), nil, nil, nil, "Codex 分析未完成")
		}
		return nil, err
	}
	if status != "completed" {
		if logger != nil {
			logger.Round(1, time.Since(startedAt), nil, nil, nil, "Codex 分析未完成")
		}
		return nil, fmt.Errorf("Codex turn ended with status %q", status)
	}
	runner.mu.Lock()
	defer runner.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if runner.state.directoryReads == 0 {
		return nil, errors.New("Codex 分析未成功读取扫描目录树，分析结果未保存；请检查目录工具后重新分析已有记录")
	}
	var total *int64
	if runner.hasUsage {
		value := runner.usage
		total = &value
	}
	if logger != nil {
		logger.Round(1, time.Since(startedAt), nil, nil, total, "")
	}
	return &cleaningrecord.AnalysisResult{TrashFiles: append([]cleaningrecord.TrashFile(nil), runner.state.trashFiles...), TopUsages: append([]cleaningrecord.DiskUsage(nil), runner.state.topUsages...), LLMOutput: runner.output.String(), TokenUsage: runner.usage}, nil
}

func (analyzer *Service) testCodexConnection(ctx context.Context, config *codexConfig) (TestConnectionResult, error) {
	testCtx, cancel := context.WithTimeout(ctx, codexTestTimeout)
	defer cancel()
	session, _, err := isolatedCodexSession(testCtx, config)
	if err != nil {
		return TestConnectionResult{}, fmt.Errorf("test Codex connection: %w", err)
	}
	defer session.close()
	runner := &codexRunner{session: session, events: make(chan codexEvent, maxCodexNotificationQueue), items: make(map[string]string), fatal: make(chan error, 1)}
	session.protocol.requestHandler = runner.handleServerRequest
	session.protocol.notificationHandler = runner.handleNotification
	status, err := analyzer.runCodexTurn(testCtx, runner, "", config.model, false)
	if err != nil {
		return TestConnectionResult{}, err
	}
	if status != "completed" || strings.TrimSpace(runner.output.String()) == "" {
		return TestConnectionResult{}, errors.New("Codex did not complete a non-empty test response")
	}
	return TestConnectionResult{Type: "success"}, nil
}

func (analyzer *Service) runCodexTurn(ctx context.Context, runner *codexRunner, language, model string, includeTools bool) (string, error) {
	runner.ctx = ctx
	runner.allowTools = includeTools
	if runner.onDelta == nil {
		runner.onDelta = func(string) {}
	}
	threadParams := map[string]any{"cwd": runner.session.cwd, "ephemeral": true, "sandbox": "read-only", "environments": []any{}, "baseInstructions": codexSystemPrompt, "approvalPolicy": "never", "allowProviderModelFallback": false}
	if model != "" {
		threadParams["model"] = model
	}
	if includeTools {
		threadParams["dynamicTools"] = codexDynamicTools()
	}
	threadResponse, err := codexTimedRequest(ctx, runner.session.protocol, "thread/start", threadParams)
	if err != nil {
		return "", fmt.Errorf("start Codex thread: %w", err)
	}
	var thread struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	if err := json.Unmarshal(threadResponse, &thread); err != nil || thread.Thread.ID == "" {
		return "", errors.New("Codex thread/start returned no thread id")
	}
	runner.thread = thread.Thread.ID
	input := "Reply with a short confirmation that the local Codex connection works."
	if includeTools {
		input = fmt.Sprintf("Analyze the already scanned directory tree. User language: %s. Scan root: %s. Use analyze_directory to inspect it progressively, then report findings and record candidates only through application tools. Do not infer file paths.", language, runner.state.tree.RootPath)
	}
	turnResponse, err := codexTimedRequest(ctx, runner.session.protocol, "turn/start", map[string]any{"threadId": runner.thread, "input": []map[string]any{{"type": "text", "text": input}}, "environments": []any{}})
	if err != nil {
		return "", fmt.Errorf("start Codex turn: %w", err)
	}
	var started struct {
		Turn struct {
			ID string `json:"id"`
		} `json:"turn"`
	}
	if err := json.Unmarshal(turnResponse, &started); err != nil || started.Turn.ID == "" {
		return "", errors.New("Codex turn/start returned no turn id")
	}
	runner.turn = started.Turn.ID
	runner.setActive(true)
	if runner.turnReady != nil {
		close(runner.turnReady)
	}
	for {
		select {
		case event := <-runner.events:
			runner.releaseEventBytes(len(event.params))
			status, done, err := runner.consume(event)
			if err != nil {
				return "", err
			}
			if done {
				runner.setActive(false)
				return status, nil
			}
		case <-ctx.Done():
			runner.setActive(false)
			interruptCtx, cancel := context.WithTimeout(context.Background(), time.Second)
			_, _ = runner.session.protocol.request(interruptCtx, "turn/interrupt", map[string]any{"threadId": runner.thread, "turnId": runner.turn})
			cancel()
			return "", ctx.Err()
		case <-runner.session.protocol.done:
			runner.setActive(false)
			if err := runner.session.process.wait(); err != nil {
				return "", fmt.Errorf("Codex app-server exited: %w", err)
			}
			return "", io.ErrUnexpectedEOF
		case err := <-runner.fatal:
			runner.setActive(false)
			return "", err
		}
	}
}

func (runner *codexRunner) handleServerRequest(ctx context.Context, method string, raw json.RawMessage) (any, error) {
	if runner.turnReady != nil {
		select {
		case <-runner.turnReady:
		case <-runner.ctx.Done():
			return nil, runner.ctx.Err()
		}
	}
	if method != "item/tool/call" {
		err := errors.New("Codex requested an unsupported operation")
		runner.failTask(err)
		return nil, err
	}
	if !runner.allowTools || runner.state == nil {
		err := errors.New("Codex requested an unavailable application tool")
		runner.failTask(err)
		return nil, err
	}
	var request struct {
		ThreadID  string          `json:"threadId"`
		TurnID    string          `json:"turnId"`
		CallID    string          `json:"callId"`
		Tool      string          `json:"tool"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(raw, &request); err != nil {
		return nil, err
	}
	if request.ThreadID != runner.thread || request.TurnID != runner.turn || request.CallID == "" {
		err := errors.New("Codex tool call does not match the active turn")
		runner.failTask(err)
		return nil, err
	}
	runner.mu.Lock()
	defer runner.mu.Unlock()
	if !runner.active || runner.ctx != nil && runner.ctx.Err() != nil {
		return nil, errors.New("Codex tool call arrived after the turn stopped")
	}
	if cached := runner.state.callCache[request.CallID]; cached != nil {
		var response any
		if err := json.Unmarshal(cached, &response); err != nil {
			return nil, err
		}
		return response, nil
	}
	arguments, err := parseCodexToolArguments(request.Arguments)
	if err != nil {
		return codexToolResponse(false, err.Error()), nil
	}
	message, err := runner.state.callTool(request.Tool, arguments)
	if err != nil && !isCodexToolName(request.Tool) {
		runner.failTask(errors.New("Codex requested an unsupported application tool"))
	}
	response := codexToolResponse(err == nil, message)
	if err != nil {
		response = codexToolResponse(false, err.Error())
	}
	encoded, err := json.Marshal(response)
	if err != nil || len(encoded) > maxCodexToolResponse {
		return nil, errors.New("Codex tool result exceeds the 1 MiB limit")
	}
	runner.state.callCache[request.CallID] = encoded
	return response, nil
}

func isCodexToolName(name string) bool {
	for _, tool := range codexDynamicTools() {
		if tool["name"] == name {
			return true
		}
	}
	return false
}

func (runner *codexRunner) failTask(err error) {
	if runner.fatal == nil {
		return
	}
	select {
	case runner.fatal <- err:
	default:
	}
}

func (runner *codexRunner) handleNotification(_ context.Context, method string, params json.RawMessage) {
	runner.eventMu.Lock()
	if runner.eventBytes+len(params) > maxCodexNotificationBytes {
		runner.eventMu.Unlock()
		runner.failTask(errors.New("Codex notification queue exceeded its byte limit"))
		return
	}
	runner.eventBytes += len(params)
	runner.eventMu.Unlock()
	select {
	case runner.events <- codexEvent{method: method, params: append(json.RawMessage(nil), params...)}:
	default:
		runner.releaseEventBytes(len(params))
	}
}

func (runner *codexRunner) releaseEventBytes(size int) {
	runner.eventMu.Lock()
	runner.eventBytes -= size
	runner.eventMu.Unlock()
}

func (runner *codexRunner) appendMessage(itemID, value string) error {
	if runner.output.Len()+len(value) > maxCodexOutputBytes {
		return ErrCodexOutputTooLarge
	}
	runner.items[itemID] += value
	runner.output.WriteString(value)
	if value != "" && runner.isActive() {
		runner.onDelta(value)
	}
	return nil
}

func (runner *codexRunner) setActive(active bool) {
	runner.mu.Lock()
	runner.active = active
	runner.mu.Unlock()
}

func (runner *codexRunner) isActive() bool {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.active && (runner.ctx == nil || runner.ctx.Err() == nil)
}

func (runner *codexRunner) consume(event codexEvent) (string, bool, error) {
	var common struct {
		ThreadID string `json:"threadId"`
		TurnID   string `json:"turnId"`
		Turn     struct {
			ID string `json:"id"`
		} `json:"turn"`
	}
	if err := json.Unmarshal(event.params, &common); err != nil {
		return "", false, err
	}
	if event.method == "turn/completed" && common.TurnID == "" {
		common.TurnID = common.Turn.ID
	}
	if common.ThreadID != runner.thread || common.TurnID != runner.turn {
		return "", false, nil
	}
	switch event.method {
	case "item/agentMessage/delta":
		var notification struct {
			ItemID string `json:"itemId"`
			Delta  string `json:"delta"`
		}
		if err := json.Unmarshal(event.params, &notification); err != nil {
			return "", false, err
		}
		if err := runner.appendMessage(notification.ItemID, notification.Delta); err != nil {
			return "", false, err
		}
	case "item/completed":
		var notification struct {
			Item struct {
				ID   string `json:"id"`
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"item"`
		}
		if err := json.Unmarshal(event.params, &notification); err != nil {
			return "", false, err
		}
		if notification.Item.Type == "agentMessage" {
			streamed := runner.items[notification.Item.ID]
			if streamed == "" && notification.Item.Text != "" {
				if err := runner.appendMessage(notification.Item.ID, notification.Item.Text); err != nil {
					return "", false, err
				}
			}
			if streamed != "" && strings.HasPrefix(notification.Item.Text, streamed) {
				suffix := strings.TrimPrefix(notification.Item.Text, streamed)
				if err := runner.appendMessage(notification.Item.ID, suffix); err != nil {
					return "", false, err
				}
			}
		}
	case "thread/tokenUsage/updated":
		var notification struct {
			TokenUsage struct {
				Total struct {
					TotalTokens int64 `json:"totalTokens"`
				} `json:"total"`
			} `json:"tokenUsage"`
		}
		if err := json.Unmarshal(event.params, &notification); err == nil && notification.TokenUsage.Total.TotalTokens >= 0 {
			runner.usage, runner.hasUsage = notification.TokenUsage.Total.TotalTokens, true
		}
	case "turn/completed":
		var notification struct {
			Turn struct {
				Status string `json:"status"`
			} `json:"turn"`
		}
		if err := json.Unmarshal(event.params, &notification); err != nil {
			return "", false, err
		}
		if notification.Turn.Status == "completed" {
			return "completed", true, nil
		}
		return notification.Turn.Status, true, fmt.Errorf("Codex turn ended with status %q", notification.Turn.Status)
	}
	return "", false, nil
}
