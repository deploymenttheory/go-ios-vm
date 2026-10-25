//go:build !darwin && !linux

package machine

import (
	"errors"
	"fmt"
	"os"
)

var errWouldBlock = errors.New("lock held")

func lockExclusive(*os.File) error {
	return fmt.Errorf("machine locks: %w", errors.ErrUnsupported)
}
