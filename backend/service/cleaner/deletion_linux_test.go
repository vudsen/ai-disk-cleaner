//go:build linux

package cleaner

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"ai-disk-cleanner/backend/data/models/cleaningrecord"
	modelscanner "ai-disk-cleanner/backend/model/scanner"
	"ai-disk-cleanner/backend/util"
)

func TestLinuxDeletionBoundaryRejectsNestedMountAndSymlinkComponent(t *testing.T) {
	root := t.TempDir()
	targetDirectory := filepath.Join(root, "parent")
	if err := os.Mkdir(targetDirectory, 0700); err != nil {
		t.Fatal(err)
	}
	mounts := []util.MountInfo{{MountPoint: root, MajorMinor: "8:1"}, {MountPoint: filepath.Join(targetDirectory, "bind"), MajorMinor: "8:1"}}
	if err := validateLinuxDeletionBoundaryWithMounts(root, targetDirectory, mounts); err == nil {
		t.Fatal("directory containing same-device bind mount was allowed")
	}

	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "linked")); err != nil {
		t.Fatal(err)
	}
	if err := validateLinuxDeletionBoundaryWithMounts(root, filepath.Join(root, "linked"), mounts); err == nil {
		t.Fatal("symlink path component was allowed")
	}
}

func TestDeleteTrashFilesRejectsSymlinkPathComponent(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	target := filepath.Join(outside, "keep.txt")
	if err := os.WriteFile(target, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "linked")); err != nil {
		t.Fatal(err)
	}
	store := newTestStore(t)
	record := &cleaningrecord.CleaningRecord{
		ID: 1, Path: root,
		TrashFiles: []cleaningrecord.TrashFile{{Name: "target", Path: "linked/keep.txt", Size: 4}},
		TopUsages:  []cleaningrecord.DiskUsage{}, State: cleaningrecord.CLEANING_STATE_DONE,
	}
	if err := store.CreateCleaningRecord(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	service := newServiceWithScanner(context.Background(), store, nil, nil, nil)
	service.tree = &modelscanner.FileTree{RootPath: root}
	service.treeSnapshot = &CleaningTaskSnapshot{ID: record.ID, Path: root}

	failures, err := service.DeleteTrashFiles(record.ID, []string{"linked/keep.txt"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(failures) != 1 {
		t.Fatalf("failures = %#v, want one boundary failure", failures)
	}
	if _, err := os.Stat(target); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	} else if err == nil {
		return
	}
	t.Fatal("target outside the scan root was removed")
}
