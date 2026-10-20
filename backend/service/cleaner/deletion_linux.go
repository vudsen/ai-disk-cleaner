//go:build linux

package cleaner

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"ai-disk-cleanner/backend/util"
)

var protectedDeletionRoots = []string{"/proc", "/sys", "/dev", "/run"}

func validateLinuxDeletionBoundary(root, target string) error {
	mounts, err := util.ReadMountInfo()
	if err != nil {
		return fmt.Errorf("read mount information: %w", err)
	}
	return validateLinuxDeletionBoundaryWithMounts(root, target, mounts)
}

func validateLinuxDeletionBoundaryWithMounts(root, target string, mounts []util.MountInfo) error {
	root, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	target, err = filepath.Abs(target)
	if err != nil {
		return err
	}
	if !cleanerPathWithin(root, target) || root == target {
		return fmt.Errorf("target %q is outside the scan boundary", target)
	}
	for _, protected := range protectedDeletionRoots {
		if cleanerPathWithin(protected, target) || cleanerPathWithin(target, protected) {
			return fmt.Errorf("target %q contains a protected virtual path", target)
		}
	}
	ownerMount := ""
	for _, mount := range mounts {
		mountPath := filepath.Clean(mount.MountPoint)
		if cleanerPathWithin(mountPath, root) && len(mountPath) > len(ownerMount) {
			ownerMount = mountPath
		}
	}
	for _, mount := range mounts {
		mountPath := filepath.Clean(mount.MountPoint)
		if mountPath != ownerMount && (cleanerPathWithin(mountPath, target) || cleanerPathWithin(target, mountPath)) {
			return fmt.Errorf("target %q crosses nested mount %q", target, mountPath)
		}
	}
	relative, _ := filepath.Rel(root, target)
	current := root
	for _, component := range strings.Split(relative, string(filepath.Separator)) {
		if component == "" || component == "." {
			continue
		}
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if err != nil {
			return fmt.Errorf("inspect deletion path %q: %w", current, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("target %q contains unsupported symbolic link %q", target, current)
		}
	}
	return nil
}

func cleanerPathWithin(parent, child string) bool {
	relative, err := filepath.Rel(filepath.Clean(parent), filepath.Clean(child))
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
