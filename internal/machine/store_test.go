package machine

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/deploymenttheory/go-ios-vm/internal/firmware"
	"github.com/deploymenttheory/go-ios-vm/internal/guest"
	"github.com/deploymenttheory/go-ios-vm/internal/host"
)

// helperEnv selects a helper process mode; see TestMain.
const helperEnv = "GO_IOS_VM_MACHINE_HELPER"

func TestMain(m *testing.M) {
	if mode := os.Getenv(helperEnv); mode != "" {
		runHelper(mode, os.Getenv("HELPER_LIBRARY"), os.Getenv("HELPER_MACHINE"))
		return
	}
	os.Exit(m.Run())
}

// runHelper owns a machine in a separate process, reports "ready" and then
// blocks until it is killed.
func runHelper(mode, library, name string) {
	fail := func(err error) {
		fmt.Println("error:", err)
		os.Exit(1)
	}
	m, err := Library{Dir: library}.Open(name)
	if err != nil {
		fail(err)
	}
	owner, err := m.Lock("helper " + mode)
	if err != nil {
		fail(err)
	}
	switch mode {
	case "hold":
	case "interrupt-write":
		afterTempWrite = func() {
			fmt.Println("ready")
			select {}
		}
		if err := m.RecordNative(owner, host.StateRunning); err != nil {
			fail(err)
		}
	case "interrupt-attempt":
		if _, err := m.Begin(owner, StageRestore); err != nil {
			fail(err)
		}
	}
	fmt.Println("ready")
	select {}
}

// startHelper runs mode against the machine and waits for it to be ready. The
// returned function kills the process with SIGKILL and waits for it.
func startHelper(t *testing.T, mode string, m *Machine) (pid int, kill func()) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), helperEnv+"="+mode, "HELPER_LIBRARY="+filepath.Dir(m.Dir), "HELPER_MACHINE="+m.Name)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil || line != "ready\n" {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatalf("helper %s: %q, %v", mode, line, err)
	}
	killed := false
	kill = func() {
		if !killed {
			killed = true
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}
	t.Cleanup(kill)
	return cmd.Process.Pid, kill
}

func testConfig(name string) Config {
	return Config{
		Name:       name,
		Identifier: []byte{0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef},
		Pair:       firmware.SupportedPair,
		Hardware:   Hardware{CPUs: 8, MemoryMiB: 8192, DiskSizeGB: 64, Network: "nat"},
		Files:      DefaultFiles,
	}
}

func create(t *testing.T, lib Library, name string) (*Machine, *Owner) {
	t.Helper()
	m, o, err := lib.Create(testConfig(name), "test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(o.Release)
	return m, o
}

func writeArtifact(t *testing.T, m *Machine, name, content string) {
	t.Helper()
	path := filepath.Join(m.Dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func complete(t *testing.T, m *Machine, o *Owner, stage Stage, artifacts ...string) {
	t.Helper()
	a, err := m.Begin(o, stage)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Complete(artifacts...); err != nil {
		t.Fatal(err)
	}
}

func stageState(t *testing.T, m *Machine, stage Stage) StageState {
	t.Helper()
	states, err := m.Stages()
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range states {
		if s.Stage == stage {
			return s
		}
	}
	t.Fatalf("no state for %s", stage)
	return StageState{}
}

func TestReopenPreservesIdentityAndReceipts(t *testing.T) {
	lib := Library{Dir: t.TempDir()}
	m, o := create(t, lib, "phone")
	writeArtifact(t, m, "evidence/acquire.json", `{"iphone":"24A446"}`)
	complete(t, m, o, StageAcquire, "evidence/acquire.json")
	o.Release()

	reopened, err := Library{Dir: lib.Dir}.Open("phone")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(reopened.Config.Identifier, m.Config.Identifier) || reopened.Config.Pair != firmware.SupportedPair {
		t.Fatalf("identity changed: %+v", reopened.Config)
	}
	s := stageState(t, reopened, StageAcquire)
	if !s.Completed || s.Problem != nil || len(s.Receipt.Evidence) != 1 || s.Receipt.Evidence[0].Name != "evidence/acquire.json" {
		t.Fatalf("acquire after reopening: %+v", s)
	}
	if s := stageState(t, reopened, StagePrepare); s.Completed || s.Receipt != nil || s.Latest != nil {
		t.Fatalf("prepare was never run but reports %+v", s)
	}
}

func TestCreateNeverReplacesAMachine(t *testing.T) {
	lib := Library{Dir: t.TempDir()}
	m, _ := create(t, lib, "phone")
	before, err := os.ReadFile(filepath.Join(m.Dir, configFile))
	if err != nil {
		t.Fatal(err)
	}
	other := testConfig("phone")
	other.Identifier = []byte{0xff}
	if _, _, err := lib.Create(other, "test"); !errors.Is(err, ErrExists) {
		t.Fatalf("second create: %v, want ErrExists", err)
	}
	after, err := os.ReadFile(filepath.Join(m.Dir, configFile))
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("existing configuration changed: %v", err)
	}
}

func TestCreateValidatesConfiguration(t *testing.T) {
	lib := Library{Dir: t.TempDir()}
	tests := map[string]func(*Config){
		"empty name":         func(c *Config) { c.Name = "" },
		"hidden name":        func(c *Config) { c.Name = ".phone" },
		"path in name":       func(c *Config) { c.Name = "a/b" },
		"no identifier":      func(c *Config) { c.Identifier = nil },
		"no pair":            func(c *Config) { c.Pair = firmware.Pair{} },
		"no cpus":            func(c *Config) { c.Hardware.CPUs = 0 },
		"disk outside":       func(c *Config) { c.Files.Disk = "../disk.img" },
		"absolute nvram":     func(c *Config) { c.Files.NVRAM = "/tmp/nvram.bin" },
		"reserved file":      func(c *Config) { c.Files.SEPStorage = configFile },
		"shared storage":     func(c *Config) { c.Files.NVRAM = c.Files.Disk },
		"hidden storage":     func(c *Config) { c.Files.ROM = ".rom" },
		"storage in subdirs": func(c *Config) { c.Files.SEPROM = "roms/sep.bin" },
	}
	for name, mutate := range tests {
		cfg := testConfig("phone")
		mutate(&cfg)
		if _, _, err := lib.Create(cfg, "test"); !errors.Is(err, ErrInvalidConfig) && !errors.Is(err, ErrInvalidName) {
			t.Errorf("%s: Create = %v, want a validation error", name, err)
		}
	}
	if entries, err := lib.List(); err != nil || len(entries) != 0 {
		t.Fatalf("invalid creates left %d machines (%v)", len(entries), err)
	}
}

func TestOpenReportsCorruptRecordsWithoutReplacingThem(t *testing.T) {
	lib := Library{Dir: t.TempDir()}
	m, o := create(t, lib, "phone")
	o.Release()
	path := filepath.Join(m.Dir, configFile)
	good, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		content string
		want    error
	}{
		{"truncated", string(good[:len(good)/2]), ErrCorrupt},
		{"not json", "identifier=0123", ErrCorrupt},
		{"future schema", strings.Replace(string(good), `"schema_version": 1`, `"schema_version": 2`, 1), ErrUnsupportedSchema},
		{"unknown field", strings.Replace(string(good), `"name":`, `"template": "x", "name":`, 1), ErrCorrupt},
		{"other name", strings.Replace(string(good), `"name": "phone"`, `"name": "tablet"`, 1), ErrCorrupt},
		{"no identifier", strings.Replace(string(good), `"identifier": "ASNFZ4mrze8="`, `"identifier": ""`, 1), ErrCorrupt},
	}
	for _, tt := range tests {
		if tt.content == string(good) {
			t.Fatalf("%s: fixture did not change the record", tt.name)
		}
		if err := os.WriteFile(path, []byte(tt.content), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := lib.Open("phone")
		if !errors.Is(err, tt.want) {
			t.Errorf("%s: Open = %v, want %v", tt.name, err, tt.want)
		}
		if errors.Is(tt.want, ErrCorrupt) && !strings.Contains(err.Error(), "a replacement identity is never generated") {
			t.Errorf("%s: error is not actionable: %v", tt.name, err)
		}
		if after, _ := os.ReadFile(path); string(after) != tt.content {
			t.Errorf("%s: Open rewrote the record", tt.name)
		}
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err := lib.Open("phone"); !errors.Is(err, ErrCorrupt) {
		t.Errorf("missing record in an existing directory: %v, want ErrCorrupt", err)
	}
	if _, err := lib.Open("absent"); !errors.Is(err, ErrNotFound) {
		t.Errorf("absent machine: %v, want ErrNotFound", err)
	}
}

func TestListIncludesMachinesThatCannotBeOpened(t *testing.T) {
	lib := Library{Dir: t.TempDir()}
	create(t, lib, "a")
	b, _ := create(t, lib, "b")
	if err := os.WriteFile(filepath.Join(b.Dir, configFile), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	entries, err := lib.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Name != "a" || entries[0].Err != nil || entries[1].Name != "b" || !errors.Is(entries[1].Err, ErrCorrupt) {
		t.Fatalf("entries %+v", entries)
	}
	if entries, err := (Library{Dir: filepath.Join(lib.Dir, "absent")}).List(); err != nil || entries != nil {
		t.Fatalf("missing library: %v, %v", entries, err)
	}
}

func TestChangedOrMissingEvidenceIsNotCompleted(t *testing.T) {
	lib := Library{Dir: t.TempDir()}
	m, o := create(t, lib, "phone")
	writeArtifact(t, m, "restore.log", "restore succeeded\n")
	complete(t, m, o, StageAcquire, "restore.log")

	writeArtifact(t, m, "restore.log", "restore failed\n")
	if s := stageState(t, m, StageAcquire); s.Completed || !errors.Is(s.Problem, ErrArtifactChanged) {
		t.Fatalf("changed evidence: %+v", s)
	}
	if err := os.Remove(filepath.Join(m.Dir, "restore.log")); err != nil {
		t.Fatal(err)
	}
	if s := stageState(t, m, StageAcquire); s.Completed || !errors.Is(s.Problem, ErrArtifactMissing) {
		t.Fatalf("missing evidence: %+v", s)
	}
	if err := os.WriteFile(m.receiptPath(StageAcquire), []byte(`{"schema_version":1,"stage":"acquire","evidence":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if s := stageState(t, m, StageAcquire); s.Completed || !errors.Is(s.Problem, ErrCorrupt) {
		t.Fatalf("receipt without evidence: %+v", s)
	}
	if err := os.WriteFile(m.receiptPath(StageAcquire), []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if s := stageState(t, m, StageAcquire); s.Completed || !errors.Is(s.Problem, ErrCorrupt) {
		t.Fatalf("corrupt receipt: %+v", s)
	}
	obs, err := m.Observe()
	if err != nil {
		t.Fatal(err)
	}
	if obs.Completed(StageAcquire) {
		t.Fatal("observation reports a stage completed whose receipt is corrupt")
	}
}

func TestFailedAndCanceledAttemptsHaveNoReceipt(t *testing.T) {
	lib := Library{Dir: t.TempDir()}
	m, o := create(t, lib, "phone")
	a, err := m.Begin(o, StageRestore)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Fail(errors.New("first fatal: device refused iBSS")); err != nil {
		t.Fatal(err)
	}
	s := stageState(t, m, StageRestore)
	if s.Completed || s.Receipt != nil || s.Latest.Status != StatusFailed || s.Latest.Error != "first fatal: device refused iBSS" {
		t.Fatalf("failed attempt: %+v", s)
	}
	if err := a.Complete(); !errors.Is(err, ErrInvalidRecord) {
		t.Fatalf("completing a failed attempt: %v, want ErrInvalidRecord", err)
	}
	a, err = m.Begin(o, StageInstall)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Cancel(nil); err != nil {
		t.Fatal(err)
	}
	if s := stageState(t, m, StageInstall); s.Completed || s.Latest.Status != StatusCanceled || s.Latest.Error == "" {
		t.Fatalf("canceled attempt: %+v", s)
	}
	a, err = m.Begin(o, StagePatch)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Complete(); err == nil {
		t.Fatal("completed a stage without evidence")
	}
	if err := a.Complete("../outside.json"); !errors.Is(err, ErrInvalidRecord) {
		t.Fatalf("evidence outside the machine: %v", err)
	}
	if err := a.Complete("absent.json"); !errors.Is(err, ErrArtifactMissing) {
		t.Fatalf("absent evidence: %v", err)
	}
	if s := stageState(t, m, StagePatch); s.Completed || s.Latest.Status != StatusRunning {
		t.Fatalf("rejected completion: %+v", s)
	}
}

func TestBeginWithdrawsThatStageAndLaterReceipts(t *testing.T) {
	lib := Library{Dir: t.TempDir()}
	m, o := create(t, lib, "phone")
	writeArtifact(t, m, "e.json", "{}")
	complete(t, m, o, StageAcquire, "e.json")
	complete(t, m, o, StagePrepare, "e.json")
	complete(t, m, o, StagePatch, "e.json")
	if _, err := m.Begin(o, StagePrepare); err != nil {
		t.Fatal(err)
	}
	for stage, want := range map[Stage]bool{StageAcquire: true, StagePrepare: false, StagePatch: false} {
		if s := stageState(t, m, stage); s.Completed != want {
			t.Errorf("%s completed = %v, want %v", stage, s.Completed, want)
		}
	}
}

func TestMutationsRequireTheCurrentOwner(t *testing.T) {
	lib := Library{Dir: t.TempDir()}
	m, o := create(t, lib, "phone")
	other, otherOwner := create(t, lib, "other")
	if _, err := m.Begin(otherOwner, StageAcquire); !errors.Is(err, ErrNotOwner) {
		t.Errorf("another machine's owner: %v", err)
	}
	if err := other.RecordNative(o, host.StateRunning); !errors.Is(err, ErrNotOwner) {
		t.Errorf("another machine's owner: %v", err)
	}
	a, err := m.Begin(o, StageAcquire)
	if err != nil {
		t.Fatal(err)
	}
	o.Release()
	o.Release()
	if err := a.Fail(errors.New("x")); !errors.Is(err, ErrNotOwner) {
		t.Errorf("released owner: %v", err)
	}
	if _, err := m.Begin(nil, StageAcquire); !errors.Is(err, ErrNotOwner) {
		t.Errorf("no owner: %v", err)
	}
}

func TestOneOwnerPerMachineWithinAProcess(t *testing.T) {
	lib := Library{Dir: t.TempDir()}
	m, o := create(t, lib, "phone")
	if _, err := m.Lock("second"); !errors.Is(err, ErrBusy) || !strings.Contains(err.Error(), fmt.Sprintf("pid %d (test)", os.Getpid())) {
		t.Fatalf("second Lock: %v", err)
	}
	o.Release()
	o2, err := m.Lock("second")
	if err != nil {
		t.Fatalf("Lock after Release: %v", err)
	}
	o2.Release()
}

func TestCompetingProcessCannotOwnAndDeathReleases(t *testing.T) {
	lib := Library{Dir: t.TempDir()}
	m, o := create(t, lib, "phone")
	o.Release()
	pid, kill := startHelper(t, "hold", m)
	_, err := m.Lock("launch")
	if !errors.Is(err, ErrBusy) || !strings.Contains(err.Error(), fmt.Sprintf("pid %d (helper hold)", pid)) {
		t.Fatalf("Lock while another process owns the machine: %v", err)
	}
	kill()
	o, err = m.Lock("launch")
	if err != nil {
		t.Fatalf("Lock after the owner died: %v", err)
	}
	o.Release()
}

func TestInterruptedWriteKeepsThePreviousRecord(t *testing.T) {
	lib := Library{Dir: t.TempDir()}
	m, o := create(t, lib, "phone")
	if err := m.RecordNative(o, host.StateStopped); err != nil {
		t.Fatal(err)
	}
	o.Release()
	_, kill := startHelper(t, "interrupt-write", m)
	kill()

	state, _, err := m.LastNative()
	if err != nil || state != host.StateStopped {
		t.Fatalf("after an interrupted write: %q, %v; want the previous %q", state, err, host.StateStopped)
	}
	temps, _ := filepath.Glob(filepath.Join(m.Dir, tempPrefix+"*"))
	if len(temps) != 1 {
		t.Fatalf("interrupted write left %d temporary files, want 1", len(temps))
	}
	o, err = m.Lock("recover")
	if err != nil {
		t.Fatal(err)
	}
	defer o.Release()
	if temps, _ := filepath.Glob(filepath.Join(m.Dir, tempPrefix+"*")); len(temps) != 0 {
		t.Fatalf("new owner kept %v", temps)
	}
	if _, err := lib.Open("phone"); err != nil {
		t.Fatalf("Open after an interrupted write: %v", err)
	}
}

func TestInterruptedAttemptIsNotCompleted(t *testing.T) {
	lib := Library{Dir: t.TempDir()}
	m, o := create(t, lib, "phone")
	o.Release()
	_, kill := startHelper(t, "interrupt-attempt", m)
	kill()
	s := stageState(t, m, StageRestore)
	if s.Completed || s.Receipt != nil || s.Latest == nil || s.Latest.Status != StatusRunning {
		t.Fatalf("interrupted restore: %+v", s)
	}
	obs, err := m.Observe()
	if err != nil {
		t.Fatal(err)
	}
	if obs.Completed(StageRestore) {
		t.Fatal("an interrupted restore is reported completed")
	}
}

func TestObserveKeepsNativeAndGuestSeparate(t *testing.T) {
	lib := Library{Dir: t.TempDir()}
	m, o := create(t, lib, "phone")
	before := time.Now().UTC()
	if err := m.RecordNative(o, host.StateRunning); err != nil {
		t.Fatal(err)
	}
	state, at, err := m.LastNative()
	if err != nil || state != host.StateRunning || at.Before(before.Add(-time.Second)) {
		t.Fatalf("LastNative = %q, %v, %v", state, at, err)
	}
	obs, err := m.Observe()
	if err != nil {
		t.Fatal(err)
	}
	if obs.Native != host.StateRunning || obs.Guest != guest.ReadinessUnknown || len(obs.Stages) != 0 {
		t.Fatalf("observation %+v", obs)
	}
}
