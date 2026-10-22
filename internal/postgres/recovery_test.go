package postgres

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/deploymenttheory/go-ios-vm/internal/model"
	"github.com/deploymenttheory/go-ios-vm/internal/workflow"
)

func recoveryFixture(t *testing.T, s *Store, id string, plan []string) (model.Run, model.Target) {
	t.Helper()
	ctx := context.Background()
	data := []byte(id + " IPSW")
	artifact := model.Artifact{ID: model.Digest(data), SHA256: model.Digest(data), Size: int64(len(data))}
	target := model.Target{ID: model.ID(artifact.ID, "0"), ArtifactID: artifact.ID}
	if err := s.Register(ctx, artifact, []model.Target{target}); err != nil {
		t.Fatal(err)
	}
	run := model.Run{ID: id, TargetID: target.ID, Started: time.Now(),
		InputSHA256: artifact.SHA256, InputSize: artifact.Size, Extractor: "fixture/v1",
		Planned: plan, LeaseToken: "first-worker"}
	if err := s.Start(ctx, run); err != nil {
		t.Fatal(err)
	}
	return run, target
}

func TestCrashedRunIsDiscoverableAndResumesExactIdentity(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	run, _ := recoveryFixture(t, s, "crashed-run", []string{"manifest", "tree"})
	first := model.ObservationBatch{Component: "manifest", Final: &model.Coverage{Component: "manifest", State: "complete"}}
	if err := s.PublishBatch(ctx, run, first); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReconcileStale(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.Resume(ctx, run.ID, "other-worker"); !errors.Is(err, errRunLeaseUnavailable) {
		t.Fatalf("active worker was stolen: %v", err)
	}
	if _, err := s.pool.Exec(ctx, `UPDATE analysis_run SET lease_expires=now()-interval '1 second' WHERE id=$1`, run.ID); err != nil {
		t.Fatal(err)
	}
	recoverable, err := s.ReconcileStale(ctx)
	if err != nil || len(recoverable) != 1 || recoverable[0].State != "interrupted" {
		t.Fatalf("crash recovery: %+v %v", recoverable, err)
	}
	stored := recoverable[0]
	if stored.InputSHA256 != run.InputSHA256 || stored.InputSize != run.InputSize ||
		stored.Extractor != run.Extractor || len(stored.Coverage) != 0 {
		t.Fatalf("reconciliation changed identity or invented coverage: %+v", stored)
	}
	if err := s.PublishBatch(ctx, run, model.ObservationBatch{Component: "tree"}); !errors.Is(err, errRunIsAlreadyTerminal) {
		t.Fatalf("crashed worker published after reconciliation: %v", err)
	}
	if err := s.Resume(ctx, run.ID, "second-worker"); err != nil {
		t.Fatal(err)
	}
	run.LeaseToken = "second-worker"
	checkpoints, err := s.Checkpoints(ctx, run.ID)
	if err != nil || checkpoints["manifest"].State != "complete" {
		t.Fatalf("checkpoint lost: %+v %v", checkpoints, err)
	}
	if err := s.PublishBatch(ctx, run, model.ObservationBatch{
		Component: "tree", Final: &model.Coverage{Component: "tree", State: "unexamined", Reason: "fixture"},
	}); err != nil {
		t.Fatal(err)
	}
	finished, err := s.Finish(ctx, run.ID, run.LeaseToken)
	if err != nil || finished.State != "partial" || len(finished.Coverage) != 2 {
		t.Fatalf("resumed finalization: %+v %v", finished, err)
	}
}

func TestConcurrentFinalizationReturnsOneCommittedResult(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	run, _ := recoveryFixture(t, s, "concurrent-finish", []string{"manifest"})
	if err := s.PublishBatch(ctx, run, model.ObservationBatch{
		Component: "manifest", Final: &model.Coverage{Component: "manifest", State: "complete"},
	}); err != nil {
		t.Fatal(err)
	}
	const attempts = 8
	results := make(chan model.Run, attempts)
	errorsCh := make(chan error, attempts)
	var wg sync.WaitGroup
	for range attempts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			finished, err := s.Finish(ctx, run.ID, run.LeaseToken)
			results <- finished
			errorsCh <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errorsCh)
	for err := range errorsCh {
		if err != nil {
			t.Fatal(err)
		}
	}
	for finished := range results {
		if finished.State != "complete" || len(finished.Coverage) != 1 {
			t.Fatalf("contradictory result: %+v", finished)
		}
	}
	var rows int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM run_coverage WHERE run_id=$1`, run.ID).Scan(&rows); err != nil || rows != 1 {
		t.Fatalf("duplicate ledger: %d %v", rows, err)
	}
	if err := s.FailBatched(ctx, run.ID, "failed", run.LeaseToken); !errors.Is(err, errRunLeaseUnavailable) {
		t.Fatalf("failure overwrote completion: %v", err)
	}
	if _, err := s.pool.Exec(ctx, `DELETE FROM run_coverage WHERE run_id=$1`, run.ID); err == nil {
		t.Fatal("PostgreSQL allowed deleting terminal coverage")
	}
}

func TestDatabaseRejectsContradictoryTerminalCoverage(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	run, _ := recoveryFixture(t, s, "coverage-guard", []string{"manifest"})
	if _, err := s.pool.Exec(ctx, `UPDATE analysis_run SET state='complete',finished=now() WHERE id=$1`, run.ID); err == nil {
		t.Fatal("PostgreSQL finalized a run without coverage")
	}
	if err := s.PublishBatch(ctx, run, model.ObservationBatch{
		Component: "manifest", Final: &model.Coverage{Component: "manifest", State: "unexamined"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, `INSERT INTO run_coverage(run_id,component,state) VALUES($1,'manifest','unexamined')`, run.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, `UPDATE analysis_run SET state='complete',finished=now() WHERE id=$1`, run.ID); err == nil {
		t.Fatal("PostgreSQL treated unexamined coverage as complete")
	}
	if _, err := s.pool.Exec(ctx, `UPDATE analysis_run SET state='partial',finished=now() WHERE id=$1`, run.ID); err != nil {
		t.Fatalf("valid partial state rejected: %v", err)
	}
	if _, err := s.pool.Exec(ctx, `UPDATE analysis_run SET state='failed' WHERE id=$1`, run.ID); err == nil {
		t.Fatal("PostgreSQL changed a finalized state")
	}
}

func TestFinalizationDatabaseFailureRollsBackLedgerAndCanRetry(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	run, _ := recoveryFixture(t, s, "finish-db-failure", []string{"manifest"})
	if err := s.PublishBatch(ctx, run, model.ObservationBatch{
		Component: "manifest", Final: &model.Coverage{Component: "manifest", State: "complete"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, `CREATE FUNCTION reject_finish() RETURNS trigger LANGUAGE plpgsql AS $$
	 BEGIN IF NEW.state='complete' THEN RAISE EXCEPTION 'injected finish failure'; END IF;
	 RETURN NEW; END $$`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, `CREATE TRIGGER reject_finish BEFORE UPDATE ON analysis_run
	 FOR EACH ROW EXECUTE FUNCTION reject_finish()`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Finish(ctx, run.ID, run.LeaseToken); err == nil {
		t.Fatal("injected finalization failure was ignored")
	}
	stored, err := s.Run(ctx, run.ID)
	if err != nil || stored.State != "running" || len(stored.Coverage) != 0 {
		t.Fatalf("failed transaction published terminal result: %+v %v", stored, err)
	}
	if _, err := s.pool.Exec(ctx, `DROP TRIGGER reject_finish ON analysis_run`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, `DROP FUNCTION reject_finish()`); err != nil {
		t.Fatal(err)
	}
	finished, err := s.Finish(ctx, run.ID, run.LeaseToken)
	if err != nil || finished.State != "complete" || len(finished.Coverage) != 1 {
		t.Fatalf("retry finalization: %+v %v", finished, err)
	}
}

func TestBatchedCancellationRemainsDistinctFromCoverage(t *testing.T) {
	s := testStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	data := []byte("cancelled fixture IPSW")
	artifact := model.Artifact{ID: model.Digest(data), SHA256: model.Digest(data), Size: int64(len(data))}
	target := model.Target{ID: model.ID(artifact.ID, "0"), ArtifactID: artifact.ID}
	run, err := workflow.InspectBatched(
		ctx, s, artifact, []model.Target{target}, 0, "", "fixture/v1", []string{"manifest", "tree"},
		func(_ context.Context, _ map[string]bool, emit func(string, []model.Observation, model.Coverage) error) error {
			if err := emit("manifest", nil, model.Coverage{Component: "manifest", State: "complete"}); err != nil {
				return err
			}
			cancel()
			return ctx.Err()
		},
	)
	if !errors.Is(err, context.Canceled) || run.State != "cancelled" {
		t.Fatalf("cancellation result: %+v %v", run, err)
	}
	stored, err := s.Run(context.Background(), run.ID)
	if err != nil || stored.State != "cancelled" || len(stored.Coverage) != 0 {
		t.Fatalf("cancellation invented final coverage: %+v %v", stored, err)
	}
	checkpoints, err := s.Checkpoints(context.Background(), run.ID)
	if err != nil || checkpoints["manifest"].State != "complete" {
		t.Fatalf("completed batch was lost: %+v %v", checkpoints, err)
	}
}

type finishBeforeResumeStore struct {
	*Store
	owner string
}

func (s *finishBeforeResumeStore) Resume(ctx context.Context, id, token string) error {
	if _, err := s.Store.Finish(ctx, id, s.owner); err != nil {
		return err
	}
	return s.Store.Resume(ctx, id, token)
}

func TestResumeReturnsCommitThatRacesWithClaim(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	run, target := recoveryFixture(t, s, "finish-before-resume", []string{"manifest"})
	if err := s.PublishBatch(ctx, run, model.ObservationBatch{
		Component: "manifest", Final: &model.Coverage{Component: "manifest", State: "complete"},
	}); err != nil {
		t.Fatal(err)
	}
	artifact := model.Artifact{ID: target.ArtifactID, SHA256: run.InputSHA256, Size: run.InputSize}
	finished, err := workflow.InspectBatched(
		ctx, &finishBeforeResumeStore{Store: s, owner: run.LeaseToken}, artifact,
		[]model.Target{target}, 0, run.ID, run.Extractor, run.Planned,
		func(context.Context, map[string]bool, func(string, []model.Observation, model.Coverage) error) error {
			t.Fatal("committed run was inspected again")
			return nil
		},
	)
	if err != nil || finished.State != "complete" || len(finished.Coverage) != 1 {
		t.Fatalf("race with finalization: %+v %v", finished, err)
	}
}
