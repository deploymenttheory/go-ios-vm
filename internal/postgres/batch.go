package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/go-ios-vm/internal/model"
)

// Resume reopens an interrupted run. Its caller must first verify the complete
// archive SHA-256 and match it against the persisted run input identity.
func (s *Store) Resume(ctx context.Context, id string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("Resume: Begin: %w", err)
	}
	defer rollback(ctx, tx)
	var state string
	if err := tx.QueryRow(ctx, "SELECT state FROM analysis_run WHERE id=$1 FOR UPDATE", id).
		Scan(&state); err != nil {
		return fmt.Errorf("Resume: run: %w", err)
	}
	if state == "complete" || state == "partial" {
		return errResumeUnavailable
	}
	if _, err := tx.Exec(
		ctx,
		"UPDATE analysis_run SET state='running',finished=NULL WHERE id=$1",
		id,
	); err != nil {
		return fmt.Errorf("Resume: update: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("Resume: Commit: %w", err)
	}
	return nil
}

func (s *Store) Checkpoints(ctx context.Context, id string) (map[string]model.Coverage, error) {
	rows, err := s.pool.Query(ctx,
		"SELECT component,state,reason FROM run_checkpoint WHERE run_id=$1", id)
	if err != nil {
		return nil, fmt.Errorf("Checkpoints: Query: %w", err)
	}
	defer rows.Close()
	out := map[string]model.Coverage{}
	for rows.Next() {
		var c model.Coverage
		if err := rows.Scan(&c.Component, &c.State, &c.Reason); err != nil {
			return nil, fmt.Errorf("Checkpoints: Scan: %w", err)
		}
		out[c.Component] = c
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("Checkpoints: Err: %w", err)
	}
	return out, nil
}

// PublishBatch commits at most one caller-bounded slice of observations. The
// key and digest make a retry safe after an ambiguous response. A component
// checkpoint is committed with its last batch, never before its observations.
func (s *Store) PublishBatch(
	ctx context.Context,
	run model.Run,
	batch model.ObservationBatch,
) error {
	if batch.Component == "" || batch.Ordinal < 0 ||
		(batch.Final != nil && batch.Final.Component != batch.Component) {
		return errConflictingBatch
	}
	if len(batch.Observations) > model.MaxObservationsPerBatch {
		return errBatchExceedsBound
	}
	encoded, err := json.Marshal(batch)
	if err != nil {
		return fmt.Errorf("PublishBatch: Marshal: %w", err)
	}
	digest := model.Digest(encoded)
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("PublishBatch: Begin: %w", err)
	}
	defer rollback(ctx, tx)
	var state, targetID, artifactID string
	var planJSON []byte
	if err := tx.QueryRow(ctx, `SELECT r.state,r.target_id,t.artifact_id,r.planned_components FROM analysis_run r
	 JOIN firmware_target t ON t.id=r.target_id WHERE r.id=$1 FOR UPDATE OF r`, run.ID).
		Scan(&state, &targetID, &artifactID, &planJSON); err != nil {
		return fmt.Errorf("PublishBatch: run: %w", err)
	}
	if state != "running" {
		return fmt.Errorf("%w: %s", errRunIsAlreadyTerminal, state)
	}
	if targetID != run.TargetID {
		return errRunTargetMismatch
	}
	var plan []string
	if err := json.Unmarshal(planJSON, &plan); err != nil {
		return fmt.Errorf("PublishBatch: plan: %w", err)
	}
	if !slices.Contains(plan, batch.Component) {
		return errIncompleteBatchPlan
	}
	var prior string
	err = tx.QueryRow(ctx, `SELECT digest FROM run_batch WHERE run_id=$1 AND component=$2 AND ordinal=$3`,
		run.ID, batch.Component, batch.Ordinal).
		Scan(&prior)
	if err == nil {
		if prior != digest {
			return errConflictingBatch
		}
		return nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("PublishBatch: prior: %w", err)
	}
	var hasCheckpoint bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM run_checkpoint WHERE run_id=$1 AND component=$2)`,
		run.ID, batch.Component).
		Scan(&hasCheckpoint); err != nil {
		return fmt.Errorf("PublishBatch: checkpoint: %w", err)
	}
	if hasCheckpoint {
		return errConflictingBatch
	}
	if batch.Ordinal > 0 {
		var previous bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM run_batch WHERE run_id=$1 AND component=$2 AND ordinal=$3)`,
			run.ID, batch.Component, batch.Ordinal-1).
			Scan(&previous); err != nil {
			return fmt.Errorf("PublishBatch: previous: %w", err)
		}
		if !previous {
			return errIncompleteBatchPlan
		}
	}
	if err := s.putObservations(ctx, tx, run, artifactID, batch.Observations); err != nil {
		return err
	}
	if _, err := tx.Exec(
		ctx,
		`INSERT INTO run_batch(run_id,component,ordinal,digest) VALUES($1,$2,$3,$4)`,
		run.ID,
		batch.Component,
		batch.Ordinal,
		digest,
	); err != nil {
		return fmt.Errorf("PublishBatch: batch: %w", err)
	}
	if batch.Final != nil {
		if _, err := tx.Exec(ctx, `INSERT INTO run_checkpoint(run_id,component,state,reason)
		 VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING`, run.ID, batch.Component,
			batch.Final.State, batch.Final.Reason); err != nil {
			return fmt.Errorf("PublishBatch: checkpoint: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("PublishBatch: Commit: %w", err)
	}
	return nil
}

// Finish writes the authoritative coverage ledger only when every planned
// component has a committed checkpoint. Incomplete runs have no final ledger.
func (s *Store) Finish(ctx context.Context, id string) (model.Run, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return model.Run{}, fmt.Errorf("Finish: Begin: %w", err)
	}
	defer rollback(ctx, tx)
	var state string
	var planJSON []byte
	if err := tx.QueryRow(ctx, `SELECT state,planned_components FROM analysis_run WHERE id=$1 FOR UPDATE`, id).
		Scan(&state, &planJSON); err != nil {
		return model.Run{}, fmt.Errorf("Finish: run: %w", err)
	}
	if state != "running" {
		return model.Run{}, fmt.Errorf("%w: %s", errRunIsAlreadyTerminal, state)
	}
	var plan []string
	if err := json.Unmarshal(planJSON, &plan); err != nil {
		return model.Run{}, fmt.Errorf("Finish: plan: %w", err)
	}
	if len(plan) == 0 {
		return model.Run{}, errIncompleteBatchPlan
	}
	rows, err := tx.Query(
		ctx,
		`SELECT component,state,reason FROM run_checkpoint WHERE run_id=$1`,
		id,
	)
	if err != nil {
		return model.Run{}, fmt.Errorf("Finish: checkpoints: %w", err)
	}
	checkpoints := map[string]model.Coverage{}
	for rows.Next() {
		var c model.Coverage
		if err := rows.Scan(&c.Component, &c.State, &c.Reason); err != nil {
			rows.Close()
			return model.Run{}, fmt.Errorf("Finish: Scan: %w", err)
		}
		checkpoints[c.Component] = c
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return model.Run{}, fmt.Errorf("Finish: Err: %w", err)
	}
	if len(checkpoints) != len(plan) {
		return model.Run{}, errIncompleteBatchPlan
	}
	coverage := make([]model.Coverage, 0, len(plan))
	finalState := "complete"
	for _, component := range plan {
		c, ok := checkpoints[component]
		if !ok {
			return model.Run{}, errIncompleteBatchPlan
		}
		if c.State != "complete" {
			finalState = "partial"
		}
		coverage = append(coverage, c)
		if _, err := tx.Exec(ctx, `INSERT INTO run_coverage(run_id,component,state,reason)
		 VALUES($1,$2,$3,$4)`, id, c.Component, c.State, c.Reason); err != nil {
			return model.Run{}, fmt.Errorf("Finish: ledger: %w", err)
		}
	}
	encoded, err := json.Marshal(coverage)
	if err != nil {
		return model.Run{}, fmt.Errorf("Finish: Marshal: %w", err)
	}
	if _, err := tx.Exec(
		ctx,
		`UPDATE analysis_run SET state=$2,finished=now(),coverage=$3 WHERE id=$1`,
		id,
		finalState,
		encoded,
	); err != nil {
		return model.Run{}, fmt.Errorf("Finish: update: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return model.Run{}, fmt.Errorf("Finish: Commit: %w", err)
	}
	return s.Run(ctx, id)
}
