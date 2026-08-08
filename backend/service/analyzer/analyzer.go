package analyzer

import (
	"ai-disk-cleanner/backend/data/models/cleaningrecord"
	"ai-disk-cleanner/backend/data/models/setting"
	modelscanner "ai-disk-cleanner/backend/model/scanner"
	"context"
	"errors"
	"fmt"

	"github.com/openai/openai-go/v3"
)

// Service implements the LLM analysis service with an OpenAI-compatible API.
type Service struct {
	settings settingStore
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
	scanMode string,
	onDelta func(string),
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
	prompt, err := systemPromptForMode(scanMode, config.autoContextCompressEnabled)
	if err != nil {
		return nil, err
	}
	agent, err := newAgent(ctx, tree, prompt, onDelta, config, language)
	if err != nil {
		return nil, err
	}
	run, err := agent.run()
	if err != nil {
		return nil, err
	}
	return run, nil
}

// TestConnection sends a minimal request using the supplied, potentially unsaved settings.
func (analyzer *Service) TestConnection(ctx context.Context, settings []setting.Setting) error {
	config, err := parseLLMConfig(settings)
	if err != nil {
		return err
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
	if _, err := client.Chat.Completions.New(ctx, params); err != nil {
		return fmt.Errorf("test LLM connection: %w", err)
	}
	return nil
}
