package cli

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/deploymenttheory/go-ios-vm/internal/preflight"
)

func withProbes(t *testing.T, mutate func(*preflight.Probes)) {
	t.Helper()
	saved := preflightProbes
	preflightProbes = func(goos, goarch string) preflight.Probes {
		p := preflight.Probes{
			GOOS: goos, GOARCH: goarch,
			ProductVersion: func() (string, error) { return "27.0.1", nil },
			Sysctl: func(name string) (int64, error) {
				return map[string]int64{"hw.memsize": 24 << 30, "hw.ncpu": 10}[name], nil
			},
			Capabilities:  func() ([]string, error) { return nil, nil },
			ResearchModel: func() (bool, error) { return false, nil },
			Entitlements:  func(context.Context) (map[string]bool, error) { return map[string]bool{}, nil },
			Command: func(_ context.Context, _ string, args ...string) (string, error) {
				if len(args) > 0 && args[0] == "allow-research-guests" {
					return "Allow Research Guests status: enabled\n", nil
				}
				return "/SDKs/iPhoneOS.sdk", nil
			},
			LookPath: func(name string) (string, error) { return "/usr/bin/" + name, nil },
			Space: func(string) (preflight.Space, error) {
				return preflight.Space{ID: "data", MountPath: "/System/Volumes/Data", Free: 200 << 30}, nil
			},
			CacheEntries: func(string) ([]preflight.Entry, error) {
				return []preflight.Entry{{Name: "13d574.ipsw", Size: 6 << 30}}, nil
			},
		}
		if mutate != nil {
			mutate(&p)
		}
		return p
	}
	t.Cleanup(func() { preflightProbes = saved })
}

func TestHostPreflightReport(t *testing.T) {
	withProbes(t, nil)
	r := runWith(t, hostEnv(), "host", "preflight")
	if r.code != ExitOK || r.stderr != "" {
		t.Fatalf("%+v", r)
	}
	if line := lineContaining(r.stdout, "Virtualization entitlements on this executable"); !strings.HasPrefix(line, "unverified ") {
		t.Errorf("entitlements row %q, want unverified", line)
	}
	for _, want := range []string{
		"Host preflight (read-only; not evidence that a machine restores or boots)",
		"Manual preparation (this runner never changes host policy):",
		"/System/Volumes/Data: 200.0 GiB free",
		"ipsw cache: /Users/u/Library/Caches/go-ios-vm/ipsw",
		"IPSW cache: 13d574.ipsw (6.0 GiB); existing entries are never modified",
		"About 15 GiB was free when the plan was written",
		"0 unsupported (0 blocking)",
	} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("report does not contain %q:\n%s", want, r.stdout)
		}
	}
}

func TestHostPreflightFailsOnUnmetPrerequisites(t *testing.T) {
	withProbes(t, func(p *preflight.Probes) {
		p.Space = func(string) (preflight.Space, error) {
			return preflight.Space{ID: "data", MountPath: "/System/Volumes/Data", Free: 9 << 30}, nil
		}
	})
	r := runWith(t, hostEnv(), "host", "preflight", "--json")
	if r.code != ExitFailure || !strings.Contains(r.stderr, "host preflight: host prerequisite unmet: Free space on /System/Volumes/Data") {
		t.Fatalf("%+v", r)
	}
	var report preflight.Report
	if err := json.Unmarshal([]byte(r.stdout), &report); err != nil {
		t.Fatalf("stdout is not a JSON report: %v\n%s", err, r.stdout)
	}
	if len(report.Unmet()) != 1 || report.Paths.Library == "" || len(report.Volumes) != 1 {
		t.Fatalf("report %+v", report)
	}
}
