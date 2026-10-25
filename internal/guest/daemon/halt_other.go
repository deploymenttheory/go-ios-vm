//go:build !(ios && cgo)

package daemon

import "fmt"

// Halt is only available in the iOS guest build.
func Halt() error { return fmt.Errorf("halt: %w", ErrUnsupported) }
