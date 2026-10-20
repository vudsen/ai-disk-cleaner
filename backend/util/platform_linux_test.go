//go:build linux

package util

import (
	"os"
	"testing"
)

func TestLinuxPlatformExports(t *testing.T) {
	disks, err := ListDisks()
	if err != nil {
		t.Fatalf("ListDisks() error = %v", err)
	}
	if disks == nil {
		t.Fatal("ListDisks() returned a nil slice")
	}

	if got, want := IsRunningAsAdministrator(), os.Geteuid() == 0; got != want {
		t.Fatalf("IsRunningAsAdministrator() = %v, want %v", got, want)
	}
}
