package analyzer

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"ai-disk-cleanner/backend/data/models/cleaningrecord"
	"ai-disk-cleanner/backend/i18n"
	modelscanner "ai-disk-cleanner/backend/model/scanner"

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
	prompt, err := systemPromptForMode(scanMode)
	if err != nil {
		return nil, err
	}
	config, err := analyzer.loadLLMConfig(ctx)
	if err != nil {
		return nil, err
	}
	client := newOpenAIClient(config)

	messages := []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage(prompt),
		openai.UserMessage(i18n.AnalyzerUserPrompt(language)),
	}
	manager := newManager()
	diskContext := newDiskCleanerContext(tree)
	llmTools := buildTools(manager)

	var output strings.Builder
	var tokenUsage int64
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		params := openai.ChatCompletionNewParams{
			Messages:  messages,
			Model:     config.model,
			Tools:     llmTools,
			MaxTokens: openai.Int(config.maxTokens),
			StreamOptions: openai.ChatCompletionStreamOptionsParam{
				IncludeUsage: openai.Bool(true),
			},
		}
		extraFields := make(map[string]any, len(config.extraBody)+1)
		for key, value := range config.extraBody {
			extraFields[key] = value
		}
		if len(extraFields) > 0 {
			params.SetExtraFields(extraFields)
		}

		stream := client.Chat.Completions.NewStreaming(
			ctx,
			params,
		)
		accumulator := openai.ChatCompletionAccumulator{}
		for stream.Next() {
			chunk := stream.Current()
			if !accumulator.AddChunk(chunk) {
				_ = stream.Close()
				return nil, errors.New("analyze disk: could not accumulate LLM stream")
			}
			if len(chunk.Choices) > 0 {
				delta := chunk.Choices[0].Delta.Content
				if delta != "" {
					output.WriteString(delta)
					onDelta(delta)
				}
			}
		}
		streamErr := stream.Err()
		_ = stream.Close()
		if streamErr != nil {
			return nil, fmt.Errorf("analyze disk stream: %w", streamErr)
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if len(accumulator.Choices) == 0 {
			return nil, errors.New("analyze disk: LLM returned no choices")
		}

		tokenUsage += accumulator.Usage.CompletionTokens
		message := accumulator.Choices[0].Message
		if len(message.ToolCalls) == 0 {
			break
		}
		messages = append(messages, message.ToParam())
		for _, item := range message.ToolCalls {
			function := item.Function
			result, err := manager.Invoke(
				function.Name,
				function.Arguments,
				diskContext,
			)
			if err != nil {
				result = err.Error()
			}
			messages = append(messages, openai.ToolMessage(result, item.ID))
		}
	}

	return &cleaningrecord.AnalysisResult{
		TrashFiles: diskContext.TrashFiles,
		TopUsages:  diskContext.TopUsages,
		LLMOutput:  output.String(),
		TokenUsage: tokenUsage,
	}, nil
}
