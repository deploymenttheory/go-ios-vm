// Package flock takes exclusive advisory file locks that the operating system
// releases when the holding process exits.
package flock

import "errors"

// ErrHeld reports a lock held by another open file description.
var ErrHeld = errors.New("lock held by another process")
