// Package service owns the process-wide application services.
package service

import (
	"fmt"

	"ai-disk-cleanner/backend/data"
	"ai-disk-cleanner/backend/service/analyzer"
	"ai-disk-cleanner/backend/service/cleaner"
	"ai-disk-cleanner/backend/service/cleaninghistory"
	"ai-disk-cleanner/backend/service/migration"
	"ai-disk-cleanner/backend/service/scanner"
	"ai-disk-cleanner/backend/service/setting"
	"ai-disk-cleanner/backend/service/tasklog"
)

var (
	initializationAttempted bool
	analyzerService         *analyzer.Service
	cleanerService          *cleaner.Service
	cleaningHistoryService  *cleaninghistory.Service
	migrationService        *migration.Service
	scannerService          *scanner.Service
	settingService          *setting.Service
	taskLogService          *tasklog.Service
	buildServices           = setupServices
)

type services struct {
	analyzer        *analyzer.Service
	cleaner         *cleaner.Service
	cleaningHistory *cleaninghistory.Service
	migration       *migration.Service
	scanner         *scanner.Service
	setting         *setting.Service
	taskLog         *tasklog.Service
}

// Initialize creates every application service during Wails startup.
// It must be called exactly once, after backend/ctx has been initialized.
func Initialize() error {
	if initializationAttempted {
		panic("service manager: Initialize called more than once")
	}
	initializationAttempted = true

	created, err := buildServices()
	if err != nil {
		return err
	}
	publishServices(created)
	return nil
}

func setupServices() (services, error) {
	store, err := data.GetStore()
	if err != nil {
		return services{}, fmt.Errorf("initialize services: %w", err)
	}

	newScannerService := scanner.NewService()
	newSettingService := setting.NewService(store)
	newAnalyzerService := analyzer.NewService(store)
	newTaskLogService := tasklog.NewService()
	newCleaningHistoryService := cleaninghistory.NewService(store, newTaskLogService)
	newMigrationService := migration.NewService(store)
	newCleanerService := cleaner.NewService(
		store.Store,
		newAnalyzerService,
		newScannerService,
		newTaskLogService,
	)

	if err := newCleaningHistoryService.CleanupOnStartup(); err != nil {
		return services{}, fmt.Errorf("initialize cleaning history service: %w", err)
	}

	return services{
		analyzer:        newAnalyzerService,
		cleaner:         newCleanerService,
		cleaningHistory: newCleaningHistoryService,
		migration:       newMigrationService,
		scanner:         newScannerService,
		setting:         newSettingService,
		taskLog:         newTaskLogService,
	}, nil
}

func publishServices(created services) {
	// Publish only after all construction and startup work has succeeded.
	scannerService = created.scanner
	settingService = created.setting
	taskLogService = created.taskLog
	analyzerService = created.analyzer
	cleaningHistoryService = created.cleaningHistory
	migrationService = created.migration
	cleanerService = created.cleaner
}

func GetAnalyzerService() *analyzer.Service {
	if analyzerService == nil {
		panic("service manager: analyzer service is not initialized")
	}
	return analyzerService
}

func GetTaskLogService() *tasklog.Service {
	if taskLogService == nil {
		panic("task log service is not initialized")
	}
	return taskLogService
}

func GetCleanerService() *cleaner.Service {
	if cleanerService == nil {
		panic("service manager: cleaner service is not initialized")
	}
	return cleanerService
}

func GetCleaningHistoryService() *cleaninghistory.Service {
	if cleaningHistoryService == nil {
		panic("service manager: cleaning history service is not initialized")
	}
	return cleaningHistoryService
}

func GetMigrationService() *migration.Service {
	if migrationService == nil {
		panic("service manager: migration service is not initialized")
	}
	return migrationService
}

func GetScannerService() *scanner.Service {
	if scannerService == nil {
		panic("service manager: scanner service is not initialized")
	}
	return scannerService
}

func GetSettingService() *setting.Service {
	if settingService == nil {
		panic("service manager: setting service is not initialized")
	}
	return settingService
}
