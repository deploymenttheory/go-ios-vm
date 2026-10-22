package inspection

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
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
			data, _, err := decodePayload(o.Evidence, TreeLimit)
			if err != nil {
				t.Fatal(err)
			}
			if binary.LittleEndian.Uint64(data[o.Source.Offset:o.Source.Offset+o.Source.Length]) != ^uint64(0) {
				t.Fatal("source offset does not recover exact unsigned value")
			}
		}
		if o.Subject == "driver/test.synthetic.driver" && o.Property == "OSBundleLibraries" {
			foundDriver = true
		}
	}
	if !foundTree || !foundDriver {
		t.Fatalf("missing extraction: tree=%v driver=%v coverage=%+v", foundTree, foundDriver, result.Coverage)
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
	if _, _, err := decodePayload(testfixture.IM4P("dtre", encoded), 32); err == nil {
		t.Fatal("output cap ignored")
	}
	decoded, _, err := decodePayload(testfixture.IM4P("dtre", encoded), len(source))
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
	zero := model.Observation{Subject: "device", Property: "reg", Presence: "present", Value: json.RawMessage(`0`)}
	changes := Compare([]model.Finding{{Observation: zero}}, nil)
	if changes[0].State != "unexamined" {
		t.Fatal("missing treated as zero/absence")
	}
	changed := zero
	changed.Value = json.RawMessage(`1`)
	changes = Compare([]model.Finding{{Observation: zero}, {Observation: changed}}, []model.Finding{{Observation: zero}})
	if changes[0].State != "conflict" {
		t.Fatal("contradiction discarded")
	}
}
