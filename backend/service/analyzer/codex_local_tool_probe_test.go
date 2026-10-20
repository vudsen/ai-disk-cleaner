package analyzer

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	modelscanner "ai-disk-cleanner/backend/model/scanner"
)

// An opt-in integration test with the installed CLI and a localhost-only provider.
// It performs real dynamic tool execution without using credentials or inference.
func TestCodexInstalledCLIDynamicToolsRoundTrip(t *testing.T) {
	if os.Getenv("AI_DISK_CODEX_TOOL_PROBE") != "1" {
		t.Skip("set AI_DISK_CODEX_TOOL_PROBE=1 to exercise the installed CLI with a local fake provider")
	}
	var mu sync.Mutex
	requests := 0
	sawDirectoryOutput := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Input []map[string]any `json:"input"`
			Tools []map[string]any `json:"tools"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		requests++
		for _, item := range body.Input {
			if item["type"] == "function_call_output" && strings.Contains(fmt.Sprint(item["output"]), "cache") {
				sawDirectoryOutput = true
			}
		}
		for _, tool := range body.Tools {
			if tool["type"] == "function" {
				name, _ := tool["name"].(string)
				if strings.Contains(name, "exec") || strings.Contains(name, "shell") {
					t.Errorf("unexpected execution tool: %s", name)
				}
			}
		}
		w.Header().Set("Content-Type", "text/event-stream")
		emit := func(kind string, values map[string]any) {
			values["type"] = kind
			encoded, _ := json.Marshal(values)
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", kind, encoded)
			w.(http.Flusher).Flush()
		}
		emit("response.created", map[string]any{"response": map[string]any{"id": "resp_probe", "object": "response", "status": "in_progress"}})
		var item map[string]any
		switch requests {
		case 1:
			item = map[string]any{"id": "fc_read", "type": "function_call", "name": "analyze_directory", "call_id": "call_read", "arguments": `{"path":"/"}`}
		case 2:
			item = map[string]any{"id": "fc_usage", "type": "function_call", "name": "add_top_usages", "call_id": "call_usage", "arguments": `{"usages":[{"path":"/cache","description":"Cached fixture"}]}`}
		default:
			item = map[string]any{"id": "msg_probe", "type": "message", "role": "assistant", "content": []map[string]any{{"type": "output_text", "text": "Snapshot inspected.", "annotations": []any{}}}}
		}
		emit("response.output_item.done", map[string]any{"output_index": 0, "item": item})
		emit("response.completed", map[string]any{"response": map[string]any{"id": "resp_probe", "object": "response", "status": "completed", "output": []any{item}, "usage": map[string]int{"input_tokens": 10, "output_tokens": 5, "total_tokens": 15}}})
	}))
	defer server.Close()
	home := t.TempDir()
	config := fmt.Sprintf("model = \"gpt-4.1\"\nmodel_provider = \"local_probe\"\n[features]\ncode_mode_host = false\n[mcp_servers.synthetic_probe]\ncommand = \"/usr/bin/false\"\n[model_providers.local_probe]\nname = \"Local test provider\"\nbase_url = %q\nwire_api = \"responses\"\nenv_key = \"AI_DISK_PROBE_KEY\"\nrequires_openai_auth = false\n", server.URL+"/v1")
	if err := os.WriteFile(filepath.Join(home, "config.toml"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODEX_HOME", home)
	t.Setenv("AI_DISK_PROBE_KEY", "local-test-only")
	cache := &modelscanner.FileNode{Name: "cache", RelativePath: "cache", Type: modelscanner.NodeTypeDirectory, DiskSize: 4096}
	tree := &modelscanner.FileTree{RootPath: "/unmounted-fixture", Root: &modelscanner.FileNode{Type: modelscanner.NodeTypeDirectory, Children: []*modelscanner.FileNode{cache}, ChildrenByName: map[string]*modelscanner.FileNode{"cache": cache}}}
	service := newService(statusSettingsStore{})
	result, err := service.analyzeWithCodex(context.Background(), tree, "en", nil, nil, &codexConfig{timeout: 90})
	if err != nil {
		t.Fatal(err)
	}
	if result.LLMOutput != "Snapshot inspected." || len(result.TopUsages) != 1 || result.TopUsages[0].Size != 4096 {
		t.Fatalf("result = %#v", result)
	}
	mu.Lock()
	defer mu.Unlock()
	if requests != 3 || !sawDirectoryOutput {
		t.Fatalf("requests=%d directory output=%v", requests, sawDirectoryOutput)
	}
}
