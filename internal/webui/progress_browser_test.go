package webui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/go-ios-vm/internal/model"
	"github.com/deploymenttheory/go-ios-vm/internal/postgres"
)

// The fallback keeps this stacked PR testable before LIVE-02 lands below it.
// In the temporary integration worktree the same browser flow uses LIVE-02's
// real route, because the base handler returns 200 and bypasses the fallback.
func monitorFixtureHandler(reader *postgres.Store, raw *pgx.Conn) http.Handler {
	base := NewReadHandler(reader)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/progress/events") {
			base.ServeHTTP(w, r)
			return
		}
		recorded := httptest.NewRecorder()
		base.ServeHTTP(recorded, r)
		if recorded.Code != http.StatusNotFound {
			for key, values := range recorded.Header() {
				for _, value := range values {
					w.Header().Add(key, value)
				}
			}
			w.WriteHeader(recorded.Code)
			_, _ = w.Write(recorded.Body.Bytes())
			return
		}
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) != 6 || parts[0] != "api" || parts[1] != "v1" ||
			parts[4] != "progress" || parts[5] != "events" {
			http.NotFound(w, r)
			return
		}
		id := parts[3]
		ctx := r.Context()
		p, err := reader.Progress(ctx, id)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		events, err := reader.ProgressEvents(ctx, id)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		var run *postgres.RunRecord
		stale := false
		if p.Kind == "inspection" {
			value, runErr := reader.BrowseRun(ctx, id)
			if runErr != nil {
				http.NotFound(w, r)
				return
			}
			run = &value
			if p.State == "running" {
				_ = raw.QueryRow(ctx, `SELECT lease_expires IS NULL OR lease_expires<=now() FROM analysis_run WHERE id=$1`, id).Scan(&stale)
			}
		}
		effective := p.State
		if stale {
			effective = "stale"
		}
		since, _ := strconv.ParseInt(r.URL.Query().Get("since"), 10, 64)
		retained := []model.ProgressEvent{}
		next := since
		oldest := int64(0)
		if len(events) != 0 {
			oldest = events[0].Sequence
		}
		for _, event := range events {
			if event.Sequence > since {
				retained = append(retained, event)
				next = event.Sequence
			}
		}
		apiOK(w, r, map[string]any{
			"snapshot": map[string]any{"progress": p, "effective_state": effective,
				"stale": stale, "cursor": strconv.FormatInt(p.Sequence, 10), "run": run},
			"events": retained, "since": strconv.FormatInt(since, 10),
			"next_cursor":     strconv.FormatInt(next, 10),
			"oldest_sequence": strconv.FormatInt(oldest, 10),
			"gap":             oldest > 0 && since < oldest-1, "has_more": false,
		})
	})
}

func monitorBrowserDOM(t *testing.T, browser, pageURL string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, browser,
		"--headless", "--no-sandbox", "--disable-gpu", "--disable-dev-shm-usage",
		"--disable-background-networking", "--no-first-run", "--no-default-browser-check",
		"--user-data-dir="+t.TempDir(), "--virtual-time-budget=5000", "--window-size=900,800", "--dump-dom", pageURL,
	)
	output, err := cmd.Output()
	if err != nil && (ctx.Err() != context.DeadlineExceeded || !strings.Contains(string(output), "</html>")) {
		t.Fatalf("browser: %v; DOM tail: %s", err, tail(string(output), 2000))
	}
	return string(output)
}

func monitorDeepLink(serverURL, id, kind string) string {
	return serverURL + "/?" + url.Values{
		"screen": {"monitor"}, "operation": {id}, "operation_kind": {kind},
	}.Encode()
}

func monitorBrowserScreenshots(t *testing.T, browser, pageURL string) {
	t.Helper()
	directory := os.Getenv("GOIOSVM_BROWSER_SCREENSHOT_DIR")
	if directory == "" {
		return
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, capture := range []struct{ name, size string }{
		{"monitor-desktop.png", "1360,900"}, {"monitor-mobile.png", "500,900"},
	} {
		path := filepath.Join(directory, capture.name)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		cmd := exec.CommandContext(ctx, browser,
			"--headless", "--no-sandbox", "--disable-gpu", "--disable-dev-shm-usage",
			"--disable-background-networking", "--no-first-run", "--no-default-browser-check",
			"--user-data-dir="+t.TempDir(), "--virtual-time-budget=5000",
			"--window-size="+capture.size, "--screenshot="+path, pageURL,
		)
		output, runErr := cmd.CombinedOutput()
		cancel()
		info, statErr := os.Stat(path)
		if statErr != nil || info.Size() == 0 {
			t.Fatalf("capture %s: %v: %s", capture.name, runErr, tail(string(output), 600))
		}
	}
}

// TestBrowserMonitorCommittedTransitions is opt-in because CI does not install
// Chrome. The browser is a second process reading a separate PostgreSQL pool.
func TestBrowserMonitorCommittedTransitions(t *testing.T) {
	browser := os.Getenv("GOIOSVM_BROWSER")
	if browser == "" {
		t.Skip("set GOIOSVM_BROWSER to run the browser smoke test")
	}
	writer, reader, raw := monitorTestStores(t)
	server := httptest.NewServer(monitorFixtureHandler(reader, raw))
	defer server.Close()
	ctx := context.Background()
	attempt, err := writer.StartAcquisitionProgress(ctx, "iPhone12,1", "15.0", "19A346")
	if err != nil {
		t.Fatal(err)
	}
	artifact := model.Artifact{ID: strings.Repeat("a", 64), SHA256: strings.Repeat("a", 64), Size: 100}
	target := model.Target{ID: strings.Repeat("b", 64), ArtifactID: artifact.ID, Identity: 1,
		Products: []string{"iPhone12,1"}, Build: "19A346", Version: "15.0"}
	if err := writer.Register(ctx, artifact, []model.Target{target}); err != nil {
		t.Fatal(err)
	}
	run := model.Run{ID: "monitor-live-run", TargetID: target.ID, State: "running", Started: time.Now().UTC(),
		Planned: []string{"manifest", "disk-metadata"}, Extractor: "fixture/v1", LeaseToken: "monitor-worker"}
	if err := writer.Start(ctx, run); err != nil {
		t.Fatal(err)
	}
	initial := monitorBrowserDOM(t, browser, monitorDeepLink(server.URL, attempt, "acquisition"))
	for _, want := range []string{"iPhone12,1 · 15.0 · 19A346", "resolving", "total unknown", "running"} {
		if !strings.Contains(initial, want) {
			t.Fatalf("running fetch missing %q: %s", want, tail(initial, 1800))
		}
	}
	transferred := int64(7)
	if err := writer.AdvanceAcquisitionProgress(ctx, attempt, postgres.AcquisitionProgressUpdate{
		Phase: "transferring", CompletedBytes: &transferred,
	}); err != nil {
		t.Fatal(err)
	}
	if err := writer.InspectionPhase(ctx, run.ID, run.LeaseToken, "manifest"); err != nil {
		t.Fatal(err)
	}
	if err := writer.PublishBatch(ctx, run, model.ObservationBatch{
		Component: "manifest", Final: &model.Coverage{Component: "manifest", State: "complete"},
	}); err != nil {
		t.Fatal(err)
	}
	updated := monitorBrowserDOM(t, browser, monitorDeepLink(server.URL, run.ID, "inspection"))
	for _, want := range []string{"1 of 2", "1 retained batches", "Committed checkpoints so far", "manifest"} {
		if !strings.Contains(updated, want) {
			t.Fatalf("committed inspection progress missing %q: %s", want, tail(updated, 1800))
		}
	}
	if _, err := raw.Exec(ctx, `UPDATE analysis_run SET lease_expires=now()-interval '1 second' WHERE id=$1`, run.ID); err != nil {
		t.Fatal(err)
	}
	stale := monitorBrowserDOM(t, browser, monitorDeepLink(server.URL, run.ID, "inspection"))
	if !strings.Contains(stale, "The inspection lease expired") || !strings.Contains(stale, "stale") {
		t.Fatalf("worker crash was presented as completed: %s", tail(stale, 1800))
	}
	if _, err := writer.ReconcileStale(ctx); err != nil {
		t.Fatal(err)
	}
	if err := writer.AdvanceAcquisitionProgress(ctx, attempt, postgres.AcquisitionProgressUpdate{
		Phase: "failed", State: "failed", DiagnosticCode: "source_unavailable",
	}); err != nil {
		t.Fatal(err)
	}
	retry, err := writer.StartAcquisitionProgress(ctx, "iPhone12,1", "15.0", "19A346")
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.AdvanceAcquisitionProgress(ctx, retry, postgres.AcquisitionProgressUpdate{
		Phase: "cancelled", State: "cancelled", DiagnosticCode: "user_cancelled",
	}); err != nil {
		t.Fatal(err)
	}
	partial := model.Run{ID: "monitor-partial-run", TargetID: target.ID, State: "running", Started: time.Now().UTC(),
		Planned: []string{"manifest", "disk-metadata"}, Extractor: "fixture/v1", LeaseToken: "partial-worker"}
	if err := writer.Start(ctx, partial); err != nil {
		t.Fatal(err)
	}
	for _, coverage := range []model.Coverage{
		{Component: "manifest", State: "complete"},
		{Component: "disk-metadata", State: "unexamined", Reason: "unsupported codec"},
	} {
		if err := writer.PublishBatch(ctx, partial, model.ObservationBatch{Component: coverage.Component, Final: &coverage}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := writer.Finish(ctx, partial.ID, partial.LeaseToken); err != nil {
		t.Fatal(err)
	}
	terminal := monitorBrowserDOM(t, browser, monitorDeepLink(server.URL, partial.ID, "inspection"))
	for _, want := range []string{"failed", "cancelled", "interrupted", "partial", "Final component coverage", "unsupported codec", "Browse this exact run"} {
		if !strings.Contains(terminal, want) {
			t.Fatalf("terminal monitor missing %q: %s", want, tail(terminal, 1800))
		}
	}
	monitorBrowserScreenshots(t, browser, monitorDeepLink(server.URL, partial.ID, "inspection"))
}
