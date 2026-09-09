package cleaningrecord

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"gorm.io/gorm"
)

// TrashFileLevel describes the risk of a cleanup candidate.
type TrashFileLevel int

const (
	LOW TrashFileLevel = iota
	MEDIUM
	HIGH
)

// TrashFile is one file or directory suggested by the LLM.
type TrashFile struct {
	Name      string         `json:"name" jsonschema_description:"删除说明标题"`
	Reason    string         `json:"reason" jsonschema_description:"你的建议或者原因，为什么添加这个文件或文件夹"`
	Path      string         `json:"path" jsonschema_description:"文件或目录路径, 允许使用 glob 表达式，例如 /foo/*.log"`
	Size      int64          `json:"size" jsonschema_description:"候选文件或目录占用的总字节数"`
	Level     TrashFileLevel `json:"level" jsonschema:"enum=0,enum=1,enum=2" jsonschema_description:"风险等级, 0: A类, 1: B类, 2: C类"`
	IsDeleted bool           `json:"isDeleted" jsonschema_description:"文件或目录是否已删除"`
}

// DiskUsage describes a notable disk usage location.
type DiskUsage struct {
	Path        string `json:"path" jsonschema_description:"该建议指向的文件或目录路径"`
	Size        int64  `json:"size" jsonschema_description:"文件大小"`
	Description string `json:"description" jsonschema_description:"该目录用途"`
}

// AnalysisResult is the persistable result of one LLM analysis.
type AnalysisResult struct {
	TrashFiles []TrashFile `json:"trashFiles"`
	TopUsages  []DiskUsage `json:"topUsages"`
	LLMOutput  string      `json:"llmOutput"`
	TokenUsage int64       `json:"tokenUsage"`
}

// Store exposes cleaning-record persistence over the initialized GORM database.
type Store struct {
	db *gorm.DB
}

func NewStore(db *gorm.DB) *Store {
	return &Store{db: db}
}

func (store *Store) Close() error {
	if store == nil || store.db == nil {
		return nil
	}
	sqlDB, err := store.db.DB()
	if err != nil {
		return fmt.Errorf("get sqlite connection: %w", err)
	}
	if err := sqlDB.Close(); err != nil {
		return fmt.Errorf("close sqlite database: %w", err)
	}
	return nil
}

const (
	// CLEANING_STATE_SCANNING 正在扫描目录大小
	CLEANING_STATE_SCANNING = "SCANNING"
	// CLEANING_STATE_ANALYZING 正在调用 LLM 分析
	CLEANING_STATE_ANALYZING = "ANALYZING"
	// 失败
	CLEANING_STATE_ERROR = "ERROR"
	CLEANING_STATE_DONE  = "DONE"
	// CLEANING_STATE_CANCELLED was stopped explicitly by the user.
	CLEANING_STATE_CANCELLED = "CANCELLED"
)

// CleaningRecord records the result of a disk scan and cleanup.
type CleaningRecord struct {
	ID           int64       `gorm:"column:id;primaryKey;autoIncrement:false" json:"id"`
	StartTime    time.Time   `gorm:"column:start_time;not null" json:"startTime"`
	FreedSize    int64       `gorm:"column:freed_size;not null" json:"freedSize"`
	TrashSize    int64       `gorm:"column:trash_size;not null" json:"trashSize"`
	TrashFiles   []TrashFile `gorm:"column:trash_files;type:text;serializer:json;not null" json:"trashFiles"`
	TopUsages    []DiskUsage `gorm:"column:top_usages;type:text;serializer:json;not null" json:"topUsages"`
	Path         string      `gorm:"column:path;not null" json:"path"`
	LLMOutput    string      `gorm:"column:llm_output;not null" json:"llmOutput"`
	TokenUsage   int64       `gorm:"column:token_usage;not null" json:"tokenUsage"`
	State        string      `gorm:"column:state;not null" json:"state"`
	ErrorMessage string      `gorm:"column:error_message;not null;default:''" json:"errorMessage"`
}

// TableName keeps the table name singular as required by the database schema.
func (CleaningRecord) TableName() string {
	return "cleaning_record"
}

// BeforeCreate assigns a Unix timestamp in seconds when an ID was not supplied.
func (record *CleaningRecord) BeforeCreate(_ *gorm.DB) error {
	if record.ID == 0 {
		record.ID = time.Now().UnixMilli()
	}
	return nil
}

// UpdateCleaningRecordState updates a record while a task moves through its lifecycle.
func (store *Store) UpdateCleaningRecordState(
	ctx context.Context,
	id int64,
	state string,
	errorMessage string,
) error {
	result := store.db.WithContext(ctx).Model(&CleaningRecord{}).
		Where("id = ?", id).
		Updates(map[string]any{"state": state, "error_message": errorMessage})
	if result.Error != nil {
		return fmt.Errorf("update cleaning record %d state: %w", id, result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("update cleaning record %d state: %w", id, gorm.ErrRecordNotFound)
	}
	return nil
}

// CompleteCleaningRecord stores the final LLM result and marks the task done.
func (store *Store) CompleteCleaningRecord(
	ctx context.Context,
	id int64,
	result *AnalysisResult,
) error {
	if result == nil {
		return fmt.Errorf("complete cleaning record %d: result is nil", id)
	}
	update := &CleaningRecord{
		TrashFiles:   result.TrashFiles,
		TopUsages:    result.TopUsages,
		LLMOutput:    result.LLMOutput,
		TokenUsage:   result.TokenUsage,
		State:        CLEANING_STATE_DONE,
		ErrorMessage: "",
	}
	dbResult := store.db.WithContext(ctx).Model(&CleaningRecord{}).
		Where("id = ?", id).
		Select(
			"TrashFiles",
			"TopUsages",
			"LLMOutput",
			"TokenUsage",
			"State",
			"ErrorMessage",
		).
		Updates(update)
	if dbResult.Error != nil {
		return fmt.Errorf("complete cleaning record %d: %w", id, dbResult.Error)
	}
	if dbResult.RowsAffected == 0 {
		return fmt.Errorf("complete cleaning record %d: %w", id, gorm.ErrRecordNotFound)
	}
	return nil
}

// MarkInterruptedCleaningRecords marks tasks left active by an earlier process as failed.
func (store *Store) MarkInterruptedCleaningRecords(ctx context.Context) error {
	result := store.db.WithContext(ctx).Model(&CleaningRecord{}).
		Where("state IN ?", []string{CLEANING_STATE_SCANNING, CLEANING_STATE_ANALYZING}).
		Updates(map[string]any{
			"state":         CLEANING_STATE_ERROR,
			"error_message": "应用已重启，任务被中断",
		})
	if result.Error != nil {
		return fmt.Errorf("mark interrupted cleaning records: %w", result.Error)
	}
	return nil
}

// CreateCleaningRecord persists a cleaning record.
func (store *Store) CreateCleaningRecord(ctx context.Context, record *CleaningRecord) error {
	if err := store.db.WithContext(ctx).Create(record).Error; err != nil {
		return fmt.Errorf("create cleaning record: %w", err)
	}
	return nil
}

// GetCleaningRecord returns one cleaning record by ID.
func (store *Store) GetCleaningRecord(ctx context.Context, id int64) (*CleaningRecord, error) {
	var record CleaningRecord
	if err := store.db.WithContext(ctx).First(&record, "id = ?", id).Error; err != nil {
		return nil, fmt.Errorf("get cleaning record %d: %w", id, err)
	}
	return &record, nil
}

func (store *Store) SaveDeletedTrashFiles(
	ctx context.Context,
	record *CleaningRecord,
) error {
	result := store.db.WithContext(ctx).Model(&CleaningRecord{}).
		Where("id = ?", record.ID).
		Select("TrashFiles", "FreedSize").
		Updates(record)
	if result.Error != nil {
		return fmt.Errorf("save deleted trash files for record %d: %w", record.ID, result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("save deleted trash files for record %d: %w", record.ID, gorm.ErrRecordNotFound)
	}
	return nil
}

// MarkTrashFileMigrated marks every cleanup candidate that resolves to source
// as deleted. TrashFiles is JSON-serialized, so matching records are updated
// inside one transaction.
func (store *Store) MarkTrashFileMigrated(ctx context.Context, source string) error {
	if strings.TrimSpace(source) == "" {
		return fmt.Errorf("resolve migrated trash file: source path is empty")
	}
	source, err := filepath.Abs(filepath.Clean(strings.TrimSpace(source)))
	if err != nil {
		return fmt.Errorf("resolve migrated trash file: %w", err)
	}

	return store.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var records []CleaningRecord
		if err := tx.Find(&records).Error; err != nil {
			return fmt.Errorf("list cleaning records for migrated trash file: %w", err)
		}
		for recordIndex := range records {
			record := &records[recordIndex]
			changed := false
			for fileIndex := range record.TrashFiles {
				file := &record.TrashFiles[fileIndex]
				if file.IsDeleted || !trashFileMatchesSource(record.Path, file.Path, source) {
					continue
				}
				file.IsDeleted = true
				changed = true
			}
			if !changed {
				continue
			}
			result := tx.Model(&CleaningRecord{}).
				Where("id = ?", record.ID).
				Select("TrashFiles").
				Updates(record)
			if result.Error != nil {
				return fmt.Errorf("mark migrated trash files for record %d: %w", record.ID, result.Error)
			}
			if result.RowsAffected == 0 {
				return fmt.Errorf("mark migrated trash files for record %d: %w", record.ID, gorm.ErrRecordNotFound)
			}
		}
		return nil
	})
}

func trashFileMatchesSource(rootPath string, candidatePath string, source string) bool {
	target := candidatePath
	if !filepath.IsAbs(target) {
		target = filepath.Join(rootPath, target)
	}
	target, err := filepath.Abs(filepath.Clean(target))
	return err == nil && strings.EqualFold(target, source)
}

// ListCleaningRecords returns cleaning records ordered from newest to oldest.
func (store *Store) ListCleaningRecords(ctx context.Context) ([]CleaningRecord, error) {
	var records []CleaningRecord
	if err := store.db.WithContext(ctx).Order("id DESC").Find(&records).Error; err != nil {
		return nil, fmt.Errorf("list cleaning records: %w", err)
	}
	return records, nil
}

// ListOldCleaningRecords selects candidates without deleting them.
func (store *Store) ListOldCleaningRecords(ctx context.Context, maxCount int) ([]CleaningRecord, error) {
	if maxCount < 0 {
		return nil, fmt.Errorf("list old cleaning records: max count must not be negative")
	}
	var records []CleaningRecord
	err := store.db.WithContext(ctx).Select("id", "start_time").
		Order("start_time DESC").Order("id DESC").Offset(maxCount).Find(&records).Error
	if err != nil {
		return nil, fmt.Errorf("list old cleaning records: %w", err)
	}
	return records, nil
}

// DeleteCleaningRecordsByIDs deletes only candidates whose logs were handled.
func (store *Store) DeleteCleaningRecordsByIDs(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	return store.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&CleaningRecord{}, "id IN ?", ids).Error; err != nil {
			return fmt.Errorf("delete cleaning records: %w", err)
		}
		return nil
	})
}
