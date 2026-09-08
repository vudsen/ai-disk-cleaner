package analyzer

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	modelscanner "ai-disk-cleanner/backend/model/scanner"
	"ai-disk-cleanner/backend/service/tasklog"
	"github.com/openai/openai-go/v3"
)

type recordedRound struct {
	input, output, total *int64
	summary              string
}
type logRecorder struct {
	rounds []recordedRound
	events []string
	tools  []string
}

func (recorder *logRecorder) Event(name, format string, args ...any) {
	recorder.events = append(recorder.events, name+fmt.Sprintf(format, args...))
}
func (recorder *logRecorder) Round(_ int, _ time.Duration, input, output, total *int64, summary string) {
	recorder.rounds = append(recorder.rounds, recordedRound{input, output, total, summary})
}
func (recorder *logRecorder) ToolRequest(_ int, id, name, args string) {
	recorder.tools = append(recorder.tools, id+" "+name+" "+args)
}

func TestUsagePresenceIncludingValidZero(t *testing.T) {
	for _, test := range []struct {
		name, response       string
		input, output, total bool
	}{
		{"missing", `{}`, false, false, false},
		{"null", `{"usage":null}`, false, false, false},
		{"zero", `{"usage":{"prompt_tokens":0,"completion_tokens":0,"total_tokens":0}}`, true, true, true},
		{"partial", `{"usage":{"total_tokens":12}}`, false, false, true},
		{"no_total", `{"usage":{"prompt_tokens":10,"completion_tokens":2}}`, true, true, false},
		{"negative", `{"usage":{"total_tokens":-1}}`, false, false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var response openai.ChatCompletion
			if err := json.Unmarshal([]byte(test.response), &response); err != nil {
				t.Fatal(err)
			}
			recorder := &logRecorder{}
			agent := &Agent{log: recorder}
			agent.logCompletion(1, 0, &response)
			round := recorder.rounds[0]
			if (round.input != nil) != test.input || (round.output != nil) != test.output || (round.total != nil) != test.total {
				t.Fatalf("round=%+v", round)
			}
		})
	}
}

func TestAgentLogsUsageAndToolRequestWithoutResponseBodies(t *testing.T) {
	recorder := &logRecorder{}
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		if requests == 1 {
			fmt.Fprint(w, `{"id":"c1","choices":[{"index":0,"message":{"role":"assistant","content":"PRIVATE_REPLY","tool_calls":[{"id":"call_1","type":"function","function":{"name":"unknown_tool","arguments":"{\"path\":\"keep_parameter\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}}`)
			return
		}
		fmt.Fprint(w, `{"id":"c2","choices":[{"index":0,"message":{"role":"assistant","content":"PRIVATE_REPLY_2"},"finish_reason":"stop"}],"usage":{"prompt_tokens":15,"completion_tokens":3,"total_tokens":18}}`)
	}))
	defer server.Close()
	agent, err := newAgent(context.Background(), &modelscanner.FileTree{}, "PRIVATE_PROMPT", nil, &llmConfig{model: "test", secret: "PRIVATE_KEY", baseURL: server.URL, maxTokens: 100000}, "en")
	if err != nil {
		t.Fatal(err)
	}
	agent.log = recorder
	result, err := agent.run()
	if err != nil {
		t.Fatal(err)
	}
	if result.TokenUsage != 30 || len(recorder.rounds) != 2 || len(recorder.tools) != 1 {
		t.Fatalf("result=%+v log=%+v", result, recorder)
	}
	if !strings.Contains(recorder.tools[0], "keep_parameter") {
		t.Fatal(recorder.tools)
	}
	logged := fmt.Sprint(recorder.events, recorder.tools)
	if strings.Contains(logged, "PRIVATE_") || strings.Contains(logged, "not found") {
		t.Fatal(logged)
	}
	resetAnalyzeContext(agent, "PRIVATE_COMPRESSION_SUMMARY")
	if agent.usedTokens != 30 || strings.Contains(fmt.Sprint(recorder.events), "PRIVATE_") {
		t.Fatal(recorder.events)
	}
}

func TestLLMFailureSummaryDoesNotExposeRawResponse(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int
		body   string
	}{
		{"http_error", 400, `{"error":{"message":"PRIVATE_RESPONSE","type":"invalid_request_error"}}`},
		{"no_choices", 200, `{"choices":[],"usage":{"total_tokens":7}}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(test.status)
				fmt.Fprint(w, test.body)
			}))
			defer server.Close()
			agent, _ := newAgent(context.Background(), &modelscanner.FileTree{}, "prompt", nil, &llmConfig{model: "test", secret: "key", baseURL: server.URL, maxTokens: 10000}, "en")
			recorder := &logRecorder{}
			agent.log = recorder
			_, err := agent.run()
			if err == nil {
				t.Fatal("expected failure")
			}
			summary := tasklog.ErrorSummary(err)
			if strings.Contains(summary, "PRIVATE_RESPONSE") || strings.Contains(summary, "choices\"") {
				t.Fatal(summary)
			}
			if len(recorder.rounds) != 1 {
				t.Fatal(recorder.rounds)
			}
			if test.status == 400 && recorder.rounds[0].total != nil {
				t.Fatal("failed request counted as known")
			}
			if test.status == 200 && (recorder.rounds[0].total == nil || *recorder.rounds[0].total != 7) {
				t.Fatal("usage lost before choices check")
			}
		})
	}
}
