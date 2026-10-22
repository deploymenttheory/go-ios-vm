package postgres

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/deploymenttheory/go-ios-vm/internal/model"
)

func TestProgressReaderProcess(t *testing.T) {
	if os.Getenv("GOIOSVM_PROGRESS_READER") != "1" {
		t.Skip("helper process")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s, err := Open(ctx, os.Getenv("GOIOSVM_PROGRESS_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for ctx.Err() == nil {
		p, err := s.Progress(ctx, os.Getenv("GOIOSVM_PROGRESS_ID"))
		if err == nil && p.Phase == "downloading" && p.CompletedBytes >= 64 {
			encoded, _ := json.Marshal(p)
			fmt.Println("PROGRESS_JSON:" + string(encoded))
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("second process did not see committed progress")
}

func TestAcquisitionProgressIsVisibleToAnotherProcessAndBounded(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	id, err := s.StartAcquisitionProgress(ctx, "iPhone12,1", "14.0", "18A373")
	if err != nil {
		t.Fatal(err)
	}
	initial, err := s.Progress(ctx, id)
	if err != nil || initial.Phase != "resolving" || initial.TotalBytes != nil ||
		initial.SourceFingerprint != "" {
		t.Fatalf("unknown total or source was invented: %+v %v", initial, err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestProgressReaderProcess$")
	cmd.Env = append(os.Environ(),
		"GOIOSVM_PROGRESS_READER=1",
		"GOIOSVM_PROGRESS_DSN="+s.pool.Config().ConnString(),
		"GOIOSVM_PROGRESS_ID="+id)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	total, downloaded := int64(128), int64(64)
	if err := s.AdvanceAcquisitionProgress(ctx, id, AcquisitionProgressUpdate{
		Phase: "downloading", CompletedBytes: &downloaded, TotalBytes: &total,
		SourceFingerprint: model.Digest([]byte("exact Apple URL and ETag")),
	}); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil || !strings.Contains(output.String(), "PROGRESS_JSON:") {
		t.Fatalf("separate reader failed: %v %s", err, output.String())
	}
	if err := s.AdvanceAcquisitionProgress(ctx, id, AcquisitionProgressUpdate{
		Phase: "downloading", CompletedBytes: &downloaded,
	}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 70; i++ {
		phase := "verifying"
		if i%2 == 0 {
			phase = "downloading"
		}
		if err := s.AdvanceAcquisitionProgress(ctx, id, AcquisitionProgressUpdate{Phase: phase}); err != nil {
			t.Fatal(err)
		}
	}
	zero := int64(0)
	if err := s.AdvanceAcquisitionProgress(ctx, id, AcquisitionProgressUpdate{
		CompletedBytes: &zero,
	}); err == nil {
		t.Fatal("download counter regressed")
	}
	events, err := s.ProgressEvents(ctx, id)
	if err != nil || len(events) != progressHistoryLimit {
		t.Fatalf("history is not capped: %d %v", len(events), err)
	}
	last, err := s.Progress(ctx, id)
	if err != nil || last.CompletedBytes != downloaded ||
		last.Sequence <= progressHistoryLimit || last.Version <= initial.Version {
		t.Fatalf("committed counter or phase sequence changed: %+v %v", last, err)
	}
	if _, err := s.pool.Exec(ctx, `CREATE FUNCTION reject_progress_update() RETURNS trigger
	 LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected progress write failure'; END $$`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, `CREATE TRIGGER reject_progress_update BEFORE UPDATE
	 ON operation_progress FOR EACH ROW EXECUTE FUNCTION reject_progress_update()`); err != nil {
		t.Fatal(err)
	}
	full := int64(128)
	if err := s.AdvanceAcquisitionProgress(ctx, id, AcquisitionProgressUpdate{
		Phase: "complete", State: "complete", CompletedBytes: &full, VerifiedBytes: &full,
	}); err == nil {
		t.Fatal("injected database write failure was hidden")
	}
	visible, err := s.Progress(ctx, id)
	if err != nil || visible.State != "running" || visible.CompletedBytes != downloaded ||
		visible.Sequence != last.Sequence {
		t.Fatalf("failed write invented progress: %+v %v", visible, err)
	}
	if _, err := s.pool.Exec(ctx, `DROP TRIGGER reject_progress_update ON operation_progress`); err != nil {
		t.Fatal(err)
	}
	if err := s.AdvanceAcquisitionProgress(ctx, id, AcquisitionProgressUpdate{
		Phase: "complete", State: "complete", CompletedBytes: &full, VerifiedBytes: &full,
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.AdvanceAcquisitionProgress(ctx, id, AcquisitionProgressUpdate{
		Phase: "downloading",
	}); err == nil {
		t.Fatal("completed acquisition was rewritten")
	}
}

func TestInspectionProgressCommitsWithBatchesAndSurvivesCrash(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	run, _ := recoveryFixture(t, s, "progress-run", []string{"manifest", "tree"})
	reader, err := Open(ctx, s.pool.Config().ConnString())
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	first, err := reader.Progress(ctx, run.ID)
	if err != nil || first.InputSHA256 != run.InputSHA256 ||
		first.InputSize == nil || *first.InputSize != run.InputSize ||
		first.TotalUnits == nil || *first.TotalUnits != 2 {
		t.Fatalf("run identity and plan missing: %+v %v", first, err)
	}
	if err := s.InspectionPhase(ctx, run.ID, run.LeaseToken, "manifest"); err != nil {
		t.Fatal(err)
	}
	if err := s.PublishBatch(ctx, run, model.ObservationBatch{
		Component: "manifest", Final: &model.Coverage{Component: "manifest", State: "complete"},
	}); err != nil {
		t.Fatal(err)
	}
	p, err := reader.Progress(ctx, run.ID)
	if err != nil || p.CompletedUnits != 1 || p.PublishedBatches != 1 ||
		p.Phase != "checkpoint_published" {
		t.Fatalf("checkpoint progress not committed: %+v %v", p, err)
	}
	if _, err := s.pool.Exec(ctx, `UPDATE analysis_run SET lease_expires=now()-interval '1 second' WHERE id=$1`, run.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReconcileStale(ctx); err != nil {
		t.Fatal(err)
	}
	p, err = reader.Progress(ctx, run.ID)
	if err != nil || p.State != "interrupted" || p.CompletedUnits != 1 {
		t.Fatalf("crash reconciliation changed committed work: %+v %v", p, err)
	}
	if err := s.Resume(ctx, run.ID, "resumed-worker"); err != nil {
		t.Fatal(err)
	}
	run.LeaseToken = "resumed-worker"
	if _, err := s.pool.Exec(ctx, `CREATE FUNCTION reject_progress_update() RETURNS trigger
	 LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected progress write failure'; END $$`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, `CREATE TRIGGER reject_progress_update BEFORE UPDATE
	 ON operation_progress FOR EACH ROW EXECUTE FUNCTION reject_progress_update()`); err != nil {
		t.Fatal(err)
	}
	second := model.ObservationBatch{Component: "tree", Final: &model.Coverage{
		Component: "tree", State: "complete",
	}}
	if err := s.PublishBatch(ctx, run, second); err == nil {
		t.Fatal("injected progress write failure did not roll back the batch")
	}
	var count int
	if err := reader.pool.QueryRow(ctx, `SELECT count(*) FROM run_batch WHERE run_id=$1`, run.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("batch committed without progress: %d", count)
	}
	if _, err := s.pool.Exec(ctx, `DROP TRIGGER reject_progress_update ON operation_progress`); err != nil {
		t.Fatal(err)
	}
	if err := s.PublishBatch(ctx, run, second); err != nil {
		t.Fatal(err)
	}
	finished, err := s.Finish(ctx, run.ID, run.LeaseToken)
	if err != nil || finished.State != "complete" {
		t.Fatalf("resumed completion: %+v %v", finished, err)
	}
	p, err = reader.Progress(ctx, run.ID)
	if err != nil || p.State != "complete" || p.CompletedUnits != 2 ||
		p.PublishedBatches != 2 || p.InputSHA256 != run.InputSHA256 {
		t.Fatalf("committed terminal progress: %+v %v", p, err)
	}
}
