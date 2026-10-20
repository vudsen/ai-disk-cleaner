package setting

import (
	"context"

	appctx "ai-disk-cleanner/backend/ctx"
	settingmodel "ai-disk-cleanner/backend/data/models/setting"
)

// Service exposes system configuration operations.
type Service struct {
	ctx   context.Context
	store settingStore
}

// NewService creates the setting service for the central service manager.
func NewService(store settingStore) *Service {
	return newService(appctx.GetContext(), store)
}

func newService(ctx context.Context, store settingStore) *Service {
	return &Service{ctx: ctx, store: store}
}

func (service *Service) List() ([]settingmodel.Setting, error) {
	return service.store.ListSettings(service.ctx)
}

func (service *Service) Save(settings []settingmodel.Setting) error {
	if err := validateSettingShape(settings); err != nil {
		return err
	}
	settingsToSave := settings
	if len(settings) == len(legacySettingKeys) {
		current, err := service.store.ListSettings(service.ctx)
		if err != nil {
			return err
		}
		values := make(map[string]string, len(current))
		for _, item := range current {
			values[item.Key] = item.Value
		}
		defaults := map[string]string{
			"llm.provider":          "api",
			"codex.executable":      "",
			"codex.model":           "",
			"codex.timeout-seconds": "600",
		}
		settingsToSave = append([]settingmodel.Setting(nil), settings...)
		for key, defaultValue := range defaults {
			value, exists := values[key]
			if !exists {
				value = defaultValue
			}
			settingsToSave = append(settingsToSave, settingmodel.Setting{Key: key, Value: value})
		}
	}
	if err := validateSettings(settingsToSave); err != nil {
		return err
	}
	return service.store.SaveSettings(service.ctx, settingsToSave)
}
