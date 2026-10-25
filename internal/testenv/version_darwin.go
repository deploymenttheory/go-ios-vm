package testenv

import (
	"fmt"
	"strconv"
	"strings"
	"syscall"
)

func checkMacOS(minMajor int) error {
	version, err := syscall.Sysctl("kern.osproductversion")
	if err != nil {
		return fmt.Errorf("read macOS version: %w", err)
	}
	major, _, _ := strings.Cut(version, ".")
	if n, err := strconv.Atoi(major); err != nil || n < minMajor {
		return fmt.Errorf("%w: running macOS %s", ErrUnmet, version)
	}
	return nil
}
