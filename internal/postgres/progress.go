package postgres

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/go-ios-vm/internal/model"
)

var (
	ErrProgressIdentity         = errors.New("progress identity or total changed")
	errProgressRunStateMismatch = errors.New("progress and run state disagree")
)

const progressHistoryLimit = 64

type AcquisitionProgressUpdate struct {
	Phase             string
	State             string
	CompletedBytes    *int64
	VerifiedBytes     *int64
	TotalBytes        *int64
	SourceFingerprint string
	DiagnosticCode    string
}

type progressUpdate struct {
	phase, state, component, fingerprint, diagnostic string
	bytes, verified, totalBytes, units, totalUnits   *int64
	batches                                          int64
}

// StartAcquisitionProgress commits a safe request before catalog resolution.
// The returned ID identifies this attempt even if selection or transfer fails.
func (s *Store) StartAcquisitionProgress(
	ctx context.Context,
	product, version, build string,
) (string, error) {
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", fmt.Errorf("StartAcquisitionProgress: nonce: %w", err)
	}
	id := hex.EncodeToString(nonce[:])
	request, err := json.Marshal(map[string]string{
		"product": product, "version": version, "build": build,
	})
	if err != nil {
		return "", fmt.Errorf("StartAcquisitionProgress: request: %w", err)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("StartAcquisitionProgress: Begin: %w", err)
	}
	defer rollback(ctx, tx)
	_, err = tx.Exec(ctx, `INSERT INTO operation_progress(id,kind,request,phase,state)
	 VALUES($1,'acquisition',$2,'resolving','running')`, id, request)
	if err != nil {
		return "", fmt.Errorf("StartAcquisitionProgress: insert: %w", err)
	}
	if err := recordProgressEvent(ctx, tx, id, 1, "resolving", "running", "", ""); err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE operation_progress SET sequence=1 WHERE id=$1`, id); err != nil {
		return "", fmt.Errorf("StartAcquisitionProgress: sequence: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("StartAcquisitionProgress: Commit: %w", err)
	}
	return id, nil
}

func (s *Store) AdvanceAcquisitionProgress(
	ctx context.Context,
	id string,
	u AcquisitionProgressUpdate,
) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("AdvanceAcquisitionProgress: Begin: %w", err)
	}
	defer rollback(ctx, tx)
	if err := advanceProgress(ctx, tx, id, progressUpdate{
		phase: u.Phase, state: u.State, bytes: u.CompletedBytes,
		verified:   u.VerifiedBytes,
		totalBytes: u.TotalBytes, fingerprint: u.SourceFingerprint,
		diagnostic: u.DiagnosticCode,
	}); err != nil {
		return fmt.Errorf("AdvanceAcquisitionProgress: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("AdvanceAcquisitionProgress: Commit: %w", err)
	}
	return nil
}

func advanceProgress(ctx context.Context, tx pgx.Tx, id string, u progressUpdate) error {
	var p model.Progress
	var totalBytes, totalUnits *int64
	var fingerprint *string
	err := tx.QueryRow(ctx, `SELECT kind,phase,state,component,completed_bytes,verified_bytes,total_bytes,
	 completed_units,total_units,published_batches,source_fingerprint,diagnostic_code,sequence
	 FROM operation_progress WHERE id=$1 FOR UPDATE`, id).
		Scan(
			&p.Kind, &p.Phase, &p.State, &p.Component, &p.CompletedBytes, &p.VerifiedBytes, &totalBytes,
			&p.CompletedUnits, &totalUnits, &p.PublishedBatches,
			&fingerprint, &p.DiagnosticCode, &p.Sequence,
		)
	if err != nil {
		return fmt.Errorf("advanceProgress: read: %w", err)
	}
	if fingerprint != nil {
		p.SourceFingerprint = *fingerprint
	}
	changedPhase, err := mergeProgress(&p, &totalBytes, &totalUnits, u)
	if err != nil {
		return err
	}
	var finished *time.Time
	if p.State != "running" {
		now := time.Now().UTC()
		finished = &now
	}
	_, err = tx.Exec(ctx, `UPDATE operation_progress SET phase=$2,state=$3,component=$4,
	 completed_bytes=$5,verified_bytes=$6,total_bytes=$7,completed_units=$8,total_units=$9,
	 published_batches=$10,source_fingerprint=NULLIF($11,''),diagnostic_code=$12,
	 sequence=$13,version=version+1,updated=now(),finished=$14 WHERE id=$1`,
		id, p.Phase, p.State, p.Component, p.CompletedBytes, p.VerifiedBytes,
		totalBytes, p.CompletedUnits, totalUnits, p.PublishedBatches,
		p.SourceFingerprint, p.DiagnosticCode, p.Sequence, finished)
	if err != nil {
		return fmt.Errorf("advanceProgress: update: %w", err)
	}
	if changedPhase {
		return recordProgressEvent(ctx, tx, id, p.Sequence, p.Phase, p.State,
			p.Component, p.DiagnosticCode)
	}
	return nil
}

func mergeProgress(
	p *model.Progress,
	totalBytes, totalUnits **int64,
	u progressUpdate,
) (bool, error) {
	priorPhase, priorState, priorComponent, priorDiagnostic := p.Phase, p.State, p.Component, p.DiagnosticCode
	if priorState != "running" && (p.Kind == "acquisition" || u.state != "running") {
		return false, ErrProgressIdentity
	}
	if u.phase != "" {
		p.Phase = u.phase
	}
	if u.state != "" {
		p.State = u.state
		if u.state == "running" && priorState != "running" {
			p.DiagnosticCode = ""
		}
	}
	if u.component != "" {
		p.Component = u.component
	}
	if u.diagnostic != "" {
		p.DiagnosticCode = u.diagnostic
	}
	if u.fingerprint != "" {
		if p.SourceFingerprint != "" && p.SourceFingerprint != u.fingerprint {
			return false, ErrProgressIdentity
		}
		p.SourceFingerprint = u.fingerprint
	}
	if err := mergeProgressCounters(p, totalBytes, totalUnits, u); err != nil {
		return false, err
	}
	if p.Kind == "acquisition" && p.State == "complete" &&
		(*totalBytes == nil || p.CompletedBytes != **totalBytes || p.VerifiedBytes != **totalBytes) {
		return false, ErrProgressIdentity
	}
	changedPhase := p.Phase != priorPhase || p.State != priorState ||
		p.Component != priorComponent || p.DiagnosticCode != priorDiagnostic
	if changedPhase {
		p.Sequence++
	}
	return changedPhase, nil
}

func mergeProgressCounters(
	p *model.Progress,
	totalBytes, totalUnits **int64,
	u progressUpdate,
) error {
	if u.totalBytes != nil {
		if *totalBytes != nil && **totalBytes != *u.totalBytes {
			return ErrProgressIdentity
		}
		*totalBytes = u.totalBytes
	}
	if u.totalUnits != nil {
		if *totalUnits != nil && **totalUnits != *u.totalUnits {
			return ErrProgressIdentity
		}
		*totalUnits = u.totalUnits
	}
	if u.bytes != nil {
		if *u.bytes < p.CompletedBytes {
			return ErrProgressIdentity
		}
		p.CompletedBytes = *u.bytes
	}
	if u.verified != nil {
		if *u.verified < p.VerifiedBytes {
			return ErrProgressIdentity
		}
		p.VerifiedBytes = *u.verified
	}
	if u.units != nil {
		if *u.units < p.CompletedUnits {
			return ErrProgressIdentity
		}
		p.CompletedUnits = *u.units
	}
	if u.batches < 0 {
		return ErrProgressIdentity
	}
	p.PublishedBatches += u.batches
	if (*totalBytes != nil && (p.CompletedBytes > **totalBytes || p.VerifiedBytes > **totalBytes)) ||
		(*totalUnits != nil && p.CompletedUnits > **totalUnits) {
		return ErrProgressIdentity
	}
	return nil
}

func recordProgressEvent(
	ctx context.Context,
	tx pgx.Tx,
	id string,
	sequence int64,
	phase, state, component, diagnostic string,
) error {
	_, err := tx.Exec(ctx, `INSERT INTO operation_progress_event
	 (operation_id,sequence,phase,state,component,diagnostic_code)
	 VALUES($1,$2,$3,$4,$5,$6)`, id, sequence, phase, state, component, diagnostic)
	if err != nil {
		return fmt.Errorf("recordProgressEvent: insert: %w", err)
	}
	_, err = tx.Exec(ctx, `DELETE FROM operation_progress_event
	 WHERE operation_id=$1 AND sequence<=$2`, id, sequence-progressHistoryLimit)
	if err != nil {
		return fmt.Errorf("recordProgressEvent: trim: %w", err)
	}
	return nil
}

func (s *Store) Progress(ctx context.Context, id string) (model.Progress, error) {
	return progressFrom(ctx, s.pool, id)
}

type progressQuerier interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func progressFrom(ctx context.Context, q progressQuerier, id string) (model.Progress, error) {
	var p model.Progress
	var runID, inputSHA, fingerprint *string
	err := q.QueryRow(ctx, `SELECT id,kind,run_id,input_sha256,input_size,
	 source_fingerprint,phase,state,component,completed_bytes,verified_bytes,total_bytes,
	 completed_units,total_units,published_batches,diagnostic_code,sequence,version,
	 started,updated,finished FROM operation_progress WHERE id=$1`, id).Scan(
		&p.ID, &p.Kind, &runID, &inputSHA, &p.InputSize, &fingerprint,
		&p.Phase, &p.State, &p.Component, &p.CompletedBytes, &p.VerifiedBytes, &p.TotalBytes,
		&p.CompletedUnits, &p.TotalUnits, &p.PublishedBatches,
		&p.DiagnosticCode, &p.Sequence, &p.Version, &p.Started, &p.Updated, &p.Finished,
	)
	if err != nil {
		return model.Progress{}, fmt.Errorf("Progress: Query: %w", err)
	}
	if runID != nil {
		p.RunID = *runID
	}
	if inputSHA != nil {
		p.InputSHA256 = *inputSHA
	}
	if fingerprint != nil {
		p.SourceFingerprint = *fingerprint
	}
	return p, nil
}

func (s *Store) ProgressEvents(ctx context.Context, id string) ([]model.ProgressEvent, error) {
	return progressEventsFrom(ctx, s.pool, id)
}

func progressEventsFrom(
	ctx context.Context,
	q progressQuerier,
	id string,
) ([]model.ProgressEvent, error) {
	rows, err := q.Query(ctx, `SELECT operation_id,sequence,phase,state,component,
	 diagnostic_code,created FROM operation_progress_event WHERE operation_id=$1
	 ORDER BY sequence LIMIT 65`, id)
	if err != nil {
		return nil, fmt.Errorf("ProgressEvents: Query: %w", err)
	}
	defer rows.Close()
	out := []model.ProgressEvent{}
	for rows.Next() {
		var event model.ProgressEvent
		if err := rows.Scan(&event.OperationID, &event.Sequence, &event.Phase,
			&event.State, &event.Component, &event.DiagnosticCode, &event.Created); err != nil {
			return nil, fmt.Errorf("ProgressEvents: Scan: %w", err)
		}
		out = append(out, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ProgressEvents: Err: %w", err)
	}
	if len(out) > progressHistoryLimit {
		return nil, ErrBrowseTooLarge
	}
	return out, nil
}

// ProgressStatus reads the durable snapshot, retained events, run ledger and
// lease within one read-only database snapshot. A stale lease is reported, not
// reconciled: only the worker/recovery path may change a run's state.
type ProgressStatus struct {
	Progress       model.Progress
	Events         []model.ProgressEvent
	Run            *RunRecord
	Stale          bool
	EffectiveState string
}

func (s *Store) ReadProgressStatus(ctx context.Context, id string) (ProgressStatus, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return ProgressStatus{}, fmt.Errorf("ReadProgressStatus: BeginTx: %w", err)
	}
	defer rollback(ctx, tx)
	p, err := progressFrom(ctx, tx, id)
	if err != nil {
		return ProgressStatus{}, err
	}
	events, err := progressEventsFrom(ctx, tx, id)
	if err != nil {
		return ProgressStatus{}, err
	}
	status := ProgressStatus{
		Progress: p, Events: events, EffectiveState: p.State,
	}
	if p.Kind == "inspection" {
		run, runErr := browseRunTx(ctx, tx, id)
		if runErr != nil {
			return ProgressStatus{}, runErr
		}
		if run.State != p.State {
			return ProgressStatus{}, errProgressRunStateMismatch
		}
		status.Run = &run
		if p.State == "running" {
			if err := tx.QueryRow(ctx, `SELECT lease_expires IS NULL OR lease_expires<=now()
			 FROM analysis_run WHERE id=$1`, id).Scan(&status.Stale); err != nil {
				return ProgressStatus{}, fmt.Errorf("ReadProgressStatus: lease: %w", err)
			}
			if status.Stale {
				status.EffectiveState = "stale"
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return ProgressStatus{}, fmt.Errorf("ReadProgressStatus: Commit: %w", err)
	}
	return status, nil
}

// InspectionPhase reports the component currently being decoded. Published
// counts are advanced only by the batch transaction that retains evidence.
func (s *Store) InspectionPhase(ctx context.Context, id, token, component string) error {
	if component == "" {
		return ErrProgressIdentity
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("InspectionPhase: Begin: %w", err)
	}
	defer rollback(ctx, tx)
	var owner string
	err = tx.QueryRow(ctx, `SELECT COALESCE(lease_token,'') FROM analysis_run
	 WHERE id=$1 AND state='running' FOR UPDATE`, id).Scan(&owner)
	if err != nil || owner != token {
		return errRunLeaseUnavailable
	}
	if err := advanceProgress(ctx, tx, id, progressUpdate{
		phase: "component_inspection", component: component,
	}); err != nil {
		return fmt.Errorf("InspectionPhase: progress: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("InspectionPhase: Commit: %w", err)
	}
	return nil
}
