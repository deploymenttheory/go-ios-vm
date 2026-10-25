//go:build !darwin && !linux

package flock

import (
	"errors"
	"fmt"
	"os"
)

// TryLock is unsupported on this platform.
func TryLock(string) (*os.File, error) {
	return nil, fmt.Errorf("file locks: %w", errors.ErrUnsupported)
}
