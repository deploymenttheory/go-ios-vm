package inspection

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"sort"

	"howett.net/plist"
)

// ManifestProbe establishes only the identity claimed by the remote ZIP's
// BuildManifest. A full download and checksum are required before publication.
type ManifestIdentity struct {
	Index   int    `json:"index"`
	Board   string `json:"board"`
	Variant string `json:"variant"`
}

type ManifestProbe struct {
	Version          string             `json:"version"`
	Build            string             `json:"build"`
	Products         []string           `json:"products"`
	Boards           []string           `json:"boards"`
	Variants         []string           `json:"variants"`
	Identities       int                `json:"identities"`
	BuildIdentities  []ManifestIdentity `json:"build_identities"`
	ManifestSHA256   string             `json:"manifest_sha256"`
	ManifestBytes    int                `json:"manifest_bytes"`
	RawBuildManifest []byte             `json:"-"`
}

func ProbeManifest(
	ctx context.Context,
	reader io.ReaderAt,
	size int64,
	product, version, build string,
) (ManifestProbe, error) {
	var result ManifestProbe
	z, err := zip.NewReader(reader, size)
	if err != nil {
		return result, fmt.Errorf("ProbeManifest: zip.NewReader: %w", err)
	}
	if len(z.File) > 200000 {
		return result, errZIPMemberBudgetExceeded
	}
	a := &Archive{ZIP: z, members: make(map[string]int, len(z.File))}
	for i, member := range z.File {
		if _, exists := a.members[member.Name]; exists {
			return result, fmt.Errorf("%w: %q", errAmbiguousDuplicateZIPMember, member.Name)
		}
		a.members[member.Name] = i
	}
	data, _, err := a.Read(ctx, "BuildManifest.plist", ManifestLimit)
	if err != nil {
		return result, err
	}
	var manifest Manifest
	if _, err := plist.Unmarshal(data, &manifest); err != nil {
		return result, fmt.Errorf("ProbeManifest: plist.Unmarshal: %w", err)
	}
	if manifest.ProductVersion == "" || manifest.ProductBuildVersion == "" ||
		len(manifest.SupportedProductTypes) == 0 || len(manifest.BuildIdentities) == 0 {
		return result, errBuildManifestMissingTargetIdentity
	}
	foundProduct := false
	for _, p := range manifest.SupportedProductTypes {
		if p == product {
			foundProduct = true
		}
	}
	if !foundProduct || version != "" && version != manifest.ProductVersion ||
		build != "" && build != manifest.ProductBuildVersion {
		return result, errManifestTargetMismatch
	}
	boards := map[string]bool{}
	variants := map[string]bool{}
	for index, identity := range manifest.BuildIdentities {
		result.BuildIdentities = append(result.BuildIdentities, ManifestIdentity{
			Index: index, Board: identity.Info.DeviceClass, Variant: identity.Info.Variant,
		})
		if identity.Info.DeviceClass != "" {
			boards[identity.Info.DeviceClass] = true
		}
		if identity.Info.Variant != "" {
			variants[identity.Info.Variant] = true
		}
	}
	for board := range boards {
		result.Boards = append(result.Boards, board)
	}
	for variant := range variants {
		result.Variants = append(result.Variants, variant)
	}
	sort.Strings(result.Boards)
	sort.Strings(result.Variants)
	digest := sha256.Sum256(data)
	result.Version = manifest.ProductVersion
	result.Build = manifest.ProductBuildVersion
	result.Products = manifest.SupportedProductTypes
	result.Identities = len(manifest.BuildIdentities)
	result.ManifestSHA256 = hex.EncodeToString(digest[:])
	result.ManifestBytes = len(data)
	result.RawBuildManifest = data
	return result, nil
}
