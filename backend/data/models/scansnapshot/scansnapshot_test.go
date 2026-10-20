package scansnapshot

import (
	modelscanner "ai-disk-cleanner/backend/model/scanner"
	"context"
	"errors"
	gormsqlite "gorm.io/driver/sqlite"

	"gorm.io/gorm"
	"math"
	_ "modernc.org/sqlite"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestSnapshotSurvivesDatabaseReopenAndRestoresLookup(t *testing.T) {
	for _, root := range []string{"/data", `C:\data`, "C:/data", `\\server\share`} {
		t.Run(root, func(t *testing.T) {
			file := &modelscanner.FileNode{Name: `a\b.log`, Type: modelscanner.NodeTypeFile, DiskSize: 4096, ApparentSize: 123, Inode: math.MaxUint64, HardLink: true, ModifiedAt: time.Unix(123, 0).UTC()}
			if root != "/data" {
				file.Name = "a.log"
			}
			tree := &modelscanner.FileTree{RootPath: root, Root: &modelscanner.FileNode{Name: "data", Type: modelscanner.NodeTypeDirectory, DiskSize: 4096, ItemCount: 2, Children: []*modelscanner.FileNode{file}}, ScanSummary: modelscanner.ScanSummary{ReadErrorCount: 2, Warnings: []modelscanner.ScanWarning{{Path: "unreadable", Code: "permission"}}}}
			dbPath := filepath.Join(t.TempDir(), "snap.db")
			open := func() *gorm.DB {
				db, err := gorm.Open(gormsqlite.Dialector{DriverName: "sqlite", DSN: dbPath}, &gorm.Config{})
				if err != nil {
					t.Fatal(err)
				}
				return db
			}
			db := open()
			if err := db.AutoMigrate(&Snapshot{}); err != nil {
				t.Fatal(err)
			}
			if err := NewStore(db).SaveScanTree(context.Background(), 42, tree); err != nil {
				t.Fatal(err)
			}
			sqlDB, _ := db.DB()
			sqlDB.Close()
			db = open()
			sqlDB, _ = db.DB()
			defer sqlDB.Close()
			restored, err := NewStore(db).LoadScanTree(context.Background(), 42)
			if err != nil {
				t.Fatal(err)
			}
			found, err := restored.FindNode("/" + file.Name)
			if err != nil {
				t.Fatal(err)
			}
			if found.Inode != math.MaxUint64 || found.DiskSize != 4096 || !found.HardLink || !found.ModifiedAt.Equal(file.ModifiedAt) || !reflect.DeepEqual(restored.ScanSummary, tree.ScanSummary) {
				t.Fatalf("lossy restore: %#v", restored)
			}
			separator := "/"
			if strings.Contains(root, `\`) {
				separator = `\`
			}
			if found.FullPath != root+separator+file.Name {
				t.Fatalf("full path = %q", found.FullPath)
			}
			if found.RelativePath != file.Name || restored.Root.ChildrenByName[file.Name] != found || restored.Root.ItemCount != 2 {
				t.Fatalf("paths/maps lost: %#v", found)
			}
			metadata, err := NewStore(db).ScanTreeProgress(context.Background(), []int64{42})
			if err != nil || metadata[42].ScannedSize != 4096 {
				t.Fatalf("metadata = %#v, %v", metadata, err)
			}
			if err := db.Model(&Snapshot{}).Where("record_id = ?", 42).Update("payload", []byte("corrupt")).Error; err != nil {
				t.Fatal(err)
			}
			// Metadata remains usable even if the blob cannot be decoded.
			if _, err := NewStore(db).ScanTreeProgress(context.Background(), []int64{42}); err != nil {
				t.Fatal(err)
			}
			if _, err := NewStore(db).LoadScanTree(context.Background(), 42); err == nil {
				t.Fatal("corruption accepted")
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if _, err := NewStore(db).LoadScanTree(ctx, 42); !errors.Is(err, context.Canceled) {
				t.Fatalf("cancel=%v", err)
			}
		})
	}
}
