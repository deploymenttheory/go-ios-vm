package postgres

import (
	"context"
	"encoding/json"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/tern/v2/migrate"

	"github.com/deploymenttheory/go-ios-vm/internal/model"
)

func TestRevisionChainsPreserveConflictingEvidence(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	a := model.Artifact{
		ID:     model.Digest([]byte("archive")),
		SHA256: model.Digest([]byte("archive")),
		Size:   7,
	}
	target := model.Target{ID: model.ID(a.ID, "0"), ArtifactID: a.ID}
	other := model.Target{ID: model.ID(a.ID, "1"), ArtifactID: a.ID, Identity: 1}
	if err := s.Register(ctx, a, []model.Target{target, other}); err != nil {
		t.Fatal(err)
	}
	makeObservation := func(tgt model.Target, value int, extractor string) model.Observation {
		t.Helper()
		o, err := model.NewObservation(
			tgt,
			"device",
			"address",
			value,
			model.Location{
				ArtifactID: a.ID,
				Member:     "tree",
				Coordinate: "plist-selector",
				Selector:   "/device/address",
			},
			model.NewBlob([]byte(extractor+" bytes")),
			extractor,
		)
		if err != nil {
			t.Fatal(err)
		}
		return o
	}
	o1 := makeObservation(target, 1, "parser/v1")
	o2 := makeObservation(target, 2, "parser/v2")
	oOther := makeObservation(other, 3, "parser/other")
	for i, item := range []struct {
		target       model.Target
		observations []model.Observation
	}{
		{target, []model.Observation{o1, o2}}, {other, []model.Observation{oOther}},
	} {
		run := model.Run{
			ID:       model.ID("revision-test-run", string(rune('a'+i))),
			TargetID: item.target.ID,
			State:    "complete",
			Started:  time.Now(),
			Coverage: []model.Coverage{{Component: "tree", State: "complete"}},
		}
		if err := s.Start(ctx, run); err != nil {
			t.Fatal(err)
		}
		if err := s.Publish(ctx, run, item.observations); err != nil {
			t.Fatal(err)
		}
	}
	chains, err := s.FindingChains(ctx, target.ID, "")
	if err != nil || len(chains) != 1 || len(chains[0].Revisions) != 2 {
		t.Fatal("parser conflict lost", chains, err)
	}
	chain := chains[0]
	first := chain.Revisions[0]
	if first.Observation == nil || first.Observation.ID == "" {
		t.Fatal("missing direct observation")
	}
	inferredReceipt, err := s.AddRevision(ctx, model.RevisionDraft{
		TargetID:   target.ID,
		Subject:    "device",
		Property:   "address",
		Basis:      "inferred",
		Presence:   "present",
		Value:      json.RawMessage(`{"kind":"candidate","address":{"u64":"1"}}`),
		Rationale:  "the first parser's interpretation is plausible, but the second conflicts",
		Author:     "analyst",
		Supersedes: first.ID,
		References: []model.BasisReference{
			{Role: "supporting", ObservationID: o1.ID},
			{Role: "contradicting", ObservationID: o2.ID},
			{Role: "supporting", RevisionID: first.ID},
		},
	})
	if err != nil || inferredReceipt.FindingID != chain.ID || inferredReceipt.RevisionID == "" {
		t.Fatal("inference receipt missing", err)
	}
	inferred, err := s.FindingChain(ctx, inferredReceipt.FindingID)
	if err != nil || len(inferred.Revisions) != 3 {
		t.Fatal("inference missing", err)
	}
	var correction model.FindingRevision
	for _, revision := range inferred.Revisions {
		if revision.Supersedes == first.ID {
			correction = revision
		}
	}
	if correction.ID == "" || correction.Basis != "inferred" || len(correction.References) != 3 {
		t.Fatal("typed basis or linked correction missing", correction)
	}
	unknownReceipt, err := s.AddRevision(ctx, model.RevisionDraft{
		TargetID:   target.ID,
		Subject:    "device",
		Property:   "address",
		Basis:      "unknown",
		Rationale:  "the parser disagreement leaves the address unresolved",
		Author:     "analyst",
		Supersedes: correction.ID,
		References: []model.BasisReference{{Role: "contradicting", RevisionID: correction.ID}},
	})
	if err != nil || unknownReceipt.FindingID != chain.ID || unknownReceipt.RevisionID == "" {
		t.Fatal("unknown receipt missing", err)
	}
	unknown, err := s.FindingChain(ctx, unknownReceipt.FindingID)
	if err != nil || len(unknown.Revisions) != 4 {
		t.Fatal("unknown chain missing", err)
	}
	observedReceipt, err := s.AddRevision(ctx, model.RevisionDraft{
		TargetID: target.ID, Subject: "device", Property: "address", Basis: "observed",
		ObservationID: o2.ID, Rationale: "corrected parser reading", Author: "analyst",
		Supersedes: correction.ID,
	})
	if err != nil || observedReceipt.FindingID != chain.ID || observedReceipt.RevisionID == "" {
		t.Fatal("observed receipt missing", err)
	}
	observed, err := s.FindingChain(ctx, observedReceipt.FindingID)
	if err != nil || len(observed.Revisions) != 5 {
		t.Fatal("observed correction missing", err)
	}
	foundObservedCorrection := false
	for _, revision := range observed.Revisions {
		if revision.Supersedes == correction.ID && revision.Basis == "observed" {
			foundObservedCorrection = revision.Observation != nil &&
				revision.Observation.ID == o2.ID &&
				string(revision.Value) == string(o2.Value) &&
				revision.Presence == o2.Presence
		}
	}
	if !foundObservedCorrection {
		t.Fatal("observed correction did not derive its value from published evidence")
	}
	if _, err = s.pool.Exec(ctx,
		"UPDATE finding_revision SET rationale='overwrite' WHERE id=$1", first.ID); err == nil {
		t.Fatal("earlier revision was mutable")
	}
	if _, err = s.pool.Exec(ctx,
		"UPDATE observation SET document='{}'::jsonb WHERE id=$1", o1.ID); err == nil {
		t.Fatal("raw observation was mutable")
	}
	for _, bad := range []model.RevisionDraft{
		{TargetID: target.ID, Subject: "device", Property: "address", Basis: "inferred", Presence: "absent", Rationale: "invalid foreign evidence", Author: "test", References: []model.BasisReference{{Role: "supporting", ObservationID: oOther.ID}}},
		{TargetID: target.ID, Subject: "device", Property: "address", Basis: "inferred", Presence: "absent", Rationale: "missing evidence", Author: "test", References: []model.BasisReference{{Role: "supporting", ObservationID: "missing"}}},
		{TargetID: target.ID, Subject: "other", Property: "property", Basis: "unknown", Rationale: "wrong chain", Author: "test", Supersedes: first.ID},
	} {
		if _, err := s.AddRevision(ctx, bad); err == nil {
			t.Fatal("invalid cross-target/missing/supersedes reference accepted", bad)
		}
	}
	retained, err := s.Evidence(ctx, first.Observation.EvidenceID)
	expectedBytes := "parser/v1 bytes"
	if first.Observation.ID == o2.ID {
		expectedBytes = "parser/v2 bytes"
	}
	if err != nil || string(retained) != expectedBytes {
		t.Fatal("original supporting bytes changed", err)
	}
	final, err := s.FindingChain(ctx, chain.ID)
	if err != nil || len(final.Revisions) != 5 {
		t.Fatal("failed revision leaked or chain lost", err)
	}
	t.Run("backup restore", func(t *testing.T) {
		if os.Getenv("GOIOSVM_TEST_DOCKER_BACKUP") != "1" {
			t.Skip("set GOIOSVM_TEST_DOCKER_BACKUP=1 for Docker pg_dump/pg_restore integration")
		}
		verifyRevisionBackupRestore(t, s, final, first.Observation.EvidenceID)
	})
}

func verifyRevisionBackupRestore(
	t *testing.T,
	s *Store,
	original model.FindingChain,
	evidenceID string,
) {
	t.Helper()
	ctx := context.Background()
	schema := s.pool.Config().ConnConfig.RuntimeParams["search_path"]
	if !strings.HasPrefix(schema, "test_") {
		t.Fatal("backup test requires an isolated test schema")
	}
	name := "revision_restore_" + strings.TrimPrefix(schema, "test_")
	compose := []string{
		"compose",
		"-p",
		"go-ios-vm",
		"-f",
		"../../compose.yaml",
		"exec",
		"-T",
		"postgres",
	}
	run := func(input []byte, args ...string) []byte {
		t.Helper()
		cmd := exec.CommandContext(ctx, "docker", append(compose, args...)...)
		cmd.Stdin = strings.NewReader(string(input))
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("Docker database command failed: %s: %v: %s", args[0], err, output)
		}
		return output
	}
	run(nil, "createdb", "-U", "goiosvm", name)
	t.Cleanup(func() { run(nil, "dropdb", "-U", "goiosvm", name) })
	baseURL, err := url.Parse(os.Getenv("GOIOSVM_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	database := strings.TrimPrefix(baseURL.Path, "/")
	backup := run(nil, "pg_dump", "-U", "goiosvm", "-d", database, "-Fc", "-n", schema)
	run(backup, "pg_restore", "-U", "goiosvm", "-d", name, "--no-owner", "--exit-on-error")
	baseURL.Path = "/" + name
	query := baseURL.Query()
	query.Set("search_path", schema)
	baseURL.RawQuery = query.Encode()
	restored, err := Open(ctx, baseURL.String())
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	got, err := restored.FindingChain(ctx, original.ID)
	if err != nil || !reflect.DeepEqual(got, original) {
		t.Fatal("backup/restore changed revision chain or basis references", err)
	}
	before, err := s.Evidence(ctx, evidenceID)
	if err != nil {
		t.Fatal(err)
	}
	after, err := restored.Evidence(ctx, evidenceID)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("backup/restore changed supporting bytes", err)
	}
}

func TestMigrationBackfillsLegacyObservedRevision(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	conn, err := s.pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	m, err := migrate.NewMigrator(ctx, conn.Conn(), "schema_version")
	if err != nil {
		t.Fatal(err)
	}
	sub, err := fs.Sub(migrations, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	if err = m.LoadMigrations(sub); err != nil {
		t.Fatal(err)
	}
	if err = m.MigrateTo(ctx, 1); err != nil {
		t.Fatal(err)
	}
	conn.Release()
	a := model.Artifact{
		ID:     model.Digest([]byte("legacy archive")),
		SHA256: model.Digest([]byte("legacy archive")),
		Size:   14,
	}
	target := model.Target{ID: model.ID(a.ID, "0"), ArtifactID: a.ID}
	if err := s.Register(ctx, a, []model.Target{target}); err != nil {
		t.Fatal(err)
	}
	bytes := []byte("legacy support")
	o, err := model.NewObservation(
		target,
		"legacy-device",
		"register",
		0,
		model.Location{
			ArtifactID: a.ID,
			Member:     "tree",
			Coordinate: "plist-selector",
			Selector:   "/legacy",
		},
		model.NewBlob(bytes),
		"legacy/parser",
	)
	if err != nil {
		t.Fatal(err)
	}
	run := model.Run{
		ID: "legacy-run", TargetID: target.ID, Started: time.Now(), State: "partial",
		Coverage: []model.Coverage{{Component: "tree", State: "complete"}},
	}
	if _, err = s.pool.Exec(
		ctx,
		"INSERT INTO analysis_run(id,target_id,state,started,coverage) VALUES($1,$2,'running',$3,$4)",
		run.ID,
		run.TargetID,
		run.Started,
		`[]`,
	); err != nil {
		t.Fatal(err)
	}
	document, err := json.Marshal(o)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rollback(ctx, tx)
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{"INSERT INTO evidence_blob(id,size,complete) VALUES($1,$2,true)", []any{o.EvidenceID, len(bytes)}},
		{"INSERT INTO evidence_chunk(id,data) VALUES($1,$2)", []any{o.EvidenceID, bytes}},
		{"INSERT INTO blob_chunk(blob_id,ordinal,chunk_id) VALUES($1,0,$1)", []any{o.EvidenceID}},
		{"INSERT INTO observation(id,target_id,evidence_id,document) VALUES($1,$2,$3,$4)", []any{o.ID, target.ID, o.EvidenceID, document}},
		{"INSERT INTO run_observation(run_id,observation_id) VALUES($1,$2)", []any{run.ID, o.ID}},
		{"INSERT INTO finding_revision(id,observation_id,basis,rationale) VALUES($1,$2,'observed','original reading')", []any{"legacy-revision", o.ID}},
	} {
		if _, err = tx.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatal(err)
		}
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err = s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	chains, err := s.FindingChains(ctx, target.ID, "")
	if err != nil || len(chains) != 1 || len(chains[0].Revisions) != 1 {
		t.Fatal("legacy chain not backfilled", err)
	}
	legacy := chains[0].Revisions[0]
	if legacy.ID != "legacy-revision" || legacy.Observation == nil ||
		legacy.Observation.ID != o.ID ||
		len(legacy.References) != 1 ||
		legacy.References[0].ObservationID != o.ID {
		t.Fatal("legacy direct evidence link lost", legacy)
	}
	retained, err := s.Evidence(ctx, o.EvidenceID)
	if err != nil || string(retained) != string(bytes) {
		t.Fatal("legacy bytes lost", err)
	}
}
