package analyzer

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ai-disk-cleanner/backend/data/models/cleaningrecord"
	"ai-disk-cleanner/backend/data/models/setting"
	modelscanner "ai-disk-cleanner/backend/model/scanner"
)

type analyzerSettingStore struct {
	settings []setting.Setting
	err      error
}

func (store *analyzerSettingStore) ListSettings(context.Context) ([]setting.Setting, error) {
	return store.settings, store.err
}

func TestAnalyzerStreamsTextAndReturnsUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/chat/completions" {
			http.NotFound(writer, request)
			return
		}
		var body struct {
			Model     string `json:"model"`
			MaxTokens int64  `json:"max_tokens"`
			Seed      int64  `json:"seed"`
		}
		if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if body.Model != "database-model" || body.MaxTokens != 4321 || body.Seed != 42 {
			t.Errorf("request config = %#v", body)
		}
		writer.Header().Set("Content-Type", "text/event-stream")
		_, _ = writer.Write([]byte(
			"data: {\"id\":\"chat-1\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"test\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"分析\"},\"finish_reason\":null}]}\n\n" +
				"data: {\"id\":\"chat-1\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"test\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"完成\"},\"finish_reason\":null}]}\n\n" +
				"data: {\"id\":\"chat-1\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"test\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n" +
				"data: {\"id\":\"chat-1\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"test\",\"choices\":[],\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":2,\"total_tokens\":5}}\n\n" +
				"data: [DONE]\n\n",
		))
	}))
	defer server.Close()

	analyzer := newService(&analyzerSettingStore{settings: []setting.Setting{
		{Key: "llm.secret", Value: "database-secret"},
		{Key: "llm.url", Value: server.URL},
		{Key: "llm.model", Value: "database-model"},
		{Key: "llm.max-token", Value: "4321"},
		{Key: "llm.extra-body", Value: `{"seed":42}`},
	}})
	var deltas strings.Builder
	result, err := analyzer.Analyze(
		context.Background(),
		&modelscanner.FileTree{
			RootPath: "C:\\test",
			Root: &modelscanner.FileNode{
				Name: "test",
				Type: modelscanner.NodeTypeDirectory,
			},
		},
		"zh_CN",
		"fast",
		func(delta string) { deltas.WriteString(delta) },
	)
	if err != nil {
		t.Fatalf("Analyze() error = %v", err)
	}
	if result.LLMOutput != "分析完成" || deltas.String() != "分析完成" {
		t.Fatalf("output = %q, deltas = %q", result.LLMOutput, deltas.String())
	}
	if result.TokenUsage != 2 {
		t.Fatalf("TokenUsage = %d, want 2", result.TokenUsage)
	}
}

func TestLoadLLMConfigRejectsMissingAndUnreadableSettings(t *testing.T) {
	t.Run("missing value", func(t *testing.T) {
		analyzer := newService(&analyzerSettingStore{settings: []setting.Setting{
			{Key: "llm.secret", Value: ""},
			{Key: "llm.url", Value: "https://example.com"},
			{Key: "llm.model", Value: "model"},
			{Key: "llm.max-token", Value: "50000"},
			{Key: "llm.extra-body", Value: ""},
		}})
		if _, err := analyzer.loadLLMConfig(context.Background()); err == nil ||
			!strings.Contains(err.Error(), "llm.secret") {
			t.Fatalf("loadLLMConfig() error = %v", err)
		}
	})

	t.Run("store error", func(t *testing.T) {
		storeErr := errors.New("database unavailable")
		analyzer := newService(&analyzerSettingStore{err: storeErr})
		if _, err := analyzer.loadLLMConfig(context.Background()); !errors.Is(err, storeErr) {
			t.Fatalf("loadLLMConfig() error = %v, want %v", err, storeErr)
		}
	})

	t.Run("invalid extra body", func(t *testing.T) {
		analyzer := newService(&analyzerSettingStore{settings: []setting.Setting{
			{Key: "llm.secret", Value: "secret"},
			{Key: "llm.url", Value: "https://example.com"},
			{Key: "llm.model", Value: "model"},
			{Key: "llm.max-token", Value: "50000"},
			{Key: "llm.extra-body", Value: "[]"},
		}})
		if _, err := analyzer.loadLLMConfig(context.Background()); err == nil ||
			!strings.Contains(err.Error(), "llm.extra-body") {
			t.Fatalf("loadLLMConfig() error = %v", err)
		}
	})
}

func TestAddToolParameterSchemasUseObjectRoot(t *testing.T) {
	tests := []struct {
		tool     tool
		property string
	}{
		{tool: newAddTrashFileTool(), property: "files"},
		{tool: newAddUsageTool(), property: "usages"},
	}
	for _, tt := range tests {
		t.Run(tt.tool.Name(), func(t *testing.T) {
			schema := tt.tool.ParameterSchema()
			if schema["type"] != "object" {
				t.Fatalf("schema type = %v, want object", schema["type"])
			}
			properties, ok := schema["properties"].(map[string]any)
			if !ok {
				t.Fatalf("schema properties = %T, want map[string]any", schema["properties"])
			}
			if _, ok := properties[tt.property]; !ok {
				t.Fatalf("schema is missing property %q", tt.property)
			}
		})
	}
}

func TestAddTrashFileSchemaFields(t *testing.T) {
	schema := newAddTrashFileTool().ParameterSchema()
	properties := schema["properties"].(map[string]any)
	files := properties["files"].(map[string]any)
	items := files["items"].(map[string]any)
	fileProperties := items["properties"].(map[string]any)
	want := []string{"name", "reason", "path", "level"}
	if len(fileProperties) != len(want) {
		t.Fatalf("file properties = %#v, want exactly %v", fileProperties, want)
	}
	for _, field := range want {
		if _, ok := fileProperties[field]; !ok {
			t.Fatalf("file schema is missing property %q", field)
		}
	}
}

func TestAddToolsAcceptWrappedArrays(t *testing.T) {
	ctx := &diskCleanerContext{}
	if _, err := newAddTrashFileTool().invoke(ctx, `{"files":[{"name":"清理临时日志","reason":"日志可以安全重新生成","path":"temp.log","level":0}]}`); err != nil {
		t.Fatalf("invoke add_trash_file: %v", err)
	}
	if _, err := newAddUsageTool().invoke(ctx, `{"usages":[{"path":"cache","size":20,"description":"缓存"}]}`); err != nil {
		t.Fatalf("invoke add_top_usages: %v", err)
	}
	if len(ctx.TrashFiles) != 1 ||
		ctx.TrashFiles[0].Name != "清理临时日志" ||
		ctx.TrashFiles[0].Reason != "日志可以安全重新生成" ||
		ctx.TrashFiles[0].Path != "temp.log" ||
		ctx.TrashFiles[0].Level != cleaningrecord.LOW {
		t.Fatalf("TrashFiles = %#v", ctx.TrashFiles)
	}
	if len(ctx.TopUsages) != 1 || ctx.TopUsages[0].Path != "cache" {
		t.Fatalf("TopUsages = %#v", ctx.TopUsages)
	}
}

func TestAddTrashFileKeepsParentAndRemovesNestedPaths(t *testing.T) {
	ctx := &diskCleanerContext{}
	tool := newAddTrashFileTool()
	if _, err := tool.invoke(ctx, `{"files":[
		{"name":"清理日志","reason":"日志可重新生成","path":"/AppData/Local/JetBrains/IntelliJIdea2024.3/log","level":0},
		{"name":"保留相似目录","reason":"这是另一个版本","path":"/AppData/Local/JetBrains/IntelliJIdea2024.30","level":2}
	]}`); err != nil {
		t.Fatalf("invoke child paths: %v", err)
	}
	if _, err := tool.invoke(ctx, `{"files":[
		{"name":"清理历史版本","reason":"该历史版本已确认不再使用","path":"/AppData/Local/JetBrains/IntelliJIdea2024.3/","level":1}
	]}`); err != nil {
		t.Fatalf("invoke parent path: %v", err)
	}
	if len(ctx.TrashFiles) != 2 {
		t.Fatalf("TrashFiles = %#v, want parent and unrelated sibling", ctx.TrashFiles)
	}
	if ctx.TrashFiles[0].Path != "/AppData/Local/JetBrains/IntelliJIdea2024.30" {
		t.Fatalf("TrashFiles[0] = %#v", ctx.TrashFiles[0])
	}
	if ctx.TrashFiles[1].Path != "/AppData/Local/JetBrains/IntelliJIdea2024.3/" {
		t.Fatalf("TrashFiles[1] = %#v", ctx.TrashFiles[1])
	}
}
