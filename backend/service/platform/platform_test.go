package platform

import (
	"errors"
	"os"
	"runtime"
	"testing"

	"ai-disk-cleanner/backend/model"
)

func TestGetInfoUsesNativePlatformAndHome(t *testing.T) {
	service := newService(func() (string, error) { return "/home/test-user", nil }, func() ([]model.DiskInfo, error) { return []model.DiskInfo{}, nil }, func() bool { return false })

	info, err := service.GetInfo()
	if err != nil {
		t.Fatalf("GetInfo() error = %v", err)
	}
	if info.OS != runtime.GOOS {
		t.Fatalf("OS = %q, want %q", info.OS, runtime.GOOS)
	}
	if info.HomeDirectory != "/home/test-user" {
		t.Fatalf("HomeDirectory = %q", info.HomeDirectory)
	}
	if runtime.GOOS == "linux" && info.DefaultScanPath != info.HomeDirectory {
		t.Fatalf("Linux DefaultScanPath = %q, want home %q", info.DefaultScanPath, info.HomeDirectory)
	}
}

func TestGetInfoReportsHomeLookupError(t *testing.T) {
	wantErr := errors.New("no home")
	service := newService(func() (string, error) { return "", wantErr }, nil, nil)
	if _, err := service.GetInfo(); !errors.Is(err, wantErr) {
		t.Fatalf("GetInfo() error = %v, want %v", err, wantErr)
	}
}

func TestIsRunningAsAdministratorUsesNativeCheck(t *testing.T) {
	service := newService(nil, nil, func() bool { return os.Geteuid() == 0 })
	if got, want := service.IsRunningAsAdministrator(), os.Geteuid() == 0; got != want {
		t.Fatalf("IsRunningAsAdministrator() = %v, want %v", got, want)
	}
}
