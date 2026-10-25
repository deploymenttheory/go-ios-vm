package firmware

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// Errors from source selection.
var (
	ErrNoSource  = errors.New("no source for firmware")
	ErrAmbiguous = errors.New("firmware selection is ambiguous")
)

// Source is where an archive for an identity can be obtained. Exactly one of
// Path and URL is set. Size and SHA256 are expectations when known; a source
// never establishes identity by itself: the archive's manifest does.
type Source struct {
	Identity Identity `json:"identity"`
	// Path is a local archive, used in place.
	Path string `json:"path,omitempty"`
	URL  string `json:"url,omitempty"`
	// Size and SHA256 are the expected archive length and digest, or zero.
	Size   int64  `json:"size,omitempty"`
	SHA256 string `json:"sha256,omitempty"`
	// Selection records where the source came from.
	Selection Selection `json:"selection"`
}

// Selection is the attributed metadata a source was chosen from.
type Selection struct {
	Provider    string    `json:"provider"`
	Reference   string    `json:"reference,omitempty"`
	RetrievedAt time.Time `json:"retrieved_at,omitzero"`
	// ResponseSHA256 is the digest of the raw provider response.
	ResponseSHA256 string `json:"response_sha256,omitempty"`
	// Raw is the provider's record for the selected archive, as received.
	Raw json.RawMessage `json:"raw,omitempty"`
}

// KnownSources are the attributed sources for the supported pair.
var KnownSources = []Source{
	{
		Identity: SupportedPair.IPhone,
		URL:      "https://updates.cdn-apple.com/2026FallFCS/38dca0ee-bb5d-4132-ad13-62d57bcd6d32/iPhone17,3_27.0.1_24A446_Restore.ipsw",
		Size:     12_343_857_610,
		SHA256:   "fc87bb9da8c03d0154d87531950f4a170195f6663ee5208653fa7a26bafb9469",
		Selection: Selection{
			Provider:    "ipsw.me v4 and Apple CDN HEAD (x-amz-meta-digest-sha256)",
			Reference:   "docs/research-artifacts/verification/metadata-verification.json",
			RetrievedAt: time.Date(2026, 10, 19, 17, 27, 54, 0, time.UTC),
		},
	},
	{
		Identity: SupportedPair.CloudOS,
		URL:      "https://updates.cdn-apple.com/private-cloud-compute/c0ecdb4b310cf5239ab2b248dd3098eec297dc5aa3bbe6ada27273262b0b8b64",
		Selection: Selection{
			Provider:  "vphone-cli firmware catalog (cloud264)",
			Reference: "https://github.com/Lakr233/vphone-cli/blob/b2804adb45a653ff3981f0705108975414dc8d5d/VPhoneKit/VPhoneCoreKit/Firmware/VPhoneFirmwareCatalog.swift",
		},
	},
}

// KnownSource returns the attributed source for id.
func KnownSource(id Identity) (Source, error) {
	for _, s := range KnownSources {
		if s.Identity == id {
			return s, nil
		}
	}
	return Source{}, fmt.Errorf("%w %s", ErrNoSource, id)
}

// IPSWMeEndpoint is the ipsw.me v4 device API.
const IPSWMeEndpoint = "https://api.ipsw.me/v4/device/"

// ipswMeDevice is an ipsw.me v4 device response. Boards are objects
// (boardconfig, platform, cpid, bdid); the decoder also accepts the older
// string form so neither shape fails the selection.
type ipswMeDevice struct {
	Identifier string            `json:"identifier"`
	Boards     []ipswMeBoard     `json:"boards"`
	Firmwares  []json.RawMessage `json:"firmwares"`
}

type ipswMeBoard struct {
	BoardConfig string `json:"boardconfig"`
	Platform    string `json:"platform"`
	CPID        int64  `json:"cpid"`
	BDID        int64  `json:"bdid"`
}

func (b *ipswMeBoard) UnmarshalJSON(data []byte) error {
	var name string
	if err := json.Unmarshal(data, &name); err == nil {
		b.BoardConfig = name
		return nil
	}
	type plain ipswMeBoard
	if err := json.Unmarshal(data, (*plain)(b)); err != nil {
		return fmt.Errorf("ipsw.me board: %w", err)
	}
	return nil
}

type ipswMeFirmware struct {
	Identifier string `json:"identifier"`
	Version    string `json:"version"`
	BuildID    string `json:"buildid"`
	SHA256     string `json:"sha256sum"`
	Size       int64  `json:"filesize"`
	URL        string `json:"url"`
}

// SelectIPSWMe selects exactly id from an ipsw.me v4 device response. Zero or
// several matching firmwares are errors; nothing newer is substituted.
func SelectIPSWMe(id Identity, response []byte, retrievedAt time.Time) (Source, error) {
	var dev ipswMeDevice
	if err := json.Unmarshal(response, &dev); err != nil {
		return Source{}, fmt.Errorf("%w: ipsw.me response: %w", ErrNoSource, err)
	}
	if dev.Identifier != id.Device {
		return Source{}, fmt.Errorf("%w: ipsw.me response is for %q, want %s", ErrNoSource, dev.Identifier, id.Device)
	}
	var matches []json.RawMessage
	var chosen ipswMeFirmware
	for _, raw := range dev.Firmwares {
		var fw ipswMeFirmware
		if err := json.Unmarshal(raw, &fw); err != nil {
			return Source{}, fmt.Errorf("%w: ipsw.me firmware: %w", ErrNoSource, err)
		}
		if fw.Identifier == id.Device && fw.BuildID == id.Build && fw.Version == id.Version {
			matches = append(matches, raw)
			chosen = fw
		}
	}
	switch {
	case len(matches) == 0:
		return Source{}, fmt.Errorf("%w %s in ipsw.me response", ErrNoSource, id)
	case len(matches) > 1:
		return Source{}, fmt.Errorf("%w: ipsw.me lists %d archives for %s", ErrAmbiguous, len(matches), id)
	}
	u, err := url.Parse(chosen.URL)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return Source{}, fmt.Errorf("%w: ipsw.me URL %q is not https", ErrNoSource, chosen.URL)
	}
	sum := sha256.Sum256(response)
	return Source{
		Identity: id, URL: chosen.URL, Size: chosen.Size, SHA256: chosen.SHA256,
		Selection: Selection{
			Provider: "ipsw.me v4", Reference: IPSWMeEndpoint + id.Device + "?type=ipsw",
			RetrievedAt: retrievedAt, ResponseSHA256: hex.EncodeToString(sum[:]), Raw: matches[0],
		},
	}, nil
}

// FetchIPSWMe queries ipsw.me for id and selects it exactly.
func FetchIPSWMe(ctx context.Context, client *http.Client, endpoint string, id Identity) (Source, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+url.PathEscape(id.Device)+"?type=ipsw", nil)
	if err != nil {
		return Source{}, fmt.Errorf("ipsw.me request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return Source{}, fmt.Errorf("ipsw.me request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // read-only
	if resp.StatusCode != http.StatusOK {
		return Source{}, fmt.Errorf("%w: ipsw.me returned %s", ErrNoSource, resp.Status)
	}
	var body bytes.Buffer
	if _, err := io.Copy(&body, io.LimitReader(resp.Body, 16<<20)); err != nil {
		return Source{}, fmt.Errorf("ipsw.me response: %w", err)
	}
	return SelectIPSWMe(id, body.Bytes(), time.Now().UTC())
}
