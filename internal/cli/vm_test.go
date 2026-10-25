package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/deploymenttheory/go-ios-vm/internal/firmware"
	"github.com/deploymenttheory/go-ios-vm/internal/host"
	"github.com/deploymenttheory/go-ios-vm/internal/machine"
)

func newMachine(t *testing.T, lib machine.Library, name string) (*machine.Machine, *machine.Owner) {
	t.Helper()
	m, o, err := lib.Create(machine.Config{
		Name:       name,
		Identifier: []byte{1, 2, 3, 4},
		Pair:       firmware.SupportedPair,
		Hardware:   machine.Hardware{CPUs: 8, MemoryMiB: 8192, DiskSizeGB: 64, Network: "nat"},
		Files:      machine.DefaultFiles,
	}, "test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(o.Release)
	return m, o
}

func TestVMListEmptyLibrary(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "machines")
	r := runWith(t, hostEnv(), "vm", "list", "--library", dir)
	if r.code != ExitOK || r.stdout != "No machines in "+dir+".\n" || r.stderr != "" {
		t.Fatalf("%+v", r)
	}
}

func TestVMListAndInfo(t *testing.T) {
	lib := machine.Library{Dir: t.TempDir()}
	m, o := newMachine(t, lib, "phone")
	if err := os.WriteFile(filepath.Join(m.Dir, "acquire.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	a, err := m.Begin(o, machine.StageAcquire)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Complete("acquire.json"); err != nil {
		t.Fatal(err)
	}
	a, err = m.Begin(o, machine.StagePrepare)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Fail(errors.New("cloudOS manifest has no vphone600ap identity")); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Begin(o, machine.StagePatch); err != nil {
		t.Fatal(err)
	}
	if err := m.RecordNative(o, host.StateStopped); err != nil {
		t.Fatal(err)
	}
	broken, _ := newMachine(t, lib, "broken")
	if err := os.WriteFile(filepath.Join(broken.Dir, "machine.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}

	r := runWith(t, hostEnv(), "--library", lib.Dir, "vm", "list")
	if r.code != ExitOK || r.stderr != "" {
		t.Fatalf("list: %+v", r)
	}
	for _, want := range []string{"NAME", "phone   1/6", "stopped at ", "broken  -", "machine record is corrupt"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("list does not contain %q:\n%s", want, r.stdout)
		}
	}

	r = runWith(t, hostEnv(), "--library", lib.Dir, "vm", "info", "phone")
	if r.code != ExitOK || r.stderr != "" {
		t.Fatalf("info: %+v", r)
	}
	for _, want := range []string{
		"Name:       phone",
		"Identifier: 4 bytes, SHA-256 ",
		"Firmware:   iPhone17,3 iOS 27.0.1 (24A446) + cloudOS 26.4 (23E5207q)",
		"Hardware:   8 CPUs, 8192 MiB memory, 64 GB sparse disk, nat network",
		"Storage:    disk.img (absent), nvram.bin (absent), sep-storage.bin (absent), rom.bin (absent), sep-rom.bin (absent)",
		"Native:     stopped, last observed ",
		"Guest:      unknown (no daemon response is recorded)",
		"acquire     completed (1 evidence file(s) verified)",
		"prepare     failed: cloudOS manifest has no vphone600ap identity",
		"patch       running, or interrupted if no session owns the machine",
		"first-boot  pending",
	} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("info does not contain %q:\n%s", want, r.stdout)
		}
	}

	if err := os.WriteFile(filepath.Join(m.Dir, "acquire.json"), []byte(`{"changed":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	r = runWith(t, hostEnv(), "--library", lib.Dir, "vm", "info", "phone")
	if !strings.Contains(r.stdout, "acquire     not completed: stage evidence has changed") {
		t.Errorf("info after changing evidence:\n%s", r.stdout)
	}
}

func TestVMInfoErrors(t *testing.T) {
	lib := machine.Library{Dir: t.TempDir()}
	broken, _ := newMachine(t, lib, "broken")
	if err := os.WriteFile(filepath.Join(broken.Dir, "machine.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name, want string
		code       int
	}{
		{"absent", "vm info: machine not found: absent", ExitFailure},
		{"broken", "a replacement identity is never generated", ExitFailure},
		{"../escape", "invalid machine name", ExitFailure},
	}
	for _, tt := range tests {
		r := runWith(t, hostEnv(), "--library", lib.Dir, "vm", "info", tt.name)
		if r.code != tt.code || !strings.Contains(r.stderr, tt.want) || r.stdout != "" {
			t.Errorf("%s: %+v, want exit %d and %q", tt.name, r, tt.code, tt.want)
		}
	}
}
