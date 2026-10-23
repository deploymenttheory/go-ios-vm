package postgres

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/go-ios-vm/internal/model"
)

// ProgressBrowseItem deliberately exposes only the request's catalog identity.
// Source URLs, cache paths and worker leases are never part of this read model.
type ProgressBrowseItem struct {
	Progress       model.Progress `json:"progress"`
	EffectiveState string         `json:"effective_state"`
	Stale          bool           `json:"stale"`
	Product        string         `json:"product,omitempty"`
	Version        string         `json:"version,omitempty"`
	Build          string         `json:"build,omitempty"`
	TargetID       string         `json:"target_id,omitempty"`
	AcquisitionID  *int64         `json:"acquisition_id,omitempty"`
}

type ProgressBrowseFilter struct {
	Kind  string
	State string // "terminal" selects all non-running states.
	Page  PageOptions
}

type progressCursor struct {
	Updated time.Time `json:"updated"`
	ID      string    `json:"id"`
	Kind    string    `json:"kind"`
	State   string    `json:"state"`
}

func decodeProgressCursor(value string) (progressCursor, error) {
	if value == "" {
		return progressCursor{}, nil
	}
	if len(value) > 512 {
		return progressCursor{}, ErrInvalidCursor
	}
	data, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return progressCursor{}, ErrInvalidCursor
	}
	var cursor progressCursor
	if err := json.Unmarshal(data, &cursor); err != nil ||
		cursor.Updated.IsZero() || cursor.ID == "" {
		return progressCursor{}, ErrInvalidCursor
	}
	return cursor, nil
}

func encodeProgressCursor(item ProgressBrowseItem, filter ProgressBrowseFilter) string {
	data, _ := json.Marshal(progressCursor{
		Updated: item.Progress.Updated, ID: item.Progress.ID,
		Kind: filter.Kind, State: filter.State,
	})
	return base64.RawURLEncoding.EncodeToString(data)
}

// BrowseProgress returns bounded, recent committed snapshots from a single
// database snapshot. A running inspection with an expired lease is marked stale.
func (s *Store) BrowseProgress(
	ctx context.Context,
	filter ProgressBrowseFilter,
) (Page[ProgressBrowseItem], error) {
	limit, err := pageLimit(filter.Page)
	if err != nil {
		return Page[ProgressBrowseItem]{}, err
	}
	if filter.Kind != "" && filter.Kind != "acquisition" && filter.Kind != "inspection" {
		return Page[ProgressBrowseItem]{}, ErrInvalidBrowsePage
	}
	switch filter.State {
	case "", "running", "terminal", "interrupted", "complete", "partial", "failed", "cancelled":
	default:
		return Page[ProgressBrowseItem]{}, ErrInvalidBrowsePage
	}
	cursor, err := decodeProgressCursor(filter.Page.Cursor)
	if err != nil {
		return Page[ProgressBrowseItem]{}, err
	}
	if filter.Page.Cursor != "" && (cursor.Kind != filter.Kind || cursor.State != filter.State) {
		return Page[ProgressBrowseItem]{}, ErrInvalidCursor
	}
	var cursorTime *time.Time
	if !cursor.Updated.IsZero() {
		cursorTime = &cursor.Updated
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly,
	})
	if err != nil {
		return Page[ProgressBrowseItem]{}, fmt.Errorf("BrowseProgress: BeginTx: %w", err)
	}
	defer rollback(ctx, tx)
	rows, err := tx.Query(ctx, `SELECT p.id,p.updated,
	 COALESCE(p.request->>'product',''),COALESCE(p.request->>'version',''),COALESCE(p.request->>'build',''),
	 COALESCE(r.target_id,''),a.id,
	 (p.kind='inspection' AND p.state='running' AND (r.lease_expires IS NULL OR r.lease_expires<=now()))
	 FROM operation_progress p
	 LEFT JOIN analysis_run r ON r.id=p.run_id
	 LEFT JOIN LATERAL (SELECT id FROM acquisition WHERE attempt_id=p.id ORDER BY created DESC,id DESC LIMIT 1) a ON true
	 WHERE ($1='' OR p.kind=$1)
	 AND ($2='' OR ($2='terminal' AND p.state<>'running') OR p.state=$2)
	 AND ($3::timestamptz IS NULL OR (p.updated,p.id)<($3::timestamptz,$4::text))
	 ORDER BY p.updated DESC,p.id DESC LIMIT $5`,
		filter.Kind, filter.State, cursorTime, cursor.ID, limit+1)
	if err != nil {
		return Page[ProgressBrowseItem]{}, fmt.Errorf("BrowseProgress: Query: %w", err)
	}
	type entry struct {
		item  ProgressBrowseItem
		stale bool
	}
	entries := make([]entry, 0, limit+1)
	for rows.Next() {
		var e entry
		if err := rows.Scan(&e.item.Progress.ID, &e.item.Progress.Updated,
			&e.item.Product, &e.item.Version, &e.item.Build,
			&e.item.TargetID, &e.item.AcquisitionID, &e.stale); err != nil {
			rows.Close()
			return Page[ProgressBrowseItem]{}, fmt.Errorf("BrowseProgress: Scan: %w", err)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return Page[ProgressBrowseItem]{}, fmt.Errorf("BrowseProgress: Rows: %w", err)
	}
	rows.Close()
	out := Page[ProgressBrowseItem]{Items: []ProgressBrowseItem{}}
	for _, e := range entries {
		if len(out.Items) == limit {
			out.NextCursor = encodeProgressCursor(out.Items[len(out.Items)-1], filter)
			break
		}
		e.item.Progress, err = progressForBrowse(ctx, tx, e.item.Progress.ID)
		if err != nil {
			return Page[ProgressBrowseItem]{}, fmt.Errorf("BrowseProgress: Progress: %w", err)
		}
		e.item.Stale = e.stale
		e.item.EffectiveState = e.item.Progress.State
		if e.stale {
			e.item.EffectiveState = "stale"
		}
		out.Items = append(out.Items, e.item)
	}
	if err := tx.Commit(ctx); err != nil {
		return Page[ProgressBrowseItem]{}, fmt.Errorf("BrowseProgress: Commit: %w", err)
	}
	return out, nil
}

func progressForBrowse(ctx context.Context, tx pgx.Tx, id string) (model.Progress, error) {
	var p model.Progress
	var runID, inputSHA, fingerprint *string
	err := tx.QueryRow(ctx, `SELECT id,kind,run_id,input_sha256,input_size,
	 source_fingerprint,phase,state,component,completed_bytes,verified_bytes,total_bytes,
	 completed_units,total_units,published_batches,diagnostic_code,sequence,version,
	 started,updated,finished FROM operation_progress WHERE id=$1`, id).Scan(
		&p.ID, &p.Kind, &runID, &inputSHA, &p.InputSize, &fingerprint,
		&p.Phase, &p.State, &p.Component, &p.CompletedBytes, &p.VerifiedBytes, &p.TotalBytes,
		&p.CompletedUnits, &p.TotalUnits, &p.PublishedBatches,
		&p.DiagnosticCode, &p.Sequence, &p.Version, &p.Started, &p.Updated, &p.Finished,
	)
	if err != nil {
		return model.Progress{}, fmt.Errorf("progressForBrowse: Query: %w", err)
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
