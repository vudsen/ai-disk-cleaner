//go:build linux

package util

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestLinuxDataAndStatePathsRespectAbsoluteXDGOverrides(t *testing.T) {
	env := map[string]string{
		"XDG_DATA_HOME":  "/mnt/private/data",
		"XDG_STATE_HOME": "/mnt/private/state",
		"HOME":           "/home/test-user",
	}
	getenv := func(key string) string { return env[key] }
	home := func() (string, error) { return env["HOME"], nil }

	data, err := linuxDataFile(getenv, home)
	if err != nil {
		t.Fatal(err)
	}
	if want := "/mnt/private/data/ai-disk-cleaner/data.db"; data != want {
		t.Fatalf("linuxDataFile() = %q, want %q", data, want)
	}
	state, err := linuxLogDirectory(getenv, home)
	if err != nil {
		t.Fatal(err)
	}
	if want := "/mnt/private/state/ai-disk-cleaner/logs"; state != want {
		t.Fatalf("linuxLogDirectory() = %q, want %q", state, want)
	}
}

func TestLinuxDataAndStatePathsIgnoreRelativeXDGOverrides(t *testing.T) {
	env := map[string]string{
		"XDG_DATA_HOME":  "relative/data",
		"XDG_STATE_HOME": "relative/state",
	}
	getenv := func(key string) string { return env[key] }
	home := func() (string, error) { return "/home/test-user", nil }

	data, err := linuxDataFile(getenv, home)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join("/home/test-user", ".local/share/ai-disk-cleaner/data.db"); data != want {
		t.Fatalf("linuxDataFile() = %q, want %q", data, want)
	}
	state, err := linuxLogDirectory(getenv, home)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join("/home/test-user", ".local/state/ai-disk-cleaner/logs"); state != want {
		t.Fatalf("linuxLogDirectory() = %q, want %q", state, want)
	}
}

func TestLinuxDataAndStatePathsFailWithoutHome(t *testing.T) {
	wantErr := errors.New("no home")
	home := func() (string, error) { return "", wantErr }
	getenv := func(string) string { return "" }
	if _, err := linuxDataFile(getenv, home); !errors.Is(err, wantErr) {
		t.Fatalf("linuxDataFile() error = %v, want %v", err, wantErr)
	}
	if _, err := linuxLogDirectory(getenv, home); !errors.Is(err, wantErr) {
		t.Fatalf("linuxLogDirectory() error = %v, want %v", err, wantErr)
	}
}
