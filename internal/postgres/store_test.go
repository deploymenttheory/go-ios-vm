package postgres

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/deploymenttheory/go-ios-vm/internal/model"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	dsn := os.Getenv("GOIOSVM_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set GOIOSVM_TEST_DATABASE_URL for PostgreSQL integration")
	}
	ctx := context.Background()
	admin, err := Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	var nonce [8]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		t.Fatal(err)
	}
	schema := "test_" + hex.EncodeToString(nonce[:])
	if _, err = admin.pool.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal("invalid test database URL")
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	store, err := Open(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		store.Close()
		admin.pool.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
		admin.Close()
	})
	if err = store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err = store.Migrate(ctx); err != nil {
		t.Fatal("migration is not idempotent", err)
	}
	return store
}

func TestAtomicEvidencePublicationAndReuse(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	a := model.Artifact{
		ID:     model.Digest([]byte("image")),
		SHA256: model.Digest([]byte("image")),
		Size:   5,
	}
	target := model.Target{ID: model.ID(a.ID, "0"), ArtifactID: a.ID}
	if err := s.Register(ctx, a, []model.Target{target}); err != nil {
		t.Fatal(err)
	}
	source := model.Location{
		ArtifactID: a.ID,
		Member:     "tree",
		Coordinate: "decoded-payload-byte",
		Selector:   "/device/value",
		Length:     1,
	}
	evidence := []byte(strings.Repeat("supporting bytes", 100000))
	o, err := model.NewObservation(
		target,
		"device",
		"value",
		uint64(0),
		source,
		model.NewBlob(evidence),
		"fixture/v1",
	)
	if err != nil {
		t.Fatal(err)
	}
	start := func(id string) model.Run {
		r := model.Run{
			ID:       id,
			TargetID: target.ID,
			Started:  time.Now(),
			State:    "partial",
			Coverage: []model.Coverage{
				{Component: "tree", State: "complete"},
				{Component: "kernel", State: "unexamined"},
			},
		}
		if err := s.Start(ctx, r); err != nil {
			t.Fatal(err)
		}
		return r
	}
	first := start("first")
	bad := o
	bad.TargetID = "wrong"
	if err = s.Publish(ctx, first, []model.Observation{o, bad}); err == nil {
		t.Fatal("wrong target published")
	}
	var count int
	if err = s.pool.QueryRow(ctx, "SELECT count(*) FROM evidence_blob").
		Scan(&count); err != nil ||
		count != 0 {
		t.Fatal("failed transaction leaked evidence", count, err)
	}
	if err = s.Publish(ctx, first, []model.Observation{o}); err != nil {
		t.Fatal(err)
	}
	second := start("second")
	if err = s.Publish(ctx, second, []model.Observation{o}); err != nil {
		t.Fatal(err)
	}
	if err = s.pool.QueryRow(ctx, "SELECT count(*) FROM evidence_blob").
		Scan(&count); err != nil ||
		count != 1 {
		t.Fatal("rerun duplicated evidence", count, err)
	}
	retained, err := s.Evidence(ctx, o.EvidenceID)
	if err != nil || string(retained) != string(evidence) {
		t.Fatal("support bytes not recoverable", err)
	}
	findings, err := s.Findings(ctx, "", "second")
	if err != nil || len(findings) != 1 {
		t.Fatal(findings, err)
	}
	var value uint64
	if err = json.Unmarshal(findings[0].Observation.Value, &value); err != nil || value != 0 {
		t.Fatal("zero value lost")
	}
	forged := start("forged")
	forged.TargetID = "wrong"
	if err = s.Publish(ctx, forged, nil); err == nil {
		t.Fatal("forged run target accepted")
	}
	quota := start("quota")
	s.EvidenceLimit = int64(len(evidence))
	other, err := model.NewObservation(
		target,
		"device",
		"other",
		1,
		source,
		model.NewBlob([]byte("new evidence")),
		"fixture/v2",
	)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Publish(ctx, quota, []model.Observation{other}); err == nil {
		t.Fatal("evidence quota ignored")
	}
	if err = s.Fail(ctx, quota.ID, "failed"); err != nil {
		t.Fatal(err)
	}
	r, err := s.Run(ctx, quota.ID)
	if err != nil || r.State != "failed" {
		t.Fatal(r, err)
	}
}
