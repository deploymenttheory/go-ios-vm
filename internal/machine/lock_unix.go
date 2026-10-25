//go:build darwin || linux

package machine

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

var errWouldBlock = errors.New("lock held")

func lockExclusive(f *os.File) error {
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) //nolint:gosec // file descriptors fit in int
		switch {
		case err == nil:
			return nil
		case errors.Is(err, syscall.EINTR):
			continue
		case errors.Is(err, syscall.EWOULDBLOCK):
			return errWouldBlock
		default:
			return fmt.Errorf("flock: %w", err)
		}
	}
}
