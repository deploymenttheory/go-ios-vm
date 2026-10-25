package cli

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/deploymenttheory/go-ios-vm/internal/buildinfo"
)

type result struct {
	code           int
	stdout, stderr string
}

func runWith(t *testing.T, env Env, args ...string) result {
	t.Helper()
	var stdout, stderr bytes.Buffer
	env.Stdout, env.Stderr = &stdout, &stderr
	code := Run(context.Background(), args, env)
	return result{code, stdout.String(), stderr.String()}
}

func hostEnv() Env {
	return Env{
		GOOS: "darwin", GOARCH: "arm64",
		UserCacheDir:  func() (string, error) { return "/Users/u/Library/Caches", nil },
		UserConfigDir: func() (string, error) { return "/Users/u/Library/Application Support", nil },
	}
}

func TestHelpListsCommandTreeAndDefaults(t *testing.T) {
	for _, args := range [][]string{{"help"}, {"-h"}, {"--help"}} {
		r := runWith(t, hostEnv(), args...)
		if r.code != ExitOK || r.stderr != "" {
			t.Fatalf("%v: code %d, stderr %q", args, r.code, r.stderr)
		}
		for _, want := range []string{
			"host preflight", "vm create <name>", "vm launch <name>", "vm stop <name>", "vm list",
			"vm info <name>", "fw prepare <name>", "fw patch <name>", "restore <name>", "version",
			filepath.Join("/Users/u/Library/Application Support", "go-ios-vm", "machines"),
			filepath.Join("/Users/u/Library/Caches", "go-ios-vm", "ipsw"),
			"iPhone17,3 iOS 27.0.1 (24A446) + cloudOS 26.4 (23E5207q)",
		} {
			if !strings.Contains(r.stdout, want) {
				t.Errorf("%v: help does not mention %q:\n%s", args, want, r.stdout)
			}
		}
	}
}

func TestHelpMarksEveryUnimplementedCommand(t *testing.T) {
	r := runWith(t, hostEnv(), "help")
	for _, c := range commands {
		line := lineContaining(r.stdout, "  "+c.name())
		if c.implemented() == strings.Contains(line, "not implemented: "+c.ticket) {
			t.Errorf("help line for %q misreports its status: %q", c.name(), line)
		}
	}
}

func lineContaining(s, substr string) string {
	for line := range strings.Lines(s) {
		if strings.Contains(line, substr) {
			return line
		}
	}
	return ""
}

func TestHelpReportsUnavailableDefaults(t *testing.T) {
	env := hostEnv()
	env.UserCacheDir = func() (string, error) { return "", errors.New("$HOME is not defined") }
	r := runWith(t, env, "help")
	if r.code != ExitOK || !strings.Contains(r.stdout, "no default: $HOME is not defined") {
		t.Fatalf("code %d, stdout:\n%s", r.code, r.stdout)
	}
}

func TestCommandHelp(t *testing.T) {
	for _, args := range [][]string{{"help", "vm", "create"}, {"vm", "create", "-h"}} {
		r := runWith(t, hostEnv(), args...)
		if r.code != ExitOK {
			t.Fatalf("%v: code %d, stderr %q", args, r.code, r.stderr)
		}
		for _, want := range []string{"Usage: go-ios-vm vm create [flags] <name>", "not implemented in this build; tracked by E2E-01 (#96)", "-cpus", "(default 8)", "-memory", "(default 8192)", "-disk-size", "(default 64)", "-network", `(default "nat")`, "-library"} {
			if !strings.Contains(r.stdout, want) {
				t.Errorf("%v: command help does not mention %q:\n%s", args, want, r.stdout)
			}
		}
	}
}

func TestUsageErrors(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{nil, "no command given"},
		{[]string{"launchpad"}, `unknown command "launchpad"`},
		{[]string{"vm"}, "vm: missing subcommand (one of: create, launch, stop, list, info)"},
		{[]string{"vm", "snapshot"}, `vm: unknown subcommand "snapshot"`},
		{[]string{"vm", "create"}, "vm create: want 1 argument(s) <name>, got 0"},
		{[]string{"vm", "create", "a", "b"}, "vm create: want 1 argument(s) <name>, got 2"},
		{[]string{"vm", "list", "--bogus"}, "vm list: flag provided but not defined: -bogus"},
		{[]string{"vm", "create", "a", "--cpus", "many"}, `vm create: invalid value "many" for flag -cpus`},
		{[]string{"--bogus", "version"}, "flag provided but not defined: -bogus"},
		{[]string{"help", "vm", "create", "extra"}, `help: unknown command "vm create extra"`},
	}
	for _, tt := range tests {
		r := runWith(t, hostEnv(), tt.args...)
		if r.code != ExitUsage {
			t.Errorf("%v: code %d, want %d", tt.args, r.code, ExitUsage)
		}
		if !strings.Contains(r.stderr, tt.want) || !strings.Contains(r.stderr, "Run 'go-ios-vm help' for usage.") {
			t.Errorf("%v: stderr %q, want %q", tt.args, r.stderr, tt.want)
		}
		if r.stdout != "" {
			t.Errorf("%v: stdout %q, want empty", tt.args, r.stdout)
		}
	}
}

func TestUnimplementedCommandsFailWithoutClaimingSuccess(t *testing.T) {
	for _, c := range commands {
		if c.implemented() {
			continue
		}
		args := append([]string{}, c.path...)
		for range c.nargs {
			args = append(args, "phone")
		}
		r := runWith(t, hostEnv(), args...)
		if r.code != ExitUnsupported {
			t.Errorf("%s: code %d, want %d", c.name(), r.code, ExitUnsupported)
		}
		want := c.name() + ": not implemented in this build (tracked by " + c.ticket + ")"
		if !strings.Contains(r.stderr, want) {
			t.Errorf("%s: stderr %q, want %q", c.name(), r.stderr, want)
		}
		if r.stdout != "" {
			t.Errorf("%s: stdout %q, want empty", c.name(), r.stdout)
		}
	}
}

func TestNativeCommandsRejectUnsupportedPlatforms(t *testing.T) {
	for _, platform := range [][2]string{{"linux", "arm64"}, {"darwin", "amd64"}, {"windows", "amd64"}} {
		env := hostEnv()
		env.GOOS, env.GOARCH = platform[0], platform[1]
		r := runWith(t, env, "vm", "info", "phone")
		if r.code != ExitUnsupported {
			t.Errorf("%v: code %d, want %d", platform, r.code, ExitUnsupported)
		}
		want := "vm info: unsupported platform " + platform[0] + "/" + platform[1] + "; go-ios-vm requires darwin/arm64"
		if !strings.Contains(r.stderr, want) {
			t.Errorf("%v: stderr %q, want %q", platform, r.stderr, want)
		}
	}
}

func TestVersionRunsAnywhereAndReportsPins(t *testing.T) {
	env := hostEnv()
	env.GOOS, env.GOARCH = "linux", "amd64"
	r := runWith(t, env, "version")
	if r.code != ExitOK || r.stderr != "" {
		t.Fatalf("code %d, stderr %q", r.code, r.stderr)
	}
	for _, want := range []string{
		"go-ios-vm ",
		"vphone-cli:                " + buildinfo.VPhoneCLIRevision,
		"go-bindings-macosplatform: " + buildinfo.MacOSBindingsRevision,
		"firmware pair:             iPhone17,3 iOS 27.0.1 (24A446) + cloudOS 26.4 (23E5207q)",
	} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("version output does not contain %q:\n%s", want, r.stdout)
		}
	}
}

func TestGlobalFlagsBeforeOrAfterCommand(t *testing.T) {
	for _, args := range [][]string{
		{"--library", "/lib", "--ipsw-cache", "/cache", "vm", "info", "phone"},
		{"vm", "info", "phone", "--library", "/lib", "--ipsw-cache", "/cache"},
		{"vm", "info", "--library=/lib", "phone", "-ipsw-cache=/cache"},
	} {
		inv := parseOnly(t, args)
		if inv.library != "/lib" || inv.ipswCache != "/cache" || len(inv.args) != 1 || inv.args[0] != "phone" {
			t.Errorf("%v: library %q, cache %q, args %q", args, inv.library, inv.ipswCache, inv.args)
		}
	}
	inv := parseOnly(t, []string{"vm", "info", "phone"})
	if want := filepath.Join("/Users/u/Library/Application Support", "go-ios-vm", "machines"); inv.library != want {
		t.Errorf("default library %q, want %q", inv.library, want)
	}
	if want := filepath.Join("/Users/u/Library/Caches", "go-ios-vm", "ipsw"); inv.ipswCache != want {
		t.Errorf("default cache %q, want %q", inv.ipswCache, want)
	}
}

func TestArgumentsAfterDoubleDashArePositional(t *testing.T) {
	inv := parseOnly(t, []string{"vm", "info", "--", "-phone"})
	if len(inv.args) != 1 || inv.args[0] != "-phone" {
		t.Fatalf("args %q, want [-phone]", inv.args)
	}
}

// parseOnly runs args against a stand-in for "vm info" that captures the
// invocation instead of failing as unimplemented.
func parseOnly(t *testing.T, args []string) *invocation {
	t.Helper()
	cmd, _, err := lookup([]string{"vm", "info"})
	if err != nil {
		t.Fatal(err)
	}
	var got *invocation
	run := cmd.run
	cmd.run = func(_ context.Context, inv *invocation) error {
		got = inv
		return nil
	}
	defer func() { cmd.run = run }()
	if r := runWith(t, hostEnv(), args...); r.code != ExitOK {
		t.Fatalf("%v: code %d, stderr %q", args, r.code, r.stderr)
	}
	return got
}
