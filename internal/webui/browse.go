package webui

import (
	"context"
	"encoding/hex"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/go-ios-vm/internal/postgres"
)

const browseTimeout = 3 * time.Second

type acquisitionView struct {
	postgres.AcquisitionRecord
	SourceOrigin  string `json:"source_origin,omitempty"`
	PartialReason string `json:"partial_reason,omitempty"`
}

func acquisitionPublic(record postgres.AcquisitionRecord) acquisitionView {
	view := acquisitionView{AcquisitionRecord: record}
	parsed, err := url.Parse(record.Source)
	if err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") &&
		parsed.Hostname() != "" {
		view.SourceOrigin = parsed.Scheme + "://" + parsed.Hostname()
	}
	if record.Status == "failed" {
		view.PartialReason = "acquisition source failed; search scope is incomplete"
	}
	return view
}

func apiOK(w http.ResponseWriter, r *http.Request, data any) {
	jsonResponse(w, r, http.StatusOK, response{
		APIVersion: "v1", State: "ready", ReadOnly: true, Data: data,
	})
}

func apiError(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	jsonResponse(w, r, status, response{
		APIVersion: "v1", State: "unavailable", ReadOnly: true,
		Diagnostic: &diagnostic{Code: code, Message: message},
	})
}

func browseError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		apiError(w, r, http.StatusNotFound, "not_found", "Record was not found")
	case errors.Is(err, postgres.ErrInvalidBrowsePage), errors.Is(err, postgres.ErrInvalidCursor):
		apiError(w, r, http.StatusBadRequest, "invalid_query", "Invalid browse query")
	default:
		apiError(
			w, r, http.StatusServiceUnavailable,
			"storage_failure", "Evidence database is unavailable",
		)
	}
}

func validQuery(r *http.Request, allowed ...string) bool {
	keys := make(map[string]bool, len(allowed))
	for _, key := range allowed {
		keys[key] = true
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		return false
	}
	for key, values := range query {
		if !keys[key] || len(values) != 1 || len(values[0]) > 512 {
			return false
		}
	}
	return true
}

func parsePage(r *http.Request) (postgres.PageOptions, error) {
	options := postgres.PageOptions{Cursor: r.URL.Query().Get("cursor")}
	if values, present := r.URL.Query()["limit"]; present {
		value := values[0]
		limit, err := strconv.Atoi(value)
		if err != nil || limit < 1 || limit > postgres.MaxBrowsePage {
			return options, postgres.ErrInvalidBrowsePage
		}
		options.Limit = limit
	}
	return options, nil
}

func parseTime(r *http.Request, key string) (time.Time, error) {
	value := r.URL.Query().Get(key)
	if value == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, postgres.ErrInvalidBrowsePage
	}
	return parsed.UTC(), nil
}

func parseTimeRange(r *http.Request) (*time.Time, *time.Time, error) {
	fromValue, err := parseTime(r, "from")
	if err != nil {
		return nil, nil, err
	}
	beforeValue, err := parseTime(r, "before")
	if err != nil ||
		(!fromValue.IsZero() && !beforeValue.IsZero() && !fromValue.Before(beforeValue)) {
		return nil, nil, postgres.ErrInvalidBrowsePage
	}
	var from, before *time.Time
	if !fromValue.IsZero() {
		from = &fromValue
	}
	if !beforeValue.IsZero() {
		before = &beforeValue
	}
	return from, before, nil
}

func validDigest(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func serveBrowse(w http.ResponseWriter, r *http.Request, store *postgres.Store) bool {
	path, ok := strings.CutPrefix(r.URL.Path, "/api/v1/")
	if !ok {
		return false
	}
	parts := strings.Split(path, "/")
	if len(parts) < 1 || len(parts) > 2 {
		return false
	}
	ctx, cancel := context.WithTimeout(r.Context(), browseTimeout)
	defer cancel()
	switch parts[0] {
	case "artifacts":
		browseArtifacts(ctx, w, r, store, parts)
	case "targets":
		browseTargets(ctx, w, r, store, parts)
	case "acquisitions":
		browseAcquisitions(ctx, w, r, store, parts)
	case "runs":
		browseRuns(ctx, w, r, store, parts)
	default:
		return false
	}
	return true
}

func browseArtifacts(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	store *postgres.Store,
	parts []string,
) {
	if len(parts) == 2 {
		if !validDigest(parts[1]) || !validQuery(r) {
			browseError(w, r, postgres.ErrInvalidBrowsePage)
			return
		}
		item, err := store.BrowseArtifact(ctx, parts[1])
		if err != nil {
			browseError(w, r, err)
			return
		}
		apiOK(w, r, item)
		return
	}
	if !validQuery(r, "sha256", "limit", "cursor") {
		browseError(w, r, postgres.ErrInvalidBrowsePage)
		return
	}
	sha := r.URL.Query().Get("sha256")
	if sha != "" && !validDigest(sha) {
		browseError(w, r, postgres.ErrInvalidBrowsePage)
		return
	}
	page, err := parsePage(r)
	if err == nil && page.Cursor != "" && !validDigest(page.Cursor) {
		err = postgres.ErrInvalidCursor
	}
	if err == nil {
		result, queryErr := store.BrowseArtifacts(ctx, sha, page)
		if queryErr == nil {
			apiOK(w, r, result)
			return
		}
		err = queryErr
	}
	browseError(w, r, err)
}

func browseTargets(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	store *postgres.Store,
	parts []string,
) {
	if len(parts) == 2 {
		if parts[1] == "" || len(parts[1]) > 128 || !validQuery(r) {
			browseError(w, r, postgres.ErrInvalidBrowsePage)
			return
		}
		item, err := store.BrowseTarget(ctx, parts[1])
		if err != nil {
			browseError(w, r, err)
			return
		}
		apiOK(w, r, item)
		return
	}
	if !validQuery(r, "artifact_id", "product", "build", "identity", "limit", "cursor") {
		browseError(w, r, postgres.ErrInvalidBrowsePage)
		return
	}
	filter := postgres.TargetFilter{
		ArtifactID: r.URL.Query().Get("artifact_id"),
		Product:    r.URL.Query().Get("product"),
		Build:      r.URL.Query().Get("build"),
	}
	if value := r.URL.Query().Get("identity"); value != "" {
		identity, err := strconv.Atoi(value)
		if err != nil || identity < 0 {
			browseError(w, r, postgres.ErrInvalidBrowsePage)
			return
		}
		filter.Identity = &identity
	}
	var err error
	filter.Page, err = parsePage(r)
	if err == nil && filter.Page.Cursor != "" && len(filter.Page.Cursor) > 128 {
		err = postgres.ErrInvalidCursor
	}
	if err == nil {
		result, queryErr := store.BrowseTargets(ctx, filter)
		if queryErr == nil {
			apiOK(w, r, result)
			return
		}
		err = queryErr
	}
	browseError(w, r, err)
}

func browseAcquisitions(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	store *postgres.Store,
	parts []string,
) {
	if len(parts) == 2 {
		if !validQuery(r) {
			browseError(w, r, postgres.ErrInvalidBrowsePage)
			return
		}
		id, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil || id <= 0 {
			browseError(w, r, postgres.ErrInvalidBrowsePage)
			return
		}
		item, err := store.BrowseAcquisition(ctx, id)
		if err != nil {
			browseError(w, r, err)
			return
		}
		apiOK(w, r, acquisitionPublic(item))
		return
	}
	if !validQuery(r, "product", "build", "status", "from", "before", "limit", "cursor") {
		browseError(w, r, postgres.ErrInvalidBrowsePage)
		return
	}
	from, before, err := parseTimeRange(r)
	if err != nil {
		browseError(w, r, err)
		return
	}
	page, err := parsePage(r)
	if err != nil {
		browseError(w, r, err)
		return
	}
	result, err := store.BrowseAcquisitions(ctx, postgres.AcquisitionFilter{
		Product: r.URL.Query().Get("product"), Build: r.URL.Query().Get("build"),
		Status: r.URL.Query().Get("status"), From: from, Before: before, Page: page,
	})
	if err != nil {
		browseError(w, r, err)
		return
	}
	items := make([]acquisitionView, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, acquisitionPublic(item))
	}
	apiOK(w, r, postgres.Page[acquisitionView]{Items: items, NextCursor: result.NextCursor})
}

func browseRuns(
	ctx context.Context,
	w http.ResponseWriter,
	r *http.Request,
	store *postgres.Store,
	parts []string,
) {
	if len(parts) == 2 {
		if parts[1] == "" || len(parts[1]) > 128 || !validQuery(r) {
			browseError(w, r, postgres.ErrInvalidBrowsePage)
			return
		}
		item, err := store.BrowseRun(ctx, parts[1])
		if err != nil {
			browseError(w, r, err)
			return
		}
		apiOK(w, r, item)
		return
	}
	if !validQuery(r, "target_id", "state", "from", "before", "limit", "cursor") {
		browseError(w, r, postgres.ErrInvalidBrowsePage)
		return
	}
	from, before, err := parseTimeRange(r)
	if err != nil {
		browseError(w, r, err)
		return
	}
	page, err := parsePage(r)
	if err != nil {
		browseError(w, r, err)
		return
	}
	result, err := store.BrowseRuns(ctx, postgres.RunFilter{
		TargetID: r.URL.Query().Get("target_id"), State: r.URL.Query().Get("state"),
		From: from, Before: before, Page: page,
	})
	if err != nil {
		browseError(w, r, err)
		return
	}
	apiOK(w, r, result)
}
