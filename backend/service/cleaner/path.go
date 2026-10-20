package cleaner

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"ai-disk-cleanner/backend/util"
)

func samePath(left string, right string) bool {
	leftPath, leftErr := filepath.Abs(left)
	rightPath, rightErr := filepath.Abs(right)
	return leftErr == nil && rightErr == nil && util.SamePath(leftPath, rightPath)
}

func validateDirectory(directoryPath string) (string, error) {
	if strings.TrimSpace(directoryPath) == "" {
		return "", errors.New("start cleaning: directory path is empty")
	}
	absolutePath, err := filepath.Abs(directoryPath)
	if err != nil {
		return "", fmt.Errorf("start cleaning: resolve directory %q: %w", directoryPath, err)
	}
	info, err := os.Stat(absolutePath)
	if err != nil {
		return "", fmt.Errorf("start cleaning: stat directory %q: %w", absolutePath, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("start cleaning: path %q is not a directory", absolutePath)
	}
	return absolutePath, nil
}

func toAbsPath(rootPath string, candidatePath string) (string, error) {
	root, err := filepath.Abs(rootPath)
	if err != nil {
		return "", err
	}
	target := candidatePath
	if !filepath.IsAbs(target) {
		target = filepath.Join(root, target)
	}
	target, err = filepath.Abs(target)
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(root, target)
	if err != nil || relative == "." || relative == ".." ||
		strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("delete trash file: path %q is outside scan root", candidatePath)
	}
	return target, nil
}
