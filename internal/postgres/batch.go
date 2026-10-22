package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/go-ios-vm/internal/model"
)

// Resume claims a failed, cancelled, interrupted, or expired run. Its caller
// must first verify the complete archive against the persisted input identity.
func (s *Store) Resume(ctx context.Context, id, token string) error {
	if token == "" {
		return errRunLeaseUnavailable
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("Resume: Begin: %w", err)
	}
	defer rollback(ctx, tx)
	var state string
	var expired bool
	if err := tx.QueryRow(ctx, "SELECT state,lease_expires IS NULL OR lease_expires<now() FROM analysis_run WHERE id=$1 FOR UPDATE", id).
		Scan(&state, &expired); err != nil {
		return fmt.Errorf("Resume: run: %w", err)
	}
	if state == "complete" || state == "partial" {
		return errResumeUnavailable
	}
	if state == "running" && !expired {
		return errRunLeaseUnavailable
	}
	if _, err := tx.Exec(
		ctx,
		`UPDATE analysis_run SET state='running',finished=NULL,lease_token=$2,
		 lease_expires=now()+interval '2 minutes' WHERE id=$1`,
		id, token,
	); err != nil {
		return fmt.Errorf("Resume: update: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("Resume: Commit: %w", err)
	}
	return nil
}

// Heartbeat renews only the current claim. A former worker cannot extend a
// lease after another process has reconciled and resumed the run.
func (s *Store) Heartbeat(ctx context.Context, id, token string) error {
	if token == "" {
		return errRunLeaseUnavailable
	}
	tag, err := s.pool.Exec(ctx, `UPDATE analysis_run SET lease_expires=now()+interval '2 minutes'
	 WHERE id=$1 AND lease_token=$2 AND state='running'`, id, token)
	if err != nil {
		return fmt.Errorf("Heartbeat: Exec: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return errRunLeaseUnavailable
	}
	return nil
}

// ReconcileStale marks expired running claims as interrupted. Completed
// batches and input identity are retained for a later verified resume.
func (s *Store) ReconcileStale(ctx context.Context) ([]model.Run, error) {
	_, err := s.pool.Exec(
		ctx,
		`UPDATE analysis_run SET state='interrupted',lease_token=NULL,lease_expires=NULL
		 WHERE state='running' AND lease_expires<now()`,
	)
	if err != nil {
		return nil, fmt.Errorf("ReconcileStale: update: %w", err)
	}
	rows, err := s.pool.Query(ctx, `SELECT id FROM analysis_run
	 WHERE state IN ('interrupted','failed','cancelled') ORDER BY started DESC`)
	if err != nil {
		return nil, fmt.Errorf("ReconcileStale: Query: %w", err)
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, fmt.Errorf("ReconcileStale: Scan: %w", err)
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, fmt.Errorf("ReconcileStale: Err: %w", err)
	}
	runs := make([]model.Run, 0, len(ids))
	for _, id := range ids {
		run, err := s.Run(ctx, id)
		if err != nil {
			return runs, fmt.Errorf("ReconcileStale: Run: %w", err)
		}
		if run.State == "interrupted" || run.State == "failed" || run.State == "cancelled" {
			runs = append(runs, run)
		}
	}
	return runs, nil
}

// FailBatched records a genuine parser failure or cancellation only for the
// worker that still owns the run. It never overwrites a completed result.
func (s *Store) FailBatched(ctx context.Context, id, state, token string) error {
	if state != "failed" && state != "cancelled" {
		return errInvalidPublicationState
	}
	tag, err := s.pool.Exec(ctx, `UPDATE analysis_run
	 SET state=$3,finished=now(),lease_token=NULL,lease_expires=NULL
	 WHERE id=$1 AND lease_token=$2 AND state='running'`, id, token, state)
	if err != nil {
		return fmt.Errorf("FailBatched: Exec: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return errRunLeaseUnavailable
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
	var state, targetID, artifactID, leaseToken string
	var planJSON []byte
	if err := tx.QueryRow(ctx, `SELECT r.state,r.target_id,t.artifact_id,r.planned_components,COALESCE(r.lease_token,'') FROM analysis_run r
	 JOIN firmware_target t ON t.id=r.target_id WHERE r.id=$1 FOR UPDATE OF r`, run.ID).
		Scan(&state, &targetID, &artifactID, &planJSON, &leaseToken); err != nil {
		return fmt.Errorf("PublishBatch: run: %w", err)
	}
	if state != "running" {
		return fmt.Errorf("%w: %s", errRunIsAlreadyTerminal, state)
	}
	if leaseToken != run.LeaseToken {
		return errRunLeaseUnavailable
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
func (s *Store) Finish(ctx context.Context, id, token string) (model.Run, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return model.Run{}, fmt.Errorf("Finish: Begin: %w", err)
	}
	defer rollback(ctx, tx)
	var state, leaseToken string
	var planJSON []byte
	if err := tx.QueryRow(ctx, `SELECT state,planned_components,COALESCE(lease_token,'') FROM analysis_run WHERE id=$1 FOR UPDATE`, id).
		Scan(&state, &planJSON, &leaseToken); err != nil {
		return model.Run{}, fmt.Errorf("Finish: run: %w", err)
	}
	if state == "complete" || state == "partial" {
		if err := tx.Commit(ctx); err != nil {
			return model.Run{}, fmt.Errorf("Finish: terminal Commit: %w", err)
		}
		readCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		return s.Run(readCtx, id)
	}
	if state != "running" {
		return model.Run{}, fmt.Errorf("%w: %s", errRunIsAlreadyTerminal, state)
	}
	if leaseToken != token {
		return model.Run{}, errRunLeaseUnavailable
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
		`UPDATE analysis_run SET state=$2,finished=now(),coverage=$3,lease_token=NULL,lease_expires=NULL WHERE id=$1`,
		id,
		finalState,
		encoded,
	); err != nil {
		return model.Run{}, fmt.Errorf("Finish: update: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		// A lost commit response is ambiguous. Re-read on a fresh connection;
		// a committed terminal run is the authoritative result.
		readCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if committed, readErr := s.Run(readCtx, id); readErr == nil &&
			(committed.State == "complete" || committed.State == "partial") {
			return committed, nil
		}
		return model.Run{}, fmt.Errorf("Finish: Commit: %w", err)
	}
	readCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	return s.Run(readCtx, id)
}
