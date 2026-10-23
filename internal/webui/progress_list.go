package webui

import (
	"context"
	"net/http"
	"time"

	"github.com/deploymenttheory/go-ios-vm/internal/postgres"
)

func serveProgressList(w http.ResponseWriter, r *http.Request, store *postgres.Store) bool {
	if r.URL.Path != "/api/v1/progress" {
		return false
	}
	if !validQuery(r, "kind", "state", "limit", "cursor") {
		browseError(w, r, postgres.ErrInvalidBrowsePage)
		return true
	}
	page, err := parsePage(r)
	if err != nil {
		browseError(w, r, err)
		return true
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	items, err := store.BrowseProgress(ctx, postgres.ProgressBrowseFilter{
		Kind: r.URL.Query().Get("kind"), State: r.URL.Query().Get("state"), Page: page,
	})
	if err != nil {
		browseError(w, r, err)
		return true
	}
	apiOK(w, r, items)
	return true
}
