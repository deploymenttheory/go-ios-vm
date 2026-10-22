package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	lzfse "github.com/blacktop/lzfse-cgo"
	"github.com/jackc/pgx/v5"

	"github.com/deploymenttheory/go-ios-vm/internal/acquisition"
	"github.com/deploymenttheory/go-ios-vm/internal/model"
	"github.com/deploymenttheory/go-ios-vm/internal/testfixture"
)

func TestCLICancelledNativeDecodeKeepsRunInterrupted(t *testing.T) {
	dsn := os.Getenv("GOIOSVM_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("PostgreSQL integration")
	}
	admin, err := pgx.Connect(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	var nonce [8]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		t.Fatal(err)
	}
	schema := "native_test_" + hex.EncodeToString(nonce[:])
	if _, err := admin.Exec(context.Background(), "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
		admin.Close(context.Background())
	})
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	t.Setenv("GOIOSVM_DATABASE_URL", u.String())
	t.Setenv("GOIOSVM_CACHE_DIR", t.TempDir())
	var stdout, stderr bytes.Buffer
	if code := runCLI(context.Background(), []string{"db", "migrate"}, &stdout, &stderr); code != 0 {
		t.Fatalf("migrate exit %d: %s", code, stderr.String())
	}
	path := filepath.Join(t.TempDir(), "compressed-tree.ipsw")
	makeCompressedTreeIPSW(t, path)
	pidFile := filepath.Join(t.TempDir(), "worker.pid")
	t.Setenv("GOIOSVM_NATIVE_TEST_MODE", "stall:"+pidFile)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stdout.Reset()
	stderr.Reset()
	finished := make(chan int, 1)
	go func() {
		finished <- runCLI(ctx, []string{"--format", "json", "inspect", "--image", path,
			"--identity", "0"}, &stdout, &stderr)
	}()
	deadline := time.After(5 * time.Second)
	for {
		if _, err := os.Stat(pidFile); err == nil {
			break
		}
		select {
		case code := <-finished:
			t.Fatalf("inspection exited before worker, code %d: %s", code, stderr.String())
		case <-deadline:
			t.Fatal("native decode did not start")
		case <-time.After(10 * time.Millisecond):
		}
	}
	cancel()
	select {
	case code := <-finished:
		if code != 130 {
			t.Fatalf("cancel exit %d: %s", code, stderr.String())
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cancelled CLI did not return")
	}
	conn, err := pgx.Connect(context.Background(), u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(context.Background())
	var state string
	var checkpoints, coverage int
	if err := conn.QueryRow(context.Background(), `SELECT state,
	 (SELECT count(*) FROM run_checkpoint WHERE run_id=analysis_run.id),
	 (SELECT count(*) FROM run_coverage WHERE run_id=analysis_run.id)
	 FROM analysis_run`).Scan(&state, &checkpoints, &coverage); err != nil {
		t.Fatal(err)
	}
	if state != "cancelled" || checkpoints != 1 || coverage != 0 {
		t.Fatalf("cancelled run published as complete: %s, checkpoints=%d, coverage=%d",
			state, checkpoints, coverage)
	}
	// A worker-side resource failure is a retained partial component, while
	// cancellation above leaves the run interrupted without final coverage.
	t.Setenv("GOIOSVM_NATIVE_TEST_MODE", "resource")
	stdout.Reset()
	stderr.Reset()
	if code := runCLI(context.Background(), []string{
		"--format", "json", "inspect", "--image", path, "--identity", "0",
	}, &stdout, &stderr); code != 2 {
		t.Fatalf("resource failure exit %d: %s", code, stderr.String())
	}
	var response struct {
		State   string    `json:"state"`
		Results model.Run `json:"results"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.State != "partial" || response.Results.State != "partial" {
		t.Fatalf("worker failure reported complete: %+v", response)
	}
	foundResource := false
	for _, c := range response.Results.Coverage {
		if c.Component == "DeviceTree" && c.State == "partial" &&
			bytes.Contains([]byte(c.Reason), []byte("worker resource limit")) {
			foundResource = true
		}
	}
	if !foundResource {
		t.Fatalf("worker failure lost coverage reason: %+v", response.Results.Coverage)
	}
}

func makeCompressedTreeIPSW(t *testing.T, path string) {
	t.Helper()
	if err := testfixture.IPSW(path, "14.0", 1); err != nil {
		t.Fatal(err)
	}
	archive, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	rewritten := path + ".tmp"
	file, err := os.Create(rewritten)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	for _, member := range archive.File {
		reader, err := member.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(reader)
		if closeErr := reader.Close(); err == nil {
			err = closeErr
		}
		if err != nil {
			t.Fatal(err)
		}
		if member.Name == "Firmware/tree.im4p" {
			data = testfixture.IM4P("dtre", lzfse.EncodeBuffer(testfixture.Tree(1)))
		}
		w, err := writer.CreateHeader(&zip.FileHeader{Name: member.Name, Method: zip.Store})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := errors.Join(writer.Close(), file.Close()); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(rewritten, path); err != nil {
		t.Fatal(err)
	}
}

func TestListingTextReportsPartialSourcesAndExactNextCommand(t *testing.T) {
	listing := acquisition.BuildListing([]acquisition.Response{
		{Source: "https://gdmf.apple.com/fixture", Status: "failed", Error: "offline"},
		{
			Source: "https://api.ipsw.me/fixture",
			Status: "complete",
			Candidates: []acquisition.Candidate{
				{
					Product: "iPhone12,1",
					Version: "14.0",
					Build:   "18A373",
					Kind:    "restore_ipsw",
					URL:     "https://updates.cdn-apple.com/fixture.ipsw",
					Source:  "https://api.ipsw.me/fixture",
				},
			},
		},
	}, "iPhone12,1", "14.0")
	if listingState(listing) != "partial" {
		t.Fatal("failed source reported complete")
	}
	var out bytes.Buffer
	printFirmwareListing(&out, listingState(listing), listing)
	for _, text := range []string{"PARTIAL", "Source failed", "unverified_catalog_locator", "firmware fetch --product 'iPhone12,1' --version '14.0' --build '18A373'"} {
		if !bytes.Contains(out.Bytes(), []byte(text)) {
			t.Fatalf("missing %q in %s", text, out.String())
		}
	}
}

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

func TestResourceStatusShowsConfiguredBudgetsWithoutDatabase(t *testing.T) {
	t.Setenv("GOIOSVM_CACHE_DIR", t.TempDir())
	var out, errout bytes.Buffer
	c := newCommand(&out, &errout)
	c.SetArgs([]string{
		"--format", "json", "--cache-limit", "4096", "--disk-reserve", "1024",
		"--evidence-limit", "2048", "resources", "status",
	})
	if err := c.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err, errout.String())
	}
	var response struct {
		State   string          `json:"state"`
		Results json.RawMessage `json:"results"`
	}
	if err := json.Unmarshal(out.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.State != "complete" {
		t.Fatalf("unexpected state: %+v", response)
	}
	var result struct {
		Cache struct {
			LimitBytes   int64 `json:"limit_bytes"`
			ReserveBytes int64 `json:"reserve_bytes"`
		} `json:"cache"`
		DiskStaging struct {
			LimitBytes uint64 `json:"limit_bytes"`
		} `json:"disk_staging"`
		EvidenceLimitBytes            int64 `json:"evidence_limit_bytes"`
		InspectionParallelismPerCache int   `json:"inspection_parallelism_per_cache"`
	}
	if err := json.Unmarshal(response.Results, &result); err != nil {
		t.Fatal(err)
	}
	if result.Cache.LimitBytes != 4096 || result.Cache.ReserveBytes != 1024 ||
		result.EvidenceLimitBytes != 2048 || result.DiskStaging.LimitBytes != 8<<30 ||
		result.InspectionParallelismPerCache != 1 {
		t.Fatalf("resource configuration lost: %+v", result)
	}
	out.Reset()
	errout.Reset()
	c = newCommand(&out, &errout)
	c.SetArgs([]string{"--cache-limit", "4096", "resources", "status"})
	if err := c.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err, errout.String())
	}
	if !bytes.Contains(out.Bytes(), []byte("resource budgets")) ||
		!bytes.Contains(out.Bytes(), []byte("cache:")) ||
		!bytes.Contains(out.Bytes(), []byte("OS staging:")) {
		t.Fatalf("resource text summary incomplete: %s", out.String())
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
	lastSchemaVersion := 0
	invoke := func(args ...string) (json.RawMessage, error) {
		t.Helper()
		var out, errout bytes.Buffer
		c := newCommand(&out, &errout)
		c.SetArgs(append([]string{"--format", "json"}, args...))
		err := c.ExecuteContext(context.Background())
		if out.Len() == 0 {
			return nil, err
		}
		var e struct {
			SchemaVersion int             `json:"schema_version"`
			Results       json.RawMessage `json:"results"`
		}
		if decodeErr := json.Unmarshal(out.Bytes(), &e); decodeErr != nil {
			t.Fatal(decodeErr, out.String())
		}
		lastSchemaVersion = e.SchemaVersion
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
		if i == 0 {
			var stdout, stderr bytes.Buffer
			code := runCLI(context.Background(), []string{"--format", "json", "inspect", "--image", path, "--identity", "0"}, &stdout, &stderr)
			if code != 2 {
				t.Fatalf("partial inspection exit %d: %s", code, stderr.String())
			}
			response := decodeOneEnvelope(t, stdout.Bytes())
			if response.State != "partial" || response.Results == nil || response.Diagnostic != nil ||
				!bytes.Contains(stderr.Bytes(), []byte("Hashing IPSW")) {
				t.Fatalf("partial output mixed with progress: %s; stderr %s", stdout.String(), stderr.String())
			}
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
	changedPath := filepath.Join(cache, "changed.ipsw")
	if err := testfixture.IPSW(changedPath, "14.0", 999); err != nil {
		t.Fatal(err)
	}
	if _, err := invoke("inspect", "--image", changedPath, "--identity", "0", "--resume", runs[0].ID); err == nil || classify(err) != "changed_input" {
		t.Fatalf("changed archive resume diagnostic: %v", err)
	}
	if _, err := invoke("inspect", "--image", filepath.Join(cache, "missing.ipsw"), "--identity", "0", "--resume", runs[0].ID); err == nil || classify(err) != "missing_input" {
		t.Fatalf("missing archive resume diagnostic: %v", err)
	}
	b, err := invoke("finding", "list", "--run", runs[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if lastSchemaVersion != 2 {
		t.Fatal("finding chains must identify their output schema version")
	}
	var findings []model.FindingChain
	if err = json.Unmarshal(b, &findings); err != nil || len(findings) == 0 {
		t.Fatal(err)
	}
	if len(findings[0].Revisions) == 0 || findings[0].Revisions[0].Observation == nil {
		t.Fatal("direct finding revision missing observation", findings[0])
	}
	initial := findings[0].Revisions[0]
	b, err = invoke("finding", "revise", "--target", findings[0].TargetID,
		"--subject", findings[0].Subject, "--property", findings[0].Property,
		"--basis", "inferred", "--presence", "present", "--value", `{"kind":"test-interpretation"}`,
		"--rationale", "fixture interpretation", "--supersedes", initial.ID,
		"--support-observation", initial.Observation.ID)
	if err != nil {
		t.Fatal(err)
	}
	var receipt model.RevisionReceipt
	if err = json.Unmarshal(b, &receipt); err != nil || receipt.FindingID != findings[0].ID || receipt.RevisionID == "" {
		t.Fatal("committed revision identity not returned", err)
	}
	if lastSchemaVersion != 2 {
		t.Fatal("revision receipt must identify its output schema version")
	}
	b, err = invoke("finding", "show", "--finding", receipt.FindingID)
	if err != nil {
		t.Fatal(err)
	}
	var revised model.FindingChain
	if err = json.Unmarshal(b, &revised); err != nil || len(revised.Revisions) != 2 {
		t.Fatal("revision chain not shown", err)
	}
	linked := false
	for _, revision := range revised.Revisions {
		linked = linked || revision.Supersedes == initial.ID
	}
	if !linked {
		t.Fatal("revision did not link to earlier reading")
	}
	b, err = invoke("finding", "assess", "--finding", receipt.FindingID,
		"--revision", receipt.RevisionID, "--basis", "inferred", "--state", "accepted",
		"--reviewer", "fixture-reviewer", "--reason", "retained source checked")
	if err != nil {
		t.Fatal(err)
	}
	var assessment model.AssessmentEvent
	if err = json.Unmarshal(b, &assessment); err != nil || assessment.ID == 0 ||
		assessment.RevisionID != receipt.RevisionID || lastSchemaVersion != 2 {
		t.Fatal("assessment receipt missing exact revision", err)
	}
	if _, err = invoke("finding", "assess", "--finding", receipt.FindingID,
		"--revision", receipt.RevisionID, "--basis", "inferred", "--state", "disputed",
		"--reviewer", "second-reviewer", "--reason", "needs another source"); err == nil || classify(err) != "assessment_conflict" {
		t.Fatalf("stale review did not report conflict: %v", err)
	}
	b, err = invoke("finding", "show", "--finding", receipt.FindingID)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(b, &revised); err != nil {
		t.Fatal(err)
	}
	for _, r := range revised.Revisions {
		if r.ID == receipt.RevisionID && (r.Assessment != "accepted" ||
			len(r.AssessmentHistory) != 1 || r.AssessmentHistory[0].ID != assessment.ID) {
			t.Fatalf("assessment history not browsable: %+v", r)
		}
	}
	if _, err = invoke("finding", "show", "--finding", findings[0].ID); err != nil {
		t.Fatal("finding show failed", err)
	}
	if _, err = invoke(
		"finding",
		"show",
		"--evidence",
		initial.Observation.EvidenceID,
	); err != nil {
		t.Fatal("source removal destroyed retained evidence", err)
	}
	if lastSchemaVersion != 1 {
		t.Fatal("unchanged evidence output must keep schema version 1")
	}
	if _, err = invoke("compare", "--left", runs[0].ID, "--right", runs[1].ID); err != nil {
		t.Fatal(err)
	}
}
