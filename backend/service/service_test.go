package service

import (
	"errors"
	"testing"

	"ai-disk-cleanner/backend/service/analyzer"
	"ai-disk-cleanner/backend/service/cleaner"
	"ai-disk-cleanner/backend/service/cleaninghistory"
	"ai-disk-cleanner/backend/service/migration"
	"ai-disk-cleanner/backend/service/scanner"
	"ai-disk-cleanner/backend/service/setting"
	"ai-disk-cleanner/backend/service/tasklog"
)

func TestGettersPanicBeforeInitialization(t *testing.T) {
	resetManagerForTest(t)
	getters := []func(){
		func() { GetAnalyzerService() },
		func() { GetCleanerService() },
		func() { GetCleaningHistoryService() },
		func() { GetMigrationService() },
		func() { GetScannerService() },
		func() { GetSettingService() },
		func() { GetTaskLogService() },
	}
	for _, getter := range getters {
		assertPanics(t, getter)
	}
}

func TestInitializePublishesEveryService(t *testing.T) {
	resetManagerForTest(t)
	want := services{
		analyzer:        &analyzer.Service{},
		cleaner:         &cleaner.Service{},
		cleaningHistory: &cleaninghistory.Service{},
		migration:       &migration.Service{},
		scanner:         &scanner.Service{},
		setting:         &setting.Service{},
		taskLog:         &tasklog.Service{},
	}
	buildServices = func() (services, error) { return want, nil }

	if err := Initialize(); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	if GetAnalyzerService() != want.analyzer ||
		GetCleanerService() != want.cleaner ||
		GetCleaningHistoryService() != want.cleaningHistory ||
		GetMigrationService() != want.migration ||
		GetScannerService() != want.scanner ||
		GetSettingService() != want.setting || GetTaskLogService() != want.taskLog {
		t.Fatal("a getter returned a different service instance")
	}
	assertPanics(t, func() { _ = Initialize() })
}

func TestInitializeFailurePublishesNothing(t *testing.T) {
	resetManagerForTest(t)
	wantErr := errors.New("initialization failed")
	buildServices = func() (services, error) { return services{}, wantErr }

	if err := Initialize(); !errors.Is(err, wantErr) {
		t.Fatalf("Initialize() error = %v, want %v", err, wantErr)
	}
	assertPanics(t, func() { GetCleanerService() })
	assertPanics(t, func() { _ = Initialize() })
}

func resetManagerForTest(t *testing.T) {
	t.Helper()
	initializationAttempted = false
	analyzerService = nil
	cleanerService = nil
	cleaningHistoryService = nil
	migrationService = nil
	scannerService = nil
	settingService = nil
	taskLogService = nil
	buildServices = setupServices
	t.Cleanup(func() {
		initializationAttempted = false
		analyzerService = nil
		cleanerService = nil
		cleaningHistoryService = nil
		migrationService = nil
		scannerService = nil
		settingService = nil
		taskLogService = nil
		buildServices = setupServices
	})
}

func assertPanics(t *testing.T, action func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatal("action did not panic")
		}
	}()
	action()
}
