package migration

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	migrationmodel "ai-disk-cleanner/backend/data/models/migration"
	"ai-disk-cleanner/backend/util"
)

type migrationStepStage uint8

const (
	stageCopied migrationStepStage = iota + 1
	stageSourceDeleted
	stageLinkCreated
)

func (service *Service) copySource(
	source string,
	destinationDirectory string,
	name string,
) (string, error) {
	normalizedSource, dest, err := resolveCopyPaths(source, destinationDirectory, name)
	if err != nil {
		return "", err
	}
	key := migrationStepKey(normalizedSource, dest)
	if service.steps[key] >= stageCopied {
		if _, err := os.Lstat(dest); err != nil {
			return "", fmt.Errorf("inspect completed destination copy: %w", err)
		}
		return dest, nil
	}

	source, dest, _, err = validateCreate(source, destinationDirectory, name)
	if err != nil {
		return "", err
	}
	if err := copyPath(source, dest); err != nil {
		cleanupErr := removePath(dest)
		return "", joinErrors("copy source to destination", err, "clean partial destination", cleanupErr)
	}
	service.steps[key] = stageCopied
	return dest, nil
}

func (service *Service) deleteSource(source string, dest string) error {
	source, dest, err := normalizeStepPaths(source, dest)
	if err != nil {
		return err
	}
	key := migrationStepKey(source, dest)
	if service.steps[key] < stageCopied {
		return errors.New("migration copy step has not completed")
	}
	destinationInfo, err := os.Lstat(dest)
	if err != nil {
		return fmt.Errorf("inspect destination copy: %w", err)
	}
	if destinationInfo.Mode()&os.ModeSymlink != 0 {
		return errors.New("destination copy cannot be a symbolic link")
	}

	sourceInfo, err := os.Lstat(source)
	if errors.Is(err, os.ErrNotExist) {
		service.steps[key] = stageSourceDeleted
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect source: %w", err)
	}
	if sourceInfo.Mode()&os.ModeSymlink != 0 {
		return errors.New("source is already a symbolic link")
	}
	if sourceInfo.IsDir() != destinationInfo.IsDir() {
		return errors.New("source and destination copy types do not match")
	}
	if err := removePath(source); err != nil {
		return fmt.Errorf("remove source: %w", err)
	}
	service.steps[key] = stageSourceDeleted
	return nil
}

func (service *Service) createLink(
	source string,
	dest string,
	name string,
) (*migrationmodel.Migration, error) {
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." || filepath.Base(name) != name {
		return nil, errors.New("migration name must be a single file or directory name")
	}
	var err error
	source, dest, err = normalizeStepPaths(source, dest)
	if err != nil {
		return nil, err
	}
	key := migrationStepKey(source, dest)
	if service.steps[key] < stageSourceDeleted {
		return nil, errors.New("migration source deletion step has not completed")
	}
	if filepath.Base(dest) != name {
		return nil, errors.New("migration name does not match destination")
	}
	if _, err := os.Lstat(dest); err != nil {
		return nil, fmt.Errorf("inspect destination copy: %w", err)
	}

	info, err := os.Lstat(source)
	switch {
	case errors.Is(err, os.ErrNotExist):
		if err := service.symlink(dest, source); err != nil {
			return nil, fmt.Errorf("create symbolic link: %w", err)
		}
	case err != nil:
		return nil, fmt.Errorf("inspect source link: %w", err)
	case info.Mode()&os.ModeSymlink == 0:
		return nil, errors.New("source still exists and is not a symbolic link")
	default:
		target, readErr := os.Readlink(source)
		if readErr != nil {
			return nil, fmt.Errorf("read source symbolic link: %w", readErr)
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(source), target)
		}
		if !samePath(target, dest) {
			return nil, errors.New("source symbolic link points to a different destination")
		}
	}

	records, err := service.store.ListMigrations(service.ctx)
	if err != nil {
		return nil, fmt.Errorf("list migrations before persist: %w", err)
	}
	for index := range records {
		if samePath(records[index].Source, source) && samePath(records[index].Dest, dest) {
			if err := service.store.MarkTrashFileMigrated(service.ctx, source); err != nil {
				return nil, fmt.Errorf("mark migrated trash file deleted: %w", err)
			}
			service.steps[key] = stageLinkCreated
			return &records[index], nil
		}
	}

	record := &migrationmodel.Migration{Name: name, Source: source, Dest: dest}
	if err := service.store.CreateMigration(service.ctx, record); err != nil {
		return nil, fmt.Errorf("persist migration: %w", err)
	}
	if err := service.store.MarkTrashFileMigrated(service.ctx, source); err != nil {
		return nil, fmt.Errorf("mark migrated trash file deleted: %w", err)
	}
	service.steps[key] = stageLinkCreated
	return record, nil
}

func normalizeStepPaths(source string, dest string) (string, string, error) {
	if strings.TrimSpace(source) == "" {
		return "", "", errors.New("source path is required")
	}
	if strings.TrimSpace(dest) == "" {
		return "", "", errors.New("destination path is required")
	}
	source, err := filepath.Abs(filepath.Clean(strings.TrimSpace(source)))
	if err != nil {
		return "", "", fmt.Errorf("resolve source path: %w", err)
	}
	if filepath.Dir(source) == source {
		return "", "", errors.New("a volume root cannot be migrated")
	}
	dest, err = filepath.Abs(filepath.Clean(strings.TrimSpace(dest)))
	if err != nil {
		return "", "", fmt.Errorf("resolve destination path: %w", err)
	}
	if samePath(source, dest) {
		return "", "", errors.New("source and destination must be different")
	}
	return source, dest, nil
}

func resolveCopyPaths(source string, destinationDirectory string, name string) (string, string, error) {
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." || filepath.Base(name) != name {
		return "", "", errors.New("migration name must be a single file or directory name")
	}
	if strings.TrimSpace(destinationDirectory) == "" {
		return "", "", errors.New("destination directory is required")
	}
	destinationDirectory, err := filepath.Abs(filepath.Clean(strings.TrimSpace(destinationDirectory)))
	if err != nil {
		return "", "", fmt.Errorf("resolve destination directory: %w", err)
	}
	return normalizeStepPaths(source, filepath.Join(destinationDirectory, name))
}

func migrationStepKey(source string, dest string) string {
	return util.PathKey(source) + "\x00" + util.PathKey(dest)
}
