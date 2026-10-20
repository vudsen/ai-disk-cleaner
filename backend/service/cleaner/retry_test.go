package cleaner

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"ai-disk-cleanner/backend/data/models/cleaningrecord"
	modelscanner "ai-disk-cleanner/backend/model/scanner"
)

func TestRetryHistoryUsesSavedTreeWithoutScanning(t *testing.T) {
	for _, state := range []string{"ERROR", "CANCELLED", "DONE"} {
		t.Run(state, func(t *testing.T) {
			store := newTestStore(t)
			record := &cleaningrecord.CleaningRecord{ID: 1, Path: "/no-longer-mounted", State: state, TrashFiles: []cleaningrecord.TrashFile{}, TopUsages: []cleaningrecord.DiskUsage{}}
			if err := store.CreateCleaningRecord(context.Background(), record); err != nil {
				t.Fatal(err)
			}
			tree := testTree()
			tree.RootPath = record.Path
			if err := store.SaveScanTree(context.Background(), 1, tree); err != nil {
				t.Fatal(err)
			}
			done := make(chan CleaningTaskSnapshot, 1)
			service := newServiceWithScanner(context.Background(), store, fakeAnalyzer{analyze: func(_ context.Context, restored *modelscanner.FileTree, language string, _ func(string)) (*cleaningrecord.AnalysisResult, error) {
				if restored.RootPath != record.Path || restored.Root.ItemCount != tree.Root.ItemCount || language != "en" {
					t.Errorf("restored tree/language = %#v/%s", restored, language)
				}
				return &cleaningrecord.AnalysisResult{LLMOutput: "Recovered"}, nil
			}}, func(event string, payload any) {
				if event == EventTaskUpdated {
					s := payload.(CleaningTaskSnapshot)
					if s.State == "DONE" || s.State == "ERROR" {
						done <- s
					}
				}
			},
				func(context.Context, string, func(modelscanner.ScanProgress)) (*modelscanner.FileTree, error) {
					t.Error("retry must never scan")
					return nil, errors.New("unexpected scan")
				})
			retry, err := service.RetryCleaning(1, "en")
			if err != nil || retry.ID != 1 || retry.State != "ANALYZING" {
				t.Fatalf("retry = %#v, %v", retry, err)
			}
			select {
			case s := <-done:
				if s.State != "DONE" {
					t.Fatal(s)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("retry timed out")
			}
			records, err := service.ListCleaningRecords()
			if err != nil || len(records) != 1 || records[0].LLMOutput != "Recovered" || records[0].ScanProgress == nil || records[0].ScanProgress.ItemCount != tree.Root.ItemCount {
				t.Fatalf("records = %#v, %v", records, err)
			}
		})
	}
}

func TestRetryMissingSnapshotNeverScans(t *testing.T) {
	store := newTestStore(t)
	if err := store.CreateCleaningRecord(context.Background(), &cleaningrecord.CleaningRecord{ID: 1, Path: t.TempDir(), State: "ERROR", TrashFiles: []cleaningrecord.TrashFile{}, TopUsages: []cleaningrecord.DiskUsage{}}); err != nil {
		t.Fatal(err)
	}
	service := newServiceWithScanner(context.Background(), store, fakeAnalyzer{}, nil, func(context.Context, string, func(modelscanner.ScanProgress)) (*modelscanner.FileTree, error) {
		t.Error("missing snapshot triggered scan")
		return nil, errors.New("unexpected scan")
	})
	if _, err := service.RetryCleaning(1, "en"); err == nil || !strings.Contains(err.Error(), "快照") {
		t.Fatalf("missing snapshot error = %v", err)
	}
	service.active = &activeTask{}
	if _, err := service.RetryCleaning(1, "en"); !errors.Is(err, ErrTaskRunning) {
		t.Fatal(err)
	}
}

func TestCompletedScanPersistsTreeBeforeAnalyzerFailure(t *testing.T) {
	store := newTestStore(t)
	done := make(chan CleaningTaskSnapshot, 1)
	service := newServiceWithScanner(context.Background(), store, fakeAnalyzer{analyze: func(context.Context, *modelscanner.FileTree, string, func(string)) (*cleaningrecord.AnalysisResult, error) {
		return nil, errors.New("offline")
	}}, func(event string, payload any) {
		if event == EventTaskUpdated {
			s := payload.(CleaningTaskSnapshot)
			if s.State == "ERROR" {
				done <- s
			}
		}
	}, func(context.Context, string, func(modelscanner.ScanProgress)) (*modelscanner.FileTree, error) {
		return testTree(), nil
	})
	started, err := service.StartCleaning(t.TempDir(), "en")
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("task timed out")
	}
	tree, err := store.LoadScanTree(context.Background(), started.ID)
	if err != nil || tree.Root.ItemCount != 1 {
		t.Fatalf("saved tree=%#v, %v", tree, err)
	}
}

func TestRetryCanPersistExistingRAMTreeWithoutScanning(t *testing.T) {
	store := newTestStore(t)
	record := &cleaningrecord.CleaningRecord{ID: 1, Path: "/offline", State: "ERROR", TrashFiles: []cleaningrecord.TrashFile{}, TopUsages: []cleaningrecord.DiskUsage{}}
	if err := store.CreateCleaningRecord(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	done := make(chan CleaningTaskSnapshot, 1)
	service := newServiceWithScanner(context.Background(), store, fakeAnalyzer{analyze: func(context.Context, *modelscanner.FileTree, string, func(string)) (*cleaningrecord.AnalysisResult, error) {
		return &cleaningrecord.AnalysisResult{}, nil
	}}, func(event string, payload any) {
		if event == EventTaskUpdated {
			s := payload.(CleaningTaskSnapshot)
			if s.State == "DONE" || s.State == "ERROR" {
				done <- s
			}
		}
	}, func(context.Context, string, func(modelscanner.ScanProgress)) (*modelscanner.FileTree, error) {
		t.Error("RAM retry scanned disk")
		return nil, errors.New("unexpected scan")
	})
	service.tree = testTree()
	service.tree.RootPath = record.Path
	service.treeSnapshot = &CleaningTaskSnapshot{ID: 1, Path: record.Path, State: "ERROR"}
	if _, err := service.RetryCleaning(1, "en"); err != nil {
		t.Fatal(err)
	}
	select {
	case s := <-done:
		if s.State != "DONE" {
			t.Fatal(s)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
	if _, err := store.LoadScanTree(context.Background(), 1); err != nil {
		t.Fatalf("RAM tree was not persisted: %v", err)
	}
}

type brokenSnapshotStore struct{ *testStore }

func (store brokenSnapshotStore) LoadScanTree(context.Context, int64) (*modelscanner.FileTree, error) {
	return nil, errors.New("corrupt snapshot fixture")
}
func TestRetryCorruptSnapshotFailsWithoutScanning(t *testing.T) {
	store := newTestStore(t)
	if err := store.CreateCleaningRecord(context.Background(), &cleaningrecord.CleaningRecord{ID: 1, Path: "/offline", State: "DONE", TrashFiles: []cleaningrecord.TrashFile{}, TopUsages: []cleaningrecord.DiskUsage{}}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveScanTree(context.Background(), 1, testTree()); err != nil {
		t.Fatal(err)
	}
	done := make(chan CleaningTaskSnapshot, 1)
	service := newServiceWithScanner(context.Background(), brokenSnapshotStore{store}, fakeAnalyzer{}, func(event string, payload any) {
		if event == EventTaskUpdated {
			s := payload.(CleaningTaskSnapshot)
			if s.State == "ERROR" {
				done <- s
			}
		}
	}, func(context.Context, string, func(modelscanner.ScanProgress)) (*modelscanner.FileTree, error) {
		t.Error("corrupt snapshot triggered scan")
		return nil, errors.New("unexpected scan")
	})
	if _, err := service.RetryCleaning(1, "en"); err != nil {
		t.Fatal(err)
	}
	select {
	case s := <-done:
		if !strings.Contains(s.ErrorMessage, "corrupt snapshot") {
			t.Fatal(s)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout")
	}
}
