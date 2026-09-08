package cleaner

import (
	"context"
	"errors"
	"time"

	"ai-disk-cleanner/backend/data/models/cleaningrecord"
	modelscanner "ai-disk-cleanner/backend/model/scanner"
	"ai-disk-cleanner/backend/service/tasklog"
)

type activeTask struct {
	snapshot CleaningTaskSnapshot
	ctx      context.Context
	cancel   context.CancelFunc
	done     chan struct{}
	language string
	log      *tasklog.Session
	logError string
}

func (service *Service) run(task *activeTask) {
	defer task.log.Close()
	tree, err := service.scan(task.ctx, task.snapshot.Path, func(progress modelscanner.ScanProgress) {
		service.updateScanProgress(task, progress)
	})
	if err != nil {
		task.logError = "扫描阶段：" + tasklog.ErrorSummary(err)
		service.finishFromError(task, err)
		return
	}
	if err := task.ctx.Err(); err != nil {
		service.finishFromError(task, err)
		return
	}

	task.log.Event("扫描完成", "占用=%.2f GiB 字节数=%d 条目数=%d", float64(tree.Root.DiskSize)/(1<<30), tree.Root.DiskSize, tree.Root.ItemCount)
	if err := service.store.UpdateCleaningRecordState(
		service.ctx,
		task.snapshot.ID,
		cleaningrecord.CLEANING_STATE_ANALYZING,
		"",
	); err != nil {
		task.logError = "分析状态保存失败：" + tasklog.ErrorSummary(err)
		service.finish(task, cleaningrecord.CLEANING_STATE_ERROR, err.Error(), false)
		return
	}
	if err := task.ctx.Err(); err != nil {
		service.finishFromError(task, err)
		return
	}

	service.mu.Lock()
	if service.active != task {
		service.mu.Unlock()
		return
	}
	service.tree = tree
	task.snapshot.State = cleaningrecord.CLEANING_STATE_ANALYZING
	task.snapshot.ScanProgress = modelscanner.ScanProgress{
		CurrentPath: tree.RootPath,
		ItemCount:   tree.Root.ItemCount,
		ScannedSize: tree.Root.DiskSize,
	}
	snapshot := task.snapshot
	service.mu.Unlock()
	service.emit(EventTaskUpdated, snapshot)

	result, err := service.analyzer.Analyze(task.ctx, tree, task.language, func(delta string) {
		service.appendLLMDelta(task, delta)
	}, task.log)
	if err != nil {
		task.logError = "分析阶段：" + tasklog.ErrorSummary(err)
		service.finishFromError(task, err)
		return
	}
	if err := task.ctx.Err(); err != nil {
		service.finishFromError(task, err)
		return
	}
	if result == nil {
		service.finish(task, cleaningrecord.CLEANING_STATE_ERROR, "LLM 分析未返回结果", false)
		return
	}

	operationContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	err = service.store.CompleteCleaningRecord(operationContext, task.snapshot.ID, result)
	cancel()
	if err != nil {
		task.logError = "分析结果保存失败：" + tasklog.ErrorSummary(err)
		service.finish(task, cleaningrecord.CLEANING_STATE_ERROR, err.Error(), false)
		return
	}

	service.mu.Lock()
	if service.active == task {
		task.snapshot.LLMOutput = result.LLMOutput
	}
	service.mu.Unlock()
	service.finish(task, cleaningrecord.CLEANING_STATE_DONE, "", true)
}

func (service *Service) updateScanProgress(task *activeTask, progress modelscanner.ScanProgress) {
	service.mu.Lock()
	if service.active != task || task.ctx.Err() != nil {
		service.mu.Unlock()
		return
	}
	task.snapshot.ScanProgress = progress
	snapshot := task.snapshot
	service.mu.Unlock()
	service.emit(EventTaskUpdated, snapshot)
}

func (service *Service) appendLLMDelta(task *activeTask, delta string) {
	if delta == "" {
		return
	}
	service.mu.Lock()
	if service.active != task || task.ctx.Err() != nil {
		service.mu.Unlock()
		return
	}
	task.snapshot.LLMOutput += delta
	recordID := task.snapshot.ID
	service.mu.Unlock()
	service.emit(EventLLMDelta, LLMDelta{RecordID: recordID, Delta: delta})
}

func (service *Service) finishFromError(task *activeTask, err error) {
	if task.logError == "" {
		task.logError = tasklog.ErrorSummary(err)
	}
	if errors.Is(err, context.Canceled) || errors.Is(task.ctx.Err(), context.Canceled) {
		service.finish(task, cleaningrecord.CLEANING_STATE_CANCELLED, "", false)
		return
	}
	service.finish(task, cleaningrecord.CLEANING_STATE_ERROR, err.Error(), false)
}

func (service *Service) finish(
	task *activeTask,
	state string,
	errorMessage string,
	keepTree bool,
) {
	if state != cleaningrecord.CLEANING_STATE_DONE {
		operationContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		persistErr := service.store.UpdateCleaningRecordState(
			operationContext,
			task.snapshot.ID,
			state,
			errorMessage,
		)
		cancel()
		if persistErr != nil {
			task.log.Event("状态保存失败", "%s", tasklog.ErrorSummary(persistErr))
		}
		if persistErr != nil && errorMessage == "" {
			errorMessage = persistErr.Error()
			if task.logError == "" {
				task.logError = "状态保存失败：" + tasklog.ErrorSummary(persistErr)
			}
		}
	}

	if task.logError == "" && errorMessage != "" {
		task.logError = errorMessage
	}
	task.log.Finish(logState(state), task.logError)
	service.mu.Lock()
	if service.active != task {
		service.mu.Unlock()
		return
	}
	task.snapshot.State = state
	task.snapshot.ErrorMessage = errorMessage
	task.snapshot.Stopping = false
	snapshot := task.snapshot
	retainTree := keepTree || (state == cleaningrecord.CLEANING_STATE_ERROR && service.tree != nil)
	if retainTree {
		service.treeSnapshot = &snapshot
	} else {
		service.tree = nil
		service.treeSnapshot = nil
	}
	service.active = nil
	task.cancel()
	close(task.done)
	service.mu.Unlock()

	service.emit(EventTaskUpdated, snapshot)
}
