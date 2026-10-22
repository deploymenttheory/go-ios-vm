package inspection

import (
	"bytes"
	"context"
	"debug/macho"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/blacktop/ipsw/pkg/img4"

	"github.com/deploymenttheory/go-ios-vm/internal/model"
)

var (
	errIdentityIndexOutsideBuildManifest = errors.New(
		"identity index outside BuildManifest",
	)
	errEncryptedPayload = errors.New(
		"encrypted IM4P payload; no plaintext inspection",
	)
	errShortIM4PPayload = errors.New(
		"short IM4P payload",
	)
	errLZFSEDecodeFailed = errors.New(
		"LZFSE decode failed",
	)
	errLZFSEExceedsOutputBudget = errors.New(
		"LZFSE exceeds output budget",
	)
	errLZSSAdapterIsNotQualified = errors.New(
		"LZSS adapter is not qualified",
	)
	errUnsupportedIM4PCompressionDeclaration = errors.New(
		"unsupported IM4P compression declaration",
	)
	errPayloadExceedsOutputBudget = errors.New(
		"payload exceeds output budget",
	)
	errPrelinkedMetadataMissing = errors.New(
		"prelinked driver metadata not found; fileset inspection is not implemented",
	)
	errDriverMetadataExceeds16MiB = errors.New(
		"driver metadata exceeds 16 MiB",
	)
	errDriverHasNoBundleIdentity = errors.New(
		"driver has no bundle identity",
	)
)

const (
	Extractor   = "go-ios-vm/static-v1;ipsw=12847a212ba1;lzfse-cgo=v1.2.0"
	TreeLimit   = 16 << 20
	KernelLimit = 128 << 20
)

type Result struct {
	Observations []model.Observation
	Coverage     []model.Coverage
}

func (a *Archive) Inspect(ctx context.Context, identity int) (Result, error) {
	result := Result{Observations: []model.Observation{}, Coverage: []model.Coverage{}}
	err := a.InspectEach(
		ctx,
		identity,
		nil,
		func(_ string, observations []model.Observation, coverage model.Coverage) error {
			result.Observations = append(result.Observations, observations...)
			result.Coverage = append(result.Coverage, coverage)
			return nil
		},
	)
	return result, err
}

func decodePayload(ctx context.Context, raw []byte, limit int) ([]byte, []string, error) {
	p, err := img4.ParsePayload(raw)
	if err != nil {
		return nil, nil, fmt.Errorf("decodePayload: img4.ParsePayload: %w", err)
	}
	if p.Encrypted {
		return nil, nil, errEncryptedPayload
	}
	transforms := []string{"zip-member", "im4p-data"}
	if len(p.Data) < 4 {
		return nil, nil, errShortIM4PPayload
	}
	if strings.HasPrefix(string(p.Data[:4]), "bvx") {
		decoded, err := decodeNativeLZFSE(ctx, p.Data, limit)
		if err != nil {
			return nil, nil, err
		}
		return decoded, append(transforms, "lzfse"), nil
	}
	if strings.HasPrefix(string(p.Data), "complzss") {
		return nil, nil, errLZSSAdapterIsNotQualified
	}
	if p.Compression.UncompressedSize > 0 {
		return nil, nil, errUnsupportedIM4PCompressionDeclaration
	}
	if len(p.Data) > limit {
		return nil, nil, errPayloadExceedsOutputBudget
	}
	return p.Data, transforms, nil
}

func readKernel(t model.Target, data, raw []byte, src model.Location) ([]model.Observation, error) {
	f, err := macho.NewFile(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("readKernel: macho.NewFile: %w", err)
	}
	defer f.Close()
	out := []model.Observation{}
	evidence := model.NewBlob(raw)
	src.Selector = "MachO/header"
	src.Length = 32
	if f.Magic == macho.Magic32 {
		src.Length = 28
	}
	o, err := model.NewObservation(
		t,
		"kernel",
		"mach_header",
		f.FileHeader,
		src,
		evidence,
		Extractor,
	)
	if err != nil {
		return out, fmt.Errorf("readKernel: model.NewObservation: %w", err)
	}
	out = append(out, o)
	var section *macho.Section
	for _, s := range f.Sections {
		if s.Seg == "__PRELINK_INFO" && s.Name == "__info" {
			section = s
			break
		}
	}
	if section == nil {
		return out, errPrelinkedMetadataMissing
	}
	if section.Size > prelinkSectionLimit {
		return out, errDriverMetadataExceeds16MiB
	}
	b, err := section.Data()
	if err != nil {
		return out, fmt.Errorf("readKernel: Data: %w", err)
	}
	b = bytes.TrimRight(b, "\x00")
	info, err := parsePrelink(b)
	if err != nil {
		return out, fmt.Errorf("readKernel: parsePrelink: %w", err)
	}
	drivers, ok := info["_PrelinkInfoDictionary"].([]any)
	if !ok {
		return out, fmt.Errorf(
			"readKernel: %w: driver dictionary is missing or invalid",
			errInvalidPrelink,
		)
	}
	src.Offset = uint64(section.Offset)
	src.Length = uint64(len(b))
	src.Selector = "__PRELINK_INFO/__info"
	for i, driver := range drivers {
		d, ok := driver.(map[string]any)
		if !ok {
			return out, fmt.Errorf(
				"readKernel: %w: driver %d is not a dictionary",
				errInvalidPrelink,
				i,
			)
		}
		id, _ := d["CFBundleIdentifier"].(string)
		if id == "" {
			return out, fmt.Errorf("%w: index %d", errDriverHasNoBundleIdentity, i)
		}
		for _, key := range []string{"CFBundleIdentifier", "CFBundleVersion", "OSBundleLibraries", "IOKitPersonalities"} {
			if value, ok := d[key]; ok {
				loc := src
				loc.Selector += fmt.Sprintf("/_PrelinkInfoDictionary/%d/%s", i, key)
				o, err := model.NewObservation(
					t,
					"driver/"+id,
					key,
					exactJSON(value),
					loc,
					evidence,
					Extractor,
				)
				if err != nil {
					return out, fmt.Errorf("readKernel: model.NewObservation: %w", err)
				}
				out = append(out, o)
			}
		}
	}
	return out, nil
}

// JSON consumers must not round 64-bit firmware addresses through floating point.
func exactJSON(value any) any {
	b, err := json.Marshal(value)
	if err != nil {
		return value
	}
	var v any
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	if d.Decode(&v) != nil {
		return value
	}
	var walk func(any) any
	walk = func(x any) any {
		switch n := x.(type) {
		case json.Number:
			return map[string]string{"type": "number", "decimal": string(n)}
		case map[string]any:
			for k, v := range n {
				n[k] = walk(v)
			}
		case []any:
			for i, v := range n {
				n[i] = walk(v)
			}
		}
		return x
	}
	return walk(v)
}

func escape(
	s string,
) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "~", "~0"), "/", "~1")
}
