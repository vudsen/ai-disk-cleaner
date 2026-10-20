package analyzer

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"ai-disk-cleanner/backend/data/models/setting"
)

func legacyAPISettings(baseURL string) []setting.Setting {
	return []setting.Setting{
		{Key: "llm.secret", Value: "test-key"},
		{Key: "llm.url", Value: baseURL},
		{Key: "llm.model", Value: "legacy-model"},
		{Key: "llm.max-token", Value: "620000"},
		{Key: "llm.auto-context-compress", Value: "true"},
		{Key: "llm.extra-body", Value: `{"temperature":0.2,"messages":[{"role":"system","content":"ignored"}],"max_tokens":1,"stream":true}`},
		{Key: "record.max.count", Value: "100"},
	}
}

func TestParseProviderConfigDefaultsLegacySevenKeySettingsToAPI(t *testing.T) {
	provider, err := parseProviderConfig(legacyAPISettings("https://example.test/v1"))
	if err != nil {
		t.Fatalf("parseProviderConfig() error = %v", err)
	}
	if provider.provider != providerAPI || provider.api == nil || provider.codex != nil {
		t.Fatalf("provider = %#v, want API branch", provider)
	}
	if provider.api.model != "legacy-model" || provider.api.secret != "test-key" || provider.api.maxTokens != 620000 {
		t.Fatalf("API config = %#v", provider.api)
	}
}

func TestParseProviderConfigCodexIgnoresInactiveAPIValues(t *testing.T) {
	executable := filepath.Join(t.TempDir(), "path with spaces", "codex")
	settings := []setting.Setting{
		{Key: "llm.provider", Value: "codex"},
		{Key: "llm.secret", Value: "stale secret"},
		{Key: "llm.url", Value: "not a URL"},
		{Key: "llm.model", Value: "stale API model"},
		{Key: "llm.max-token", Value: "invalid"},
		{Key: "llm.auto-context-compress", Value: "invalid"},
		{Key: "llm.extra-body", Value: "not JSON"},
		{Key: "record.max.count", Value: "100"},
		{Key: "codex.executable", Value: executable},
		{Key: "codex.model", Value: ""},
		{Key: "codex.timeout-seconds", Value: "600"},
	}
	provider, err := parseProviderConfig(settings)
	if err != nil {
		t.Fatalf("parseProviderConfig() error = %v", err)
	}
	if provider.provider != providerCodex || provider.api != nil || provider.codex == nil {
		t.Fatalf("provider = %#v, want Codex branch", provider)
	}
	if provider.codex.executable != executable || provider.codex.model != "" || provider.codex.timeout != 600 {
		t.Fatalf("Codex config = %#v", provider.codex)
	}
}

func TestParseProviderConfigRejectsInvalidProviderAndTimeout(t *testing.T) {
	for _, test := range []struct {
		name     string
		provider string
		exec     string
		timeout  string
	}{
		{name: "unknown provider", provider: "other", timeout: "600"},
		{name: "relative executable", provider: "codex", exec: "codex", timeout: "600"},
		{name: "timeout too short", provider: "codex", exec: "/usr/bin/codex", timeout: "29"},
		{name: "timeout too long", provider: "codex", exec: "/usr/bin/codex", timeout: "3601"},
		{name: "non integer timeout", provider: "codex", exec: "/usr/bin/codex", timeout: "soon"},
	} {
		t.Run(test.name, func(t *testing.T) {
			settings := []setting.Setting{
				{Key: "llm.provider", Value: test.provider},
				{Key: "codex.executable", Value: test.exec},
				{Key: "codex.timeout-seconds", Value: test.timeout},
			}
			if _, err := parseProviderConfig(settings); err == nil {
				t.Fatal("parseProviderConfig() error = nil")
			}
		})
	}
}

func TestTestConnectionPreservesAPIRequest(t *testing.T) {
	var request map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()

	service := newService(nil)
	result, err := service.TestConnection(context.Background(), legacyAPISettings(server.URL+"/v1"))
	if err != nil {
		t.Fatalf("TestConnection() error = %v", err)
	}
	if result.Type != "success" {
		t.Fatalf("TestConnection() result = %#v", result)
	}
	if request["model"] != "legacy-model" || request["max_tokens"] != float64(8) || request["stream"] != false || request["temperature"] != float64(0.2) {
		t.Fatalf("API request = %#v", request)
	}
	if len(request["messages"].([]any)) != 1 {
		t.Fatalf("extra body replaced API messages: %#v", request["messages"])
	}
}

func TestTestConnectionDoesNotFallBackFromCodexToAPI(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"unexpected"}}]}`))
	}))
	defer server.Close()
	settings := append(legacyAPISettings(server.URL+"/v1"),
		setting.Setting{Key: "llm.provider", Value: "codex"},
		setting.Setting{Key: "codex.executable", Value: filepath.Join(t.TempDir(), "missing-codex")},
		setting.Setting{Key: "codex.model", Value: ""},
		setting.Setting{Key: "codex.timeout-seconds", Value: "600"},
	)

	_, err := newService(nil).TestConnection(context.Background(), settings)
	if err == nil {
		t.Fatalf("TestConnection() error = %v, want local Codex probe result", err)
	}
	if requests != 0 {
		t.Fatalf("API requests = %d, want no fallback request", requests)
	}
}

func TestTestConnectionPreservesAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"invalid model"}}`))
	}))
	defer server.Close()

	result, err := newService(nil).TestConnection(context.Background(), legacyAPISettings(server.URL+"/v1"))
	if err == nil || !strings.Contains(err.Error(), "test LLM connection") {
		t.Fatalf("TestConnection() = (%#v, %v), want wrapped API error", result, err)
	}
}
