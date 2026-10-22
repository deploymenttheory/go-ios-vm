package inspection

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"

	"github.com/deploymenttheory/go-apfs-v3/apfs"
	"github.com/deploymenttheory/go-apfs-v3/diskimage"
	"github.com/deploymenttheory/go-ios-vm/internal/model"
)

var (
	errDiskMetadataCaptureExceeds32MiB = errors.New(
		"disk metadata capture exceeds 32 MiB",
	)
	errOSDiskImageNotDeclared = errors.New(
		"OS disk image not declared",
	)
	errOSDiskMemberIsMissing = errors.New(
		"OS disk member is missing",
	)
	errCompressedDiskUnsupported = errors.New(
		"disk image is ZIP-compressed; bounded staging adapter is not implemented",
	)
	errDiskMemberOutsideSource = errors.New(
		"disk member outside source",
	)
)

type readRange struct {
	Offset int64  `json:"offset,string"`
	Data   []byte `json:"data"`
}
type captureSource struct {
	ctx context.Context
	*io.SectionReader
	ranges []readRange
	used   int
	seen   map[int64][]byte
}

func (s *captureSource) ReadAt(p []byte, offset int64) (int, error) {
	if err := s.ctx.Err(); err != nil {
		return 0, fmt.Errorf("disk read cancelled: %w", err)
	}
	if old, ok := s.seen[offset]; ok && len(old) >= len(p) {
		copy(p, old)
		return len(p), nil
	}
	if len(p) > 32<<20-s.used {
		return 0, errDiskMetadataCaptureExceeds32MiB
	}
	n, err := s.SectionReader.ReadAt(p, offset)
	if n > 0 {
		b := append([]byte(nil), p[:n]...)
		s.ranges = append(s.ranges, readRange{offset, b})
		s.seen[offset] = b
		s.used += n
	}
	return n, err //nolint:wrapcheck // io.ReaderAt requires the original io.EOF sentinel.
}

func (a *Archive) inspectDisk(ctx context.Context, t model.Target) ([]model.Observation, error) {
	name, ok := t.Components["OS"]
	if !ok {
		return nil, errOSDiskImageNotDeclared
	}
	index, ok := a.members[name]
	if !ok {
		return nil, errOSDiskMemberIsMissing
	}
	member := a.ZIP.File[index]
	if member.Method != zip.Store {
		return nil, errCompressedDiskUnsupported
	}
	offset, err := member.DataOffset()
	if err != nil {
		return nil, fmt.Errorf("inspectDisk: DataOffset: %w", err)
	}
	if member.UncompressedSize64 > math.MaxInt64 {
		return nil, errDiskMemberOutsideSource
	}
	memberSize := int64(member.UncompressedSize64)
	if offset < 0 || memberSize > a.Artifact.Size || offset > a.Artifact.Size-memberSize {
		return nil, errDiskMemberOutsideSource
	}
	source := &captureSource{
		ctx:           ctx,
		SectionReader: io.NewSectionReader(a.file, offset, memberSize),
		seen:          map[int64][]byte{},
	}
	image, err := diskimage.New(source)
	if err != nil {
		return nil, fmt.Errorf("inspectDisk: diskimage.New: %w", err)
	}
	defer image.Close()
	type item struct {
		subject, property string
		value             any
	}
	items := []item{
		{
			"disk/OS",
			"layout",
			map[string]any{
				"format":        image.Format,
				"partition_map": image.PartitionMap,
				"size":          image.Size(),
				"partitions":    image.Partitions,
			},
		},
	}
	var gap error
	for _, part := range image.Partitions {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("inspectDisk: Err: %w", err)
		}
		view, err := image.Partition(part.Index)
		if err != nil {
			gap = err
			break
		}
		// Probe the APFS magic without guessing a filesystem from the filename.
		header := make([]byte, 36)
		if _, err = view.ReadAt(header, 0); err != nil {
			gap = err
			break
		}
		if string(header[32:36]) != "NXSB" {
			continue
		}
		container, err := apfs.Open(view)
		if err != nil {
			gap = err
			break
		}
		items = append(
			items,
			item{fmt.Sprintf("disk/OS/partition/%d", part.Index), "apfs", container},
		)
	}
	evidence, err := json.Marshal(struct {
		Schema string      `json:"schema"`
		Ranges []readRange `json:"ranges"`
	}{"encoded-disk-read-ranges/v1", source.ranges})
	if err != nil {
		return nil, fmt.Errorf("inspectDisk: json.Marshal: %w", err)
	}
	loc := model.Location{
		ArtifactID: t.ArtifactID,
		Member:     name,
		Entry:      index,
		Coordinate: "zip-member-byte-ranges",
		Transforms: []string{"zip-stored-member", "go-apfs-v3@v0.1.0"},
	}
	out := []model.Observation{}
	blob := model.NewBlob(evidence)
	for _, item := range items {
		loc.Selector = item.subject + "/" + item.property
		o, err := model.NewObservation(
			t,
			item.subject,
			item.property,
			exactJSON(item.value),
			loc,
			blob,
			"go-ios-vm/disk-v1;go-apfs-v3=v0.1.0",
		)
		if err != nil {
			return out, fmt.Errorf("inspectDisk: model.NewObservation: %w", err)
		}
		out = append(out, o)
	}
	return out, gap
}
