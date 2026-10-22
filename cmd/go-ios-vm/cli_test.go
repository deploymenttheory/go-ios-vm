package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/go-ios-vm/internal/model"
	"github.com/deploymenttheory/go-ios-vm/internal/testfixture"
)

func TestHelpAndConfigDoNotNeedDatabase(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"--format", "bad", "cache", "status"}} {
		var out, errout bytes.Buffer
		c := newCommand(&out, &errout)
		c.SetArgs(args)
		err := c.ExecuteContext(context.Background())
		if args[0] == "--help" && err != nil {
			t.Fatal(err)
		}
		if args[0] == "--format" && err == nil {
			t.Fatal("bad format accepted")
		}
	}
}

func TestPersistRestartCompareAndRemoveSource(t *testing.T) {
	dsn := os.Getenv("GOIOSVM_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("PostgreSQL integration")
	}
	admin, err := pgx.Connect(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	var nonce [8]byte
	rand.Read(nonce[:])
	schema := "cli_test_" + hex.EncodeToString(nonce[:])
	if _, err = admin.Exec(context.Background(), "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
		admin.Close(context.Background())
	})
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal("invalid test URL")
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	t.Setenv("GOIOSVM_DATABASE_URL", u.String())
	cache := t.TempDir()
	t.Setenv("GOIOSVM_CACHE_DIR", cache)
	invoke := func(args ...string) (json.RawMessage, error) {
		t.Helper()
		var out, errout bytes.Buffer
		c := newCommand(&out, &errout)
		c.SetArgs(append([]string{"--format", "json"}, args...))
		err := c.ExecuteContext(context.Background())
		if out.Len() == 0 {
			return nil, err
		}
		var e struct{ Results json.RawMessage }
		if decodeErr := json.Unmarshal(out.Bytes(), &e); decodeErr != nil {
			t.Fatal(decodeErr, out.String())
		}
		return e.Results, err
	}
	if _, err := invoke("db", "migrate"); err != nil {
		t.Fatal(err)
	}
	var runs []model.Run
	for i, version := range []string{"14.0", "15.0"} {
		path := filepath.Join(cache, version+".ipsw")
		if err := testfixture.IPSW(path, version, uint64(i)); err != nil {
			t.Fatal(err)
		}
		b, err := invoke("inspect", "--image", path, "--identity", "0")
		if _, ok := err.(*partialError); !ok {
			t.Fatalf("expected truthful partial coverage, got %v", err)
		}
		var run model.Run
		if err = json.Unmarshal(b, &run); err != nil {
			t.Fatal(err)
		}
		runs = append(runs, run)
		if err = os.Remove(path); err != nil {
			t.Fatal(err)
		}
	}
	b, err := invoke("finding", "list", "--run", runs[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	var findings []model.Finding
	if err = json.Unmarshal(b, &findings); err != nil || len(findings) == 0 {
		t.Fatal(err)
	}
	if _, err = invoke(
		"finding",
		"show",
		"--evidence",
		findings[0].Observation.EvidenceID,
	); err != nil {
		t.Fatal("source removal destroyed retained evidence", err)
	}
	if _, err = invoke("compare", "--left", runs[0].ID, "--right", runs[1].ID); err != nil {
		t.Fatal(err)
	}
}
