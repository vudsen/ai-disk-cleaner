//go:build linux

package scanner

import (
	"errors"
	"fmt"
	"io"
	"os"
)

func validateScanRoot(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("symbolic link scan roots are not supported")
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := file.Readdirnames(1); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}
