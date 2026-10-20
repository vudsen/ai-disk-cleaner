package data

import (
	"ai-disk-cleanner/backend/data/models/cleaningrecord"
	"ai-disk-cleanner/backend/data/models/scansnapshot"
	"context"
	"gorm.io/gorm"
)

// DeleteCleaningRecordsByIDs removes records and their snapshots atomically.
func (store *Store) DeleteCleaningRecordsByIDs(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	return store.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&scansnapshot.Snapshot{}, "record_id IN ?", ids).Error; err != nil {
			return err
		}
		return tx.Delete(&cleaningrecord.CleaningRecord{}, "id IN ?", ids).Error
	})
}
