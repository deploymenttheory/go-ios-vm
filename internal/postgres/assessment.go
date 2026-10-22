package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/go-ios-vm/internal/model"
)

var (
	ErrAssessmentConflict = errors.New(
		"assessment changed; reload the finding and use its current assessment ID",
	)
	ErrAssessmentOwnership = errors.New("revision does not belong to the stated finding and basis")
)

// ReviewRevision records a decision against one exact revision and basis.
// ExpectedID is zero for the first decision; every later decision names the
// latest event so concurrent reviewers cannot silently replace one another.
func (s *Store) ReviewRevision(
	ctx context.Context,
	draft model.AssessmentDraft,
) (model.AssessmentEvent, error) {
	if err := draft.Validate(); err != nil {
		return model.AssessmentEvent{}, fmt.Errorf("ReviewRevision: Validate: %w", err)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return model.AssessmentEvent{}, fmt.Errorf("ReviewRevision: Begin: %w", err)
	}
	defer rollback(ctx, tx)
	var id string
	err = tx.QueryRow(ctx, `SELECT id FROM finding_revision
	 WHERE id=$1 AND finding_id=$2 AND basis=$3 FOR UPDATE`,
		draft.RevisionID, draft.FindingID, draft.Basis).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.AssessmentEvent{}, ErrAssessmentOwnership
	}
	if err != nil {
		return model.AssessmentEvent{}, fmt.Errorf("ReviewRevision: owner: %w", err)
	}
	var latest int64
	err = tx.QueryRow(ctx, `SELECT id FROM finding_assessment_event
	 WHERE revision_id=$1 ORDER BY id DESC LIMIT 1`, id).Scan(&latest)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return model.AssessmentEvent{}, fmt.Errorf("ReviewRevision: latest: %w", err)
	}
	if latest != draft.ExpectedID {
		return model.AssessmentEvent{}, ErrAssessmentConflict
	}
	var previous any
	if latest != 0 {
		previous = latest
	}
	event := model.AssessmentEvent{
		FindingID: draft.FindingID, RevisionID: id, Basis: draft.Basis,
		State: draft.State, Reviewer: draft.Reviewer, Reason: draft.Reason,
		PreviousID: latest,
	}
	err = tx.QueryRow(ctx, `INSERT INTO finding_assessment_event
	 (finding_id,revision_id,basis,state,reviewer,reason,previous_id)
	 VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id,created`,
		event.FindingID, event.RevisionID, event.Basis, event.State,
		event.Reviewer, event.Reason, previous).Scan(&event.ID, &event.Created)
	if err != nil {
		return model.AssessmentEvent{}, fmt.Errorf("ReviewRevision: insert: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return model.AssessmentEvent{}, fmt.Errorf("ReviewRevision: Commit: %w", err)
	}
	return event, nil
}

// invalidateAssessments follows supporting revision dependencies. The caller
// supplies changed revision IDs and the new revision that caused the change.
// Only reviewed states get a needs_review event; previous decisions stay in
// the append-only history.
func invalidateAssessments(ctx context.Context, tx pgx.Tx, causeID string, seeds []string) error {
	if len(seeds) == 0 {
		return nil
	}
	rows, err := tx.Query(ctx, `WITH RECURSIVE affected(id) AS (
	 SELECT unnest($1::text[])
	 UNION
	 SELECT b.revision_id FROM finding_basis b JOIN affected a ON true
	  JOIN finding_revision source ON source.id=a.id
	 WHERE b.role='supporting' AND
	  (b.reference_revision_id=a.id OR b.observation_id=source.observation_id)
	)
	SELECT r.id,r.finding_id,r.basis FROM finding_revision r
	 WHERE r.id IN (SELECT id FROM affected) ORDER BY r.id FOR UPDATE OF r`, seeds)
	if err != nil {
		return fmt.Errorf("invalidateAssessments: dependents: %w", err)
	}
	type owner struct{ revision, finding, basis string }
	owners := []owner{}
	for rows.Next() {
		var o owner
		if err := rows.Scan(&o.revision, &o.finding, &o.basis); err != nil {
			rows.Close()
			return fmt.Errorf("invalidateAssessments: owner: %w", err)
		}
		owners = append(owners, o)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return fmt.Errorf("invalidateAssessments: rows: %w", err)
	}
	for _, o := range owners {
		var latest int64
		var state string
		err := tx.QueryRow(ctx, `SELECT id,state FROM finding_assessment_event
		 WHERE revision_id=$1 ORDER BY id DESC LIMIT 1`, o.revision).Scan(&latest, &state)
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && state == "needs_review") {
			continue
		}
		if err != nil {
			return fmt.Errorf("invalidateAssessments: latest: %w", err)
		}
		_, err = tx.Exec(ctx, `INSERT INTO finding_assessment_event
		 (finding_id,revision_id,basis,state,reviewer,reason,previous_id,cause_revision_id)
		 VALUES($1,$2,$3,'needs_review','system',$4,$5,$6)`, o.finding,
			o.revision, o.basis, "Support or interpretation changed with revision "+causeID,
			latest, causeID)
		if err != nil {
			return fmt.Errorf("invalidateAssessments: insert: %w", err)
		}
	}
	return nil
}

func conflictingObserved(
	ctx context.Context,
	tx pgx.Tx,
	findingID, newID string,
) ([]string, error) {
	rows, err := tx.Query(ctx, `SELECT old.id FROM finding_revision old
	 JOIN finding_revision newer ON newer.id=$2
	 WHERE old.finding_id=$1 AND old.id<>newer.id AND old.basis='observed'
	 AND (old.presence IS DISTINCT FROM newer.presence OR old.value IS DISTINCT FROM newer.value)`,
		findingID, newID)
	if err != nil {
		return nil, fmt.Errorf("conflictingObserved: Query: %w", err)
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("conflictingObserved: Scan: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("conflictingObserved: Err: %w", err)
	}
	return ids, nil
}
