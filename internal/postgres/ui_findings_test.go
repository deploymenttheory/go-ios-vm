package postgres

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/go-ios-vm/internal/model"
)

func TestFindingBrowseAndRetainedEvidencePreview(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	artifact, target, first, second := assessmentFixture(t, s)
	archivePath := filepath.Join(t.TempDir(), "source.ipsw")
	if err := os.WriteFile(archivePath, []byte("test cache object"), 0o600); err != nil {
		t.Fatal(err)
	}
	artifact.Path = archivePath
	if err := s.Register(ctx, artifact, []model.Target{target}); err != nil {
		t.Fatal(err)
	}
	publishAssessmentObservation(t, s, target, first)
	publishAssessmentObservation(t, s, target, second)
	chains, err := s.FindingChains(ctx, target.ID, "")
	if err != nil || len(chains) != 1 || len(chains[0].Revisions) != 2 {
		t.Fatalf("fixture chain: %+v %v", chains, err)
	}
	initial := chains[0].Revisions[0]
	reviewAssessment(t, s, chains[0].ID, initial, 0)
	receipt, err := s.AddRevision(ctx, model.RevisionDraft{
		TargetID: target.ID, Subject: "device", Property: "register",
		Basis: "inferred", Presence: "present", Value: []byte(`0`),
		Rationale: "corrected reading", Author: "reviewer", Supersedes: initial.ID,
		References: []model.BasisReference{{Role: "supporting", ObservationID: first.ID},
			{Role: "contradicting", RevisionID: initial.ID}},
	})
	if err != nil {
		t.Fatal(err)
	}
	page, more, err := s.FindingPage(ctx, FindingFilter{
		TargetID: target.ID, Subject: "dev", Property: "reg", Basis: "inferred", Limit: 1,
	})
	if err != nil || more || len(page) != 1 || page[0].ID != receipt.FindingID ||
		page[0].RevisionCount != 3 {
		t.Fatalf("finding page: %+v more=%v err=%v", page, more, err)
	}
	wildcard, _, err := s.FindingPage(
		ctx,
		FindingFilter{TargetID: target.ID, Subject: "%", Limit: 10},
	)
	if err != nil || len(wildcard) != 0 {
		t.Fatalf("prefix filter treated wildcard as pattern: %+v %v", wildcard, err)
	}
	reviewPage, _, err := s.FindingPage(ctx, FindingFilter{
		TargetID: target.ID, Assessment: "needs_review", Limit: 10,
	})
	if err != nil || len(reviewPage) != 1 || reviewPage[0].ID != receipt.FindingID {
		t.Fatalf("latest assessment filter: %+v %v", reviewPage, err)
	}
	separateRevisions, _, err := s.FindingPage(ctx, FindingFilter{
		TargetID: target.ID, Basis: "inferred", Assessment: "needs_review", Limit: 10,
	})
	if err != nil || len(separateRevisions) != 0 {
		t.Fatalf("basis and assessment matched different revisions: %+v %v", separateRevisions, err)
	}
	chain, err := s.FindingForTarget(ctx, target.ID, receipt.FindingID)
	if err != nil || len(chain.Revisions) != 3 {
		t.Fatalf("finding detail: %+v %v", chain, err)
	}
	corrected := revisionByID(t, chain, receipt.RevisionID)
	if corrected.Supersedes != initial.ID || corrected.Assessment != "unreviewed" ||
		len(corrected.References) != 2 || corrected.References[0].Role != "contradicting" ||
		len(revisionByID(t, chain, initial.ID).AssessmentHistory) != 2 {
		t.Fatalf("revision chain lost assessment or basis: %+v", corrected)
	}
	if _, err := s.FindingForTarget(
		ctx,
		"another-target",
		receipt.FindingID,
	); !errors.Is(
		err,
		pgx.ErrNoRows,
	) {
		t.Fatalf("cross-target finding leaked: %v", err)
	}
	observations, more, err := s.ObservationPage(ctx, ObservationFilter{
		TargetID: target.ID, Subject: "dev", Property: "reg", Limit: 1,
	})
	if err != nil || !more || len(observations) != 1 {
		t.Fatalf("observation page: %+v more=%v err=%v", observations, more, err)
	}
	remaining, more, err := s.ObservationPage(ctx, ObservationFilter{
		TargetID: target.ID, AfterID: observations[0].ID, Limit: 1,
	})
	if err != nil || more || len(remaining) != 1 || remaining[0].ID == observations[0].ID {
		t.Fatalf("observation cursor: %+v more=%v err=%v", remaining, more, err)
	}
	if _, err := s.ObservationForTarget(
		ctx,
		"another-target",
		first.ID,
	); !errors.Is(
		err,
		pgx.ErrNoRows,
	) {
		t.Fatalf("cross-target observation leaked: %v", err)
	}
	if err := os.Remove(archivePath); err != nil {
		t.Fatal(err)
	}
	preview, err := s.PreviewEvidence(ctx, target.ID, first.EvidenceID, 4)
	if err != nil || preview.State != "complete" || !preview.DigestVerified ||
		preview.Hex != hex.EncodeToString(first.Evidence[:4]) || !preview.Truncated ||
		preview.DeclaredBytes != int64(len(first.Evidence)) {
		t.Fatalf("retained preview: %+v %v", preview, err)
	}
	if _, err := s.PreviewEvidence(
		ctx,
		"another-target",
		first.EvidenceID,
		4,
	); !errors.Is(
		err,
		pgx.ErrNoRows,
	) {
		t.Fatalf("cross-target evidence leaked: %v", err)
	}
	if _, err := s.PreviewEvidence(
		ctx,
		target.ID,
		first.EvidenceID,
		4097,
	); !errors.Is(
		err,
		ErrInvalidBrowseFilter,
	) {
		t.Fatalf("unbounded preview accepted: %v", err)
	}
	zero, err := model.NewObservation(
		target,
		"device",
		"empty-source",
		uint64(math.MaxUint64),
		model.Location{
			ArtifactID: first.Source.ArtifactID,
			Member:     "tree",
			Coordinate: "plist-selector",
			Selector:   "/empty",
		},
		model.NewBlob([]byte{}),
		"fixture/empty",
	)
	if err != nil {
		t.Fatal(err)
	}
	publishAssessmentObservation(t, s, target, zero)
	storedZero, err := s.ObservationForTarget(ctx, target.ID, zero.ID)
	if err != nil || string(storedZero.Value) != "18446744073709551615" {
		t.Fatalf("unsigned value lost precision: %s %v", storedZero.Value, err)
	}
	firstPage, more, err := s.FindingPage(ctx, FindingFilter{TargetID: target.ID, Limit: 1})
	if err != nil || !more || len(firstPage) != 1 {
		t.Fatalf("first finding page: %+v more=%v err=%v", firstPage, more, err)
	}
	nextPage, more, err := s.FindingPage(ctx, FindingFilter{
		TargetID: target.ID,
		Limit:    1,
		AfterID:  firstPage[0].ID,
	})
	if err != nil || more || len(nextPage) != 1 || nextPage[0].ID == firstPage[0].ID {
		t.Fatalf("next finding page: %+v more=%v err=%v", nextPage, more, err)
	}
	empty, err := s.PreviewEvidence(ctx, target.ID, zero.EvidenceID, 4)
	if err != nil || empty.State != "complete" || empty.DeclaredBytes != 0 ||
		empty.PreviewBytes != 0 || !empty.DigestVerified || empty.Truncated {
		t.Fatalf("empty evidence confused with unavailable: %+v %v", empty, err)
	}
	missing := model.Digest([]byte("unavailable source"))
	_, err = s.pool.Exec(
		ctx,
		`INSERT INTO evidence_blob(id,size,complete) VALUES($1,8,false)`,
		missing,
	)
	if err != nil {
		t.Fatal(err)
	}
	incomplete := first
	incomplete.ID = model.ID("incomplete", first.ID)
	incomplete.EvidenceID = missing
	encoded, err := json.Marshal(incomplete)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.pool.Exec(ctx, `INSERT INTO observation(id,target_id,evidence_id,document)
	 VALUES($1,$2,$3,$4)`, incomplete.ID, target.ID, missing, encoded)
	if err != nil {
		t.Fatal(err)
	}
	unavailable, err := s.PreviewEvidence(ctx, target.ID, missing, 4)
	if err != nil || unavailable.State != "unavailable" || unavailable.DigestVerified ||
		unavailable.DeclaredBytes != 8 || unavailable.PreviewBytes != 0 {
		t.Fatalf("incomplete evidence looked complete: %+v %v", unavailable, err)
	}
}
