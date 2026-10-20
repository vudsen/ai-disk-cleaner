//go:build linux

package scanner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	modelscanner "ai-disk-cleanner/backend/model/scanner"
)

func TestLinuxScanRejectsUnreadableRoot(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can read a mode-000 directory")
	}
	root := filepath.Join(t.TempDir(), "unreadable")
	if err := os.Mkdir(root, 0000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(root, 0700) })
	if _, err := scanGDUContext(t.Context(), root, nil); err == nil {
		t.Fatal("expected unreadable root to fail")
	}
}

func TestLinuxScanSummarizesUnreadableChildDirectory(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can read a mode-000 child directory")
	}
	root := t.TempDir()
	blocked := filepath.Join(root, "blocked")
	if err := os.Mkdir(blocked, 0000); err != nil {
		t.Fatal(err)
	}
	tree, err := scanGDUContext(t.Context(), root, nil)
	if err != nil {
		t.Fatalf("scanGDUContext() error = %v", err)
	}
	if tree.ScanSummary.ReadErrorCount == 0 {
		t.Fatalf("read error count = 0, want unreadable child to be reported; summary=%#v", tree.ScanSummary)
	}
	for _, warning := range tree.ScanSummary.Warnings {
		if warning.Code == "read_error" && warning.Path == blocked {
			return
		}
	}
	t.Fatalf("unreadable child warning missing for %q: %#v", blocked, tree.ScanSummary.Warnings)
}

func TestLinuxScanUsesFilesystemMetadataForLinksSparseFilesAndFIFO(t *testing.T) {
	root := t.TempDir()
	firstPath := filepath.Join(root, "first.bin")
	if err := os.WriteFile(firstPath, []byte("hard linked payload"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(firstPath, filepath.Join(root, "second-link.bin")); err != nil {
		t.Skipf("hard links are unavailable: %v", err)
	}
	if err := os.Symlink(firstPath, filepath.Join(root, "directory-link")); err != nil {
		t.Skipf("symbolic links are unavailable: %v", err)
	}
	sparsePath := filepath.Join(root, "sparse.bin")
	sparse, err := os.Create(sparsePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := sparse.Truncate(8 << 20); err != nil {
		_ = sparse.Close()
		t.Fatal(err)
	}
	if err := sparse.Close(); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(root, "named-pipe"), 0600); err != nil {
		t.Skipf("FIFO creation is unavailable: %v", err)
	}

	tree, err := scanGDUContext(t.Context(), root, nil)
	if err != nil {
		t.Fatalf("scanGDUContext() error = %v", err)
	}
	first, err := tree.FindNode("first.bin")
	if err != nil {
		t.Fatal(err)
	}
	second, err := tree.FindNode("second-link.bin")
	if err != nil {
		t.Fatal(err)
	}
	if !first.HardLink || !second.HardLink || first.Inode == 0 || first.Inode != second.Inode {
		t.Fatalf("hard link metadata was not preserved: first=%#v second=%#v", first, second)
	}
	link, err := tree.FindNode("directory-link")
	if err != nil {
		t.Fatal(err)
	}
	if !link.NotRegular || link.IsDir() {
		t.Fatalf("symlink was not represented as a non-directory item: %#v", link)
	}
	sparseNode, err := tree.FindNode("sparse.bin")
	if err != nil {
		t.Fatal(err)
	}
	if sparseNode.ApparentSize <= sparseNode.DiskSize {
		t.Fatalf("sparse file sizes are not distinguished: apparent=%d disk=%d", sparseNode.ApparentSize, sparseNode.DiskSize)
	}
	if _, err := tree.FindNode("named-pipe"); err != nil {
		t.Fatalf("scan did not retain FIFO entry: %v", err)
	}
	if tree.Root.ApparentSize != first.ApparentSize+sparseNode.ApparentSize+link.ApparentSize {
		t.Fatalf("hard-link apparent size was counted more than once: root=%d first=%d sparse=%d symlink=%d", tree.Root.ApparentSize, first.ApparentSize, sparseNode.ApparentSize, link.ApparentSize)
	}
}

func TestLinuxScanObservesCancellationDuringDirectoryWalk(t *testing.T) {
	root := t.TempDir()
	for directoryIndex := 0; directoryIndex < 100; directoryIndex++ {
		directory := filepath.Join(root, fmt.Sprintf("directory-%03d", directoryIndex))
		if err := os.Mkdir(directory, 0700); err != nil {
			t.Fatal(err)
		}
		for fileIndex := 0; fileIndex < 20; fileIndex++ {
			path := filepath.Join(directory, fmt.Sprintf("file-%03d", fileIndex))
			if err := os.WriteFile(path, []byte("scan cancellation fixture"), 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var callbackCount atomic.Int32
	_, err := scanGDUContext(ctx, root, func(modelscanner.ScanProgress) {
		if callbackCount.Add(1) == 1 {
			cancel()
		}
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("scanGDUContext() error = %v, want context.Canceled", err)
	}
	callbacksAtReturn := callbackCount.Load()
	time.Sleep(20 * time.Millisecond)
	if got := callbackCount.Load(); got != callbacksAtReturn {
		t.Fatalf("progress callbacks continued after return: at return %d, later %d", callbacksAtReturn, got)
	}
}
