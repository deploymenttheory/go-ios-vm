package inspection

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploymenttheory/go-apfs-v3/apfs"
	"github.com/deploymenttheory/go-apfs-v3/filesystem"
	"github.com/deploymenttheory/go-apfs-v3/session"
	"github.com/deploymenttheory/go-ios-vm/internal/model"
	"github.com/deploymenttheory/go-ios-vm/internal/postgres"
)

func fixtureAPFSDisk(t *testing.T) []byte {
	t.Helper()
	ctx := context.Background()
	clock := time.Date(2025, 6, 7, 8, 9, 10, 0, time.UTC)
	s, err := session.Create(ctx, filepath.Join(t.TempDir(), "session"), session.Options{
		Names: filesystem.NameRules{Format: "APFS", NormalizationInsensitive: true},
		Time:  &clock,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.Mkdir(
		ctx,
		[]string{"/System/Library/Extensions/Example.kext"},
		session.MkdirOptions{Parents: true},
	); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{
		"Info.plist":  []byte("driver identity and dependencies"),
		"Empty.plist": nil,
		"Large.plist": bytes.Repeat([]byte("x"), filesystemFileLimit+1),
		"Program.bin": []byte("binary payload outside selected content format"),
	} {
		host := filepath.Join(t.TempDir(), name)
		if err := os.WriteFile(host, data, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Copy(
			ctx,
			[]string{host},
			"/System/Library/Extensions/Example.kext/"+name,
			session.CopyOptions{FromHost: true},
		); err != nil {
			t.Fatal(err)
		}
	}
	plan, err := apfs.Plan(ctx, s, apfs.BuildOptions{Name: "Test", Time: clock})
	if err != nil {
		t.Fatal(err)
	}
	var disk bytes.Buffer
	if err := plan.Write(ctx, &disk); err != nil {
		t.Fatal(err)
	}
	return disk.Bytes()
}

func TestAPFSFileContentAndSourceSurviveArchiveRemoval(t *testing.T) {
	archivePath := diskIPSW(t, zip.Store, fixtureAPFSDisk(t))
	a, err := Open(context.Background(), archivePath)
	if err != nil {
		t.Fatal(err)
	}
	result, err := a.Inspect(context.Background(), 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(archivePath); err != nil {
		t.Fatal(err)
	}

	var found, empty, large, absent, skipped bool
	for _, o := range result.Observations {
		if err := o.Validate(); err != nil {
			t.Fatal(err)
		}
		if o.Property == "search" && o.Presence == "absent" &&
			strings.Contains(string(o.Value), "/Library/Extensions") {
			absent = true
		}
		if o.Property != "content" {
			continue
		}
		if o.Presence == "unexamined" {
			if strings.Contains(string(o.Value), "Program.bin") {
				skipped = strings.Contains(string(o.Value), "outside selected")
			}
			continue
		}
		var value struct {
			Path     string `json:"path"`
			Complete bool   `json:"complete"`
		}
		if err := json.Unmarshal(o.Value, &value); err != nil {
			t.Fatal(err)
		}
		if o.Source.Coordinate != "apfs-logical-file-byte" ||
			!strings.Contains(o.Source.Selector, "/data") ||
			o.Source.Length != uint64(len(o.Evidence)) ||
			o.TargetID != a.Targets[0].ID {
			t.Fatalf("invalid file provenance: %+v", o)
		}
		switch value.Path {
		case "/System/Library/Extensions/Example.kext/Info.plist":
			found = bytes.Equal(o.Evidence, []byte("driver identity and dependencies")) &&
				value.Complete
		case "/System/Library/Extensions/Example.kext/Empty.plist":
			empty = len(o.Evidence) == 0 && value.Complete
		case "/System/Library/Extensions/Example.kext/Large.plist":
			large = len(o.Evidence) == filesystemFileLimit && !value.Complete
		}
	}
	if !found || !empty || !large || !absent || !skipped {
		t.Fatalf(
			"content status: found=%v empty=%v large=%v absent=%v skipped=%v",
			found,
			empty,
			large,
			absent,
			skipped,
		)
	}
	var coverage model.Coverage
	for _, c := range result.Coverage {
		if c.Component == "filesystem-content" {
			coverage = c
		}
	}
	if coverage.State != "partial" || !strings.Contains(coverage.Reason, "truncated") ||
		!strings.Contains(coverage.Reason, "filesystem-wide absence is unexamined") {
		t.Fatalf("coverage: %+v", coverage)
	}
}

func TestAPFSContentCheckpointResume(t *testing.T) {
	archivePath := diskIPSW(t, zip.Store, fixtureAPFSDisk(t))
	a, err := Open(context.Background(), archivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	for _, tc := range []struct {
		name          string
		done          map[string]bool
		wantComponent string
		wantContent   bool
	}{
		{
			name: "disk checkpoint retained",
			done: map[string]bool{
				"manifest": true, "DeviceTree": true, "KernelCache": true,
				"disk-metadata": true,
			},
			wantComponent: "filesystem-content",
			wantContent:   true,
		},
		{
			name: "filesystem checkpoint retained",
			done: map[string]bool{
				"manifest": true, "DeviceTree": true, "KernelCache": true,
				"filesystem-content": true,
			},
			wantComponent: "disk-metadata",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var components []string
			var hasContent bool
			err := a.InspectEach(
				context.Background(), 0, tc.done,
				func(component string, observations []model.Observation, coverage model.Coverage) error {
					components = append(components, component)
					if coverage.Component != component {
						t.Fatalf("component coverage mismatch: %+v", coverage)
					}
					for _, observation := range observations {
						if observation.Property == "content" {
							hasContent = true
						}
					}
					return nil
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			if len(components) != 1 || components[0] != tc.wantComponent ||
				hasContent != tc.wantContent {
				t.Fatalf("components=%v hasContent=%v", components, hasContent)
			}
		})
	}
}

func TestAPFSContentRetainedInPostgresAfterArchiveRemoval(t *testing.T) {
	dsn := os.Getenv("GOIOSVM_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set GOIOSVM_TEST_DATABASE_URL for PostgreSQL integration")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	var nonce [8]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		t.Fatal(err)
	}
	schema := "test_apfs_" + hex.EncodeToString(nonce[:])
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
	archivePath := diskIPSW(t, zip.Store, fixtureAPFSDisk(t))
	a, err := Open(ctx, archivePath)
	if err != nil {
		t.Fatal(err)
	}
	result, err := a.Inspect(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Register(ctx, a.Artifact, a.Targets); err != nil {
		t.Fatal(err)
	}
	run := model.Run{
		ID:       "apfs-content-test",
		TargetID: a.Targets[0].ID,
		State:    "partial",
		Started:  time.Now().UTC(),
		Coverage: result.Coverage,
	}
	if err := store.Start(ctx, run); err != nil {
		t.Fatal(err)
	}
	if err := store.Publish(ctx, run, result.Observations); err != nil {
		t.Fatal(err)
	}
	var evidenceID string
	for _, o := range result.Observations {
		if o.Property == "content" &&
			bytes.Equal(o.Evidence, []byte("driver identity and dependencies")) {
			evidenceID = o.EvidenceID
			break
		}
	}
	if evidenceID == "" {
		t.Fatal("missing retained file evidence")
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(archivePath); err != nil {
		t.Fatal(err)
	}
	store.Close()
	store, err = postgres.Open(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	data, err := store.Evidence(ctx, evidenceID)
	if err != nil || !bytes.Equal(data, []byte("driver identity and dependencies")) {
		t.Fatalf("retained content: %v", err)
	}
	findings, err := store.Findings(ctx, a.Targets[0].ID, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	var source bool
	for _, f := range findings {
		if f.Observation.EvidenceID == evidenceID &&
			f.Observation.Source.Coordinate == "apfs-logical-file-byte" &&
			strings.Contains(f.Observation.Source.Selector, "/data") {
			source = true
		}
	}
	if !source {
		t.Fatal("file source reference not retrievable after archive removal")
	}
}
