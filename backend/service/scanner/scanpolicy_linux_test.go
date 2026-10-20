//go:build linux

package scanner

import (
	"testing"

	"ai-disk-cleanner/backend/util"
)

func TestLinuxScanPolicySkipsVirtualAndNestedMountsByPathBoundary(t *testing.T) {
	mounts := []util.MountInfo{
		{MountPoint: "/", MajorMinor: "8:1", FSType: "ext4"},
		{MountPoint: "/proc", MajorMinor: "0:5", FSType: "proc"},
		{MountPoint: "/home/user/mnt", MajorMinor: "8:1", FSType: "ext4", Root: "/data"},
	}
	policy, err := newLinuxScanPolicy("/home/user", mounts)
	if err != nil {
		t.Fatalf("newLinuxScanPolicy() error = %v", err)
	}
	if !policy.ignoreDirectory("proc", "/proc") {
		t.Fatal("/proc should be excluded")
	}
	if policy.ignoreDirectory("proc", "/home/user/project/proc") {
		t.Fatal("project directory named proc must remain included")
	}
	if !policy.ignoreDirectory("mnt", "/home/user/mnt") {
		t.Fatal("same-device nested bind mount should be excluded")
	}
	if got := policy.summary().ExcludedCount; got != 2 {
		t.Fatalf("ExcludedCount = %d, want 2", got)
	}
}

func TestLinuxScanPolicyAllowsExplicitMountRootAndExcludesItsChildren(t *testing.T) {
	mounts := []util.MountInfo{
		{MountPoint: "/", MajorMinor: "8:1", FSType: "ext4"},
		{MountPoint: "/home", MajorMinor: "8:1", FSType: "ext4", Root: "/home"},
		{MountPoint: "/home/data", MajorMinor: "8:2", FSType: "xfs"},
	}
	policy, err := newLinuxScanPolicy("/home", mounts)
	if err != nil {
		t.Fatal(err)
	}
	if policy.ignoreDirectory("home", "/home") {
		t.Fatal("explicit mount root must be scanned")
	}
	if !policy.ignoreDirectory("data", "/home/data") {
		t.Fatal("nested mount must be excluded")
	}
}

func TestLinuxScanPolicyRejectsVirtualSystemRoots(t *testing.T) {
	_, err := newLinuxScanPolicy("/proc/1", []util.MountInfo{{MountPoint: "/", FSType: "ext4"}})
	if err == nil {
		t.Fatal("expected virtual system root to be rejected")
	}
}
