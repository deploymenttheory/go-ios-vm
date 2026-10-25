package firmware

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"howett.net/plist"
)

// fakeArchive builds a small restore archive whose manifest describes the
// given build. Padding makes it large enough to interrupt mid-transfer.
func fakeArchive(t *testing.T, version, build string, devices, classes []string, padding int) []byte {
	t.Helper()
	identities := make([]map[string]any, 0, len(classes))
	for _, c := range classes {
		identities = append(identities, map[string]any{"ApBoardID": "0x90", "ApChipID": "0xFE01", "Info": map[string]any{"DeviceClass": c, "Variant": "Customer Erase Install (IPSW)"}})
	}
	manifest, err := plist.Marshal(map[string]any{
		"ProductVersion": version, "ProductBuildVersion": build,
		"SupportedProductTypes": devices, "BuildIdentities": identities,
	}, plist.XMLFormat)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.CreateHeader(&zip.FileHeader{Name: "Firmware/padding.bin", Method: zip.Store})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(bytes.Repeat([]byte{0xA5}, padding)); err != nil {
		t.Fatal(err)
	}
	w, err = zw.Create(manifestName)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(manifest); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func iphoneArchive(t *testing.T) []byte {
	return fakeArchive(t, "27.0.1", "24A446", []string{"iPhone17,3"}, []string{"d94ap"}, 256<<10)
}

func cloudArchive(t *testing.T) []byte {
	return fakeArchive(t, "26.4", "23E5207q", []string{"iPhone99,11", "ComputeModule14,2"}, []string{"vphone600ap", "vresearch101ap"}, 64<<10)
}

func digest(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// server serves body, honouring Range unless ignoreRange is set, and cutting
// the first cutAfter bytes' connection short when cutAfter > 0.
type server struct {
	body        []byte
	ignoreRange bool
	cutAfter    atomic.Int64
	requests    atomic.Int64
	ranges      []string
}

func (s *server) start(t *testing.T) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.requests.Add(1)
		start := 0
		if rng := r.Header.Get("Range"); rng != "" {
			s.ranges = append(s.ranges, rng)
			if !s.ignoreRange {
				n, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(rng, "bytes="), "-"))
				if err != nil || n >= len(s.body) {
					http.Error(w, "bad range", http.StatusRequestedRangeNotSatisfiable)
					return
				}
				start = n
			}
		}
		body := s.body[start:]
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		if start > 0 {
			w.Header().Set("Content-Range", "bytes "+strconv.Itoa(start)+"-"+strconv.Itoa(len(s.body)-1)+"/"+strconv.Itoa(len(s.body)))
			w.WriteHeader(http.StatusPartialContent)
		}
		if cut := s.cutAfter.Swap(0); cut > 0 {
			_, _ = w.Write(body[:cut])
			if hj, ok := w.(http.Hijacker); ok {
				conn, _, err := hj.Hijack()
				if err == nil {
					_ = conn.Close()
				}
			}
			return
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(ts.Close)
	return ts
}

func writeFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}
