package platform

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	"ai-disk-cleanner/backend/model"
	"ai-disk-cleanner/backend/util"
)

func newService(homeDirectory func() (string, error), listDisks func() ([]model.DiskInfo, error), isAdmin func() bool) *Service {
	return &Service{homeDirectory: homeDirectory, listDisks: listDisks, isAdmin: isAdmin}
}

func defaultHomeDirectory() (string, error)       { return os.UserHomeDir() }
func defaultListDisks() ([]model.DiskInfo, error) { return util.ListDisks() }
func defaultIsAdmin() bool                        { return util.IsRunningAsAdministrator() }

func getPlatformInfo(homeDirectory func() (string, error)) (model.PlatformInfo, error) {
	home, err := homeDirectory()
	if err != nil {
		return model.PlatformInfo{}, fmt.Errorf("resolve home directory: %w", err)
	}
	home = strings.TrimSpace(home)
	if home == "" {
		return model.PlatformInfo{}, fmt.Errorf("resolve home directory: empty path")
	}

	info := model.PlatformInfo{OS: runtime.GOOS, HomeDirectory: home}
	switch runtime.GOOS {
	case "linux":
		info.DefaultScanPath = home
	case "windows":
		info.DefaultScanPath = os.Getenv("SystemDrive")
		if info.DefaultScanPath == "" {
			info.DefaultScanPath = `C:`
		}
		info.DefaultScanPath += `\`
	default:
		return model.PlatformInfo{}, fmt.Errorf("platform %q is not supported", runtime.GOOS)
	}
	return info, nil
}
