package inspection

import (
	"archive/zip"
	"bytes"
	"compress/flate"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"howett.net/plist"

	"github.com/deploymenttheory/go-ios-vm/internal/model"
)

func diskIPSW(t *testing.T, method uint16, disk []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "disk.ipsw")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	z := zip.NewWriter(f)
	manifest, err := plist.Marshal(map[string]any{
		"ProductVersion": "14.0", "ProductBuildVersion": "synthetic",
		"SupportedProductTypes": []string{"iPhone12,1"},
		"BuildIdentities": []any{
			map[string]any{
				"Info": map[string]any{"DeviceClass": "n104ap", "Variant": "Erase"},
				"Manifest": map[string]any{
					"OS": map[string]any{"Info": map[string]any{"Path": "OS.dmg"}},
				},
			},
		},
	}, plist.XMLFormat)
	if err != nil {
		t.Fatal(err)
	}
	for _, member := range []struct {
		name   string
		method uint16
		data   []byte
	}{{"BuildManifest.plist", zip.Store, manifest}, {"OS.dmg", method, disk}} {
		w, err := z.CreateHeader(&zip.FileHeader{Name: member.name, Method: member.method})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(member.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

func diskLayout(t *testing.T, path string) (model.Observation, model.Coverage) {
	t.Helper()
	a, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	result, err := a.Inspect(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	var layout model.Observation
	var coverage model.Coverage
	for _, o := range result.Observations {
		if o.Subject == "disk/OS" && o.Property == "layout" {
			layout = o
		}
	}
	for _, c := range result.Coverage {
		if c.Component == "disk-metadata" {
			coverage = c
		}
	}
	return layout, coverage
}

func TestStoredAndCompressedDiskLayoutAndEvidence(t *testing.T) {
	disk := make([]byte, 4096)
	stored, storedCoverage := diskLayout(t, diskIPSW(t, zip.Store, disk))
	compressed, compressedCoverage := diskLayout(t, diskIPSW(t, zip.Deflate, disk))
	if storedCoverage.State != "complete" || compressedCoverage.State != "complete" {
		t.Fatalf("coverage: stored=%+v compressed=%+v", storedCoverage, compressedCoverage)
	}
	if !bytes.Equal(stored.Value, compressed.Value) {
		t.Fatalf("layout differs: stored=%s compressed=%s", stored.Value, compressed.Value)
	}
	if err := compressed.Validate(); err != nil {
		t.Fatal(err)
	}
	if compressed.Source.Coordinate != "zip-encoded-member" ||
		compressed.Source.Length != uint64(len(compressed.Evidence)) ||
		len(compressed.Source.Transforms) != 2 ||
		compressed.Source.Transforms[0] != "zip-deflate-member" {
		t.Fatalf("encoded source missing: %+v", compressed.Source)
	}
	r := flate.NewReader(bytes.NewReader(compressed.Evidence))
	decoded, err := io.ReadAll(r)
	r.Close()
	if err != nil || !bytes.Equal(decoded, disk) {
		t.Fatalf("retained ZIP bytes do not recover source disk: %v", err)
	}
	var captured struct {
		Schema string `json:"schema"`
	}
	if err := json.Unmarshal(
		stored.Evidence,
		&captured,
	); err != nil ||
		captured.Schema != "encoded-disk-read-ranges/v1" {
		t.Fatalf("stored evidence changed: %v, %+v", err, captured)
	}
}

func TestDiskStagingPlanMatchesSelectedMemberAndLimit(t *testing.T) {
	for _, method := range []uint16{zip.Store, zip.Deflate} {
		path := diskIPSW(t, method, make([]byte, 4096))
		a, err := Open(context.Background(), path)
		if err != nil {
			t.Fatal(err)
		}
		plan, err := a.DiskStagingPlan(0)
		if err != nil {
			t.Fatal(err)
		}
		if plan.Member != "OS.dmg" || plan.RequiresStaging != (method == zip.Deflate) || !plan.Fits {
			t.Fatalf("method %d plan: %+v", method, plan)
		}
		if method == zip.Deflate {
			if plan.DeclaredBytes != 4096 || plan.LimitBytes != diskStageLimit ||
				plan.ReserveBytes != diskStageReserve {
				t.Fatalf("unexpected staging estimate: %+v", plan)
			}
			a.ZIP.File[a.members["OS.dmg"]].UncompressedSize64 = diskStageLimit + 1
			plan, err = a.DiskStagingPlan(0)
			if err != nil || plan.Fits || plan.Reason != "declared OS image exceeds staging limit" {
				t.Fatalf("oversized stage plan: %+v, %v", plan, err)
			}
		}
		if err := a.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func stageOptions(t *testing.T, limit uint64) diskStageOptions {
	t.Helper()
	return diskStageOptions{
		root: t.TempDir(), limit: limit, reserve: 0,
		free: func(string) (uint64, error) { return limit + 1024, nil },
	}
}

func assertEmptyStageRoot(t *testing.T, root string) {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 0 {
		t.Fatalf("incomplete staging remains: %v, %v", entries, err)
	}
}

func TestDiskStagingLimitsAndCleanup(t *testing.T) {
	path := diskIPSW(t, zip.Deflate, bytes.Repeat([]byte("abcd"), 1<<18))
	a, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	member := a.ZIP.File[a.members["OS.dmg"]]
	tooSmall := stageOptions(t, 1024)
	if _, err := stageDiskMember(
		context.Background(),
		member,
		tooSmall,
	); !errors.Is(
		err,
		errDiskStageLimit,
	) {
		t.Fatalf("declared limit: %v", err)
	}
	assertEmptyStageRoot(t, tooSmall.root)
	space := stageOptions(t, member.UncompressedSize64)
	space.reserve = 2048
	space.free = func(string) (uint64, error) { return member.UncompressedSize64 + 1024, nil }
	if _, err := stageDiskMember(
		context.Background(),
		member,
		space,
	); !errors.Is(
		err,
		errDiskStageSpace,
	) {
		t.Fatalf("free space reserve: %v", err)
	}
	assertEmptyStageRoot(t, space.root)
	spaceDuringWrite := stageOptions(t, member.UncompressedSize64)
	spaceChecks := 0
	spaceDuringWrite.free = func(string) (uint64, error) {
		spaceChecks++
		if spaceChecks > 1 {
			return 0, nil
		}
		return member.UncompressedSize64 + 1024, nil
	}
	if _, err := stageDiskMember(
		context.Background(), member, spaceDuringWrite,
	); !errors.Is(err, errDiskStageSpace) {
		t.Fatalf("space exhausted during staging: %v", err)
	}
	assertEmptyStageRoot(t, spaceDuringWrite.root)
	good := stageOptions(t, member.UncompressedSize64)
	staged, err := stageDiskMember(context.Background(), member, good)
	if err != nil {
		t.Fatal(err)
	}
	if staged.size != member.UncompressedSize64 {
		t.Fatal("staging size mismatch")
	}
	if err := staged.Close(); err != nil {
		t.Fatal(err)
	}
	assertEmptyStageRoot(t, good.root)

	ctx, cancel := context.WithCancel(context.Background())
	cancelled := stageOptions(t, member.UncompressedSize64)
	calls := 0
	cancelled.free = func(string) (uint64, error) {
		calls++
		if calls == 2 {
			cancel()
		}
		return member.UncompressedSize64 + 1024, nil
	}
	if _, err := stageDiskMember(ctx, member, cancelled); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation: %v", err)
	}
	assertEmptyStageRoot(t, cancelled.root)
}

func TestOversizedEncodedEvidenceAndCancelledStageHavePartialCoverage(t *testing.T) {
	path := diskIPSW(t, zip.Deflate, bytes.Repeat([]byte("data"), 1<<18))
	a, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	member := a.ZIP.File[a.members["OS.dmg"]]
	originalSize := member.CompressedSize64
	member.CompressedSize64 = diskEvidenceLimit + 1
	result, err := a.Inspect(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	assertDiskGap(t, result, "32 MiB retained source-byte limit")
	member.CompressedSize64 = originalSize

	ctx, cancel := context.WithCancel(context.Background())
	stage := stageOptions(t, member.UncompressedSize64)
	calls := 0
	stage.free = func(string) (uint64, error) {
		calls++
		if calls == 2 {
			cancel()
		}
		return member.UncompressedSize64 + 1024, nil
	}
	a.stageOptions = stage
	result, err = a.Inspect(ctx, 0)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation not propagated: %v", err)
	}
	assertDiskGap(t, result, "canceled")
	assertEmptyStageRoot(t, stage.root)
}

func assertDiskGap(t *testing.T, result Result, reason string) {
	t.Helper()
	for _, c := range result.Coverage {
		if c.Component == "disk-metadata" {
			if c.State != "partial" || !bytes.Contains([]byte(c.Reason), []byte(reason)) {
				t.Fatalf("disk gap missing: %+v", c)
			}
			for _, o := range result.Observations {
				if o.Subject == "disk/OS" {
					t.Fatal("unsupported disk emitted a finding")
				}
			}
			return
		}
	}
	t.Fatal("disk coverage missing")
}

func TestMeasuredExpansionAndFreeSpaceDuringWrite(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "staging")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	w := &diskStageWriter{
		ctx: context.Background(), file: file, limit: 4,
		free: func(string) (uint64, error) { return 4, nil },
	}
	if _, err := w.Write([]byte("abcd")); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("e")); !errors.Is(err, errDiskStageLimit) {
		t.Fatalf("measured limit: %v", err)
	}
	if info, err := file.Stat(); err != nil || info.Size() != 4 {
		t.Fatalf("write past limit: %v, %v", info, err)
	}
	w.limit = 5
	w.free = func(string) (uint64, error) { return 0, nil }
	if _, err := w.Write([]byte("e")); !errors.Is(err, errDiskStageSpace) {
		t.Fatalf("mid-stage free space: %v", err)
	}
}

func TestCorruptAndTruncatedCompressedDiskArePartial(t *testing.T) {
	for _, tc := range []struct {
		name           string
		compressedSize uint32
		flip           bool
	}{
		{"truncated", 1, false},
		{"corrupt", 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := diskIPSW(t, zip.Deflate, bytes.Repeat([]byte("disk-data"), 1024))
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			central := bytes.LastIndex(raw, []byte{'P', 'K', 1, 2})
			if central < 0 {
				t.Fatal("missing central directory")
			}
			// The final central-directory entry belongs to OS.dmg.
			if tc.flip {
				reader, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
				if err != nil {
					t.Fatal(err)
				}
				off, err := reader.File[1].DataOffset()
				if err != nil {
					t.Fatal(err)
				}
				raw[off+int64(reader.File[1].CompressedSize64/2)] ^= 0xff
			} else {
				binary.LittleEndian.PutUint32(raw[central+20:central+24], tc.compressedSize)
			}
			if err := os.WriteFile(path, raw, 0o600); err != nil {
				t.Fatal(err)
			}
			a, err := Open(context.Background(), path)
			if err != nil {
				t.Fatal(err)
			}
			defer a.Close()
			stage := stageOptions(t, a.ZIP.File[a.members["OS.dmg"]].UncompressedSize64)
			a.stageOptions = stage
			result, err := a.Inspect(context.Background(), 0)
			if err != nil {
				t.Fatal(err)
			}
			assertDiskGap(t, result, "copy compressed disk member")
			assertEmptyStageRoot(t, stage.root)
		})
	}
}
