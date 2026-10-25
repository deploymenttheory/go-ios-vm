package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/deploymenttheory/go-ios-vm/internal/buildinfo"
	"github.com/deploymenttheory/go-ios-vm/internal/firmware"
	"github.com/deploymenttheory/go-ios-vm/internal/machine"
)

func goTool(t *testing.T) string {
	t.Helper()
	path, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go toolchain not on PATH")
	}
	return path
}

// TestBinaryWithoutCgo builds the host command with CGO_ENABLED=0 and checks
// its exit codes and output streams as a subprocess.
func TestBinaryWithoutCgo(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "go-ios-vm")
	build := exec.CommandContext(t.Context(), goTool(t), "build", "-o", bin, ".")
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("CGO_ENABLED=0 go build: %v\n%s", err, out)
	}

	tests := []struct {
		args       []string
		code       int
		stdout     string
		stderr     string
		nativeOnly bool
	}{
		{args: []string{"version"}, stdout: buildinfo.VPhoneCLIRevision},
		{args: []string{"help"}, stdout: "vm create <name>"},
		{args: []string{}, code: 2, stderr: "no command given"},
		{args: []string{"vm", "create", "phone"}, code: 3, stderr: "vm create: not implemented in this build", nativeOnly: true},
	}
	for _, tt := range tests {
		if tt.nativeOnly && (runtime.GOOS != "darwin" || runtime.GOARCH != "arm64") {
			continue
		}
		var stdout, stderr bytes.Buffer
		cmd := exec.CommandContext(t.Context(), bin, tt.args...)
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		err := cmd.Run()
		code := 0
		if exit := (*exec.ExitError)(nil); errors.As(err, &exit) {
			code = exit.ExitCode()
		} else if err != nil {
			t.Fatalf("%v: %v", tt.args, err)
		}
		if code != tt.code {
			t.Errorf("%v: exit %d, want %d (stderr %q)", tt.args, code, tt.code, stderr.String())
		}
		if !strings.Contains(stdout.String(), tt.stdout) || !strings.Contains(stderr.String(), tt.stderr) {
			t.Errorf("%v: stdout %q, stderr %q", tt.args, stdout.String(), stderr.String())
		}
		if tt.code != 0 && stdout.Len() != 0 {
			t.Errorf("%v: failing command wrote to stdout: %q", tt.args, stdout.String())
		}
	}
}

// TestPinnedDependenciesBuildWithoutCgo builds every pinned host dependency
// with CGO_ENABLED=0. Packages that also carry an optional cgo implementation
// are logged; with cgo disabled they compile their pure-Go files.
func TestPinnedDependenciesBuildWithoutCgo(t *testing.T) {
	gotool := goTool(t)
	build := exec.CommandContext(t.Context(), gotool, "build", "-tags", "pins", "../../internal/pins")
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("CGO_ENABLED=0 go build -tags pins: %v\n%s", err, out)
	}

	list := exec.CommandContext(t.Context(), gotool, "list", "-deps", "-tags", "pins",
		"-f", "{{if and (not .Standard) .CgoFiles}}{{.ImportPath}}{{end}}",
		".", "../../internal/pins")
	list.Env = append(os.Environ(), "CGO_ENABLED=1")
	out, err := list.Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	if optional := strings.Fields(string(out)); len(optional) > 0 {
		t.Logf("optional cgo implementations, excluded with CGO_ENABLED=0: %v", optional)
	}
}

// allowedModules is the reviewed module closure of the host command and the
// pinned packages. A new module must be reviewed before it is added here: the
// host may not depend on third-party C restore libraries or on wrappers that
// execute upstream host programs.
var allowedModules = []string{
	"github.com/blacktop/go-dwarf",
	"github.com/blacktop/go-macho",
	"github.com/deploymenttheory/go-bindings-macosplatform",
	"github.com/deploymenttheory/go-ios-vm",
	"github.com/ebitengine/purego",
	"golang.org/x/arch",
	"howett.net/plist",
}

func TestDependencyModulesAllowed(t *testing.T) {
	list := exec.CommandContext(t.Context(), goTool(t), "list", "-deps", "-tags", "pins",
		"-f", "{{with .Module}}{{.Path}}{{end}}", ".", "../../internal/pins")
	out, err := list.Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	for _, module := range strings.Fields(string(out)) {
		if !slices.Contains(allowedModules, module) {
			t.Errorf("unreviewed module %s in the host dependency closure", module)
		}
	}
}

// TestBindingsPin checks that go.mod requires the agreed bindings revision.
func TestBindingsPin(t *testing.T) {
	data, err := os.ReadFile("../../go.mod")
	if err != nil {
		t.Fatal(err)
	}
	for line := range strings.Lines(string(data)) {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "github.com/deploymenttheory/go-bindings-macosplatform" {
			if !strings.HasSuffix(fields[1], "-"+buildinfo.MacOSBindingsRevision[:12]) {
				t.Fatalf("go.mod requires bindings %s, want revision %s", fields[1], buildinfo.MacOSBindingsRevision)
			}
			return
		}
	}
	t.Fatal("go.mod does not require go-bindings-macosplatform")
}

// TestRestartPreservesIdentityAndReceipts creates a machine with a completed
// stage in this process, then reads it back through a fresh CLI process.
func TestRestartPreservesIdentityAndReceipts(t *testing.T) {
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		t.Skip("vm info needs the darwin/arm64 host")
	}
	bin := filepath.Join(t.TempDir(), "go-ios-vm")
	build := exec.CommandContext(t.Context(), goTool(t), "build", "-o", bin, ".")
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	lib := machine.Library{Dir: t.TempDir()}
	identifier := []byte("stable-native-identifier")
	m, o, err := lib.Create(machine.Config{
		Name: "phone", Identifier: identifier, Pair: firmware.SupportedPair,
		Hardware: machine.Hardware{CPUs: 8, MemoryMiB: 8192, DiskSizeGB: 64, Network: "nat"},
		Files:    machine.DefaultFiles,
	}, "test")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(m.Dir, "acquire.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	a, err := m.Begin(o, machine.StageAcquire)
	if err == nil {
		err = a.Complete("acquire.json")
	}
	if err != nil {
		t.Fatal(err)
	}
	o.Release()

	sum := sha256.Sum256(identifier)
	for range 2 {
		out, err := exec.CommandContext(t.Context(), bin, "vm", "info", "phone", "--library", lib.Dir).Output()
		if err != nil {
			t.Fatalf("vm info: %v", err)
		}
		for _, want := range []string{
			fmt.Sprintf("Identifier: %d bytes, SHA-256 %s", len(identifier), hex.EncodeToString(sum[:8])),
			"acquire     completed (1 evidence file(s) verified)",
		} {
			if !strings.Contains(string(out), want) {
				t.Fatalf("vm info after restart does not contain %q:\n%s", want, out)
			}
		}
	}
}

// TestHostAndGuestStaySeparate keeps the guest daemon out of the host binary
// and the macOS bindings out of the iOS guest build.
func TestHostAndGuestStaySeparate(t *testing.T) {
	gotool := goTool(t)
	host, err := exec.CommandContext(t.Context(), gotool, "list", "-deps", ".").Output()
	if err != nil {
		t.Fatalf("go list host: %v", err)
	}
	if strings.Contains(string(host), "internal/guest/daemon") {
		t.Error("the host binary imports the guest daemon")
	}
	guestList := exec.CommandContext(t.Context(), gotool, "list", "-deps", "../go-ios-vm-guestd")
	guestList.Env = append(os.Environ(), "GOOS=ios", "GOARCH=arm64", "CGO_ENABLED=1")
	guest, err := guestList.Output()
	if err != nil {
		t.Fatalf("go list guest: %v", err)
	}
	if strings.Contains(string(guest), "go-bindings-macosplatform") {
		t.Error("the iOS guest build imports the macOS bindings")
	}
	if !strings.Contains(string(guest), "internal/guest/daemon") {
		t.Error("guest dependency listing looks wrong: no daemon package")
	}
}
