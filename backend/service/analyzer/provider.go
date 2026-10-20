package analyzer

import (
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"ai-disk-cleanner/backend/data/models/setting"
)

type providerKind string

const (
	providerAPI   providerKind = "api"
	providerCodex providerKind = "codex"

	defaultCodexTimeoutSeconds = 600
)

type codexConfig struct {
	executable string
	model      string
	timeout    int
}

type providerConfig struct {
	provider providerKind
	api      *llmConfig
	codex    *codexConfig
}

func parseProviderConfig(settings []setting.Setting) (*providerConfig, error) {
	values := make(map[string]string, len(settings))
	for _, item := range settings {
		values[item.Key] = item.Value
	}

	provider := providerKind(strings.TrimSpace(values["llm.provider"]))
	if provider == "" {
		provider = providerAPI
	}
	switch provider {
	case providerAPI:
		api, err := parseLLMConfig(settings)
		if err != nil {
			return nil, err
		}
		return &providerConfig{provider: providerAPI, api: api}, nil
	case providerCodex:
		executable := strings.TrimSpace(values["codex.executable"])
		if executable != "" && !filepath.IsAbs(executable) {
			return nil, errors.New("load Codex configuration: codex.executable must be an absolute path")
		}
		timeout := defaultCodexTimeoutSeconds
		if rawTimeout := strings.TrimSpace(values["codex.timeout-seconds"]); rawTimeout != "" {
			parsed, err := strconv.Atoi(rawTimeout)
			if err != nil || parsed < 30 || parsed > 3600 {
				return nil, errors.New("load Codex configuration: codex.timeout-seconds must be an integer from 30 to 3600")
			}
			timeout = parsed
		}
		return &providerConfig{
			provider: providerCodex,
			codex: &codexConfig{
				executable: executable,
				model:      strings.TrimSpace(values["codex.model"]),
				timeout:    timeout,
			},
		}, nil
	default:
		return nil, fmt.Errorf("load LLM configuration: unsupported provider %q", provider)
	}
}
