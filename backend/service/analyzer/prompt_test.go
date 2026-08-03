package analyzer

import (
	"strings"
	"testing"
)

func TestSystemPromptForMode(t *testing.T) {
	fastPrompt, err := systemPromptForMode("fast")
	if err != nil {
		t.Fatalf("systemPromptForMode(fast) error = %v", err)
	}
	if !strings.Contains(fastPrompt, "快速、保守") {
		t.Fatal("fast mode did not select SYSTEM_FAST.md")
	}

	deepPrompt, err := systemPromptForMode("deep")
	if err != nil {
		t.Fatalf("systemPromptForMode(deep) error = %v", err)
	}
	if !strings.Contains(deepPrompt, "极度保守") {
		t.Fatal("deep mode did not select SYSTEM.md")
	}

	if _, err := systemPromptForMode("unknown"); err == nil {
		t.Fatal("unknown scan mode was accepted")
	}
}
