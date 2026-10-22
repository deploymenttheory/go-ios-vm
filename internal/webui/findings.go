package webui

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/go-ios-vm/internal/postgres"
)

type resultCursor struct {
	Scope   string `json:"scope"`
	Filters string `json:"filters"`
	ID      string `json:"id"`
}

type resultList struct {
	Items      any    `json:"items"`
	NextCursor string `json:"next_cursor,omitempty"`
}

func filterDigest(parts ...string) string {
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])
}

func encodeResultCursor(c resultCursor) string {
	b, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(b)
}

func decodeResultCursor(raw, scope, filters string) (resultCursor, bool) {
	if raw == "" {
		return resultCursor{}, true
	}
	if len(raw) > 8192 {
		return resultCursor{}, false
	}
	b, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return resultCursor{}, false
	}
	var c resultCursor
	if json.Unmarshal(b, &c) != nil || c.Scope != scope || c.Filters != filters || c.ID == "" {
		return resultCursor{}, false
	}
	return c, true
}

func resultError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	apiError(w, r, status, code, message)
}

func resultStorageError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, pgx.ErrNoRows) {
		resultError(w, r, http.StatusNotFound, "not_found", "Result was not found for this target")
		return
	}
	if errors.Is(err, postgres.ErrInvalidBrowseFilter) {
		resultError(w, r, http.StatusBadRequest, "invalid_request", "Invalid result filter")
		return
	}
	resultError(w, r, http.StatusServiceUnavailable,
		"storage_failure", "Evidence database is unavailable")
}

func resultOK(w http.ResponseWriter, r *http.Request, data any) {
	apiOK(w, r, data)
}

func resultLimit(raw string, defaultValue, ceiling int) (int, bool) {
	if raw == "" {
		return defaultValue, true
	}
	n, err := strconv.Atoi(raw)
	return n, err == nil && n >= 1 && n <= ceiling
}

func serveFindings(w http.ResponseWriter, r *http.Request, store *postgres.Store) bool {
	path := r.URL.Path
	if path != "/api/v1/findings" && !strings.HasPrefix(path, "/api/v1/findings/") &&
		path != "/api/v1/observations" && !strings.HasPrefix(path, "/api/v1/observations/") &&
		!strings.HasPrefix(path, "/api/v1/evidence/") {
		return false
	}
	if store == nil {
		resultError(w, r, http.StatusServiceUnavailable,
			"storage_failure", "Evidence database is unavailable")
		return true
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	switch {
	case path == "/api/v1/findings":
		serveFindingList(ctx, w, r, store)
	case strings.HasPrefix(path, "/api/v1/findings/"):
		serveFindingDetail(ctx, w, r, store, strings.TrimPrefix(path, "/api/v1/findings/"))
	case path == "/api/v1/observations":
		serveObservationList(ctx, w, r, store)
	case strings.HasPrefix(path, "/api/v1/observations/"):
		serveObservationDetail(ctx, w, r, store, strings.TrimPrefix(path, "/api/v1/observations/"))
	default:
		serveEvidencePreview(ctx, w, r, store, strings.TrimPrefix(path, "/api/v1/evidence/"))
	}
	return true
}

func serveFindingList(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	store *postgres.Store,
) {
	if !validQuery(
		r,
		"target_id",
		"subject",
		"property",
		"component",
		"basis",
		"assessment",
		"limit",
		"cursor",
	) {
		resultError(
			w,
			r,
			http.StatusBadRequest,
			"invalid_request",
			"Unknown or repeated result filter",
		)
		return
	}
	q := r.URL.Query()
	limit, ok := resultLimit(q.Get("limit"), 25, 100)
	if !ok || q.Get("target_id") == "" {
		resultError(
			w,
			r,
			http.StatusBadRequest,
			"invalid_request",
			"Select a target and a limit of 1 to 100",
		)
		return
	}
	filter := postgres.FindingFilter{
		TargetID: q.Get("target_id"),
		Subject:  q.Get("subject"),
		Property: q.Get("property"),
		Component: q.Get(
			"component",
		),
		Basis:      q.Get("basis"),
		Assessment: q.Get("assessment"),
		Limit:      limit,
	}
	signature := filterDigest(
		filter.TargetID,
		filter.Subject,
		filter.Property,
		filter.Component,
		filter.Basis,
		filter.Assessment,
	)
	cursor, ok := decodeResultCursor(q.Get("cursor"), "findings", signature)
	if !ok {
		resultError(
			w,
			r,
			http.StatusBadRequest,
			"invalid_cursor",
			"Cursor does not match these filters",
		)
		return
	}
	filter.AfterID = cursor.ID
	items, more, err := store.FindingPage(ctx, filter)
	if err != nil {
		resultStorageError(w, r, err)
		return
	}
	page := resultList{Items: items}
	if more {
		last := items[len(items)-1]
		page.NextCursor = encodeResultCursor(resultCursor{
			Scope: "findings", Filters: signature, ID: last.ID,
		})
	}
	resultOK(w, r, page)
}

func serveFindingDetail(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	store *postgres.Store,
	id string,
) {
	if !validQuery(r, "target_id") || r.URL.Query().Get("target_id") == "" || id == "" ||
		strings.Contains(id, "/") {
		resultError(w, r, http.StatusBadRequest, "invalid_request", "Select one finding and target")
		return
	}
	chain, err := store.FindingForTarget(ctx, r.URL.Query().Get("target_id"), id)
	if err != nil {
		resultStorageError(w, r, err)
		return
	}
	resultOK(w, r, chain)
}

func serveObservationList(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	store *postgres.Store,
) {
	if !validQuery(
		r,
		"target_id",
		"run_id",
		"subject",
		"property",
		"component",
		"limit",
		"cursor",
	) {
		resultError(
			w,
			r,
			http.StatusBadRequest,
			"invalid_request",
			"Unknown or repeated result filter",
		)
		return
	}
	q := r.URL.Query()
	limit, ok := resultLimit(q.Get("limit"), 25, 100)
	if !ok || q.Get("target_id") == "" {
		resultError(
			w,
			r,
			http.StatusBadRequest,
			"invalid_request",
			"Select a target and a limit of 1 to 100",
		)
		return
	}
	filter := postgres.ObservationFilter{
		TargetID: q.Get("target_id"), RunID: q.Get("run_id"), Subject: q.Get("subject"),
		Property: q.Get("property"), Component: q.Get("component"), Limit: limit,
	}
	signature := filterDigest(
		filter.TargetID,
		filter.RunID,
		filter.Subject,
		filter.Property,
		filter.Component,
	)
	cursor, ok := decodeResultCursor(q.Get("cursor"), "observations", signature)
	if !ok {
		resultError(
			w,
			r,
			http.StatusBadRequest,
			"invalid_cursor",
			"Cursor does not match these filters",
		)
		return
	}
	filter.AfterID = cursor.ID
	items, more, err := store.ObservationPage(ctx, filter)
	if err != nil {
		resultStorageError(w, r, err)
		return
	}
	page := resultList{Items: items}
	if more {
		page.NextCursor = encodeResultCursor(resultCursor{
			Scope: "observations", Filters: signature, ID: items[len(items)-1].ID,
		})
	}
	resultOK(w, r, page)
}

func serveObservationDetail(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	store *postgres.Store,
	id string,
) {
	if !validQuery(r, "target_id") || r.URL.Query().Get("target_id") == "" || id == "" ||
		strings.Contains(id, "/") {
		resultError(
			w,
			r,
			http.StatusBadRequest,
			"invalid_request",
			"Select one observation and target",
		)
		return
	}
	item, err := store.ObservationForTarget(ctx, r.URL.Query().Get("target_id"), id)
	if err != nil {
		resultStorageError(w, r, err)
		return
	}
	resultOK(w, r, item)
}

func serveEvidencePreview(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	store *postgres.Store,
	path string,
) {
	if !validQuery(r, "target_id", "bytes") || r.URL.Query().Get("target_id") == "" ||
		!strings.HasSuffix(path, "/preview") {
		resultError(
			w,
			r,
			http.StatusBadRequest,
			"invalid_request",
			"Select evidence, target and preview size",
		)
		return
	}
	id := strings.TrimSuffix(path, "/preview")
	if strings.Contains(id, "/") {
		resultError(w, r, http.StatusBadRequest, "invalid_request", "Invalid evidence digest")
		return
	}
	maxBytes, ok := resultLimit(r.URL.Query().Get("bytes"), 256, 4096)
	if !ok {
		resultError(
			w,
			r,
			http.StatusBadRequest,
			"invalid_request",
			"Preview must be 1 to 4096 bytes",
		)
		return
	}
	preview, err := store.PreviewEvidence(ctx, r.URL.Query().Get("target_id"), id, maxBytes)
	if err != nil {
		resultStorageError(w, r, err)
		return
	}
	resultOK(w, r, preview)
}
