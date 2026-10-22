package webui

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
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

func monitorTestStores(t *testing.T) (*postgres.Store, *postgres.Store, *pgx.Conn) {
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
	schema := "test_ui_monitor_" + hex.EncodeToString(nonce[:])
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
	return writer, reader, raw
}

func monitorListRequest(t *testing.T, server *httptest.Server, path string) (int, map[string]any, string) {
	t.Helper()
	response, err := server.Client().Get(server.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	var envelope map[string]any
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return response.StatusCode, envelope, string(body)
}

func TestProgressListDiscoversUnpublishedFetchAndStaleRun(t *testing.T) {
	writer, reader, raw := monitorTestStores(t)
	server := httptest.NewServer(NewReadHandler(reader))
	defer server.Close()
	ctx := context.Background()
	attempt, err := writer.StartAcquisitionProgress(ctx, "iPhone12,1", "15.0", "19A346")
	if err != nil {
		t.Fatal(err)
	}
	code, envelope, body := monitorListRequest(t, server, "/api/v1/progress?kind=acquisition&state=running&limit=1")
	if code != http.StatusOK || envelope["state"] != "ready" || envelope["read_only"] != true {
		t.Fatalf("active list: %d %s", code, body)
	}
	items := envelope["data"].(map[string]any)["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["progress"].(map[string]any)["id"] != attempt ||
		items[0].(map[string]any)["acquisition_id"] != nil {
		t.Fatalf("unpublished attempt not discoverable: %s", body)
	}
	if _, err := raw.Exec(ctx, `UPDATE operation_progress SET request=jsonb_set(request,'{source}',to_jsonb('https://user:secret@example.test/private'::text)) WHERE id=$1`, attempt); err != nil {
		t.Fatal(err)
	}
	bytes := int64(7)
	if err := writer.AdvanceAcquisitionProgress(ctx, attempt, postgres.AcquisitionProgressUpdate{
		Phase: "transferring", CompletedBytes: &bytes,
	}); err != nil {
		t.Fatal(err)
	}
	_, _, body = monitorListRequest(t, server, "/api/v1/progress?state=running")
	if strings.Contains(body, "secret") || !strings.Contains(body, `"completed_bytes":7`) {
		t.Fatalf("unsafe request or stale snapshot: %s", body)
	}
	if err := writer.AdvanceAcquisitionProgress(ctx, attempt, postgres.AcquisitionProgressUpdate{
		Phase: "failed", State: "failed", DiagnosticCode: "source_unavailable",
	}); err != nil {
		t.Fatal(err)
	}
	if err := writer.Acquisition(ctx, "https://user:secret@example.test/private",
		map[string]any{"source": "private"}, "failed", []byte("source evidence"), attempt); err != nil {
		t.Fatal(err)
	}
	_, envelope, body = monitorListRequest(t, server, "/api/v1/progress?kind=acquisition&state=terminal")
	items = envelope["data"].(map[string]any)["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["effective_state"] != "failed" ||
		items[0].(map[string]any)["acquisition_id"] == nil || strings.Contains(body, "secret") {
		t.Fatalf("terminal state or source leak: %s", body)
	}
	retry, err := writer.StartAcquisitionProgress(ctx, "iPhone12,1", "15.0", "19A346")
	if err != nil {
		t.Fatal(err)
	}
	_, envelope, body = monitorListRequest(t, server, "/api/v1/progress?kind=acquisition&state=running")
	items = envelope["data"].(map[string]any)["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["progress"].(map[string]any)["id"] != retry {
		t.Fatalf("retry attempt was collapsed with the failed attempt: %s", body)
	}
	if err := writer.AdvanceAcquisitionProgress(ctx, retry, postgres.AcquisitionProgressUpdate{
		Phase: "cancelled", State: "cancelled", DiagnosticCode: "user_cancelled",
	}); err != nil {
		t.Fatal(err)
	}
	_, envelope, body = monitorListRequest(t, server, "/api/v1/progress?kind=acquisition&state=terminal&limit=1")
	page := envelope["data"].(map[string]any)
	items = page["items"].([]any)
	cursor, _ := page["next_cursor"].(string)
	if len(items) != 1 || cursor == "" {
		t.Fatalf("terminal page was not bounded: %s", body)
	}
	_, envelope, body = monitorListRequest(t, server, "/api/v1/progress?kind=acquisition&state=terminal&limit=1&cursor="+url.QueryEscape(cursor))
	if len(envelope["data"].(map[string]any)["items"].([]any)) != 1 {
		t.Fatalf("terminal next page: %s", body)
	}
	artifact := model.Artifact{ID: strings.Repeat("a", 64), SHA256: strings.Repeat("a", 64), Size: 100}
	target := model.Target{ID: strings.Repeat("b", 64), ArtifactID: artifact.ID, Identity: 1,
		Products: []string{"iPhone12,1"}, Build: "19A346", Version: "15.0"}
	if err := writer.Register(ctx, artifact, []model.Target{target}); err != nil {
		t.Fatal(err)
	}
	run := model.Run{ID: "monitor-run", TargetID: target.ID, State: "running", Started: time.Now().UTC(),
		Planned: []string{"manifest"}, Extractor: "fixture/v1", LeaseToken: "monitor-worker"}
	if err := writer.Start(ctx, run); err != nil {
		t.Fatal(err)
	}
	if _, err := raw.Exec(ctx, `UPDATE analysis_run SET lease_expires=now()-interval '1 second' WHERE id=$1`, run.ID); err != nil {
		t.Fatal(err)
	}
	_, envelope, body = monitorListRequest(t, server, "/api/v1/progress?kind=inspection&state=running")
	items = envelope["data"].(map[string]any)["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["effective_state"] != "stale" ||
		items[0].(map[string]any)["target_id"] != target.ID {
		t.Fatalf("stale inspection scope: %s", body)
	}
	for _, path := range []string{
		"/api/v1/progress?kind=unknown", "/api/v1/progress?state=unknown",
		"/api/v1/progress?limit=101", "/api/v1/progress?cursor=broken",
		"/api/v1/progress?kind=acquisition&kind=inspection",
	} {
		code, _, _ := monitorListRequest(t, server, path)
		if code != http.StatusBadRequest {
			t.Fatalf("%s: status %d", path, code)
		}
	}
}
