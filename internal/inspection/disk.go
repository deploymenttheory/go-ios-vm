package inspection

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"

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
	errCompressedDiskEvidenceExceeds32MiB = errors.New(
		"compressed disk member exceeds 32 MiB retained source-byte limit",
	)
	errDiskMemberOutsideSource = errors.New(
		"disk member outside source",
	)
	errUnsupportedDiskZIPMethod = errors.New("unsupported disk ZIP method")
)

// Stored images already cap captured source reads at this size. A Deflate
// member needs its complete encoded stream to replay arbitrary decoded reads;
// a prefix cannot support a disk observation after the original is removed.
const diskEvidenceLimit = 32 << 20

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

type diskMemberRead struct {
	source          *captureSource
	encodedEvidence []byte
	encodedSize     uint64
	staged          *stagedDisk
}

func (s *captureSource) ReadAt(p []byte, offset int64) (int, error) {
	if err := s.ctx.Err(); err != nil {
		return 0, fmt.Errorf("disk read cancelled: %w", err)
	}
	if old, ok := s.seen[offset]; ok && len(old) >= len(p) {
		copy(p, old)
		return len(p), nil
	}
	if len(p) > diskEvidenceLimit-s.used {
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

func (a *Archive) readDiskMember(ctx context.Context, member *zip.File) (diskMemberRead, error) {
	offset, err := member.DataOffset()
	if err != nil {
		return diskMemberRead{}, fmt.Errorf("readDiskMember: DataOffset: %w", err)
	}
	if member.UncompressedSize64 > math.MaxInt64 || member.CompressedSize64 > math.MaxInt64 {
		return diskMemberRead{}, errDiskMemberOutsideSource
	}
	memberSize := int64(member.UncompressedSize64)
	encodedSize := int64(member.CompressedSize64)
	if member.Method == zip.Deflate && encodedSize > diskEvidenceLimit {
		return diskMemberRead{}, errCompressedDiskEvidenceExceeds32MiB
	}
	if offset < 0 || encodedSize > a.Artifact.Size || offset > a.Artifact.Size-encodedSize {
		return diskMemberRead{}, errDiskMemberOutsideSource
	}
	if member.Method == zip.Store &&
		(memberSize > a.Artifact.Size || offset > a.Artifact.Size-memberSize) {
		return diskMemberRead{}, errDiskMemberOutsideSource
	}
	var diskFile *os.File
	var encodedEvidence []byte
	var staged *stagedDisk
	if member.Method != zip.Store {
		if member.Method != zip.Deflate {
			return diskMemberRead{}, fmt.Errorf(
				"%w: %d",
				errUnsupportedDiskZIPMethod,
				member.Method,
			)
		}
		encodedEvidence, err = io.ReadAll(
			contextReader{ctx, io.NewSectionReader(a.file, offset, encodedSize)},
		)
		if err != nil {
			return diskMemberRead{}, fmt.Errorf("readDiskMember: encoded member read: %w", err)
		}
		if int64(len(encodedEvidence)) != encodedSize {
			return diskMemberRead{}, errDiskMemberOutsideSource
		}
		options := a.stageOptions
		if options.free == nil {
			options = defaultDiskStageOptions()
		}
		staged, err = stageDiskMember(ctx, member, options)
		if err != nil {
			return diskMemberRead{}, fmt.Errorf("readDiskMember: stage ZIP member: %w", err)
		}
		diskFile = staged.file
		offset = 0
	} else {
		diskFile = a.file
	}
	return diskMemberRead{
		source: &captureSource{
			ctx:           ctx,
			SectionReader: io.NewSectionReader(diskFile, offset, memberSize),
			seen:          map[int64][]byte{},
		},
		encodedEvidence: encodedEvidence,
		encodedSize:     member.CompressedSize64,
		staged:          staged,
	}, nil
}

func (a *Archive) inspectDisk(
	ctx context.Context,
	t model.Target,
) (_ []model.Observation, resultErr error) {
	name, ok := t.Components["OS"]
	if !ok {
		return nil, errOSDiskImageNotDeclared
	}
	index, ok := a.members[name]
	if !ok {
		return nil, errOSDiskMemberIsMissing
	}
	read, err := a.readDiskMember(ctx, a.ZIP.File[index])
	if err != nil {
		return nil, err
	}
	if read.staged != nil {
		defer func() {
			if err := read.staged.Close(); err != nil {
				resultErr = errors.Join(
					resultErr,
					fmt.Errorf("inspectDisk: cleanup staging: %w", err),
				)
			}
		}()
	}
	source := read.source
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
	loc := model.Location{
		ArtifactID: t.ArtifactID,
		Member:     name,
		Entry:      index,
		Coordinate: "zip-member-byte-ranges",
		Transforms: []string{"zip-stored-member", "go-apfs-v3@v0.1.0"},
	}
	var evidence []byte
	if read.staged != nil {
		evidence = read.encodedEvidence
		loc.Coordinate = "zip-encoded-member"
		loc.Length = read.encodedSize
		loc.Transforms = []string{"zip-deflate-member", "go-apfs-v3@v0.1.0"}
	} else {
		evidence, err = json.Marshal(struct {
			Schema string      `json:"schema"`
			Ranges []readRange `json:"ranges"`
		}{"encoded-disk-read-ranges/v1", source.ranges})
		if err != nil {
			return nil, fmt.Errorf("inspectDisk: json.Marshal: %w", err)
		}
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
