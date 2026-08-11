package analyzer

import (
	_ "embed"
	"strings"
)

//go:embed SYSTEM_FAST.md
var systemPrompt string

//go:embed CONTEXT_COMPRESS_RULE_SUFFIX.md
var contextCompressRuleSuffix string

func buildBaseSystemPrompt(autoContextCompressEnabled bool) string {
	builder := strings.Builder{}
	builder.WriteString(systemPrompt)
	if autoContextCompressEnabled {
		builder.WriteString(contextCompressRuleSuffix)
	}
	return builder.String()
}
