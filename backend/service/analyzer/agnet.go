package analyzer

import (
	"ai-disk-cleanner/backend/data/models/cleaningrecord"
	"ai-disk-cleanner/backend/i18n"
	modelscanner "ai-disk-cleanner/backend/model/scanner"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/openai/openai-go/v3"
)

var myLog = log.New(
	os.Stdout,
	"[openai] ",
	log.LstdFlags,
)

type agentContextState string

const (
	agentContextStateLow agentContextState = "low"
	agentStateMedium     agentContextState = "medium"
	agentStateHigh       agentContextState = "high"
)

type Agent struct {
	ctx              context.Context
	tree             *modelscanner.FileTree
	baseSystemPrompt string
	onDelta          func(string)
	config           *llmConfig
	language         string
	state            agentContextState
	usedTokens       int64
	totalTokens      int64
	messages         []openai.ChatCompletionMessageParamUnion
	TrashFiles       []cleaningrecord.TrashFile
	TopUsages        []cleaningrecord.DiskUsage
}

func newAgent(
	ctx context.Context,
	tree *modelscanner.FileTree,
	baseSystemPrompt string,
	onDelta func(string),
	config *llmConfig,
	language string,
) (*Agent, error) {
	if onDelta == nil {
		onDelta = func(string) {}
	}
	if tree == nil {
		return nil, errors.New("analyze disk: file tree is nil")
	}
	return &Agent{
		ctx:              ctx,
		tree:             tree,
		baseSystemPrompt: baseSystemPrompt,
		onDelta:          onDelta,
		config:           config,
		language:         language,
		state:            agentContextStateLow,
		usedTokens:       0,
		messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(baseSystemPrompt),
			openai.UserMessage(i18n.AnalyzerUserPrompt(language)),
		},
		TrashFiles: make([]cleaningrecord.TrashFile, 0),
		TopUsages:  make([]cleaningrecord.DiskUsage, 0),
	}, nil
}

func (agent *Agent) beforeCompletions() error {
	if agent.usedTokens >= agent.config.maxTokens {
		return errors.New("analyze disk: Maximum number of tokens exceeded")
	}
	return nil
}

func (agent *Agent) afterCompletions() {
	if agent.totalTokens >= 7000 {
		myLog.Println("Switch to agent high state")
		agent.state = agentStateHigh
	} else {
		agent.state = agentContextStateLow
	}
}

const agentContextUsageMediumSuffix = `<agent_runtime_instruction>
WARNING: You have used half of the context size. Use 'clear_analyze_history' only for specific non-root child directories whose descendant scan results you have already read, whose analysis is complete, and which you will no longer reference. Never pass '/' and never clear a directory branch that is still being analyzed or needed for later reasoning.
</agent_runtime_instruction>
`
const agentContextUsageHighSuffix = `<agent_runtime_instruction>
URGENT: You have used most of the context size. Either stop scanning and summarize the final result, or use 'clear_analyze_history' only on specific non-root child directories whose descendant scan results you have already read, finished analyzing, and will no longer reference. Never pass '/' and do not clear active directory branches.
</agent_runtime_instruction>
`

func (agent *Agent) buildSystemPrompt() openai.ChatCompletionMessageParamUnion {
	switch agent.state {
	case agentStateMedium:
		return openai.SystemMessage(agent.baseSystemPrompt + agentContextUsageMediumSuffix)
	case agentStateHigh:
		return openai.SystemMessage(agent.baseSystemPrompt + agentContextUsageHighSuffix)
	default:
		return openai.SystemMessage(agent.baseSystemPrompt)
	}
}

func (agent *Agent) run() (*cleaningrecord.AnalysisResult, error) {
	client := newOpenAIClient(agent.config)

	manager := newManager()

	var output strings.Builder
	for {
		if err := agent.ctx.Err(); err != nil {
			return nil, err
		}

		params := openai.ChatCompletionNewParams{
			Messages:  agent.messages,
			Model:     agent.config.model,
			Tools:     buildTools(manager, agent),
			MaxTokens: openai.Int(50000),
		}
		extraFields := make(map[string]any, len(agent.config.extraBody)+1)
		for key, value := range agent.config.extraBody {
			extraFields[key] = value
		}
		extraFields["stream"] = false
		if len(extraFields) > 0 {
			params.SetExtraFields(extraFields)
		}

		agent.messages[0] = agent.buildSystemPrompt()
		err := agent.beforeCompletions()
		if err != nil {
			return nil, err
		}
		completion, err := client.Chat.Completions.New(agent.ctx, params)
		if err != nil {
			return nil, fmt.Errorf("analyze disk: %w", err)
		}
		agent.usedTokens += completion.Usage.PromptTokens + completion.Usage.TotalTokens
		agent.totalTokens = completion.Usage.TotalTokens
		myLog.Println("Completion turn finished, total", completion.Usage.TotalTokens, "used", agent.usedTokens, "state", agent.state)
		agent.afterCompletions()
		if err := agent.ctx.Err(); err != nil {
			return nil, err
		}
		if len(completion.Choices) == 0 {
			return nil, errors.New("analyze disk: LLM returned no choices")
		}

		message := completion.Choices[0].Message
		if message.Content != "" {
			output.WriteString(message.Content)
			agent.onDelta(message.Content)
		}
		if len(message.ToolCalls) == 0 {
			break
		}
		agent.messages = append(agent.messages, message.ToParam())
		for _, item := range message.ToolCalls {
			function := item.Function
			result, err := manager.Invoke(
				function.Name,
				function.Arguments,
				agent,
			)
			if err != nil {
				result = err.Error()
			}
			agent.messages = append(agent.messages, openai.ToolMessage(result, item.ID))
		}
	}

	return &cleaningrecord.AnalysisResult{
		TrashFiles: agent.TrashFiles,
		TopUsages:  agent.TopUsages,
		LLMOutput:  output.String(),
		TokenUsage: agent.usedTokens,
	}, nil
}
