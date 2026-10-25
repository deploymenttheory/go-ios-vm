// Package restore defines the contract for restoring prepared firmware onto a
// created virtual device over its DFU endpoint.
package restore

import (
	"context"

	"github.com/deploymenttheory/go-ios-vm/internal/progress"
)

// Request selects the device and the prepared restore tree.
type Request struct {
	// ECID is the created virtual device's identity. A restore never targets a
	// device whose observed ECID differs.
	ECID uint64
	// PreparedDir is the prepared restore tree.
	PreparedDir string
}

// Result is the evidence of a completed restore.
type Result struct {
	ECID uint64
	// FinalStatus is the restore service's final status message.
	FinalStatus string
}

// Restorer restores one device at a time. It returns a Result only when the
// device reported a successful final status; an interrupted or canceled
// restore returns an error and is never resumable at a byte offset. When
// several failures occur, the error is the first fatal one.
type Restorer interface {
	Restore(ctx context.Context, req Request, report progress.Reporter) (Result, error)
}
