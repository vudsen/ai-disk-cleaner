package analyzer

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/openai/openai-go/v3"
)

func TestNormalizeAnalyzeHistoryTargetsAndStrictDescendants(t *testing.T) {
	targets, err := normalizeAnalyzeHistoryTargets([]string{
		"/foo//bar/",
		"/foo/bar",
		"/foo/./bar/child/..",
	})
	if err != nil {
		t.Fatalf("normalize targets: %v", err)
	}
	if len(targets) != 1 || targets[0] != "/foo/bar" {
		t.Fatalf("targets = %#v, want [/foo/bar]", targets)
	}

	tests := []struct {
		candidate string
		want      bool
	}{
		{candidate: "/foo/bar", want: false},
		{candidate: "/foo/bar/child", want: true},
		{candidate: "/foo/bar2/child", want: false},
		{candidate: "/foo", want: false},
	}
	for _, tt := range tests {
		if got := isStrictAnalyzeHistoryDescendant(tt.candidate, targets); got != tt.want {
			t.Errorf("isStrictAnalyzeHistoryDescendant(%q) = %v, want %v", tt.candidate, got, tt.want)
		}
	}
	if !isStrictAnalyzeHistoryDescendant("/foo", []string{"/"}) {
		t.Fatal("/foo must be a strict descendant of /")
	}
	if isStrictAnalyzeHistoryDescendant("/", []string{"/"}) {
		t.Fatal("/ must not be a strict descendant of itself")
	}

	invalid := []string{"", "   ", "foo/bar", `/foo\bar`}
	for _, value := range invalid {
		if _, err := normalizeAnalyzeHistoryTargets([]string{"/valid", value}); err == nil {
			t.Errorf("normalize targets with %q: expected error", value)
		}
	}
}

func TestClearAnalyzeHistoryPreservesIntegrityAndIsIdempotent(t *testing.T) {
	agent := &Agent{messages: []openai.ChatCompletionMessageParamUnion{
		mustHistoryMessage(t, `{"role":"system","content":"system"}`),
		mustHistoryMessage(t, `{
			"role":"assistant",
			"content":"keep this summary",
			"tool_calls":[
				{"id":"keep-target","type":"function","function":{"name":"analyze_directory","arguments":"{\"path\":\"/foo/bar\",\"depth\":1}"}},
				{"id":"remove-child","type":"function","function":{"name":"analyze_directory","arguments":"{\"path\":\"/foo/bar/child\",\"depth\":2}"}},
				{"id":"keep-prefix","type":"function","function":{"name":"analyze_directory","arguments":"{\"path\":\"/foo/bar2/child\",\"depth\":1}"}},
				{"id":"keep-unpaired","type":"function","function":{"name":"analyze_directory","arguments":"{\"path\":\"/foo/bar/unpaired\",\"depth\":1}"}},
				{"id":"keep-malformed","type":"function","function":{"name":"analyze_directory","arguments":"{\"path\":"}},
				{"id":"keep-other","type":"function","function":{"name":"add_top_usages","arguments":"{\"usages\":[]}"}}
			]
		}`),
		mustHistoryMessage(t, `{"role":"tool","tool_call_id":"keep-target","content":"target"}`),
		mustHistoryMessage(t, `{"role":"tool","tool_call_id":"remove-child","content":"child"}`),
		mustHistoryMessage(t, `{"role":"tool","tool_call_id":"keep-prefix","content":"prefix"}`),
		mustHistoryMessage(t, `{"role":"tool","tool_call_id":"keep-malformed","content":"error"}`),
		mustHistoryMessage(t, `{"role":"tool","tool_call_id":"keep-other","content":"true"}`),
		mustHistoryMessage(t, `{
			"role":"assistant",
			"tool_calls":[
				{"id":"remove-empty-message","type":"function","function":{"name":"analyze_directory","arguments":"{\"path\":\"/foo/bar/empty\",\"depth\":1}"}}
			]
		}`),
		mustHistoryMessage(t, `{"role":"tool","tool_call_id":"remove-empty-message","content":"empty"}`),
	}}

	result, err := clearAnalyzeHistory(agent, []string{"/foo//bar/./"})
	if err != nil {
		t.Fatalf("clear analyze history: %v", err)
	}
	if result != `{"removedAnalyzeCalls":2}` {
		t.Fatalf("result = %s", result)
	}

	callIDs, resultIDs, assistantContents := inspectHistory(t, agent.messages)
	for _, removed := range []string{"remove-child", "remove-empty-message"} {
		if slices.Contains(callIDs, removed) || slices.Contains(resultIDs, removed) {
			t.Fatalf("history still contains removed ID %q", removed)
		}
	}
	for _, kept := range []string{"keep-target", "keep-prefix", "keep-unpaired", "keep-malformed", "keep-other"} {
		if !slices.Contains(callIDs, kept) {
			t.Errorf("history is missing kept call ID %q", kept)
		}
	}
	if !slices.Contains(assistantContents, "keep this summary") {
		t.Fatalf("assistant contents = %#v", assistantContents)
	}

	before := marshalHistory(t, agent.messages)
	result, err = clearAnalyzeHistory(agent, []string{"/foo/bar"})
	if err != nil {
		t.Fatalf("clear analyze history again: %v", err)
	}
	if result != `{"removedAnalyzeCalls":0}` {
		t.Fatalf("second result = %s", result)
	}
	if after := marshalHistory(t, agent.messages); after != before {
		t.Fatal("idempotent compaction changed the remaining history")
	}
}

func TestClearAnalyzeHistoryRejectsInvalidPathsAtomically(t *testing.T) {
	agent := &Agent{messages: []openai.ChatCompletionMessageParamUnion{
		mustHistoryMessage(t, `{"role":"assistant","tool_calls":[{"id":"child","type":"function","function":{"name":"analyze_directory","arguments":"{\"path\":\"/foo/bar/child\",\"depth\":1}"}}]}`),
		mustHistoryMessage(t, `{"role":"tool","tool_call_id":"child","content":"result"}`),
	}}
	before := marshalHistory(t, agent.messages)
	if _, err := clearAnalyzeHistory(agent, []string{"/foo/bar", `/invalid\path`}); err == nil {
		t.Fatal("clearAnalyzeHistory() expected invalid path error")
	}
	if after := marshalHistory(t, agent.messages); after != before {
		t.Fatal("invalid request mutated analysis history")
	}
}

func mustHistoryMessage(t *testing.T, value string) openai.ChatCompletionMessageParamUnion {
	t.Helper()
	var result openai.ChatCompletionMessageParamUnion
	if err := json.Unmarshal([]byte(value), &result); err != nil {
		t.Fatalf("decode test history message: %v", err)
	}
	return result
}

func marshalHistory(t *testing.T, messages []openai.ChatCompletionMessageParamUnion) string {
	t.Helper()
	data, err := json.Marshal(messages)
	if err != nil {
		t.Fatalf("encode test history: %v", err)
	}
	return string(data)
}

func inspectHistory(
	t *testing.T,
	messages []openai.ChatCompletionMessageParamUnion,
) (callIDs []string, resultIDs []string, assistantContents []string) {
	t.Helper()
	for _, message := range messages {
		fields, role, err := decodeHistoryMessage(message)
		if err != nil {
			t.Fatalf("decode history: %v", err)
		}
		switch role {
		case "assistant":
			assistantContents = append(assistantContents, decodeHistoryString(fields["content"]))
			calls, err := decodeHistoryToolCalls(fields)
			if err != nil {
				t.Fatalf("decode tool calls: %v", err)
			}
			for _, rawCall := range calls {
				var call historyToolCall
				if err := json.Unmarshal(rawCall, &call); err != nil {
					t.Fatalf("decode tool call: %v", err)
				}
				callIDs = append(callIDs, call.ID)
			}
		case "tool":
			resultIDs = append(resultIDs, decodeHistoryString(fields["tool_call_id"]))
		}
	}
	return callIDs, resultIDs, assistantContents
}
