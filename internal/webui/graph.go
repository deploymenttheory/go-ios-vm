package webui

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/go-ios-vm/internal/postgres"
)

func serveGraph(w http.ResponseWriter, r *http.Request, store *postgres.Store) bool {
	if r.URL.Path != "/api/v1/graph" {
		return false
	}
	if store == nil {
		apiError(w, r, http.StatusServiceUnavailable, "storage_failure",
			"Evidence database is unavailable")
		return true
	}
	if !validQuery(r, "target_id", "node_id", "edge_kind", "limit") {
		apiError(w, r, http.StatusBadRequest, "invalid_request", "Unknown or repeated graph filter")
		return true
	}
	q := r.URL.Query()
	limit, ok := resultLimit(q.Get("limit"), 25, 100)
	if !ok || q.Get("target_id") == "" || len(q.Get("target_id")) > 256 ||
		len(q.Get("edge_kind")) > 80 || len(q.Get("node_id")) > 8192 ||
		strings.ContainsAny(q.Get("edge_kind"), " \t\n") {
		apiError(w, r, http.StatusBadRequest, "invalid_request",
			"Select a target and a limit of 1 to 100")
		return true
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	graph, err := store.GraphNeighborhood(ctx, postgres.GraphQuery{
		TargetID: q.Get("target_id"), NodeID: q.Get("node_id"),
		EdgeKind: q.Get("edge_kind"), Limit: limit,
	})
	if errors.Is(err, postgres.ErrInvalidGraphRequest) {
		apiError(w, r, http.StatusBadRequest, "invalid_request", "Invalid graph selection")
		return true
	}
	if errors.Is(err, pgx.ErrNoRows) {
		apiError(w, r, http.StatusNotFound, "not_found", "Graph node was not found for this target")
		return true
	}
	if err != nil {
		apiError(w, r, http.StatusServiceUnavailable, "storage_failure",
			"Evidence database is unavailable")
		return true
	}
	apiOK(w, r, graph)
	return true
}
