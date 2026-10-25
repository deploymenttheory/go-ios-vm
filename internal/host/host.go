// Package host is the boundary to the native Virtualization runtime on the
// macOS host. It reports what the native framework observes about a virtual
// machine and nothing about the guest operating system: a running machine may
// still be in DFU, booting or unresponsive. Guest readiness belongs to package
// guest.
package host

import "context"

// State is the native virtual machine state reported by Virtualization.
type State string

// Native states. StateUnknown means no native observation exists.
const (
	StateUnknown  State = ""
	StateStopped  State = "stopped"
	StateStarting State = "starting"
	StateRunning  State = "running"
	StatePausing  State = "pausing"
	StatePaused   State = "paused"
	StateResuming State = "resuming"
	StateStopping State = "stopping"
	StateError    State = "error"
)

// StartOptions selects how a machine starts.
type StartOptions struct {
	// ForceDFU starts the machine in its DFU recovery endpoint.
	ForceDFU bool
}

// Machine is a native virtual machine owned by this process.
//
// Methods wait for the native completion outside the machine's configured
// queue. Canceling ctx abandons the wait only; it does not cancel native work,
// whose late completion must still be reconciled through State.
type Machine interface {
	Start(ctx context.Context, opts StartOptions) error
	Stop(ctx context.Context) error
	State(ctx context.Context) (State, error)
}
