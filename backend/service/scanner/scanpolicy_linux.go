//go:build linux

package scanner

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	modelscanner "ai-disk-cleanner/backend/model/scanner"
	"ai-disk-cleanner/backend/util"
)

type scanPolicy struct {
	ignoreDirectory func(name, path string) bool
	ignoreFile      func(name string) bool
	summary         func() modelscanner.ScanSummary
}

var virtualSystemRoots = []string{"/proc", "/sys", "/dev", "/run"}

func buildScanPolicy(ctx context.Context, root string) (scanPolicy, error) {
	mounts, err := util.ReadMountInfo()
	if err != nil {
		return scanPolicy{}, fmt.Errorf("read mount information: %w", err)
	}
	return newLinuxScanPolicyWithContext(ctx, root, mounts)
}

func newLinuxScanPolicy(root string, mounts []util.MountInfo) (scanPolicy, error) {
	return newLinuxScanPolicyWithContext(context.Background(), root, mounts)
}

func newLinuxScanPolicyWithContext(ctx context.Context, root string, mounts []util.MountInfo) (scanPolicy, error) {
	root = filepath.Clean(root)
	if !filepath.IsAbs(root) {
		return scanPolicy{}, fmt.Errorf("scan root %q is not absolute", root)
	}
	for _, virtualRoot := range virtualSystemRoots {
		if pathWithin(virtualRoot, root) {
			return scanPolicy{}, fmt.Errorf("scan path %q is a protected virtual system path", root)
		}
	}

	ownedMount := ""
	for _, entry := range mounts {
		mountPath := filepath.Clean(entry.MountPoint)
		if pathWithin(mountPath, root) && len(mountPath) > len(ownedMount) {
			ownedMount = mountPath
		}
	}
	if ownedMount == "" {
		return scanPolicy{}, fmt.Errorf("scan path %q is not covered by a mount", root)
	}

	nestedMounts := make(map[string]struct{})
	for _, entry := range mounts {
		mountPath := filepath.Clean(entry.MountPoint)
		if mountPath != root && pathWithin(root, mountPath) {
			nestedMounts[mountPath] = struct{}{}
		}
	}
	type policyState struct {
		sync.Mutex
		excluded int64
	}
	state := &policyState{}
	return scanPolicy{
		ignoreDirectory: func(_ string, path string) bool {
			if ctx.Err() != nil {
				return true
			}
			path = filepath.Clean(path)
			if _, excluded := nestedMounts[path]; excluded {
				state.Lock()
				state.excluded++
				state.Unlock()
				return true
			}
			for _, virtualRoot := range virtualSystemRoots {
				if pathWithin(virtualRoot, path) {
					state.Lock()
					state.excluded++
					state.Unlock()
					return true
				}
			}
			return false
		},
		ignoreFile: func(string) bool { return ctx.Err() != nil },
		summary: func() modelscanner.ScanSummary {
			state.Lock()
			defer state.Unlock()
			return modelscanner.ScanSummary{ExcludedCount: state.excluded}
		},
	}, nil
}

func pathWithin(parent, child string) bool {
	relative, err := filepath.Rel(filepath.Clean(parent), filepath.Clean(child))
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
