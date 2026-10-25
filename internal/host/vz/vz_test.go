package vz

import (
	"errors"
	"testing"
)

func TestMACAddressIsStableLocalUnicast(t *testing.T) {
	a := MACAddress([]byte("identity-a"))
	if a.String() != MACAddress([]byte("identity-a")).String() {
		t.Fatal("MAC address is not stable for one identity")
	}
	if a.String() == MACAddress([]byte("identity-b")).String() {
		t.Fatal("two identities share a MAC address")
	}
	if a[0]&0x01 != 0 || a[0]&0x02 == 0 || len(a) != 6 {
		t.Fatalf("%s is not a locally administered unicast address", a)
	}
}

func TestParseDFUSerial(t *testing.T) {
	d, err := ParseDFUSerial("SDOM:01 CPID:FE01 CPRV:00 CPFM:03 SCEP:01 BDID:90 ECID:001A2B3C4D5E6F70 IBFL:3C SRTG:[iBoot-12345]")
	if err != nil || d.ECID != 0x001A2B3C4D5E6F70 || d.CPID != 0xFE01 {
		t.Fatalf("ParseDFUSerial = %+v, %v", d, err)
	}
	for _, bad := range []string{"", "CPID:FE01 BDID:90", "ECID:not-hex", "CPID:zz ECID:01"} {
		if _, err := ParseDFUSerial(bad); err == nil {
			t.Errorf("ParseDFUSerial(%q) succeeded", bad)
		}
	}
}

func TestMatchDFUSelectsOnlyTheStoredIdentity(t *testing.T) {
	devices := []DFUDevice{{ECID: 1, CPID: 0x8140}, {ECID: 0xABC, CPID: 0xFE01}, {ECID: 2}}
	d, err := MatchDFU(devices, 0xABC)
	if err != nil || d.ECID != 0xABC {
		t.Fatalf("MatchDFU = %+v, %v", d, err)
	}
	if _, err := MatchDFU(devices, 0xDEF); !errors.Is(err, ErrNoDFU) {
		t.Errorf("absent ECID: %v", err)
	}
	if _, err := MatchDFU(append(devices, DFUDevice{ECID: 0xABC}), 0xABC); !errors.Is(err, ErrNoDFU) {
		t.Errorf("duplicate ECID: %v", err)
	}
	if _, err := MatchDFU(nil, 0xABC); !errors.Is(err, ErrNoDFU) {
		t.Errorf("no devices: %v", err)
	}
}
