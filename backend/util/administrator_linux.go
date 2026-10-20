//go:build linux

package util

import "os"

func IsRunningAsAdministrator() bool {
	return os.Geteuid() == 0
}
