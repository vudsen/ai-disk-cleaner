package analyzer

import (
	_ "embed"
	"fmt"
	"strings"
)

//go:embed SYSTEM.md
var systemPrompt string

//go:embed SYSTEM_FAST.md
var fastSystemPrompt string

//go:embed CONTEXT_COMPRESS_RULE_SUFFIX.md
var contextCompressRuleSuffix string

func systemPromptForMode(scanMode string, autoContextCompressEnabled bool) (string, error) {
	builder := strings.Builder{}
	switch scanMode {
	case "fast":
		builder.WriteString(fastSystemPrompt)
		break
	case "deep":
		builder.WriteString(systemPrompt)
		break
	default:
		return "", fmt.Errorf("analyze disk: unsupported scan mode %q", scanMode)
	}
	if autoContextCompressEnabled {
		builder.WriteString(contextCompressRuleSuffix)
	}
	return builder.String(), nil
}
