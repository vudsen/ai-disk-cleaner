package setting

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	settingmodel "ai-disk-cleanner/backend/data/models/setting"
)

type settingStore interface {
	ListSettings(context.Context) ([]settingmodel.Setting, error)
	SaveSettings(context.Context, []settingmodel.Setting) error
}

var supportedSettingKeys = map[string]struct{}{
	"llm.provider":              {},
	"llm.secret":                {},
	"llm.url":                   {},
	"llm.model":                 {},
	"llm.max-token":             {},
	"llm.auto-context-compress": {},
	"llm.extra-body":            {},
	"record.max.count":          {},
	"codex.executable":          {},
	"codex.model":               {},
	"codex.timeout-seconds":     {},
}

var legacySettingKeys = map[string]struct{}{
	"llm.secret":                {},
	"llm.url":                   {},
	"llm.model":                 {},
	"llm.max-token":             {},
	"llm.auto-context-compress": {},
	"llm.extra-body":            {},
	"record.max.count":          {},
}

func validateSettingShape(settings []settingmodel.Setting) error {
	if len(settings) != len(legacySettingKeys) && len(settings) != len(supportedSettingKeys) {
		return fmt.Errorf("all %d legacy or %d current settings must be provided", len(legacySettingKeys), len(supportedSettingKeys))
	}

	seen := make(map[string]struct{}, len(settings))
	for _, item := range settings {
		if _, supported := supportedSettingKeys[item.Key]; !supported {
			return fmt.Errorf("unsupported setting key %q", item.Key)
		}
		if _, duplicate := seen[item.Key]; duplicate {
			return fmt.Errorf("duplicate setting key %q", item.Key)
		}
		seen[item.Key] = struct{}{}
	}

	if len(settings) == len(legacySettingKeys) {
		for key := range legacySettingKeys {
			if _, exists := seen[key]; !exists {
				return fmt.Errorf("legacy settings are missing %q", key)
			}
		}
	} else if len(seen) != len(supportedSettingKeys) {
		return fmt.Errorf("all %d current settings must be provided", len(supportedSettingKeys))
	}
	return nil
}

func validateSettings(settings []settingmodel.Setting) error {
	if err := validateSettingShape(settings); err != nil {
		return err
	}
	values := make(map[string]string, len(settings))
	for _, item := range settings {
		values[item.Key] = item.Value
	}
	provider := strings.TrimSpace(values["llm.provider"])
	if provider == "" {
		provider = "api"
	}
	if provider != "api" && provider != "codex" {
		return fmt.Errorf("llm.provider must be api or codex")
	}
	if err := validatePositiveInteger(values, "record.max.count"); err != nil {
		return err
	}
	if provider == "api" {
		if err := validatePositiveInteger(values, "llm.max-token"); err != nil {
			return err
		}
		if _, err := strconv.ParseBool(values["llm.auto-context-compress"]); err != nil {
			return fmt.Errorf("llm.auto-context-compress must be a boolean")
		}
		extraBody := strings.TrimSpace(values["llm.extra-body"])
		if extraBody != "" {
			var decoded map[string]any
			if err := json.Unmarshal([]byte(extraBody), &decoded); err != nil || decoded == nil {
				return fmt.Errorf("llm.extra-body must be a JSON object")
			}
		}
		return nil
	}

	executable := strings.TrimSpace(values["codex.executable"])
	if executable != "" && !filepath.IsAbs(executable) {
		return fmt.Errorf("codex.executable must be an absolute path")
	}
	if timeout := strings.TrimSpace(values["codex.timeout-seconds"]); timeout != "" {
		seconds, err := strconv.Atoi(timeout)
		if err != nil || seconds < 30 || seconds > 3600 {
			return fmt.Errorf("codex.timeout-seconds must be an integer from 30 to 3600")
		}
	}
	return nil
}

func validatePositiveInteger(values map[string]string, key string) error {
	value, err := strconv.ParseInt(values[key], 10, 64)
	if err != nil || value <= 0 {
		return fmt.Errorf("%s must be a positive integer", key)
	}
	return nil
}
