// Package vz builds and runs a machine's research virtual machine through
// Apple's Virtualization framework: the PV=3 research hardware model, the
// machine's stable identity, the vendored OpenAVB and OpenSEP ROMs, NVRAM, SEP
// storage, a sparse disk, PL011 serial, entropy, vsock, NAT networking, the
// display, touch and keyboard. Public classes go through the pinned bindings'
// typed wrappers; private interfaces are checked by package native first.
package vz

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
)

// Screen is the guest display.
type Screen struct {
	Width, Height, PixelsPerInch int
}

// DefaultScreen is vphone-cli's iPhone display: 1290 x 2796 pixels at 460 ppi.
var DefaultScreen = Screen{Width: 1290, Height: 2796, PixelsPerInch: 460}

// MACAddress derives the machine's network address from its identifier: a
// locally administered unicast address, so the guest keeps one address and
// one DHCP lease across launches without storing another value.
func MACAddress(identifier []byte) net.HardwareAddr {
	sum := sha256.Sum256(append([]byte("go-ios-vm mac\x00"), identifier...))
	mac := net.HardwareAddr(sum[:6])
	mac[0] = mac[0]&^0x01 | 0x02 // unicast, locally administered
	return mac
}

// ErrNoDFU reports that no DFU device with the expected identity was found.
var ErrNoDFU = errors.New("no DFU device with the expected ECID")

// DFUDevice is a USB device in DFU mode as the host registry reports it.
type DFUDevice struct {
	ECID   uint64
	CPID   uint32
	Serial string
}

// ParseDFUSerial parses a DFU device's USB serial string, for example
// "SDOM:01 CPID:FE01 CPRV:00 CPFM:03 SCEP:01 BDID:90 ECID:001A2B3C4D5E6F70 IBFL:3C".
func ParseDFUSerial(serial string) (DFUDevice, error) {
	d := DFUDevice{Serial: serial}
	var haveECID bool
	for field := range strings.FieldsSeq(serial) {
		key, value, ok := strings.Cut(field, ":")
		if !ok {
			continue
		}
		switch key {
		case "ECID":
			n, err := strconv.ParseUint(value, 16, 64)
			if err != nil {
				return DFUDevice{}, fmt.Errorf("DFU serial %q: ECID: %w", serial, err)
			}
			d.ECID, haveECID = n, true
		case "CPID":
			n, err := strconv.ParseUint(value, 16, 32)
			if err != nil {
				return DFUDevice{}, fmt.Errorf("DFU serial %q: CPID: %w", serial, err)
			}
			d.CPID = uint32(n)
		}
	}
	if !haveECID {
		return DFUDevice{}, fmt.Errorf("%w: DFU serial %q has no ECID", ErrNoDFU, serial)
	}
	return d, nil
}

// MatchDFU returns the one device whose ECID is ecid. Devices with other ECIDs,
// including physical phones in DFU, are never selected.
func MatchDFU(devices []DFUDevice, ecid uint64) (DFUDevice, error) {
	var found []DFUDevice
	for _, d := range devices {
		if d.ECID == ecid {
			found = append(found, d)
		}
	}
	switch len(found) {
	case 1:
		return found[0], nil
	case 0:
		return DFUDevice{}, fmt.Errorf("%w: want ECID %016X among %d DFU device(s)", ErrNoDFU, ecid, len(devices))
	default:
		return DFUDevice{}, fmt.Errorf("%w: %d DFU devices report ECID %016X", ErrNoDFU, len(found), ecid)
	}
}
