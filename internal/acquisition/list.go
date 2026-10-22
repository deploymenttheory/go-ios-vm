package acquisition

import (
	"fmt"
	"sort"
	"strings"
)

const (
	statusUnverified = "unverified_catalog_locator"
	statusAmbiguous  = "ambiguous_catalog_locator"
)

// RestoreRow is a catalog locator. Fetch still probes and verifies the archive.
type RestoreRow struct {
	Candidate
	Sources     []string `json:"sources"`
	Status      string   `json:"status"`
	Fetchable   bool     `json:"fetchable"`
	NextCommand string   `json:"next_command,omitempty"`
}

type SourceStatus struct {
	Source string `json:"source"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

type Listing struct {
	Product  string         `json:"product"`
	Query    string         `json:"query,omitempty"`
	Sources  []SourceStatus `json:"sources"`
	Restores []RestoreRow   `json:"restores"`
}

// BuildListing combines identical URLs across catalogs while retaining their
// source attribution. A product/version/build with conflicting URLs or declared
// checksums is shown, but cannot be offered as an exact fetch command.
func BuildListing(responses []Response, product, query string) Listing {
	listing := Listing{
		Product: product, Query: query,
		Sources: []SourceStatus{}, Restores: []RestoreRow{},
	}
	byURL := map[string]int{}
	for _, response := range responses {
		listing.Sources = append(listing.Sources, SourceStatus{
			Source: response.Source, Status: response.Status, Error: response.Error,
		})
		for _, candidate := range response.Candidates {
			if candidate.Kind != "restore_ipsw" || candidate.Product != product ||
				!isAppleRestoreIPSW(candidate.URL) ||
				!strings.Contains(strings.ToLower(candidate.Version), strings.ToLower(query)) &&
					!strings.Contains(strings.ToLower(candidate.Build), strings.ToLower(query)) {
				continue
			}
			key := candidate.Product + "\x00" + candidate.Version + "\x00" + candidate.Build + "\x00" + candidate.URL
			if index, ok := byURL[key]; ok {
				row := &listing.Restores[index]
				row.Sources = append(row.Sources, response.Source)
				if conflictingDeclarations(row.Candidate, candidate) {
					row.Status = statusAmbiguous
				}
				if row.SHA1 == "" {
					row.SHA1 = candidate.SHA1
				}
				if row.Size == 0 {
					row.Size = candidate.Size
				}
				continue
			}
			byURL[key] = len(listing.Restores)
			listing.Restores = append(listing.Restores, RestoreRow{
				Candidate: candidate, Sources: []string{response.Source}, Status: statusUnverified,
			})
		}
	}
	byBuild := map[string][]int{}
	for i, row := range listing.Restores {
		key := row.Product + "\x00" + row.Version + "\x00" + row.Build
		byBuild[key] = append(byBuild[key], i)
	}
	for _, indices := range byBuild {
		ambiguous := len(indices) > 1
		for _, index := range indices {
			row := &listing.Restores[index]
			if ambiguous {
				row.Status = statusAmbiguous
			}
			if row.Status == statusUnverified && row.Version != "" && row.Build != "" {
				row.Fetchable = true
				row.NextCommand = fmt.Sprintf(
					"firmware fetch --product %s --version %s --build %s",
					shellQuote(row.Product), shellQuote(row.Version), shellQuote(row.Build),
				)
			}
		}
	}
	sort.Slice(listing.Restores, func(i, j int) bool {
		a, b := listing.Restores[i], listing.Restores[j]
		if a.Version != b.Version {
			return a.Version < b.Version
		}
		if a.Build != b.Build {
			return a.Build < b.Build
		}
		return a.URL < b.URL
	})
	return listing
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

func conflictingDeclarations(a, b Candidate) bool {
	checksumConflict := a.SHA1 != "" && b.SHA1 != "" && !strings.EqualFold(a.SHA1, b.SHA1)
	sizeConflict := a.Size > 0 && b.Size > 0 && a.Size != b.Size
	return checksumConflict || sizeConflict
}
