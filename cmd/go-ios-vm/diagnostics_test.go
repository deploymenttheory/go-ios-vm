package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/deploymenttheory/go-ios-vm/internal/model"
)

func decodeOneEnvelope(t *testing.T, data []byte) envelope {
	t.Helper()
	var value envelope
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&value); err != nil {
		t.Fatalf("invalid JSON envelope: %v: %s", err, data)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		t.Fatalf("stdout has more than one JSON value: %v: %s", err, data)
	}
	if value.SchemaVersion != model.SchemaVersion {
		t.Fatalf("schema version %d", value.SchemaVersion)
	}
	return value
}

func TestJSONSuccessInvalidFatalAndCancelled(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("GOIOSVM_CACHE_DIR", cache)
	t.Setenv("GOIOSVM_DATABASE_URL", "")
	tests := []struct {
		name, state, code string
		args              []string
		cancel            bool
		exit              int
	}{
		{name: "success", state: "complete", args: []string{"--format", "json", "cache", "status"}, exit: 0},
		{name: "missing input", state: "failed", code: "missing_input", args: []string{"--format", "json", "target", "list"}, exit: 1},
		{name: "incomplete exact fetch", state: "failed", code: "missing_input", args: []string{"--format", "json", "firmware", "fetch", "--product", "iPhone12,1"}, exit: 1},
		{name: "invalid command", state: "failed", code: "invalid_invocation", args: []string{"unrecognized", "--format=json"}, exit: 1},
		{name: "finding selector", state: "failed", code: "invalid_invocation", args: []string{"--format", "json", "finding", "show"}, exit: 1},
		{name: "finding revision input", state: "failed", code: "invalid_invocation", args: []string{"--format", "json", "finding", "revise"}, exit: 1},
		{name: "storage failure", state: "failed", code: "storage_failure", args: []string{"--format", "json", "--database-url", "postgres://user:secret@localhost:bad/db", "db", "migrate"}, exit: 1},
		{name: "cancelled", state: "cancelled", code: "cancelled", args: []string{"--format", "json", "cache", "prune", "--apply"}, cancel: true, exit: 130},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			if tc.cancel {
				cancelled, cancel := context.WithCancel(ctx)
				cancel()
				ctx = cancelled
			}
			var stdout, stderr bytes.Buffer
			if exit := runCLI(ctx, tc.args, &stdout, &stderr); exit != tc.exit {
				t.Fatalf("exit %d, expected %d; stdout %s; stderr %s", exit, tc.exit, stdout.String(), stderr.String())
			}
			value := decodeOneEnvelope(t, stdout.Bytes())
			if value.State != tc.state {
				t.Fatalf("state %q, expected %q", value.State, tc.state)
			}
			if tc.code == "" {
				if value.Diagnostic != nil || value.Results == nil {
					t.Fatalf("success shape: %+v", value)
				}
			} else if value.Diagnostic == nil || value.Diagnostic.Code != tc.code || value.Results != nil {
				t.Fatalf("failure shape: %+v", value)
			}
			if strings.Contains(stdout.String()+stderr.String(), "secret") {
				t.Fatal("database password appeared in output")
			}
		})
	}
}

func TestPartialResponseKeepsSingleEnvelopeAndExitTwo(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := json.NewEncoder(&stdout).Encode(envelope{SchemaVersion: model.SchemaVersion, State: "partial", Results: map[string]string{"run": "retained"}}); err != nil {
		t.Fatal(err)
	}
	w := &countedWriter{writer: &stdout, bytes: stdout.Len()}
	if got := finishCLI(context.Background(), "json", w, &stderr, &partialError{}); got != 2 {
		t.Fatalf("exit %d", got)
	}
	value := decodeOneEnvelope(t, stdout.Bytes())
	if value.State != "partial" || value.Diagnostic != nil || stderr.Len() != 0 {
		t.Fatalf("partial response changed: %+v; stderr %s", value, stderr.String())
	}
}

func TestImageInputAndCredentialRedaction(t *testing.T) {
	if got := classify(imageInput(os.ErrNotExist)); got != "missing_input" {
		t.Fatalf("missing image classified %s", got)
	}
	if got := classify(imageInput(zip.ErrFormat)); got != "unsupported_or_corrupt" {
		t.Fatalf("corrupt image classified %s", got)
	}
	for _, value := range []string{
		"postgres://name:secret@localhost/db",
		"postgresql://name:secret@localhost/db",
		"postgres://name:se@cret@localhost/db",
		"host=localhost password=secret user=name",
	} {
		message := safeDiagnostic(errors.New(value)).Message
		if strings.Contains(message, "secret") {
			t.Fatalf("credential leaked from %q: %q", value, message)
		}
	}
}

func TestTextSummaryAvoidsStructuredDump(t *testing.T) {
	cache := t.TempDir()
	path := filepath.Join(cache, strings.Repeat("a", 64)+".ipsw")
	if err := os.WriteFile(path, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if code := runCLI(context.Background(), []string{"--cache-dir", cache, "cache", "status"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "1 managed cache entries") || !strings.Contains(stdout.String(), path) || strings.Contains(stdout.String(), "\"path\"") {
		t.Fatalf("unexpected text summary: %s", stdout.String())
	}
}

func TestFindingTextSummariesShowRevisionChainAndReceipt(t *testing.T) {
	chain := model.FindingChain{
		ID: "finding-1", TargetID: "target-1", Subject: "device", Property: "address",
		Revisions: []model.FindingRevision{
			{ID: "revision-1", Basis: "observed", Presence: "present", Value: json.RawMessage(`0`),
				Rationale: "direct reading", References: []model.BasisReference{{Role: "supporting", ObservationID: "observation-1",
					Observation: &model.Observation{EvidenceID: "digest-1", Source: model.Location{Member: "tree", Selector: "/device/address"}}}}},
			{ID: "revision-2", Basis: "unknown", Presence: "unexamined", Supersedes: "revision-1",
				Rationale: "parser disagreement", References: []model.BasisReference{{Role: "contradicting", RevisionID: "revision-1"}}},
		},
	}
	for _, tc := range []struct {
		name  string
		value any
		want  []string
	}{
		{"list", []model.FindingChain{chain}, []string{"1 findings", "device address", "revision-1", "revision-2"}},
		{"show", chain, []string{"finding revision chain", "value: 0", "rationale: parser disagreement", "contradicting revision revision-1", "source: tree /device/address; evidence digest-1"}},
		{"revise", model.RevisionReceipt{FindingID: "finding-1", RevisionID: "revision-3"}, []string{"revision-3 recorded", "finding show --finding finding-1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			if err := printSummary(&out, "complete", tc.value); err != nil {
				t.Fatal(err)
			}
			for _, part := range tc.want {
				if !strings.Contains(out.String(), part) {
					t.Fatalf("missing %q in %s", part, out.String())
				}
			}
		})
	}
}
