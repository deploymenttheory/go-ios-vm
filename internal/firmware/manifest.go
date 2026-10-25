package firmware

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"howett.net/plist"
)

// manifestName is the restore archive's build manifest.
const manifestName = "BuildManifest.plist"

// maxManifestBytes bounds how much of a manifest is read; real manifests are
// around 1 MiB.
const maxManifestBytes = 32 << 20

// CloudOSBoard is the device class a cloudOS archive must carry for the
// supported pair: the vphone600 identity its kernel, SEP and device tree come
// from.
const CloudOSBoard = "vphone600ap"

// Errors from archive inspection.
var (
	ErrNotArchive       = errors.New("not a restore archive")
	ErrManifest         = errors.New("invalid build manifest")
	ErrIdentityMismatch = errors.New("firmware identity does not match")
)

// Manifest is the part of a build manifest that identifies an archive.
type Manifest struct {
	ProductVersion        string          `plist:"ProductVersion" json:"product_version"`
	ProductBuildVersion   string          `plist:"ProductBuildVersion" json:"product_build_version"`
	SupportedProductTypes []string        `plist:"SupportedProductTypes" json:"supported_product_types"`
	BuildIdentities       []BuildIdentity `plist:"BuildIdentities" json:"-"`
}

// BuildIdentity is one restore identity of a manifest.
type BuildIdentity struct {
	ApBoardID string `plist:"ApBoardID"`
	ApChipID  string `plist:"ApChipID"`
	Info      struct {
		DeviceClass string `plist:"DeviceClass"`
		Variant     string `plist:"Variant"`
	} `plist:"Info"`
}

// DeviceClasses lists the manifest's distinct device classes.
func (m Manifest) DeviceClasses() []string {
	var classes []string
	for _, bi := range m.BuildIdentities {
		if c := bi.Info.DeviceClass; c != "" && !slices.Contains(classes, c) {
			classes = append(classes, c)
		}
	}
	slices.Sort(classes)
	return classes
}

// ReadManifest reads the build manifest of the restore archive in r without
// extracting anything else. Exactly one manifest must be present at the
// archive root.
func ReadManifest(r io.ReaderAt, size int64) (Manifest, error) {
	zr, err := zip.NewReader(r, size)
	if err != nil {
		return Manifest{}, fmt.Errorf("%w: %w", ErrNotArchive, err)
	}
	var found *zip.File
	for _, f := range zr.File {
		if f.Name != manifestName {
			continue
		}
		if found != nil {
			return Manifest{}, fmt.Errorf("%w: more than one %s", ErrManifest, manifestName)
		}
		found = f
	}
	if found == nil {
		return Manifest{}, fmt.Errorf("%w: no %s", ErrManifest, manifestName)
	}
	if found.UncompressedSize64 > maxManifestBytes {
		return Manifest{}, fmt.Errorf("%w: %s is %d bytes", ErrManifest, manifestName, found.UncompressedSize64)
	}
	rc, err := found.Open()
	if err != nil {
		return Manifest{}, fmt.Errorf("%w: %w", ErrManifest, err)
	}
	defer rc.Close() //nolint:errcheck // read-only
	data, err := io.ReadAll(io.LimitReader(rc, maxManifestBytes+1))
	if err != nil {
		return Manifest{}, fmt.Errorf("%w: %w", ErrManifest, err)
	}
	if len(data) > maxManifestBytes {
		return Manifest{}, fmt.Errorf("%w: %s exceeds %d bytes", ErrManifest, manifestName, maxManifestBytes)
	}
	var m Manifest
	if _, err := plist.Unmarshal(data, &m); err != nil {
		return Manifest{}, fmt.Errorf("%w: %w", ErrManifest, err)
	}
	if m.ProductBuildVersion == "" || m.ProductVersion == "" {
		return Manifest{}, fmt.Errorf("%w: no product version or build", ErrManifest)
	}
	return m, nil
}

// Match reports whether the manifest is exactly id. An iOS archive must list
// id's device; a cloudOS archive must carry CloudOSBoard.
func (m Manifest) Match(id Identity) error {
	var diffs []string
	if m.ProductBuildVersion != id.Build {
		diffs = append(diffs, fmt.Sprintf("build %s, want %s", m.ProductBuildVersion, id.Build))
	}
	if m.ProductVersion != id.Version {
		diffs = append(diffs, fmt.Sprintf("version %s, want %s", m.ProductVersion, id.Version))
	}
	if id.Device != "" && !slices.Contains(m.SupportedProductTypes, id.Device) {
		diffs = append(diffs, fmt.Sprintf("devices %v, want %s", m.SupportedProductTypes, id.Device))
	}
	if id.OS == SupportedPair.CloudOS.OS && !slices.Contains(m.DeviceClasses(), CloudOSBoard) {
		diffs = append(diffs, fmt.Sprintf("device classes %v, want %s", m.DeviceClasses(), CloudOSBoard))
	}
	if len(diffs) > 0 {
		return fmt.Errorf("%w %s: %s", ErrIdentityMismatch, id, strings.Join(diffs, "; "))
	}
	return nil
}
