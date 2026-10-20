//go:build linux

package util

import (
	"errors"
	"strings"
	"testing"
)

func TestParseMountInfoDecodesMountPathsAndPreservesBindEntries(t *testing.T) {
	input := strings.Join([]string{
		"36 25 8:1 / / rw,relatime shared:1 - ext4 /dev/sda1 rw",
		"37 36 8:1 /data /mnt/a\\040b rw,relatime master:2 - ext4 /dev/sda1 rw,errors=remount-ro",
		"38 36 8:1 /data /mnt/back\\134slash rw,relatime - ext4 /dev/sda1 rw",
		"39 36 8:1 /data /mnt/line\\012break rw,relatime - ext4 /dev/sda1 rw",
		"this is a malformed row",
		"40 36 8:1 /data /mnt/a\\040b rw,relatime bind:1 - ext4 /dev/sda1 rw",
	}, "\n")

	got, err := parseMountInfo(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parseMountInfo() error = %v", err)
	}
	if len(got) != 5 {
		t.Fatalf("got %d entries, want 5 valid entries: %#v", len(got), got)
	}
	if got[1].MountPoint != "/mnt/a b" || got[1].Root != "/data" || got[1].FSType != "ext4" {
		t.Fatalf("decoded mount entry = %#v", got[1])
	}
	if got[1].OptionalFields[0] != "master:2" || got[1].SuperOptions != "rw,errors=remount-ro" {
		t.Fatalf("mount fields were not split around separator: %#v", got[1])
	}
	if got[2].MountPoint != "/mnt/back\\slash" || got[3].MountPoint != "/mnt/line\nbreak" {
		t.Fatalf("escaped mount paths were not decoded: %#v", got[2:4])
	}
	if got[4].MountPoint != got[1].MountPoint || got[4].OptionalFields[0] != "bind:1" {
		t.Fatalf("duplicate bind mount information was lost: %#v", got[4])
	}
}

func TestParseMountInfoReturnsScannerReadError(t *testing.T) {
	_, err := parseMountInfo(errorReader{})
	if err == nil {
		t.Fatal("parseMountInfo() error = nil, want read error")
	}
}

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }
