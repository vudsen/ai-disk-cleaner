package analyzer

import (
	"ai-disk-cleanner/backend/data/models/cleaningrecord"
	"ai-disk-cleanner/backend/data/models/setting"
	modelscanner "ai-disk-cleanner/backend/model/scanner"
	"ai-disk-cleanner/backend/service/tasklog"
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/openai/openai-go/v3"
)

// Service implements the LLM analysis service with an OpenAI-compatible API.
type Service struct {
	settings settingStore
}

// TestConnectionResult describes the outcome of an LLM connection test.
type TestConnectionResult struct {
	Type        string `json:"type"`
	I18nMessage string `json:"i18nMessage"`
}

func (err *analysisError) Error() string      { return err.cause.Error() }
func (err *analysisError) Unwrap() error      { return err.cause }
func (err *analysisError) LogSummary() string { return err.summary }
func (err *invalidResponseError) Error() string {
	return fmt.Sprintf("invalid LLM response: status=%d content-type=%s body=%s", err.status, err.contentType, err.body)
}

// NewService creates the analyzer service for the central service manager.
func NewService(settings settingStore) *Service {
	return newService(settings)
}

func newService(settings settingStore) *Service {
	return &Service{settings: settings}
}

func (analyzer *Service) Analyze(
	ctx context.Context,
	tree *modelscanner.FileTree,
	language string,
	onDelta func(string),
	session *tasklog.Session,
) (*cleaningrecord.AnalysisResult, error) {
	if tree == nil {
		return nil, errors.New("analyze disk: file tree is nil")
	}
	if onDelta == nil {
		onDelta = func(string) {}
	}
	config, err := analyzer.loadLLMConfig(ctx)
	if err != nil {
		return nil, err
	}
	prompt := buildBaseSystemPrompt(config.autoContextCompressEnabled)
	session.Event("分析开始", "模型=%s 语言=%s Token预算=%d 自动压缩=%t", config.model, language, config.maxTokens, config.autoContextCompressEnabled)
	agent, err := newAgent(ctx, tree, prompt, onDelta, config, language)
	if err != nil {
		return nil, err
	}
	agent.log = session
	run, err := agent.run()
	if err != nil {
		return nil, err
	}
	return run, nil
}

// TestConnection sends a minimal request using the supplied, potentially unsaved settings.
func (analyzer *Service) TestConnection(ctx context.Context, settings []setting.Setting) (TestConnectionResult, error) {
	config, err := parseLLMConfig(settings)
	if err != nil {
		return TestConnectionResult{}, err
	}

	params := openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage("reply ok only"),
		},
		Model:     config.model,
		MaxTokens: openai.Int(8),
	}
	extraFields := make(map[string]any, len(config.extraBody)+1)
	for key, value := range config.extraBody {
		if key == "messages" || key == "max_tokens" || key == "stream" {
			continue
		}
		extraFields[key] = value
	}
	extraFields["stream"] = false
	params.SetExtraFields(extraFields)

	client := newOpenAIClient(config)
	completion, err := client.Chat.Completions.New(ctx, params)
	if err != nil {
		return TestConnectionResult{}, fmt.Errorf("test LLM connection: %w", err)
	}
	message := make(map[string]any, 0)
	err = json.Unmarshal([]byte(completion.Choices[0].Message.RawJSON()), &message)
	if err != nil {
		return TestConnectionResult{}, err
	}
	_, ok := message["reasoning_content"]
	if ok {
		return TestConnectionResult{
			Type:        "warning",
			I18nMessage: "settings.testConnection.thinkingModeEnabled",
		}, nil
	}
	return TestConnectionResult{
		Type:        "success",
		I18nMessage: "",
	}, nil
}
