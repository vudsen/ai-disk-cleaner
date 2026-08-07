package analyzer

import (
	"ai-disk-cleanner/backend/data/models/cleaningrecord"
	modelscanner "ai-disk-cleanner/backend/model/scanner"
	"context"
	"errors"
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
