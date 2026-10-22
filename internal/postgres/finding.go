package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/go-ios-vm/internal/model"
)

var errBasisReferenceMissing = errors.New("finding basis has no reference")

func publishObservedRevision(ctx context.Context, tx pgx.Tx, o model.Observation) error {
	findingID, err := getOrCreateFinding(ctx, tx, o.TargetID, o.Subject, o.Property)
	if err != nil {
		return fmt.Errorf("finding: %w", err)
	}
	revisionID := model.ID("finding", o.ID)
	tag, err := tx.Exec(ctx,
		`INSERT INTO finding_revision(id,finding_id,observation_id,basis,rationale,presence,value)
		 VALUES($1,$2,$3,'observed','Direct extraction; not a claim of emulated behavior',$4,$5)
		 ON CONFLICT DO NOTHING`,
		revisionID, findingID, o.ID, o.Presence, o.Value)
	if err != nil {
		return fmt.Errorf("revision: %w", err)
	}
	_, err = tx.Exec(ctx,
		`INSERT INTO finding_basis(revision_id,role,observation_id)
		 VALUES($1,'supporting',$2) ON CONFLICT DO NOTHING`, revisionID, o.ID)
	if err != nil {
		return fmt.Errorf("basis: %w", err)
	}
	if tag.RowsAffected() == 1 {
		seeds, err := conflictingObserved(ctx, tx, findingID, revisionID)
		if err != nil {
			return err
		}
		if err := invalidateAssessments(ctx, tx, revisionID, seeds); err != nil {
			return err
		}
	}
	return nil
}

func getOrCreateFinding(
	ctx context.Context,
	tx pgx.Tx,
	target, subject, property string,
) (string, error) {
	_, err := tx.Exec(
		ctx,
		"INSERT INTO finding(target_id,subject,property) VALUES($1,$2,$3) ON CONFLICT(target_id,subject,property) DO NOTHING",
		target,
		subject,
		property,
	)
	if err != nil {
		return "", fmt.Errorf("getOrCreateFinding: insert: %w", err)
	}
	var id string
	err = tx.QueryRow(ctx,
		"SELECT id FROM finding WHERE target_id=$1 AND subject=$2 AND property=$3",
		target, subject, property).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("getOrCreateFinding: select: %w", err)
	}
	return id, nil
}

// AddRevision publishes a linked interpretation atomically. Its references must
// already exist as retained evidence, and the database validates their target.
func (s *Store) AddRevision(
	ctx context.Context,
	draft model.RevisionDraft,
) (model.RevisionReceipt, error) {
	if err := draft.Validate(); err != nil {
		return model.RevisionReceipt{}, fmt.Errorf("AddRevision: Validate: %w", err)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return model.RevisionReceipt{}, fmt.Errorf("AddRevision: Begin: %w", err)
	}
	defer rollback(ctx, tx)
	id, err := getOrCreateFinding(ctx, tx, draft.TargetID, draft.Subject, draft.Property)
	if err != nil {
		return model.RevisionReceipt{}, fmt.Errorf("AddRevision: finding: %w", err)
	}
	var supersedes *string
	if draft.Supersedes != "" {
		supersedes = &draft.Supersedes
	}
	presence := draft.Presence
	if draft.Basis == "unknown" {
		presence = "unexamined"
	}
	var value *string
	if len(draft.Value) > 0 {
		encoded := string(draft.Value)
		value = &encoded
	}
	var revisionID string
	err = tx.QueryRow(
		ctx,
		`INSERT INTO finding_revision(id,finding_id,observation_id,basis,rationale,presence,value,author,supersedes)
		 VALUES(gen_random_uuid()::text,$1,NULLIF($2,''),$3,$4,$5,$6::jsonb,$7,$8) RETURNING id`,
		id,
		draft.ObservationID,
		draft.Basis,
		draft.Rationale,
		presence,
		value,
		draft.Author,
		supersedes,
	).Scan(&revisionID)
	if err != nil {
		return model.RevisionReceipt{}, fmt.Errorf("AddRevision: revision: %w", err)
	}
	refs := draft.References
	if draft.Basis == "observed" {
		refs = append(
			[]model.BasisReference{{Role: "supporting", ObservationID: draft.ObservationID}},
			refs...)
	}
	for _, ref := range refs {
		var observationID, referenceRevisionID *string
		if ref.ObservationID != "" {
			observationID = &ref.ObservationID
		} else {
			referenceRevisionID = &ref.RevisionID
		}
		_, err = tx.Exec(ctx,
			`INSERT INTO finding_basis(revision_id,role,observation_id,reference_revision_id)
			 VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`,
			revisionID, ref.Role, observationID, referenceRevisionID)
		if err != nil {
			return model.RevisionReceipt{}, fmt.Errorf("AddRevision: reference: %w", err)
		}
	}
	seeds := []string{}
	if draft.Supersedes != "" {
		seeds = append(seeds, draft.Supersedes)
	}
	for _, ref := range refs {
		if ref.Role == "contradicting" && ref.RevisionID != "" {
			seeds = append(seeds, ref.RevisionID)
		}
	}
	if draft.Basis == "observed" {
		conflicts, err := conflictingObserved(ctx, tx, id, revisionID)
		if err != nil {
			return model.RevisionReceipt{}, fmt.Errorf("AddRevision: conflicts: %w", err)
		}
		seeds = append(seeds, conflicts...)
	}
	if err := invalidateAssessments(ctx, tx, revisionID, seeds); err != nil {
		return model.RevisionReceipt{}, fmt.Errorf("AddRevision: assessments: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return model.RevisionReceipt{}, fmt.Errorf("AddRevision: Commit: %w", err)
	}
	return model.RevisionReceipt{FindingID: id, RevisionID: revisionID}, nil
}

func (s *Store) FindingChain(ctx context.Context, id string) (model.FindingChain, error) {
	chains, err := s.findingChains(ctx, "", "", id)
	if err != nil {
		return model.FindingChain{}, err
	}
	if len(chains) == 0 {
		return model.FindingChain{}, fmt.Errorf("FindingChain: %w", pgx.ErrNoRows)
	}
	return chains[0], nil
}

func (s *Store) FindingChains(
	ctx context.Context,
	target, run string,
) ([]model.FindingChain, error) {
	return s.findingChains(ctx, target, run, "")
}

func (s *Store) findingChains(
	ctx context.Context,
	target, run, findingID string,
) ([]model.FindingChain, error) {
	rows, err := s.pool.Query(ctx, `SELECT f.id,f.target_id,f.subject,f.property FROM finding f
	 WHERE ($1='' OR f.target_id=$1) AND ($2='' OR f.id=$2)
	 AND ($3='' OR EXISTS (SELECT 1 FROM finding_revision r JOIN run_observation ro
	   ON ro.observation_id=r.observation_id WHERE r.finding_id=f.id AND ro.run_id=$3))
	 ORDER BY f.target_id,f.subject,f.property`, target, findingID, run)
	if err != nil {
		return nil, fmt.Errorf("FindingChains: findings: %w", err)
	}
	chains := []model.FindingChain{}
	ids := []string{}
	for rows.Next() {
		var f model.FindingChain
		if err = rows.Scan(&f.ID, &f.TargetID, &f.Subject, &f.Property); err != nil {
			rows.Close()
			return nil, fmt.Errorf("FindingChains: Scan: %w", err)
		}
		f.Revisions = []model.FindingRevision{}
		chains = append(chains, f)
		ids = append(ids, f.ID)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, fmt.Errorf("FindingChains: Err: %w", err)
	}
	if len(ids) == 0 {
		return chains, nil
	}
	indices := make(map[string]int, len(chains))
	for i := range chains {
		indices[chains[i].ID] = i
	}
	rows, err = s.pool.Query(
		ctx,
		`SELECT r.id,r.finding_id,r.basis,r.presence,r.value,r.rationale,r.author,
	 r.created,r.supersedes,o.document
	 FROM finding_revision r LEFT JOIN observation o ON o.id=r.observation_id
	 WHERE r.finding_id=ANY($1) ORDER BY r.created,r.id`,
		ids,
	)
	if err != nil {
		return nil, fmt.Errorf("FindingChains: revisions: %w", err)
	}
	revisionIndex := map[string][2]int{}
	for rows.Next() {
		var r model.FindingRevision
		var fid string
		var value, observation []byte
		var supersedes *string
		if err = rows.Scan(&r.ID, &fid, &r.Basis, &r.Presence, &value, &r.Rationale,
			&r.Author, &r.Created, &supersedes, &observation); err != nil {
			rows.Close()
			return nil, fmt.Errorf("FindingChains: revision Scan: %w", err)
		}
		if supersedes != nil {
			r.Supersedes = *supersedes
		}
		r.Value = value
		if observation != nil {
			var o model.Observation
			if err = json.Unmarshal(observation, &o); err != nil {
				rows.Close()
				return nil, fmt.Errorf("FindingChains: observation: %w", err)
			}
			r.Observation = &o
		}
		r.References = []model.BasisReference{}
		r.Assessment = "unreviewed"
		r.AssessmentHistory = []model.AssessmentEvent{}
		i := indices[fid]
		j := len(chains[i].Revisions)
		chains[i].Revisions = append(chains[i].Revisions, r)
		revisionIndex[r.ID] = [2]int{i, j}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, fmt.Errorf("FindingChains: revision Err: %w", err)
	}
	revisionIDs := make([]string, 0, len(revisionIndex))
	for id := range revisionIndex {
		revisionIDs = append(revisionIDs, id)
	}
	rows, err = s.pool.Query(
		ctx,
		`SELECT b.revision_id,b.role,b.observation_id,b.reference_revision_id,o.document
	 FROM finding_basis b LEFT JOIN observation o ON o.id=b.observation_id
	 WHERE b.revision_id=ANY($1) ORDER BY b.revision_id,b.role,b.observation_id,b.reference_revision_id`,
		revisionIDs,
	)
	if err != nil {
		return nil, fmt.Errorf("FindingChains: basis: %w", err)
	}
	for rows.Next() {
		var id, role string
		var oid, rid *string
		var document []byte
		if err = rows.Scan(&id, &role, &oid, &rid, &document); err != nil {
			rows.Close()
			return nil, fmt.Errorf("FindingChains: basis Scan: %w", err)
		}
		ref := model.BasisReference{Role: role}
		switch {
		case oid != nil:
			ref.ObservationID = *oid
			var o model.Observation
			if err = json.Unmarshal(document, &o); err != nil {
				rows.Close()
				return nil, fmt.Errorf("FindingChains: basis observation: %w", err)
			}
			ref.Observation = &o
		case rid != nil:
			ref.RevisionID = *rid
		default:
			rows.Close()
			return nil, errBasisReferenceMissing
		}
		at := revisionIndex[id]
		chains[at[0]].Revisions[at[1]].References = append(
			chains[at[0]].Revisions[at[1]].References,
			ref,
		)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, fmt.Errorf("FindingChains: basis Err: %w", err)
	}
	rows, err = s.pool.Query(ctx, `SELECT id,finding_id,revision_id,basis,state,reviewer,
	 reason,previous_id,cause_revision_id,created FROM finding_assessment_event
	 WHERE revision_id=ANY($1) ORDER BY revision_id,id`, revisionIDs)
	if err != nil {
		return nil, fmt.Errorf("FindingChains: assessments: %w", err)
	}
	for rows.Next() {
		var event model.AssessmentEvent
		var previous *int64
		var cause *string
		if err := rows.Scan(&event.ID, &event.FindingID, &event.RevisionID, &event.Basis,
			&event.State, &event.Reviewer, &event.Reason, &previous, &cause,
			&event.Created); err != nil {
			rows.Close()
			return nil, fmt.Errorf("FindingChains: assessment Scan: %w", err)
		}
		if previous != nil {
			event.PreviousID = *previous
		}
		if cause != nil {
			event.CauseRevisionID = *cause
		}
		at := revisionIndex[event.RevisionID]
		r := &chains[at[0]].Revisions[at[1]]
		r.AssessmentHistory = append(r.AssessmentHistory, event)
		r.Assessment = event.State
		r.AssessmentID = event.ID
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, fmt.Errorf("FindingChains: assessment Err: %w", err)
	}
	return chains, nil
}
