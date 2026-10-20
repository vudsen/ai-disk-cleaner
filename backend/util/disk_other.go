//go:build !windows && !linux

package util

import (
	"errors"

	"ai-disk-cleanner/backend/model"
)

func ListDisks() ([]model.DiskInfo, error) {
	return nil, errors.New("disk enumeration is currently only supported on Windows and Linux")
}
