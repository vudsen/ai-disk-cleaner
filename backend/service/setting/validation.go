package setting

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	settingmodel "ai-disk-cleanner/backend/data/models/setting"
)

type settingStore interface {
	ListSettings(context.Context) ([]settingmodel.Setting, error)
	SaveSettings(context.Context, []settingmodel.Setting) error
}

var supportedSettingKeys = map[string]struct{}{
	"llm.secret":                {},
	"llm.url":                   {},
	"llm.model":                 {},
	"llm.max-token":             {},
	"llm.auto-context-compress": {},
	"llm.extra-body":            {},
	"record.max.count":          {},
}

func validateSettings(settings []settingmodel.Setting) error {
	if len(settings) != len(supportedSettingKeys) {
		return fmt.Errorf("all %d settings must be provided", len(supportedSettingKeys))
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
		if item.Key == "llm.max-token" || item.Key == "record.max.count" {
			value, err := strconv.ParseInt(item.Value, 10, 64)
			if err != nil || value <= 0 {
				return fmt.Errorf("%s must be a positive integer", item.Key)
			}
		}
		if item.Key == "llm.auto-context-compress" {
			if _, err := strconv.ParseBool(item.Value); err != nil {
				return fmt.Errorf("%s must be a boolean", item.Key)
			}
		}
		if item.Key == "llm.extra-body" {
			value := strings.TrimSpace(item.Value)
			if value == "" {
				continue
			}
			var extraBody map[string]any
			if err := json.Unmarshal([]byte(value), &extraBody); err != nil || extraBody == nil {
				return fmt.Errorf("%s must be a JSON object", item.Key)
			}
		}
	}
	return nil
}
