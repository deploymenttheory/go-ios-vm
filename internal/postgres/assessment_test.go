package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/deploymenttheory/go-ios-vm/internal/model"
)

func assessmentFixture(t *testing.T, s *Store) (model.Artifact, model.Target, model.Observation, model.Observation) {
	t.Helper()
	ctx := context.Background()
	data := []byte("assessment fixture IPSW")
	artifact := model.Artifact{ID: model.Digest(data), SHA256: model.Digest(data), Size: int64(len(data))}
	target := model.Target{ID: model.ID(artifact.ID, "0"), ArtifactID: artifact.ID}
	if err := s.Register(ctx, artifact, []model.Target{target}); err != nil {
		t.Fatal(err)
	}
	makeObservation := func(value int, parser string) model.Observation {
		t.Helper()
		o, err := model.NewObservation(target, "device", "register", value,
			model.Location{ArtifactID: artifact.ID, Member: "tree", Coordinate: "plist-selector", Selector: "/device/register"},
			model.NewBlob([]byte(parser+" source bytes")), parser)
		if err != nil {
			t.Fatal(err)
		}
		return o
	}
	return artifact, target, makeObservation(1, "parser/v1"), makeObservation(2, "parser/v2")
}

func publishAssessmentObservation(t *testing.T, s *Store, target model.Target, o model.Observation) {
	t.Helper()
	ctx := context.Background()
	run := model.Run{ID: model.ID("assessment-run", o.ID), TargetID: target.ID,
		State: "complete", Started: time.Now(), Coverage: []model.Coverage{{Component: "tree", State: "complete"}}}
	if err := s.Start(ctx, run); err != nil {
		t.Fatal(err)
	}
	if err := s.Publish(ctx, run, []model.Observation{o}); err != nil {
		t.Fatal(err)
	}
}

func reviewAssessment(t *testing.T, s *Store, findingID string, revision model.FindingRevision, expected int64) model.AssessmentEvent {
	t.Helper()
	event, err := s.ReviewRevision(context.Background(), model.AssessmentDraft{
		FindingID: findingID, RevisionID: revision.ID, Basis: revision.Basis,
		State: "accepted", Reviewer: "fixture-reviewer", Reason: "source and interpretation checked",
		ExpectedID: expected,
	})
	if err != nil {
		t.Fatal(err)
	}
	return event
}

func revisionByID(t *testing.T, chain model.FindingChain, id string) model.FindingRevision {
	t.Helper()
	for _, r := range chain.Revisions {
		if r.ID == id {
			return r
		}
	}
	t.Fatalf("revision %s missing", id)
	return model.FindingRevision{}
}

func TestExactAssessmentOwnershipAndConcurrentReview(t *testing.T) {
	s := testStore(t)
	_, target, first, second := assessmentFixture(t, s)
	publishAssessmentObservation(t, s, target, first)
	chain, err := s.FindingChains(context.Background(), target.ID, "")
	if err != nil || len(chain) != 1 || len(chain[0].Revisions) != 1 {
		t.Fatalf("fixture finding: %+v %v", chain, err)
	}
	finding := chain[0]
	revision := finding.Revisions[0]
	draft := model.AssessmentDraft{FindingID: finding.ID, RevisionID: revision.ID,
		Basis: revision.Basis, State: "accepted", Reviewer: "reviewer", Reason: "verified source", ExpectedID: 0}
	const reviewers = 8
	var wg sync.WaitGroup
	results := make(chan error, reviewers)
	for range reviewers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := s.ReviewRevision(context.Background(), draft)
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	successes, conflicts := 0, 0
	for err := range results {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrAssessmentConflict):
			conflicts++
		default:
			t.Fatalf("unexpected concurrent result: %v", err)
		}
	}
	if successes != 1 || conflicts != reviewers-1 {
		t.Fatalf("concurrent decisions: successes=%d conflicts=%d", successes, conflicts)
	}
	draft.FindingID = "other-finding"
	draft.ExpectedID = 1
	if _, err := s.ReviewRevision(context.Background(), draft); !errors.Is(err, ErrAssessmentOwnership) {
		t.Fatalf("wrong finding was assessed: %v", err)
	}
	draft.FindingID = finding.ID
	draft.Basis = "inferred"
	if _, err := s.ReviewRevision(context.Background(), draft); !errors.Is(err, ErrAssessmentOwnership) {
		t.Fatalf("wrong basis was assessed: %v", err)
	}
	publishAssessmentObservation(t, s, target, second)
	stored, err := s.FindingChain(context.Background(), finding.ID)
	if err != nil {
		t.Fatal(err)
	}
	secondRevision := model.ID("finding", second.ID)
	if got := revisionByID(t, stored, secondRevision); got.Assessment != "unreviewed" {
		t.Fatalf("contradictory reading inherited assessment: %+v", got)
	}
	firstRevision := revisionByID(t, stored, revision.ID)
	if firstRevision.Assessment != "needs_review" || len(firstRevision.AssessmentHistory) != 2 ||
		firstRevision.AssessmentHistory[0].State != "accepted" {
		t.Fatalf("prior decision was not retained and invalidated: %+v", firstRevision)
	}
	if _, err := s.pool.Exec(context.Background(), `UPDATE finding_assessment_event SET reason='rewritten' WHERE id=$1`,
		firstRevision.AssessmentID); err == nil {
		t.Fatal("assessment history was mutable")
	}
}

func TestAssessmentDecisionHistoryAndExplicitContradiction(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	_, target, first, _ := assessmentFixture(t, s)
	publishAssessmentObservation(t, s, target, first)
	chains, err := s.FindingChains(ctx, target.ID, "")
	if err != nil || len(chains) != 1 || len(chains[0].Revisions) != 1 {
		t.Fatalf("fixture finding: %+v %v", chains, err)
	}
	finding := chains[0]
	revision := finding.Revisions[0]
	draft := model.AssessmentDraft{FindingID: finding.ID, RevisionID: revision.ID,
		Basis: revision.Basis, Reviewer: "reviewer", Reason: "initial review", State: "accepted"}
	states := []string{"accepted", "rejected", "disputed"}
	for _, state := range states {
		draft.State = state
		event, err := s.ReviewRevision(ctx, draft)
		if err != nil {
			t.Fatalf("record %s: %v", state, err)
		}
		if event.PreviousID != draft.ExpectedID {
			t.Fatalf("broken event chain: %+v", event)
		}
		draft.ExpectedID = event.ID
	}
	current, err := s.FindingChain(ctx, finding.ID)
	if err != nil {
		t.Fatal(err)
	}
	reviewed := revisionByID(t, current, revision.ID)
	if reviewed.Assessment != "disputed" || len(reviewed.AssessmentHistory) != len(states) {
		t.Fatalf("decision history missing: %+v", reviewed)
	}
	for i, state := range states {
		if reviewed.AssessmentHistory[i].State != state {
			t.Fatalf("decision %d changed: %+v", i, reviewed.AssessmentHistory[i])
		}
	}
	receipt, err := s.AddRevision(ctx, model.RevisionDraft{
		TargetID: target.ID, Subject: "device", Property: "register", Basis: "unknown",
		Rationale: "a contradictory interpretation was recorded", Author: "fixture",
		References: []model.BasisReference{{Role: "contradicting", RevisionID: revision.ID}},
	})
	if err != nil {
		t.Fatal(err)
	}
	current, err = s.FindingChain(ctx, finding.ID)
	if err != nil {
		t.Fatal(err)
	}
	prior := revisionByID(t, current, revision.ID)
	if prior.Assessment != "needs_review" || len(prior.AssessmentHistory) != 4 ||
		prior.AssessmentHistory[3].CauseRevisionID != receipt.RevisionID {
		t.Fatalf("explicit contradiction did not invalidate review: %+v", prior)
	}
	if successor := revisionByID(t, current, receipt.RevisionID); successor.Assessment != "unreviewed" {
		t.Fatalf("contradictory interpretation inherited review: %+v", successor)
	}
}

func TestChangedSupportInvalidatesDependentsAndBackupRestoresHistory(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	_, target, first, second := assessmentFixture(t, s)
	publishAssessmentObservation(t, s, target, first)
	chains, err := s.FindingChains(ctx, target.ID, "")
	if err != nil || len(chains) != 1 {
		t.Fatalf("fixture finding: %+v %v", chains, err)
	}
	finding := chains[0]
	observed := finding.Revisions[0]
	byRevision, err := s.AddRevision(ctx, model.RevisionDraft{
		TargetID: target.ID, Subject: "device", Property: "register", Basis: "inferred",
		Presence: "present", Value: json.RawMessage(`{"u64":"1"}`),
		Rationale: "derived from the reviewed parser reading", Author: "fixture",
		References: []model.BasisReference{{Role: "supporting", RevisionID: observed.ID}},
	})
	if err != nil {
		t.Fatal(err)
	}
	byObservation, err := s.AddRevision(ctx, model.RevisionDraft{
		TargetID: target.ID, Subject: "device", Property: "register", Basis: "inferred",
		Presence: "present", Value: json.RawMessage(`{"u64":"1"}`),
		Rationale: "derived directly from the retained source", Author: "fixture",
		References: []model.BasisReference{{Role: "supporting", ObservationID: first.ID}},
	})
	if err != nil {
		t.Fatal(err)
	}
	finding, err = s.FindingChain(ctx, finding.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{observed.ID, byRevision.RevisionID, byObservation.RevisionID} {
		reviewAssessment(t, s, finding.ID, revisionByID(t, finding, id), 0)
	}
	publishAssessmentObservation(t, s, target, second)
	finding, err = s.FindingChain(ctx, finding.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{observed.ID, byRevision.RevisionID, byObservation.RevisionID} {
		r := revisionByID(t, finding, id)
		if r.Assessment != "needs_review" || len(r.AssessmentHistory) != 2 ||
			r.AssessmentHistory[0].State != "accepted" {
			t.Fatalf("changed support did not invalidate %s: %+v", id, r)
		}
	}
	if newReading := revisionByID(t, finding, model.ID("finding", second.ID)); newReading.Assessment != "unreviewed" {
		t.Fatalf("new reading inherited review: %+v", newReading)
	}
	dependent := revisionByID(t, finding, byRevision.RevisionID)
	reviewAssessment(t, s, finding.ID, dependent, dependent.AssessmentID)
	successor, err := s.AddRevision(ctx, model.RevisionDraft{
		TargetID: target.ID, Subject: "device", Property: "register", Basis: "unknown",
		Rationale: "support is under review", Author: "fixture", Supersedes: dependent.ID,
		References: []model.BasisReference{{Role: "contradicting", RevisionID: dependent.ID}},
	})
	if err != nil {
		t.Fatal(err)
	}
	finding, err = s.FindingChain(ctx, finding.ID)
	if err != nil {
		t.Fatal(err)
	}
	dependent = revisionByID(t, finding, dependent.ID)
	if dependent.Assessment != "needs_review" || len(dependent.AssessmentHistory) != 4 {
		t.Fatalf("supersede did not invalidate re-reviewed revision: %+v", dependent)
	}
	if got := revisionByID(t, finding, successor.RevisionID); got.Assessment != "unreviewed" {
		t.Fatalf("successor inherited assessment: %+v", got)
	}
	t.Run("backup restore", func(t *testing.T) {
		if os.Getenv("GOIOSVM_TEST_DOCKER_BACKUP") != "1" {
			t.Skip("set GOIOSVM_TEST_DOCKER_BACKUP=1 for Docker pg_dump/pg_restore integration")
		}
		verifyRevisionBackupRestore(t, s, finding, first.EvidenceID)
	})
}
