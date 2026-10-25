// Package guest is the boundary to the Go daemon running inside the iOS guest.
// Readiness comes only from a bounded daemon response from the intended
// machine; native running state never implies it.
package guest

import "context"

// Readiness is what the guest daemon has demonstrated.
type Readiness string

// Readiness values. ReadinessUnknown means the daemon has not been asked.
const (
	ReadinessUnknown     Readiness = ""
	ReadinessNotReady    Readiness = "not-ready"
	ReadinessReady       Readiness = "ready"
	ReadinessUnreachable Readiness = "unreachable"
)

// DeviceInfo is the identity the daemon reports for its guest.
type DeviceInfo struct {
	ProductType string
	Version     string
	Build       string
}

// Client talks to one machine's guest daemon.
type Client interface {
	// Ready asks the daemon for readiness within ctx's deadline.
	Ready(ctx context.Context) (Readiness, error)
	DeviceInfo(ctx context.Context) (DeviceInfo, error)
	// Shutdown asks the guest to shut down. A nil error means the daemon
	// acknowledged the request, not that the machine has stopped.
	Shutdown(ctx context.Context) error
}
