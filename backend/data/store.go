package data

import (
	"context"
	"fmt"
	"sync"

	"ai-disk-cleanner/backend/data/models/cleaningrecord"
	"ai-disk-cleanner/backend/data/models/migration"
	"ai-disk-cleanner/backend/data/models/scansnapshot"
	"ai-disk-cleanner/backend/data/models/setting"
	"ai-disk-cleanner/backend/util"

	gormsqlite "gorm.io/driver/sqlite"
	"gorm.io/gorm"
	_ "modernc.org/sqlite"
)

var (
	storeOnce     sync.Once
	storeInstance *Store
	storeError    error
)

// GetStore returns the process-wide Store, initializing it on first use.
func GetStore() (*Store, error) {
	storeOnce.Do(func() {
		storeInstance, storeError = openDefaultSQLite()
	})
	return storeInstance, storeError
}

// openDefaultSQLite opens the application database in its platform data directory.
func openDefaultSQLite() (*Store, error) {
	databasePath, err := util.DataFilePath()
	if err != nil {
		return nil, fmt.Errorf("resolve application database path: %w", err)
	}
	return openSQLite(databasePath)
}

// openSQLite opens a SQLite database and creates or updates its schema.
func openSQLite(databasePath string) (*Store, error) {
	db, err := gorm.Open(gormsqlite.Dialector{
		DriverName: "sqlite",
		DSN:        databasePath,
	}, &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}

	if err := db.AutoMigrate(
		&cleaningrecord.CleaningRecord{},
		&scansnapshot.Snapshot{},
		&migration.Migration{},
		&setting.Setting{},
	); err != nil {
		closeDatabase(db)
		return nil, fmt.Errorf("migrate database: %w", err)
	}

	store := &Store{
		Store:          cleaningrecord.NewStore(db),
		SnapshotStore:  scansnapshot.NewStore(db),
		db:             db,
		MigrationStore: migration.NewStore(db),
		SettingStore:   setting.NewStore(db),
	}
	if err := store.EnsureDefaultSettings(context.Background()); err != nil {
		closeDatabase(db)
		return nil, err
	}

	return store, nil
}

// Store combines persistence operations for all data models.
type Store struct {
	db *gorm.DB
	*scansnapshot.SnapshotStore
	*cleaningrecord.Store
	*migration.MigrationStore
	*setting.SettingStore
}

func closeDatabase(db *gorm.DB) {
	sqlDB, err := db.DB()
	if err == nil {
		_ = sqlDB.Close()
	}
}
