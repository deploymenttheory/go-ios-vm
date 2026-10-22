package acquisition

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"howett.net/plist"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestDiscoverySeparatesReleaseAndRestore(t *testing.T) {
	request := Request{Product: "iPhone12,1", Version: "14.0"}
	releases, err := parsePMV(
		[]byte(
			`{"PublicAssetSets":{"iOS":[{"ProductVersion":"14.0","Build":"18A373","SupportedDevices":["iPhone12,1"]}]}}`,
		),
		request,
		PMV,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(releases) != 1 || releases[0].Kind != "release" || releases[0].URL != "" {
		t.Fatal(releases)
	}
	data, _ := plist.Marshal(
		map[string]any{
			"MobileDeviceSoftwareVersionsByVersion": map[string]any{
				"1": map[string]any{
					"MobileDeviceSoftwareVersions": map[string]any{
						"iPhone12,1": map[string]any{
							"18A373": map[string]any{
								"Restore": map[string]string{
									"ProductVersion": "14.0",
									"BuildVersion":   "18A373",
									"FirmwareURL":    "https://updates.cdn-apple.com/fixture.ipsw",
								},
							},
						},
					},
				},
			},
		},
		plist.XMLFormat,
	)
	restores, err := parseRestore(data, request, RestoreCatalog)
	if err != nil {
		t.Fatal(err)
	}
	if len(restores) != 1 {
		t.Fatal(restores)
	}
	if _, err = Select([]Response{{Candidates: releases}}, request); err == nil {
		t.Fatal("release metadata accepted as IPSW")
	}
	if _, err = Select([]Response{{Candidates: restores}}, request); err != nil {
		t.Fatal(err)
	}
	if err = AppleURL("https://apple.com.attacker.invalid/a"); err == nil {
		t.Fatal("invalid source accepted")
	}
}

func TestFailedVerificationAndCancellationLeaveNoPublishedImage(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		t.Run(map[bool]string{false: "identity", true: "cancelled"}[cancelled], func(t *testing.T) {
			cache := Cache{Dir: t.TempDir(), Limit: 1024}
			client := NewClient()
			client.HTTP = &http.Client{
				Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
					return &http.Response{
						StatusCode:    200,
						ContentLength: 4,
						Body:          io.NopCloser(bytes.NewReader([]byte("data"))),
						Header:        http.Header{"Etag": []string{`"stable"`}},
					}, nil
				}),
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if cancelled {
				cancel()
			}
			_, err := client.Fetch(
				ctx,
				Candidate{URL: "https://updates.cdn-apple.com/a.ipsw", Size: 4, ETag: `"stable"`},
				cache,
				func(context.Context, string, Candidate) error { return io.ErrUnexpectedEOF },
			)
			if err == nil {
				t.Fatal("expected verification/cancellation failure")
			}
			entries, _ := cache.Entries()
			if len(entries) != 0 {
				t.Fatal("failed image published")
			}
			files, _ := os.ReadDir(cache.Dir)
			for _, f := range files {
				if f.Name() != ".lock" {
					t.Fatal("staging leak", f.Name())
				}
			}
		})
	}
}

func TestFetchRejectsHistoricalLocatorSizeMismatch(t *testing.T) {
	cache := Cache{Dir: t.TempDir(), Limit: 1024}
	client := NewClient()
	client.HTTP.Transport = roundTrip(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode:    http.StatusOK,
			ContentLength: 4,
			Body:          io.NopCloser(bytes.NewReader([]byte("data"))),
			Header:        http.Header{"Etag": []string{`"stable"`}},
		}, nil
	})
	called := false
	_, err := client.Fetch(
		context.Background(),
		Candidate{URL: "https://updates.cdn-apple.com/a.ipsw", Size: 5, ETag: `"stable"`},
		cache,
		func(context.Context, string, Candidate) error { called = true; return nil },
	)
	if !errors.Is(err, errCatalogSizeMismatch) || called {
		t.Fatalf("mismatched size reached verification: %v, %v", err, called)
	}
	entries, err := cache.Entries()
	if err != nil || len(entries) != 0 {
		t.Fatalf("mismatched image entered cache: %v, %v", entries, err)
	}
}

func TestLeasePreventsPruneAndExternalFilesSurvive(t *testing.T) {
	cache := Cache{Dir: t.TempDir(), Limit: 4096}
	managed := filepath.Join(cache.Dir, strings.Repeat("a", 64)+".ipsw")
	outside := filepath.Join(cache.Dir, "original.ipsw")
	for _, p := range []string{managed, outside} {
		if err := os.WriteFile(p, []byte("image"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	release, err := cache.Lease(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err = cache.Prune(ctx, true); err == nil {
		t.Fatal("active lease evicted")
	}
	release()
	preview, err := cache.Prune(context.Background(), false)
	if err != nil || len(preview) != 1 {
		t.Fatal(preview, err)
	}
	if _, err = cache.Prune(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(outside); err != nil {
		t.Fatal("external image removed")
	}
}

func TestFetchRejectsSourceChangedAfterProbe(t *testing.T) {
	client := NewClient()
	client.HTTP.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("If-Match") != `"probed"` {
			t.Fatal("full fetch lost ETag precondition")
		}
		return &http.Response{
			StatusCode:    200,
			ContentLength: 4,
			Header:        http.Header{"Etag": []string{`"changed"`}},
			Body:          io.NopCloser(strings.NewReader("data")),
		}, nil
	})
	cache := Cache{Dir: t.TempDir(), Limit: 1024}
	_, err := client.Fetch(
		context.Background(),
		Candidate{URL: "https://updates.cdn-apple.com/a.ipsw", Size: 4, ETag: `"probed"`},
		cache,
		func(context.Context, string, Candidate) error { return nil },
	)
	if !errors.Is(err, errCDNObjectChanged) {
		t.Fatal("changed CDN object accepted after manifest probe", err)
	}
	files, err := os.ReadDir(cache.Dir)
	if err != nil || len(files) != 1 || files[0].Name() != ".lock" {
		t.Fatal("changed object left a cache image", files, err)
	}
}

func TestInterruptedFetchResumesOnlyMatchingObject(t *testing.T) {
	client := NewClient()
	attempt := 0
	client.HTTP.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		attempt++
		if r.Header.Get("If-Match") != `"stable"` {
			t.Fatal("missing ETag precondition")
		}
		if attempt == 1 {
			if r.Header.Get("Range") != "" {
				t.Fatal("unexpected initial range")
			}
			return &http.Response{
				StatusCode: 200, ContentLength: 4,
				Header: http.Header{"Etag": []string{`"stable"`}},
				Body:   io.NopCloser(strings.NewReader("da")),
			}, nil
		}
		if r.Header.Get("Range") != "bytes=2-" {
			t.Fatal("resume did not start after persisted bytes", r.Header)
		}
		return &http.Response{
			StatusCode: 206, ContentLength: 2,
			Header: http.Header{
				"Etag":          []string{`"stable"`},
				"Content-Range": []string{"bytes 2-3/4"},
			},
			Body: io.NopCloser(strings.NewReader("ta")),
		}, nil
	})
	cache := Cache{Dir: t.TempDir(), Limit: 1024}
	candidate := Candidate{URL: "https://updates.cdn-apple.com/a.ipsw", Size: 4, ETag: `"stable"`}
	verify := func(_ context.Context, path string, _ Candidate) error {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if string(data) != "data" {
			t.Fatalf("corrupt resumed image: %q", data)
		}
		return nil
	}
	if _, err := client.Fetch(context.Background(), candidate, cache, verify); err == nil {
		t.Fatal("truncated initial response accepted")
	}
	entries, err := cache.Entries()
	if err != nil || len(entries) != 1 || entries[0].Size != 2 {
		t.Fatal("partial bytes not retained", entries, err)
	}
	artifact, err := client.Fetch(context.Background(), candidate, cache, verify)
	if err != nil || artifact.Size != 4 || attempt != 2 {
		t.Fatal(artifact, err, attempt)
	}
	entries, err = cache.Entries()
	if err != nil || len(entries) != 1 || !strings.HasSuffix(entries[0].Path, ".ipsw") {
		t.Fatal("partial not replaced by verified image", entries, err)
	}
}

func TestProgressingImageDownloadOutlivesClientTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("ETag", `"stable"`)
		w.Header().Set("Content-Length", "4")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("da"))
		w.(http.Flusher).Flush()
		time.Sleep(80 * time.Millisecond)
		_, _ = w.Write([]byte("ta"))
	}))
	defer server.Close()
	localURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	client := NewClient()
	client.HTTP.Timeout = 20 * time.Millisecond
	client.HTTP.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		local := r.Clone(r.Context())
		local.URL.Scheme = localURL.Scheme
		local.URL.Host = localURL.Host
		local.Host = localURL.Host
		return http.DefaultTransport.RoundTrip(local)
	})
	cache := Cache{Dir: t.TempDir(), Limit: 1024}
	var phases []string
	artifact, err := client.Fetch(
		context.Background(),
		Candidate{URL: "https://updates.cdn-apple.com/a.ipsw", Size: 4, ETag: `"stable"`},
		cache,
		func(_ context.Context, path string, _ Candidate) error {
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			if string(data) != "data" {
				return io.ErrUnexpectedEOF
			}
			return nil
		},
		func(_ context.Context, p FetchProgress) error {
			if p.DownloadedBytes > p.TotalBytes || p.VerifiedBytes > p.TotalBytes {
				t.Fatalf("progress exceeded known total: %+v", p)
			}
			phases = append(phases, p.Phase)
			return nil
		},
	)
	if err != nil || artifact.Size != 4 {
		t.Fatalf("progressing download failed after short client timeout: %+v, %v", artifact, err)
	}
	if strings.Join(phases, ",") != "downloading,verifying,identity_check,published" {
		t.Fatalf("fetch phase transitions: %v", phases)
	}
}
