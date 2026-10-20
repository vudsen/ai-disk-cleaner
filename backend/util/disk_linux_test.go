//go:build linux

package util

import (
	"errors"
	"testing"

	"ai-disk-cleanner/backend/model"
)

func TestBuildLinuxDiskListFiltersDeduplicatesAndSortsMounts(t *testing.T) {
	entries := []mountInfo{
		{MountPoint: "/mnt/z", FSType: "ext4", Source: "/dev/z"},
		{MountPoint: "/run", FSType: "tmpfs", Source: "tmpfs"},
		{MountPoint: "/", FSType: "overlay", Source: "overlay"},
		{MountPoint: "/mnt/a", FSType: "xfs", Source: "/dev/old"},
		{MountPoint: "/mnt/a", FSType: "xfs", Source: "/dev/new"},
		{MountPoint: "/network", FSType: "nfs4", Source: "server:/share"},
		{MountPoint: "/var/lib/container", FSType: "overlay", Source: "overlay"},
	}
	stat := func(path string) (statfsInfo, error) {
		if path == "/mnt/z" {
			return statfsInfo{}, errors.New("mount disappeared")
		}
		return statfsInfo{Blocks: 100, Bfree: 20, Bavail: 10, Bsize: 512}, nil
	}

	got, err := buildLinuxDiskList(entries, stat)
	if err != nil {
		t.Fatalf("buildLinuxDiskList() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %#v, want root and one accessible ext4/xfs mount", got)
	}
	if got[0].Path != "/" || got[1].Path != "/mnt/a" {
		t.Fatalf("mount order = %#v", got)
	}
	if got[1].Name != "/dev/new (/mnt/a)" {
		t.Fatalf("duplicate path did not keep active final mount: %#v", got[1])
	}
	assertDiskCapacity(t, got[0])
}

func TestBuildLinuxDiskListUsesAvailableAndAllocatedBlocks(t *testing.T) {
	entries := []mountInfo{{MountPoint: "/", FSType: "ext4", Source: "/dev/root"}}
	got, err := buildLinuxDiskList(entries, func(string) (statfsInfo, error) {
		return statfsInfo{Blocks: 100, Bfree: 30, Bavail: 10, Bsize: 4096}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Free != 10*4096 || got[0].Used != 70*4096 || got[0].Total != 100*4096 {
		t.Fatalf("disk capacity = %#v", got)
	}
}

func TestBuildLinuxDiskListReturnsEmptyWhenNoMountCanBeRead(t *testing.T) {
	got, err := buildLinuxDiskList([]mountInfo{{MountPoint: "/", FSType: "ext4"}}, func(string) (statfsInfo, error) {
		return statfsInfo{}, errors.New("statfs failed")
	})
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || len(got) != 0 {
		t.Fatalf("disk list = %#v, want an empty non-nil slice", got)
	}
}

func assertDiskCapacity(t *testing.T, disk model.DiskInfo) {
	t.Helper()
	if disk.Total != 100*512 || disk.Free != 10*512 || disk.Used != 80*512 {
		t.Fatalf("capacity = %#v", disk)
	}
}
