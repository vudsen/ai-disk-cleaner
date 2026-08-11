package cleaner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	appctx "ai-disk-cleanner/backend/ctx"
	"ai-disk-cleanner/backend/data/models/cleaningrecord"
	modelscanner "ai-disk-cleanner/backend/model/scanner"
	serviceScanner "ai-disk-cleanner/backend/service/scanner"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const (
	EventTaskUpdated = "cleaning:task-updated"
	EventLLMDelta    = "cleaning:llm-delta"
)

var (
	ErrTaskRunning  = errors.New("a cleaning task is already running")
	ErrNoActiveTask = errors.New("no active cleaning task")
)

// Analyzer performs the LLM phase. Implementations must stop when ctx is cancelled.
type Analyzer interface {
	Analyze(
		ctx context.Context,
		tree *modelscanner.FileTree,
		language string,
		onDelta func(string),
	) (*cleaningrecord.AnalysisResult, error)
}

type ScanFunc func(
	context.Context,
	string,
	func(modelscanner.ScanProgress),
) (*modelscanner.FileTree, error)

type EventEmitter func(string, any)

// CleaningTaskSnapshot is the frontend-safe in-memory view of the current task.
type CleaningTaskSnapshot struct {
	ID           int64                     `json:"id"`
	StartTime    time.Time                 `json:"startTime"`
	Path         string                    `json:"path"`
	State        string                    `json:"state"`
	ErrorMessage string                    `json:"errorMessage"`
	LLMOutput    string                    `json:"llmOutput"`
	ScanProgress modelscanner.ScanProgress `json:"scanProgress"`
	Stopping     bool                      `json:"stopping"`
}

// LLMDelta is emitted for each assistant text fragment.
type LLMDelta struct {
	RecordID int64  `json:"recordId"`
	Delta    string `json:"delta"`
}

// DeleteFailure describes a trash file that could not be deleted.
type DeleteFailure struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

// Service owns the process-wide cleaning task and the latest scanned tree.
type Service struct {
	ctx      context.Context
	store    *cleaningrecord.Store
	analyzer Analyzer
	scan     ScanFunc
	emit     EventEmitter

	mu           sync.RWMutex
	active       *activeTask
	tree         *modelscanner.FileTree
	treeSnapshot *CleaningTaskSnapshot
}

// NewService creates the cleaner service for the central service manager.
func NewService(
	store *cleaningrecord.Store,
	analyzer Analyzer,
	scanner *serviceScanner.Service,
) *Service {
	if scanner == nil {
		panic("cleaner service: scanner is nil")
	}
	emit := EventEmitter(func(string, any) {})
	if appctx.HasWailsEvents() {
		emit = func(eventName string, payload any) {
			runtime.EventsEmit(appctx.GetContext(), eventName, payload)
		}
	}
	return newServiceWithScanner(
		appctx.GetContext(),
		store,
		analyzer,
		emit,
		scanner.ParseGDUContext,
	)
}

func newServiceWithScanner(
	ctx context.Context,
	store *cleaningrecord.Store,
	analyzer Analyzer,
	emit EventEmitter,
	scan ScanFunc,
) *Service {
	if ctx == nil {
		ctx = context.Background()
	}
	if emit == nil {
		emit = func(string, any) {}
	}
	if scan == nil {
		scan = serviceScanner.NewService().ParseGDUContext
	}
	return &Service{
		ctx:      ctx,
		store:    store,
		analyzer: analyzer,
		scan:     scan,
		emit:     emit,
	}
}

// StartCleaning creates the record synchronously and starts the expensive work in the background.
func (service *Service) StartCleaning(directoryPath string, language string) (*CleaningTaskSnapshot, error) {
	if service.store == nil {
		return nil, errors.New("start cleaning: record store is nil")
	}
	if service.analyzer == nil {
		return nil, errors.New("start cleaning: analyzer is nil")
	}
	absolutePath, err := validateDirectory(directoryPath)
	if err != nil {
		return nil, err
	}

	service.mu.Lock()
	if service.active != nil {
		service.mu.Unlock()
		return nil, ErrTaskRunning
	}

	record := &cleaningrecord.CleaningRecord{
		StartTime:  time.Now(),
		TrashFiles: make([]cleaningrecord.TrashFile, 0),
		TopUsages:  make([]cleaningrecord.DiskUsage, 0),
		Path:       absolutePath,
		State:      cleaningrecord.CLEANING_STATE_SCANNING,
	}
	if err := service.store.CreateCleaningRecord(service.ctx, record); err != nil {
		service.mu.Unlock()
		return nil, err
	}

	taskContext, cancel := context.WithCancel(service.ctx)
	task := &activeTask{
		snapshot: CleaningTaskSnapshot{
			ID:        record.ID,
			StartTime: record.StartTime,
			Path:      absolutePath,
			State:     cleaningrecord.CLEANING_STATE_SCANNING,
		},
		ctx:      taskContext,
		cancel:   cancel,
		done:     make(chan struct{}),
		language: language,
	}
	service.active = task
	service.tree = nil
	service.treeSnapshot = nil
	snapshot := task.snapshot
	service.mu.Unlock()

	service.emit(EventTaskUpdated, snapshot)
	go service.run(task)
	return &snapshot, nil
}

// StopCleaning requests cancellation. The task remains active until its worker exits.
func (service *Service) StopCleaning(recordID int64) error {
	service.mu.Lock()
	if service.active == nil {
		service.mu.Unlock()
		return ErrNoActiveTask
	}
	if service.active.snapshot.ID != recordID {
		service.mu.Unlock()
		return fmt.Errorf(
			"stop cleaning record %d: active record is %d",
			recordID,
			service.active.snapshot.ID,
		)
	}
	service.active.snapshot.Stopping = true
	snapshot := service.active.snapshot
	cancel := service.active.cancel
	service.mu.Unlock()

	cancel()
	service.emit(EventTaskUpdated, snapshot)
	return nil
}

func (service *Service) GetActiveCleaning() *CleaningTaskSnapshot {
	service.mu.RLock()
	defer service.mu.RUnlock()
	if service.active != nil {
		snapshot := service.active.snapshot
		return &snapshot
	}
	if service.tree == nil || service.treeSnapshot == nil {
		return nil
	}
	snapshot := *service.treeSnapshot
	return &snapshot
}

func (service *Service) ListCleaningRecords() ([]cleaningrecord.CleaningRecord, error) {
	return service.store.ListCleaningRecords(service.ctx)
}

func (service *Service) DeleteTrashFiles(
	recordID int64,
	selectedPaths []string,
	keepOriginalDirectories bool,
) ([]DeleteFailure, error) {
	if len(selectedPaths) == 0 {
		return nil, errors.New("delete trash files: no files selected")
	}
	service.mu.RLock()
	defer service.mu.RUnlock()

	record, err := service.store.GetCleaningRecord(service.ctx, recordID)
	if err != nil {
		return nil, err
	}
	currentRootPath := record.Path

	if !samePath(record.Path, currentRootPath) {
		return nil, fmt.Errorf("delete trash files for record %d: scan root does not match the current in-memory tree", recordID)
	}
	knownCandidates := make(map[string]int, len(record.TrashFiles)*2)
	for index, candidate := range record.TrashFiles {
		knownCandidates[candidate.Path] = index
		if target, targetErr := toAbsPath(record.Path, candidate.Path); targetErr == nil {
			knownCandidates[target] = index
		}
	}
	selected := make(map[string]struct{}, len(selectedPaths))
	for _, path := range selectedPaths {
		candidateIndex, ok := knownCandidates[path]
		if !ok && keepOriginalDirectories {
			candidateIndex, ok = containingTrashCandidate(record, path)
		}
		if !ok {
			return nil, fmt.Errorf("delete trash file: path %q is not a candidate in record %d", path, recordID)
		}
		candidate := record.TrashFiles[candidateIndex]
		if candidate.IsDeleted {
			return nil, fmt.Errorf("delete trash file: path %q has already been deleted", candidate.Path)
		}
		if strings.ContainsAny(candidate.Path, "*?[") {
			return nil, fmt.Errorf("delete trash file: glob path %q requires manual review", candidate.Path)
		}
		selected[candidate.Path] = struct{}{}
	}

	failures := make([]DeleteFailure, 0)
	var freedSize int64
	for index := range record.TrashFiles {
		candidate := &record.TrashFiles[index]
		if _, ok := selected[candidate.Path]; !ok {
			continue
		}
		target, err := toAbsPath(record.Path, candidate.Path)
		if err != nil {
			failures = append(failures, DeleteFailure{Path: resolvedCandidatePath(record.Path, candidate.Path), Message: err.Error()})
			continue
		}
		info, err := os.Lstat(target)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			failures = append(failures, DeleteFailure{Path: target, Message: err.Error()})
			continue
		}
		if info != nil {
			deleteFailures, deletedSize := removeTrashTarget(target, info, keepOriginalDirectories)
			freedSize += deletedSize
			if len(deleteFailures) > 0 {
				failures = append(failures, deleteFailures...)
				continue
			}
			if !keepOriginalDirectories || !info.IsDir() {
				freedSize += candidate.Size
			}
		}
		candidate.IsDeleted = true
	}
	record.FreedSize += freedSize
	if err := service.store.SaveDeletedTrashFiles(service.ctx, record); err != nil {
		return failures, err
	}
	return failures, nil
}

func containingTrashCandidate(record *cleaningrecord.CleaningRecord, requestedPath string) (int, bool) {
	requestedTarget, err := filepath.Abs(requestedPath)
	if err != nil {
		return 0, false
	}
	candidateIndex := 0
	longestTarget := -1
	for index, candidate := range record.TrashFiles {
		target, err := toAbsPath(record.Path, candidate.Path)
		if err != nil {
			continue
		}
		info, err := os.Lstat(target)
		if err != nil || !info.IsDir() {
			continue
		}
		relative, err := filepath.Rel(target, requestedTarget)
		if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			continue
		}
		if len(target) > longestTarget {
			candidateIndex = index
			longestTarget = len(target)
		}
	}
	return candidateIndex, longestTarget >= 0
}

func resolvedCandidatePath(rootPath string, candidatePath string) string {
	target := candidatePath
	if !filepath.IsAbs(target) {
		target = filepath.Join(rootPath, target)
	}
	absolutePath, err := filepath.Abs(target)
	if err != nil {
		return candidatePath
	}
	return absolutePath
}

func removeTrashTarget(target string, info os.FileInfo, keepOriginalDirectory bool) ([]DeleteFailure, int64) {
	return removeTrashTargetWith(target, info, keepOriginalDirectory, os.RemoveAll)
}

func removeTrashTargetWith(
	target string,
	info os.FileInfo,
	keepOriginalDirectory bool,
	remove func(string) error,
) ([]DeleteFailure, int64) {
	if !keepOriginalDirectory || !info.IsDir() {
		if err := remove(target); err != nil {
			return []DeleteFailure{{Path: target, Message: err.Error()}}, 0
		}
		return nil, 0
	}
	entries, err := os.ReadDir(target)
	if err != nil {
		return []DeleteFailure{{Path: target, Message: err.Error()}}, 0
	}
	failures := make([]DeleteFailure, 0)
	var freedSize int64
	for _, entry := range entries {
		childPath := filepath.Join(target, entry.Name())
		sizeBefore, sizeBeforeErr := trashTargetSize(childPath)
		if err := remove(childPath); err != nil {
			failures = append(failures, DeleteFailure{Path: childPath, Message: err.Error()})
			if sizeBeforeErr == nil {
				sizeAfter, sizeAfterErr := trashTargetSize(childPath)
				switch {
				case errors.Is(sizeAfterErr, os.ErrNotExist):
					freedSize += sizeBefore
				case sizeAfterErr == nil && sizeBefore > sizeAfter:
					freedSize += sizeBefore - sizeAfter
				}
			}
			continue
		}
		if sizeBeforeErr == nil {
			freedSize += sizeBefore
		}
	}
	return failures, freedSize
}

func trashTargetSize(target string) (int64, error) {
	var size int64
	err := filepath.WalkDir(target, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		size += info.Size()
		return nil
	})
	return size, err
}

// Tree returns the latest completed scan tree.
func (service *Service) Tree() *modelscanner.FileTree {
	service.mu.RLock()
	defer service.mu.RUnlock()
	return service.tree
}

// Close cancels the active task and waits for it to leave the single-task slot.
func (service *Service) Close(ctx context.Context) error {
	service.mu.RLock()
	if service.active == nil {
		service.mu.RUnlock()
		return nil
	}
	cancel := service.active.cancel
	done := service.active.done
	service.mu.RUnlock()

	cancel()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
