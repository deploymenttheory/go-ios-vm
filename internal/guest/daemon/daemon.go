// Package daemon is the Go guest daemon's service: readiness, device
// information and graceful shutdown. Platform calls are injected, so the
// service is tested on any host and the iOS build supplies the real ones.
package daemon

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/deploymenttheory/go-ios-vm/internal/guest"
	"github.com/deploymenttheory/go-ios-vm/internal/guest/protocol"
)

// Platform is what the service asks of the guest operating system.
type Platform struct {
	// Identity reads the guest's product type, version and build.
	Identity func() (guest.DeviceInfo, error)
	// IsRoot reports whether the daemon may halt the guest.
	IsRoot func() bool
	// Halt asks the guest to shut down. On success the guest stops; an error
	// means it will not.
	Halt func() error
	// Logf records a diagnostic.
	Logf func(format string, args ...any)
}

// Service implements protocol.Service.
type Service struct {
	platform  Platform
	version   string
	startedAt time.Time
}

var _ protocol.Service = (*Service)(nil)

// New returns the service for platform.
func New(p Platform, version string) *Service {
	if p.Logf == nil {
		p.Logf = func(string, ...any) {}
	}
	return &Service{platform: p, version: version, startedAt: time.Now().UTC()}
}

// Ping reports readiness from the daemon's own checks. The daemon is ready
// only when it can read a complete guest identity; it never answers ready by
// default.
func (s *Service) Ping(context.Context) (protocol.PingResult, error) {
	res := protocol.PingResult{
		Daemon:    protocol.DaemonInfo{Version: s.version, PID: os.Getpid(), StartedAt: s.startedAt},
		CheckedAt: time.Now().UTC(),
	}
	id, err := s.platform.Identity()
	switch {
	case err != nil:
		res.Reasons = append(res.Reasons, "guest identity unreadable: "+err.Error())
	case id.ProductType == "" || id.Version == "" || id.Build == "":
		res.Reasons = append(res.Reasons, fmt.Sprintf("guest identity incomplete: %+v", id))
	}
	res.Identity = id
	res.Ready = len(res.Reasons) == 0
	return res, nil
}

// DeviceInfo returns the guest's identity as the guest reports it.
func (s *Service) DeviceInfo(context.Context) (guest.DeviceInfo, error) {
	id, err := s.platform.Identity()
	if err != nil {
		return guest.DeviceInfo{}, &protocol.Error{Code: protocol.CodeOperationFailed, Message: "read guest identity: " + err.Error()}
	}
	return id, nil
}

// Shutdown validates the request; the returned action halts the guest after
// the acknowledgment has been sent.
func (s *Service) Shutdown(_ context.Context, p protocol.ShutdownParams) (protocol.ShutdownResult, func(), error) {
	if !p.Force {
		return protocol.ShutdownResult{}, nil, &protocol.Error{Code: protocol.CodeInvalidRequest, Message: "pass force: true to shut down the guest"}
	}
	if !s.platform.IsRoot() {
		return protocol.ShutdownResult{}, nil, &protocol.Error{Code: protocol.CodeOperationFailed, Message: "shutdown requires root"}
	}
	if s.platform.Halt == nil {
		return protocol.ShutdownResult{}, nil, &protocol.Error{Code: protocol.CodeOperationFailed, Message: "shutdown is not available on this platform"}
	}
	halt := func() {
		if err := s.platform.Halt(); err != nil {
			// The acknowledgment is already sent; the host sees the machine keep
			// running and falls back to its bounded native stop.
			s.platform.Logf("go-ios-vm-guestd: halt failed after acknowledgment: %v", err)
		}
	}
	return protocol.ShutdownResult{Accepted: true, Completion: "the virtual machine stops"}, halt, nil
}

// ErrUnsupported reports a platform call this build cannot make.
var ErrUnsupported = errors.New("not supported on this platform")
