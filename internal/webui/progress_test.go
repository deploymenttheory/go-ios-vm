package webui

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/go-ios-vm/internal/model"
	"github.com/deploymenttheory/go-ios-vm/internal/postgres"
)

func liveTestStores(t *testing.T) (*postgres.Store, *postgres.Store, *pgx.Conn, string) {
	t.Helper()
	dsn := os.Getenv("GOIOSVM_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set GOIOSVM_TEST_DATABASE_URL for PostgreSQL integration")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	var nonce [8]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		t.Fatal(err)
	}
	schema := "test_ui_live_" + hex.EncodeToString(nonce[:])
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	query := u.Query()
	query.Set("search_path", schema)
	u.RawQuery = query.Encode()
	writer, err := postgres.Open(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	reader, err := postgres.Open(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	raw, err := pgx.Connect(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		reader.Close()
		writer.Close()
		_ = raw.Close(context.Background())
		_, _ = admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
		_ = admin.Close(context.Background())
	})
	return writer, reader, raw, u.String()
}

func liveGet(t *testing.T, handler http.Handler, path string) (int, map[string]any, string) {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8765"+path, nil)
	r.RemoteAddr = "127.0.0.1:4711"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	var envelope map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("%s: %v: %s", path, err, w.Body.String())
	}
	return w.Code, envelope, w.Body.String()
}

func liveData(t *testing.T, handler http.Handler, path string) map[string]any {
	t.Helper()
	status, envelope, body := liveGet(t, handler, path)
	if status != http.StatusOK || envelope["state"] != "ready" || envelope["read_only"] != true {
		t.Fatalf("%s: status=%d %s", path, status, body)
	}
	return envelope["data"].(map[string]any)
}

func TestLiveAcquisitionPollingAndRetainedCursor(t *testing.T) {
	writer, reader, _, dsn := liveTestStores(t)
	ctx := context.Background()
	id, err := writer.StartAcquisitionProgress(ctx, "iPhone12,1", "15.0", "19A346")
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/acquisitions/" + id + "/progress"
	handler := NewReadHandler(reader)
	initial := liveData(t, handler, path)
	progress := initial["progress"].(map[string]any)
	if initial["effective_state"] != "running" || initial["cursor"] != "1" ||
		progress["total_bytes"] != nil || progress["completed_bytes"] != float64(0) {
		t.Fatalf("unknown total or initial state invented: %+v", initial)
	}
	completed := int64(12)
	if err := writer.AdvanceAcquisitionProgress(ctx, id, postgres.AcquisitionProgressUpdate{
		CompletedBytes: &completed,
	}); err != nil {
		t.Fatal(err)
	}
	page := liveData(t, handler, path+"/events?since=1")
	if len(page["events"].([]any)) != 0 || page["next_cursor"] != "1" ||
		page["snapshot"].(map[string]any)["progress"].(map[string]any)["completed_bytes"] != float64(12) ||
		page["snapshot"].(map[string]any)["effective_state"] != "running" {
		t.Fatalf("counter-only update implied success or disappeared: %+v", page)
	}
	for _, phase := range []string{"downloading", "verifying"} {
		if err := writer.AdvanceAcquisitionProgress(ctx, id, postgres.AcquisitionProgressUpdate{
			Phase: phase,
		}); err != nil {
			t.Fatal(err)
		}
	}
	page = liveData(t, handler, path+"/events?since=1&limit=1")
	if !page["has_more"].(bool) || page["next_cursor"] != "2" ||
		len(page["events"].([]any)) != 1 ||
		page["events"].([]any)[0].(map[string]any)["phase"] != "downloading" {
		t.Fatalf("first ordered page: %+v", page)
	}
	page = liveData(t, handler, path+"/events?since=2")
	if page["has_more"].(bool) || page["next_cursor"] != "3" ||
		page["events"].([]any)[0].(map[string]any)["phase"] != "verifying" {
		t.Fatalf("second ordered page: %+v", page)
	}
	for i := range 70 {
		phase := "downloading"
		if i%2 != 0 {
			phase = "verifying"
		}
		if err := writer.AdvanceAcquisitionProgress(ctx, id, postgres.AcquisitionProgressUpdate{
			Phase: phase,
		}); err != nil {
			t.Fatal(err)
		}
	}
	// A fresh store and HTTP handler model a UI process restart. Neither the
	// cursor nor the last snapshot depends on the earlier server's memory.
	reopened, err := postgres.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(reopened.Close)
	handler = NewReadHandler(reopened)
	page = liveData(t, handler, path+"/events?since=1")
	events := page["events"].([]any)
	if !page["gap"].(bool) || len(events) != 64 || page["has_more"].(bool) ||
		page["oldest_sequence"] == "1" || page["next_cursor"] != page["snapshot"].(map[string]any)["cursor"] {
		t.Fatalf("retained history gap is unclear: %+v", page)
	}
	prior := float64(0)
	for _, item := range events {
		sequence := item.(map[string]any)["sequence"].(float64)
		if sequence <= prior {
			t.Fatalf("events unordered: %v after %v", sequence, prior)
		}
		prior = sequence
	}
	if status, _, _ := liveGet(t, handler, path+"/events?since=999999"); status != 400 {
		t.Fatalf("future cursor status=%d", status)
	}
	for _, bad := range []string{"/events?since=-1", "/events?since=abc", "/events?limit=65", "?unknown=1"} {
		if status, _, _ := liveGet(t, handler, path+bad); status != 400 {
			t.Fatalf("invalid query %s status=%d", bad, status)
		}
	}
	if err := writer.AdvanceAcquisitionProgress(ctx, id, postgres.AcquisitionProgressUpdate{
		Phase: "failed", State: "failed", DiagnosticCode: "download_failed",
	}); err != nil {
		t.Fatal(err)
	}
	terminal := liveData(t, handler, path)
	if terminal["effective_state"] != "failed" ||
		terminal["progress"].(map[string]any)["diagnostic_code"] != "download_failed" {
		t.Fatalf("failed attempt lost terminal state: %+v", terminal)
	}
	if status, _, _ := liveGet(t, handler, "/api/v1/runs/"+id+"/progress"); status != 404 {
		t.Fatalf("wrong progress kind status=%d", status)
	}
	for _, method := range []string{http.MethodHead, http.MethodPost} {
		r := httptest.NewRequest(method, "http://127.0.0.1:8765"+path, nil)
		r.RemoteAddr = "127.0.0.1:4711"
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if method == http.MethodHead && (w.Code != http.StatusOK || w.Body.Len() != 0) {
			t.Fatalf("HEAD returned %d and %d bytes", w.Code, w.Body.Len())
		}
		if method == http.MethodPost && w.Code != http.StatusMethodNotAllowed {
			t.Fatalf("POST status=%d", w.Code)
		}
	}
	reopened.Close()
	if status, _, body := liveGet(t, handler, path); status != http.StatusServiceUnavailable ||
		strings.Contains(body, "postgres") || strings.Contains(body, "password") {
		t.Fatalf("database failure leaked details: %d %s", status, body)
	}
}

func TestLiveRunStaleReconciliationAndFinalCoverage(t *testing.T) {
	writer, reader, raw, _ := liveTestStores(t)
	ctx := context.Background()
	artifact := model.Artifact{ID: strings.Repeat("a", 64), SHA256: strings.Repeat("a", 64), Size: 128}
	target := model.Target{
		ID: strings.Repeat("b", 64), ArtifactID: artifact.ID,
		Identity: 1, Products: []string{"iPhone12,1"}, Version: "15.0", Build: "19A346",
	}
	if err := writer.Register(ctx, artifact, []model.Target{target}); err != nil {
		t.Fatal(err)
	}
	run := model.Run{
		ID: "live-run", TargetID: target.ID, State: "running",
		Started: time.Now().UTC(), Planned: []string{"manifest", "disk-metadata"},
		Extractor: "fixture/v1", LeaseToken: "first-worker",
	}
	if err := writer.Start(ctx, run); err != nil {
		t.Fatal(err)
	}
	handler := NewReadHandler(reader)
	path := "/api/v1/runs/" + run.ID + "/progress"
	initial := liveData(t, handler, path)
	if initial["effective_state"] != "running" || initial["stale"] != false ||
		initial["run"].(map[string]any)["final_coverage_available"] != false {
		t.Fatalf("initial run status: %+v", initial)
	}
	tx, err := raw.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `UPDATE operation_progress SET completed_units=1,
	 version=version+1 WHERE id=$1`, run.ID); err != nil {
		t.Fatal(err)
	}
	still := liveData(t, handler, path)
	if still["progress"].(map[string]any)["completed_units"] != float64(0) {
		t.Fatalf("uncommitted work leaked: %+v", still)
	}
	if err := tx.Rollback(ctx); err != nil {
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
	committed := liveData(t, handler, path)
	if committed["progress"].(map[string]any)["completed_units"] != float64(1) ||
		committed["progress"].(map[string]any)["published_batches"] != float64(1) ||
		committed["run"].(map[string]any)["final_coverage_available"] != false {
		t.Fatalf("checkpoint or final coverage invented: %+v", committed)
	}
	if _, err := raw.Exec(ctx, `UPDATE analysis_run SET lease_expires=now()-interval '1 second'
	 WHERE id=$1`, run.ID); err != nil {
		t.Fatal(err)
	}
	stale := liveData(t, NewReadHandler(reader), path)
	if stale["effective_state"] != "stale" || stale["stale"] != true ||
		stale["progress"].(map[string]any)["state"] != "running" ||
		stale["run"].(map[string]any)["state"] != "running" {
		t.Fatalf("expired lease was reconciled or hidden: %+v", stale)
	}
	if _, err := writer.ReconcileStale(ctx); err != nil {
		t.Fatal(err)
	}
	interrupted := liveData(t, handler, path)
	if interrupted["effective_state"] != "interrupted" || interrupted["stale"] != false ||
		interrupted["progress"].(map[string]any)["diagnostic_code"] != "worker_lost" {
		t.Fatalf("reconciled interruption hidden: %+v", interrupted)
	}
	page := liveData(t, handler, path+"/events?since="+committed["cursor"].(string))
	if page["events"].([]any)[0].(map[string]any)["state"] != "interrupted" {
		t.Fatalf("interruption event missing: %+v", page)
	}
	if err := writer.Resume(ctx, run.ID, "second-worker"); err != nil {
		t.Fatal(err)
	}
	run.LeaseToken = "second-worker"
	if err := writer.PublishBatch(ctx, run, model.ObservationBatch{
		Component: "disk-metadata", Final: &model.Coverage{
			Component: "disk-metadata", State: "partial", Reason: "bounded fixture scope",
		},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Finish(ctx, run.ID, run.LeaseToken); err != nil {
		t.Fatal(err)
	}
	final := liveData(t, handler, path)
	ledger := final["run"].(map[string]any)
	if final["effective_state"] != "partial" || ledger["final_coverage_available"] != true ||
		len(ledger["final_coverage"].([]any)) != 2 || final["stale"] != false {
		t.Fatalf("partial terminal coverage absent: %+v", final)
	}
	for _, state := range []string{"failed", "cancelled", "complete"} {
		other := model.Run{
			ID: "live-" + state, TargetID: target.ID, State: "running",
			Started: time.Now().UTC(), Planned: []string{"manifest"},
			Extractor: "fixture/v1", LeaseToken: "worker-" + state,
		}
		if err := writer.Start(ctx, other); err != nil {
			t.Fatal(err)
		}
		if state == "complete" {
			if err := writer.PublishBatch(ctx, other, model.ObservationBatch{
				Component: "manifest", Final: &model.Coverage{Component: "manifest", State: "complete"},
			}); err != nil {
				t.Fatal(err)
			}
			if _, err := writer.Finish(ctx, other.ID, other.LeaseToken); err != nil {
				t.Fatal(err)
			}
		} else if err := writer.FailBatched(ctx, other.ID, state, other.LeaseToken); err != nil {
			t.Fatal(err)
		}
		got := liveData(t, handler, "/api/v1/runs/"+other.ID+"/progress")
		if got["effective_state"] != state || got["progress"].(map[string]any)["state"] != state {
			t.Fatalf("%s collapsed: %+v", state, got)
		}
	}
	if status, _, body := liveGet(t, handler, "/api/v1/runs/missing/progress"); status != http.StatusNotFound || strings.Contains(body, "postgres") {
		t.Fatalf("missing run leaked diagnostic: %d %s", status, body)
	}
}
