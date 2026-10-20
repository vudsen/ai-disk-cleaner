package data

import (
	"ai-disk-cleanner/backend/data/models/cleaningrecord"
	"ai-disk-cleanner/backend/data/models/scansnapshot"
	modelscanner "ai-disk-cleanner/backend/model/scanner"
	"context"
	"path/filepath"
	"testing"
)

func TestHistoryRetentionRemovesSnapshotsAtomically(t *testing.T) {
	store, err := openSQLite(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	for _, id := range []int64{1, 2} {
		if err := store.CreateCleaningRecord(ctx, &cleaningrecord.CleaningRecord{ID: id, Path: "/offline", State: "ERROR", TrashFiles: []cleaningrecord.TrashFile{}, TopUsages: []cleaningrecord.DiskUsage{}}); err != nil {
			t.Fatal(err)
		}
		if err := store.SaveScanTree(ctx, id, &modelscanner.FileTree{RootPath: "/offline", Root: &modelscanner.FileNode{Type: modelscanner.NodeTypeDirectory, ItemCount: 12}}); err != nil {
			t.Fatal(err)
		}
	}
	// A failed record deletion must roll back removal of its snapshot too.
	if err := store.db.Exec("CREATE TRIGGER reject_record_delete BEFORE DELETE ON cleaning_record BEGIN SELECT RAISE(ABORT, 'fixture'); END").Error; err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteCleaningRecordsByIDs(ctx, []int64{1}); err == nil {
		t.Fatal("trigger did not reject deletion")
	}
	if _, err := store.LoadScanTree(ctx, 1); err != nil {
		t.Fatalf("snapshot removed despite rollback: %v", err)
	}
	store.db.Exec("DROP TRIGGER reject_record_delete")
	if err := store.DeleteCleaningRecordsByIDs(ctx, []int64{1}); err != nil {
		t.Fatal(err)
	}
	var count int64
	store.db.Model(&scansnapshot.Snapshot{}).Count(&count)
	if count != 1 {
		t.Fatalf("remaining snapshots=%d", count)
	}
	if _, err := store.LoadScanTree(ctx, 2); err != nil {
		t.Fatal(err)
	}
}
