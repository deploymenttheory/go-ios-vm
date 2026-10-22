package acquisition

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"howett.net/plist"
)

func TestListSearchIncludesHistoricalRestoreAndSourceAttribution(t *testing.T) {
	restore, err := plist.Marshal(map[string]any{
		"MobileDeviceSoftwareVersionsByVersion": map[string]any{
			"1": map[string]any{"MobileDeviceSoftwareVersions": map[string]any{
				"iPhone12,1": map[string]any{"19A346": map[string]any{
					"Restore": map[string]string{
						"ProductVersion": "15.0", "BuildVersion": "19A346",
						"FirmwareURL": "https://updates.cdn-apple.com/ios15.ipsw",
					},
				}},
			}},
		}}, plist.XMLFormat)
	if err != nil {
		t.Fatal(err)
	}
	client := NewClient()
	client.PMVURL = "https://gdmf.apple.com/fixture"
	client.RestoreURL = "https://itunes.apple.com/fixture"
	client.HistoricalURL = "https://api.ipsw.me/fixture/"
	client.HTTP.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		var body []byte
		switch r.URL.Hostname() {
		case "gdmf.apple.com":
			body = []byte(`{"PublicAssetSets":{"iOS":[{"ProductVersion":"14.0","Build":"18A373","SupportedDevices":["iPhone12,1"]}]}}`)
		case "itunes.apple.com":
			body = restore
		case "api.ipsw.me":
			body = []byte(`{"identifier":"iPhone12,1","firmwares":[{"identifier":"iPhone12,1","version":"14.0","buildid":"18A373","url":"https://updates.cdn-apple.com/ios14.ipsw","sha1sum":"9d9bc29986dd302e26ccbf9cba069262bd3b2295","filesize":5574812591}]}`)
		default:
			t.Fatalf("unexpected host %q", r.URL.Hostname())
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(body)), Header: http.Header{}}, nil
	})
	responses := client.List(context.Background(), "iPhone12,1")
	if len(responses) != 3 || len(responses[2].Data) == 0 || len(responses[2].Candidates) != 1 {
		t.Fatal(responses)
	}
	listing := BuildListing(responses, "iPhone12,1", "14.0")
	if len(listing.Restores) != 1 {
		t.Fatal("release-only metadata or unrelated restore entered listing", listing)
	}
	row := listing.Restores[0]
	if row.Version != "14.0" || row.Build != "18A373" || row.Size != 5574812591 ||
		row.Status != statusUnverified || !row.Fetchable || len(row.Sources) != 1 ||
		!strings.Contains(row.NextCommand, "--build '18A373'") {
		t.Fatal(row)
	}
	if got := BuildListing(responses, "iPhone12,1", "a346"); len(got.Restores) != 1 || got.Restores[0].Version != "15.0" {
		t.Fatal("build search did not match", got)
	}
	if got := BuildListing(responses, "iPhone12,1", "20.0"); len(got.Restores) != 0 || len(got.Sources) != 3 {
		t.Fatal("no-match lost source context", got)
	}
}

func TestListPreservesFailureAndRejectsAmbiguousOrNonAppleRows(t *testing.T) {
	responses := []Response{
		{Source: PMV, Status: "failed", Error: "catalog unavailable"},
		{Source: RestoreCatalog, Status: "complete", Candidates: []Candidate{
			{Product: "iPhone12,1", Version: "14.0", Build: "18A373", Kind: "restore_ipsw", URL: "https://updates.cdn-apple.com/one.ipsw", Source: RestoreCatalog},
			{Product: "iPhone12,1", Version: "14.0", Build: "18A373", Kind: "restore_ipsw", URL: "https://apple.com.attacker.invalid/two.ipsw", Source: RestoreCatalog},
		}},
		{Source: HistoricalCatalog, Status: "complete", Candidates: []Candidate{
			{Product: "iPhone12,1", Version: "14.0", Build: "18A373", Kind: "restore_ipsw", URL: "https://updates.cdn-apple.com/two.ipsw", Source: HistoricalCatalog},
		}},
	}
	listing := BuildListing(responses, "iPhone12,1", "14")
	if len(listing.Sources) != 3 || listing.Sources[0].Status != "failed" || len(listing.Restores) != 2 {
		t.Fatal(listing)
	}
	for _, row := range listing.Restores {
		if row.Status != statusAmbiguous || row.Fetchable || row.NextCommand != "" {
			t.Fatal("ambiguous selection was offered for fetch", row)
		}
	}
	if _, err := Select(responses, Request{Product: "iPhone12,1", Version: "14.0", Build: "18A373"}); !errors.Is(err, errExpectedOneExactRestoreIPSW) {
		t.Fatal("ambiguous selection accepted", err)
	}
}

func TestListKeepsHistoricalResultsWhenOneSourceFails(t *testing.T) {
	client := NewClient()
	client.PMVURL = "https://gdmf.apple.com/fixture"
	client.RestoreURL = "https://itunes.apple.com/fixture"
	client.HistoricalURL = "https://api.ipsw.me/fixture/"
	client.HTTP.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.Hostname() == "gdmf.apple.com" {
			return nil, errors.New("source offline")
		}
		body := []byte(`<?xml version="1.0"?><plist version="1.0"><dict/></plist>`)
		if r.URL.Hostname() == "api.ipsw.me" {
			body = []byte(`{"identifier":"iPhone12,1","firmwares":[{"identifier":"iPhone12,1","version":"14.0","buildid":"18A373","url":"https://updates.cdn-apple.com/ios14.ipsw","filesize":100}]}`)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(body)), Header: http.Header{}}, nil
	})
	responses := client.List(context.Background(), "iPhone12,1")
	listing := BuildListing(responses, "iPhone12,1", "14.0")
	if len(listing.Restores) != 1 || listing.Sources[0].Status != "failed" ||
		listing.Sources[0].Error == "" || listing.Restores[0].Status != statusUnverified {
		t.Fatal(listing)
	}
}

func TestSelectMergesSameURLAndRejectsConflictingMetadata(t *testing.T) {
	url := "https://updates.cdn-apple.com/one.ipsw"
	apple := Candidate{Product: "iPhone12,1", Version: "14.0", Build: "18A373", Kind: "restore_ipsw", URL: url, Source: RestoreCatalog, Size: 100}
	historical := apple
	historical.Source = HistoricalCatalog
	historical.Size = 100
	historical.SHA1 = strings.Repeat("a", 40)
	responses := []Response{{Candidates: []Candidate{apple}}, {Candidates: []Candidate{historical}}}
	selected, err := Select(responses, Request{Product: "iPhone12,1", Version: "14.0", Build: "18A373"})
	if err != nil || selected.Size != 100 || selected.SHA1 != historical.SHA1 {
		t.Fatal(selected, err)
	}
	listing := BuildListing(responses, "iPhone12,1", "14.0")
	if len(listing.Restores) != 1 || len(listing.Restores[0].Sources) != 2 || !listing.Restores[0].Fetchable {
		t.Fatal(listing)
	}
	historical.Size = 101
	responses[1].Candidates[0] = historical
	if _, err = Select(responses, Request{Product: "iPhone12,1", Version: "14.0", Build: "18A373"}); !errors.Is(err, errExpectedOneExactRestoreIPSW) {
		t.Fatal("conflicting declaration accepted", err)
	}
}
