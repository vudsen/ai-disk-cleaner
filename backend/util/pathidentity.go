package util

import (
	"path/filepath"
	"runtime"
	"strings"
)

// SamePath compares cleaned paths using the host filesystem's case rules.
func SamePath(left, right string) bool {
	left = filepath.Clean(left)
	right = filepath.Clean(right)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(left, right)
	}
	return left == right
}

// PathKey returns a stable key using the host filesystem's case rules.
func PathKey(path string) string {
	path = filepath.Clean(path)
	if runtime.GOOS == "windows" {
		return strings.ToLower(path)
	}
	return path
}
