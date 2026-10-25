// Command go-ios-vm-guestd is the Go guest daemon. It runs inside the iOS
// guest and answers the host's readiness, device-information and shutdown
// requests over vsock.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/deploymenttheory/go-ios-vm/internal/buildinfo"
	"github.com/deploymenttheory/go-ios-vm/internal/guest/daemon"
	"github.com/deploymenttheory/go-ios-vm/internal/guest/protocol"
)

func main() {
	listen := flag.String("listen", "vsock:"+strconv.Itoa(protocol.Port), "vsock:<port>, or tcp:<host:port> for local testing")
	flag.Parse()
	l, err := listenOn(*listen)
	if err != nil {
		log.Fatalf("go-ios-vm-guestd: %v", err)
	}
	version := buildinfo.Read().Version
	svc := daemon.New(daemon.Platform{
		Identity: daemon.ReadIdentity,
		IsRoot:   func() bool { return os.Geteuid() == 0 },
		Halt:     daemon.Halt,
		Logf:     log.Printf,
	}, version)
	srv := &http.Server{
		Handler:           protocol.NewHandler(svc),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		MaxHeaderBytes:    64 << 10,
	}
	log.Printf("go-ios-vm-guestd %s listening on %s %s", version, l.Addr().Network(), l.Addr())
	if err := srv.Serve(l); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("go-ios-vm-guestd: %v", err)
	}
}

func listenOn(spec string) (net.Listener, error) {
	kind, addr, ok := strings.Cut(spec, ":")
	if !ok {
		return nil, fmt.Errorf("listen %q: want vsock:<port> or tcp:<host:port>", spec) //nolint:err113 // usage
	}
	switch kind {
	case "vsock":
		port, err := strconv.ParseUint(addr, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("vsock port %q: %w", addr, err)
		}
		return daemon.ListenVsock(uint32(port))
	case "tcp":
		l, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", addr)
		if err != nil {
			return nil, fmt.Errorf("listen: %w", err)
		}
		return l, nil
	default:
		return nil, fmt.Errorf("listen %q: unknown kind %q", spec, kind) //nolint:err113 // usage
	}
}
