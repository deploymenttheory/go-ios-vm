package inspection

import (
	"bytes"
	"context"
	"debug/macho"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/blacktop/ipsw/pkg/img4"
	lzfse "github.com/blacktop/lzfse-cgo"
	"github.com/deploymenttheory/go-ios-vm/internal/model"
	"howett.net/plist"
)

const Extractor = "go-ios-vm/static-v1;ipsw=12847a212ba1;lzfse-cgo=v1.2.0"
const TreeLimit = 16 << 20
const KernelLimit = 128 << 20

type Result struct {
	Observations []model.Observation
	Coverage     []model.Coverage
}

func (a *Archive) Inspect(ctx context.Context, identity int) (Result, error) {
	result := Result{Observations: []model.Observation{}, Coverage: []model.Coverage{}}
	if identity < 0 || identity >= len(a.Targets) {
		return result, fmt.Errorf("identity index outside BuildManifest")
	}
	t := a.Targets[identity]
	source := model.Location{ArtifactID: t.ArtifactID, Member: "BuildManifest.plist", Entry: a.members["BuildManifest.plist"], Coordinate: "plist-selector", Selector: fmt.Sprintf("/BuildIdentities/%d", identity), Length: uint64(len(a.manifest))}
	add := func(subject, property string, value any, src model.Location, evidence model.Blob) error {
		o, e := model.NewObservation(t, subject, property, value, src, evidence, Extractor)
		if e == nil {
			result.Observations = append(result.Observations, o)
		}
		return e
	}
	manifestEvidence := model.NewBlob(a.manifest)
	if err := add("target", "identity", map[string]any{"products": t.Products, "board": t.Board, "version": t.Version, "build": t.Build, "variant": t.Variant}, source, manifestEvidence); err != nil {
		return result, err
	}
	names := make([]string, 0, len(t.Components))
	for name := range t.Components {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		path := t.Components[name]
		src := source
		src.Selector += "/Manifest/" + escape(name) + "/Info/Path"
		_, exists := a.members[path]
		if err := add("component/"+name, "archive_member", map[string]any{"path": path, "present": exists}, src, manifestEvidence); err != nil {
			return result, err
		}
	}
	result.Coverage = append(result.Coverage, model.Coverage{Component: "manifest", State: "complete"})
	for _, component := range []string{"DeviceTree", "KernelCache"} {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		name, ok := t.Components[component]
		if !ok {
			result.Coverage = append(result.Coverage, model.Coverage{Component: component, State: "unexamined", Reason: "component not declared by selected identity"})
			continue
		}
		limit := uint64(TreeLimit)
		if component == "KernelCache" {
			limit = KernelLimit
		}
		raw, index, err := a.Read(ctx, name, limit)
		var observations []model.Observation
		if err == nil {
			var payload []byte
			var transforms []string
			payload, transforms, err = decodePayload(raw, int(limit))
			if err == nil {
				src := model.Location{ArtifactID: t.ArtifactID, Member: name, Entry: index, Coordinate: "decoded-payload-byte", Transforms: transforms}
				if component == "DeviceTree" {
					observations, err = readTree(ctx, t, payload, raw, src)
				} else {
					observations, err = readKernel(t, payload, raw, src)
				}
			}
		}
		c := model.Coverage{Component: component, State: "complete"}
		if err != nil {
			c.State = "partial"
			c.Reason = err.Error()
		}
		result.Observations = append(result.Observations, observations...)
		result.Coverage = append(result.Coverage, c)
	}
	diskObservations, diskErr := a.inspectDisk(ctx, t)
	result.Observations = append(result.Observations, diskObservations...)
	diskCoverage := model.Coverage{Component: "disk-metadata", State: "complete"}
	if diskErr != nil {
		diskCoverage.State = "partial"
		diskCoverage.Reason = diskErr.Error()
	}
	result.Coverage = append(result.Coverage, diskCoverage)
	result.Coverage = append(result.Coverage, model.Coverage{Component: "filesystem-content", State: "unexamined", Reason: "filesystem content inspection is not implemented"})
	if err := ctx.Err(); err != nil {
		return result, err
	}
	return result, a.Unchanged()
}

func decodePayload(raw []byte, limit int) ([]byte, []string, error) {
	p, err := img4.ParsePayload(raw)
	if err != nil {
		return nil, nil, err
	}
	if p.Encrypted {
		return nil, nil, fmt.Errorf("encrypted IM4P payload; no plaintext inspection")
	}
	transforms := []string{"zip-member", "im4p-data"}
	if len(p.Data) < 4 {
		return nil, nil, fmt.Errorf("short IM4P payload")
	}
	if strings.HasPrefix(string(p.Data[:4]), "bvx") {
		dst := make([]byte, limit+1)
		n := lzfse.DecodeBufferInto(p.Data, dst)
		if n == 0 {
			return nil, nil, fmt.Errorf("LZFSE decode failed")
		}
		if n > limit {
			return nil, nil, fmt.Errorf("LZFSE exceeds %d-byte output budget", limit)
		}
		return dst[:n], append(transforms, "lzfse"), nil
	}
	if strings.HasPrefix(string(p.Data), "complzss") {
		return nil, nil, fmt.Errorf("LZSS adapter is not qualified")
	}
	if p.Compression.UncompressedSize > 0 {
		return nil, nil, fmt.Errorf("unsupported IM4P compression declaration")
	}
	if len(p.Data) > limit {
		return nil, nil, fmt.Errorf("payload exceeds output budget")
	}
	return p.Data, transforms, nil
}

func readKernel(t model.Target, data, raw []byte, src model.Location) ([]model.Observation, error) {
	f, err := macho.NewFile(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := []model.Observation{}
	evidence := model.NewBlob(raw)
	src.Selector = "MachO/header"
	src.Length = 32
	if f.Magic == macho.Magic32 {
		src.Length = 28
	}
	o, err := model.NewObservation(t, "kernel", "mach_header", f.FileHeader, src, evidence, Extractor)
	if err != nil {
		return out, err
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
		return out, fmt.Errorf("prelinked driver metadata not found; fileset inspection is not implemented")
	}
	if section.Size > 16<<20 {
		return out, fmt.Errorf("driver metadata exceeds 16 MiB")
	}
	b, err := section.Data()
	if err != nil {
		return out, err
	}
	b = bytes.TrimRight(b, "\x00")
	var info struct {
		Drivers []map[string]any `plist:"_PrelinkInfoDictionary"`
	}
	if _, err = plist.Unmarshal(b, &info); err != nil {
		return out, err
	}
	src.Offset = uint64(section.Offset)
	src.Length = uint64(len(b))
	src.Selector = "__PRELINK_INFO/__info"
	for i, d := range info.Drivers {
		id, _ := d["CFBundleIdentifier"].(string)
		if id == "" {
			return out, fmt.Errorf("driver %d has no bundle identity", i)
		}
		for _, key := range []string{"CFBundleIdentifier", "CFBundleVersion", "OSBundleLibraries", "IOKitPersonalities"} {
			if value, ok := d[key]; ok {
				loc := src
				loc.Selector += fmt.Sprintf("/_PrelinkInfoDictionary/%d/%s", i, key)
				o, err := model.NewObservation(t, "driver/"+id, key, exactJSON(value), loc, evidence, Extractor)
				if err != nil {
					return out, err
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
func escape(s string) string { return strings.ReplaceAll(strings.ReplaceAll(s, "~", "~0"), "/", "~1") }
