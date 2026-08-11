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
	if agent.config.autoContextCompressEnabled {
		// 自动压缩模式下，状态在每次压缩后切换
		return nil
	}
	// 工具拒绝思考 + 输出总结 + 汇总 + ? + 保险
	if agent.usedTokens+agent.totalTokens*5 >= agent.config.maxTokens {
		agent.state = agentStateHigh
	}
	return nil
}

func shouldCompress(agent *Agent) bool {
	if agent.state == agentStateHigh || !agent.config.autoContextCompressEnabled {
		return false
	}
	return agent.totalTokens >= 12000 || shouldSwitchToAgentHighState(agent)
}

func (agent *Agent) afterCompletions() {}

func (agent *Agent) buildSystemPrompt() openai.ChatCompletionMessageParamUnion {
	builder := strings.Builder{}
	builder.WriteString(agent.baseSystemPrompt)
	if shouldCompress(agent) {
		builder.WriteString("<should_compress>true</should_compress>")
	} else {
		builder.WriteString("<should_compress>false</should_compress>")
	}
	return openai.SystemMessage(builder.String())
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
			Messages: agent.messages,
			Model:    agent.config.model,
			Tools:    buildTools(manager, agent),
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
		agent.usedTokens += completion.Usage.TotalTokens
		agent.totalTokens = completion.Usage.TotalTokens
		myLog.Println("Completion turn finished, total", completion.Usage.TotalTokens, "used", agent.usedTokens, "state", agent.state)
		agent.afterCompletions()
		if err := agent.ctx.Err(); err != nil {
			return nil, err
		}
		if len(completion.Choices) == 0 {
			return nil, errors.New("LLM returned no choices: " + completion.RawJSON())
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
		toolCalls := message.ToolCalls

		appendToolResult := true
		for _, item := range message.ToolCalls {
			if item.Function.Name == compressContextToolName {
				appendToolResult = false
				break
			}
		}
		for _, item := range toolCalls {
			function := item.Function
			result, err := manager.Invoke(
				function.Name,
				function.Arguments,
				agent,
			)
			if err != nil {
				result = err.Error()
			}
			if appendToolResult {
				agent.messages = append(agent.messages, openai.ToolMessage(result, item.ID))
			}
		}
	}

	return &cleaningrecord.AnalysisResult{
		TrashFiles: agent.TrashFiles,
		TopUsages:  agent.TopUsages,
		LLMOutput:  output.String(),
		TokenUsage: agent.usedTokens,
	}, nil
}
