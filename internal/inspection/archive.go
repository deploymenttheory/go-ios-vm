// Package inspection adapts read-only firmware parsers to source-addressable evidence.
package inspection

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"

	"howett.net/plist"

	"github.com/deploymenttheory/go-ios-vm/internal/model"
)

var (
	errInvalidMemberLimit = errors.New(
		"ZIP member limit exceeds signed reader range",
	)
	errIPSWMustBeARegularFile = errors.New(
		"IPSW must be a regular file",
	)
	errZIPMemberBudgetExceeded = errors.New(
		"ZIP member budget exceeded",
	)
	errAmbiguousDuplicateZIPMember = errors.New(
		"ambiguous duplicate ZIP member",
	)
	errBuildManifestMissingTargetIdentity = errors.New(
		"BuildManifest missing target identity",
	)
	errSourceChangedDuringInspection = errors.New(
		"source changed during inspection",
	)
	errMissingZIPMember            = errors.New("missing ZIP member")
	errZIPMemberExceedsInputBudget = errors.New(
		"ZIP member exceeds input budget",
	)
	errZIPMemberSizeMismatch = errors.New(
		"ZIP member size mismatch",
	)
	errManifestTargetMismatch = errors.New(
		"BuildManifest does not match requested product/version/build",
	)
)

const ManifestLimit = 32 << 20

type Manifest struct {
	ProductVersion        string
	ProductBuildVersion   string
	SupportedProductTypes []string
	BuildIdentities       []struct {
		Info     struct{ DeviceClass, Variant string }
		Manifest map[string]struct{ Info struct{ Path string } }
	}
}

type Archive struct {
	file     *os.File
	original os.FileInfo
	ZIP      *zip.Reader
	Artifact model.Artifact
	Targets  []model.Target
	manifest []byte
	members  map[string]int
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, fmt.Errorf("read cancelled: %w", err)
	}
	return r.r.Read(p) //nolint:wrapcheck // io.Reader requires the original io.EOF sentinel.
}

func Open(ctx context.Context, path string) (*Archive, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("Open: filepath.Abs: %w", err)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("Open: os.Open: %w", err)
	}
	a := &Archive{file: f, members: map[string]int{}}
	ok := false
	defer func() {
		if !ok {
			f.Close()
		}
	}()
	s, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("Open: Stat: %w", err)
	}
	if !s.Mode().IsRegular() {
		return nil, errIPSWMustBeARegularFile
	}
	a.original = s
	h := sha256.New()
	if _, err = io.Copy(h, contextReader{ctx, f}); err != nil {
		return nil, fmt.Errorf("Open: io.Copy: %w", err)
	}
	digest := hex.EncodeToString(h.Sum(nil))
	a.Artifact = model.Artifact{ID: digest, SHA256: digest, Size: s.Size(), Path: path}
	a.ZIP, err = zip.NewReader(f, s.Size())
	if err != nil {
		return nil, fmt.Errorf("IPSW ZIP: %w", err)
	}
	if len(a.ZIP.File) > 200000 {
		return nil, errZIPMemberBudgetExceeded
	}
	for i, m := range a.ZIP.File {
		if _, exists := a.members[m.Name]; exists {
			return nil, fmt.Errorf("%w: %q", errAmbiguousDuplicateZIPMember, m.Name)
		}
		a.members[m.Name] = i
	}
	a.manifest, _, err = a.Read(ctx, "BuildManifest.plist", ManifestLimit)
	if err != nil {
		return nil, err
	}
	var manifest Manifest
	if _, err = plist.Unmarshal(a.manifest, &manifest); err != nil {
		return nil, fmt.Errorf("BuildManifest: %w", err)
	}
	if manifest.ProductVersion == "" || manifest.ProductBuildVersion == "" ||
		len(manifest.BuildIdentities) == 0 ||
		len(manifest.SupportedProductTypes) == 0 {
		return nil, errBuildManifestMissingTargetIdentity
	}
	for i, b := range manifest.BuildIdentities {
		t := model.Target{
			ID:         model.ID(digest, strconv.Itoa(i)),
			ArtifactID: digest,
			Identity:   i,
			Products:   manifest.SupportedProductTypes,
			Board:      b.Info.DeviceClass,
			Version:    manifest.ProductVersion,
			Build:      manifest.ProductBuildVersion,
			Variant:    b.Info.Variant,
			Components: map[string]string{},
		}
		for name, c := range b.Manifest {
			if c.Info.Path != "" {
				t.Components[name] = c.Info.Path
			}
		}
		a.Targets = append(a.Targets, t)
	}
	if err = a.Unchanged(); err != nil {
		return nil, err
	}
	ok = true
	return a, nil
}

func (a *Archive) Close() error {
	if err := a.file.Close(); err != nil {
		return fmt.Errorf("Close: Close: %w", err)
	}
	return nil
}

func (a *Archive) Unchanged() error {
	s, err := a.file.Stat()
	if err != nil {
		return fmt.Errorf("Unchanged: Stat: %w", err)
	}
	if s.Size() != a.original.Size() || !s.ModTime().Equal(a.original.ModTime()) {
		return errSourceChangedDuringInspection
	}
	return nil
}

func (a *Archive) Read(ctx context.Context, name string, limit uint64) ([]byte, int, error) {
	if limit >= math.MaxInt64 {
		return nil, 0, errInvalidMemberLimit
	}
	i, ok := a.members[name]
	if !ok {
		return nil, 0, fmt.Errorf("%w: %q", errMissingZIPMember, name)
	}
	f := a.ZIP.File[i]
	if f.UncompressedSize64 > limit {
		return nil, i, fmt.Errorf(
			"%w: %s exceeds %d bytes",
			errZIPMemberExceedsInputBudget,
			name,
			limit,
		)
	}
	r, err := f.Open()
	if err != nil {
		return nil, i, fmt.Errorf("read ZIP member %q: %w", name, err)
	}
	defer r.Close()
	b, err := io.ReadAll(io.LimitReader(contextReader{ctx, r}, int64(limit)+1))
	if err != nil {
		return nil, i, fmt.Errorf("read ZIP member %q: %w", name, err)
	}
	if uint64(len(b)) > limit || uint64(len(b)) != f.UncompressedSize64 {
		return nil, i, errZIPMemberSizeMismatch
	}
	return b, i, nil
}

func (a *Archive) Verify(product, version, build string) error {
	for _, t := range a.Targets {
		for _, p := range t.Products {
			if p == product && (version == "" || t.Version == version) &&
				(build == "" || t.Build == build) {
				return nil
			}
		}
	}
	return errManifestTargetMismatch
}
