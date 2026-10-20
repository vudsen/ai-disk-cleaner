package data

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"ai-disk-cleanner/backend/data/models/cleaningrecord"
	"ai-disk-cleanner/backend/data/models/migration"
	"ai-disk-cleanner/backend/data/models/setting"
	modelscanner "ai-disk-cleanner/backend/model/scanner"
)

func TestGetStoreReturnsSingleton(t *testing.T) {
	configDirectory := t.TempDir()
	t.Setenv("APPDATA", configDirectory)
	t.Setenv("XDG_CONFIG_HOME", configDirectory)
	t.Setenv("XDG_DATA_HOME", configDirectory)
	t.Setenv("XDG_STATE_HOME", configDirectory)

	first, err := GetStore()
	if err != nil {
		t.Fatalf("GetStore() error = %v", err)
	}
	t.Cleanup(func() {
		if err := first.Close(); err != nil {
			t.Errorf("close() error = %v", err)
		}
	})

	second, err := GetStore()
	if err != nil {
		t.Fatalf("second GetStore() error = %v", err)
	}
	if first != second {
		t.Fatal("GetStore() returned different Store instances")
	}
}

func TestOpenDefaultSQLiteUsesSameDatabaseAcrossWorkingDirectories(t *testing.T) {
	root := t.TempDir()
	dataHome := filepath.Join(root, "xdg-data")
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("HOME", filepath.Join(root, "home"))
	firstDirectory := filepath.Join(root, "first")
	secondDirectory := filepath.Join(root, "second")
	if err := os.MkdirAll(firstDirectory, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(secondDirectory, 0700); err != nil {
		t.Fatal(err)
	}

	t.Chdir(firstDirectory)
	first, err := openDefaultSQLite()
	if err != nil {
		t.Fatalf("openDefaultSQLite() in first directory: %v", err)
	}
	if err := first.SaveSettings(context.Background(), []setting.Setting{{Key: "test.cwd", Value: "same-database"}}); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	t.Chdir(secondDirectory)
	second, err := openDefaultSQLite()
	if err != nil {
		t.Fatalf("openDefaultSQLite() in second directory: %v", err)
	}
	t.Cleanup(func() {
		if err := second.Close(); err != nil {
			t.Errorf("close() error = %v", err)
		}
	})
	settings, err := second.ListSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range settings {
		if item.Key == "test.cwd" && item.Value == "same-database" {
			return
		}
	}
	t.Fatal("second working directory opened a different database")
}

func TestCleaningRecordRoundTrip(t *testing.T) {
	store, err := openSQLite(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("openSQLite() error = %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("close() error = %v", err)
		}
	})

	start := time.Now().Truncate(time.Second)
	record := &cleaningrecord.CleaningRecord{
		StartTime: start,
		FreedSize: 1024,
		TrashSize: 2048,
		TrashFiles: []cleaningrecord.TrashFile{{
			Name:      "cache",
			Reason:    "temporary data",
			Path:      "C:/cache",
			Level:     cleaningrecord.LOW,
			IsDeleted: true,
		}},
		TopUsages: []cleaningrecord.DiskUsage{{
			Path:        "C:/data",
			Size:        4096,
			Description: "application data",
		}},
		Path: "C:/",
	}

	beforeCreate := time.Now().UnixMilli()
	if err := store.CreateCleaningRecord(context.Background(), record); err != nil {
		t.Fatalf("CreateCleaningRecord() error = %v", err)
	}
	if record.ID < beforeCreate || record.ID > time.Now().UnixMilli() {
		t.Fatalf("ID = %d, want a current Unix millisecond timestamp", record.ID)
	}

	got, err := store.GetCleaningRecord(context.Background(), record.ID)
	if err != nil {
		t.Fatalf("GetCleaningRecord() error = %v", err)
	}
	if !got.StartTime.Equal(start) {
		t.Errorf("StartTime = %v, want %v", got.StartTime, start)
	}
	if !reflect.DeepEqual(got.TrashFiles, record.TrashFiles) {
		t.Errorf("TrashFiles = %#v, want %#v", got.TrashFiles, record.TrashFiles)
	}
	if !reflect.DeepEqual(got.TopUsages, record.TopUsages) {
		t.Errorf("TopUsages = %#v, want %#v", got.TopUsages, record.TopUsages)
	}
}

func TestCleaningRecordScanSummaryRoundTripAndCompletionPreservesIt(t *testing.T) {
	store, err := openSQLite(filepath.Join(t.TempDir(), "summary.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	record := &cleaningrecord.CleaningRecord{
		StartTime:  time.Now(),
		TrashFiles: []cleaningrecord.TrashFile{},
		TopUsages:  []cleaningrecord.DiskUsage{},
		Path:       "/tmp",
		State:      cleaningrecord.CLEANING_STATE_SCANNING,
	}
	if err := store.CreateCleaningRecord(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	summary := modelscanner.ScanSummary{
		ExcludedCount:  4,
		ReadErrorCount: 2,
		Warnings:       []modelscanner.ScanWarning{{Path: "/tmp/blocked", Code: "read_error"}},
	}
	if err := store.SaveCleaningRecordScanSummary(context.Background(), record.ID, summary); err != nil {
		t.Fatal(err)
	}
	if err := store.CompleteCleaningRecord(context.Background(), record.ID, &cleaningrecord.AnalysisResult{}); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetCleaningRecord(context.Background(), record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ScanSummary == nil || got.ScanSummary.ExcludedCount != summary.ExcludedCount ||
		got.ScanSummary.ReadErrorCount != summary.ReadErrorCount || len(got.ScanSummary.Warnings) != 1 ||
		got.ScanSummary.Warnings[0] != summary.Warnings[0] {
		t.Fatalf("scan summary after completion = %#v", got.ScanSummary)
	}
}

func TestOpeningDatabaseAddsNullableScanSummaryColumn(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "legacy.db")
	store, err := openSQLite(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	record := &cleaningrecord.CleaningRecord{StartTime: time.Now(), TrashFiles: []cleaningrecord.TrashFile{}, TopUsages: []cleaningrecord.DiskUsage{}, Path: "/tmp"}
	if err := store.CreateCleaningRecord(context.Background(), record); err != nil {
		_ = store.Close()
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	database, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec("ALTER TABLE cleaning_record DROP COLUMN scan_summary"); err != nil {
		_ = database.Close()
		t.Skipf("SQLite driver does not support DROP COLUMN for legacy schema fixture: %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	upgraded, err := openSQLite(databasePath)
	if err != nil {
		t.Fatalf("openSQLite() legacy database error = %v", err)
	}
	t.Cleanup(func() { _ = upgraded.Close() })
	got, err := upgraded.GetCleaningRecord(context.Background(), record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ScanSummary != nil {
		t.Fatalf("legacy scan summary = %#v, want nil", got.ScanSummary)
	}
}

func TestCleaningRecordLifecycleUpdates(t *testing.T) {
	store, err := openSQLite(filepath.Join(t.TempDir(), "lifecycle.db"))
	if err != nil {
		t.Fatalf("openSQLite() error = %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("close() error = %v", err)
		}
	})

	record := &cleaningrecord.CleaningRecord{
		StartTime:  time.Now(),
		TrashFiles: []cleaningrecord.TrashFile{},
		TopUsages:  []cleaningrecord.DiskUsage{},
		Path:       "C:/",
		State:      cleaningrecord.CLEANING_STATE_SCANNING,
	}
	if err := store.CreateCleaningRecord(context.Background(), record); err != nil {
		t.Fatalf("CreateCleaningRecord() error = %v", err)
	}
	if err := store.UpdateCleaningRecordState(
		context.Background(),
		record.ID,
		cleaningrecord.CLEANING_STATE_ANALYZING,
		"",
	); err != nil {
		t.Fatalf("UpdateCleaningRecordState() error = %v", err)
	}

	analysis := &cleaningrecord.AnalysisResult{
		TrashFiles: []cleaningrecord.TrashFile{{Name: "cache", Path: "C:/cache"}},
		TopUsages:  []cleaningrecord.DiskUsage{{Path: "C:/data", Size: 4096}},
		LLMOutput:  "done",
		TokenUsage: 42,
	}
	if err := store.CompleteCleaningRecord(context.Background(), record.ID, analysis); err != nil {
		t.Fatalf("CompleteCleaningRecord() error = %v", err)
	}
	completed, err := store.GetCleaningRecord(context.Background(), record.ID)
	if err != nil {
		t.Fatalf("GetCleaningRecord() error = %v", err)
	}
	if completed.State != cleaningrecord.CLEANING_STATE_DONE ||
		completed.LLMOutput != "done" ||
		completed.TokenUsage != 42 ||
		!reflect.DeepEqual(completed.TrashFiles, analysis.TrashFiles) {
		t.Fatalf("completed record = %#v", completed)
	}

	interrupted := &cleaningrecord.CleaningRecord{
		StartTime:  time.Now(),
		TrashFiles: []cleaningrecord.TrashFile{},
		TopUsages:  []cleaningrecord.DiskUsage{},
		Path:       "D:/",
		State:      cleaningrecord.CLEANING_STATE_SCANNING,
	}
	if err := store.CreateCleaningRecord(context.Background(), interrupted); err != nil {
		t.Fatalf("create interrupted record: %v", err)
	}
	if err := store.MarkInterruptedCleaningRecords(context.Background()); err != nil {
		t.Fatalf("MarkInterruptedCleaningRecords() error = %v", err)
	}
	gotInterrupted, err := store.GetCleaningRecord(context.Background(), interrupted.ID)
	if err != nil {
		t.Fatalf("get interrupted record: %v", err)
	}
	if gotInterrupted.State != cleaningrecord.CLEANING_STATE_ERROR || gotInterrupted.ErrorMessage == "" {
		t.Fatalf("interrupted record = %#v", gotInterrupted)
	}
}

func TestMarkTrashFileMigrated(t *testing.T) {
	store, err := openSQLite(filepath.Join(t.TempDir(), "migrated-trash-file.db"))
	if err != nil {
		t.Fatalf("openSQLite() error = %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("close() error = %v", err)
		}
	})

	root := t.TempDir()
	record := &cleaningrecord.CleaningRecord{
		StartTime: time.Now(),
		Path:      root,
		State:     cleaningrecord.CLEANING_STATE_DONE,
		TrashFiles: []cleaningrecord.TrashFile{
			{Name: "cache", Path: "cache", Size: 128},
			{Name: "other", Path: "other", Size: 256},
		},
		TopUsages: []cleaningrecord.DiskUsage{},
	}
	if err := store.CreateCleaningRecord(context.Background(), record); err != nil {
		t.Fatalf("CreateCleaningRecord() error = %v", err)
	}
	if err := store.MarkTrashFileMigrated(
		context.Background(),
		filepath.Join(root, "cache"),
	); err != nil {
		t.Fatalf("MarkTrashFileMigrated() error = %v", err)
	}

	updated, err := store.GetCleaningRecord(context.Background(), record.ID)
	if err != nil {
		t.Fatalf("GetCleaningRecord() error = %v", err)
	}
	if !updated.TrashFiles[0].IsDeleted {
		t.Fatal("migrated trash file was not marked deleted")
	}
	if updated.TrashFiles[1].IsDeleted {
		t.Fatal("unrelated trash file was marked deleted")
	}
}

func TestMigrationRoundTrip(t *testing.T) {
	store, err := openSQLite(filepath.Join(t.TempDir(), "migration.db"))
	if err != nil {
		t.Fatalf("openSQLite() error = %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("close() error = %v", err)
		}
	})

	record := &migration.Migration{
		Name:   "cache",
		Source: "C:/source/cache",
		Dest:   "D:/migrations/cache",
	}
	beforeCreate := time.Now().UnixMilli()
	if err := store.CreateMigration(context.Background(), record); err != nil {
		t.Fatalf("CreateMigration() error = %v", err)
	}
	if record.ID < beforeCreate || record.CreatedAt.IsZero() {
		t.Fatalf("migration timestamps were not assigned: %#v", record)
	}

	records, err := store.ListMigrations(context.Background())
	if err != nil {
		t.Fatalf("ListMigrations() error = %v", err)
	}
	if len(records) != 1 || records[0].Source != record.Source || records[0].Dest != record.Dest {
		t.Fatalf("ListMigrations() = %#v", records)
	}
	if err := store.DeleteMigration(context.Background(), record.ID); err != nil {
		t.Fatalf("DeleteMigration() error = %v", err)
	}
	records, err = store.ListMigrations(context.Background())
	if err != nil || len(records) != 0 {
		t.Fatalf("ListMigrations() after delete = %#v, error = %v", records, err)
	}
}

func TestSettingDefaultsAndSave(t *testing.T) {
	store, err := openSQLite(filepath.Join(t.TempDir(), "settings.db"))
	if err != nil {
		t.Fatalf("openSQLite() error = %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("close() error = %v", err)
		}
	})

	settings, err := store.ListSettings(context.Background())
	if err != nil {
		t.Fatalf("ListSettings() error = %v", err)
	}
	got := make(map[string]string, len(settings))
	for _, item := range settings {
		got[item.Key] = item.Value
	}
	if len(got) != 11 || got["llm.provider"] != "api" || got["codex.executable"] != "" || got["codex.model"] != "" || got["codex.timeout-seconds"] != "600" || got["llm.max-token"] != "620000" || got["llm.auto-context-compress"] != "false" || got["llm.extra-body"] != "" || got["record.max.count"] != "10" {
		t.Fatalf("default settings = %#v", got)
	}

	updated := []setting.Setting{
		{Key: "llm.secret", Value: "secret"},
		{Key: "llm.url", Value: "https://example.com/v1"},
		{Key: "llm.model", Value: "model"},
		{Key: "llm.max-token", Value: "64000"},
		{Key: "llm.extra-body", Value: `{"temperature":0.7}`},
		{Key: "record.max.count", Value: "20"},
	}
	if err := store.SaveSettings(context.Background(), updated); err != nil {
		t.Fatalf("SaveSettings() error = %v", err)
	}
	providerSettings := []setting.Setting{
		{Key: "llm.provider", Value: "codex"},
		{Key: "codex.executable", Value: "/custom/codex"},
		{Key: "codex.model", Value: "custom-model"},
		{Key: "codex.timeout-seconds", Value: "1800"},
	}
	if err := store.SaveSettings(context.Background(), providerSettings); err != nil {
		t.Fatalf("SaveSettings() provider settings error = %v", err)
	}
	if err := store.SaveSettings(context.Background(), []setting.Setting{
		{Key: "llm.key", Value: "obsolete"},
	}); err != nil {
		t.Fatalf("save obsolete llm.key: %v", err)
	}
	if err := store.EnsureDefaultSettings(context.Background()); err != nil {
		t.Fatalf("EnsureDefaultSettings() error = %v", err)
	}

	settings, err = store.ListSettings(context.Background())
	if err != nil {
		t.Fatalf("ListSettings() after save error = %v", err)
	}
	got = make(map[string]string, len(settings))
	for _, item := range settings {
		got[item.Key] = item.Value
	}
	if len(got) != 11 {
		t.Fatalf("settings after removing llm.key = %#v", got)
	}
	for _, want := range updated {
		if got[want.Key] != want.Value {
			t.Errorf("%s = %q, want %q", want.Key, got[want.Key], want.Value)
		}
	}
	for _, want := range providerSettings {
		if got[want.Key] != want.Value {
			t.Errorf("%s = %q, want %q", want.Key, got[want.Key], want.Value)
		}
	}
}

func TestDeleteOldCleaningRecordsKeepsNewestByStartTime(t *testing.T) {
	store, err := openSQLite(filepath.Join(t.TempDir(), "record-retention.db"))
	if err != nil {
		t.Fatalf("openSQLite() error = %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("close() error = %v", err)
		}
	})

	baseTime := time.Now().Add(-time.Hour)
	for index, offset := range []time.Duration{time.Minute, 3 * time.Minute, 2 * time.Minute, 4 * time.Minute} {
		record := &cleaningrecord.CleaningRecord{
			ID:         int64(index + 1),
			StartTime:  baseTime.Add(offset),
			TrashFiles: []cleaningrecord.TrashFile{},
			TopUsages:  []cleaningrecord.DiskUsage{},
			Path:       "C:/",
			State:      cleaningrecord.CLEANING_STATE_DONE,
		}
		if err := store.CreateCleaningRecord(context.Background(), record); err != nil {
			t.Fatalf("CreateCleaningRecord() error = %v", err)
		}
	}

	candidates, err := store.ListOldCleaningRecords(context.Background(), 2)
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]int64, 0, len(candidates))
	for _, record := range candidates {
		ids = append(ids, record.ID)
	}
	if err := store.DeleteCleaningRecordsByIDs(context.Background(), ids); err != nil {
		t.Fatal(err)
	}
	records, err := store.ListCleaningRecords(context.Background())
	if err != nil {
		t.Fatalf("ListCleaningRecords() error = %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("records count = %d, want 2", len(records))
	}
	remaining := map[int64]bool{}
	for _, record := range records {
		remaining[record.ID] = true
	}
	if !remaining[2] || !remaining[4] {
		t.Fatalf("remaining record IDs = %#v, want 2 and 4", remaining)
	}
}
