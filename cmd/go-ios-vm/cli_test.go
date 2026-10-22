package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/go-ios-vm/internal/acquisition"
	"github.com/deploymenttheory/go-ios-vm/internal/model"
	"github.com/deploymenttheory/go-ios-vm/internal/testfixture"
)

func TestListingTextReportsPartialSourcesAndExactNextCommand(t *testing.T) {
	listing := acquisition.BuildListing([]acquisition.Response{
		{Source: "https://gdmf.apple.com/fixture", Status: "failed", Error: "offline"},
		{
			Source: "https://api.ipsw.me/fixture",
			Status: "complete",
			Candidates: []acquisition.Candidate{
				{
					Product: "iPhone12,1",
					Version: "14.0",
					Build:   "18A373",
					Kind:    "restore_ipsw",
					URL:     "https://updates.cdn-apple.com/fixture.ipsw",
					Source:  "https://api.ipsw.me/fixture",
				},
			},
		},
	}, "iPhone12,1", "14.0")
	if listingState(listing) != "partial" {
		t.Fatal("failed source reported complete")
	}
	var out bytes.Buffer
	printFirmwareListing(&out, listingState(listing), listing)
	for _, text := range []string{"PARTIAL", "Source failed", "unverified_catalog_locator", "firmware fetch --product 'iPhone12,1' --version '14.0' --build '18A373'"} {
		if !bytes.Contains(out.Bytes(), []byte(text)) {
			t.Fatalf("missing %q in %s", text, out.String())
		}
	}
}

func TestHelpAndConfigDoNotNeedDatabase(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"--format", "bad", "cache", "status"}} {
		var out, errout bytes.Buffer
		c := newCommand(&out, &errout)
		c.SetArgs(args)
		err := c.ExecuteContext(context.Background())
		if args[0] == "--help" && err != nil {
			t.Fatal(err)
		}
		if args[0] == "--format" && err == nil {
			t.Fatal("bad format accepted")
		}
	}
}

func TestPersistRestartCompareAndRemoveSource(t *testing.T) {
	dsn := os.Getenv("GOIOSVM_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("PostgreSQL integration")
	}
	admin, err := pgx.Connect(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	var nonce [8]byte
	rand.Read(nonce[:])
	schema := "cli_test_" + hex.EncodeToString(nonce[:])
	if _, err = admin.Exec(context.Background(), "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
		admin.Close(context.Background())
	})
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal("invalid test URL")
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	t.Setenv("GOIOSVM_DATABASE_URL", u.String())
	cache := t.TempDir()
	t.Setenv("GOIOSVM_CACHE_DIR", cache)
	lastSchemaVersion := 0
	invoke := func(args ...string) (json.RawMessage, error) {
		t.Helper()
		var out, errout bytes.Buffer
		c := newCommand(&out, &errout)
		c.SetArgs(append([]string{"--format", "json"}, args...))
		err := c.ExecuteContext(context.Background())
		if out.Len() == 0 {
			return nil, err
		}
		var e struct {
			SchemaVersion int             `json:"schema_version"`
			Results       json.RawMessage `json:"results"`
		}
		if decodeErr := json.Unmarshal(out.Bytes(), &e); decodeErr != nil {
			t.Fatal(decodeErr, out.String())
		}
		lastSchemaVersion = e.SchemaVersion
		return e.Results, err
	}
	if _, err := invoke("db", "migrate"); err != nil {
		t.Fatal(err)
	}
	var runs []model.Run
	for i, version := range []string{"14.0", "15.0"} {
		path := filepath.Join(cache, version+".ipsw")
		if err := testfixture.IPSW(path, version, uint64(i)); err != nil {
			t.Fatal(err)
		}
		b, err := invoke("inspect", "--image", path, "--identity", "0")
		if _, ok := err.(*partialError); !ok {
			t.Fatalf("expected truthful partial coverage, got %v", err)
		}
		if i == 0 {
			var stdout, stderr bytes.Buffer
			code := runCLI(context.Background(), []string{"--format", "json", "inspect", "--image", path, "--identity", "0"}, &stdout, &stderr)
			if code != 2 {
				t.Fatalf("partial inspection exit %d: %s", code, stderr.String())
			}
			response := decodeOneEnvelope(t, stdout.Bytes())
			if response.State != "partial" || response.Results == nil || response.Diagnostic != nil ||
				!bytes.Contains(stderr.Bytes(), []byte("Hashing IPSW")) {
				t.Fatalf("partial output mixed with progress: %s; stderr %s", stdout.String(), stderr.String())
			}
		}
		var run model.Run
		if err = json.Unmarshal(b, &run); err != nil {
			t.Fatal(err)
		}
		runs = append(runs, run)
		if err = os.Remove(path); err != nil {
			t.Fatal(err)
		}
	}
	b, err := invoke("finding", "list", "--run", runs[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if lastSchemaVersion != 2 {
		t.Fatal("finding chains must identify their output schema version")
	}
	var findings []model.FindingChain
	if err = json.Unmarshal(b, &findings); err != nil || len(findings) == 0 {
		t.Fatal(err)
	}
	if len(findings[0].Revisions) == 0 || findings[0].Revisions[0].Observation == nil {
		t.Fatal("direct finding revision missing observation", findings[0])
	}
	initial := findings[0].Revisions[0]
	b, err = invoke("finding", "revise", "--target", findings[0].TargetID,
		"--subject", findings[0].Subject, "--property", findings[0].Property,
		"--basis", "inferred", "--presence", "present", "--value", `{"kind":"test-interpretation"}`,
		"--rationale", "fixture interpretation", "--supersedes", initial.ID,
		"--support-observation", initial.Observation.ID)
	if err != nil {
		t.Fatal(err)
	}
	var receipt model.RevisionReceipt
	if err = json.Unmarshal(b, &receipt); err != nil || receipt.FindingID != findings[0].ID || receipt.RevisionID == "" {
		t.Fatal("committed revision identity not returned", err)
	}
	if lastSchemaVersion != 2 {
		t.Fatal("revision receipt must identify its output schema version")
	}
	b, err = invoke("finding", "show", "--finding", receipt.FindingID)
	if err != nil {
		t.Fatal(err)
	}
	var revised model.FindingChain
	if err = json.Unmarshal(b, &revised); err != nil || len(revised.Revisions) != 2 {
		t.Fatal("revision chain not shown", err)
	}
	linked := false
	for _, revision := range revised.Revisions {
		linked = linked || revision.Supersedes == initial.ID
	}
	if !linked {
		t.Fatal("revision did not link to earlier reading")
	}
	if _, err = invoke("finding", "show", "--finding", findings[0].ID); err != nil {
		t.Fatal("finding show failed", err)
	}
	if _, err = invoke(
		"finding",
		"show",
		"--evidence",
		initial.Observation.EvidenceID,
	); err != nil {
		t.Fatal("source removal destroyed retained evidence", err)
	}
	if lastSchemaVersion != 1 {
		t.Fatal("unchanged evidence output must keep schema version 1")
	}
	if _, err = invoke("compare", "--left", runs[0].ID, "--right", runs[1].ID); err != nil {
		t.Fatal(err)
	}
}
