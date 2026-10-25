package testenv

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// recorder records how Require ended a test.
type recorder struct {
	logged, skipped, failed string
}

func (r *recorder) Helper() {}

func (r *recorder) Logf(format string, args ...any) { r.logged = fmt.Sprintf(format, args...) }

func (r *recorder) Skipf(format string, args ...any) { r.skipped = fmt.Sprintf(format, args...) }

func (r *recorder) Fatalf(format string, args ...any) { r.failed = fmt.Sprintf(format, args...) }

var (
	met   = Prerequisite{Name: "met", Check: func() error { return nil }}
	unmet = Prerequisite{Name: "research host", Check: func() error { return errors.New("SIP policy not prepared") }}
)

func TestRequire(t *testing.T) {
	tests := []struct {
		requested string
		tier      Tier
		prereqs   []Prerequisite
		skip      string
		fail      string
	}{
		{requested: "", tier: Native, skip: "native test not requested; set GO_IOS_VM_TEST_TIER=native to run it"},
		{requested: "native", tier: Live, prereqs: []Prerequisite{unmet}, skip: "live test not requested; set GO_IOS_VM_TEST_TIER=live to run it"},
		{requested: "native", tier: Native, prereqs: []Prerequisite{met}},
		{requested: "live", tier: Native, prereqs: []Prerequisite{met}},
		{requested: "live", tier: Live, prereqs: []Prerequisite{met, unmet}, fail: "live test requested but prerequisites are unmet:\n  research host: SIP policy not prepared"},
		{requested: "everything", tier: Native, fail: `invalid test tier "everything"`},
	}
	for _, tt := range tests {
		t.Setenv(TierVar, tt.requested)
		var r recorder
		Require(&r, tt.tier, tt.prereqs...)
		if r.skipped != tt.skip || !strings.Contains(r.failed, tt.fail) || (tt.fail == "") != (r.failed == "") {
			t.Errorf("requested %q, tier %s: skipped %q, failed %q; want skip %q, fail %q", tt.requested, tt.tier, r.skipped, r.failed, tt.skip, tt.fail)
		}
		ran := tt.skip == "" && tt.fail == ""
		if want := RanMarker + " " + tt.tier.String(); ran != (r.logged == want) {
			t.Errorf("requested %q, tier %s: logged %q, ran %v", tt.requested, tt.tier, r.logged, ran)
		}
	}
}

func TestAppleSilicon(t *testing.T) {
	if err := AppleSilicon(1).Check(); err != nil {
		if runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" {
			t.Fatalf("Check() on an Apple Silicon Mac = %v", err)
		}
		return
	}
	if err := AppleSilicon(1000).Check(); err == nil || !strings.Contains(err.Error(), "running macOS") {
		t.Fatalf("Check() for macOS 1000 = %v, want a version error", err)
	}
}

func TestFileFromEnv(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "iPhone.ipsw")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	p := FileFromEnv("GO_IOS_VM_TEST_IPSW", "iPhone IPSW")
	for value, want := range map[string]string{"": "not set", dir: "is not a regular file", filepath.Join(dir, "missing"): "no such file", file: ""} {
		t.Setenv("GO_IOS_VM_TEST_IPSW", value)
		err := p.Check()
		if (want == "") != (err == nil) || (err != nil && !strings.Contains(err.Error(), want)) {
			t.Errorf("%q: Check() = %v, want %q", value, err, want)
		}
	}
}

// TestTierScriptFailsWhenNothingRan runs scripts/test-tier.sh against a package
// with no tier tests: the run must fail rather than report a verified tier.
func TestTierScriptFailsWhenNothingRan(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not on PATH")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not on PATH")
	}
	tests := []struct {
		args []string
		code int
		want string
	}{
		{[]string{"live", "./internal/progress"}, 1, "live tier: no test ran in this tier; nothing was verified"},
		{[]string{"native", "./internal/progress"}, 1, "native tier: no test ran in this tier; nothing was verified"},
		{[]string{"everything"}, 2, "usage:"},
	}
	for _, tt := range tests {
		cmd := exec.CommandContext(t.Context(), "bash", append([]string{"scripts/test-tier.sh"}, tt.args...)...)
		cmd.Dir = "../.."
		out, err := cmd.CombinedOutput()
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() != tt.code {
			t.Errorf("%v: err %v, want exit %d\n%s", tt.args, err, tt.code, out)
		}
		if !strings.Contains(string(out), tt.want) {
			t.Errorf("%v: output does not contain %q:\n%s", tt.args, tt.want, out)
		}
	}
}
