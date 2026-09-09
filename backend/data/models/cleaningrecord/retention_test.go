package cleaningrecord

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	gormsqlite "gorm.io/driver/sqlite"
	"gorm.io/gorm"
	_ "modernc.org/sqlite"
)

func TestRetentionTransactionRollbackAndRetry(t *testing.T) {
	db, err := gorm.Open(gormsqlite.Dialector{DriverName: "sqlite", DSN: filepath.Join(t.TempDir(), "test.db")}, &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	store := NewStore(db)
	t.Cleanup(func() { _ = store.Close() })
	if err := db.AutoMigrate(&CleaningRecord{}); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for id := int64(1); id <= 3; id++ {
		record := &CleaningRecord{ID: id, StartTime: time.Unix(id, 0), TrashFiles: []TrashFile{}, TopUsages: []DiskUsage{}, State: CLEANING_STATE_DONE}
		if err := store.CreateCleaningRecord(ctx, record); err != nil {
			t.Fatal(err)
		}
	}
	candidates, err := store.ListOldCleaningRecords(ctx, 1)
	if err != nil || len(candidates) != 2 || candidates[0].ID != 2 || candidates[1].ID != 1 {
		t.Fatalf("candidates=%v error=%v", candidates, err)
	}
	if candidates[0].StartTime.Unix() != 2 {
		t.Fatal("StartTime missing")
	}
	if err := db.Exec(`CREATE TRIGGER reject_delete BEFORE DELETE ON cleaning_record WHEN OLD.id=2 BEGIN SELECT RAISE(ABORT, 'test failure'); END`).Error; err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteCleaningRecordsByIDs(ctx, []int64{1, 2}); err == nil {
		t.Fatal("expected rollback")
	}
	records, err := store.ListCleaningRecords(ctx)
	if err != nil || len(records) != 3 {
		t.Fatalf("records=%v error=%v", records, err)
	}
	if err := db.Exec(`DROP TRIGGER reject_delete`).Error; err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteCleaningRecordsByIDs(ctx, []int64{1, 2}); err != nil {
		t.Fatal(err)
	}
	records, err = store.ListCleaningRecords(ctx)
	if err != nil || len(records) != 1 || records[0].ID != 3 {
		t.Fatalf("records=%v error=%v", records, err)
	}
	if _, err := store.ListOldCleaningRecords(ctx, -1); err == nil {
		t.Fatal("negative count accepted")
	}
	if err := store.DeleteCleaningRecordsByIDs(ctx, nil); err != nil {
		t.Fatal(err)
	}
}
