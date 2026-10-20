//go:build !windows && !linux

package util

func IsRunningAsAdministrator() bool {
	return false
}
