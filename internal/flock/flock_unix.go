//go:build darwin || linux

package flock

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

// TryLock opens path, creating it if needed without changing its contents,
// and takes an exclusive lock without waiting. Closing the returned file
// releases the lock.
func TryLock(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600) //nolint:gosec // callers pass their own lock file
	if err != nil {
		return nil, fmt.Errorf("open lock: %w", err)
	}
	for {
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) //nolint:gosec // file descriptors fit in int
		if !errors.Is(err, syscall.EINTR) {
			break
		}
	}
	switch {
	case err == nil:
		return f, nil
	case errors.Is(err, syscall.EWOULDBLOCK):
		_ = f.Close()
		return nil, ErrHeld
	default:
		_ = f.Close()
		return nil, fmt.Errorf("flock: %w", err)
	}
}
