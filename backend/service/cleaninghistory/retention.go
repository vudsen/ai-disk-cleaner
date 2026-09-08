package cleaninghistory

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"ai-disk-cleanner/backend/data/models/cleaningrecord"
	settingmodel "ai-disk-cleanner/backend/data/models/setting"
)

const recordMaxCountKey = "record.max.count"

type store interface {
	ListSettings(context.Context) ([]settingmodel.Setting, error)
	MarkInterruptedCleaningRecords(context.Context) error
	ListOldCleaningRecords(context.Context, int) ([]cleaningrecord.CleaningRecord, error)
	DeleteCleaningRecordsByIDs(context.Context, []int64) error
}

func (service *Service) recordMaxCount() (int, error) {
	settings, err := service.store.ListSettings(service.ctx)
	if err != nil {
		return 0, fmt.Errorf("load cleaning history limit: %w", err)
	}
	for _, item := range settings {
		if item.Key != recordMaxCountKey {
			continue
		}
		maxCount, err := strconv.Atoi(strings.TrimSpace(item.Value))
		if err != nil || maxCount <= 0 {
			return 0, fmt.Errorf("%s must be a positive integer", recordMaxCountKey)
		}
		return maxCount, nil
	}
	return 0, fmt.Errorf("setting %s not found", recordMaxCountKey)
}
