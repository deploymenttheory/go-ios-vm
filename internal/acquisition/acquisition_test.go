package acquisition

import (
	"bytes"
	"context"
	"io"
	"net/http"
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
						Header:        http.Header{},
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
				Candidate{URL: "https://updates.cdn-apple.com/a.ipsw"},
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
