//go:build linux

package cleaningrecord

import "testing"

func TestTrashFileSourceMatchPreservesLinuxCase(t *testing.T) {
	if trashFileMatchesSource("/tmp/Cache", "file.txt", "/tmp/cache/file.txt") {
		t.Fatal("Linux paths differing by case must not match")
	}
	if !trashFileMatchesSource("/tmp/Cache", "file.txt", "/tmp/Cache/file.txt") {
		t.Fatal("identical Linux paths must match")
	}
}
