package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestLLMAnalysisFindsExpectedTrashFiles(t *testing.T) {
	testDirectory := "test"
	archivePath := filepath.Join(testDirectory, "fake_directory.tar.gz")
	fixturePath := filepath.Join(testDirectory, "fake_directory")
	ensureArchiveExtracted(t, archivePath, testDirectory, fixturePath)

	app := NewApp()
	app.startup(context.Background())
	if err := app.ready(); err != nil {
		t.Fatalf("start application: %v", err)
	}
	t.Cleanup(func() {
		app.shutdown(context.Background())
	})
	initializeLLMFromEnvironment(t, app)

	task, err := app.StartCleaning(fixturePath, "zh_CN")
	if err != nil {
		t.Fatalf("start cleaning: %v", err)
	}

	waitForCleaning(t, app, task.ID)
	records, err := app.ListCleaningRecords()
	if err != nil {
		t.Fatalf("list cleaning records: %v", err)
	}
	var trashFiles []string
	for _, record := range records {
		if record.ID != task.ID {
			continue
		}
		trashFiles = make([]string, 0, len(record.TrashFiles))
		for _, file := range record.TrashFiles {
			trashFiles = append(trashFiles, file.Path)
		}
		break
	}
	if trashFiles == nil {
		t.Fatalf("cleaning record %d was not found", task.ID)
	}

	want := []string{
		"/Users/tom/temp.log",
		"/Users/tom/Downloads/java.exe",
		"/Users/tom/AppData/Local/pnpm-cache",
	}
	got := make(map[string]struct{}, len(trashFiles))
	for _, path := range trashFiles {
		got[normalizeCandidatePath(path)] = struct{}{}
	}
	if missing := missingPathPrefixes(got, want); len(missing) > 0 {
		t.Fatalf("required trash path prefixes were not found\n got: %v\nmissing: %v", sortedPaths(got), missing)
	}
}

func initializeLLMFromEnvironment(t *testing.T, app *App) {
	t.Helper()
	values := map[string]string{
		"llm.url":    strings.TrimSpace(os.Getenv("LLM_BASE_URL")),
		"llm.secret": strings.TrimSpace(os.Getenv("LLM_API_KEY")),
		"llm.model":  strings.TrimSpace(os.Getenv("LLM_MODEL")),
	}
	missing := make([]string, 0, len(values))
	for key, environmentName := range map[string]string{
		"llm.url":    "LLM_BASE_URL",
		"llm.secret": "LLM_API_KEY",
		"llm.model":  "LLM_MODEL",
	} {
		if values[key] == "" {
			missing = append(missing, environmentName)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		t.Skipf("LLM integration test requires environment variables: %s", strings.Join(missing, ", "))
	}

	original, err := app.ListSettings()
	if err != nil {
		t.Fatalf("list original LLM settings: %v", err)
	}
	t.Cleanup(func() {
		if err := app.SaveSettings(original); err != nil {
			t.Errorf("restore LLM settings: %v", err)
		}
	})

	settings := append(original[:0:0], original...)
	overridden := make(map[string]bool, len(values))
	for index := range settings {
		value, ok := values[settings[index].Key]
		if !ok {
			continue
		}
		settings[index].Value = value
		overridden[settings[index].Key] = true
	}
	for key := range values {
		if !overridden[key] {
			t.Fatalf("application settings are missing required key %q", key)
		}
	}
	if err := app.SaveSettings(settings); err != nil {
		t.Fatalf("initialize LLM settings from environment: %v", err)
	}
}

func waitForCleaning(t *testing.T, app *App, recordID int64) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Minute)
	for {
		snapshot, err := app.GetActiveCleaning()
		if err != nil {
			t.Fatalf("get active cleaning: %v", err)
		}
		if snapshot == nil {
			t.Fatalf("cleaning task %d disappeared before completion", recordID)
		}
		if snapshot.ID != recordID {
			t.Fatalf("active cleaning task ID = %d, want %d", snapshot.ID, recordID)
		}
		switch snapshot.State {
		case "DONE":
			return
		case "ERROR":
			t.Fatalf("cleaning task failed: %s", snapshot.ErrorMessage)
		case "CANCELLED":
			t.Fatal("cleaning task was cancelled")
		}
		if time.Now().After(deadline) {
			t.Fatalf("cleaning task did not finish within 5 minutes (state: %s)", snapshot.State)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func ensureArchiveExtracted(
	t *testing.T,
	archivePath string,
	destinationDirectory string,
	extractedDirectory string,
) {
	t.Helper()
	if info, err := os.Stat(extractedDirectory); err == nil {
		if !info.IsDir() {
			t.Fatalf("fixture path %q exists but is not a directory", extractedDirectory)
		}
		return
	} else if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("inspect extracted fixture: %v", err)
	}

	archive, err := os.Open(archivePath)
	if err != nil {
		t.Fatalf("open fixture archive: %v", err)
	}
	t.Cleanup(func() {
		if err := archive.Close(); err != nil {
			t.Errorf("close fixture archive: %v", err)
		}
	})

	gzipReader, err := gzip.NewReader(archive)
	if err != nil {
		t.Fatalf("open gzip stream: %v", err)
	}
	t.Cleanup(func() {
		if err := gzipReader.Close(); err != nil {
			t.Errorf("close gzip stream: %v", err)
		}
	})

	archiveReader := tar.NewReader(gzipReader)
	for {
		header, err := archiveReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("read fixture archive: %v", err)
		}

		target, err := archiveTarget(destinationDirectory, header.Name)
		if err != nil {
			t.Fatalf("invalid fixture archive entry %q: %v", header.Name, err)
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(header.Mode)); err != nil {
				t.Fatalf("create fixture directory %q: %v", target, err)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				t.Fatalf("create fixture parent directory for %q: %v", target, err)
			}
			file, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
			if err != nil {
				t.Fatalf("create fixture file %q: %v", target, err)
			}
			_, copyErr := io.Copy(file, archiveReader)
			closeErr := file.Close()
			if copyErr != nil {
				t.Fatalf("extract fixture file %q: %v", target, copyErr)
			}
			if closeErr != nil {
				t.Fatalf("close fixture file %q: %v", target, closeErr)
			}
		default:
			t.Fatalf("unsupported fixture archive entry %q (type %d)", header.Name, header.Typeflag)
		}
	}
}

func archiveTarget(destinationDirectory string, archiveName string) (string, error) {
	if filepath.IsAbs(archiveName) {
		return "", errors.New("absolute path is not allowed")
	}
	destination, err := filepath.Abs(destinationDirectory)
	if err != nil {
		return "", fmt.Errorf("resolve destination: %w", err)
	}
	target := filepath.Join(destination, filepath.FromSlash(archiveName))
	relative, err := filepath.Rel(destination, target)
	if err != nil {
		return "", fmt.Errorf("resolve archive entry: %w", err)
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("path escapes destination directory")
	}
	return target, nil
}

func normalizeCandidatePath(candidatePath string) string {
	path := candidatePath
	path = filepath.ToSlash(strings.TrimSpace(path))
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return filepath.ToSlash(filepath.Clean(path))
}

func missingPathPrefixes(paths map[string]struct{}, prefixes []string) []string {
	missing := make([]string, 0)
	for _, prefix := range prefixes {
		normalizedPrefix := normalizeCandidatePath(prefix)
		matched := false
		for path := range paths {
			if strings.HasPrefix(path, normalizedPrefix) {
				matched = true
				break
			}
		}
		if !matched {
			missing = append(missing, normalizedPrefix)
		}
	}
	sort.Strings(missing)
	return missing
}

func sortedPaths(paths map[string]struct{}) []string {
	result := make([]string, 0, len(paths))
	for path := range paths {
		result = append(result, path)
	}
	sort.Strings(result)
	return result
}
