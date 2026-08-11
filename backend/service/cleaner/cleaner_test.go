package cleaner

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"ai-disk-cleanner/backend/data/models/cleaningrecord"
	modelscanner "ai-disk-cleanner/backend/model/scanner"

	gormsqlite "gorm.io/driver/sqlite"
	"gorm.io/gorm"
	_ "modernc.org/sqlite"
)

func newTestStore(t *testing.T) *cleaningrecord.Store {
	t.Helper()
	db, err := gorm.Open(gormsqlite.Dialector{
		DriverName: "sqlite",
		DSN:        filepath.Join(t.TempDir(), "test.db"),
	}, &gorm.Config{})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	if err := db.AutoMigrate(&cleaningrecord.CleaningRecord{}); err != nil {
		t.Fatalf("migrate test database: %v", err)
	}
	store := cleaningrecord.NewStore(db)
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("close test database: %v", err)
		}
	})
	return store
}

type fakeAnalyzer struct {
	analyze func(context.Context, *modelscanner.FileTree, string, func(string)) (*cleaningrecord.AnalysisResult, error)
}

func (analyzer fakeAnalyzer) Analyze(
	ctx context.Context,
	tree *modelscanner.FileTree,
	language string,
	onDelta func(string),
) (*cleaningrecord.AnalysisResult, error) {
	return analyzer.analyze(ctx, tree, language, onDelta)
}

func TestServiceRunsTaskAndPersistsResult(t *testing.T) {
	store := newTestStore(t)
	done := make(chan CleaningTaskSnapshot, 1)
	states := make([]string, 0, 3)
	service := newServiceWithScanner(
		context.Background(),
		store,
		fakeAnalyzer{analyze: func(
			_ context.Context,
			_ *modelscanner.FileTree,
			language string,
			onDelta func(string),
		) (*cleaningrecord.AnalysisResult, error) {
			if language != "zh_CN" {
				t.Errorf("language = %q, want zh_CN", language)
			}
			onDelta("分析完成")
			return &cleaningrecord.AnalysisResult{LLMOutput: "分析完成", TokenUsage: 12}, nil
		}},
		func(event string, payload any) {
			if event != EventTaskUpdated {
				return
			}
			snapshot := payload.(CleaningTaskSnapshot)
			states = append(states, snapshot.State)
			if snapshot.State == cleaningrecord.CLEANING_STATE_DONE {
				done <- snapshot
			}
		},
		func(
			context.Context,
			string,
			func(modelscanner.ScanProgress),
		) (*modelscanner.FileTree, error) {
			return testTree(), nil
		},
	)

	snapshot, err := service.StartCleaning(t.TempDir(), "zh_CN")
	if err != nil {
		t.Fatalf("StartCleaning() error = %v", err)
	}
	if snapshot.State != cleaningrecord.CLEANING_STATE_SCANNING {
		t.Fatalf("initial state = %q", snapshot.State)
	}

	select {
	case finished := <-done:
		if finished.LLMOutput != "分析完成" {
			t.Fatalf("LLMOutput = %q", finished.LLMOutput)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("task did not finish")
	}

	wantStates := []string{
		cleaningrecord.CLEANING_STATE_SCANNING,
		cleaningrecord.CLEANING_STATE_ANALYZING,
		cleaningrecord.CLEANING_STATE_DONE,
	}
	if len(states) != len(wantStates) {
		t.Fatalf("states = %#v", states)
	}
	for index := range wantStates {
		if states[index] != wantStates[index] {
			t.Fatalf("states = %#v, want %#v", states, wantStates)
		}
	}
	record, err := store.GetCleaningRecord(context.Background(), snapshot.ID)
	if err != nil {
		t.Fatalf("GetCleaningRecord() error = %v", err)
	}
	if record.State != cleaningrecord.CLEANING_STATE_DONE || record.TokenUsage != 12 {
		t.Fatalf("completed record = %#v", record)
	}
	if service.Tree() == nil {
		t.Fatal("completed tree was not retained")
	}
}

func TestServiceRejectsConcurrentTaskAndCancelsScan(t *testing.T) {
	store := newTestStore(t)
	scanStarted := make(chan struct{})
	service := newServiceWithScanner(
		context.Background(),
		store,
		fakeAnalyzer{analyze: func(
			context.Context,
			*modelscanner.FileTree,
			string,
			func(string),
		) (*cleaningrecord.AnalysisResult, error) {
			t.Fatal("analyzer must not run after scan cancellation")
			return nil, nil
		}},
		nil,
		func(
			ctx context.Context,
			_ string,
			_ func(modelscanner.ScanProgress),
		) (*modelscanner.FileTree, error) {
			close(scanStarted)
			<-ctx.Done()
			return nil, ctx.Err()
		},
	)

	first, err := service.StartCleaning(t.TempDir(), "en")
	if err != nil {
		t.Fatalf("first StartCleaning() error = %v", err)
	}
	<-scanStarted
	if _, err := service.StartCleaning(t.TempDir(), "en"); !errors.Is(err, ErrTaskRunning) {
		t.Fatalf("second StartCleaning() error = %v, want ErrTaskRunning", err)
	}
	if err := service.StopCleaning(first.ID); err != nil {
		t.Fatalf("StopCleaning() error = %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for service.GetActiveCleaning() != nil && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if service.GetActiveCleaning() != nil {
		t.Fatal("cancelled task remained active")
	}
	record, err := store.GetCleaningRecord(context.Background(), first.ID)
	if err != nil {
		t.Fatalf("GetCleaningRecord() error = %v", err)
	}
	if record.State != cleaningrecord.CLEANING_STATE_CANCELLED {
		t.Fatalf("last state = %q", record.State)
	}
	if service.Tree() != nil {
		t.Fatal("cancelled scan retained a tree")
	}
}

func TestServiceRetainsFailedAnalysisSnapshotWhileTreeExists(t *testing.T) {
	store := newTestStore(t)
	done := make(chan CleaningTaskSnapshot, 1)
	service := newServiceWithScanner(
		context.Background(),
		store,
		fakeAnalyzer{analyze: func(
			_ context.Context,
			_ *modelscanner.FileTree,
			_ string,
			onDelta func(string),
		) (*cleaningrecord.AnalysisResult, error) {
			onDelta("部分分析结果")
			return nil, errors.New("analysis failed")
		}},
		func(event string, payload any) {
			if event != EventTaskUpdated {
				return
			}
			snapshot := payload.(CleaningTaskSnapshot)
			if snapshot.State == cleaningrecord.CLEANING_STATE_ERROR {
				done <- snapshot
			}
		},
		func(
			context.Context,
			string,
			func(modelscanner.ScanProgress),
		) (*modelscanner.FileTree, error) {
			return testTree(), nil
		},
	)

	if _, err := service.StartCleaning(t.TempDir(), "en"); err != nil {
		t.Fatalf("StartCleaning() error = %v", err)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("task did not fail")
	}

	if service.Tree() == nil {
		t.Fatal("failed analysis discarded the scanned tree")
	}
	snapshot := service.GetActiveCleaning()
	if snapshot == nil {
		t.Fatal("tree snapshot was not retained after analysis failure")
	}
	if snapshot.State != cleaningrecord.CLEANING_STATE_ERROR {
		t.Fatalf("snapshot state = %q", snapshot.State)
	}
	if snapshot.ErrorMessage != "analysis failed" {
		t.Fatalf("snapshot error = %q", snapshot.ErrorMessage)
	}
	if snapshot.LLMOutput != "部分分析结果" {
		t.Fatalf("snapshot output = %q", snapshot.LLMOutput)
	}
}

func TestServiceDeletesOnlyRecordedTrashFilesInsideScanRoot(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "cache.tmp")
	if err := os.WriteFile(target, []byte("cache"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := newTestStore(t)
	storedRecord := &cleaningrecord.CleaningRecord{
		ID:   1,
		Path: root,
		TrashFiles: []cleaningrecord.TrashFile{{
			Name: "cache", Path: "cache.tmp", Size: 5, Level: cleaningrecord.LOW,
		}},
		TopUsages: make([]cleaningrecord.DiskUsage, 0),
		State:     cleaningrecord.CLEANING_STATE_DONE,
	}
	if err := store.CreateCleaningRecord(context.Background(), storedRecord); err != nil {
		t.Fatalf("CreateCleaningRecord() error = %v", err)
	}
	service := newServiceWithScanner(context.Background(), store, nil, nil, nil)
	service.tree = &modelscanner.FileTree{RootPath: root}
	service.treeSnapshot = &CleaningTaskSnapshot{ID: 1, Path: root}

	if _, err := service.DeleteTrashFiles(1, []string{"unknown.tmp"}, false); err == nil {
		t.Fatal("unrecorded path was accepted")
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("validation failure touched target: %v", err)
	}
	if _, err := service.DeleteTrashFiles(1, []string{"cache.tmp"}, false); err != nil {
		t.Fatalf("DeleteTrashFiles() error = %v", err)
	}
	if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("target still exists: %v", err)
	}
	record, err := store.GetCleaningRecord(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetCleaningRecord() error = %v", err)
	}
	if len(record.TrashFiles) != 1 || !record.TrashFiles[0].IsDeleted || record.FreedSize != 5 {
		t.Fatalf("record after delete = %#v", record)
	}
	if _, err := service.DeleteTrashFiles(1, []string{"cache.tmp"}, false); err == nil {
		t.Fatal("already deleted candidate was accepted")
	}
}

func TestServiceDeletesTrashFileSelectedByResolvedPath(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "cache.tmp")
	if err := os.WriteFile(target, []byte("cache"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := newTestStore(t)
	storedRecord := &cleaningrecord.CleaningRecord{
		ID:   1,
		Path: root,
		TrashFiles: []cleaningrecord.TrashFile{{
			Name: "cache", Path: "cache.tmp", Size: 5, Level: cleaningrecord.LOW,
		}},
		TopUsages: make([]cleaningrecord.DiskUsage, 0),
		State:     cleaningrecord.CLEANING_STATE_DONE,
	}
	if err := store.CreateCleaningRecord(context.Background(), storedRecord); err != nil {
		t.Fatalf("CreateCleaningRecord() error = %v", err)
	}
	service := newServiceWithScanner(context.Background(), store, nil, nil, nil)

	failures, err := service.DeleteTrashFiles(1, []string{target}, false)
	if err != nil {
		t.Fatalf("DeleteTrashFiles() error = %v", err)
	}
	if len(failures) != 0 {
		t.Fatalf("DeleteTrashFiles() failures = %#v", failures)
	}
	if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("target still exists: %v", err)
	}
}

func TestServiceContinuesDeletingAfterFileFailure(t *testing.T) {
	root := t.TempDir()
	validTarget := filepath.Join(root, "cache.tmp")
	if err := os.WriteFile(validTarget, []byte("cache"), 0o600); err != nil {
		t.Fatal(err)
	}
	outsidePath := filepath.Join(t.TempDir(), "outside.tmp")
	store := newTestStore(t)
	storedRecord := &cleaningrecord.CleaningRecord{
		ID:   1,
		Path: root,
		TrashFiles: []cleaningrecord.TrashFile{
			{Name: "outside", Path: outsidePath, Size: 7, Level: cleaningrecord.LOW},
			{Name: "cache", Path: "cache.tmp", Size: 5, Level: cleaningrecord.LOW},
		},
		TopUsages: make([]cleaningrecord.DiskUsage, 0),
		State:     cleaningrecord.CLEANING_STATE_DONE,
	}
	if err := store.CreateCleaningRecord(context.Background(), storedRecord); err != nil {
		t.Fatalf("CreateCleaningRecord() error = %v", err)
	}
	service := newServiceWithScanner(context.Background(), store, nil, nil, nil)

	failures, err := service.DeleteTrashFiles(1, []string{outsidePath, "cache.tmp"}, false)
	if err != nil {
		t.Fatalf("DeleteTrashFiles() error = %v", err)
	}
	if len(failures) != 1 || failures[0].Path != outsidePath || failures[0].Message == "" {
		t.Fatalf("DeleteTrashFiles() failures = %#v", failures)
	}
	if _, err := os.Stat(validTarget); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("valid target still exists: %v", err)
	}
	record, err := store.GetCleaningRecord(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetCleaningRecord() error = %v", err)
	}
	if record.TrashFiles[0].IsDeleted || !record.TrashFiles[1].IsDeleted || record.FreedSize != 5 {
		t.Fatalf("record after partial delete = %#v", record)
	}
}

func TestRemoveTrashTargetContinuesAfterChildFailure(t *testing.T) {
	target := t.TempDir()
	childContents := map[string]string{
		"first":  "123",
		"failed": "12345",
		"last":   "1234567",
	}
	for name, content := range childContents {
		childPath := filepath.Join(target, name)
		if err := os.Mkdir(childPath, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(childPath, "data"), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	info, err := os.Lstat(target)
	if err != nil {
		t.Fatal(err)
	}
	removedPaths := make([]string, 0, 3)
	failures, freedSize := removeTrashTargetWith(target, info, true, func(path string) error {
		removedPaths = append(removedPaths, path)
		if filepath.Base(path) == "failed" {
			return errors.New("access denied")
		}
		return nil
	})

	if len(removedPaths) != 3 {
		t.Fatalf("remove calls = %#v", removedPaths)
	}
	if len(failures) != 1 || failures[0].Path != filepath.Join(target, "failed") || failures[0].Message != "access denied" {
		t.Fatalf("remove failures = %#v", failures)
	}
	if freedSize != int64(len(childContents["first"])+len(childContents["last"])) {
		t.Fatalf("freed size = %d", freedSize)
	}
}

func TestServiceKeepsSelectedDirectoryAndDeletesItsContents(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "cache")
	nested := filepath.Join(target, "nested")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "cache.tmp"), []byte("cache"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "nested.tmp"), []byte("nested"), 0o600); err != nil {
		t.Fatal(err)
	}

	store := newTestStore(t)
	storedRecord := &cleaningrecord.CleaningRecord{
		ID:   1,
		Path: root,
		TrashFiles: []cleaningrecord.TrashFile{{
			Name: "cache", Path: "cache", Size: 11, Level: cleaningrecord.LOW,
		}},
		TopUsages: make([]cleaningrecord.DiskUsage, 0),
		State:     cleaningrecord.CLEANING_STATE_DONE,
	}
	if err := store.CreateCleaningRecord(context.Background(), storedRecord); err != nil {
		t.Fatalf("CreateCleaningRecord() error = %v", err)
	}
	service := newServiceWithScanner(context.Background(), store, nil, nil, nil)
	service.tree = &modelscanner.FileTree{RootPath: root}
	service.treeSnapshot = &CleaningTaskSnapshot{ID: 1, Path: root}

	if _, err := service.DeleteTrashFiles(1, []string{"cache"}, true); err != nil {
		t.Fatalf("DeleteTrashFiles() error = %v", err)
	}
	entries, err := os.ReadDir(target)
	if err != nil {
		t.Fatalf("original directory was removed: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("original directory still contains entries: %#v", entries)
	}
	record, err := store.GetCleaningRecord(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetCleaningRecord() error = %v", err)
	}
	if !record.TrashFiles[0].IsDeleted || record.FreedSize != 11 {
		t.Fatalf("record after delete = %#v", record)
	}
}

func testTree() *modelscanner.FileTree {
	return &modelscanner.FileTree{
		RootPath: "C:\\test",
		Root: &modelscanner.FileNode{
			Name:      "test",
			Type:      modelscanner.NodeTypeDirectory,
			ItemCount: 1,
		},
	}
}
