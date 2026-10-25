package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/deploymenttheory/go-ios-vm/internal/buildinfo"
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
	build := exec.Command(goTool(t), "build", "-o", bin, ".")
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
		cmd := exec.Command(bin, tt.args...)
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
	build := exec.Command(gotool, "build", "-tags", "pins", "../../internal/pins")
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("CGO_ENABLED=0 go build -tags pins: %v\n%s", err, out)
	}

	list := exec.Command(gotool, "list", "-deps", "-tags", "pins",
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
