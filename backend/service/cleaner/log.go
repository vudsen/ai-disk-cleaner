package cleaner

import (
	"ai-disk-cleanner/backend/data/models/cleaningrecord"
	"ai-disk-cleanner/backend/service/tasklog"
	"time"
)

func (service *Service) openLog(start time.Time, path string) *tasklog.Session {
	if service.logs == nil {
		return nil
	}
	return service.logs.Open(start, path)
}

func logState(state string) string {
	switch state {
	case cleaningrecord.CLEANING_STATE_DONE:
		return "成功"
	case cleaningrecord.CLEANING_STATE_CANCELLED:
		return "取消"
	default:
		return "失败"
	}
}
