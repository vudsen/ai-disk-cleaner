package analyzer

import (
	"encoding/json"
	"testing"
)

func TestParseCodexMCPServerNamesReturnsOnlySortedNames(t *testing.T) {
	response := json.RawMessage(`{"config":{"mcp_servers":{"zeta":{"command":"PRIVATE_COMMAND","env":{"TOKEN":"PRIVATE_TOKEN"}},"alpha":{"url":"https://private.example"}}}}`)
	names, err := parseCodexMCPServerNames(response)
	if err != nil {
		t.Fatalf("parseCodexMCPServerNames() error = %v", err)
	}
	if len(names) != 2 || names[0] != "alpha" || names[1] != "zeta" {
		t.Fatalf("MCP names = %#v", names)
	}
}

func TestParseCodexMCPServerNamesFailsClosedOnInvalidConfig(t *testing.T) {
	for _, response := range []json.RawMessage{
		json.RawMessage(`{"config":{"mcp_servers":null}}`),
		json.RawMessage(`{"config":{}}`),
		json.RawMessage(`not JSON`),
	} {
		if _, err := parseCodexMCPServerNames(response); err == nil {
			t.Errorf("parseCodexMCPServerNames(%s) error = nil", response)
		}
	}
}

func TestParseCodexAccountReadRespectsAuthenticationRequirement(t *testing.T) {
	for _, test := range []struct {
		name     string
		response string
		ready    bool
		code     string
	}{
		{name: "login required", response: `{"requiresOpenaiAuth":true,"account":null}`, ready: false, code: "not_authenticated"},
		{name: "account present", response: `{"requiresOpenaiAuth":true,"account":{"type":"chatgpt"}}`, ready: true, code: "ready"},
		{name: "custom provider", response: `{"requiresOpenaiAuth":false,"account":null}`, ready: true, code: "ready"},
	} {
		t.Run(test.name, func(t *testing.T) {
			status, err := parseCodexAccountRead(json.RawMessage(test.response))
			if err != nil {
				t.Fatalf("parseCodexAccountRead() error = %v", err)
			}
			if status.Ready != test.ready || status.Code != test.code {
				t.Fatalf("status = %#v", status)
			}
		})
	}
}

func TestCodexModelIsOptionalForThreadStart(t *testing.T) {
	if got := codexThreadModel(&codexConfig{}); got != nil {
		t.Fatalf("empty model = %#v, want omitted", got)
	}
	if got := codexThreadModel(&codexConfig{model: "local-model"}); got == nil || *got != "local-model" {
		t.Fatalf("configured model = %#v", got)
	}
}
