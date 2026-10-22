package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/go-ios-vm/internal/postgres"
)

type uiAddressWriter struct{ addresses chan string }

func (w uiAddressWriter) Write(p []byte) (int, error) {
	const prefix = "Inspection UI: "
	if line, ok := strings.CutPrefix(string(p), prefix); ok {
		w.addresses <- strings.TrimSpace(line)
	}
	return len(p), nil
}

func TestUIServeExistingDatabaseAndShutdown(t *testing.T) {
	dsn := os.Getenv("GOIOSVM_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set GOIOSVM_TEST_DATABASE_URL for PostgreSQL integration")
	}
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close(ctx)
	var nonce [8]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		t.Fatal(err)
	}
	schema := "test_ui_" + hex.EncodeToString(nonce[:])
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE") })
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
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	store.Close()
	t.Setenv("GOIOSVM_DATABASE_URL", u.String())
	t.Setenv("GOIOSVM_UI_LISTEN", "0.0.0.0:9876") // Changed flag must take precedence.

	serveCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	addresses := make(chan string, 1)
	var stdout bytes.Buffer
	command := newCommand(&stdout, uiAddressWriter{addresses: addresses})
	command.SetArgs([]string{"ui", "serve", "--listen", "127.0.0.1:0"})
	done := make(chan error, 1)
	go func() { done <- command.ExecuteContext(serveCtx) }()
	var address string
	select {
	case address = <-addresses:
	case err := <-done:
		t.Fatalf("UI stopped before listening: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("UI did not start")
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(address + "api/v1/health")
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil || resp.StatusCode != http.StatusOK ||
		!bytes.Contains(data, []byte(`"state":"ready"`)) {
		t.Fatalf("health: status=%d body=%s err=%v", resp.StatusCode, data, err)
	}
	resp, err = client.Get(address)
	if err != nil {
		t.Fatal(err)
	}
	data, err = io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil || resp.StatusCode != http.StatusOK ||
		!bytes.Contains(data, []byte("Inspection workspace")) {
		t.Fatalf("shell: status=%d body=%s err=%v", resp.StatusCode, data, err)
	}
	var cliOut, cliErr bytes.Buffer
	other := newCommand(&cliOut, &cliErr)
	other.SetArgs([]string{"--format", "json", "finding", "list", "--target", "no-such-target"})
	if err := other.ExecuteContext(ctx); err != nil {
		t.Fatalf("concurrent CLI read: %v (%s)", err, cliErr.String())
	}
	if !bytes.Contains(cliOut.Bytes(), []byte(`"state":"complete"`)) {
		t.Fatalf("concurrent CLI output: %s", cliOut.String())
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("UI command did not stop")
	}
	listener, err := net.Listen("tcp", strings.TrimSuffix(strings.TrimPrefix(address, "http://"), "/"))
	if err != nil {
		t.Fatalf("UI listener remained open: %v", err)
	}
	listener.Close()
}

func TestUIServeRejectsNonLoopbackAndRedactsDatabaseFailure(t *testing.T) {
	secret := "secret-ui-password"
	dsn := "postgres://user:" + secret + "@127.0.0.1:1/evidence?connect_timeout=1"
	for _, tc := range []struct {
		name string
		args []string
		code string
	}{
		{"wildcard", []string{"ui", "serve", "--listen", "0.0.0.0:8765"}, "invalid_invocation"},
		{"remote", []string{"ui", "serve", "--listen", "192.0.2.4:8765"}, "invalid_invocation"},
		{"database", []string{"ui", "serve", "--listen", "127.0.0.1:0"}, "storage_failure"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			args := append([]string{"--format", "json", "--database-url", dsn}, tc.args...)
			if status := runCLI(context.Background(), args, &stdout, &stderr); status != 1 {
				t.Fatalf("exit status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
			}
			if !bytes.Contains(stdout.Bytes(), []byte(`"code":"`+tc.code+`"`)) ||
				bytes.Contains(stdout.Bytes(), []byte(secret)) || bytes.Contains(stderr.Bytes(), []byte(secret)) {
				t.Fatalf("unsafe diagnostic: stdout=%s stderr=%s", stdout.String(), stderr.String())
			}
			if tc.name == "database" &&
				!bytes.Contains(stdout.Bytes(), []byte("evidence database is unavailable")) {
				t.Fatalf("database failure has no visible diagnostic: %s", stdout.String())
			}
		})
	}
}
