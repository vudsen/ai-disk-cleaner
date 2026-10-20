// Package platform exposes host platform information to the application.
package platform

import "ai-disk-cleanner/backend/model"

// Service provides platform and disk information.
type Service struct {
	homeDirectory func() (string, error)
	listDisks     func() ([]model.DiskInfo, error)
	isAdmin       func() bool
}

// NewService creates a platform service for the central service manager.
func NewService() *Service {
	return newService(defaultHomeDirectory, defaultListDisks, defaultIsAdmin)
}

// ListDisks returns the host's eligible disks or mounted filesystems.
func (service *Service) ListDisks() ([]model.DiskInfo, error) {
	return service.listDisks()
}

// IsRunningAsAdministrator reports whether the current process is elevated.
func (service *Service) IsRunningAsAdministrator() bool {
	return service.isAdmin()
}

// GetInfo returns the native platform and its default scan directory.
func (service *Service) GetInfo() (model.PlatformInfo, error) {
	return getPlatformInfo(service.homeDirectory)
}
