package analyzer

import (
	"encoding/json"
	"fmt"
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
			if slices.Contains(names, compatContextToolName) != tt.wantClear {
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
	if tool.Name() != compatContextToolName {
		t.Fatalf("tool name = %q", tool.Name())
	}
	if !strings.Contains(tool.Description(), "严格后代") || !strings.Contains(tool.Description(), "自身") {
		t.Fatalf("tool description does not explain preservation boundary: %q", tool.Description())
	}
	for _, phrase := range []string{"禁止传入 / 根目录", "已经读取", "后续不再"} {
		if !strings.Contains(tool.Description(), phrase) {
			t.Errorf("tool description is missing safe-compaction guidance %q: %q", phrase, tool.Description())
		}
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
	pathDescription, _ := paths["description"].(string)
	for _, phrase := range []string{"禁止传入 / 根目录", "已经读取", "后续不再"} {
		if !strings.Contains(pathDescription, phrase) {
			t.Errorf("paths description is missing safe-compaction guidance %q: %q", phrase, pathDescription)
		}
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
	if manager.toolMap[compatContextToolName] == nil {
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
		mustHistoryMessage(t, `{"role":"assistant","tool_calls":[{"id":"child","type":"function","function":{"name":"analyze_directory","arguments":"not-json"}}]}`),
		mustHistoryMessage(t, `{"role":"tool","tool_call_id":"child","content":"path,totalSize,type\n/foo/child,10,0\n"}`),
	}}
	result, err := manager.Invoke(compatContextToolName, `{"paths":["/foo"]}`, agent)
	if err != nil {
		t.Fatalf("invoke registered clear tool: %v", err)
	}
	if result != `{"removedAnalyzeEntries":1}` || len(agent.messages) != 2 {
		t.Fatalf("result = %s, history length = %d", result, len(agent.messages))
	}
}

func TestContextUsagePromptsRecommendNarrowCompletedBranches(t *testing.T) {
	for name, prompt := range map[string]string{
		"medium": agentContextUsageMediumSuffix,
		"high":   agentContextUsageHighSuffix,
	} {
		t.Run(name, func(t *testing.T) {
			for _, phrase := range []string{"specific non-root child directories", "already read", "no longer reference", "Never pass '/'"} {
				if !strings.Contains(prompt, phrase) {
					t.Errorf("runtime prompt is missing %q: %q", phrase, prompt)
				}
			}
		})
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
				{"id":"mixed-csv","type":"function","function":{"name":"analyze_directory","arguments":"not-json"}},
				{"id":"non-csv","type":"function","function":{"name":"analyze_directory","arguments":"{\"path\":\"/foo/bar/child\",\"depth\":1}"}},
				{"id":"keep-other","type":"function","function":{"name":"add_top_usages","arguments":"{\"usages\":[]}"}}
			]
		}`),
		mustHistoryMessage(t, `{"role":"tool","tool_call_id":"mixed-csv","content":"path,totalSize,type\n/foo/bar,100,0\n/foo/bar/child,40,0\n/foo/bar/child/file.log,20,1\n/foo/bar2/child,30,0\n/other,10,0\n"}`),
		mustHistoryMessage(t, `{"role":"tool","tool_call_id":"non-csv","content":"not csv"}`),
		mustHistoryMessage(t, `{"role":"tool","tool_call_id":"keep-other","content":"true"}`),
	}}
	assistantBefore := marshalHistory(t, agent.messages[1:2])

	result, err := clearAnalyzeHistory(agent, []string{"/foo//bar/./"})
	if err != nil {
		t.Fatalf("clear analyze history: %v", err)
	}
	if result != `{"removedAnalyzeEntries":2}` {
		t.Fatalf("result = %s", result)
	}

	if len(agent.messages) != 5 {
		t.Fatalf("history length = %d, want all 5 messages preserved", len(agent.messages))
	}
	if assistantAfter := marshalHistory(t, agent.messages[1:2]); assistantAfter != assistantBefore {
		t.Fatal("assistant message changed while filtering tool responses")
	}
	callIDs, resultIDs, assistantContents := inspectHistory(t, agent.messages)
	for _, kept := range []string{"mixed-csv", "non-csv", "keep-other"} {
		if !slices.Contains(callIDs, kept) {
			t.Errorf("history is missing kept call ID %q", kept)
		}
		if !slices.Contains(resultIDs, kept) {
			t.Errorf("history is missing kept result ID %q", kept)
		}
	}
	if !slices.Contains(assistantContents, "keep this summary") {
		t.Fatalf("assistant contents = %#v", assistantContents)
	}
	filteredCSV := historyToolContent(t, agent.messages, "mixed-csv")
	for _, kept := range []string{"/foo/bar,100,0", "/foo/bar2/child,30,0", "/other,10,0"} {
		if !strings.Contains(filteredCSV, kept) {
			t.Errorf("filtered CSV is missing %q: %q", kept, filteredCSV)
		}
	}
	for _, removed := range []string{"/foo/bar/child,40,0", "/foo/bar/child/file.log,20,1"} {
		if strings.Contains(filteredCSV, removed) {
			t.Errorf("filtered CSV still contains %q: %q", removed, filteredCSV)
		}
	}
	if content := historyToolContent(t, agent.messages, "non-csv"); content != "not csv" {
		t.Fatalf("non-CSV response changed: %q", content)
	}

	before := marshalHistory(t, agent.messages)
	result, err = clearAnalyzeHistory(agent, []string{"/foo/bar"})
	if err != nil {
		t.Fatalf("clear analyze history again: %v", err)
	}
	if result != `{"removedAnalyzeEntries":0}` {
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
			var calls []struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(fields["tool_calls"], &calls); err != nil {
				t.Fatalf("decode tool calls: %v", err)
			}
			for _, call := range calls {
				callIDs = append(callIDs, call.ID)
			}
		case "tool":
			resultIDs = append(resultIDs, decodeHistoryString(fields["tool_call_id"]))
		}
	}
	return callIDs, resultIDs, assistantContents
}

func historyToolContent(
	t *testing.T,
	messages []openai.ChatCompletionMessageParamUnion,
	toolCallID string,
) string {
	t.Helper()
	for _, message := range messages {
		fields, role, err := decodeHistoryMessage(message)
		if err != nil {
			t.Fatalf("decode history: %v", err)
		}
		if role == "tool" && decodeHistoryString(fields["tool_call_id"]) == toolCallID {
			return decodeHistoryString(fields["content"])
		}
	}
	t.Fatalf("tool response %q not found", toolCallID)
	return ""
}

func decodeHistoryMessage(
	message openai.ChatCompletionMessageParamUnion,
) (map[string]json.RawMessage, string, error) {
	data, err := json.Marshal(message)
	if err != nil {
		return nil, "", fmt.Errorf("encode analysis history message: %w", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, "", fmt.Errorf("decode analysis history message: %w", err)
	}
	return fields, decodeHistoryString(fields["role"]), nil
}

func decodeHistoryString(raw json.RawMessage) string {
	var result string
	_ = json.Unmarshal(raw, &result)
	return result
}
