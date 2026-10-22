package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/go-ios-vm/internal/model"
)

var ErrInvalidBrowseFilter = errors.New("invalid finding browse filter")

// FindingFilter scopes every result to one selected BuildManifest identity.
// AfterID names the last immutable finding identity seen by the caller.
type FindingFilter struct {
	TargetID, Subject, Property, Component, Basis, Assessment string
	AfterID                                                   string
	Limit                                                     int
}

type FindingSummary struct {
	ID            string `json:"id"`
	TargetID      string `json:"target_id"`
	Subject       string `json:"subject"`
	Property      string `json:"property"`
	RevisionCount int64  `json:"revision_count"`
}

func prefixPattern(value string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(value) + "%"
}

func (s *Store) FindingPage(ctx context.Context, f FindingFilter) ([]FindingSummary, bool, error) {
	if f.TargetID == "" || f.Limit < 1 || f.Limit > 100 {
		return nil, false, ErrInvalidBrowseFilter
	}
	if f.AfterID != "" {
		var exists bool
		if err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM finding
		 WHERE id=$1 AND target_id=$2)`, f.AfterID, f.TargetID).Scan(&exists); err != nil {
			return nil, false, fmt.Errorf("FindingPage: cursor: %w", err)
		}
		if !exists {
			return nil, false, ErrInvalidBrowseFilter
		}
	}
	rows, err := s.pool.Query(ctx, `SELECT f.id,f.target_id,f.subject,f.property,
	 (SELECT count(*) FROM finding_revision r WHERE r.finding_id=f.id)
	 FROM finding f WHERE f.target_id=$1
	 AND f.subject LIKE $2
	 AND f.property LIKE $3
	 AND ($4='' OR EXISTS (
	  SELECT 1 FROM finding_revision r LEFT JOIN finding_basis b ON b.revision_id=r.id
	  JOIN observation o ON o.id=COALESCE(b.observation_id,r.observation_id)
	  JOIN firmware_target t ON t.id=f.target_id
	  WHERE r.finding_id=f.id AND o.document #>> '{source,member}'=t.document->'components'->>$4))
	 AND (($5='' AND $6='') OR EXISTS (SELECT 1 FROM finding_revision r
	  WHERE r.finding_id=f.id AND ($5='' OR r.basis=$5)
	  AND ($6='' OR COALESCE((SELECT e.state FROM finding_assessment_event e
	   WHERE e.revision_id=r.id ORDER BY e.id DESC LIMIT 1),'unreviewed')=$6)))
	 AND ($7='' OR EXISTS(SELECT 1 FROM finding previous
	  WHERE previous.id=$7 AND previous.target_id=$1
	  AND (f.subject,f.property,f.id)>(previous.subject,previous.property,previous.id)))
	 ORDER BY f.subject,f.property,f.id LIMIT $8`, f.TargetID,
		prefixPattern(f.Subject), prefixPattern(f.Property),
		f.Component, f.Basis, f.Assessment, f.AfterID, f.Limit+1)
	if err != nil {
		return nil, false, fmt.Errorf("FindingPage: Query: %w", err)
	}
	defer rows.Close()
	items := []FindingSummary{}
	for rows.Next() {
		var item FindingSummary
		if err := rows.Scan(&item.ID, &item.TargetID, &item.Subject, &item.Property,
			&item.RevisionCount); err != nil {
			return nil, false, fmt.Errorf("FindingPage: Scan: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, false, fmt.Errorf("FindingPage: rows: %w", err)
	}
	more := len(items) > f.Limit
	if more {
		items = items[:f.Limit]
	}
	return items, more, nil
}

func (s *Store) FindingForTarget(
	ctx context.Context,
	targetID, id string,
) (model.FindingChain, error) {
	chains, err := s.findingChains(ctx, targetID, "", id)
	if err != nil {
		return model.FindingChain{}, err
	}
	if len(chains) == 0 {
		return model.FindingChain{}, pgx.ErrNoRows
	}
	return chains[0], nil
}

type ObservationFilter struct {
	TargetID, RunID, Subject, Property, Component, AfterID string
	Limit                                                  int
}

// ObservationPage returns bounded, target-scoped documents in stable ID order.
func (s *Store) ObservationPage(
	ctx context.Context,
	f ObservationFilter,
) ([]model.Observation, bool, error) {
	if f.TargetID == "" || f.Limit < 1 || f.Limit > 100 {
		return nil, false, ErrInvalidBrowseFilter
	}
	rows, err := s.pool.Query(ctx, `SELECT o.document FROM observation o
	 JOIN firmware_target t ON t.id=o.target_id
	 WHERE o.target_id=$1 AND ($2='' OR o.id>$2)
	 AND ($3='' OR EXISTS (SELECT 1 FROM run_observation ro
	  WHERE ro.observation_id=o.id AND ro.run_id=$3))
	 AND o.document->>'subject' LIKE $4
	 AND o.document->>'property' LIKE $5
	 AND ($6='' OR o.document #>> '{source,member}'=t.document->'components'->>$6)
	 ORDER BY o.id LIMIT $7`, f.TargetID, f.AfterID, f.RunID,
		prefixPattern(f.Subject), prefixPattern(f.Property), f.Component, f.Limit+1)
	if err != nil {
		return nil, false, fmt.Errorf("ObservationPage: Query: %w", err)
	}
	defer rows.Close()
	items := []model.Observation{}
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, false, fmt.Errorf("ObservationPage: Scan: %w", err)
		}
		var item model.Observation
		if err := json.Unmarshal(raw, &item); err != nil {
			return nil, false, fmt.Errorf("ObservationPage: document: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, false, fmt.Errorf("ObservationPage: rows: %w", err)
	}
	more := len(items) > f.Limit
	if more {
		items = items[:f.Limit]
	}
	return items, more, nil
}

func (s *Store) ObservationForTarget(
	ctx context.Context,
	targetID, id string,
) (model.Observation, error) {
	var raw []byte
	if err := s.pool.QueryRow(ctx, `SELECT document FROM observation
	 WHERE target_id=$1 AND id=$2`, targetID, id).Scan(&raw); err != nil {
		return model.Observation{}, fmt.Errorf("ObservationForTarget: %w", err)
	}
	var item model.Observation
	if err := json.Unmarshal(raw, &item); err != nil {
		return model.Observation{}, fmt.Errorf("ObservationForTarget: document: %w", err)
	}
	return item, nil
}

type EvidencePreview struct {
	ID             string `json:"id"`
	State          string `json:"state"`
	DeclaredBytes  int64  `json:"declared_bytes"`
	PreviewBytes   int    `json:"preview_bytes"`
	Hex            string `json:"hex,omitempty"`
	Text           string `json:"text,omitempty"`
	Truncated      bool   `json:"truncated"`
	DigestVerified bool   `json:"digest_verified"`
}

// PreviewEvidence checks the full stored length and digest while retaining at
// most maxBytes for the response. The target must reference the blob.
func (s *Store) PreviewEvidence(
	ctx context.Context,
	targetID, id string,
	maxBytes int,
) (EvidencePreview, error) {
	if targetID == "" || len(id) != 64 || maxBytes < 1 || maxBytes > 4096 {
		return EvidencePreview{}, ErrInvalidBrowseFilter
	}
	var size int64
	var complete bool
	err := s.pool.QueryRow(ctx, `SELECT b.size,b.complete FROM evidence_blob b
	 WHERE b.id=$1 AND EXISTS (SELECT 1 FROM observation o
	  WHERE o.evidence_id=b.id AND o.target_id=$2)`, id, targetID).Scan(&size, &complete)
	if err != nil {
		return EvidencePreview{}, fmt.Errorf("PreviewEvidence: owner: %w", err)
	}
	preview := EvidencePreview{ID: id, State: "unavailable", DeclaredBytes: size}
	if !complete {
		return preview, nil
	}
	if size < 0 || size > 128<<20 {
		return EvidencePreview{}, errUnreadableEvidence
	}
	rows, err := s.pool.Query(ctx, `SELECT c.data FROM blob_chunk b
	 JOIN evidence_chunk c ON c.id=b.chunk_id
	 WHERE b.blob_id=$1 ORDER BY b.ordinal`, id)
	if err != nil {
		return EvidencePreview{}, fmt.Errorf("PreviewEvidence: chunks: %w", err)
	}
	defer rows.Close()
	h := sha256.New()
	first := make([]byte, 0, min(int(size), maxBytes))
	var observed int64
	for rows.Next() {
		var chunk []byte
		if err := rows.Scan(&chunk); err != nil {
			return EvidencePreview{}, fmt.Errorf("PreviewEvidence: chunk: %w", err)
		}
		observed += int64(len(chunk))
		if observed > size {
			return EvidencePreview{}, errStoredEvidenceIntegrityMismatch
		}
		if _, err := h.Write(chunk); err != nil {
			return EvidencePreview{}, fmt.Errorf("PreviewEvidence: digest: %w", err)
		}
		if len(first) < maxBytes {
			first = append(first, chunk[:min(len(chunk), maxBytes-len(first))]...)
		}
	}
	if err := rows.Err(); err != nil {
		return EvidencePreview{}, fmt.Errorf("PreviewEvidence: rows: %w", err)
	}
	if observed != size || hex.EncodeToString(h.Sum(nil)) != id {
		return EvidencePreview{}, errStoredEvidenceIntegrityMismatch
	}
	preview.State = "complete"
	preview.PreviewBytes = len(first)
	preview.Hex = hex.EncodeToString(first)
	preview.Truncated = int64(len(first)) < size
	preview.DigestVerified = true
	if utf8.Valid(first) {
		preview.Text = string(first)
	}
	return preview, nil
}
