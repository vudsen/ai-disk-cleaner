package analyzer

import (
	_ "embed"
	"fmt"
)

//go:embed SYSTEM.md
var systemPrompt string

//go:embed SYSTEM_FAST.md
var fastSystemPrompt string

func systemPromptForMode(scanMode string) (string, error) {
	switch scanMode {
	case "fast":
		return fastSystemPrompt, nil
	case "deep":
		return systemPrompt, nil
	default:
		return "", fmt.Errorf("analyze disk: unsupported scan mode %q", scanMode)
	}
}
