//go:build linux

package util

import (
	"fmt"
	"math"
	"sort"

	"ai-disk-cleanner/backend/model"
	"golang.org/x/sys/unix"
)

func ListDisks() ([]model.DiskInfo, error) {
	entries, err := ReadMountInfo()
	if err != nil {
		return nil, fmt.Errorf("read mount information: %w", err)
	}
	return buildLinuxDiskList(entries, statfsForPath)
}

type statfsInfo struct {
	Blocks uint64
	Bfree  uint64
	Bavail uint64
	Bsize  uint64
}

func statfsForPath(path string) (statfsInfo, error) {
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return statfsInfo{}, err
	}
	if stat.Bsize <= 0 {
		return statfsInfo{}, fmt.Errorf("invalid filesystem block size %d", stat.Bsize)
	}
	return statfsInfo{Blocks: stat.Blocks, Bfree: stat.Bfree, Bavail: stat.Bavail, Bsize: uint64(stat.Bsize)}, nil
}

func buildLinuxDiskList(entries []mountInfo, statfs func(string) (statfsInfo, error)) ([]model.DiskInfo, error) {
	active := make(map[string]mountInfo, len(entries))
	for _, entry := range entries {
		if !eligibleLinuxMount(entry) {
			continue
		}
		active[entry.MountPoint] = entry
	}
	paths := make([]string, 0, len(active))
	for path := range active {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	sort.SliceStable(paths, func(i, j int) bool {
		if paths[i] == "/" {
			return paths[j] != "/"
		}
		if paths[j] == "/" {
			return false
		}
		return paths[i] < paths[j]
	})

	disks := make([]model.DiskInfo, 0, len(paths))
	for _, path := range paths {
		capacity, err := statfs(path)
		if err != nil {
			continue
		}
		total, okTotal := checkedBlockBytes(capacity.Blocks, capacity.Bsize)
		free, okFree := checkedBlockBytes(capacity.Bavail, capacity.Bsize)
		if capacity.Bfree > capacity.Blocks || capacity.Bavail > capacity.Blocks || !okTotal || !okFree {
			continue
		}
		used, okUsed := checkedBlockBytes(capacity.Blocks-capacity.Bfree, capacity.Bsize)
		if !okUsed {
			continue
		}
		entry := active[path]
		name := entry.Source
		if name == "" {
			name = path
		}
		disks = append(disks, model.DiskInfo{
			Name:  fmt.Sprintf("%s (%s)", name, path),
			Path:  path,
			Total: total,
			Free:  free,
			Used:  used,
		})
	}
	return disks, nil
}

func eligibleLinuxMount(entry mountInfo) bool {
	if entry.MountPoint == "" || entry.MountPoint[0] != '/' {
		return false
	}
	if entry.MountPoint == "/" && entry.FSType == "overlay" {
		return true
	}
	switch entry.FSType {
	case "overlay", "proc", "sysfs", "devtmpfs", "devpts", "tmpfs", "cgroup", "cgroup2",
		"securityfs", "debugfs", "tracefs", "configfs", "pstore", "mqueue", "hugetlbfs",
		"autofs", "bpf", "rpc_pipefs", "fusectl", "binfmt_misc", "nsfs", "squashfs":
		return false
	case "nfs", "nfs4", "cifs", "smb3", "9p", "ceph", "glusterfs", "davfs", "sshfs":
		return false
	}
	return true
}

func checkedBlockBytes(blocks, blockSize uint64) (uint64, bool) {
	if blockSize == 0 || blocks > math.MaxUint64/blockSize {
		return 0, false
	}
	return blocks * blockSize, true
}
