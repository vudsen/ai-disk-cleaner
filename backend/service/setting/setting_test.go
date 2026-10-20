package setting

import (
	"context"
	"strings"
	"testing"

	settingmodel "ai-disk-cleanner/backend/data/models/setting"
)

type fakeSettingStore struct {
	existing []settingmodel.Setting
	saved    []settingmodel.Setting
}

func (store *fakeSettingStore) ListSettings(context.Context) ([]settingmodel.Setting, error) {
	return append([]settingmodel.Setting(nil), store.existing...), nil
}

func (store *fakeSettingStore) SaveSettings(_ context.Context, settings []settingmodel.Setting) error {
	store.saved = settings
	return nil
}

func TestSaveValidatesAndPersistsSettings(t *testing.T) {
	store := &fakeSettingStore{}
	service := newService(context.Background(), store)
	settings := []settingmodel.Setting{
		{Key: "llm.secret", Value: "secret"},
		{Key: "llm.url", Value: "https://example.com/v1"},
		{Key: "llm.model", Value: "model"},
		{Key: "llm.max-token", Value: "50000"},
		{Key: "llm.auto-context-compress", Value: "false"},
		{Key: "llm.extra-body", Value: `{"temperature":0.7}`},
		{Key: "record.max.count", Value: "10"},
	}

	if err := service.Save(settings); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if len(store.saved) != 11 {
		t.Fatalf("saved %d settings, want all 11 settings", len(store.saved))
	}
}

func TestSaveRejectsInvalidMaxToken(t *testing.T) {
	store := &fakeSettingStore{}
	service := newService(context.Background(), store)
	settings := []settingmodel.Setting{
		{Key: "llm.secret", Value: ""},
		{Key: "llm.url", Value: ""},
		{Key: "llm.model", Value: ""},
		{Key: "llm.max-token", Value: "0"},
		{Key: "llm.auto-context-compress", Value: "false"},
		{Key: "llm.extra-body", Value: ""},
		{Key: "record.max.count", Value: "10"},
	}

	if err := service.Save(settings); err == nil {
		t.Fatal("Save() error = nil, want validation error")
	}
	if len(store.saved) != 0 {
		t.Fatal("invalid settings were persisted")
	}
}

func TestSaveRejectsInvalidRecordMaxCount(t *testing.T) {
	store := &fakeSettingStore{}
	service := newService(context.Background(), store)
	settings := []settingmodel.Setting{
		{Key: "llm.secret", Value: ""},
		{Key: "llm.url", Value: ""},
		{Key: "llm.model", Value: ""},
		{Key: "llm.max-token", Value: "50000"},
		{Key: "llm.auto-context-compress", Value: "false"},
		{Key: "llm.extra-body", Value: ""},
		{Key: "record.max.count", Value: "0"},
	}

	if err := service.Save(settings); err == nil {
		t.Fatal("Save() error = nil, want validation error")
	}
	if len(store.saved) != 0 {
		t.Fatal("invalid settings were persisted")
	}
}

func TestSaveRejectsInvalidExtraBody(t *testing.T) {
	store := &fakeSettingStore{}
	service := newService(context.Background(), store)
	settings := []settingmodel.Setting{
		{Key: "llm.secret", Value: ""},
		{Key: "llm.url", Value: ""},
		{Key: "llm.model", Value: ""},
		{Key: "llm.max-token", Value: "50000"},
		{Key: "llm.auto-context-compress", Value: "false"},
		{Key: "llm.extra-body", Value: "[]"},
		{Key: "record.max.count", Value: "10"},
	}

	if err := service.Save(settings); err == nil {
		t.Fatal("Save() error = nil, want validation error")
	}
	if len(store.saved) != 0 {
		t.Fatal("invalid settings were persisted")
	}
}

func TestSaveLegacySevenPreservesProviderSettings(t *testing.T) {
	store := &fakeSettingStore{existing: []settingmodel.Setting{
		{Key: "llm.provider", Value: "codex"},
		{Key: "codex.executable", Value: "/opt/Codex CLI/codex"},
		{Key: "codex.model", Value: "local-model"},
		{Key: "codex.timeout-seconds", Value: "1200"},
	}}
	service := newService(context.Background(), store)
	legacy := []settingmodel.Setting{
		{Key: "llm.secret", Value: "legacy-secret"},
		{Key: "llm.url", Value: "https://example.com/v1"},
		{Key: "llm.model", Value: "legacy-model"},
		{Key: "llm.max-token", Value: "50000"},
		{Key: "llm.auto-context-compress", Value: "false"},
		{Key: "llm.extra-body", Value: ""},
		{Key: "record.max.count", Value: "10"},
	}
	if err := service.Save(legacy); err != nil {
		t.Fatalf("Save(legacy) error = %v", err)
	}
	got := make(map[string]string, len(store.saved))
	for _, item := range store.saved {
		got[item.Key] = item.Value
	}
	if len(got) != 11 || got["llm.provider"] != "codex" || got["codex.executable"] != "/opt/Codex CLI/codex" || got["codex.model"] != "local-model" || got["codex.timeout-seconds"] != "1200" {
		t.Fatalf("saved settings = %#v", got)
	}
}

func TestSaveCodexElevenSettingsIgnoresInactiveAPIValues(t *testing.T) {
	store := &fakeSettingStore{}
	service := newService(context.Background(), store)
	settings := []settingmodel.Setting{
		{Key: "llm.provider", Value: "codex"},
		{Key: "llm.secret", Value: ""},
		{Key: "llm.url", Value: ""},
		{Key: "llm.model", Value: ""},
		{Key: "llm.max-token", Value: "inactive"},
		{Key: "llm.auto-context-compress", Value: "inactive"},
		{Key: "llm.extra-body", Value: "not JSON"},
		{Key: "record.max.count", Value: "10"},
		{Key: "codex.executable", Value: "/opt/Codex CLI/codex"},
		{Key: "codex.model", Value: ""},
		{Key: "codex.timeout-seconds", Value: "600"},
	}
	if err := service.Save(settings); err != nil {
		t.Fatalf("Save(Codex settings) error = %v", err)
	}
	if len(store.saved) != 11 {
		t.Fatalf("saved %d settings, want 11", len(store.saved))
	}
}

func TestSaveRejectsInvalidActiveProviderSettings(t *testing.T) {
	for _, test := range []struct {
		name     string
		provider string
		exec     string
		timeout  string
		want     string
	}{
		{name: "unknown provider", provider: "other", timeout: "600", want: "llm.provider"},
		{name: "relative executable", provider: "codex", exec: "codex", timeout: "600", want: "codex.executable"},
		{name: "timeout too short", provider: "codex", exec: "/usr/bin/codex", timeout: "29", want: "codex.timeout-seconds"},
		{name: "timeout too long", provider: "codex", exec: "/usr/bin/codex", timeout: "3601", want: "codex.timeout-seconds"},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := &fakeSettingStore{}
			service := newService(context.Background(), store)
			settings := []settingmodel.Setting{
				{Key: "llm.provider", Value: test.provider},
				{Key: "llm.secret", Value: ""},
				{Key: "llm.url", Value: ""},
				{Key: "llm.model", Value: ""},
				{Key: "llm.max-token", Value: "invalid"},
				{Key: "llm.auto-context-compress", Value: "invalid"},
				{Key: "llm.extra-body", Value: "not JSON"},
				{Key: "record.max.count", Value: "10"},
				{Key: "codex.executable", Value: test.exec},
				{Key: "codex.model", Value: ""},
				{Key: "codex.timeout-seconds", Value: test.timeout},
			}
			err := service.Save(settings)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Save() error = %v, want error containing %q", err, test.want)
			}
			if len(store.saved) != 0 {
				t.Fatal("invalid settings were persisted")
			}
		})
	}
}

func TestSaveRejectsUnknownAndDuplicateKeys(t *testing.T) {
	base := []settingmodel.Setting{
		{Key: "llm.provider", Value: "codex"},
		{Key: "llm.secret", Value: ""},
		{Key: "llm.url", Value: ""},
		{Key: "llm.model", Value: ""},
		{Key: "llm.max-token", Value: "inactive"},
		{Key: "llm.auto-context-compress", Value: "inactive"},
		{Key: "llm.extra-body", Value: "not JSON"},
		{Key: "record.max.count", Value: "10"},
		{Key: "codex.executable", Value: ""},
		{Key: "codex.model", Value: ""},
		{Key: "codex.timeout-seconds", Value: "600"},
	}
	for _, test := range []struct {
		name   string
		change func([]settingmodel.Setting)
	}{
		{name: "unknown", change: func(settings []settingmodel.Setting) { settings[len(settings)-1].Key = "codex.unknown" }},
		{name: "duplicate", change: func(settings []settingmodel.Setting) { settings[len(settings)-1].Key = "codex.model" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			settings := append([]settingmodel.Setting(nil), base...)
			test.change(settings)
			store := &fakeSettingStore{}
			if err := newService(context.Background(), store).Save(settings); err == nil {
				t.Fatal("Save() error = nil")
			}
			if len(store.saved) != 0 {
				t.Fatal("invalid settings were persisted")
			}
		})
	}
}
