package setting

import (
	"context"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Setting stores one system configuration value.
type Setting struct {
	Key   string `gorm:"column:key;primaryKey" json:"key"`
	Value string `gorm:"column:value;not null" json:"value"`
}

// SettingStore exposes setting persistence over the initialized GORM database.
type SettingStore struct {
	db *gorm.DB
}

func NewStore(db *gorm.DB) *SettingStore {
	return &SettingStore{db: db}
}

func (Setting) TableName() string {
	return "setting"
}

var defaultSettings = []Setting{
	{Key: "llm.secret", Value: ""},
	{Key: "llm.url", Value: ""},
	{Key: "llm.model", Value: ""},
	{Key: "llm.max-token", Value: "620000"},
	{Key: "llm.auto-context-compress", Value: "false"},
	{Key: "llm.extra-body", Value: ""},
	{Key: "record.max.count", Value: "10"},
}

// EnsureDefaultSettings creates settings that have not been configured yet.
func (store *SettingStore) EnsureDefaultSettings(ctx context.Context) error {
	settings := append([]Setting(nil), defaultSettings...)
	if err := store.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&Setting{}, "key = ?", "llm.key").Error; err != nil {
			return err
		}
		return tx.Clauses(clause.OnConflict{DoNothing: true}).
			Create(&settings).Error
	}); err != nil {
		return fmt.Errorf("ensure default settings: %w", err)
	}
	return nil
}

func (store *SettingStore) ListSettings(ctx context.Context) ([]Setting, error) {
	var settings []Setting
	if err := store.db.WithContext(ctx).Order("key").Find(&settings).Error; err != nil {
		return nil, fmt.Errorf("list settings: %w", err)
	}
	return settings, nil
}

func (store *SettingStore) SaveSettings(ctx context.Context, settings []Setting) error {
	if len(settings) == 0 {
		return nil
	}
	if err := store.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "key"}},
			DoUpdates: clause.AssignmentColumns([]string{"value"}),
		}).Create(&settings).Error
	}); err != nil {
		return fmt.Errorf("save settings: %w", err)
	}
	return nil
}
