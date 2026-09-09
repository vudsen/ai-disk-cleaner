package cleaninghistory

import (
	"ai-disk-cleanner/backend/service/tasklog"
	"context"
	"log"
	"time"

	appctx "ai-disk-cleanner/backend/ctx"
)

// Service performs startup maintenance for persisted cleaning history.
type Service struct {
	ctx       context.Context
	store     store
	removeLog func(time.Time) error
}

// NewService creates the cleaning history service for the central service manager.
func NewService(store store, logs *tasklog.Service) *Service {
	service := newService(appctx.GetContext(), store)
	service.removeLog = logs.Remove
	return service
}

func newService(ctx context.Context, store store) *Service {
	return &Service{ctx: ctx, store: store}
}

// CleanupOnStartup repairs interrupted tasks and enforces the configured history limit.
func (service *Service) CleanupOnStartup() error {
	if err := service.store.MarkInterruptedCleaningRecords(service.ctx); err != nil {
		return err
	}
	maxCount, err := service.recordMaxCount()
	if err != nil {
		return err
	}
	records, err := service.store.ListOldCleaningRecords(service.ctx, maxCount)
	if err != nil {
		return err
	}
	ids := make([]int64, 0, len(records))
	for _, record := range records {
		if err := service.removeLog(record.StartTime); err != nil {
			log.Printf("cleaning history: skip record %d: remove task log: %v", record.ID, err)
			continue
		}
		ids = append(ids, record.ID)
	}
	return service.store.DeleteCleaningRecordsByIDs(service.ctx, ids)
}
