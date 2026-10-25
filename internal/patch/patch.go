// Package patch defines the contract for firmware patches. A patch applies only
// where its expected original bytes match; any disagreement stops the stage and
// leaves the original input unchanged.
package patch

import (
	"context"
	"errors"
)

// ErrMismatch reports that a component's bytes or match count differ from what
// a patch expects.
var ErrMismatch = errors.New("patch precondition mismatch")

// Receipt records one applied patch set as stage evidence.
type Receipt struct {
	// Component names the patched component, for example "txm".
	Component string
	// Source identifies the ported upstream source revision of the patch set.
	Source       string
	InputSHA256  string
	OutputSHA256 string
	// Applied counts the individual patches applied.
	Applied int
}

// Patcher patches one firmware component. On error it returns no output; an
// error wrapping ErrMismatch means the input is not the expected component.
type Patcher interface {
	Patch(ctx context.Context, input []byte) (output []byte, receipt Receipt, err error)
}
