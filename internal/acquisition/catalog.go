// Package acquisition resolves Apple restore images and manages disposable downloads.
package acquisition

import (
	"context"
	"encoding/json"
	"fmt"
	"howett.net/plist"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

const PMV = "https://gdmf.apple.com/v2/pmv"
const RestoreCatalog = "https://itunes.apple.com/WebObjects/MZStore.woa/wa/com.apple.jingle.appserver.client.MZITunesClientCheck/version/"

type Request struct {
	Product string `json:"product"`
	Version string `json:"version"`
	Build   string `json:"build"`
}
type Candidate struct {
	Product string `json:"product"`
	Version string `json:"version"`
	Build   string `json:"build"`
	Kind    string `json:"kind"`
	URL     string `json:"url,omitempty"`
	SHA1    string `json:"sha1,omitempty"`
	Source  string `json:"source"`
}
type Response struct {
	Source     string      `json:"source"`
	Status     string      `json:"status"`
	Candidates []Candidate `json:"candidates"`
	Data       []byte      `json:"-"`
	Error      string      `json:"error,omitempty"`
}
type Client struct {
	HTTP       *http.Client
	PMVURL     string
	RestoreURL string
}

func NewClient() *Client {
	return &Client{HTTP: &http.Client{Timeout: 30 * time.Minute, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if len(via) > 5 {
			return fmt.Errorf("redirect limit")
		}
		return AppleURL(r.URL.String())
	}}, PMVURL: PMV, RestoreURL: RestoreCatalog}
}
func AppleURL(raw string) error {
	u, e := url.Parse(raw)
	if e != nil {
		return e
	}
	h := strings.ToLower(u.Hostname())
	if u.Scheme != "https" || u.User != nil || !(h == "apple.com" || strings.HasSuffix(h, ".apple.com") || h == "cdn-apple.com" || strings.HasSuffix(h, ".cdn-apple.com")) {
		return fmt.Errorf("expected an HTTPS Apple-owned URL")
	}
	return nil
}
func (c *Client) read(ctx context.Context, endpoint string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return nil, fmt.Errorf("catalog HTTP %d", res.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, (32<<20)+1))
	if err == nil && len(b) > 32<<20 {
		return nil, fmt.Errorf("catalog exceeds 32 MiB")
	}
	return b, err
}
func matches(r Request, c Candidate) bool {
	return (r.Product == "" || r.Product == c.Product) && (r.Version == "" || r.Version == c.Version) && (r.Build == "" || r.Build == c.Build)
}
func (c *Client) Discover(ctx context.Context, r Request) []Response {
	out := []Response{}
	for _, endpoint := range []string{c.PMVURL, c.RestoreURL} {
		item := Response{Source: endpoint, Status: "complete", Candidates: []Candidate{}}
		b, err := c.read(ctx, endpoint)
		item.Data = b
		if err == nil {
			if endpoint == c.PMVURL {
				item.Candidates, err = parsePMV(b, r, endpoint)
			} else {
				item.Candidates, err = parseRestore(b, r, endpoint)
			}
		}
		if err != nil {
			item.Status = "failed"
			item.Error = err.Error()

		} else if len(item.Candidates) == 0 {
			item.Status = "no_match"
		}
		out = append(out, item)
	}
	return out
}
func parsePMV(b []byte, r Request, source string) ([]Candidate, error) {
	type asset struct {
		ProductVersion   string
		Build            string
		SupportedDevices []string
	}
	var p struct {
		PublicAssetSets map[string][]asset
		AssetSets       map[string][]asset
	}
	if err := json.Unmarshal(b, &p); err != nil {
		return nil, err
	}
	out := []Candidate{}
	seen := map[string]bool{}
	for _, group := range []map[string][]asset{p.PublicAssetSets, p.AssetSets} {
		for _, entries := range group {
			for _, a := range entries {
				for _, product := range a.SupportedDevices {
					v := Candidate{Product: product, Version: a.ProductVersion, Build: a.Build, Kind: "release", Source: source}
					key := product + "/" + a.ProductVersion + "/" + a.Build
					if matches(r, v) && !seen[key] {
						out = append(out, v)
						seen[key] = true
					}
				}
			}
		}
	}
	sortCandidates(out)
	return out, nil
}
func parseRestore(b []byte, r Request, source string) ([]Candidate, error) {
	type build struct {
		BuildVersion   string
		FirmwareURL    string
		FirmwareSHA1   string
		ProductVersion string
	}
	type info struct {
		Restore *build
		SameAs  string
	}
	var p struct {
		MobileDeviceSoftwareVersionsByVersion map[string]struct{ MobileDeviceSoftwareVersions map[string]map[string]info }
	}
	if _, err := plist.Unmarshal(b, &p); err != nil {
		return nil, err
	}
	out := []Candidate{}
	seen := map[string]bool{}
	for _, group := range p.MobileDeviceSoftwareVersionsByVersion {
		for product, builds := range group.MobileDeviceSoftwareVersions {
			for key, b := range builds {
				if b.Restore == nil {
					continue
				}
				v := Candidate{Product: product, Version: b.Restore.ProductVersion, Build: b.Restore.BuildVersion, URL: b.Restore.FirmwareURL, SHA1: b.Restore.FirmwareSHA1, Kind: "restore_ipsw", Source: source}
				if v.Build == "" {
					v.Build = key
				}
				id := product + "/" + v.Build + "/" + v.URL
				if matches(r, v) && !seen[id] && AppleURL(v.URL) == nil {
					out = append(out, v)
					seen[id] = true
				}
			}
		}
	}
	sortCandidates(out)
	return out, nil
}
func sortCandidates(c []Candidate) {
	sort.Slice(c, func(i, j int) bool {
		return c[i].Product+"/"+c[i].Version+"/"+c[i].Build < c[j].Product+"/"+c[j].Version+"/"+c[j].Build
	})
}
func Select(responses []Response, r Request) (Candidate, error) {
	if r.Product == "" || (r.Version == "" && r.Build == "") {
		return Candidate{}, fmt.Errorf("fetch requires product and an exact version or build")
	}
	var candidates []Candidate
	for _, res := range responses {
		for _, c := range res.Candidates {
			if c.Kind == "restore_ipsw" && matches(r, c) {
				candidates = append(candidates, c)
			}
		}
	}
	if len(candidates) != 1 {
		return Candidate{}, fmt.Errorf("expected one exact restore IPSW, resolved %d; inspected catalogs are not a complete historical archive", len(candidates))
	}
	return candidates[0], nil
}
