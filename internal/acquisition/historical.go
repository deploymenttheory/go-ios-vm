package acquisition

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path"
	"strings"
)

var errHistoricalProductMismatch = errors.New("historical catalog product mismatch")

func isAppleRestoreIPSW(raw string) bool {
	if AppleURL(raw) != nil {
		return false
	}
	u, err := url.Parse(raw)
	return err == nil && strings.EqualFold(path.Ext(u.Path), ".ipsw")
}

// parseHistorical treats a historical index as a URL locator, never as proof of
// firmware identity or VM compatibility. The IPSW's BuildManifest must be checked.
func parseHistorical(data []byte, requested Request, source string) ([]Candidate, error) {
	var listing struct {
		Identifier string `json:"identifier"`
		Firmwares  []struct {
			Identifier string `json:"identifier"`
			Version    string `json:"version"`
			Build      string `json:"buildid"`
			URL        string `json:"url"`
			SHA1       string `json:"sha1sum"`
			Size       int64  `json:"filesize"`
		} `json:"firmwares"`
	}
	if err := json.Unmarshal(data, &listing); err != nil {
		return nil, fmt.Errorf("parseHistorical: json.Unmarshal: %w", err)
	}
	if listing.Identifier != requested.Product {
		return nil, errHistoricalProductMismatch
	}
	out := []Candidate{}
	seen := map[string]bool{}
	for _, item := range listing.Firmwares {
		if item.Identifier != requested.Product || item.Size <= 0 || !isAppleRestoreIPSW(item.URL) {
			continue
		}
		if item.SHA1 != "" {
			digest, err := hex.DecodeString(item.SHA1)
			if err != nil || len(digest) != 20 {
				continue
			}
		}
		candidate := Candidate{
			Product: item.Identifier, Version: item.Version, Build: item.Build,
			Kind: "restore_ipsw", URL: item.URL, SHA1: strings.ToLower(item.SHA1),
			Source: source, Size: item.Size,
		}
		if matches(requested, candidate) && !seen[item.URL] {
			out = append(out, candidate)
			seen[item.URL] = true
		}
	}
	sortCandidates(out)
	return out, nil
}
