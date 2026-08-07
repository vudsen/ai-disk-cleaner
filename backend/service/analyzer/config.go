package analyzer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"ai-disk-cleanner/backend/data/models/setting"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

type settingStore interface {
	ListSettings(context.Context) ([]setting.Setting, error)
}

type llmConfig struct {
	secret    string
	baseURL   string
	model     string
	maxTokens int64
	extraBody map[string]any
}

func (analyzer *Service) loadLLMConfig(ctx context.Context) (llmConfig, error) {
	if analyzer.settings == nil {
		return llmConfig{}, errors.New("load LLM configuration: setting store is nil")
	}
	settings, err := analyzer.settings.ListSettings(ctx)
	if err != nil {
		return llmConfig{}, fmt.Errorf("load LLM configuration: %w", err)
	}
	values := make(map[string]string, len(settings))
	for _, setting := range settings {
		values[setting.Key] = setting.Value
	}

	config := llmConfig{
		secret:  strings.TrimSpace(values["llm.secret"]),
		baseURL: strings.TrimSpace(values["llm.url"]),
		model:   strings.TrimSpace(values["llm.model"]),
	}
	if config.secret == "" {
		return llmConfig{}, errors.New("load LLM configuration: llm.secret is empty")
	}
	if config.baseURL == "" {
		return llmConfig{}, errors.New("load LLM configuration: llm.url is empty")
	}
	if config.model == "" {
		return llmConfig{}, errors.New("load LLM configuration: llm.model is empty")
	}
	config.maxTokens, err = strconv.ParseInt(strings.TrimSpace(values["llm.max-token"]), 10, 64)
	if err != nil || config.maxTokens <= 0 {
		return llmConfig{}, errors.New("load LLM configuration: llm.max-token must be a positive integer")
	}
	extraBody := strings.TrimSpace(values["llm.extra-body"])
	if extraBody != "" {
		if err := json.Unmarshal([]byte(extraBody), &config.extraBody); err != nil || config.extraBody == nil {
			return llmConfig{}, errors.New("load LLM configuration: llm.extra-body must be a JSON object")
		}
	}
	return config, nil
}

type debugTransport struct {
	rt http.RoundTripper
}

func truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "..."
}
func (t debugTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.rt.RoundTrip(req)
	if err != nil {
		return nil, err
	}

	contentType := resp.Header.Get("Content-Type")

	if resp.StatusCode >= 400 ||
		!strings.Contains(contentType, "application/json") {

		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		return nil, fmt.Errorf(
			"invalid LLM response: status=%d content-type=%s body=%s",
			resp.StatusCode,
			contentType,
			truncate(string(body), 1024),
		)
	}

	return resp, nil
}
func newOpenAIClient(config llmConfig) openai.Client {
	client := &http.Client{
		Transport: debugTransport{
			rt: http.DefaultTransport,
		},
	}
	return openai.NewClient(
		option.WithAPIKey(config.secret),
		option.WithBaseURL(config.baseURL),
		//option.WithDebugLog(log.New(
		//	os.Stdout,
		//	"[openai] ",
		//	log.LstdFlags,
		//)),
		option.WithHTTPClient(client),
	)
}
