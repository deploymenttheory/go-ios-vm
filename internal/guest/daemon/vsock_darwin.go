package daemon

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"sync"
	"syscall"
	"unsafe"
)

// Darwin's vsock constants (<sys/vsock.h>).
const (
	afVSOCK      = 40
	vmaddrCIDAny = 0xFFFFFFFF
)

// sockaddrVM is struct sockaddr_vm.
type sockaddrVM struct {
	Len       uint8
	Family    uint8
	Reserved1 uint16
	Port      uint32
	CID       uint32
}

// Addr is a vsock address.
type Addr struct{ CID, Port uint32 }

// Network returns "vsock".
func (Addr) Network() string { return "vsock" }

func (a Addr) String() string {
	return strconv.FormatUint(uint64(a.CID), 10) + ":" + strconv.FormatUint(uint64(a.Port), 10)
}

// ListenVsock listens for host connections on a vsock port of the guest.
func ListenVsock(port uint32) (net.Listener, error) {
	fd, err := syscall.Socket(afVSOCK, syscall.SOCK_STREAM, 0)
	if err != nil {
		return nil, fmt.Errorf("vsock socket: %w", err)
	}
	syscall.CloseOnExec(fd)
	sa := sockaddrVM{Len: uint8(unsafe.Sizeof(sockaddrVM{})), Family: afVSOCK, Port: port, CID: vmaddrCIDAny}
	if _, _, e := syscall.Syscall(syscall.SYS_BIND, uintptr(fd), uintptr(unsafe.Pointer(&sa)), unsafe.Sizeof(sa)); e != 0 {
		_ = syscall.Close(fd)
		return nil, fmt.Errorf("vsock bind port %d: %w", port, e)
	}
	if err := syscall.Listen(fd, 128); err != nil {
		_ = syscall.Close(fd)
		return nil, fmt.Errorf("vsock listen: %w", err)
	}
	return &vsockListener{fd: fd, addr: Addr{CID: vmaddrCIDAny, Port: port}}, nil
}

type vsockListener struct {
	fd   int
	addr Addr
	once sync.Once
}

// Accept blocks in accept(2); Close shuts the socket down to wake it.
func (l *vsockListener) Accept() (net.Conn, error) {
	var sa sockaddrVM
	n := uint32(unsafe.Sizeof(sa))
	for {
		r, _, e := syscall.Syscall(syscall.SYS_ACCEPT, uintptr(l.fd), uintptr(unsafe.Pointer(&sa)), uintptr(unsafe.Pointer(&n)))
		if e == syscall.EINTR {
			continue
		}
		if e != 0 {
			if errors.Is(e, syscall.EBADF) || errors.Is(e, syscall.EINVAL) || errors.Is(e, syscall.ECONNABORTED) {
				return nil, net.ErrClosed
			}
			return nil, fmt.Errorf("vsock accept: %w", e)
		}
		fd := int(r)
		syscall.CloseOnExec(fd)
		if err := syscall.SetNonblock(fd, true); err != nil {
			_ = syscall.Close(fd)
			return nil, fmt.Errorf("vsock accept: %w", err)
		}
		return &vsockConn{File: os.NewFile(uintptr(fd), "vsock"), local: l.addr, remote: Addr{CID: sa.CID, Port: sa.Port}}, nil
	}
}

func (l *vsockListener) Close() error {
	var err error
	l.once.Do(func() {
		_ = syscall.Shutdown(l.fd, syscall.SHUT_RDWR)
		err = syscall.Close(l.fd)
	})
	if err != nil {
		return fmt.Errorf("vsock close: %w", err)
	}
	return nil
}

func (l *vsockListener) Addr() net.Addr { return l.addr }

// vsockConn is an accepted connection. The nonblocking descriptor is served by
// the runtime poller, so deadlines work.
type vsockConn struct {
	*os.File
	local, remote Addr
}

func (c *vsockConn) LocalAddr() net.Addr  { return c.local }
func (c *vsockConn) RemoteAddr() net.Addr { return c.remote }
