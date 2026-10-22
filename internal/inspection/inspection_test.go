package inspection

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"testing"

	lzfse "github.com/blacktop/lzfse-cgo"

	"github.com/deploymenttheory/go-ios-vm/internal/model"
	"github.com/deploymenttheory/go-ios-vm/internal/testfixture"
)

func TestInspectRetainsReplayableSourceAndExactIdentities(t *testing.T) {
	path := filepath.Join(t.TempDir(), "synthetic.ipsw")
	if err := testfixture.IPSW(path, "14.0", 0xffffffffffffffff); err != nil {
		t.Fatal(err)
	}
	a, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if len(a.Targets) != 2 || a.Targets[0].ID == a.Targets[1].ID {
		t.Fatal("build identities collapsed")
	}
	if err = a.Verify("iPhone12,1", "15.0", ""); err == nil {
		t.Fatal("wrong build accepted")
	}
	result, err := a.Inspect(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	foundTree, foundDriver := false, false
	for _, o := range result.Observations {
		if err = o.Validate(); err != nil {
			t.Fatal(err)
		}
		if o.Property == "test-value" {
			foundTree = true
			data, _, err := decodePayload(context.Background(), o.Evidence, TreeLimit)
			if err != nil {
				t.Fatal(err)
			}
			if binary.LittleEndian.Uint64(
				data[o.Source.Offset:o.Source.Offset+o.Source.Length],
			) != ^uint64(
				0,
			) {
				t.Fatal("source offset does not recover exact unsigned value")
			}
		}
		if o.Subject == "driver/test.synthetic.driver" && o.Property == "OSBundleLibraries" {
			foundDriver = true
		}
	}
	if !foundTree || !foundDriver {
		t.Fatalf(
			"missing extraction: tree=%v driver=%v coverage=%+v",
			foundTree,
			foundDriver,
			result.Coverage,
		)
	}
	normalized, _ := json.Marshal(exactJSON(uint64(0xffffffffffffffff)))
	if !bytes.Contains(normalized, []byte(`"18446744073709551615"`)) {
		t.Fatal(string(normalized))
	}
}

func TestMalformedTreeAndBoundedLZFSE(t *testing.T) {
	raw := make([]byte, 8)
	binary.LittleEndian.PutUint32(raw, ^uint32(0))
	if _, err := treeSpans(raw); err == nil {
		t.Fatal("unbounded property count accepted")
	}
	source := bytes.Repeat([]byte("bounded decoder fixture"), 100)
	encoded := lzfse.EncodeBuffer(source)
	if len(encoded) == 0 {
		t.Fatal("encode fixture")
	}
	if _, _, err := decodePayload(context.Background(), testfixture.IM4P("dtre", encoded), 32); err == nil {
		t.Fatal("output cap ignored")
	}
	decoded, _, err := decodePayload(context.Background(), testfixture.IM4P("dtre", encoded), len(source))
	if err != nil || !bytes.Equal(decoded, source) {
		t.Fatalf("decode: %v", err)
	}
}

func TestDuplicateMembersAndCancelledHash(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ambiguous.ipsw")
	f, _ := os.Create(path)
	z := zip.NewWriter(f)
	for range 2 {
		w, _ := z.Create("BuildManifest.plist")
		w.Write([]byte("bad"))
	}
	z.Close()
	f.Close()
	if _, err := Open(context.Background(), path); err == nil {
		t.Fatal("duplicate accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Open(ctx, path); err == nil {
		t.Fatal("cancellation ignored")
	}
}

func TestCompareZeroMissingAndConflictingReadings(t *testing.T) {
	zero := model.Observation{
		Subject:  "device",
		Property: "reg",
		Presence: "present",
		Value:    json.RawMessage(`0`),
	}
	changes := Compare([]model.Finding{{Observation: zero}}, nil)
	if changes[0].State != "unexamined" {
		t.Fatal("missing treated as zero/absence")
	}
	changed := zero
	changed.Value = json.RawMessage(`1`)
	changes = Compare(
		[]model.Finding{{Observation: zero}, {Observation: changed}},
		[]model.Finding{{Observation: zero}},
	)
	if changes[0].State != "conflict" {
		t.Fatal("contradiction discarded")
	}
}

// Limits cross from the ZIP uint64 domain into io.LimitReader's signed domain.
// An empty member must not turn an overflowing limit into a successful read.
func TestArchiveRejectsUnrepresentableReadLimit(t *testing.T) {
	var data bytes.Buffer
	writer := zip.NewWriter(&data)
	if _, err := writer.CreateHeader(
		&zip.FileHeader{Name: "empty", Method: zip.Store},
	); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	reader, err := zip.NewReader(bytes.NewReader(data.Bytes()), int64(data.Len()))
	if err != nil {
		t.Fatal(err)
	}
	a := Archive{ZIP: reader, members: map[string]int{"empty": 0}}
	for _, limit := range []uint64{math.MaxInt64, math.MaxUint64} {
		if _, _, err := a.Read(
			context.Background(),
			"empty",
			limit,
		); !errors.Is(
			err,
			errInvalidMemberLimit,
		) {
			t.Fatalf("limit %d: %v", limit, err)
		}
	}
	if _, _, err := a.Read(context.Background(), "empty", 1); err != nil {
		t.Fatal(err)
	}
}

func TestReaderAdaptersPreserveEOFAndCancellation(t *testing.T) {
	ctx := context.Background()
	buffer := make([]byte, 4)
	if n, err := (contextReader{ctx, bytes.NewReader(nil)}).Read(buffer); n != 0 || err != io.EOF {
		t.Fatalf("Reader EOF: %d, %v", n, err)
	}
	source := &captureSource{
		ctx:           ctx,
		SectionReader: io.NewSectionReader(bytes.NewReader([]byte("ab")), 0, 2),
		seen:          map[int64][]byte{},
	}
	if n, err := source.ReadAt(buffer, 0); n != 2 || err != io.EOF {
		t.Fatalf("ReaderAt partial EOF: %d, %v", n, err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	source.ctx = cancelled
	if _, err := source.ReadAt(buffer[:2], 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled cached read: %v", err)
	}
}
