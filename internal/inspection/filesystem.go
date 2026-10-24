package inspection

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/deploymenttheory/go-apfs-v3/apfs"
	"github.com/deploymenttheory/go-apfs-v3/filesystem"
	"github.com/deploymenttheory/go-ios-vm/internal/model"
)

// These roots are the initial, explicit firmware/driver search scope. A
// successful scan is never evidence that a file is absent elsewhere on disk.
var firmwareFileRoots = []string{
	"System/Library/Extensions",
	"System/Library/DriverExtensions",
	"Library/Extensions",
	"usr/standalone/firmware",
}

const (
	filesystemEntryLimit = 2048
	filesystemDepthLimit = 8
	filesystemFileLimit  = 64 << 10
	filesystemTotalLimit = 8 << 20
	goAPFSVersion        = "v0.1.1-0.20261024140912-7566f1030c62"
	filesystemExtractor  = "go-ios-vm/filesystem-v1;go-apfs-v3=" + goAPFSVersion
)

type filesystemScan struct {
	ctx          context.Context
	target       model.Target
	base         model.Location
	partition    int
	volume       *apfs.Volume
	containerXID apfs.XID
	budget       *filesystemBudget
	results      []model.Observation
	gaps         []string
	omittedGaps  int
}

type filesystemBudget struct {
	entries int
	bytes   int
}

func (s *filesystemScan) recordGap(reason string) {
	if len(s.gaps) < 8 {
		s.gaps = append(s.gaps, reason)
	} else {
		s.omittedGaps++
	}
}

func (s *filesystemScan) contentStatus(object uint64, name, state, reason string) error {
	status := map[string]any{"path": "/" + name, "state": state, "reason": reason}
	evidence, err := json.Marshal(status)
	if err != nil {
		return fmt.Errorf("filesystem content status evidence: %w", err)
	}
	return s.add(
		s.subject(object), "content", "unexamined", status,
		s.location(object, "data", "apfs-file-object", 0), evidence,
	)
}

func (s *filesystemScan) location(
	object uint64,
	attribute, coordinate string,
	length uint64,
) model.Location {
	loc := s.base
	loc.Coordinate = coordinate
	loc.Selector = fmt.Sprintf(
		"partition/%d/volume/%s/view/%d/object/%d/%s",
		s.partition,
		s.volume.UUID,
		s.volumeView(),
		object,
		attribute,
	)
	loc.Offset = 0
	loc.Length = length
	loc.Transforms = append(
		append([]string(nil), s.base.Transforms...),
		"disk-image-decode",
		"apfs-volume",
	)
	return loc
}

func (s *filesystemScan) volumeView() uint64 { return uint64(s.containerXID) }

func (s *filesystemScan) add(
	subject, property, presence string,
	value any,
	loc model.Location,
	evidence []byte,
) error {
	o, err := model.NewObservationWithPresence(
		s.target,
		subject,
		property,
		presence,
		value,
		loc,
		model.NewBlob(evidence),
		filesystemExtractor,
	)
	if err != nil {
		return fmt.Errorf("filesystem observation: %w", err)
	}
	s.results = append(s.results, o)
	return nil
}

func (s *filesystemScan) status(root, state, reason string, object uint64) error {
	value := map[string]any{"path": "/" + root, "state": state}
	if reason != "" {
		value["reason"] = reason
	}
	evidence, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("filesystem status evidence: %w", err)
	}
	presence := "present"
	if state == "absent" {
		presence = "absent"
	} else if state != "complete" {
		presence = "unexamined"
	}
	return s.add(
		fmt.Sprintf("filesystem/volume/%s/search/%s", s.volume.UUID, root),
		"search",
		presence,
		value,
		s.location(object, "search/"+root, "apfs-file-object", 0),
		evidence,
	)
}

func (s *filesystemScan) subject(object uint64) string {
	return fmt.Sprintf("filesystem/volume/%s/object/%d", s.volume.UUID, object)
}

func (s *filesystemScan) visit(name string, object uint64, depth int) error {
	if err := s.ctx.Err(); err != nil {
		return fmt.Errorf("filesystem walk cancelled: %w", err)
	}
	if s.budget.entries >= filesystemEntryLimit {
		return fmt.Errorf(
			"entry budget %d at /%s: %w",
			filesystemEntryLimit,
			name,
			filesystem.ErrLimit,
		)
	}
	s.budget.entries++
	node, err := s.volume.Stat(s.ctx, object)
	if err != nil {
		return fmt.Errorf("stat /%s: %w", name, err)
	}
	loc := s.location(object, "metadata", "apfs-file-object", 0)
	loc.Selector = fmt.Sprintf(
		"partition/%d/volume/%s/view/%d/object/%d/metadata",
		s.partition,
		node.Identity.Volume,
		node.Identity.View,
		node.Identity.Object,
	)
	metadata := map[string]any{"path": "/" + name, "node": node}
	evidence, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("filesystem metadata evidence: %w", err)
	}
	if err := s.add(s.subject(object), "metadata", "present", metadata, loc, evidence); err != nil {
		return err
	}
	switch node.Metadata.Mode.Value & 0o170000 {
	case 0o040000:
		if depth >= filesystemDepthLimit {
			return fmt.Errorf(
				"directory depth budget %d at /%s: %w",
				filesystemDepthLimit,
				name,
				filesystem.ErrLimit,
			)
		}
		var children []filesystem.DirEntry
		limitReached := false
		readErr := s.volume.ReadDir(s.ctx, object, func(entry filesystem.DirEntry) error {
			if len(children) >= filesystemEntryLimit-s.budget.entries {
				limitReached = true
				return filesystem.ErrLimit
			}
			children = append(children, entry)
			return nil
		})
		if readErr != nil && !limitReached {
			return fmt.Errorf("list /%s: %w", name, readErr)
		}
		sort.Slice(children, func(i, j int) bool { return children[i].Name < children[j].Name })
		for _, child := range children {
			if err := s.visit(path.Join(name, child.Name), child.Object, depth+1); err != nil {
				return err
			}
		}
		if limitReached {
			return fmt.Errorf(
				"entry budget %d at /%s: %w",
				filesystemEntryLimit,
				name,
				filesystem.ErrLimit,
			)
		}
	case 0o100000:
		if relevantFile(name) {
			if err := s.readFile(name, node); err != nil {
				state := classifyFilesystemError(err)
				if addErr := s.contentStatus(object, name, state, err.Error()); addErr != nil {
					return addErr
				}
				s.recordGap(fmt.Sprintf("/%s: %s", name, err))
				if state == "limit" {
					return err
				}
			}
		} else if err := s.contentStatus(
			object,
			name,
			"unexamined",
			"file extension outside selected firmware/driver content formats",
		); err != nil {
			return err
		}
	}
	return nil
}

func relevantFile(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".plist") || strings.HasSuffix(lower, ".json") ||
		strings.HasSuffix(lower, ".entitlements") ||
		strings.HasSuffix(lower, ".txt")
}

func (s *filesystemScan) readFile(name string, node filesystem.Node) error {
	available := filesystemTotalLimit - s.budget.bytes
	if available == 0 {
		return fmt.Errorf(
			"content budget %d at /%s: %w",
			filesystemTotalLimit,
			name,
			filesystem.ErrLimit,
		)
	}
	value, err := s.volume.OpenData(s.ctx, node.Identity.Object)
	if err != nil {
		return fmt.Errorf("open /%s: %w", name, err)
	}
	closed := false
	defer func() {
		if !closed {
			_ = value.Close()
		}
	}()
	size := value.Size()
	if size < 0 {
		return fmt.Errorf("negative logical size at /%s: %w", name, filesystem.ErrCorrupt)
	}
	if uint64(size) != node.Size {
		return fmt.Errorf("inconsistent logical size at /%s: %w", name, filesystem.ErrCorrupt)
	}
	length := size
	if length > filesystemFileLimit {
		length = filesystemFileLimit
	}
	if length > int64(available) {
		length = int64(available)
	}
	data := make([]byte, length)
	if len(data) > 0 {
		if _, err := io.ReadFull(io.NewSectionReader(value, 0, length), data); err != nil {
			return fmt.Errorf("read /%s: %w", name, err)
		}
	}
	s.budget.bytes += len(data)
	if err := value.Close(); err != nil {
		return fmt.Errorf("close /%s: %w", name, err)
	}
	closed = true
	complete := uint64(len(data)) == node.Size
	loc := s.location(node.Identity.Object, "data", "apfs-logical-file-byte", uint64(len(data)))
	loc.Selector = fmt.Sprintf(
		"partition/%d/volume/%s/view/%d/object/%d/data",
		s.partition,
		node.Identity.Volume,
		node.Identity.View,
		node.Identity.Object,
	)
	meta := map[string]any{
		"path":           "/" + name,
		"size":           node.Size,
		"captured_bytes": len(data),
		"complete":       complete,
		"sha256":         model.Digest(data),
	}
	if !complete {
		meta["reason"] = fmt.Sprintf(
			"per-file content limit %d bytes or total content limit %d bytes",
			filesystemFileLimit,
			filesystemTotalLimit,
		)
		s.recordGap(fmt.Sprintf(
			"/%s: content truncated at %d of %d bytes",
			name,
			len(data),
			node.Size,
		))
	}
	return s.add(s.subject(node.Identity.Object), "content", "present", meta, loc, data)
}

func classifyFilesystemError(err error) string {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return "absent"
	case errors.Is(err, filesystem.ErrAuthentication):
		return "encrypted"
	case errors.Is(err, filesystem.ErrUnsupported):
		return "unsupported"
	case errors.Is(err, filesystem.ErrCorrupt):
		return "corrupt"
	case errors.Is(err, filesystem.ErrLimit), errors.Is(err, errDiskMetadataCaptureExceeds32MiB):
		return "limit"
	default:
		return "failed"
	}
}

func inspectAPFSContent(
	ctx context.Context,
	target model.Target,
	base model.Location,
	partition int,
	container *apfs.Container,
	budget *filesystemBudget,
) ([]model.Observation, []string, error) {
	var observations []model.Observation
	var gaps []string
	omittedGaps := 0
	recordGap := func(reason string) {
		if len(gaps) < 16 {
			gaps = append(gaps, reason)
		} else {
			omittedGaps++
		}
	}
	for i := range container.Volumes {
		volume := &container.Volumes[i]
		s := filesystemScan{
			ctx:          ctx,
			target:       target,
			base:         base,
			partition:    partition,
			volume:       volume,
			containerXID: container.XID,
			budget:       budget,
		}
		if volume.Encrypted {
			if err := s.status(
				"",
				"encrypted",
				"volume is encrypted and no unlock key was supplied",
				volume.Root(),
			); err != nil {
				return observations, gaps, err
			}
			recordGap(fmt.Sprintf("volume %s: encrypted", volume.UUID))
			observations = append(observations, s.results...)
			continue
		}
		for _, root := range firmwareFileRoots {
			object, err := filesystem.LookupExact(ctx, volume, root)
			if errors.Is(err, fs.ErrNotExist) {
				if err := s.status(
					root,
					"absent",
					"complete exact-path lookup in selected volume",
					volume.Root(),
				); err != nil {
					return observations, gaps, err
				}
				continue
			}
			if err == nil {
				err = s.visit(root, object, 0)
			}
			if err != nil {
				if ctx.Err() != nil {
					return observations, gaps, fmt.Errorf(
						"filesystem scan cancelled: %w",
						ctx.Err(),
					)
				}
				if object == 0 {
					object = volume.Root()
				}
				state := classifyFilesystemError(err)
				if statusErr := s.status(root, state, err.Error(), object); statusErr != nil {
					return observations, gaps, statusErr
				}
				recordGap(fmt.Sprintf("volume %s /%s: %s", volume.UUID, root, err))
				continue
			}
			if err := s.status(
				root,
				"complete",
				"selected path searched within bounded firmware/driver scope",
				object,
			); err != nil {
				return observations, gaps, err
			}
		}
		observations = append(observations, s.results...)
		for _, reason := range s.gaps {
			recordGap(reason)
		}
		if s.omittedGaps > 0 {
			recordGap(fmt.Sprintf(
				"volume %s: %d further file gaps",
				volume.UUID,
				s.omittedGaps,
			))
		}
	}
	if omittedGaps > 0 {
		gaps = append(gaps, fmt.Sprintf("%d further filesystem gaps", omittedGaps))
	}
	return observations, gaps, nil
}
