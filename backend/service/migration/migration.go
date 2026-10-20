package migration

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	appctx "ai-disk-cleanner/backend/ctx"
	migrationmodel "ai-disk-cleanner/backend/data/models/migration"
	"ai-disk-cleanner/backend/util"
)

type migrationStore interface {
	CreateMigration(context.Context, *migrationmodel.Migration) error
	GetMigration(context.Context, int64) (*migrationmodel.Migration, error)
	ListMigrations(context.Context) ([]migrationmodel.Migration, error)
	DeleteMigration(context.Context, int64) error
	MarkTrashFileMigrated(context.Context, string) error
}

// Service coordinates filesystem changes with migration persistence.
type Service struct {
	ctx     context.Context
	store   migrationStore
	symlink func(string, string) error
	steps   map[string]migrationStepStage
	mu      sync.Mutex
}

// NewService creates the migration service for the central service manager.
func NewService(store migrationStore) *Service {
	return newService(appctx.GetContext(), store)
}

func newService(ctx context.Context, store migrationStore) *Service {
	return &Service{
		ctx:     ctx,
		store:   store,
		symlink: os.Symlink,
		steps:   make(map[string]migrationStepStage),
	}
}

// CopySource copies source to destinationDirectory/name and returns the
// resolved destination path. A failed copy does not leave a partial target.
func (service *Service) CopySource(
	source string,
	destinationDirectory string,
	name string,
) (string, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	return service.copySource(source, destinationDirectory, name)
}

// DeleteSource removes source after verifying that its destination copy exists.
// Calling it again after a successful deletion is safe.
func (service *Service) DeleteSource(source string, dest string) error {
	service.mu.Lock()
	defer service.mu.Unlock()
	return service.deleteSource(source, dest)
}

// CreateLink creates the source symbolic link and persists the migration.
// Calling it again completes or reuses the same migration record.
func (service *Service) CreateLink(
	source string,
	dest string,
	name string,
) (*migrationmodel.Migration, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	return service.createLink(source, dest, name)
}

// Create copies source into destinationDirectory/name, removes source, and
// replaces source with a symbolic link. Any later failure is compensated.
func (service *Service) Create(
	source string,
	destinationDirectory string,
	name string,
) (*migrationmodel.Migration, error) {
	service.mu.Lock()
	defer service.mu.Unlock()

	source, dest, name, err := validateCreate(source, destinationDirectory, name)
	if err != nil {
		return nil, err
	}

	if err := copyPath(source, dest); err != nil {
		_ = removePath(dest)
		return nil, fmt.Errorf("copy source to destination: %w", err)
	}
	if err := removePath(source); err != nil {
		cleanupErr := removePath(dest)
		return nil, joinErrors("remove source", err, "clean destination copy", cleanupErr)
	}
	if err := service.symlink(dest, source); err != nil {
		restoreErr := restoreCopiedSource(source, dest)
		return nil, joinErrors("create symbolic link", err, "restore source", restoreErr)
	}

	record := &migrationmodel.Migration{
		Name:   name,
		Source: source,
		Dest:   dest,
	}
	if err := service.store.CreateMigration(service.ctx, record); err != nil {
		rollbackErr := restoreMigrationFilesystem(source, dest)
		return nil, joinErrors("persist migration", err, "rollback filesystem", rollbackErr)
	}
	if err := service.store.MarkTrashFileMigrated(service.ctx, source); err != nil {
		deleteRecordErr := service.store.DeleteMigration(service.ctx, record.ID)
		rollbackErr := restoreMigrationFilesystem(source, dest)
		return nil, joinErrors(
			"mark migrated trash file deleted", err,
			"delete migration record", deleteRecordErr,
			"rollback filesystem", rollbackErr,
		)
	}
	return record, nil
}

func (service *Service) List() ([]migrationmodel.Migration, error) {
	return service.store.ListMigrations(service.ctx)
}

// Restore replaces the symbolic link with the migrated data, removes the
// destination copy, and deletes the migration record.
func (service *Service) Restore(id int64) error {
	service.mu.Lock()
	defer service.mu.Unlock()

	record, err := service.store.GetMigration(service.ctx, id)
	if err != nil {
		return err
	}
	if err := validateRestore(record); err != nil {
		return err
	}

	temporary, err := temporaryRestorePath(record.Source, record.ID)
	if err != nil {
		return err
	}
	if err := copyPath(record.Dest, temporary); err != nil {
		_ = removePath(temporary)
		return fmt.Errorf("prepare restored source: %w", err)
	}

	// The temporary copy is on the source volume. Once the destination has
	// been removed, replacing the link with this copy is an atomic rename.
	if err := removePath(record.Dest); err != nil {
		_ = removePath(temporary)
		return fmt.Errorf("remove migrated destination: %w", err)
	}
	if err := os.Remove(record.Source); err != nil {
		restoreErr := copyPath(temporary, record.Dest)
		_ = removePath(temporary)
		return joinErrors("remove symbolic link", err, "restore destination", restoreErr)
	}
	if err := os.Rename(temporary, record.Source); err != nil {
		restoreDestErr := copyPath(temporary, record.Dest)
		restoreLinkErr := service.symlink(record.Dest, record.Source)
		if restoreDestErr == nil && restoreLinkErr == nil {
			_ = removePath(temporary)
		}
		return joinErrors(
			"place restored source", err,
			"restore destination", restoreDestErr,
			"restore symbolic link", restoreLinkErr,
		)
	}

	if err := service.store.DeleteMigration(service.ctx, id); err != nil {
		return fmt.Errorf("source restored but migration record could not be deleted: %w", err)
	}
	return nil
}

func validateCreate(source, destinationDirectory, name string) (string, string, string, error) {
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." || filepath.Base(name) != name {
		return "", "", "", errors.New("migration name must be a single file or directory name")
	}

	source, err := filepath.Abs(filepath.Clean(strings.TrimSpace(source)))
	if err != nil {
		return "", "", "", fmt.Errorf("resolve source path: %w", err)
	}
	if filepath.Dir(source) == source {
		return "", "", "", errors.New("a volume root cannot be migrated")
	}
	info, err := os.Lstat(source)
	if err != nil {
		return "", "", "", fmt.Errorf("inspect source: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", "", "", errors.New("source is already a symbolic link")
	}

	destinationDirectory, err = filepath.Abs(filepath.Clean(strings.TrimSpace(destinationDirectory)))
	if err != nil {
		return "", "", "", fmt.Errorf("resolve destination directory: %w", err)
	}
	directoryInfo, err := os.Stat(destinationDirectory)
	if err != nil {
		return "", "", "", fmt.Errorf("inspect destination directory: %w", err)
	}
	if !directoryInfo.IsDir() {
		return "", "", "", errors.New("destination is not a directory")
	}

	dest := filepath.Join(destinationDirectory, name)
	if samePath(source, dest) {
		return "", "", "", errors.New("source and destination must be different")
	}
	if info.IsDir() && pathContains(source, dest) {
		return "", "", "", errors.New("a directory cannot be migrated inside itself")
	}
	if _, err := os.Lstat(dest); err == nil {
		return "", "", "", errors.New("destination already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", "", "", fmt.Errorf("inspect destination: %w", err)
	}
	return source, dest, name, nil
}

func validateRestore(record *migrationmodel.Migration) error {
	info, err := os.Lstat(record.Source)
	if err != nil {
		return fmt.Errorf("inspect symbolic link: %w", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return errors.New("source is no longer a symbolic link; refusing to overwrite it")
	}
	target, err := os.Readlink(record.Source)
	if err != nil {
		return fmt.Errorf("read symbolic link: %w", err)
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(record.Source), target)
	}
	if !samePath(target, record.Dest) {
		return errors.New("symbolic link target does not match the migration record")
	}
	if _, err := os.Lstat(record.Dest); err != nil {
		return fmt.Errorf("inspect migrated destination: %w", err)
	}
	return nil
}

func temporaryRestorePath(source string, id int64) (string, error) {
	path := filepath.Join(
		filepath.Dir(source),
		fmt.Sprintf(".%s.restore-%d", filepath.Base(source), id),
	)
	if _, err := os.Lstat(path); err == nil {
		return "", errors.New("temporary restore path already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("inspect temporary restore path: %w", err)
	}
	return path, nil
}

func restoreCopiedSource(source, dest string) error {
	if err := copyPath(dest, source); err != nil {
		return err
	}
	return removePath(dest)
}

func restoreMigrationFilesystem(source, dest string) error {
	if err := os.Remove(source); err != nil {
		return fmt.Errorf("remove symbolic link: %w", err)
	}
	return restoreCopiedSource(source, dest)
}

func copyPath(source, dest string) error {
	info, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(source)
		if err != nil {
			return err
		}
		return os.Symlink(target, dest)
	}
	if info.IsDir() {
		if err := os.Mkdir(dest, info.Mode().Perm()); err != nil {
			return err
		}
		entries, err := os.ReadDir(source)
		if err != nil {
			return err
		}
		for _, entry := range entries {
			if err := copyPath(
				filepath.Join(source, entry.Name()),
				filepath.Join(dest, entry.Name()),
			); err != nil {
				return err
			}
		}
		return nil
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("unsupported source type: %s", info.Mode().Type())
	}
	return copyFile(source, dest, info.Mode().Perm())
}

func copyFile(source, dest string, mode os.FileMode) (returnErr error) {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer func() {
		if err := input.Close(); returnErr == nil && err != nil {
			returnErr = err
		}
	}()
	output, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	defer func() {
		if err := output.Close(); returnErr == nil && err != nil {
			returnErr = err
		}
	}()
	_, err = io.Copy(output, input)
	return err
}

func removePath(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
		return os.RemoveAll(path)
	}
	return os.Remove(path)
}

func pathContains(parent, child string) bool {
	relative, err := filepath.Rel(parent, child)
	return err == nil && relative != ".." &&
		!strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func samePath(left, right string) bool {
	return util.SamePath(left, right)
}

func joinErrors(label string, err error, additional ...any) error {
	if err == nil {
		return nil
	}
	message := fmt.Sprintf("%s: %v", label, err)
	for index := 0; index+1 < len(additional); index += 2 {
		other, _ := additional[index+1].(error)
		if other != nil {
			message += fmt.Sprintf("; %v: %v", additional[index], other)
		}
	}
	return errors.New(message)
}
