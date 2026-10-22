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

func resultTestStore(t *testing.T) *postgres.Store {
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
	var entropy [8]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		t.Fatal(err)
	}
	schema := "test_ui_" + hex.EncodeToString(entropy[:])
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
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

func resultRequest(t *testing.T, store *postgres.Store, path string) (int, []byte) {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8765"+path, nil)
	r.RemoteAddr = "127.0.0.1:54321"
	w := httptest.NewRecorder()
	NewReadHandler(store).ServeHTTP(w, r)
	return w.Code, w.Body.Bytes()
}

func TestFindingHTTPBrowsePreviewAndCursor(t *testing.T) {
	store := resultTestStore(t)
	ctx := context.Background()
	data := []byte("example fixture archive")
	artifact := model.Artifact{
		ID:     model.Digest(data),
		SHA256: model.Digest(data),
		Size:   int64(len(data)),
	}
	target := model.Target{
		ID:         model.ID(artifact.ID, "0"),
		ArtifactID: artifact.ID,
		Products: []string{
			"iPhone12,1",
		},
		Build:      "fixture",
		Components: map[string]string{"DeviceTree": "tree"},
	}
	if err := store.Register(ctx, artifact, []model.Target{target}); err != nil {
		t.Fatal(err)
	}
	source := model.Location{
		ArtifactID: artifact.ID,
		Member:     "tree",
		Coordinate: "plist-selector",
		Selector:   "/device/value",
	}
	evidence := []byte("retained support <script>alert(1)</script>")
	observation, err := model.NewObservation(
		target,
		"device",
		"register",
		uint64(18446744073709551615),
		source,
		model.NewBlob(evidence),
		"fixture/v1",
	)
	if err != nil {
		t.Fatal(err)
	}
	run := model.Run{
		ID:       model.ID("ui-result", observation.ID),
		TargetID: target.ID,
		State:    "complete",
		Started:  time.Now(),
		Coverage: []model.Coverage{{Component: "DeviceTree", State: "complete"}},
	}
	if err := store.Start(ctx, run); err != nil {
		t.Fatal(err)
	}
	if err := store.Publish(ctx, run, []model.Observation{observation}); err != nil {
		t.Fatal(err)
	}
	findings, err := store.FindingChains(ctx, target.ID, "")
	if err != nil || len(findings) != 1 {
		t.Fatalf("fixture findings: %+v %v", findings, err)
	}
	query := url.Values{"target_id": {target.ID}, "component": {"DeviceTree"}}
	status, body := resultRequest(t, store, "/api/v1/findings?"+query.Encode())
	if status != http.StatusOK || !strings.Contains(string(body), findings[0].ID) {
		t.Fatalf("finding list: %d %s", status, body)
	}
	status, body = resultRequest(
		t,
		store,
		"/api/v1/findings/"+findings[0].ID+"?target_id="+target.ID,
	)
	if status != http.StatusOK || !strings.Contains(string(body), `"assessment":"unreviewed"`) ||
		!strings.Contains(string(body), `18446744073709551615`) {
		t.Fatalf("finding detail: %d %s", status, body)
	}
	_, err = store.AddRevision(ctx, model.RevisionDraft{
		TargetID: target.ID, Subject: "device", Property: "alternate",
		Basis: "inferred", Presence: "present", Value: []byte(`1`),
		Rationale: "derived from retained source", Author: "fixture",
		References: []model.BasisReference{{Role: "supporting", ObservationID: observation.ID}},
	})
	if err != nil {
		t.Fatal(err)
	}
	status, body = resultRequest(t, store, "/api/v1/findings?target_id="+target.ID+"&limit=1")
	var firstPage struct {
		Data struct {
			Items      []postgres.FindingSummary `json:"items"`
			NextCursor string                    `json:"next_cursor"`
		} `json:"data"`
	}
	if status != http.StatusOK || json.Unmarshal(body, &firstPage) != nil ||
		len(firstPage.Data.Items) != 1 || firstPage.Data.NextCursor == "" {
		t.Fatalf("first finding cursor page: %d %s", status, body)
	}
	status, body = resultRequest(t, store, "/api/v1/findings?target_id="+target.ID+
		"&limit=1&cursor="+url.QueryEscape(firstPage.Data.NextCursor))
	if status != http.StatusOK || strings.Contains(string(body), firstPage.Data.Items[0].ID) {
		t.Fatalf("finding cursor repeated first item: %d %s", status, body)
	}
	status, body = resultRequest(t, store, "/api/v1/observations?target_id="+target.ID+"&limit=1")
	if status != http.StatusOK || !strings.Contains(string(body), observation.ID) {
		t.Fatalf("observation list: %d %s", status, body)
	}
	status, body = resultRequest(t, store, "/api/v1/evidence/"+observation.EvidenceID+
		"/preview?target_id="+target.ID+"&bytes=8")
	if status != http.StatusOK || !strings.Contains(string(body), `"digest_verified":true`) ||
		!strings.Contains(string(body), `"preview_bytes":8`) {
		t.Fatalf("bounded preview: %d %s", status, body)
	}
	status, body = resultRequest(t, store, "/api/v1/evidence/"+observation.EvidenceID+
		"/preview?target_id="+target.ID+"&bytes=4096")
	if status != http.StatusOK || strings.Contains(string(body), "<script>") ||
		!strings.Contains(string(body), `\u003cscript\u003e`) {
		t.Fatalf("text preview was not escaped: %d %s", status, body)
	}
	status, body = resultRequest(t, store, "/api/v1/evidence/"+observation.EvidenceID+
		"/preview?target_id=other&bytes=8")
	if status != http.StatusNotFound || strings.Contains(string(body), observation.EvidenceID) {
		t.Fatalf("cross-target preview: %d %s", status, body)
	}
	status, body = resultRequest(t, store, "/api/v1/findings?target_id="+target.ID+"&cursor=wrong")
	if status != http.StatusBadRequest ||
		!strings.Contains(string(body), `"code":"invalid_cursor"`) {
		t.Fatalf("invalid cursor: %d %s", status, body)
	}
}
