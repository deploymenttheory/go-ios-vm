package assets

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/deploymenttheory/go-ios-vm/internal/buildinfo"
	"github.com/deploymenttheory/go-ios-vm/internal/firmware"
)

func load(t *testing.T) *Lock {
	t.Helper()
	l, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func TestLockPinsUpstreamRevision(t *testing.T) {
	l := load(t)
	if l.Upstream.Revision != buildinfo.VPhoneCLIRevision || l.Upstream.License != "MIT" {
		t.Fatalf("upstream %+v, want vphone-cli %s under MIT", l.Upstream, buildinfo.VPhoneCLIRevision)
	}
	for _, a := range l.Assets {
		for _, s := range []*Source{&a.Source, a.ObtainedFrom} {
			if s != nil && s.Repository == "https://github.com/Lakr233/vphone-cli" && s.Revision != buildinfo.VPhoneCLIRevision {
				t.Errorf("%s: vphone-cli source at %s, want %s", a.ID, s.Revision, buildinfo.VPhoneCLIRevision)
			}
		}
	}
}

func TestLockCoversTheSupportedPair(t *testing.T) {
	l := load(t)
	for id, want := range map[string]firmware.Identity{
		"iphone-ipsw":         firmware.SupportedPair.IPhone,
		"cloudos-ipsw":        firmware.SupportedPair.CloudOS,
		"paravirt-gpu-bundle": firmware.SupportedPair.CloudOS,
	} {
		a, err := l.Asset(id)
		if err != nil {
			t.Fatal(err)
		}
		if got := a.Firmware.Identity(); got != want {
			t.Errorf("%s: firmware %v, want %v", id, got, want)
		}
	}
}

func TestEveryNativeExceptionIsPinned(t *testing.T) {
	for _, a := range load(t).Assets {
		if a.Kind != KindROM && a.Kind != KindGuestNative {
			continue
		}
		if a.Role == "" || a.License == "" || a.Build == "" || !isRevision(a.Source.Revision) {
			t.Errorf("%s: role, licence, build and source revision are required: %+v", a.ID, a)
		}
		switch a.DigestPolicy {
		case DigestPinned:
			if len(a.Notices) == 0 {
				t.Errorf("%s: vendored without notices", a.ID)
			}
		case DigestSourceTree:
			if len(a.Source.Files) == 0 || !isSHA256(a.TreeSHA256) {
				t.Errorf("%s: source tree without files or digest", a.ID)
			}
		default:
			t.Errorf("%s: native exception with digest policy %s", a.ID, a.DigestPolicy)
		}
	}
}

func TestVendoredROMsMatchTheirLock(t *testing.T) {
	for _, id := range []string{"openavb-rom", "opensep-rom"} {
		data, err := ROM(id)
		if err != nil {
			t.Fatalf("ROM(%q): %v", id, err)
		}
		if len(data) == 0 {
			t.Fatalf("ROM(%q) is empty", id)
		}
	}
	if _, err := ROM("gpu-compiler-plugin"); !errors.Is(err, ErrUnknown) {
		t.Errorf("ROM of a non-ROM asset = %v, want ErrUnknown", err)
	}
	if _, err := ROM("avpbooter"); !errors.Is(err, ErrUnknown) {
		t.Errorf("ROM of an unknown id = %v, want ErrUnknown", err)
	}
}

func TestVerifyFile(t *testing.T) {
	l := load(t)
	rom, err := l.Asset("openavb-rom")
	if err != nil {
		t.Fatal(err)
	}
	data, err := ROM(rom.ID)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	write := func(name string, b []byte) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, b, 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	flipped := append([]byte(nil), data...)
	flipped[len(flipped)/2] ^= 0xff

	if err := VerifyFile(rom, write("good.bin", data)); err != nil {
		t.Errorf("matching copy: %v", err)
	}
	if err := VerifyFile(rom, write("flipped.bin", flipped)); !errors.Is(err, ErrMismatch) {
		t.Errorf("changed byte: %v, want ErrMismatch", err)
	}
	if err := VerifyFile(rom, write("short.bin", data[:len(data)-1])); !errors.Is(err, ErrMismatch) {
		t.Errorf("truncated copy: %v, want ErrMismatch", err)
	}
	if err := VerifyFile(rom, dir); !errors.Is(err, ErrMismatch) {
		t.Errorf("directory: %v, want ErrMismatch", err)
	}
	err = VerifyFile(rom, filepath.Join(dir, "absent.bin"))
	if !errors.Is(err, ErrMissing) || !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("absent copy: %v, want ErrMissing and fs.ErrNotExist", err)
	}
	plugin, err := l.Asset("gpu-compiler-plugin")
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyFile(plugin, write("plugin.dylib", data)); !errors.Is(err, ErrNotPinned) {
		t.Errorf("source-tree asset: %v, want ErrNotPinned", err)
	}
}

func TestSourceTreeDigest(t *testing.T) {
	root := t.TempDir()
	for name, content := range map[string]string{"src/b.c": "int a;\n", "src/sub/a.h": "#define X 1\n"} {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// Computed independently with
	// shasum -a 256 src/sub/a.h src/b.c | LC_ALL=C sort -k2 | shasum -a 256
	const want = "a6e4dfb52f07182b7b8eb0de0f03db8191c39c9bf98caafa3440310763d555cb"
	got, err := SourceTreeDigest(root, []string{"src/sub/a.h", "src/b.c"})
	if err != nil || got != want {
		t.Fatalf("SourceTreeDigest = %s, %v; want %s", got, err, want)
	}
	if _, err := SourceTreeDigest(root, []string{"src/missing.c"}); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing file: %v, want fs.ErrNotExist", err)
	}
}

func TestLockValidationRejectsIncompleteEntries(t *testing.T) {
	good, err := load(t).Asset("openavb-rom")
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(nil)
	tests := map[string]func(*Asset){
		"no role":             func(a *Asset) { a.Role = "" },
		"no licence":          func(a *Asset) { a.License = "" },
		"short revision":      func(a *Asset) { a.Source.Revision = "65a776e" },
		"no notices":          func(a *Asset) { a.Notices = nil },
		"missing vendored":    func(a *Asset) { a.Path = "rom/OpenAVB/absent.bin" },
		"bad digest":          func(a *Asset) { a.SHA256 = "abc" },
		"unknown policy":      func(a *Asset) { a.DigestPolicy = "trusted" },
		"unknown first use":   func(a *Asset) { a.FirstRelease = "maybe" },
		"tree without digest": func(a *Asset) { a.DigestPolicy, a.Policy, a.TreeSHA256 = DigestSourceTree, PolicyBuildFromSource, "" },
		"firmware no identity": func(a *Asset) {
			a.Kind, a.DigestPolicy, a.Firmware, a.SHA256 = KindUserFirmware, DigestVerifiedAtAcquisition, nil, hex.EncodeToString(sum[:])
		},
	}
	for name, mutate := range tests {
		a := good
		a.Notices = append([]string(nil), good.Notices...)
		mutate(&a)
		if err := a.validate(); err == nil {
			t.Errorf("%s: validate() = nil", name)
		}
	}
	if err := good.validate(); err != nil {
		t.Fatalf("unmodified entry: %v", err)
	}
}

// TestNoUnclassifiedBinaries checks every tracked file: binary content must be
// a vendored lock entry, and firmware, ticket or signing file types are never
// tracked.
func TestNoUnclassifiedBinaries(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not on PATH")
	}
	out, err := exec.CommandContext(t.Context(), git, "-C", "../..", "ls-files", "-z").Output()
	if err != nil {
		t.Skipf("not a git checkout: %v", err)
	}
	allowed := map[string]bool{}
	for _, a := range load(t).Assets {
		if a.Policy == PolicyVendored {
			allowed["internal/assets/"+a.Path] = true
		}
	}
	forbidden := regexp.MustCompile(`(?i)\.(ipsw|shsh2?|im4[pmr]|img4|dmg|aea|p12|pem|key|mobileprovision|dylib)$`)
	for _, name := range strings.Split(strings.TrimRight(string(out), "\x00"), "\x00") {
		if forbidden.MatchString(name) {
			t.Errorf("%s: firmware, ticket or signing material must not be tracked", name)
			continue
		}
		data, err := os.ReadFile(filepath.Join("../..", name))
		if err != nil {
			continue // deleted in the working tree
		}
		if bytes.IndexByte(data[:min(len(data), 8000)], 0) >= 0 && !allowed[name] {
			t.Errorf("%s: binary file not classified in internal/assets/lock.json", name)
		}
	}
}
