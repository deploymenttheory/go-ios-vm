package vz

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/deploymenttheory/go-bindings-macosplatform/bindings/runtime/obj"
	"github.com/deploymenttheory/go-bindings-macosplatform/bindings/runtime/purego"

	"github.com/deploymenttheory/go-ios-vm/internal/assets"
	"github.com/deploymenttheory/go-ios-vm/internal/firmware"
	"github.com/deploymenttheory/go-ios-vm/internal/host"
	"github.com/deploymenttheory/go-ios-vm/internal/machine"
	"github.com/deploymenttheory/go-ios-vm/internal/testenv"
)

func requireNative(t *testing.T) {
	t.Helper()
	testenv.Require(t, testenv.Native, testenv.AppleSilicon(27))
}

// newMachine creates a machine with a native identifier in a temporary library.
func newMachine(t *testing.T) (*machine.Machine, *machine.Owner) {
	t.Helper()
	id, err := NewIdentifier()
	if err != nil {
		t.Fatal(err)
	}
	m, o, err := machine.Library{Dir: t.TempDir()}.Create(machine.Config{
		Name: "phone", Identifier: id, Pair: firmware.SupportedPair,
		Hardware: machine.Hardware{CPUs: 8, MemoryMiB: 8192, DiskSizeGB: 64, Network: "nat"},
		Files:    machine.DefaultFiles,
	}, "test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(o.Release)
	return m, o
}

func TestNativeIdentifierAndECIDAreStable(t *testing.T) {
	requireNative(t)
	id, err := NewIdentifier()
	if err != nil {
		t.Fatal(err)
	}
	first, err := ECID(id)
	if err != nil {
		t.Fatal(err)
	}
	again, err := ECID(append([]byte(nil), id...))
	if err != nil || again != first {
		t.Fatalf("ECID from the reloaded identifier = %016X, %v; want %016X", again, err, first)
	}
	other, err := NewIdentifier()
	if err != nil {
		t.Fatal(err)
	}
	if e, _ := ECID(other); e == first {
		t.Fatal("two new identifiers share an ECID")
	}
	t.Logf("identifier %d bytes, ECID %016X", len(id), first)
	if _, err := ECID([]byte("not an identifier")); !errors.Is(err, ErrIdentifier) {
		t.Fatalf("garbage identifier: %v", err)
	}
}

// TestNativeProvisionCreatesAndReusesStorage provisions a machine and records
// whether this process may create research NVRAM.
func TestNativeProvisionCreatesAndReusesStorage(t *testing.T) {
	requireNative(t)
	m, o := newMachine(t)
	err := Provision(m, o)
	path := func(name string) string { return filepath.Join(m.Dir, name) }

	lock, lerr := assets.Load()
	if lerr != nil {
		t.Fatal(lerr)
	}
	for file, id := range map[string]string{m.Config.Files.ROM: "openavb-rom", m.Config.Files.SEPROM: "opensep-rom"} {
		a, _ := lock.Asset(id)
		if verr := assets.VerifyFile(a, path(file)); verr != nil {
			t.Errorf("%s: %v", file, verr)
		}
	}
	var st syscall.Stat_t
	if serr := syscall.Stat(path(m.Config.Files.Disk), &st); serr != nil {
		t.Fatal(serr)
	}
	if st.Size != 64_000_000_000 || st.Blocks*512 > 1<<20 {
		t.Errorf("disk: logical %d bytes, %d allocated; want a 64 GB sparse file", st.Size, st.Blocks*512)
	}
	if err != nil {
		if !errors.Is(err, ErrUnsupportedModel) {
			t.Fatalf("Provision = %v", err)
		}
		if _, serr := os.Stat(path(m.Config.Files.NVRAM)); !errors.Is(serr, os.ErrNotExist) {
			t.Errorf("failed NVRAM creation left %s", m.Config.Files.NVRAM)
		}
		t.Logf("NVRAM not created for this process: %v", err)
		return
	}
	info, err := os.Stat(path(m.Config.Files.NVRAM))
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("NVRAM created: %d bytes", info.Size())
	before := map[string]time.Time{}
	for _, f := range []string{m.Config.Files.ROM, m.Config.Files.SEPROM, m.Config.Files.Disk, m.Config.Files.NVRAM} {
		info, _ := os.Stat(path(f))
		before[f] = info.ModTime()
	}
	if err := Provision(m, o); err != nil {
		t.Fatalf("second Provision: %v", err)
	}
	for f, mt := range before {
		if info, _ := os.Stat(path(f)); !info.ModTime().Equal(mt) {
			t.Errorf("second Provision modified %s", f)
		}
	}
}

func TestNativeProvisionRefusesChangedStorage(t *testing.T) {
	requireNative(t)
	m, o := newMachine(t)
	rom := filepath.Join(m.Dir, m.Config.Files.ROM)
	if err := os.WriteFile(rom, []byte("not OpenAVB"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Provision(m, o); !errors.Is(err, assets.ErrMismatch) {
		t.Fatalf("Provision over a changed ROM = %v, want ErrMismatch", err)
	}
	if data, _ := os.ReadFile(rom); string(data) != "not OpenAVB" {
		t.Fatal("Provision overwrote an existing ROM copy")
	}
	if err := os.Remove(rom); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(m.Dir, m.Config.Files.Disk), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := Provision(m, o); !errors.Is(err, ErrStorage) {
		t.Fatalf("Provision with a directory as the disk = %v, want ErrStorage", err)
	}
	o.Release()
	if err := Provision(m, o); !errors.Is(err, machine.ErrNotOwner) {
		t.Fatalf("Provision without ownership = %v", err)
	}
}

// TestNativeBuildReportsValidation builds the configuration and records
// Virtualization's verdict; a rejection must surface as an error and leave the
// machine's records untouched.
func TestNativeBuildReportsValidation(t *testing.T) {
	requireNative(t)
	m, o := newMachine(t)
	if _, err := Build(m, DefaultScreen); !errors.Is(err, ErrStorage) {
		t.Fatalf("Build before provisioning = %v, want ErrStorage", err)
	}
	if err := Provision(m, o); err != nil && !errors.Is(err, ErrUnsupportedModel) {
		t.Fatal(err)
	}
	// An unentitled process cannot create NVRAM; give Build a placeholder so it
	// reaches the hardware model check rather than the missing file.
	nvram := filepath.Join(m.Dir, m.Config.Files.NVRAM)
	if _, err := os.Stat(nvram); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(nvram, nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	cfg, err := Build(m, DefaultScreen)
	if err != nil {
		if !errors.Is(err, ErrInvalidConfiguration) && !errors.Is(err, ErrUnsupportedModel) {
			t.Fatalf("Build = %v", err)
		}
		t.Logf("configuration refused for this process: %v", err)
	} else {
		defer cfg.Close()
		t.Logf("configuration validated; ECID %016X", cfg.ECID)
	}
	if state, _, _ := m.LastNative(); state != host.StateUnknown {
		t.Fatalf("building recorded a native state %q", state)
	}
	if obs, err := m.Observe(); err != nil || len(obs.Stages) != 0 {
		t.Fatalf("building recorded stages: %+v %v", obs, err)
	}
}

func TestNativeFindDFUReadsTheRegistry(t *testing.T) {
	requireNative(t)
	devices, err := FindDFU()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%d USB device(s) in DFU mode", len(devices))
	if _, err := MatchDFU(devices, 0xFFFF_FFFF_FFFF_FFFF); !errors.Is(err, ErrNoDFU) {
		t.Fatalf("an unknown ECID matched: %v", err)
	}
}

// TestLiveDFUStart creates a machine, starts it in forced DFU and checks that
// the DFU device the host sees carries the stored ECID. It must run as a test
// binary signed with build/vm.entitlements (scripts/live-vm-test.sh) on a host
// that admits it.
func TestLiveDFUStart(t *testing.T) {
	testenv.Require(t, testenv.Live, testenv.AppleSilicon(27), testenv.Prerequisite{
		Name: "this process may use the PV=3 research hardware model",
		Check: func() error {
			model, err := researchHardwareModel()
			if err != nil {
				return err
			}
			if !purego.Send[bool](purego.ID(obj.ID(model)), purego.RegisterName("isSupported")) {
				return fmt.Errorf("%w: refused; sign with build/vm.entitlements on a prepared host", testenv.ErrUnmet)
			}
			return nil
		},
	})
	m, o := newMachine(t)
	if err := Provision(m, o); err != nil {
		t.Fatal(err)
	}
	cfg, err := Build(m, DefaultScreen)
	if err != nil {
		t.Fatal(err)
	}
	defer cfg.Close()
	var serial bytes.Buffer
	go func() { _, _ = io.Copy(&serial, cfg.SerialOutput) }()

	vm, err := NewVM(cfg, "com.deploymenttheory.go-ios-vm.live-dfu")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := vm.Start(ctx, host.StartOptions{ForceDFU: true}); err != nil {
		t.Fatalf("start in DFU: %v", err)
	}
	defer func() {
		if err := vm.Stop(context.Background()); err != nil {
			t.Errorf("stop: %v", err)
		}
	}()
	state, _ := vm.State(ctx)
	if err := m.RecordNative(o, state); err != nil {
		t.Fatal(err)
	}
	t.Logf("native state %s, stored ECID %016X", state, cfg.ECID)

	var match DFUDevice
	deadline := time.Now().Add(30 * time.Second)
	for {
		devices, err := FindDFU()
		if err == nil {
			if match, err = MatchDFU(devices, cfg.ECID); err == nil {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("no DFU device with ECID %016X: %v", cfg.ECID, err)
		}
		time.Sleep(time.Second)
	}
	t.Logf("DFU device %q matches the stored ECID", match.Serial)
	time.Sleep(2 * time.Second)
	if err := os.WriteFile(filepath.Join(m.Dir, "serial.log"), serial.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Logf("serial output, %d bytes:\n%s", serial.Len(), serial.String())
}
