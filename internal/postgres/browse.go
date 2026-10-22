package postgres

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/deploymenttheory/go-ios-vm/internal/model"
)

const MaxBrowsePage = 100

var (
	ErrInvalidBrowsePage = errors.New("invalid browse page")
	ErrInvalidCursor     = errors.New("invalid browse cursor")
	ErrBrowseTooLarge    = errors.New("browse detail exceeds component limit")
)

type PageOptions struct {
	Limit  int
	Cursor string
}

type Page[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"next_cursor,omitempty"`
}

type ArtifactRecord struct {
	ID                  string `json:"id"`
	SHA256              string `json:"sha256"`
	Size                int64  `json:"size"`
	RegisteredLocations int64  `json:"registered_locations"`
	SourceAvailability  string `json:"source_availability"`
}

type TargetRecord struct {
	Target   model.Target   `json:"target"`
	Artifact ArtifactRecord `json:"artifact"`
}

type TargetFilter struct {
	ArtifactID string
	Product    string
	Build      string
	Identity   *int
	Page       PageOptions
}

type AcquisitionRecord struct {
	ID                int64     `json:"id"`
	Source            string    `json:"-"`
	Status            string    `json:"status"`
	Product           string    `json:"product,omitempty"`
	Version           string    `json:"version,omitempty"`
	Build             string    `json:"build,omitempty"`
	EvidenceID        string    `json:"evidence_id"`
	EvidenceAvailable bool      `json:"evidence_available"`
	Created           time.Time `json:"created"`
}

type AcquisitionFilter struct {
	Product string
	Build   string
	Status  string
	From    *time.Time
	Before  *time.Time
	Page    PageOptions
}

type RunRecord struct {
	ID                     string             `json:"id"`
	TargetID               string             `json:"target_id"`
	State                  string             `json:"state"`
	Started                time.Time          `json:"started"`
	Finished               *time.Time         `json:"finished,omitempty"`
	InputSHA256            string             `json:"input_sha256"`
	InputSize              int64              `json:"input_size"`
	Extractor              string             `json:"extractor"`
	Planned                []string           `json:"planned_components"`
	CommittedCheckpoints   []model.Coverage   `json:"committed_checkpoints,omitempty"`
	FinalCoverage          []model.Coverage   `json:"final_coverage,omitempty"`
	CommittedBatches       []ComponentBatches `json:"committed_batches,omitempty"`
	FinalCoverageAvailable bool               `json:"final_coverage_available"`
}

type ComponentBatches struct {
	Component string `json:"component"`
	Count     int64  `json:"count"`
}

type RunFilter struct {
	TargetID string
	State    string
	From     *time.Time
	Before   *time.Time
	Page     PageOptions
}

func pageLimit(options PageOptions) (int, error) {
	if options.Limit == 0 {
		return 25, nil
	}
	if options.Limit < 1 || options.Limit > MaxBrowsePage {
		return 0, ErrInvalidBrowsePage
	}
	return options.Limit, nil
}

func artifactAvailability(locations int64) string {
	if locations > 0 {
		return "registered_unverified"
	}
	return "not_registered"
}

const artifactColumns = `a.id,a.sha256,a.size,
 (SELECT count(*) FROM artifact_location l WHERE l.artifact_id=a.id)`

func scanArtifact(row pgx.Row) (ArtifactRecord, error) {
	var a ArtifactRecord
	if err := row.Scan(&a.ID, &a.SHA256, &a.Size, &a.RegisteredLocations); err != nil {
		return a, fmt.Errorf("scanArtifact: Scan: %w", err)
	}
	a.SourceAvailability = artifactAvailability(a.RegisteredLocations)
	return a, nil
}

func (s *Store) BrowseArtifacts(
	ctx context.Context,
	sha256 string,
	page PageOptions,
) (Page[ArtifactRecord], error) {
	limit, err := pageLimit(page)
	if err != nil {
		return Page[ArtifactRecord]{}, err
	}
	rows, err := s.pool.Query(ctx, `SELECT `+artifactColumns+`
 FROM artifact a WHERE ($1='' OR a.sha256=$1) AND ($2='' OR a.id>$2)
 ORDER BY a.id LIMIT $3`, sha256, page.Cursor, limit+1)
	if err != nil {
		return Page[ArtifactRecord]{}, fmt.Errorf("BrowseArtifacts: Query: %w", err)
	}
	defer rows.Close()
	out := Page[ArtifactRecord]{Items: []ArtifactRecord{}}
	for rows.Next() {
		a, err := scanArtifact(rows)
		if err != nil {
			return out, fmt.Errorf("BrowseArtifacts: Scan: %w", err)
		}
		out.Items = append(out.Items, a)
	}
	if err := rows.Err(); err != nil {
		return out, fmt.Errorf("BrowseArtifacts: Rows: %w", err)
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		out.NextCursor = out.Items[len(out.Items)-1].ID
	}
	return out, nil
}

func (s *Store) BrowseArtifact(ctx context.Context, id string) (ArtifactRecord, error) {
	a, err := scanArtifact(s.pool.QueryRow(ctx,
		`SELECT `+artifactColumns+` FROM artifact a WHERE a.id=$1`, id))
	if err != nil {
		return a, fmt.Errorf("BrowseArtifact: Scan: %w", err)
	}
	return a, nil
}

const targetColumns = `t.document,a.id,a.sha256,a.size,
 (SELECT count(*) FROM artifact_location l WHERE l.artifact_id=a.id)`

func scanTarget(row pgx.Row) (TargetRecord, error) {
	var record TargetRecord
	var raw []byte
	if err := row.Scan(&raw, &record.Artifact.ID, &record.Artifact.SHA256,
		&record.Artifact.Size, &record.Artifact.RegisteredLocations); err != nil {
		return record, fmt.Errorf("scanTarget: Scan: %w", err)
	}
	if err := json.Unmarshal(raw, &record.Target); err != nil {
		return record, fmt.Errorf("scanTarget: json.Unmarshal: %w", err)
	}
	record.Artifact.SourceAvailability = artifactAvailability(record.Artifact.RegisteredLocations)
	return record, nil
}

func (s *Store) BrowseTargets(
	ctx context.Context,
	filter TargetFilter,
) (Page[TargetRecord], error) {
	limit, err := pageLimit(filter.Page)
	if err != nil {
		return Page[TargetRecord]{}, err
	}
	rows, err := s.pool.Query(ctx, `SELECT `+targetColumns+`
 FROM firmware_target t JOIN artifact a ON a.id=t.artifact_id
 WHERE ($1='' OR t.artifact_id=$1) AND ($2='' OR t.document->'products' ? $2)
 AND ($3='' OR t.document->>'build'=$3) AND ($4::integer IS NULL OR t.identity_index=$4)
 AND ($5='' OR t.id>$5) ORDER BY t.id LIMIT $6`,
		filter.ArtifactID, filter.Product, filter.Build, filter.Identity,
		filter.Page.Cursor, limit+1)
	if err != nil {
		return Page[TargetRecord]{}, fmt.Errorf("BrowseTargets: Query: %w", err)
	}
	defer rows.Close()
	out := Page[TargetRecord]{Items: []TargetRecord{}}
	for rows.Next() {
		record, err := scanTarget(rows)
		if err != nil {
			return out, fmt.Errorf("BrowseTargets: Scan: %w", err)
		}
		out.Items = append(out.Items, record)
	}
	if err := rows.Err(); err != nil {
		return out, fmt.Errorf("BrowseTargets: Rows: %w", err)
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		out.NextCursor = out.Items[len(out.Items)-1].Target.ID
	}
	return out, nil
}

func (s *Store) BrowseTarget(ctx context.Context, id string) (TargetRecord, error) {
	record, err := scanTarget(s.pool.QueryRow(ctx,
		`SELECT `+targetColumns+`
 FROM firmware_target t JOIN artifact a ON a.id=t.artifact_id WHERE t.id=$1`, id))
	if err != nil {
		return record, fmt.Errorf("BrowseTarget: Scan: %w", err)
	}
	return record, nil
}

const (
	acquisitionSelection = `COALESCE(a.requested #>> '{selection,product}',
 a.requested #>> '{candidate,product}',a.requested->>'product','')`
	acquisitionVersion = `COALESCE(a.requested #>> '{selection,version}',
 a.requested #>> '{candidate,version}',a.requested->>'version','')`
	acquisitionBuild = `COALESCE(a.requested #>> '{selection,build}',
 a.requested #>> '{candidate,build}',a.requested->>'build','')`
)

const acquisitionColumns = `a.id,a.source,a.status,` + acquisitionSelection + `,` +
	acquisitionVersion + `,` + acquisitionBuild + `,a.evidence_id,e.complete,a.created`

func scanAcquisition(row pgx.Row) (AcquisitionRecord, error) {
	var record AcquisitionRecord
	err := row.Scan(&record.ID, &record.Source, &record.Status, &record.Product,
		&record.Version, &record.Build, &record.EvidenceID,
		&record.EvidenceAvailable, &record.Created)
	if err != nil {
		return record, fmt.Errorf("scanAcquisition: Scan: %w", err)
	}
	return record, nil
}

func (s *Store) BrowseAcquisitions(
	ctx context.Context,
	filter AcquisitionFilter,
) (Page[AcquisitionRecord], error) {
	limit, err := pageLimit(filter.Page)
	if err != nil {
		return Page[AcquisitionRecord]{}, err
	}
	var cursor int64
	if filter.Page.Cursor != "" {
		cursor, err = strconv.ParseInt(filter.Page.Cursor, 10, 64)
		if err != nil || cursor <= 0 {
			return Page[AcquisitionRecord]{}, ErrInvalidCursor
		}
	}
	rows, err := s.pool.Query(ctx, `SELECT `+acquisitionColumns+`
 FROM acquisition a JOIN evidence_blob e ON e.id=a.evidence_id
 WHERE ($1='' OR `+acquisitionSelection+`=$1) AND ($2='' OR `+acquisitionBuild+`=$2)
 AND ($3='' OR a.status=$3) AND ($4::timestamptz IS NULL OR a.created>=$4)
 AND ($5::timestamptz IS NULL OR a.created<$5) AND ($6=0 OR a.id<$6)
 ORDER BY a.id DESC LIMIT $7`,
		filter.Product, filter.Build, filter.Status, filter.From, filter.Before,
		cursor, limit+1)
	if err != nil {
		return Page[AcquisitionRecord]{}, fmt.Errorf("BrowseAcquisitions: Query: %w", err)
	}
	defer rows.Close()
	out := Page[AcquisitionRecord]{Items: []AcquisitionRecord{}}
	for rows.Next() {
		record, err := scanAcquisition(rows)
		if err != nil {
			return out, fmt.Errorf("BrowseAcquisitions: Scan: %w", err)
		}
		out.Items = append(out.Items, record)
	}
	if err := rows.Err(); err != nil {
		return out, fmt.Errorf("BrowseAcquisitions: Rows: %w", err)
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		out.NextCursor = strconv.FormatInt(out.Items[len(out.Items)-1].ID, 10)
	}
	return out, nil
}

func (s *Store) BrowseAcquisition(ctx context.Context, id int64) (AcquisitionRecord, error) {
	record, err := scanAcquisition(s.pool.QueryRow(ctx,
		`SELECT `+acquisitionColumns+`
 FROM acquisition a JOIN evidence_blob e ON e.id=a.evidence_id WHERE a.id=$1`, id))
	if err != nil {
		return record, fmt.Errorf("BrowseAcquisition: Scan: %w", err)
	}
	return record, nil
}

type runCursor struct {
	Started time.Time `json:"started"`
	ID      string    `json:"id"`
}

func encodeRunCursor(run RunRecord) string {
	data, _ := json.Marshal(runCursor{Started: run.Started, ID: run.ID})
	return base64.RawURLEncoding.EncodeToString(data)
}

func decodeRunCursor(value string) (runCursor, error) {
	if value == "" {
		return runCursor{}, nil
	}
	if len(value) > 512 {
		return runCursor{}, ErrInvalidCursor
	}
	data, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return runCursor{}, ErrInvalidCursor
	}
	var cursor runCursor
	if err := json.Unmarshal(data, &cursor); err != nil ||
		cursor.Started.IsZero() || cursor.ID == "" {
		return runCursor{}, ErrInvalidCursor
	}
	return cursor, nil
}

const runColumns = `id,target_id,state,started,finished,input_sha256,input_size,
 extractor,planned_components`

func scanRun(row pgx.Row) (RunRecord, error) {
	var record RunRecord
	var finished pgtype.Timestamptz
	var plan []byte
	err := row.Scan(&record.ID, &record.TargetID, &record.State, &record.Started,
		&finished, &record.InputSHA256, &record.InputSize, &record.Extractor, &plan)
	if err != nil {
		return record, fmt.Errorf("scanRun: Scan: %w", err)
	}
	if finished.Valid {
		record.Finished = &finished.Time
	}
	if err := json.Unmarshal(plan, &record.Planned); err != nil {
		return record, fmt.Errorf("scanRun: json.Unmarshal: %w", err)
	}
	if len(record.Planned) > 128 {
		return record, ErrBrowseTooLarge
	}
	if record.Planned == nil {
		record.Planned = []string{}
	}
	return record, nil
}

func (s *Store) BrowseRuns(ctx context.Context, filter RunFilter) (Page[RunRecord], error) {
	limit, err := pageLimit(filter.Page)
	if err != nil {
		return Page[RunRecord]{}, err
	}
	cursor, err := decodeRunCursor(filter.Page.Cursor)
	if err != nil {
		return Page[RunRecord]{}, err
	}
	var cursorTime *time.Time
	if !cursor.Started.IsZero() {
		cursorTime = &cursor.Started
	}
	rows, err := s.pool.Query(ctx, `SELECT `+runColumns+`
 FROM analysis_run WHERE ($1='' OR target_id=$1) AND ($2='' OR state=$2)
 AND ($3::timestamptz IS NULL OR started>=$3)
 AND ($4::timestamptz IS NULL OR started<$4)
 AND ($5::timestamptz IS NULL OR (started,id)<($5::timestamptz,$6::text))
 ORDER BY started DESC,id DESC LIMIT $7`,
		filter.TargetID, filter.State, filter.From, filter.Before,
		cursorTime, cursor.ID, limit+1)
	if err != nil {
		return Page[RunRecord]{}, fmt.Errorf("BrowseRuns: Query: %w", err)
	}
	defer rows.Close()
	out := Page[RunRecord]{Items: []RunRecord{}}
	for rows.Next() {
		record, err := scanRun(rows)
		if err != nil {
			return out, fmt.Errorf("BrowseRuns: Scan: %w", err)
		}
		out.Items = append(out.Items, record)
	}
	if err := rows.Err(); err != nil {
		return out, fmt.Errorf("BrowseRuns: Rows: %w", err)
	}
	if len(out.Items) > limit {
		out.Items = out.Items[:limit]
		out.NextCursor = encodeRunCursor(out.Items[len(out.Items)-1])
	}
	return out, nil
}

func (s *Store) BrowseRun(ctx context.Context, id string) (RunRecord, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return RunRecord{}, fmt.Errorf("BrowseRun: BeginTx: %w", err)
	}
	defer rollback(ctx, tx)
	record, err := browseRunTx(ctx, tx, id)
	if err != nil {
		return record, err
	}
	if err := tx.Commit(ctx); err != nil {
		return record, fmt.Errorf("BrowseRun: Commit: %w", err)
	}
	return record, nil
}

func browseRunTx(ctx context.Context, tx pgx.Tx, id string) (RunRecord, error) {
	record, err := scanRun(tx.QueryRow(ctx,
		`SELECT `+runColumns+` FROM analysis_run WHERE id=$1`, id))
	if err != nil {
		return record, fmt.Errorf("BrowseRun: Scan: %w", err)
	}
	record.CommittedCheckpoints, err = browseCoverage(ctx, tx, "checkpoint", id)
	if err != nil {
		return record, err
	}
	record.CommittedCheckpoints = plannedCoverageOrder(record.Planned, record.CommittedCheckpoints)
	record.FinalCoverage, err = browseCoverage(ctx, tx, "final", id)
	if err != nil {
		return record, err
	}
	record.FinalCoverage = plannedCoverageOrder(record.Planned, record.FinalCoverage)
	record.FinalCoverageAvailable = len(record.FinalCoverage) > 0
	rows, err := tx.Query(ctx,
		`SELECT component,count(*) FROM run_batch
 WHERE run_id=$1 GROUP BY component ORDER BY component LIMIT 129`, id)
	if err != nil {
		return record, fmt.Errorf("BrowseRun: batches Query: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var b ComponentBatches
		if err := rows.Scan(&b.Component, &b.Count); err != nil {
			return record, fmt.Errorf("BrowseRun: batches Scan: %w", err)
		}
		record.CommittedBatches = append(record.CommittedBatches, b)
	}
	if err := rows.Err(); err != nil {
		return record, fmt.Errorf("BrowseRun: batches Rows: %w", err)
	}
	rows.Close()
	if len(record.CommittedBatches) > 128 {
		return record, ErrBrowseTooLarge
	}
	return record, nil
}

func browseCoverage(ctx context.Context, tx pgx.Tx, kind, runID string) ([]model.Coverage, error) {
	query := `SELECT component,state,reason FROM run_checkpoint
 WHERE run_id=$1 ORDER BY component LIMIT 129`
	if kind == "final" {
		query = `SELECT component,state,reason FROM run_coverage
 WHERE run_id=$1 ORDER BY component LIMIT 129`
	}
	rows, err := tx.Query(ctx, query, runID)
	if err != nil {
		return nil, fmt.Errorf("browseCoverage: Query: %w", err)
	}
	defer rows.Close()
	out := []model.Coverage{}
	for rows.Next() {
		var c model.Coverage
		if err := rows.Scan(&c.Component, &c.State, &c.Reason); err != nil {
			return nil, fmt.Errorf("browseCoverage: Scan: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("browseCoverage: Rows: %w", err)
	}
	if len(out) > 128 {
		return nil, ErrBrowseTooLarge
	}
	return out, nil
}

func plannedCoverageOrder(planned []string, coverage []model.Coverage) []model.Coverage {
	if len(planned) == 0 || len(coverage) == 0 {
		return coverage
	}
	byComponent := make(map[string]model.Coverage, len(coverage))
	for _, item := range coverage {
		byComponent[item.Component] = item
	}
	ordered := make([]model.Coverage, 0, len(coverage))
	for _, component := range planned {
		if item, ok := byComponent[component]; ok {
			ordered = append(ordered, item)
			delete(byComponent, component)
		}
	}
	for _, item := range coverage {
		if _, ok := byComponent[item.Component]; ok {
			ordered = append(ordered, item)
		}
	}
	return ordered
}
