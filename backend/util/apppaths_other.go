//go:build !windows && !linux

package util

import (
	"os"
	"path/filepath"
)

func DataFilePath() (string, error) { return "./data.db", nil }

func LogDirectoryPath() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(executable), "logs"), nil
}
