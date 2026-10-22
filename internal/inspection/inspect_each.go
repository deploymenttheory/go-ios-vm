package inspection

import (
	"context"
	"fmt"
	"sort"

	"github.com/deploymenttheory/go-ios-vm/internal/model"
)

// Components is the stable order and identity of inspection checkpoints.
var Components = []string{
	"manifest",
	"DeviceTree",
	"KernelCache",
	"disk-metadata",
	"filesystem-content",
}

// RunPlanVersion covers component semantics that do not change the stable
// observation extractor identity. A resume must use the same plan version.
const RunPlanVersion = Extractor + ";filesystem=unexamined-v1"

// InspectEach emits one completed component at a time. The caller can commit
// each component before parsing the next. A component marked done is never
// decoded again on an explicit resume.
func (a *Archive) InspectEach(
	ctx context.Context,
	identity int,
	done map[string]bool,
	emit func(string, []model.Observation, model.Coverage) error,
) error {
	if identity < 0 || identity >= len(a.Targets) {
		return errIdentityIndexOutsideBuildManifest
	}
	t := a.Targets[identity]
	add := func(out *[]model.Observation, subject, property string, value any, src model.Location, evidence model.Blob) error {
		o, err := model.NewObservation(t, subject, property, value, src, evidence, Extractor)
		if err != nil {
			return fmt.Errorf("InspectEach: NewObservation: %w", err)
		}
		*out = append(*out, o)
		return nil
	}
	if !done["manifest"] {
		source := model.Location{
			ArtifactID: t.ArtifactID,
			Member:     "BuildManifest.plist",
			Entry:      a.members["BuildManifest.plist"],
			Coordinate: "plist-selector",
			Selector:   fmt.Sprintf("/BuildIdentities/%d", identity),
			Length:     uint64(len(a.manifest)),
		}
		var observations []model.Observation
		evidence := model.NewBlob(a.manifest)
		if err := add(&observations, "target", "identity", map[string]any{
			"products": t.Products, "board": t.Board, "version": t.Version,
			"build": t.Build, "variant": t.Variant,
		}, source, evidence); err != nil {
			return err
		}
		names := make([]string, 0, len(t.Components))
		for name := range t.Components {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			if err := ctx.Err(); err != nil {
				return fmt.Errorf("InspectEach: %w", err)
			}
			path := t.Components[name]
			src := source
			src.Selector += "/Manifest/" + escape(name) + "/Info/Path"
			_, exists := a.members[path]
			if err := add(&observations, "component/"+name, "archive_member",
				map[string]any{"path": path, "present": exists}, src, evidence); err != nil {
				return err
			}
		}
		if err := a.Unchanged(); err != nil {
			return err
		}
		if err := emit(
			"manifest",
			observations,
			model.Coverage{Component: "manifest", State: "complete"},
		); err != nil {
			return err
		}
	}
	for _, component := range []string{"DeviceTree", "KernelCache"} {
		if done[component] {
			continue
		}
		if err := a.inspectPayloadComponent(ctx, t, component, emit); err != nil {
			return err
		}
	}
	if !done["disk-metadata"] {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("InspectEach: %w", err)
		}
		observations, diskErr := a.inspectDisk(ctx, t)
		c := model.Coverage{Component: "disk-metadata", State: "complete"}
		if diskErr != nil {
			c.State = "partial"
			c.Reason = diskErr.Error()
		}
		if err := a.Unchanged(); err != nil {
			return err
		}
		if err := emit("disk-metadata", observations, c); err != nil {
			return err
		}
	}
	if !done["filesystem-content"] {
		c := model.Coverage{
			Component: "filesystem-content",
			State:     "unexamined",
			Reason:    "filesystem content inspection is not implemented",
		}
		if err := emit(c.Component, nil, c); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("InspectEach: %w", err)
	}
	return a.Unchanged()
}

func (a *Archive) inspectPayloadComponent(
	ctx context.Context,
	t model.Target,
	component string,
	emit func(string, []model.Observation, model.Coverage) error,
) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("inspect %s: %w", component, err)
	}
	name, ok := t.Components[component]
	if !ok {
		return emit(component, nil, model.Coverage{
			Component: component, State: "unexamined",
			Reason: "component not declared by selected identity",
		})
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
			src := model.Location{
				ArtifactID: t.ArtifactID, Member: name, Entry: index,
				Coordinate: "decoded-payload-byte", Transforms: transforms,
			}
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
	if err := a.Unchanged(); err != nil {
		return err
	}
	return emit(component, observations, c)
}
