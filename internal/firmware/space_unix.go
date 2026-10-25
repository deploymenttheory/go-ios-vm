//go:build darwin || linux

package firmware

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// DirFree reports the free space available to this user on the file system
// holding dir, or its nearest existing parent.
func DirFree(dir string) (int64, error) {
	p := filepath.Clean(dir)
	for {
		if _, err := os.Stat(p); err == nil {
			break
		}
		parent := filepath.Dir(p)
		if parent == p {
			return 0, fmt.Errorf("%w: no existing parent of %s", ErrNoSpace, dir)
		}
		p = parent
	}
	var st syscall.Statfs_t
	if err := syscall.Statfs(p, &st); err != nil {
		return 0, fmt.Errorf("statfs %s: %w", p, err)
	}
	return int64(st.Bavail) * int64(st.Bsize), nil //nolint:gosec,unconvert // block counts fit in int64
}
