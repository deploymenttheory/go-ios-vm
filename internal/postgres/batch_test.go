package postgres

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/deploymenttheory/go-ios-vm/internal/model"
	"github.com/deploymenttheory/go-ios-vm/internal/workflow"
)

func TestInterruptedBatchesResumeWithoutReplayingCompletedComponents(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	image := []byte("verified fixture IPSW")
	artifact := model.Artifact{
		ID:     model.Digest(image),
		SHA256: model.Digest(image),
		Size:   int64(len(image)),
	}
	target := model.Target{ID: model.ID(artifact.ID, "0"), ArtifactID: artifact.ID}
	plan := []string{"manifest", "DeviceTree"}
	location := model.Location{
		ArtifactID: artifact.ID,
		Member:     "BuildManifest.plist",
		Coordinate: "plist-selector",
		Selector:   "/fixture",
		Length:     uint64(len(image)),
	}
	first, err := model.NewObservation(
		target,
		"fixture",
		"manifest",
		1,
		location,
		model.NewBlob(image),
		"fixture/v1",
	)
	if err != nil {
		t.Fatal(err)
	}
	second, err := model.NewObservation(
		target,
		"fixture",
		"tree",
		2,
		location,
		model.NewBlob(image),
		"fixture/v1",
	)
	if err != nil {
		t.Fatal(err)
	}
	tree := []model.Observation{second}
	for i := 1; i < 129; i++ {
		o, err := model.NewObservation(target, fmt.Sprintf("fixture/%d", i), "tree", i,
			location, model.NewBlob(image), "fixture/v1")
		if err != nil {
			t.Fatal(err)
		}
		tree = append(tree, o)
	}
	interrupted := errors.New("injected parser interruption")
	run, err := workflow.InspectBatched(
		ctx,
		s,
		artifact,
		[]model.Target{target},
		0,
		"",
		"fixture/v1",
		plan,
		func(_ context.Context, done map[string]bool, emit func(string, []model.Observation, model.Coverage) error) error {
			if len(done) != 0 {
				t.Fatal("new run has checkpoints")
			}
			if err := emit(
				"manifest",
				[]model.Observation{first},
				model.Coverage{Component: "manifest", State: "complete"},
			); err != nil {
				return err
			}
			var count int
			if err := s.pool.QueryRow(ctx, "SELECT count(*) FROM run_checkpoint").
				Scan(&count); err != nil {
				return err
			}
			if count != 1 {
				t.Fatalf("batch was not visible before next parse: %d", count)
			}
			return interrupted
		},
	)
	if !errors.Is(err, interrupted) || run.State != "failed" {
		t.Fatalf("interruption: %+v %v", run, err)
	}
	var batches, checkpoints, ledger int
	if err := s.pool.QueryRow(ctx, "SELECT count(*) FROM run_batch WHERE run_id=$1", run.ID).
		Scan(&batches); err != nil {
		t.Fatal(err)
	}
	if err := s.pool.QueryRow(ctx, "SELECT count(*) FROM run_checkpoint WHERE run_id=$1", run.ID).
		Scan(&checkpoints); err != nil {
		t.Fatal(err)
	}
	if err := s.pool.QueryRow(ctx, "SELECT count(*) FROM run_coverage WHERE run_id=$1", run.ID).
		Scan(&ledger); err != nil {
		t.Fatal(err)
	}
	if batches != 1 || checkpoints != 1 || ledger != 0 {
		t.Fatalf(
			"unfinished publication: batches=%d checkpoints=%d ledger=%d",
			batches,
			checkpoints,
			ledger,
		)
	}
	stored, err := s.Run(ctx, run.ID)
	if err != nil || stored.State != "failed" || len(stored.Coverage) != 0 {
		t.Fatalf("unfinished run: %+v %v", stored, err)
	}
	changed := artifact
	changed.SHA256 = model.Digest([]byte("different bytes"))
	if _, err := workflow.InspectBatched(
		ctx,
		s,
		changed,
		[]model.Target{target},
		0,
		run.ID,
		"fixture/v1",
		plan,
		func(context.Context, map[string]bool, func(string, []model.Observation, model.Coverage) error) error {
			t.Fatal("changed input decoded")
			return nil
		},
	); !errors.Is(
		err,
		workflow.ErrResumeInputMismatch,
	) {
		t.Fatalf("changed archive accepted: %v", err)
	}
	finished, err := workflow.InspectBatched(
		ctx,
		s,
		artifact,
		[]model.Target{target},
		0,
		run.ID,
		"fixture/v1",
		plan,
		func(_ context.Context, done map[string]bool, emit func(string, []model.Observation, model.Coverage) error) error {
			if !done["manifest"] || done["DeviceTree"] {
				t.Fatalf("wrong resume checkpoints: %+v", done)
			}
			return emit(
				"DeviceTree",
				tree,
				model.Coverage{Component: "DeviceTree", State: "complete"},
			)
		},
	)
	if err != nil || finished.State != "complete" || len(finished.Coverage) != 2 {
		t.Fatalf("resume: %+v %v", finished, err)
	}
	var blobs, observations int
	if err := s.pool.QueryRow(ctx, "SELECT count(*) FROM evidence_blob").Scan(&blobs); err != nil {
		t.Fatal(err)
	}
	if err := s.pool.QueryRow(ctx, "SELECT count(*) FROM run_observation WHERE run_id=$1", run.ID).
		Scan(&observations); err != nil {
		t.Fatal(err)
	}
	if blobs != 1 || observations != 130 {
		t.Fatalf(
			"retry duplicated evidence or lost observations: blobs=%d observations=%d",
			blobs,
			observations,
		)
	}
	if err := s.pool.QueryRow(ctx, "SELECT count(*) FROM run_batch WHERE run_id=$1", run.ID).
		Scan(&batches); err != nil {
		t.Fatal(err)
	}
	if batches != 3 {
		t.Fatalf("129 observations were not split into two bounded batches: %d", batches)
	}
	if err := s.pool.QueryRow(ctx, "SELECT count(*) FROM run_coverage WHERE run_id=$1", run.ID).
		Scan(&ledger); err != nil {
		t.Fatal(err)
	}
	if ledger != 2 {
		t.Fatalf("final coverage ledger has %d rows", ledger)
	}
}

func TestBatchDatabaseFailureRollbackAndIdempotentRetry(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	data := []byte("same retained bytes")
	artifact := model.Artifact{
		ID:     model.Digest(data),
		SHA256: model.Digest(data),
		Size:   int64(len(data)),
	}
	target := model.Target{ID: model.ID(artifact.ID, "0"), ArtifactID: artifact.ID}
	if err := s.Register(ctx, artifact, []model.Target{target}); err != nil {
		t.Fatal(err)
	}
	run := model.Run{
		ID: "batch-retry", TargetID: target.ID, Started: time.Now(), InputSHA256: artifact.SHA256,
		InputSize: artifact.Size, Extractor: "fixture/v1", Planned: []string{"manifest"}, LeaseToken: "fixture-worker",
	}
	if err := s.Start(ctx, run); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Finish(ctx, run.ID, run.LeaseToken); !errors.Is(err, errIncompleteBatchPlan) {
		t.Fatalf("premature finalization: %v", err)
	}
	location := model.Location{
		ArtifactID: artifact.ID,
		Member:     "BuildManifest.plist",
		Coordinate: "plist-selector",
		Selector:   "/fixture",
		Length:     uint64(len(data)),
	}
	good, err := model.NewObservation(
		target,
		"fixture",
		"value",
		7,
		location,
		model.NewBlob(data),
		"fixture/v1",
	)
	if err != nil {
		t.Fatal(err)
	}
	bad := good
	bad.TargetID = "wrong-target"
	coverage := model.Coverage{Component: "manifest", State: "complete"}
	tooLarge := model.ObservationBatch{
		Component: "manifest", Ordinal: 0,
		Observations: make([]model.Observation, model.MaxObservationsPerBatch+1), Final: &coverage,
	}
	if err := s.PublishBatch(ctx, run, tooLarge); !errors.Is(err, errBatchExceedsBound) {
		t.Fatalf("unbounded batch accepted: %v", err)
	}
	batch := model.ObservationBatch{
		Component:    "manifest",
		Ordinal:      0,
		Observations: []model.Observation{good, bad},
		Final:        &coverage,
	}
	if err := s.PublishBatch(ctx, run, batch); !errors.Is(err, errObservationTargetMismatch) {
		t.Fatalf("database rejection: %v", err)
	}
	var count int
	if err := s.pool.QueryRow(ctx, "SELECT count(*) FROM evidence_blob").
		Scan(&count); err != nil ||
		count != 0 {
		t.Fatalf("failed batch leaked evidence: %d %v", count, err)
	}
	if err := s.pool.QueryRow(ctx, "SELECT count(*) FROM run_checkpoint").
		Scan(&count); err != nil ||
		count != 0 {
		t.Fatalf("failed batch checkpointed: %d %v", count, err)
	}
	batch.Observations = []model.Observation{good}
	if _, err := s.pool.Exec(
		ctx,
		`CREATE FUNCTION reject_fixture_batch() RETURNS trigger LANGUAGE plpgsql AS $$
	 BEGIN RAISE EXCEPTION 'injected batch database failure'; END $$`,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, `CREATE TRIGGER reject_fixture_batch BEFORE INSERT ON run_batch
	 FOR EACH ROW EXECUTE FUNCTION reject_fixture_batch()`); err != nil {
		t.Fatal(err)
	}
	if err := s.PublishBatch(ctx, run, batch); err == nil {
		t.Fatal("database trigger failure was ignored")
	}
	if err := s.pool.QueryRow(ctx, "SELECT count(*) FROM evidence_blob").
		Scan(&count); err != nil ||
		count != 0 {
		t.Fatalf("database failure leaked incomplete evidence: %d %v", count, err)
	}
	if _, err := s.pool.Exec(ctx, "DROP TRIGGER reject_fixture_batch ON run_batch"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, "DROP FUNCTION reject_fixture_batch()"); err != nil {
		t.Fatal(err)
	}
	if err := s.PublishBatch(ctx, run, batch); err != nil {
		t.Fatal(err)
	}
	if err := s.PublishBatch(ctx, run, batch); err != nil {
		t.Fatalf("same batch was not idempotent: %v", err)
	}
	conflict := batch
	conflict.Observations = nil
	if err := s.PublishBatch(ctx, run, conflict); !errors.Is(err, errConflictingBatch) {
		t.Fatalf("conflicting replay accepted: %v", err)
	}
	if err := s.pool.QueryRow(ctx, "SELECT count(*) FROM run_batch").
		Scan(&count); err != nil ||
		count != 1 {
		t.Fatalf("retry duplicated batch: %d %v", count, err)
	}
	if err := s.pool.QueryRow(ctx, "SELECT count(*) FROM evidence_blob").
		Scan(&count); err != nil ||
		count != 1 {
		t.Fatalf("retry duplicated evidence: %d %v", count, err)
	}
	final, err := s.Finish(ctx, run.ID, run.LeaseToken)
	if err != nil || final.State != "complete" || len(final.Coverage) != 1 {
		t.Fatalf("final run: %+v %v", final, err)
	}
	if _, err := s.pool.Exec(
		ctx,
		`UPDATE analysis_run SET coverage='[]' WHERE id=$1`,
		run.ID,
	); err == nil {
		t.Fatal("terminal coverage document was mutable")
	}
	fromLedger, err := s.Run(ctx, run.ID)
	if err != nil || len(fromLedger.Coverage) != 1 ||
		fromLedger.Coverage[0].Component != "manifest" {
		t.Fatalf("final coverage did not come from the separate ledger: %+v %v", fromLedger, err)
	}
}
