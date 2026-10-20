package analyzer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sort"
)

func parseCodexMCPServerNames(response json.RawMessage) ([]string, error) {
	var envelope struct {
		Config json.RawMessage `json:"config"`
	}
	if err := json.Unmarshal(response, &envelope); err != nil || len(envelope.Config) == 0 || string(envelope.Config) == "null" {
		return nil, errors.New("Codex config/read returned an invalid config")
	}
	var config map[string]json.RawMessage
	if err := json.Unmarshal(envelope.Config, &config); err != nil || config == nil {
		return nil, errors.New("Codex config/read returned an invalid config object")
	}
	rawServers, exists := config["mcp_servers"]
	if !exists || string(rawServers) == "null" {
		return nil, errors.New("Codex config/read did not include MCP server configuration")
	}
	var servers map[string]json.RawMessage
	if err := json.Unmarshal(rawServers, &servers); err != nil || servers == nil {
		return nil, errors.New("Codex config/read returned invalid MCP server configuration")
	}
	names := make([]string, 0, len(servers))
	for name := range servers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

func parseCodexAccountRead(response json.RawMessage) (ProviderStatus, error) {
	var accountResponse struct {
		RequiresOpenAIAuth *bool           `json:"requiresOpenaiAuth"`
		Account            json.RawMessage `json:"account"`
	}
	if err := json.Unmarshal(response, &accountResponse); err != nil {
		return ProviderStatus{}, fmt.Errorf("decode Codex account status: %w", err)
	}
	if accountResponse.RequiresOpenAIAuth == nil {
		return ProviderStatus{}, errors.New("Codex account/read omitted authentication requirement")
	}
	if !*accountResponse.RequiresOpenAIAuth {
		return ProviderStatus{Ready: true, Code: "ready"}, nil
	}
	if len(accountResponse.Account) == 0 || string(accountResponse.Account) == "null" {
		return ProviderStatus{Ready: false, Code: "not_authenticated"}, nil
	}
	return ProviderStatus{Ready: true, Code: "ready"}, nil
}

func codexThreadModel(config *codexConfig) *string {
	if config == nil || config.model == "" {
		return nil
	}
	model := config.model
	return &model
}

func probeCodexStatus(ctx context.Context, config *codexConfig) (ProviderStatus, error) {
	executable, err := resolveCodexExecutable(config.executable)
	if errors.Is(err, ErrCodexExecutableNotFound) {
		return ProviderStatus{Code: "executable_not_found"}, nil
	}
	if errors.Is(err, ErrCodexUnsupportedExec) {
		return ProviderStatus{Code: "cli_unsupported"}, nil
	}
	if err != nil {
		return ProviderStatus{}, err
	}
	versionCtx, cancel := context.WithTimeout(ctx, codexHandshakeTimeout)
	defer cancel()
	command := exec.CommandContext(versionCtx, executable, "--version")
	command.Stderr = io.Discard
	versionOutput, err := command.Output()
	if err != nil {
		if versionCtx.Err() != nil {
			return ProviderStatus{}, fmt.Errorf("read Codex CLI version: %w", versionCtx.Err())
		}
		return ProviderStatus{Code: "cli_unsupported"}, nil
	}
	version, versionErr := parseCodexVersion(string(versionOutput))
	if versionErr != nil {
		return ProviderStatus{Code: "cli_unsupported", Version: version}, nil
	}

	names, err := readCodexMCPServerNames(ctx, executable)
	if err != nil {
		return ProviderStatus{Version: version}, err
	}
	args, err := codexIsolationArgs(names)
	if err != nil {
		return ProviderStatus{Version: version}, err
	}
	session, err := startCodexAppServer(ctx, executable, args)
	if err != nil {
		return ProviderStatus{Version: version}, err
	}
	defer session.close()
	response, err := codexTimedRequest(ctx, session.protocol, "account/read", map[string]any{})
	if err != nil {
		return ProviderStatus{Version: version}, fmt.Errorf("read Codex account status: %w", err)
	}
	status, err := parseCodexAccountRead(response)
	if err != nil {
		return ProviderStatus{Version: version}, err
	}
	status.Version = version
	return status, nil
}

func readCodexMCPServerNames(ctx context.Context, executable string) ([]string, error) {
	session, err := startCodexAppServer(ctx, executable, []string{"app-server", "--listen", "stdio://"})
	if err != nil {
		return nil, err
	}
	defer session.close()
	requestCtx, cancel := context.WithTimeout(ctx, codexHandshakeTimeout)
	defer cancel()
	response, err := session.protocol.request(requestCtx, "config/read", map[string]any{
		"cwd":           session.cwd,
		"includeLayers": false,
	})
	if err != nil {
		return nil, fmt.Errorf("read Codex MCP configuration: %w", err)
	}
	return parseCodexMCPServerNames(response)
}
