//go:build !darwin && !linux

package firmware

import (
	"errors"
	"fmt"
)

// DirFree is unsupported on this platform.
func DirFree(string) (int64, error) {
	return 0, fmt.Errorf("free space: %w", errors.ErrUnsupported)
}
