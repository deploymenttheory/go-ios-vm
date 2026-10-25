//go:build !darwin

package daemon

import (
	"fmt"
	"net"

	"github.com/deploymenttheory/go-ios-vm/internal/guest"
)

// ReadIdentity is only available on Darwin guests.
func ReadIdentity() (guest.DeviceInfo, error) {
	return guest.DeviceInfo{}, fmt.Errorf("read identity: %w", ErrUnsupported)
}

// ListenVsock is only available on Darwin guests.
func ListenVsock(uint32) (net.Listener, error) {
	return nil, fmt.Errorf("vsock: %w", ErrUnsupported)
}
