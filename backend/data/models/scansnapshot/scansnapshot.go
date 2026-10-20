package scansnapshot

import (
	modelscanner "ai-disk-cleanner/backend/model/scanner"
	"context"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Snapshot holds a compressed tree separately from lightweight history records.
type Snapshot struct {
	RecordID  int64 `gorm:"primaryKey;autoIncrement:false"`
	RootPath  string
	ItemCount int64
	DiskSize  int64
	Payload   []byte `json:"-"`
}

func (Snapshot) TableName() string { return "scan_snapshot" }

type SnapshotStore struct{ db *gorm.DB }

func NewStore(db *gorm.DB) *SnapshotStore { return &SnapshotStore{db: db} }

func (store *SnapshotStore) SaveScanTree(ctx context.Context, id int64, tree *modelscanner.FileTree) error {
	payload, err := encodeTree(ctx, tree)
	if err != nil {
		return fmt.Errorf("encode scan snapshot: %w", err)
	}
	snapshot := Snapshot{RecordID: id, RootPath: tree.RootPath, ItemCount: tree.Root.ItemCount, DiskSize: tree.Root.DiskSize, Payload: payload}
	if err := store.db.WithContext(ctx).Clauses(clause.OnConflict{UpdateAll: true}).Create(&snapshot).Error; err != nil {
		return fmt.Errorf("save scan snapshot: %w", err)
	}
	return nil
}
func (store *SnapshotStore) LoadScanTree(ctx context.Context, id int64) (*modelscanner.FileTree, error) {
	var snapshot Snapshot
	if err := store.db.WithContext(ctx).First(&snapshot, "record_id = ?", id).Error; err != nil {
		return nil, fmt.Errorf("load scan snapshot: %w", err)
	}
	tree, err := decodeTree(ctx, snapshot.Payload)
	if err != nil {
		return nil, fmt.Errorf("restore scan snapshot: %w", err)
	}
	if tree.RootPath != snapshot.RootPath || tree.Root.ItemCount != snapshot.ItemCount || tree.Root.DiskSize != snapshot.DiskSize {
		return nil, fmt.Errorf("scan snapshot metadata mismatch")
	}
	return tree, nil
}

// ScanTreeProgress never loads tree payloads, including on the history page.
func (store *SnapshotStore) ScanTreeProgress(ctx context.Context, ids []int64) (map[int64]modelscanner.ScanProgress, error) {
	result := make(map[int64]modelscanner.ScanProgress)
	if len(ids) == 0 {
		return result, nil
	}
	var rows []Snapshot
	if err := store.db.WithContext(ctx).Select("record_id", "root_path", "item_count", "disk_size").Where("record_id IN ?", ids).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		result[row.RecordID] = modelscanner.ScanProgress{CurrentPath: row.RootPath, ItemCount: row.ItemCount, ScannedSize: row.DiskSize}
	}
	return result, nil
}
