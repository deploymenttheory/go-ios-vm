// Package postgres owns transactional evidence publication.
package postgres

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/tern/v2/migrate"

	"github.com/deploymenttheory/go-ios-vm/internal/model"
)

var (
	errDatabaseURLRequired = errors.New(
		"database URL is required (GOIOSVM_DATABASE_URL)",
	)
	errInvalidPostgreSQLConfiguration      = errors.New("invalid PostgreSQL configuration")
	errEvidenceObjectExceeds128MiB         = errors.New("evidence object exceeds 128 MiB")
	errRetainedEvidenceByteBudgetExhausted = errors.New(
		"retained evidence byte budget exhausted",
	)
	errRunTargetMismatch                = errors.New("run target mismatch")
	errInvalidPublicationState          = errors.New("invalid publication state")
	errCoverageLedgerIsRequired         = errors.New("coverage ledger is required")
	errCompleteRunHasIncompleteCoverage = errors.New(
		"complete run has incomplete coverage",
	)
	errRunIsAlreadyTerminal        = errors.New("run is already terminal")
	errObservationArtifactMismatch = errors.New(
		"observation source belongs to a different artifact",
	)
	errObservationTargetMismatch = errors.New(
		"observation belongs to different target",
	)
	errConflictingEvidence = errors.New(
		"conflicting bytes for evidence identity",
	)
	errUnreadableEvidence = errors.New(
		"evidence incomplete or exceeds read limit",
	)
	errStoredEvidenceIntegrityMismatch = errors.New(
		"stored evidence integrity mismatch",
	)
)

//go:embed migrations/*.sql
var migrations embed.FS

type Store struct {
	pool          *pgxpool.Pool
	EvidenceLimit int64
}

func Open(ctx context.Context, dsn string) (*Store, error) {
	if dsn == "" {
		return nil, errDatabaseURLRequired
	}
	c, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, errInvalidPostgreSQLConfiguration
	}
	c.MaxConns = 4
	p, err := pgxpool.NewWithConfig(ctx, c)
	if err != nil {
		return nil, fmt.Errorf("create PostgreSQL pool: %w", err)
	}
	if err = p.Ping(ctx); err != nil {
		p.Close()
		return nil, fmt.Errorf("connect PostgreSQL: %w", err)
	}
	return &Store{pool: p, EvidenceLimit: 2 << 30}, nil
}
func (s *Store) Close() { s.pool.Close() }
func (s *Store) Migrate(ctx context.Context) error {
	c, err := s.pool.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("Migrate: Acquire: %w", err)
	}
	defer c.Release()
	m, err := migrate.NewMigrator(ctx, c.Conn(), "schema_version")
	if err != nil {
		return fmt.Errorf("Migrate: migrate.NewMigrator: %w", err)
	}
	sub, err := fs.Sub(migrations, "migrations")
	if err != nil {
		return fmt.Errorf("Migrate: fs.Sub: %w", err)
	}
	if err = m.LoadMigrations(sub); err != nil {
		return fmt.Errorf("Migrate: LoadMigrations: %w", err)
	}
	if err := m.Migrate(ctx); err != nil {
		return fmt.Errorf("Migrate: Migrate: %w", err)
	}
	return nil
}

// rollback must still release the transaction after its caller is cancelled.
func rollback(ctx context.Context, tx pgx.Tx) {
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	// Commit may have closed the transaction; pgx closes broken connections on failure.
	_ = tx.Rollback(cleanup)
}

func (s *Store) Register(ctx context.Context, a model.Artifact, targets []model.Target) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("Register: Begin: %w", err)
	}
	defer rollback(ctx, tx)
	if _, err = tx.Exec(
		ctx,
		"INSERT INTO artifact VALUES($1,$2,$3) ON CONFLICT DO NOTHING",
		a.ID,
		a.SHA256,
		a.Size,
	); err != nil {
		return fmt.Errorf("Register: Exec: %w", err)
	}
	if a.Path != "" {
		if _, err = tx.Exec(
			ctx,
			"INSERT INTO artifact_location VALUES($1,$2) ON CONFLICT DO NOTHING",
			a.ID,
			a.Path,
		); err != nil {
			return fmt.Errorf("Register: Exec: %w", err)
		}
	}
	for _, t := range targets {
		b, e := json.Marshal(t)
		if e != nil {
			return fmt.Errorf("Register: json.Marshal: %w", e)
		}
		if _, err = tx.Exec(
			ctx,
			"INSERT INTO firmware_target VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING",
			t.ID,
			a.ID,
			t.Identity,
			b,
		); err != nil {
			return fmt.Errorf("Register: Exec: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("Register: Commit: %w", err)
	}
	return nil
}

func (s *Store) Start(ctx context.Context, r model.Run) error {
	_, err := s.pool.Exec(
		ctx,
		"INSERT INTO analysis_run(id,target_id,state,started) VALUES($1,$2,'running',$3)",
		r.ID,
		r.TargetID,
		r.Started,
	)
	if err != nil {
		return fmt.Errorf("Start: Exec: %w", err)
	}
	return nil
}

func putBlob(ctx context.Context, tx pgx.Tx, data []byte, limit int64) (string, error) {
	if len(data) > 128<<20 {
		return "", errEvidenceObjectExceeds128MiB
	}
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(719230113)"); err != nil {
		return "", fmt.Errorf("putBlob: Exec: %w", err)
	}
	id := model.Digest(data)
	var complete bool
	if err := tx.QueryRow(ctx, "SELECT complete FROM evidence_blob WHERE id=$1", id).
		Scan(&complete); err == nil &&
		complete {
		return id, nil
	} else if err != nil &&
		!errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("putBlob: Scan: %w", err)
	}
	var used int64
	if err := tx.QueryRow(ctx, "SELECT COALESCE(SUM(size),0) FROM evidence_blob").
		Scan(&used); err != nil {
		return "", fmt.Errorf("putBlob: Scan: %w", err)
	}
	if int64(len(data)) > limit-used {
		return "", errRetainedEvidenceByteBudgetExhausted
	}
	_, err := tx.Exec(
		ctx,
		"INSERT INTO evidence_blob(id,size) VALUES($1,$2) ON CONFLICT DO NOTHING",
		id,
		len(data),
	)
	if err != nil {
		return "", fmt.Errorf("putBlob: Exec: %w", err)
	}
	for off, n := 0, 0; off < len(data); n++ {
		end := off + 1<<20
		if end > len(data) {
			end = len(data)
		}
		chunk := data[off:end]
		cid := model.Digest(chunk)
		if _, err = tx.Exec(
			ctx,
			"INSERT INTO evidence_chunk VALUES($1,$2) ON CONFLICT DO NOTHING",
			cid,
			chunk,
		); err != nil {
			return "", fmt.Errorf("putBlob: Exec: %w", err)
		}
		if _, err = tx.Exec(
			ctx,
			"INSERT INTO blob_chunk VALUES($1,$2,$3) ON CONFLICT DO NOTHING",
			id,
			n,
			cid,
		); err != nil {
			return "", fmt.Errorf("putBlob: Exec: %w", err)
		}
		off = end
	}
	_, err = tx.Exec(ctx, "UPDATE evidence_blob SET complete=true WHERE id=$1", id)
	if err != nil {
		return id, fmt.Errorf("putBlob: Exec: %w", err)
	}
	return id, nil
}

func (s *Store) Publish(
	ctx context.Context,
	run model.Run,
	observations []model.Observation,
) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("Publish: Begin: %w", err)
	}
	defer rollback(ctx, tx)
	var state, targetID, artifactID string
	if err = tx.QueryRow(ctx, "SELECT r.state,r.target_id,t.artifact_id FROM analysis_run r JOIN firmware_target t ON t.id=r.target_id WHERE r.id=$1 FOR UPDATE OF r", run.ID).
		Scan(&state, &targetID, &artifactID); err != nil {
		return fmt.Errorf("Publish: Scan: %w", err)
	}
	if targetID != run.TargetID {
		return errRunTargetMismatch
	}
	if run.State != "complete" && run.State != "partial" {
		return errInvalidPublicationState
	}
	if len(run.Coverage) == 0 {
		return errCoverageLedgerIsRequired
	}
	for _, c := range run.Coverage {
		if run.State == "complete" && c.State != "complete" {
			return errCompleteRunHasIncompleteCoverage
		}
	}
	if state != "running" {
		return fmt.Errorf("%w: %s", errRunIsAlreadyTerminal, state)
	}
	verified := map[string][]byte{}
	for _, o := range observations {
		if o.Source.ArtifactID != artifactID {
			return errObservationArtifactMismatch
		}
		if o.TargetID != run.TargetID {
			return errObservationTargetMismatch
		}
		if err = o.ValidateMetadata(); err != nil {
			return fmt.Errorf("Publish: ValidateMetadata: %w", err)
		}
		id := o.EvidenceID
		if prior, ok := verified[id]; ok {
			sameBacking := len(prior) == len(o.Evidence) &&
				(len(prior) == 0 || &prior[0] == &o.Evidence[0])
			if !sameBacking && !bytes.Equal(prior, o.Evidence) {
				return errConflictingEvidence
			}
		} else {
			if err = o.Validate(); err != nil {
				return fmt.Errorf("Publish: Validate: %w", err)
			}
			if _, err = putBlob(ctx, tx, o.Evidence, s.EvidenceLimit); err != nil {
				return err
			}
			verified[id] = o.Evidence
		}
		b, e := json.Marshal(o)
		if e != nil {
			return fmt.Errorf("Publish: json.Marshal: %w", e)
		}
		if _, err = tx.Exec(
			ctx,
			"INSERT INTO observation VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING",
			o.ID,
			o.TargetID,
			id,
			b,
		); err != nil {
			return fmt.Errorf("Publish: Exec: %w", err)
		}
		if _, err = tx.Exec(
			ctx,
			"INSERT INTO run_observation VALUES($1,$2) ON CONFLICT DO NOTHING",
			run.ID,
			o.ID,
		); err != nil {
			return fmt.Errorf("Publish: Exec: %w", err)
		}
		if _, err = tx.Exec(
			ctx,
			"INSERT INTO finding_revision(id,observation_id,basis,rationale) VALUES($1,$2,'observed','Direct extraction; not a claim of emulated behavior') ON CONFLICT DO NOTHING",
			model.ID("finding", o.ID),
			o.ID,
		); err != nil {
			return fmt.Errorf("Publish: Exec: %w", err)
		}
	}
	b, err := json.Marshal(run.Coverage)
	if err != nil {
		return fmt.Errorf("Publish: json.Marshal: %w", err)
	}
	if _, err = tx.Exec(
		ctx,
		"UPDATE analysis_run SET state=$2,finished=now(),coverage=$3 WHERE id=$1",
		run.ID,
		run.State,
		b,
	); err != nil {
		return fmt.Errorf("Publish: Exec: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("Publish: Commit: %w", err)
	}
	return nil
}

func (s *Store) Fail(ctx context.Context, id, state string) error {
	_, err := s.pool.Exec(
		ctx,
		"UPDATE analysis_run SET state=$2,finished=now() WHERE id=$1 AND state='running'",
		id,
		state,
	)
	if err != nil {
		return fmt.Errorf("Fail: Exec: %w", err)
	}
	return nil
}

func (s *Store) Acquisition(
	ctx context.Context,
	source string,
	request any,
	status string,
	data []byte,
) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("Acquisition: Begin: %w", err)
	}
	defer rollback(ctx, tx)
	id, err := putBlob(ctx, tx, data, s.EvidenceLimit)
	if err != nil {
		return err
	}
	b, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("Acquisition: json.Marshal: %w", err)
	}
	_, err = tx.Exec(
		ctx,
		"INSERT INTO acquisition(source,requested,status,evidence_id) VALUES($1,$2,$3,$4)",
		source,
		b,
		status,
		id,
	)
	if err != nil {
		return fmt.Errorf("Acquisition: Exec: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("Acquisition: Commit: %w", err)
	}
	return nil
}

func (s *Store) Findings(ctx context.Context, target, run string) ([]model.Finding, error) {
	rows, err := s.pool.Query(
		ctx,
		`SELECT DISTINCT f.id,f.basis,f.assessment,o.document FROM finding_revision f JOIN observation o ON o.id=f.observation_id JOIN run_observation ro ON ro.observation_id=o.id WHERE ($1='' OR o.target_id=$1) AND ($2='' OR ro.run_id=$2) ORDER BY f.id`,
		target,
		run,
	)
	if err != nil {
		return nil, fmt.Errorf("Findings: Query: %w", err)
	}
	defer rows.Close()
	out := []model.Finding{}
	for rows.Next() {
		var f model.Finding
		var b []byte
		if err = rows.Scan(&f.ID, &f.Basis, &f.Assessment, &b); err != nil {
			return nil, fmt.Errorf("Findings: Scan: %w", err)
		}
		if err = json.Unmarshal(b, &f.Observation); err != nil {
			return nil, fmt.Errorf("Findings: json.Unmarshal: %w", err)
		}
		f.RunID = run
		out = append(out, f)
	}
	if err := rows.Err(); err != nil {
		return out, fmt.Errorf("Findings: Err: %w", err)
	}
	return out, nil
}

func (s *Store) Evidence(ctx context.Context, id string) ([]byte, error) {
	var size int64
	var complete bool
	err := s.pool.QueryRow(ctx, "SELECT size,complete FROM evidence_blob WHERE id=$1", id).
		Scan(&size, &complete)
	if err != nil {
		return nil, fmt.Errorf("Evidence: Scan: %w", err)
	}
	if !complete || size < 0 || size > 128<<20 {
		return nil, errUnreadableEvidence
	}
	rows, err := s.pool.Query(
		ctx,
		"SELECT c.data FROM blob_chunk b JOIN evidence_chunk c ON c.id=b.chunk_id WHERE b.blob_id=$1 ORDER BY b.ordinal",
		id,
	)
	if err != nil {
		return nil, fmt.Errorf("Evidence: Query: %w", err)
	}
	defer rows.Close()
	out := make([]byte, 0, int(size))
	for rows.Next() {
		var b []byte
		if err = rows.Scan(&b); err != nil {
			return nil, fmt.Errorf("Evidence: Scan: %w", err)
		}
		out = append(out, b...)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("Evidence: Err: %w", err)
	}
	if int64(len(out)) != size || model.Digest(out) != id {
		return nil, errStoredEvidenceIntegrityMismatch
	}
	return out, nil
}

func (s *Store) Run(ctx context.Context, id string) (model.Run, error) {
	var r model.Run
	var b []byte
	err := s.pool.QueryRow(ctx, "SELECT id,target_id,state,started,coverage FROM analysis_run WHERE id=$1", id).
		Scan(&r.ID, &r.TargetID, &r.State, &r.Started, &b)
	if err != nil {
		return r, fmt.Errorf("Run: Scan: %w", err)
	}
	err = json.Unmarshal(b, &r.Coverage)
	if err != nil {
		return r, fmt.Errorf("Run: json.Unmarshal: %w", err)
	}
	return r, nil
}
