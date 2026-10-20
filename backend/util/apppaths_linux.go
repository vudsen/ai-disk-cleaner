//go:build linux

package util

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func DataFilePath() (string, error) {
	path, err := linuxDataFile(os.Getenv, os.UserHomeDir)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return "", fmt.Errorf("create application data directory: %w", err)
	}
	if err := os.Chmod(filepath.Dir(path), 0700); err != nil {
		return "", fmt.Errorf("secure application data directory: %w", err)
	}
	return path, nil
}

func LogDirectoryPath() (string, error) {
	path, err := linuxLogDirectory(os.Getenv, os.UserHomeDir)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(path, 0700); err != nil {
		return "", fmt.Errorf("create application log directory: %w", err)
	}
	if err := os.Chmod(path, 0700); err != nil {
		return "", fmt.Errorf("secure application log directory: %w", err)
	}
	return path, nil
}

func linuxDataFile(getenv func(string) string, homeDirectory func() (string, error)) (string, error) {
	return linuxAppPath(getenv("XDG_DATA_HOME"), ".local/share", "data.db", homeDirectory)
}

func linuxLogDirectory(getenv func(string) string, homeDirectory func() (string, error)) (string, error) {
	return linuxAppPath(getenv("XDG_STATE_HOME"), ".local/state", filepath.Join("logs", ""), homeDirectory)
}

func linuxAppPath(xdgHome, fallback, suffix string, homeDirectory func() (string, error)) (string, error) {
	base := strings.TrimSpace(xdgHome)
	if !filepath.IsAbs(base) {
		home, err := homeDirectory()
		if err != nil {
			return "", fmt.Errorf("resolve home directory: %w", err)
		}
		if !filepath.IsAbs(home) {
			return "", fmt.Errorf("resolve home directory: path is not absolute")
		}
		base = filepath.Join(home, fallback)
	}
	return filepath.Join(base, "ai-disk-cleaner", suffix), nil
}
