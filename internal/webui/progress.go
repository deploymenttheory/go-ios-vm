package webui

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/deploymenttheory/go-ios-vm/internal/model"
	"github.com/deploymenttheory/go-ios-vm/internal/postgres"
)

const (
	progressTimeout   = 3 * time.Second
	maxProgressEvents = 64
)

type progressView struct {
	Progress       model.Progress      `json:"progress"`
	EffectiveState string              `json:"effective_state"`
	Stale          bool                `json:"stale"`
	Cursor         string              `json:"cursor"`
	Run            *postgres.RunRecord `json:"run,omitempty"`
}

type progressEventsView struct {
	Snapshot       progressView          `json:"snapshot"`
	Events         []model.ProgressEvent `json:"events"`
	Since          string                `json:"since"`
	NextCursor     string                `json:"next_cursor"`
	OldestSequence string                `json:"oldest_sequence"`
	Gap            bool                  `json:"gap"`
	HasMore        bool                  `json:"has_more"`
}

func progressPublic(status postgres.ProgressStatus) progressView {
	return progressView{
		Progress: status.Progress, EffectiveState: status.EffectiveState,
		Stale: status.Stale, Cursor: strconv.FormatInt(status.Progress.Sequence, 10),
		Run: status.Run,
	}
}

// A cursor is the last event sequence seen for this operation URL. Counter
// updates may have no event, so every event response also includes a snapshot.
func progressEventPage(status postgres.ProgressStatus, since int64, limit int) progressEventsView {
	events := status.Events
	page := progressEventsView{
		Snapshot: progressPublic(status), Events: []model.ProgressEvent{},
		Since: strconv.FormatInt(since, 10), NextCursor: strconv.FormatInt(since, 10),
	}
	if len(events) == 0 {
		return page
	}
	oldest := events[0].Sequence
	page.OldestSequence = strconv.FormatInt(oldest, 10)
	page.Gap = since < oldest-1
	for _, event := range events {
		if event.Sequence <= since {
			continue
		}
		if len(page.Events) == limit {
			page.HasMore = true
			break
		}
		page.Events = append(page.Events, event)
		page.NextCursor = strconv.FormatInt(event.Sequence, 10)
	}
	return page
}

func parseProgressSince(value string) (int64, bool) {
	if len(value) == 0 || len(value) > 19 {
		return 0, false
	}
	for _, c := range value {
		if c < '0' || c > '9' {
			return 0, false
		}
	}
	n, err := strconv.ParseInt(value, 10, 64)
	return n, err == nil
}

func serveProgress(w http.ResponseWriter, r *http.Request, store *postgres.Store) bool {
	if store == nil {
		return false
	}
	path, ok := strings.CutPrefix(r.URL.Path, "/api/v1/")
	if !ok {
		return false
	}
	parts := strings.Split(path, "/")
	if len(parts) < 3 || len(parts) > 4 || parts[2] != "progress" ||
		(parts[0] != "acquisitions" && parts[0] != "runs") ||
		(len(parts) == 4 && parts[3] != "events") {
		return false
	}
	if parts[1] == "" || len(parts[1]) > 128 {
		browseError(w, r, postgres.ErrInvalidBrowsePage)
		return true
	}
	eventsRoute := len(parts) == 4
	if (!eventsRoute && !validQuery(r)) ||
		(eventsRoute && !validQuery(r, "since", "limit")) {
		browseError(w, r, postgres.ErrInvalidBrowsePage)
		return true
	}
	since, limit := int64(0), maxProgressEvents
	if eventsRoute {
		if values, present := r.URL.Query()["since"]; present {
			var valid bool
			since, valid = parseProgressSince(values[0])
			if !valid {
				browseError(w, r, postgres.ErrInvalidCursor)
				return true
			}
		}
		if values, present := r.URL.Query()["limit"]; present {
			var err error
			limit, err = strconv.Atoi(values[0])
			if err != nil || limit < 1 || limit > maxProgressEvents {
				browseError(w, r, postgres.ErrInvalidBrowsePage)
				return true
			}
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), progressTimeout)
	defer cancel()
	status, err := store.ReadProgressStatus(ctx, parts[1])
	if err != nil {
		browseError(w, r, err)
		return true
	}
	if (parts[0] == "acquisitions" && status.Progress.Kind != "acquisition") ||
		(parts[0] == "runs" && status.Progress.Kind != "inspection") {
		apiError(w, r, http.StatusNotFound, "not_found", "Record was not found")
		return true
	}
	if !eventsRoute {
		apiOK(w, r, progressPublic(status))
		return true
	}
	if since > status.Progress.Sequence {
		browseError(w, r, postgres.ErrInvalidCursor)
		return true
	}
	apiOK(w, r, progressEventPage(status, since, limit))
	return true
}
