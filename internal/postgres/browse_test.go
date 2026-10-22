package postgres

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/deploymenttheory/go-ios-vm/internal/model"
)

func browseFixtureTargets(t *testing.T, s *Store) (model.Artifact, []model.Target) {
	t.Helper()
	ctx := context.Background()
	a := model.Artifact{
		ID: strings.Repeat("a", 64), SHA256: strings.Repeat("a", 64),
		Size: 100, Path: "/missing/fixture-a.ipsw",
	}
	b := model.Artifact{
		ID: strings.Repeat("b", 64), SHA256: strings.Repeat("b", 64), Size: 200,
	}
	targets := []model.Target{
		{ID: strings.Repeat("1", 64), ArtifactID: a.ID, Identity: 0,
			Products: []string{"iPhone12,1"}, Board: "n104ap", Version: "14.0", Build: "18A373"},
		{ID: strings.Repeat("2", 64), ArtifactID: a.ID, Identity: 1,
			Products: []string{"iPhone12,1"}, Board: "n104ap", Version: "14.0", Build: "18A373"},
		{ID: strings.Repeat("3", 64), ArtifactID: b.ID, Identity: 0,
			Products: []string{"iPhone12,1"}, Board: "n104ap", Version: "15.0", Build: "19A346"},
	}
	if err := s.Register(ctx, a, targets[:2]); err != nil {
		t.Fatal(err)
	}
	if err := s.Register(ctx, b, targets[2:]); err != nil {
		t.Fatal(err)
	}
	return a, targets
}

func TestBrowseExactTargetsAndStableArtifactPages(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	a, targets := browseFixtureTargets(t, s)
	filter := TargetFilter{Product: "iPhone12,1", Build: "18A373", Page: PageOptions{Limit: 1}}
	first, err := s.BrowseTargets(ctx, filter)
	if err != nil || len(first.Items) != 1 || first.Items[0].Target.ID != targets[0].ID ||
		first.NextCursor != targets[0].ID {
		t.Fatalf("first target page: %+v %v", first, err)
	}
	if first.Items[0].Artifact.ID != a.ID || first.Items[0].Artifact.SourceAvailability != "registered_unverified" {
		t.Fatalf("target provenance: %+v", first.Items[0])
	}
	// A newly inserted row before the cursor must not duplicate the first page.
	newTarget := model.Target{ID: strings.Repeat("0", 64), ArtifactID: a.ID,
		Identity: 2, Products: []string{"iPhone12,1"}, Build: "18A373"}
	if err := s.Register(ctx, a, []model.Target{newTarget}); err != nil {
		t.Fatal(err)
	}
	filter.Page.Cursor = first.NextCursor
	second, err := s.BrowseTargets(ctx, filter)
	if err != nil || len(second.Items) != 1 || second.Items[0].Target.ID != targets[1].ID {
		t.Fatalf("second target page: %+v %v", second, err)
	}
	identity := 1
	exact, err := s.BrowseTargets(ctx, TargetFilter{
		Product: "iPhone12,1", Build: "18A373", Identity: &identity,
	})
	if err != nil || len(exact.Items) != 1 || exact.Items[0].Target.ID != targets[1].ID {
		t.Fatalf("exact identity: %+v %v", exact, err)
	}
	other, err := s.BrowseTargets(ctx, TargetFilter{Product: "iPhone12,1", Build: "19A346"})
	if err != nil || len(other.Items) != 1 || other.Items[0].Target.ID != targets[2].ID {
		t.Fatalf("other build: %+v %v", other, err)
	}
	detail, err := s.BrowseTarget(ctx, targets[1].ID)
	if err != nil || detail.Target.Identity != 1 || detail.Artifact.ID != a.ID {
		t.Fatalf("target detail: %+v %v", detail, err)
	}
	artifacts, err := s.BrowseArtifacts(ctx, "", PageOptions{Limit: 1})
	if err != nil || len(artifacts.Items) != 1 || artifacts.Items[0].ID != a.ID {
		t.Fatalf("first artifact page: %+v %v", artifacts, err)
	}
	newArtifact := model.Artifact{ID: strings.Repeat("0", 64), SHA256: strings.Repeat("0", 64), Size: 1}
	if err := s.Register(ctx, newArtifact, nil); err != nil {
		t.Fatal(err)
	}
	secondArtifact, err := s.BrowseArtifacts(ctx, "", PageOptions{Limit: 1, Cursor: artifacts.NextCursor})
	if err != nil || len(secondArtifact.Items) != 1 || secondArtifact.Items[0].ID != strings.Repeat("b", 64) {
		t.Fatalf("second artifact page: %+v %v", secondArtifact, err)
	}
}

func TestBrowseAcquisitionEvidenceAndStablePages(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	request := map[string]any{"selection": map[string]string{
		"product": "iPhone12,1", "version": "14.0", "build": "18A373",
	}}
	if err := s.Acquisition(ctx, "https://catalog.example/old", request, "failed", []byte("failed source")); err != nil {
		t.Fatal(err)
	}
	if err := s.Acquisition(ctx, "https://catalog.example/new", request, "verified", []byte("verified source")); err != nil {
		t.Fatal(err)
	}
	filter := AcquisitionFilter{Product: "iPhone12,1", Build: "18A373", Page: PageOptions{Limit: 1}}
	first, err := s.BrowseAcquisitions(ctx, filter)
	if err != nil || len(first.Items) != 1 || first.Items[0].Status != "verified" ||
		!first.Items[0].EvidenceAvailable || first.NextCursor == "" {
		t.Fatalf("first acquisition page: %+v %v", first, err)
	}
	if err := s.Acquisition(ctx, "https://catalog.example/latest", request, "verified", []byte("new source")); err != nil {
		t.Fatal(err)
	}
	filter.Page.Cursor = first.NextCursor
	second, err := s.BrowseAcquisitions(ctx, filter)
	if err != nil || len(second.Items) != 1 || second.Items[0].Status != "failed" ||
		second.Items[0].ID >= first.Items[0].ID {
		t.Fatalf("second acquisition page: %+v %v", second, err)
	}
	failed, err := s.BrowseAcquisitions(ctx, AcquisitionFilter{
		Product: "iPhone12,1", Build: "18A373", Status: "failed",
	})
	if err != nil || len(failed.Items) != 1 || failed.Items[0].ID != second.Items[0].ID {
		t.Fatalf("failed acquisition filter: %+v %v", failed, err)
	}
	detail, err := s.BrowseAcquisition(ctx, second.Items[0].ID)
	if err != nil || detail.EvidenceID == "" || detail.Product != "iPhone12,1" {
		t.Fatalf("acquisition detail: %+v %v", detail, err)
	}
	future := time.Now().UTC().Add(time.Hour)
	none, err := s.BrowseAcquisitions(ctx, AcquisitionFilter{From: &future})
	if err != nil || len(none.Items) != 0 {
		t.Fatalf("time filter: %+v %v", none, err)
	}
}

func TestBrowseRunsKeepsCheckpointsSeparateFromFinalCoverage(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	a, targets := browseFixtureTargets(t, s)
	base := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	states := []string{"failed", "complete", "partial", "running"}
	for i, state := range states {
		id := "browse-" + state
		started := base.Add(time.Duration(i) * time.Minute)
		var finished *time.Time
		if state != "running" {
			finish := started.Add(time.Second)
			finished = &finish
		}
		_, err := s.pool.Exec(ctx, `INSERT INTO analysis_run
 (id,target_id,state,started,finished,coverage,input_sha256,input_size,extractor,planned_components)
 VALUES($1,$2,$3,$4,$5,'[]',$6,$7,'fixture/v1','["manifest","disk-metadata"]')`,
			id, targets[0].ID, state, started, finished, a.SHA256, a.Size)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.pool.Exec(ctx, `INSERT INTO run_batch(run_id,component,ordinal,digest)
 VALUES($1,'manifest',0,$2)`, id, strings.Repeat("f", 64)); err != nil {
			t.Fatal(err)
		}
		if _, err := s.pool.Exec(ctx, `INSERT INTO run_checkpoint(run_id,component,state,reason)
 VALUES($1,'manifest','complete','')`, id); err != nil {
			t.Fatal(err)
		}
		if state == "partial" || state == "complete" {
			coverageState, reason := "complete", ""
			if state == "partial" {
				coverageState, reason = "partial", "disk codec unsupported"
			}
			if _, err := s.pool.Exec(ctx, `INSERT INTO run_coverage(run_id,component,state,reason)
 VALUES($1,'manifest','complete',''),($1,'disk-metadata',$2,$3)`, id, coverageState, reason); err != nil {
				t.Fatal(err)
			}
		}
	}
	page, err := s.BrowseRuns(ctx, RunFilter{State: "running", Page: PageOptions{Limit: 1}})
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != "browse-running" {
		t.Fatalf("running filter: %+v %v", page, err)
	}
	for _, state := range states {
		record, err := s.BrowseRun(ctx, "browse-"+state)
		if err != nil || record.State != state || len(record.CommittedCheckpoints) != 1 ||
			len(record.CommittedBatches) != 1 || record.CommittedBatches[0].Count != 1 {
			t.Fatalf("run %s detail: %+v %v", state, record, err)
		}
		final := state == "partial" || state == "complete"
		if record.FinalCoverageAvailable != final || (len(record.FinalCoverage) == 2) != final {
			t.Fatalf("run %s final coverage: %+v", state, record)
		}
		if state == "partial" && record.FinalCoverage[1].Reason != "disk codec unsupported" {
			t.Fatalf("partial reason lost: %+v", record.FinalCoverage)
		}
	}
	all, err := s.BrowseRuns(ctx, RunFilter{Page: PageOptions{Limit: 2}})
	if err != nil || len(all.Items) != 2 || all.NextCursor == "" {
		t.Fatalf("first run page: %+v %v", all, err)
	}
	if _, err := s.pool.Exec(ctx, `INSERT INTO analysis_run
 (id,target_id,state,started,coverage,input_sha256,input_size,extractor,planned_components)
 VALUES('browse-new',$1,'running',$2,'[]',$3,$4,'fixture/v1','[]')`,
		targets[0].ID, time.Now().UTC(), a.SHA256, a.Size); err != nil {
		t.Fatal(err)
	}
	older, err := s.BrowseRuns(ctx, RunFilter{Page: PageOptions{Limit: 2, Cursor: all.NextCursor}})
	if err != nil || len(older.Items) != 2 || older.Items[0].ID == all.Items[0].ID ||
		older.Items[1].ID == all.Items[1].ID {
		t.Fatalf("stable run cursor: %+v %v", older, err)
	}
}
