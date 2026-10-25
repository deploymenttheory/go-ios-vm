//go:build !darwin

package testenv

import "fmt"

func checkMacOS(int) error {
	return fmt.Errorf("%w: not macOS", ErrUnmet)
}
