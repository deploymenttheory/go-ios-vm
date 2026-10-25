// Package testenv selects test tiers. Portable and fixture tests always run.
// Native capability tests and live virtual-machine tests run only when their
// tier is requested through GO_IOS_VM_TEST_TIER; once requested, a missing
// prerequisite fails the test rather than skipping it, so an unavailable host
// is never reported as a passing integration check.
package testenv

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
)

// TierVar is the environment variable that requests a test tier.
const TierVar = "GO_IOS_VM_TEST_TIER"

// Tier is a set of tests with shared prerequisites. Each tier includes the
// tiers below it.
type Tier int

// Test tiers.
const (
	// Portable tests use fixtures only and run everywhere.
	Portable Tier = iota
	// Native tests call nonprivileged macOS runtime facilities.
	Native
	// Live tests need an authorized research host and may download firmware,
	// request restore tickets or boot a guest.
	Live
)

func (t Tier) String() string {
	switch t {
	case Portable:
		return "portable"
	case Native:
		return "native"
	case Live:
		return "live"
	default:
		return "tier(" + strconv.Itoa(int(t)) + ")"
	}
}

var (
	// ErrInvalidTier reports an unrecognized TierVar value.
	ErrInvalidTier = errors.New("invalid test tier")
	// ErrUnmet reports a prerequisite this host does not meet.
	ErrUnmet = errors.New("prerequisite unmet")
)

// ParseTier parses a TierVar value. The empty string is Portable.
func ParseTier(s string) (Tier, error) {
	switch s {
	case "", "portable":
		return Portable, nil
	case "native":
		return Native, nil
	case "live":
		return Live, nil
	default:
		return Portable, fmt.Errorf("%w %q in %s (want portable, native or live)", ErrInvalidTier, s, TierVar)
	}
}

// TB is the subset of testing.TB used here.
type TB interface {
	Helper()
	Logf(format string, args ...any)
	Skipf(format string, args ...any)
	Fatalf(format string, args ...any)
}

// RanMarker prefixes the line Require logs when a test proceeds in a tier.
// scripts/test-tier.sh counts these lines so that a tier run in which no test
// actually ran fails instead of passing.
const RanMarker = "testenv: running in tier"

// Prerequisite is one condition a native or live test needs.
type Prerequisite struct {
	Name  string
	Check func() error
}

// Requested returns the tier requested by the environment.
func Requested() (Tier, error) { return ParseTier(os.Getenv(TierVar)) }

// Require skips t unless tier was requested. When it was, Require fails t for
// the first unmet prerequisite.
func Require(t TB, tier Tier, prereqs ...Prerequisite) {
	t.Helper()
	requested, err := Requested()
	if err != nil {
		t.Fatalf("%v", err)
		return
	}
	if requested < tier {
		t.Skipf("%s test not requested; set %s=%s to run it", tier, TierVar, tier)
		return
	}
	var unmet []string
	for _, p := range prereqs {
		if err := p.Check(); err != nil {
			unmet = append(unmet, p.Name+": "+err.Error())
		}
	}
	if len(unmet) > 0 {
		t.Fatalf("%s test requested but prerequisites are unmet:\n  %s", tier, strings.Join(unmet, "\n  "))
		return
	}
	t.Logf("%s %s", RanMarker, tier)
}

// AppleSilicon requires a darwin/arm64 host running at least minMajor.
func AppleSilicon(minMajor int) Prerequisite {
	return Prerequisite{
		Name: fmt.Sprintf("darwin/arm64 host on macOS %d or later", minMajor),
		Check: func() error {
			if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
				return fmt.Errorf("%w: running on %s/%s", ErrUnmet, runtime.GOOS, runtime.GOARCH)
			}
			return checkMacOS(minMajor)
		},
	}
}

// FileFromEnv requires the environment variable name to name an existing file.
func FileFromEnv(name, purpose string) Prerequisite {
	return Prerequisite{
		Name: purpose + " (" + name + ")",
		Check: func() error {
			path := os.Getenv(name)
			if path == "" {
				return fmt.Errorf("%w: not set", ErrUnmet)
			}
			info, err := os.Stat(path) //nolint:gosec // the path is the tester's own artifact
			if err != nil {
				return fmt.Errorf("%w: %w", ErrUnmet, err)
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf("%w: %s is not a regular file", ErrUnmet, path)
			}
			return nil
		},
	}
}
