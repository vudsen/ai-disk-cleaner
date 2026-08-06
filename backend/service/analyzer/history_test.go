package analyzer

import (
	"encoding/json"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/openai/openai-go/v3"
)

func TestBuildToolsFiltersByAgentContextState(t *testing.T) {
	manager := newManager()
	tests := []struct {
		name         string
		state        agentContextState
		wantAnalyze  bool
		wantClear    bool
		wantToolSize int
	}{
		{
			name:         "low",
			state:        agentContextStateLow,
			wantAnalyze:  true,
			wantClear:    false,
			wantToolSize: 3,
		},
		{
			name:         "medium",
			state:        agentStateMedium,
			wantAnalyze:  true,
			wantClear:    true,
			wantToolSize: 4,
		},
		{
			name:         "high",
			state:        agentStateHigh,
			wantAnalyze:  false,
			wantClear:    true,
			wantToolSize: 3,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			names := builtToolNames(manager, &Agent{state: tt.state})
			if len(names) != tt.wantToolSize {
				t.Fatalf("tool names = %#v, want %d tools", names, tt.wantToolSize)
			}
			if slices.Contains(names, analyzeDirectoryToolName) != tt.wantAnalyze {
				t.Errorf("analyze_directory availability in %#v", names)
			}
			if slices.Contains(names, clearAnalyzeHistoryToolName) != tt.wantClear {
				t.Errorf("clear_analyze_history availability in %#v", names)
			}
			for _, alwaysSupported := range []string{"add_trash_file", "add_top_usages"} {
				if !slices.Contains(names, alwaysSupported) {
					t.Errorf("tool names are missing %q: %#v", alwaysSupported, names)
				}
			}
		})
	}
}

func TestClearAnalyzeHistoryToolContractAndRegistration(t *testing.T) {
	tool := newClearAnalyzeHistoryTool()
	if tool.Name() != clearAnalyzeHistoryToolName {
		t.Fatalf("tool name = %q", tool.Name())
	}
	if !strings.Contains(tool.Description(), "严格后代") || !strings.Contains(tool.Description(), "自身") {
		t.Fatalf("tool description does not explain preservation boundary: %q", tool.Description())
	}

	schema := tool.ParameterSchema()
	if schema["type"] != "object" || schema["additionalProperties"] != false {
		t.Fatalf("schema root = %#v", schema)
	}
	required, ok := schema["required"].([]string)
	if !ok || !slices.Equal(required, []string{"paths"}) {
		t.Fatalf("schema required = %#v", schema["required"])
	}
	properties := schema["properties"].(map[string]any)
	paths := properties["paths"].(map[string]any)
	if paths["type"] != "array" {
		t.Fatalf("paths schema = %#v", paths)
	}
	items := paths["items"].(map[string]any)
	pattern, ok := items["pattern"].(string)
	if !ok {
		t.Fatalf("path item pattern = %#v", items["pattern"])
	}
	matcher, err := regexp.Compile(pattern)
	if err != nil {
		t.Fatalf("compile path pattern %q: %v", pattern, err)
	}
	if !matcher.MatchString("/foo/bar") || matcher.MatchString("foo/bar") || matcher.MatchString(`/foo\bar`) {
		t.Fatalf("path pattern %q does not enforce logical paths", pattern)
	}
	if _, err := tool.invoke(&Agent{}, `{}`); err == nil {
		t.Fatal("invoke without paths: expected error")
	}

	manager := newManager()
	if manager.toolMap[clearAnalyzeHistoryToolName] == nil {
		t.Fatal("clear_analyze_history is not registered")
	}
	definitions, err := json.Marshal(buildTools(manager, &Agent{state: agentStateMedium}))
	if err != nil {
		t.Fatalf("encode tool definitions: %v", err)
	}
	if !strings.Contains(string(definitions), `"name":"clear_analyze_history"`) ||
		!strings.Contains(string(definitions), `"strict":true`) {
		t.Fatalf("clear tool is missing from strict definitions: %s", definitions)
	}

	agent := &Agent{messages: []openai.ChatCompletionMessageParamUnion{
		mustHistoryMessage(t, `{"role":"assistant","tool_calls":[{"id":"child","type":"function","function":{"name":"analyze_directory","arguments":"{\"path\":\"/foo/child\",\"depth\":1}"}}]}`),
		mustHistoryMessage(t, `{"role":"tool","tool_call_id":"child","content":"result"}`),
	}}
	result, err := manager.Invoke(clearAnalyzeHistoryToolName, `{"paths":["/foo"]}`, agent)
	if err != nil {
		t.Fatalf("invoke registered clear tool: %v", err)
	}
	if result != `{"removedAnalyzeCalls":1}` || len(agent.messages) != 0 {
		t.Fatalf("result = %s, history length = %d", result, len(agent.messages))
	}
}

func builtToolNames(manager *toolsManager, agent *Agent) []string {
	definitions := buildTools(manager, agent)
	result := make([]string, 0, len(definitions))
	for _, definition := range definitions {
		if definition.OfFunction != nil {
			result = append(result, definition.OfFunction.Function.Name)
		}
	}
	return result
}

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
