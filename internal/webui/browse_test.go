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

func browseTestStore(t *testing.T) *postgres.Store {
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
	schema := "test_ui_browse_" + hex.EncodeToString(nonce[:])
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
	store, err := postgres.Open(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		store.Close()
		_, _ = admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
		_ = admin.Close(context.Background())
	})
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	return store
}

func TestBrowseAPIsKeepIdentityScopeAndSecretsSeparate(t *testing.T) {
	store := browseTestStore(t)
	ctx := context.Background()
	artifact := model.Artifact{
		ID: strings.Repeat("a", 64), SHA256: strings.Repeat("a", 64),
		Size: 100, Path: "/private/secret/cache.ipsw",
	}
	targets := []model.Target{
		{ID: strings.Repeat("1", 64), ArtifactID: artifact.ID, Identity: 0,
			Products: []string{"iPhone12,1"}, Version: "14.0", Build: "18A373"},
		{ID: strings.Repeat("2", 64), ArtifactID: artifact.ID, Identity: 1,
			Products: []string{"iPhone12,1"}, Version: "14.0", Build: "18A373"},
	}
	if err := store.Register(ctx, artifact, targets); err != nil {
		t.Fatal(err)
	}
	request := map[string]any{"selection": map[string]string{
		"product": "iPhone12,1", "build": "18A373",
	}}
	if err := store.Acquisition(ctx,
		"https://user:secret@catalog.example/private-token?key=secret",
		request, "failed", []byte("retained source")); err != nil {
		t.Fatal(err)
	}
	run := model.Run{
		ID: "ui-browse-run", TargetID: targets[0].ID, State: "running",
		Started: time.Now().UTC(), Planned: []string{"manifest", "disk-metadata"},
		Extractor: "fixture/v1",
	}
	if err := store.Start(ctx, run); err != nil {
		t.Fatal(err)
	}
	if err := store.PublishBatch(ctx, run, model.ObservationBatch{
		Component: "manifest", Ordinal: 0,
		Final: &model.Coverage{Component: "manifest", State: "complete"},
	}); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(NewReadHandler(store))
	defer server.Close()
	get := func(path string) (int, map[string]any, string) {
		t.Helper()
		resp, err := server.Client().Get(server.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		var envelope map[string]any
		if err := json.Unmarshal(body, &envelope); err != nil {
			t.Fatalf("%s: %v: %s", path, err, body)
		}
		return resp.StatusCode, envelope, string(body)
	}
	status, envelope, _ := get("/api/v1/targets?product=iPhone12%2C1&build=18A373&limit=1")
	if status != http.StatusOK {
		t.Fatalf("target page status=%d", status)
	}
	page := envelope["data"].(map[string]any)
	first := page["items"].([]any)
	if len(first) != 1 || first[0].(map[string]any)["target"].(map[string]any)["id"] != targets[0].ID {
		t.Fatalf("first exact target page: %+v", page)
	}
	cursor := page["next_cursor"].(string)
	status, envelope, _ = get("/api/v1/targets?product=iPhone12%2C1&build=18A373&limit=1&cursor=" + cursor)
	if status != http.StatusOK ||
		envelope["data"].(map[string]any)["items"].([]any)[0].(map[string]any)["target"].(map[string]any)["id"] != targets[1].ID {
		t.Fatalf("second exact target page: %+v", envelope)
	}
	status, _, body := get("/api/v1/artifacts/" + artifact.ID)
	if status != http.StatusOK || strings.Contains(body, "/private/secret") ||
		!strings.Contains(body, "registered_unverified") {
		t.Fatalf("artifact availability or path leakage: %d %s", status, body)
	}
	status, _, body = get("/api/v1/acquisitions?status=failed")
	if status != http.StatusOK || strings.Contains(body, "secret") ||
		!strings.Contains(body, "catalog.example") ||
		!strings.Contains(body, "search scope is incomplete") ||
		!strings.Contains(body, `"evidence_available":true`) {
		t.Fatalf("acquisition provenance or secret leakage: %d %s", status, body)
	}
	status, _, body = get("/api/v1/runs/" + run.ID)
	if status != http.StatusOK || !strings.Contains(body, `"state":"running"`) ||
		!strings.Contains(body, `"component":"manifest"`) ||
		!strings.Contains(body, `"final_coverage_available":false`) ||
		strings.Contains(body, `"final_coverage":`) {
		t.Fatalf("run scope: %d %s", status, body)
	}
	for _, path := range []string{
		"/api/v1/targets?limit=101", "/api/v1/runs?cursor=bad",
		"/api/v1/targets?unknown=value", "/api/v1/acquisitions?from=bad",
	} {
		if status, _, _ := get(path); status != http.StatusBadRequest {
			t.Fatalf("unbounded/invalid query %s status=%d", path, status)
		}
	}
	if status, _, _ := get("/api/v1/runs/missing"); status != http.StatusNotFound {
		t.Fatalf("missing run status=%d", status)
	}
}
