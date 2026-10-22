package acquisition

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"testing"
)

func TestHistoricalLocatorKeepsExactAppleImage(t *testing.T) {
	data := []byte(`{"identifier":"iPhone12,1","firmwares":[
		{"identifier":"iPhone12,1","version":"14.0","buildid":"18A373","url":"https://updates.cdn-apple.com/public.ipsw","sha1sum":"9d9bc29986dd302e26ccbf9cba069262bd3b2295","filesize":5574812591},
		{"identifier":"iPhone12,1","version":"14.0","buildid":"18A5351d","url":"https://updates.cdn-apple.com/seed.ipsw","filesize":5507405643},
		{"identifier":"iPhone12,1","version":"14.0","buildid":"18A373","url":"https://apple.com.attacker.invalid/malicious.ipsw","filesize":5574812591},
		{"identifier":"iPhone12,1","version":"14.0","buildid":"18A373","url":"https://updates.cdn-apple.com/ota.zip","filesize":5574812591}
	]}`)
	request := Request{Product: "iPhone12,1", Version: "14.0", Build: "18A373"}
	candidates, err := parseHistorical(data, request, HistoricalCatalog)
	if err != nil || len(candidates) != 1 {
		t.Fatalf("expected one exact Apple locator, got %v, %v", candidates, err)
	}
	if candidates[0].Build != "18A373" || candidates[0].Size != 5574812591 ||
		candidates[0].SHA1 != "9d9bc29986dd302e26ccbf9cba069262bd3b2295" {
		t.Fatal(candidates[0])
	}
}

func TestHistoricalLocatorRefusesWrongProductAndAmbiguousImages(t *testing.T) {
	data := []byte(`{"identifier":"iPhone12,1","firmwares":[
		{"identifier":"iPhone12,1","version":"14.0","buildid":"18A373","url":"https://updates.cdn-apple.com/one.ipsw","filesize":100},
		{"identifier":"iPhone12,1","version":"14.0","buildid":"18A373","url":"https://updates.cdn-apple.com/two.ipsw","filesize":101}
	]}`)
	request := Request{Product: "iPhone12,1", Version: "14.0", Build: "18A373"}
	if _, err := parseHistorical(
		data,
		Request{Product: "iPhone12,3"},
		HistoricalCatalog,
	); !errors.Is(
		err,
		errHistoricalProductMismatch,
	) {
		t.Fatalf("wrong product accepted: %v", err)
	}
	candidates, err := parseHistorical(data, request, HistoricalCatalog)
	if err != nil || len(candidates) != 2 {
		t.Fatalf("ambiguous images not retained for explicit failure: %v, %v", candidates, err)
	}
	if _, err := Select(
		[]Response{{Candidates: candidates}},
		request,
	); !errors.Is(
		err,
		errExpectedOneExactRestoreIPSW,
	) {
		t.Fatalf("ambiguous images selected: %v", err)
	}
	if _, err := Select(
		[]Response{{Candidates: candidates}},
		Request{Product: "iPhone12,1", Version: "15.0"},
	); !errors.Is(
		err,
		errExpectedOneExactRestoreIPSW,
	) {
		t.Fatalf("unresolved release selected: %v", err)
	}
}

func TestDiscoveryUsesHistoricalLocatorOnlyAfterAppleMiss(t *testing.T) {
	client := NewClient()
	client.PMVURL = "https://gdmf.apple.com/fixture"
	client.RestoreURL = "https://itunes.apple.com/fixture"
	client.HistoricalURL = "https://api.ipsw.me/fixture/"
	calls := map[string]int{}
	client.HTTP.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		calls[r.URL.Hostname()]++
		var body []byte
		switch r.URL.Hostname() {
		case "gdmf.apple.com":
			body = []byte(`{"PublicAssetSets":{}}`)
		case "itunes.apple.com":
			body = []byte(`<?xml version="1.0"?><plist version="1.0"><dict/></plist>`)
		case "api.ipsw.me":
			body = []byte(
				`{"identifier":"iPhone12,1","firmwares":[{"identifier":"iPhone12,1","version":"14.0","buildid":"18A373","url":"https://updates.cdn-apple.com/fixture.ipsw","filesize":100}]}`,
			)
		default:
			t.Fatalf("unexpected host %q", r.URL.Hostname())
		}
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(bytes.NewReader(body)),
			Header:     http.Header{},
		}, nil
	})
	request := Request{Product: "iPhone12,1", Version: "14.0", Build: "18A373"}
	responses := client.Discover(context.Background(), request)
	if len(responses) != 3 || responses[2].Status != "complete" || len(responses[2].Data) == 0 {
		t.Fatal(responses)
	}
	if _, err := Select(responses, request); err != nil {
		t.Fatal(err)
	}
	if calls["api.ipsw.me"] != 1 {
		t.Fatal(calls)
	}
}
