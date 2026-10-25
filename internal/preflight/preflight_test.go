package preflight

import (
	"context"
	"errors"
	"strings"
	"testing"
)

const gb = 1 << 30

// goodHost is a prepared host with room for one create.
func goodHost() Probes {
	return Probes{
		GOOS: "darwin", GOARCH: "arm64",
		ProductVersion: func() (string, error) { return "27.0.1", nil },
		Sysctl: func(name string) (int64, error) {
			return map[string]int64{"kern.hv_vmm_present": 0, "hw.memsize": 24 * gb, "hw.ncpu": 10}[name], nil
		},
		Capabilities:  func() ([]string, error) { return nil, nil },
		ResearchModel: func() (bool, error) { return true, nil },
		Entitlements: func(context.Context) (map[string]bool, error) {
			have := map[string]bool{}
			for _, e := range VMEntitlements {
				have[e] = true
			}
			return have, nil
		},
		Command: func(_ context.Context, name string, args ...string) (string, error) {
			switch strings.Join(append([]string{name}, args...), " ") {
			case "/usr/bin/csrutil allow-research-guests status":
				return "Allow Research Guests status: enabled\n", nil
			case "/usr/bin/csrutil status":
				return "System Integrity Protection status: disabled.\n", nil
			case "/usr/sbin/nvram boot-args":
				return "boot-args\tamfi_get_out_of_my_way=1\n", nil
			case "/usr/bin/xcrun --sdk iphoneos --show-sdk-path":
				return "/SDKs/iPhoneOS27.1.sdk\n", nil
			}
			return "", errors.New("unexpected command " + name)
		},
		LookPath: func(name string) (string, error) { return "/usr/bin/" + name, nil },
		Space: func(string) (Space, error) {
			return Space{ID: "data", MountPath: "/System/Volumes/Data", Free: 200 * gb}, nil
		},
		CacheEntries: func(string) ([]Entry, error) { return nil, nil },
	}
}

var paths = Paths{IPSWCache: "/cache/ipsw", Library: "/lib/machines", Scratch: "/tmp"}

func result(t *testing.T, r Report, id string) Result {
	t.Helper()
	for _, res := range r.Results {
		if res.ID == id {
			return res
		}
	}
	t.Fatalf("no %s result in %+v", id, r.Results)
	return Result{}
}

func TestPreparedHost(t *testing.T) {
	r := Run(context.Background(), goodHost(), paths)
	if err := r.RequireCreate(); err != nil {
		t.Fatalf("RequireCreate on a prepared host: %v", err)
	}
	for _, res := range r.Results {
		if res.ID == "code-integrity" {
			if res.Status != Unverified {
				t.Errorf("code integrity is %s; it is only known once the signed process starts", res.Status)
			}
			continue
		}
		if res.Status != Supported {
			t.Errorf("%s: %s (%s)", res.ID, res.Status, res.Observed)
		}
	}
}

func TestUnsupportedHosts(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Probes)
		id     string
		status Status
		block  bool
	}{
		{"Intel Mac", func(p *Probes) { p.GOARCH = "amd64" }, "platform", Unsupported, true},
		{"Linux", func(p *Probes) { p.GOOS = "linux" }, "platform", Unsupported, true},
		{"old macOS", func(p *Probes) { p.ProductVersion = func() (string, error) { return "26.6.2", nil } }, "macos", Unsupported, true},
		{"nested VM", func(p *Probes) {
			base := p.Sysctl
			p.Sysctl = func(n string) (int64, error) {
				if n == "kern.hv_vmm_present" {
					return 1, nil
				}
				return base(n)
			}
		}, "nested", Unsupported, true},
		{"8 GiB host", func(p *Probes) {
			base := p.Sysctl
			p.Sysctl = func(n string) (int64, error) {
				if n == "hw.memsize" {
					return 8 * gb, nil
				}
				return base(n)
			}
		}, "memory", Unsupported, true},
		{"absent private API", func(p *Probes) {
			p.Capabilities = func() ([]string, error) { return []string{"-[VZMacOSBootLoader _setROMURL:]"}, nil }
		}, "virtualization", Unsupported, true},
		{"research guests disabled", func(p *Probes) {
			base := p.Command
			p.Command = func(ctx context.Context, name string, args ...string) (string, error) {
				if len(args) > 0 && args[0] == "allow-research-guests" {
					return "Allow Research Guests status: disabled\n", nil
				}
				return base(ctx, name, args...)
			}
		}, "research-guests", Unsupported, true},
		{"entitled but refused", func(p *Probes) { p.ResearchModel = func() (bool, error) { return false, nil } }, "research-model", Unsupported, true},
		{"missing codesign", func(p *Probes) {
			p.LookPath = func(name string) (string, error) {
				if name == "codesign" {
					return "", errors.New("not found")
				}
				return "/usr/bin/" + name, nil
			}
		}, "tool-codesign", Unsupported, true},
		{"no iPhoneOS SDK", func(p *Probes) {
			base := p.Command
			p.Command = func(ctx context.Context, name string, args ...string) (string, error) {
				if name == "/usr/bin/xcrun" {
					return "", errors.New("xcrun: SDK \"iphoneos\" cannot be located")
				}
				return base(ctx, name, args...)
			}
		}, "tool-iphoneos-sdk", Unsupported, true},
		{"low space", func(p *Probes) {
			p.Space = func(string) (Space, error) { return Space{ID: "data", MountPath: "/", Free: 9 * gb}, nil }
		}, "space-data", Unsupported, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := goodHost()
			tt.mutate(&p)
			r := Run(context.Background(), p, paths)
			res := result(t, r, tt.id)
			if res.Status != tt.status || res.Blocking != tt.block {
				t.Fatalf("%s: %s blocking=%v (%s)", tt.id, res.Status, res.Blocking, res.Observed)
			}
			if res.Status == Unsupported && res.Remedy == "" {
				t.Errorf("%s: unsupported without a remedy", tt.id)
			}
			err := r.RequireCreate()
			if !errors.Is(err, ErrPrerequisiteUnmet) || !strings.Contains(err.Error(), res.Title) {
				t.Fatalf("RequireCreate = %v, want it to name %q", err, res.Title)
			}
		})
	}
}

func TestUnknownAuthorizationIsNeverSupported(t *testing.T) {
	p := goodHost()
	p.Entitlements = func(context.Context) (map[string]bool, error) { return map[string]bool{}, nil }
	p.ResearchModel = func() (bool, error) { return false, nil }
	base := p.Command
	p.Command = func(ctx context.Context, name string, args ...string) (string, error) {
		switch {
		case name == "/usr/bin/csrutil" && len(args) > 0 && args[0] == "allow-research-guests":
			return "", errors.New("csrutil: permission denied")
		case name == "/usr/sbin/nvram":
			return "", errors.New("nvram: data was not found")
		}
		return base(ctx, name, args...)
	}
	r := Run(context.Background(), p, paths)
	for _, id := range []string{"entitlements", "research-model", "research-guests", "code-integrity"} {
		if res := result(t, r, id); res.Status != Unverified {
			t.Errorf("%s: %s, want unverified (%s)", id, res.Status, res.Observed)
		}
	}
	if res := result(t, r, "code-integrity"); !strings.Contains(res.Observed, "AMFI enforcing") || res.Remedy == "" {
		t.Errorf("code integrity: %+v", res)
	}
	if err := r.RequireCreate(); err != nil {
		t.Fatalf("unverified checks blocked creation: %v", err)
	}
}

func TestProbeErrorsAreUnverified(t *testing.T) {
	p := goodHost()
	fail := errors.New("probe failed")
	p.ProductVersion = func() (string, error) { return "", fail }
	p.Sysctl = func(string) (int64, error) { return 0, fail }
	p.Capabilities = func() ([]string, error) { return nil, fail }
	p.ResearchModel = func() (bool, error) { return false, fail }
	p.Entitlements = func(context.Context) (map[string]bool, error) { return nil, fail }
	p.CacheEntries = func(string) ([]Entry, error) { return nil, fail }
	p.Space = func(string) (Space, error) { return Space{}, fail }
	r := Run(context.Background(), p, paths)
	for _, res := range r.Results {
		if strings.Contains(res.Observed, "probe failed") && res.Status != Unverified {
			t.Errorf("%s: probe error reported as %s", res.ID, res.Status)
		}
	}
	for _, id := range []string{"macos", "nested", "memory", "cpus", "virtualization", "ipsw-cache", "space-library"} {
		if res := result(t, r, id); res.Status != Unverified {
			t.Errorf("%s: %s", id, res.Status)
		}
	}
}

func TestSharedAndSeparateVolumes(t *testing.T) {
	p := goodHost()
	p.Space = func(path string) (Space, error) {
		if path == paths.Library {
			return Space{ID: "external", MountPath: "/Volumes/External", Free: 40 * gb}, nil
		}
		return Space{ID: "data", MountPath: "/System/Volumes/Data", Free: 20 * gb}, nil
	}
	r := Run(context.Background(), p, paths)
	if len(r.Volumes) != 2 {
		t.Fatalf("volumes %+v", r.Volumes)
	}
	data, external := r.Volumes[0], r.Volumes[1]
	if len(data.Paths) != 2 || data.Need() != IPhoneIPSWBytes+CloudOSIPSWBytes {
		t.Errorf("cache and scratch volume: paths %v, need %d", data.Paths, data.Need())
	}
	if want := int64(RestoreTreeBytes + MachineDiskBytes + MachineNVRAMBytes + MachineSEPBytes); external.Need() != want {
		t.Errorf("library volume needs %d, want %d", external.Need(), want)
	}
	if res := result(t, r, "space-data"); res.Status == Unsupported {
		t.Errorf("20 GiB for the IPSWs alone was reported unsupported: %+v", res)
	}

	// Everything on one volume is summed against one free figure.
	p.Space = func(string) (Space, error) { return Space{ID: "data", MountPath: "/", Free: 40 * gb}, nil }
	r = Run(context.Background(), p, paths)
	if len(r.Volumes) != 1 || len(r.Volumes[0].Paths) != 3 {
		t.Fatalf("shared volume %+v", r.Volumes)
	}
	if res := result(t, r, "space-data"); res.Status != Unsupported {
		t.Errorf("40 GiB shared by cache and library: %s (%s)", res.Status, res.Observed)
	}
}

func TestExistingCacheEntryReducesDemand(t *testing.T) {
	p := goodHost()
	p.CacheEntries = func(string) ([]Entry, error) {
		return []Entry{{Name: iphoneIPSWName, Size: IPhoneIPSWBytes}, {Name: "13d574.ipsw", Size: 6 * gb}}, nil
	}
	r := Run(context.Background(), p, paths)
	if len(r.Cache) != 2 {
		t.Fatalf("cache %+v", r.Cache)
	}
	for _, d := range r.Volumes[0].Demand {
		if strings.Contains(d.What, "iPhone17,3") {
			t.Fatalf("a cached iPhone IPSW is still counted: %+v", d)
		}
	}
	if !strings.Contains(strings.Join(r.Notes, "\n"), "FW-02 verifies it before use") {
		t.Errorf("notes do not qualify the cached entry: %v", r.Notes)
	}

	// The same name with a different size is not the target archive.
	p.CacheEntries = func(string) ([]Entry, error) { return []Entry{{Name: iphoneIPSWName, Size: 100}}, nil }
	r = Run(context.Background(), p, paths)
	if r.Volumes[0].Need() < IPhoneIPSWBytes {
		t.Fatal("a truncated cache entry reduced the demand")
	}
}

func TestReportNotesThePlanningObservation(t *testing.T) {
	r := Run(context.Background(), goodHost(), paths)
	if !strings.Contains(strings.Join(r.Notes, "\n"), "About 15 GiB was free when the plan was written") {
		t.Fatalf("notes %v", r.Notes)
	}
}

func TestDescribeSIP(t *testing.T) {
	custom := "System Integrity Protection status: unknown (Custom Configuration).\n\nConfiguration:\n\tApple Internal: disabled\n\tKext Signing: enabled\n\tDebugging Restrictions: disabled\n"
	if got := describeSIP(custom); got != "SIP unknown (Custom Configuration) (debugging restrictions disabled)" {
		t.Errorf("custom: %q", got)
	}
	if got := describeSIP("System Integrity Protection status: enabled.\n"); got != "SIP enabled" {
		t.Errorf("enabled: %q", got)
	}
}

// TestRunsOnlyReadOnlyCommands pins the host commands preflight runs to an
// allowlist of read-only invocations.
func TestRunsOnlyReadOnlyCommands(t *testing.T) {
	readOnly := map[string]bool{
		"/usr/bin/csrutil allow-research-guests status": true,
		"/usr/bin/csrutil status":                       true,
		"/usr/sbin/nvram boot-args":                     true,
		"/usr/bin/xcrun --sdk iphoneos --show-sdk-path": true,
	}
	p := goodHost()
	base := p.Command
	var ran []string
	p.Command = func(ctx context.Context, name string, args ...string) (string, error) {
		ran = append(ran, strings.Join(append([]string{name}, args...), " "))
		return base(ctx, name, args...)
	}
	Run(context.Background(), p, paths)
	if len(ran) == 0 {
		t.Fatal("no commands recorded")
	}
	for _, c := range ran {
		if !readOnly[c] {
			t.Errorf("preflight ran %q, which is not on the read-only allowlist", c)
		}
	}
}
