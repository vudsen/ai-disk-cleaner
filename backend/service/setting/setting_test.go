package setting

import (
	"context"
	"testing"

	settingmodel "ai-disk-cleanner/backend/data/models/setting"
)

type fakeSettingStore struct {
	saved []settingmodel.Setting
}

func (store *fakeSettingStore) ListSettings(context.Context) ([]settingmodel.Setting, error) {
	return nil, nil
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
	if len(store.saved) != len(settings) {
		t.Fatalf("saved %d settings, want %d", len(store.saved), len(settings))
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
