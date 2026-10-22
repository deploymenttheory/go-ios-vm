// Package postgres owns transactional evidence publication.
package postgres

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"github.com/deploymenttheory/go-ios-vm/internal/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/tern/v2/migrate"
	"io/fs"
)

//go:embed migrations/*.sql
var migrations embed.FS

type Store struct {
	pool          *pgxpool.Pool
	EvidenceLimit int64
}

func Open(ctx context.Context, dsn string) (*Store, error) {
	if dsn == "" {
		return nil, fmt.Errorf("database URL is required (GOIOSVM_DATABASE_URL)")
	}
	c, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("invalid PostgreSQL configuration")
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
		return err
	}
	defer c.Release()
	m, err := migrate.NewMigrator(ctx, c.Conn(), "schema_version")
	if err != nil {
		return err
	}
	sub, err := fs.Sub(migrations, "migrations")
	if err != nil {
		return err
	}
	if err = m.LoadMigrations(sub); err != nil {
		return err
	}
	return m.Migrate(ctx)
}
func (s *Store) Register(ctx context.Context, a model.Artifact, targets []model.Target) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(ctx, "INSERT INTO artifact VALUES($1,$2,$3) ON CONFLICT DO NOTHING", a.ID, a.SHA256, a.Size); err != nil {
		return err
	}
	if a.Path != "" {
		if _, err = tx.Exec(ctx, "INSERT INTO artifact_location VALUES($1,$2) ON CONFLICT DO NOTHING", a.ID, a.Path); err != nil {
			return err
		}
	}
	for _, t := range targets {
		b, e := json.Marshal(t)
		if e != nil {
			return e
		}
		if _, err = tx.Exec(ctx, "INSERT INTO firmware_target VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING", t.ID, a.ID, t.Identity, b); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
func (s *Store) Start(ctx context.Context, r model.Run) error {
	_, err := s.pool.Exec(ctx, "INSERT INTO analysis_run(id,target_id,state,started) VALUES($1,$2,'running',$3)", r.ID, r.TargetID, r.Started)
	return err
}
func putBlob(ctx context.Context, tx pgx.Tx, data []byte, limit int64) (string, error) {
	if len(data) > 128<<20 {
		return "", fmt.Errorf("evidence object exceeds 128 MiB")
	}
	if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(719230113)"); err != nil {
		return "", err
	}
	id := model.Digest(data)
	var complete bool
	if err := tx.QueryRow(ctx, "SELECT complete FROM evidence_blob WHERE id=$1", id).Scan(&complete); err == nil && complete {
		return id, nil
	} else if err != nil && err != pgx.ErrNoRows {
		return "", err
	}
	var used int64
	if err := tx.QueryRow(ctx, "SELECT COALESCE(SUM(size),0) FROM evidence_blob").Scan(&used); err != nil {
		return "", err
	}
	if int64(len(data)) > limit-used {
		return "", fmt.Errorf("retained evidence byte budget exhausted")
	}
	_, err := tx.Exec(ctx, "INSERT INTO evidence_blob(id,size) VALUES($1,$2) ON CONFLICT DO NOTHING", id, len(data))
	if err != nil {
		return "", err
	}
	for off, n := 0, 0; off < len(data); n++ {
		end := off + 1<<20
		if end > len(data) {
			end = len(data)
		}
		chunk := data[off:end]
		cid := model.Digest(chunk)
		if _, err = tx.Exec(ctx, "INSERT INTO evidence_chunk VALUES($1,$2) ON CONFLICT DO NOTHING", cid, chunk); err != nil {
			return "", err
		}
		if _, err = tx.Exec(ctx, "INSERT INTO blob_chunk VALUES($1,$2,$3) ON CONFLICT DO NOTHING", id, n, cid); err != nil {
			return "", err
		}
		off = end
	}
	_, err = tx.Exec(ctx, "UPDATE evidence_blob SET complete=true WHERE id=$1", id)
	return id, err
}
func (s *Store) Publish(ctx context.Context, run model.Run, observations []model.Observation) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	var state, targetID, artifactID string
	if err = tx.QueryRow(ctx, "SELECT r.state,r.target_id,t.artifact_id FROM analysis_run r JOIN firmware_target t ON t.id=r.target_id WHERE r.id=$1 FOR UPDATE OF r", run.ID).Scan(&state, &targetID, &artifactID); err != nil {
		return err
	}
	if targetID != run.TargetID {
		return fmt.Errorf("run target mismatch")
	}
	if run.State != "complete" && run.State != "partial" {
		return fmt.Errorf("invalid publication state")
	}
	if len(run.Coverage) == 0 {
		return fmt.Errorf("coverage ledger is required")
	}
	for _, c := range run.Coverage {
		if run.State == "complete" && c.State != "complete" {
			return fmt.Errorf("complete run has incomplete coverage")
		}
	}
	if state != "running" {
		return fmt.Errorf("run is already terminal: %s", state)
	}
	verified := map[string][]byte{}
	for _, o := range observations {
		if o.Source.ArtifactID != artifactID {
			return fmt.Errorf("observation source belongs to a different artifact")
		}
		if o.TargetID != run.TargetID {
			return fmt.Errorf("observation belongs to different target")
		}
		if err = o.ValidateMetadata(); err != nil {
			return err
		}
		id := o.EvidenceID
		if prior, ok := verified[id]; ok {
			sameBacking := len(prior) == len(o.Evidence) && (len(prior) == 0 || &prior[0] == &o.Evidence[0])
			if !sameBacking && !bytes.Equal(prior, o.Evidence) {
				return fmt.Errorf("conflicting bytes for evidence identity")
			}
		} else {
			if err = o.Validate(); err != nil {
				return err
			}
			if _, err = putBlob(ctx, tx, o.Evidence, s.EvidenceLimit); err != nil {
				return err
			}
			verified[id] = o.Evidence
		}
		b, e := json.Marshal(o)
		if e != nil {
			return e
		}
		if _, err = tx.Exec(ctx, "INSERT INTO observation VALUES($1,$2,$3,$4) ON CONFLICT DO NOTHING", o.ID, o.TargetID, id, b); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "INSERT INTO run_observation VALUES($1,$2) ON CONFLICT DO NOTHING", run.ID, o.ID); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "INSERT INTO finding_revision(id,observation_id,basis,rationale) VALUES($1,$2,'observed','Direct extraction; not a claim of emulated behavior') ON CONFLICT DO NOTHING", model.ID("finding", o.ID), o.ID); err != nil {
			return err
		}
	}
	b, err := json.Marshal(run.Coverage)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "UPDATE analysis_run SET state=$2,finished=now(),coverage=$3 WHERE id=$1", run.ID, run.State, b); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (s *Store) Fail(ctx context.Context, id, state string) error {
	_, err := s.pool.Exec(ctx, "UPDATE analysis_run SET state=$2,finished=now() WHERE id=$1 AND state='running'", id, state)
	return err
}
func (s *Store) Acquisition(ctx context.Context, source string, request any, status string, data []byte) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	id, err := putBlob(ctx, tx, data, s.EvidenceLimit)
	if err != nil {
		return err
	}
	b, err := json.Marshal(request)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, "INSERT INTO acquisition(source,requested,status,evidence_id) VALUES($1,$2,$3,$4)", source, b, status, id)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (s *Store) Findings(ctx context.Context, target, run string) ([]model.Finding, error) {
	rows, err := s.pool.Query(ctx, `SELECT DISTINCT f.id,f.basis,f.assessment,o.document FROM finding_revision f JOIN observation o ON o.id=f.observation_id JOIN run_observation ro ON ro.observation_id=o.id WHERE ($1='' OR o.target_id=$1) AND ($2='' OR ro.run_id=$2) ORDER BY f.id`, target, run)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Finding{}
	for rows.Next() {
		var f model.Finding
		var b []byte
		if err = rows.Scan(&f.ID, &f.Basis, &f.Assessment, &b); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(b, &f.Observation); err != nil {
			return nil, err
		}
		f.RunID = run
		out = append(out, f)
	}
	return out, rows.Err()
}
func (s *Store) Evidence(ctx context.Context, id string) ([]byte, error) {
	var size int64
	var complete bool
	err := s.pool.QueryRow(ctx, "SELECT size,complete FROM evidence_blob WHERE id=$1", id).Scan(&size, &complete)
	if err != nil {
		return nil, err
	}
	if !complete || size > 128<<20 {
		return nil, fmt.Errorf("evidence incomplete or exceeds read limit")
	}
	rows, err := s.pool.Query(ctx, "SELECT c.data FROM blob_chunk b JOIN evidence_chunk c ON c.id=b.chunk_id WHERE b.blob_id=$1 ORDER BY b.ordinal", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]byte, 0, int(size))
	for rows.Next() {
		var b []byte
		if err = rows.Scan(&b); err != nil {
			return nil, err
		}
		out = append(out, b...)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	if int64(len(out)) != size || model.Digest(out) != id {
		return nil, fmt.Errorf("stored evidence integrity mismatch")
	}
	return out, nil
}
func (s *Store) Run(ctx context.Context, id string) (model.Run, error) {
	var r model.Run
	var b []byte
	err := s.pool.QueryRow(ctx, "SELECT id,target_id,state,started,coverage FROM analysis_run WHERE id=$1", id).Scan(&r.ID, &r.TargetID, &r.State, &r.Started, &b)
	if err != nil {
		return r, err
	}
	err = json.Unmarshal(b, &r.Coverage)
	return r, err
}
