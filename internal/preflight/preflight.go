// Package preflight checks whether this host can create and run the supported
// machine, without changing anything. Every check reports what it observed and
// is supported, unsupported or unverified; nothing it cannot observe is
// reported as supported. Host preparation stays manual: preflight explains it
// but never changes SIP, research-guest policy or AMFI, installs a helper,
// deletes cached files or downloads firmware.
package preflight

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// Status is the outcome of one check.
type Status string

// Check outcomes.
const (
	Supported   Status = "supported"
	Unsupported Status = "unsupported"
	// Unverified means the requirement could not be established either way.
	Unverified Status = "unverified"
)

// Result is one check.
type Result struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Status   Status `json:"status"`
	Observed string `json:"observed"`
	// Remedy is the manual preparation that would resolve the check.
	Remedy string `json:"remedy,omitempty"`
	// Blocking marks a check whose unsupported result must stop machine
	// creation before any destructive work.
	Blocking bool `json:"blocking"`
}

// Demand is space a create needs on one volume.
type Demand struct {
	What  string `json:"what"`
	Bytes int64  `json:"bytes"`
	// Basis says where the figure comes from.
	Basis string `json:"basis"`
}

// Volume is a file system holding one or more of the runner's paths.
type Volume struct {
	ID        string   `json:"id"`
	MountPath string   `json:"mount_path"`
	Free      int64    `json:"free"`
	Paths     []string `json:"paths"`
	Demand    []Demand `json:"demand"`
}

// Need is the sum of the volume's demand.
func (v Volume) Need() int64 {
	var n int64
	for _, d := range v.Demand {
		n += d.Bytes
	}
	return n
}

// Paths are the directories the runner uses.
type Paths struct {
	IPSWCache string `json:"ipsw_cache"`
	Library   string `json:"library"`
	Scratch   string `json:"scratch"`
}

// Report is a complete preflight run.
type Report struct {
	Results []Result `json:"results"`
	Paths   Paths    `json:"paths"`
	Volumes []Volume `json:"volumes"`
	Cache   []Entry  `json:"cache"`
	Notes   []string `json:"notes"`
}

// ErrPrerequisiteUnmet reports a known unmet prerequisite for creating a
// machine.
var ErrPrerequisiteUnmet = errors.New("host prerequisite unmet")

// Unmet returns the blocking checks that are unsupported.
func (r Report) Unmet() []Result {
	var out []Result
	for _, res := range r.Results {
		if res.Blocking && res.Status == Unsupported {
			out = append(out, res)
		}
	}
	return out
}

// RequireCreate returns an error wrapping ErrPrerequisiteUnmet that names every
// known unmet prerequisite, or nil when none is known. Unverified checks do not
// block creation here; they are proven or disproven by the stage that needs
// them.
func (r Report) RequireCreate() error {
	unmet := r.Unmet()
	if len(unmet) == 0 {
		return nil
	}
	names := make([]string, len(unmet))
	for i, u := range unmet {
		names[i] = u.Title + " (" + u.Observed + ")"
	}
	return fmt.Errorf("%w: %s", ErrPrerequisiteUnmet, strings.Join(names, "; "))
}

// Entry is a file found in the IPSW cache. Preflight does not open archives; an
// entry matches the supported pair only if FW-02's verification says so.
type Entry struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
}

// Space is a volume's identity and free space.
type Space struct {
	ID        string
	MountPath string
	Free      int64
}

// Probes observe the host. Tests replace them; DefaultProbes reads the real
// host and only ever reads.
type Probes struct {
	GOOS, GOARCH string
	// ProductVersion returns the macOS version, for example "27.0.1".
	ProductVersion func() (string, error)
	// Sysctl reads an integer sysctl.
	Sysctl func(name string) (int64, error)
	// Capabilities returns the required Virtualization interfaces this host
	// lacks.
	Capabilities func() (missing []string, err error)
	// ResearchModel reports whether this process may use the PV=3 model.
	ResearchModel func() (bool, error)
	// Entitlements returns the entitlements this executable carries.
	Entitlements func(ctx context.Context) (map[string]bool, error)
	// Command runs a read-only host command and returns its output.
	Command func(ctx context.Context, name string, args ...string) (string, error)
	// LookPath finds a tool.
	LookPath func(name string) (string, error)
	// Space reports the volume holding path, or its nearest existing parent.
	Space func(path string) (Space, error)
	// CacheEntries lists the IPSW cache without modifying it.
	CacheEntries func(dir string) ([]Entry, error)
}

// Defaults for the machine preflight checks against.
const (
	MinMacOSMajor = 27
	VMCPUs        = 8
	VMMemoryBytes = 8 << 30
)

// VMEntitlements are the entitlements vphone-cli's VM process carries
// (VPhoneVirtualization.entitlements at the pinned revision).
var VMEntitlements = []string{
	"com.apple.private.virtualization",
	"com.apple.private.virtualization.security-research",
	"com.apple.security.virtualization",
	"com.apple.vm.networking",
	"com.apple.private.bmk.allow",
}

// Space demands. The iPhone IPSW size is Apple's CDN Content-Length recorded
// in docs/research-artifacts/verification; the others are vphone-cli's
// measurements (Research/Host/disk_space.md at the pinned revision, iPhone17,3
// iOS 27.0 24A435 with cloudOS 23E5207q).
const (
	IPhoneIPSWBytes   = 12_343_857_610
	CloudOSIPSWBytes  = 1_200_000_000
	RestoreTreeBytes  = 11_000_000_000
	MachineDiskBytes  = 20_500_000_000
	MachineNVRAMBytes = 33_600_000
	MachineSEPBytes   = 512 << 10
	DiskLogicalBytes  = 64_000_000_000
	iphoneIPSWName    = "iPhone17,3_27.0.1_24A446_Restore.ipsw"
	planningFreeNote  = "About 15 GiB was free when the plan was written; that observation is not evidence and current free space is measured above."
)

// Run performs every check. It never fails as a whole: a probe error becomes an
// unverified result.
func Run(ctx context.Context, p Probes, paths Paths) Report {
	r := Report{Paths: paths}
	add := func(res Result) { r.Results = append(r.Results, res) }

	platform := p.GOOS + "/" + p.GOARCH
	if platform != "darwin/arm64" {
		add(Result{ID: "platform", Title: "Apple Silicon Mac", Status: Unsupported, Observed: platform,
			Remedy: "Run on a physical Apple Silicon Mac with macOS 27 or later.", Blocking: true})
		r.Notes = append(r.Notes, "Other checks were skipped on an unsupported platform.")
		return r
	}
	add(Result{ID: "platform", Title: "Apple Silicon Mac", Status: Supported, Observed: platform, Blocking: true})
	add(checkMacOS(p))
	add(checkNested(p))
	add(checkMemory(p))
	add(checkCPUs(p))
	add(checkCapabilities(p))
	entitled := checkEntitlements(ctx, p)
	add(entitled)
	add(checkResearchModel(p, entitled.Status == Supported))
	add(checkResearchGuests(ctx, p))
	add(checkCodeIntegrity(ctx, p))
	for _, t := range tools {
		add(checkTool(ctx, p, t))
	}
	r.Cache, r.Volumes = storage(p, paths, &r)
	r.Notes = append(r.Notes, planningFreeNote,
		"Preflight is read-only and is not evidence that a machine restores or boots.")
	return r
}

func checkMacOS(p Probes) Result {
	res := Result{ID: "macos", Title: "macOS 27 or later", Blocking: true}
	v, err := p.ProductVersion()
	if err != nil {
		res.Status, res.Observed = Unverified, err.Error()
		return res
	}
	res.Observed = "macOS " + v
	major, _, _ := strings.Cut(v, ".")
	if n, err := strconv.Atoi(major); err == nil && n >= MinMacOSMajor {
		res.Status = Supported
	} else {
		res.Status, res.Remedy = Unsupported, "Update the host to macOS 27 or later."
	}
	return res
}

func checkNested(p Probes) Result {
	res := Result{ID: "nested", Title: "Physical host, not a virtual machine", Blocking: true}
	v, err := p.Sysctl("kern.hv_vmm_present")
	switch {
	case err != nil:
		res.Status, res.Observed = Unverified, err.Error()
	case v != 0:
		res.Status, res.Observed = Unsupported, "running inside a virtual machine"
		res.Remedy = "PV=3 research guests do not run in a nested macOS VM; use a physical Mac."
	default:
		res.Status, res.Observed = Supported, "physical host"
	}
	return res
}

func checkMemory(p Probes) Result {
	res := Result{ID: "memory", Title: "Memory for an 8 GiB guest", Blocking: true}
	v, err := p.Sysctl("hw.memsize")
	switch {
	case err != nil:
		res.Status, res.Observed = Unverified, err.Error()
	case v <= VMMemoryBytes:
		res.Status, res.Observed = Unsupported, Size(v)+" installed"
		res.Remedy = "The default guest takes 8 GiB; use a host with more memory."
	default:
		res.Status, res.Observed = Supported, Size(v)+" installed"
	}
	return res
}

func checkCPUs(p Probes) Result {
	res := Result{ID: "cpus", Title: "CPUs for an 8-CPU guest"}
	v, err := p.Sysctl("hw.ncpu")
	switch {
	case err != nil:
		res.Status, res.Observed = Unverified, err.Error()
	case v < VMCPUs:
		res.Status, res.Observed = Unverified, strconv.FormatInt(v, 10)+" CPUs"
		res.Remedy = "Fewer host CPUs than the default guest; Virtualization validates the count at create."
	default:
		res.Status, res.Observed = Supported, strconv.FormatInt(v, 10)+" CPUs"
	}
	return res
}

func checkCapabilities(p Probes) Result {
	res := Result{ID: "virtualization", Title: "Virtualization interfaces", Blocking: true}
	missing, err := p.Capabilities()
	switch {
	case err != nil:
		res.Status, res.Observed = Unverified, err.Error()
	case len(missing) > 0:
		res.Status, res.Observed = Unsupported, "missing "+strings.Join(missing, ", ")
		res.Remedy = "This macOS build lacks interfaces the runner needs; see internal/host/native for the inventory."
	default:
		res.Status, res.Observed = Supported, "every required class and selector is present"
	}
	return res
}

func checkEntitlements(ctx context.Context, p Probes) Result {
	res := Result{ID: "entitlements", Title: "Virtualization entitlements on this executable"}
	have, err := p.Entitlements(ctx)
	if err != nil {
		res.Status, res.Observed = Unverified, err.Error()
		return res
	}
	var missing []string
	for _, e := range VMEntitlements {
		if !have[e] {
			missing = append(missing, e)
		}
	}
	if len(missing) == 0 {
		res.Status, res.Observed = Supported, "all present"
		return res
	}
	res.Status = Unverified
	res.Observed = "missing " + strings.Join(missing, ", ")
	res.Remedy = "The VM process must be signed with these entitlements before it can start a research guest."
	return res
}

func checkResearchModel(p Probes, entitled bool) Result {
	res := Result{ID: "research-model", Title: "PV=3 research hardware model"}
	ok, err := p.ResearchModel()
	switch {
	case err != nil:
		res.Status, res.Observed = Unverified, err.Error()
	case ok:
		res.Status, res.Observed = Supported, "supported for this process"
	case entitled:
		res.Status, res.Observed, res.Blocking = Unsupported, "refused for an entitled process", true
		res.Remedy = "Virtualization refuses the research model on this host even with the entitlements."
	default:
		res.Status, res.Observed = Unverified, "refused for this unentitled process; the signed VM process decides"
	}
	return res
}

func checkResearchGuests(ctx context.Context, p Probes) Result {
	res := Result{ID: "research-guests", Title: "Research guests allowed", Blocking: true,
		Remedy: "In macOS Recovery, run `csrutil allow-research-guests enable`, then restart."}
	out, err := p.Command(ctx, "/usr/bin/csrutil", "allow-research-guests", "status")
	low := strings.ToLower(out)
	switch {
	case err != nil:
		res.Status, res.Observed = Unverified, "csrutil: "+err.Error()
	case strings.Contains(low, "enabled"):
		res.Status, res.Observed, res.Remedy = Supported, strings.TrimSpace(out), ""
	case strings.Contains(low, "disabled"):
		res.Status, res.Observed = Unsupported, strings.TrimSpace(out)
	default:
		res.Status, res.Observed = Unverified, strings.TrimSpace(out)
	}
	return res
}

// checkCodeIntegrity reports SIP and AMFI as observed. Whether this host
// admits an ad hoc signed process carrying private entitlements is only known
// once the signed VM process starts, so the result is never supported here.
func checkCodeIntegrity(ctx context.Context, p Probes) Result {
	res := Result{ID: "code-integrity", Title: "Host admits the entitled VM process", Status: Unverified}
	sip, sipErr := p.Command(ctx, "/usr/bin/csrutil", "status")
	args, argsErr := p.Command(ctx, "/usr/sbin/nvram", "boot-args")
	var obs []string
	switch {
	case sipErr != nil:
		obs = append(obs, "SIP status unavailable: "+sipErr.Error())
	default:
		obs = append(obs, describeSIP(sip))
	}
	amfiOff := argsErr == nil && strings.Contains(args, "amfi_get_out_of_my_way=1")
	if amfiOff {
		obs = append(obs, "boot-args disable AMFI checks")
	} else {
		obs = append(obs, "AMFI enforcing (no amfi_get_out_of_my_way boot-arg)")
	}
	res.Observed = strings.Join(obs, "; ")
	if !amfiOff {
		res.Remedy = "AMFI rejects ad hoc signed binaries carrying private entitlements unless the owner relaxes host policy, " +
			"for example SIP disabled in Recovery and `sudo nvram boot-args=\"amfi_get_out_of_my_way=1\"`. " +
			"That is the machine owner's decision; this runner never changes it."
	}
	return res
}

func describeSIP(out string) string {
	first, _, _ := strings.Cut(strings.TrimSpace(out), "\n")
	first = strings.TrimSpace(strings.TrimPrefix(first, "System Integrity Protection status:"))
	var off []string
	for line := range strings.Lines(out) {
		name, value, ok := strings.Cut(strings.TrimSpace(line), ":")
		if ok && strings.TrimSpace(value) == "disabled" && name != "Apple Internal" {
			off = append(off, strings.ToLower(name))
		}
	}
	desc := "SIP " + strings.TrimSuffix(first, ".")
	if len(off) > 0 {
		desc += " (" + strings.Join(off, ", ") + " disabled)"
	}
	return desc
}

type tool struct {
	id, title, remedy string
	blocking          bool
	find              func(ctx context.Context, p Probes) (string, error)
}

func lookPath(name string) func(context.Context, Probes) (string, error) {
	return func(_ context.Context, p Probes) (string, error) { return p.LookPath(name) }
}

var tools = []tool{
	{id: "tool-codesign", title: "codesign", blocking: true, find: lookPath("codesign"),
		remedy: "codesign ships with macOS; restore it or the command-line tools."},
	{id: "tool-hdiutil", title: "hdiutil", blocking: true, find: lookPath("hdiutil"),
		remedy: "hdiutil ships with macOS."},
	{id: "tool-diskutil", title: "diskutil", blocking: true, find: lookPath("diskutil"),
		remedy: "diskutil ships with macOS."},
	{id: "tool-iphoneos-sdk", title: "iPhoneOS SDK for the guest daemon", blocking: true,
		remedy: "Install Xcode with its iOS platform and select it with xcode-select.",
		find: func(ctx context.Context, p Probes) (string, error) {
			out, err := p.Command(ctx, "/usr/bin/xcrun", "--sdk", "iphoneos", "--show-sdk-path")
			return strings.TrimSpace(out), err
		}},
}

func checkTool(ctx context.Context, p Probes, t tool) Result {
	res := Result{ID: t.id, Title: t.title, Blocking: t.blocking}
	path, err := t.find(ctx, p)
	if err != nil || path == "" {
		res.Status, res.Remedy = Unsupported, t.remedy
		res.Observed = "not found"
		if err != nil {
			res.Observed += ": " + err.Error()
		}
		return res
	}
	res.Status, res.Observed = Supported, path
	return res
}

func storage(p Probes, paths Paths, r *Report) ([]Entry, []Volume) {
	entries, err := p.CacheEntries(paths.IPSWCache)
	if err != nil {
		r.Results = append(r.Results, Result{ID: "ipsw-cache", Title: "IPSW cache readable", Status: Unverified, Observed: err.Error()})
	}
	cachedIPhone := slices.ContainsFunc(entries, func(e Entry) bool { return e.Name == iphoneIPSWName && e.Size == IPhoneIPSWBytes })

	demand := map[string][]Demand{
		"ipsw cache": {
			{What: "cloudOS 26.4 (23E5207q) IPSW", Bytes: CloudOSIPSWBytes, Basis: "vphone-cli measurement; exact size known after acquisition"},
		},
		"library": {
			{What: "restore tree while building", Bytes: RestoreTreeBytes, Basis: "vphone-cli measurement"},
			{What: "machine disk after first boot", Bytes: MachineDiskBytes, Basis: "vphone-cli measurement; 64 GB logical sparse"},
			{What: "NVRAM and SEP storage", Bytes: MachineNVRAMBytes + MachineSEPBytes, Basis: "vphone-cli measurement"},
		},
	}
	if !cachedIPhone {
		demand["ipsw cache"] = append([]Demand{{What: "iPhone17,3 iOS 27.0.1 (24A446) IPSW", Bytes: IPhoneIPSWBytes,
			Basis: "Apple CDN Content-Length recorded 2026-10-19"}}, demand["ipsw cache"]...)
	} else {
		r.Notes = append(r.Notes, "A cache entry has the iPhone IPSW's name and size; FW-02 verifies it before use.")
	}

	var volumes []Volume
	for _, labelled := range []struct{ label, path string }{
		{"ipsw cache", paths.IPSWCache}, {"library", paths.Library}, {"scratch", paths.Scratch},
	} {
		sp, err := p.Space(labelled.path)
		if err != nil {
			r.Results = append(r.Results, Result{ID: "space-" + strings.ReplaceAll(labelled.label, " ", "-"),
				Title: "Free space for the " + labelled.label, Status: Unverified, Observed: err.Error()})
			continue
		}
		i := slices.IndexFunc(volumes, func(v Volume) bool { return v.ID == sp.ID })
		if i < 0 {
			volumes = append(volumes, Volume{ID: sp.ID, MountPath: sp.MountPath, Free: sp.Free})
			i = len(volumes) - 1
		}
		volumes[i].Paths = append(volumes[i].Paths, labelled.label+": "+filepath.Clean(labelled.path))
		volumes[i].Demand = append(volumes[i].Demand, demand[labelled.label]...)
	}
	for _, v := range volumes {
		res := Result{ID: "space-" + v.ID, Title: "Free space on " + v.MountPath, Blocking: true,
			Observed: fmt.Sprintf("%s free, %s needed", Size(v.Free), Size(v.Need()))}
		switch {
		case v.Free < v.Need():
			res.Status = Unsupported
			res.Remedy = fmt.Sprintf("Free at least %s more, or point --ipsw-cache or --library at another volume.", Size(v.Need()-v.Free))
		case v.Free < v.Need()+DiskLogicalBytes-MachineDiskBytes:
			res.Status = Unverified
			res.Remedy = "Enough for a create, but the sparse disk can grow to 64 GB as the guest writes."
		default:
			res.Status = Supported
		}
		r.Results = append(r.Results, res)
	}
	return entries, volumes
}

// Size formats a byte count in GiB, or MiB below 1 GiB.
func Size(b int64) string {
	if b < 1<<30 {
		return strconv.FormatFloat(float64(b)/(1<<20), 'f', 1, 64) + " MiB"
	}
	return strconv.FormatFloat(float64(b)/(1<<30), 'f', 1, 64) + " GiB"
}
