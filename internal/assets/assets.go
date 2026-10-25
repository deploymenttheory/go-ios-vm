// Package assets is the lock manifest of the native exceptions and firmware
// inputs the runner uses: the vendored boot ROMs, the native guest components
// built from pinned source, the Apple GPU bundle extracted from cloudOS and the
// user-supplied IPSWs. Each entry records its role, exact source pin, digest
// policy and licence; digest-pinned files are verified before use.
//
// docs/port-inventory.md describes the inventory and the decisions behind it.
package assets

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/deploymenttheory/go-ios-vm/internal/firmware"
)

//go:embed lock.json
var lockJSON []byte

//go:embed rom
var vendored embed.FS

// Kind is what an asset is.
type Kind string

// Asset kinds.
const (
	KindROM           Kind = "rom"
	KindGuestNative   Kind = "guest-native"
	KindAppleFirmware Kind = "apple-firmware"
	KindUserFirmware  Kind = "user-firmware"
)

// Policy is how an asset is obtained.
type Policy string

// Acquisition policies.
const (
	PolicyVendored            Policy = "vendored"
	PolicyBuildFromSource     Policy = "build-from-source"
	PolicyExtractFromFirmware Policy = "extract-from-firmware"
	PolicyUserSupplied        Policy = "user-supplied"
)

// DigestPolicy is how an asset's content is verified.
type DigestPolicy string

// Digest policies.
const (
	// DigestPinned fixes the file's SHA-256 and size in the lock.
	DigestPinned DigestPolicy = "pinned"
	// DigestSourceTree fixes the digest of the source files the asset is built
	// from; the built digest is recorded when it is packaged.
	DigestSourceTree DigestPolicy = "source-tree"
	// DigestRecordedAtExtraction records digests when the asset is extracted
	// from verified firmware.
	DigestRecordedAtExtraction DigestPolicy = "recorded-at-extraction"
	// DigestVerifiedAtAcquisition verifies the firmware identity and download
	// digest before the archive is used.
	DigestVerifiedAtAcquisition DigestPolicy = "verified-at-acquisition"
)

// Use is an asset's status for the first release.
type Use string

// First-release status.
const (
	// UseRequired assets are needed by the first release.
	UseRequired Use = "required"
	// UseCandidate assets are installed by upstream's standard preset; whether
	// the first release needs them is still to be established on a live guest.
	UseCandidate Use = "candidate"
)

// Source pins where an asset comes from.
type Source struct {
	Repository string   `json:"repository,omitempty"`
	Revision   string   `json:"revision,omitempty"`
	Path       string   `json:"path,omitempty"`
	Files      []string `json:"files,omitempty"`
}

// Firmware names the firmware build an asset is or comes from.
type Firmware struct {
	OS      string `json:"os"`
	Device  string `json:"device,omitempty"`
	Version string `json:"version"`
	Build   string `json:"build"`
}

// Identity returns f as a firmware identity.
func (f Firmware) Identity() firmware.Identity {
	return firmware.Identity{OS: f.OS, Device: f.Device, Version: f.Version, Build: f.Build}
}

// Asset is one lock entry.
type Asset struct {
	ID           string       `json:"id"`
	Kind         Kind         `json:"kind"`
	FirstRelease Use          `json:"first_release"`
	Role         string       `json:"role"`
	Policy       Policy       `json:"policy"`
	DigestPolicy DigestPolicy `json:"digest_policy"`
	// Path is a vendored asset's path within this package.
	Path       string `json:"path,omitempty"`
	SHA256     string `json:"sha256,omitempty"`
	Size       int64  `json:"size,omitempty"`
	TreeSHA256 string `json:"tree_sha256,omitempty"`
	// Source is where the asset is built or taken from.
	Source Source `json:"source"`
	// ObtainedFrom is where a vendored copy was taken from, when that differs
	// from its source.
	ObtainedFrom *Source `json:"obtained_from,omitempty"`
	// Origin is where ported source originally came from.
	Origin      *Source           `json:"origin,omitempty"`
	Firmware    *Firmware         `json:"firmware,omitempty"`
	Expected    map[string]string `json:"expected,omitempty"`
	Build       string            `json:"build"`
	InstallPath string            `json:"install_path,omitempty"`
	License     string            `json:"license"`
	Notices     []string          `json:"notices,omitempty"`
	Notes       string            `json:"notes,omitempty"`
}

// Lock is the parsed lock manifest.
type Lock struct {
	SchemaVersion int `json:"schema_version"`
	Upstream      struct {
		Source
		License string `json:"license"`
		Notice  string `json:"notice"`
	} `json:"upstream"`
	Assets []Asset `json:"assets"`
}

// Errors.
var (
	ErrInvalidLock = errors.New("invalid asset lock")
	ErrUnknown     = errors.New("unknown asset")
	ErrMissing     = errors.New("asset missing")
	ErrMismatch    = errors.New("asset does not match its lock")
	ErrNotPinned   = errors.New("asset has no pinned file digest")
)

// Load parses and validates the embedded lock manifest.
func Load() (*Lock, error) {
	var l Lock
	dec := json.NewDecoder(bytes.NewReader(lockJSON))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&l); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidLock, err)
	}
	if err := l.validate(); err != nil {
		return nil, err
	}
	return &l, nil
}

// Asset returns the entry with the given id.
func (l *Lock) Asset(id string) (Asset, error) {
	for _, a := range l.Assets {
		if a.ID == id {
			return a, nil
		}
	}
	return Asset{}, fmt.Errorf("%w %q", ErrUnknown, id)
}

func (l *Lock) validate() error {
	if l.SchemaVersion != 1 {
		return fmt.Errorf("%w: schema version %d", ErrInvalidLock, l.SchemaVersion)
	}
	if !isRevision(l.Upstream.Revision) || l.Upstream.License == "" {
		return fmt.Errorf("%w: upstream needs a revision and licence", ErrInvalidLock)
	}
	seen := map[string]bool{}
	for _, a := range l.Assets {
		if seen[a.ID] {
			return fmt.Errorf("%w: duplicate asset %q", ErrInvalidLock, a.ID)
		}
		seen[a.ID] = true
		if err := a.validate(); err != nil {
			return fmt.Errorf("%w: %s: %w", ErrInvalidLock, a.ID, err)
		}
	}
	return nil
}

var errField = errors.New("field")

func (a Asset) validate() error {
	if a.ID == "" || a.Role == "" || a.Build == "" || a.License == "" {
		return fmt.Errorf("%w: id, role, build and license are required", errField)
	}
	if !slices.Contains([]Use{UseRequired, UseCandidate}, a.FirstRelease) {
		return fmt.Errorf("%w: first_release %q", errField, a.FirstRelease)
	}
	switch a.Kind {
	case KindROM, KindGuestNative:
		if !strings.HasPrefix(a.Source.Repository, "https://") || !isRevision(a.Source.Revision) {
			return fmt.Errorf("%w: a native exception needs an exact source repository and revision", errField)
		}
	case KindAppleFirmware, KindUserFirmware:
		if a.Firmware == nil {
			return fmt.Errorf("%w: firmware identity is required", errField)
		}
	default:
		return fmt.Errorf("%w: kind %q", errField, a.Kind)
	}
	switch a.DigestPolicy {
	case DigestPinned:
		if a.Policy != PolicyVendored || a.Path == "" || !isSHA256(a.SHA256) || a.Size <= 0 || len(a.Notices) == 0 {
			return fmt.Errorf("%w: a pinned asset is vendored with a path, SHA-256, size and notices", errField)
		}
		for _, n := range append([]string{a.Path}, a.Notices...) {
			if _, err := fs.Stat(vendored, n); err != nil {
				return fmt.Errorf("vendored file: %w", err)
			}
		}
	case DigestSourceTree:
		if a.Policy != PolicyBuildFromSource || !isSHA256(a.TreeSHA256) || len(a.Source.Files) == 0 {
			return fmt.Errorf("%w: a source-tree asset is built from listed files with a tree SHA-256", errField)
		}
	case DigestRecordedAtExtraction, DigestVerifiedAtAcquisition:
		if a.Firmware == nil {
			return fmt.Errorf("%w: digest policy %s needs a firmware identity", errField, a.DigestPolicy)
		}
	default:
		return fmt.Errorf("%w: digest_policy %q", errField, a.DigestPolicy)
	}
	return nil
}

// ROM returns a vendored ROM image after verifying it against the lock.
func ROM(id string) ([]byte, error) {
	l, err := Load()
	if err != nil {
		return nil, err
	}
	a, err := l.Asset(id)
	if err != nil {
		return nil, err
	}
	if a.Kind != KindROM || a.Policy != PolicyVendored {
		return nil, fmt.Errorf("%w: %s is not a vendored ROM", ErrUnknown, id)
	}
	data, err := vendored.ReadFile(a.Path)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %w", ErrMissing, id, err)
	}
	if err := a.check(int64(len(data)), sha256.Sum256(data)); err != nil {
		return nil, err
	}
	return data, nil
}

// VerifyFile checks that the file at path is a's pinned content. A missing file
// returns an error wrapping both ErrMissing and fs.ErrNotExist; different
// content returns ErrMismatch.
func VerifyFile(a Asset, path string) error {
	if a.DigestPolicy != DigestPinned {
		return fmt.Errorf("%w: %s uses %s", ErrNotPinned, a.ID, a.DigestPolicy)
	}
	f, err := os.Open(path) //nolint:gosec // the path is the machine's own asset copy
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("%w: %s: %w", ErrMissing, a.ID, err)
		}
		return fmt.Errorf("open %s: %w", a.ID, err)
	}
	defer f.Close() //nolint:errcheck // read-only
	info, err := f.Stat()
	if err != nil {
		return fmt.Errorf("stat %s: %w", a.ID, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%w: %s: %s is not a regular file", ErrMismatch, a.ID, path)
	}
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return fmt.Errorf("read %s: %w", a.ID, err)
	}
	return a.check(n, [sha256.Size]byte(h.Sum(nil)))
}

func (a Asset) check(size int64, sum [sha256.Size]byte) error {
	if got := hex.EncodeToString(sum[:]); size != a.Size || got != a.SHA256 {
		return fmt.Errorf("%w: %s is %d bytes with SHA-256 %s, want %d bytes with %s", ErrMismatch, a.ID, size, got, a.Size, a.SHA256)
	}
	return nil
}

// SourceTreeDigest returns the tree digest of files under root: the SHA-256 of
// the lines "<file SHA-256>  <path>\n", one per file in bytewise path order,
// which is what `shasum -a 256 <files> | LC_ALL=C sort -k2 | shasum -a 256`
// computes. Paths use forward slashes relative to root.
func SourceTreeDigest(root string, files []string) (string, error) {
	sorted := slices.Clone(files)
	slices.Sort(sorted)
	var lines strings.Builder
	for _, name := range sorted {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name))) //nolint:gosec // paths come from the lock
		if err != nil {
			return "", fmt.Errorf("source tree: %w", err)
		}
		sum := sha256.Sum256(data)
		fmt.Fprintf(&lines, "%s  %s\n", hex.EncodeToString(sum[:]), name)
	}
	sum := sha256.Sum256([]byte(lines.String()))
	return hex.EncodeToString(sum[:]), nil
}

func isSHA256(s string) bool { return len(s) == 64 && isLowerHex(s) }

func isRevision(s string) bool { return len(s) == 40 && isLowerHex(s) }

func isLowerHex(s string) bool {
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
