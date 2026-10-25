package firmware

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestReadManifestAndMatch(t *testing.T) {
	data := iphoneArchive(t)
	m, err := ReadManifest(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Match(SupportedPair.IPhone); err != nil {
		t.Fatalf("iPhone archive: %v", err)
	}
	if err := m.Match(SupportedPair.CloudOS); !errors.Is(err, ErrIdentityMismatch) {
		t.Fatalf("iPhone archive as cloudOS: %v", err)
	}
	cloud := cloudArchive(t)
	m, err = ReadManifest(bytes.NewReader(cloud), int64(len(cloud)))
	if err != nil || m.Match(SupportedPair.CloudOS) != nil {
		t.Fatalf("cloudOS archive: %v %v", err, m.Match(SupportedPair.CloudOS))
	}

	tests := map[string][]byte{
		"newer iPhone build":      fakeArchive(t, "27.1", "24B5050", []string{"iPhone17,3"}, []string{"d94ap"}, 0),
		"other device":            fakeArchive(t, "27.0.1", "24A446", []string{"iPhone17,1"}, []string{"d93ap"}, 0),
		"iOS 15 cache entry":      fakeArchive(t, "15.0", "19A346", []string{"iPhone12,1", "iPhone11,8"}, []string{"n104ap", "n841ap"}, 0),
		"cloudOS without vphone":  fakeArchive(t, "26.4", "23E244", []string{"ComputeModule14,2"}, []string{"vresearch101ap"}, 0),
		"same build no vphone600": fakeArchive(t, "26.4", "23E5207q", []string{"ComputeModule14,2"}, []string{"vresearch101ap"}, 0),
	}
	for name, archive := range tests {
		m, err := ReadManifest(bytes.NewReader(archive), int64(len(archive)))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if m.Match(SupportedPair.IPhone) == nil || m.Match(SupportedPair.CloudOS) == nil {
			t.Errorf("%s matched the supported pair", name)
		}
	}
}

func TestReadManifestRejectsMalformedArchives(t *testing.T) {
	var twoManifests bytes.Buffer
	zw := zip.NewWriter(&twoManifests)
	for range 2 {
		w, _ := zw.Create(manifestName)
		_, _ = w.Write([]byte("<plist><dict/></plist>"))
	}
	_ = zw.Close()
	var noManifest, badPlist, noBuild bytes.Buffer
	for _, c := range []struct {
		buf  *bytes.Buffer
		name string
		body string
	}{
		{&noManifest, "Restore.plist", "x"},
		{&badPlist, manifestName, "<plist><dict><key>ProductVersion</key>"},
		{&noBuild, manifestName, `<?xml version="1.0"?><plist version="1.0"><dict><key>ProductVersion</key><string>27.0.1</string></dict></plist>`},
	} {
		zw := zip.NewWriter(c.buf)
		w, _ := zw.Create(c.name)
		_, _ = w.Write([]byte(c.body))
		_ = zw.Close()
	}
	full := iphoneArchive(t)
	tests := []struct {
		name string
		data []byte
		want error
	}{
		{"not a zip", []byte("not a zip archive"), ErrNotArchive},
		{"truncated", full[:len(full)/2], ErrNotArchive},
		{"two manifests", twoManifests.Bytes(), ErrManifest},
		{"no manifest", noManifest.Bytes(), ErrManifest},
		{"malformed plist", badPlist.Bytes(), ErrManifest},
		{"no build", noBuild.Bytes(), ErrManifest},
	}
	for _, tt := range tests {
		if _, err := ReadManifest(bytes.NewReader(tt.data), int64(len(tt.data))); !errors.Is(err, tt.want) {
			t.Errorf("%s: %v, want %v", tt.name, err, tt.want)
		}
	}
}

func TestSelectIPSWMe(t *testing.T) {
	const response = `{
	  "name": "iPhone 16 Pro Max", "identifier": "iPhone17,3",
	  "boards": [{"boardconfig": "D94AP", "platform": "t8140", "cpid": 33088, "bdid": 14}],
	  "firmwares": [
	    {"identifier": "iPhone17,3", "version": "27.1", "buildid": "24B5050", "sha256sum": "aa", "filesize": 1, "url": "https://updates.cdn-apple.com/newer.ipsw", "signed": true},
	    {"identifier": "iPhone17,3", "version": "27.0.1", "buildid": "24A446", "sha256sum": "fc87bb9da8c03d0154d87531950f4a170195f6663ee5208653fa7a26bafb9469", "filesize": 12343857610, "url": "https://updates.cdn-apple.com/2026FallFCS/38dca0ee-bb5d-4132-ad13-62d57bcd6d32/iPhone17,3_27.0.1_24A446_Restore.ipsw", "signed": true}
	  ]}`
	at := time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC)
	src, err := SelectIPSWMe(SupportedPair.IPhone, []byte(response), at)
	if err != nil {
		t.Fatal(err)
	}
	known, _ := KnownSource(SupportedPair.IPhone)
	if src.URL != known.URL || src.Size != known.Size || src.SHA256 != known.SHA256 {
		t.Fatalf("selected %+v, want the known source", src)
	}
	if src.Selection.Provider != "ipsw.me v4" || src.Selection.RetrievedAt != at || len(src.Selection.ResponseSHA256) != 64 ||
		!strings.Contains(string(src.Selection.Raw), `"buildid": "24A446"`) {
		t.Fatalf("selection metadata %+v", src.Selection)
	}

	stringBoards := strings.Replace(response, `[{"boardconfig": "D94AP", "platform": "t8140", "cpid": 33088, "bdid": 14}]`, `["D94AP"]`, 1)
	if _, err := SelectIPSWMe(SupportedPair.IPhone, []byte(stringBoards), at); err != nil {
		t.Errorf("string-shaped boards: %v", err)
	}
	duplicate := strings.Replace(response, `"version": "27.1", "buildid": "24B5050"`, `"version": "27.0.1", "buildid": "24A446"`, 1)
	if _, err := SelectIPSWMe(SupportedPair.IPhone, []byte(duplicate), at); !errors.Is(err, ErrAmbiguous) {
		t.Errorf("two matching firmwares: %v, want ErrAmbiguous", err)
	}
	missing := strings.Replace(response, `"buildid": "24A446"`, `"buildid": "24A447"`, 1)
	if _, err := SelectIPSWMe(SupportedPair.IPhone, []byte(missing), at); !errors.Is(err, ErrNoSource) {
		t.Errorf("build absent: %v, want ErrNoSource rather than a substitute", err)
	}
	other := strings.Replace(response, `"identifier": "iPhone17,3",`, `"identifier": "iPhone17,1",`, 1)
	if _, err := SelectIPSWMe(SupportedPair.IPhone, []byte(other), at); !errors.Is(err, ErrNoSource) {
		t.Errorf("other device response: %v", err)
	}
	http := strings.Replace(response, `"https://updates.cdn-apple.com/2026FallFCS`, `"http://updates.cdn-apple.com/2026FallFCS`, 1)
	if _, err := SelectIPSWMe(SupportedPair.IPhone, []byte(http), at); !errors.Is(err, ErrNoSource) {
		t.Errorf("plain http URL: %v", err)
	}
}

func newCache(t *testing.T) *Cache {
	t.Helper()
	return &Cache{Dir: filepath.Join(t.TempDir(), "ipsw")}
}

func TestDownloadPublishesOnlyVerifiedArchives(t *testing.T) {
	body := iphoneArchive(t)
	srv := &server{body: body}
	ts := srv.start(t)
	c := newCache(t)
	src := Source{Identity: SupportedPair.IPhone, URL: ts.URL + "/a.ipsw", Size: int64(len(body)), SHA256: digest(body),
		Selection: Selection{Provider: "test"}}

	a, err := c.Get(context.Background(), SupportedPair.IPhone, &src, nil)
	if err != nil {
		t.Fatal(err)
	}
	if a.SHA256 != digest(body) || filepath.Base(a.Path) != digest(body)+".ipsw" {
		t.Fatalf("archive %+v", a)
	}
	var rec Record
	data, err := os.ReadFile(filepath.Join(c.Dir, a.SHA256+".json"))
	if err != nil || json.Unmarshal(data, &rec) != nil {
		t.Fatalf("record: %v", err)
	}
	if rec.Identity != SupportedPair.IPhone || rec.Manifest.ProductBuildVersion != "24A446" || rec.Source.URL != src.URL ||
		rec.RetrievedAt.IsZero() || rec.Source.Selection.Provider != "test" {
		t.Fatalf("record %+v", rec)
	}

	// A second Get uses the verified entry without a request.
	before := srv.requests.Load()
	if _, err := c.Get(context.Background(), SupportedPair.IPhone, &src, nil); err != nil {
		t.Fatal(err)
	}
	if srv.requests.Load() != before {
		t.Fatal("a verified cache entry was downloaded again")
	}
	if entries, _ := os.ReadDir(filepath.Join(c.Dir, stagingDir)); len(entries) != 0 {
		t.Fatalf("staging not empty: %v", entries)
	}
}

func TestInterruptedDownloadResumesAndNeverPublishesEarly(t *testing.T) {
	body := iphoneArchive(t)
	srv := &server{body: body}
	srv.cutAfter.Store(int64(len(body) / 3))
	ts := srv.start(t)
	c := newCache(t)
	src := Source{Identity: SupportedPair.IPhone, URL: ts.URL + "/a.ipsw", Size: int64(len(body)), SHA256: digest(body)}

	if _, err := c.Get(context.Background(), SupportedPair.IPhone, &src, nil); !errors.Is(err, ErrIncomplete) {
		t.Fatalf("interrupted download: %v, want ErrIncomplete", err)
	}
	if entries, _ := c.Entries(); len(entries) != 0 {
		t.Fatalf("interrupted download published %v", entries)
	}
	part := filepath.Join(c.Dir, stagingDir, "24A446.part")
	if info, err := os.Stat(part); err != nil || info.Size() != int64(len(body)/3) {
		t.Fatalf("partial download not kept: %v", err)
	}
	a, err := c.Get(context.Background(), SupportedPair.IPhone, &src, nil)
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	if a.SHA256 != digest(body) || len(srv.ranges) != 1 || srv.ranges[0] != "bytes="+itoa(len(body)/3)+"-" {
		t.Fatalf("resume used ranges %v", srv.ranges)
	}
}

func TestIgnoredRangeRestartsTheDownload(t *testing.T) {
	body := iphoneArchive(t)
	srv := &server{body: body, ignoreRange: true}
	srv.cutAfter.Store(int64(len(body) / 2))
	ts := srv.start(t)
	c := newCache(t)
	src := Source{Identity: SupportedPair.IPhone, URL: ts.URL + "/a.ipsw", Size: int64(len(body)), SHA256: digest(body)}
	if _, err := c.Get(context.Background(), SupportedPair.IPhone, &src, nil); !errors.Is(err, ErrIncomplete) {
		t.Fatal(err)
	}
	a, err := c.Get(context.Background(), SupportedPair.IPhone, &src, nil)
	if err != nil || a.SHA256 != digest(body) {
		t.Fatalf("restart after an ignored range: %v", err)
	}
}

func TestRejectedDownloadsAreNotPublished(t *testing.T) {
	body := iphoneArchive(t)
	wrong := fakeArchive(t, "15.0", "19A346", []string{"iPhone12,1"}, []string{"n104ap"}, 1024)
	tests := []struct {
		name string
		body []byte
		src  func(url string) Source
		want error
	}{
		{"digest mismatch", body, func(u string) Source {
			return Source{Identity: SupportedPair.IPhone, URL: u, Size: int64(len(body)), SHA256: strings.Repeat("0", 64)}
		}, ErrDigest},
		{"size mismatch", body, func(u string) Source {
			return Source{Identity: SupportedPair.IPhone, URL: u, Size: int64(len(body)) + 1}
		}, ErrSize},
		{"wrong identity", wrong, func(u string) Source { return Source{Identity: SupportedPair.IPhone, URL: u} }, ErrIdentityMismatch},
		{"not an archive", []byte("<html>error</html>"), func(u string) Source { return Source{Identity: SupportedPair.IPhone, URL: u} }, ErrNotArchive},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := (&server{body: tt.body}).start(t)
			c := newCache(t)
			src := tt.src(ts.URL + "/a.ipsw")
			if _, err := c.Get(context.Background(), SupportedPair.IPhone, &src, nil); !errors.Is(err, tt.want) {
				t.Fatalf("Get = %v, want %v", err, tt.want)
			}
			if entries, _ := c.Entries(); len(entries) != 0 {
				t.Fatalf("published %v", entries)
			}
		})
	}
}

func TestNoSpaceStopsBeforeDownloading(t *testing.T) {
	body := iphoneArchive(t)
	srv := &server{body: body}
	ts := srv.start(t)
	c := newCache(t)
	c.Free = func(string) (int64, error) { return int64(len(body)) - 1, nil }
	src := Source{Identity: SupportedPair.IPhone, URL: ts.URL + "/a.ipsw", Size: int64(len(body))}
	if _, err := c.Get(context.Background(), SupportedPair.IPhone, &src, nil); !errors.Is(err, ErrNoSpace) {
		t.Fatalf("Get = %v, want ErrNoSpace", err)
	}
	if srv.requests.Load() != 0 {
		t.Fatal("downloaded without enough space")
	}
}

func TestCanceledDownloadIsNotPublished(t *testing.T) {
	body := iphoneArchive(t)
	ts := (&server{body: body}).start(t)
	c := newCache(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	src := Source{Identity: SupportedPair.IPhone, URL: ts.URL + "/a.ipsw", Size: int64(len(body))}
	if _, err := c.Get(ctx, SupportedPair.IPhone, &src, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("Get = %v, want context.Canceled", err)
	}
	if entries, _ := c.Entries(); len(entries) != 0 {
		t.Fatalf("published %v", entries)
	}
}

func TestExistingEntriesAreUntouchedAndNeverSelected(t *testing.T) {
	c := newCache(t)
	ios15 := fakeArchive(t, "15.0", "19A346", []string{"iPhone12,1", "iPhone11,8"}, []string{"n104ap", "n841ap"}, 4096)
	ios14 := fakeArchive(t, "14.0", "18A373", []string{"iPhone12,1", "iPhone11,8"}, []string{"n104ap", "n841ap"}, 4096)
	foreign := map[string][]byte{digest(ios15) + ".ipsw": ios15, digest(ios14) + ".ipsw": ios14}
	for name, data := range foreign {
		writeFile(t, filepath.Join(c.Dir, name), data)
	}
	writeFile(t, filepath.Join(c.Dir, ".lock"), nil)
	stat := func() map[string]os.FileInfo {
		out := map[string]os.FileInfo{}
		for name := range foreign {
			info, err := os.Stat(filepath.Join(c.Dir, name))
			if err != nil {
				t.Fatal(err)
			}
			out[name] = info
		}
		return out
	}
	before := stat()

	body := iphoneArchive(t)
	ts := (&server{body: body}).start(t)
	src := Source{Identity: SupportedPair.IPhone, URL: ts.URL + "/a.ipsw", Size: int64(len(body))}
	if _, err := c.Get(context.Background(), SupportedPair.IPhone, &src, nil); err != nil {
		t.Fatal(err)
	}
	for name, info := range stat() {
		b := before[name]
		data, _ := os.ReadFile(filepath.Join(c.Dir, name))
		if info.Size() != b.Size() || !info.ModTime().Equal(b.ModTime()) || !bytes.Equal(data, foreign[name]) {
			t.Errorf("existing entry %s changed", name)
		}
	}
	entries, err := c.Entries()
	if err != nil {
		t.Fatal(err)
	}
	verified := 0
	for _, e := range entries {
		if e.Record != nil {
			verified++
		}
	}
	if len(entries) != 3 || verified != 1 {
		t.Fatalf("entries %+v", entries)
	}

	// A foreign archive that really is iOS 15 cannot satisfy the target, even
	// when offered by name as a local source.
	decoy := filepath.Join(t.TempDir(), "iPhone17,3_27.0.1_24A446_Restore.ipsw")
	writeFile(t, decoy, ios15)
	local := Source{Identity: SupportedPair.IPhone, Path: decoy}
	if _, err := (&Cache{Dir: t.TempDir()}).Get(context.Background(), SupportedPair.IPhone, &local, nil); !errors.Is(err, ErrIdentityMismatch) {
		t.Fatalf("decoy local archive: %v, want ErrIdentityMismatch", err)
	}
}

func TestLocalArchiveIsVerifiedInPlace(t *testing.T) {
	body := iphoneArchive(t)
	path := filepath.Join(t.TempDir(), "any-name.zip")
	writeFile(t, path, body)
	c := newCache(t)
	a, err := c.Get(context.Background(), SupportedPair.IPhone, &Source{Identity: SupportedPair.IPhone, Path: path}, nil)
	if err != nil || a.Path != path || a.SHA256 != digest(body) {
		t.Fatalf("local archive: %+v, %v", a, err)
	}
	if entries, _ := c.Entries(); len(entries) != 0 {
		t.Fatal("a local archive was copied into the cache")
	}
	bad := Source{Identity: SupportedPair.IPhone, Path: path, SHA256: strings.Repeat("1", 64)}
	if _, err := c.Get(context.Background(), SupportedPair.IPhone, &bad, nil); !errors.Is(err, ErrDigest) {
		t.Fatalf("local archive with the wrong digest: %v", err)
	}
}

func TestTamperedCacheEntryIsReported(t *testing.T) {
	body := iphoneArchive(t)
	ts := (&server{body: body}).start(t)
	c := newCache(t)
	src := Source{Identity: SupportedPair.IPhone, URL: ts.URL + "/a.ipsw"}
	a, err := c.Get(context.Background(), SupportedPair.IPhone, &src, nil)
	if err != nil {
		t.Fatal(err)
	}
	tampered := append([]byte(nil), body...)
	tampered[100] ^= 0xFF
	writeFile(t, a.Path, tampered)
	if _, err := c.Get(context.Background(), SupportedPair.IPhone, &src, nil); !errors.Is(err, ErrCacheEntry) {
		t.Fatalf("tampered entry: %v, want ErrCacheEntry", err)
	}
}

func TestCacheLockExcludesConcurrentPublishers(t *testing.T) {
	c := newCache(t)
	unlock, err := c.lock()
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	if _, err := c.Get(context.Background(), SupportedPair.IPhone, &Source{Identity: SupportedPair.IPhone, Path: "x"}, nil); !errors.Is(err, ErrCacheBusy) {
		t.Fatalf("Get while locked: %v, want ErrCacheBusy", err)
	}
}

func TestResolveDownloadsThePair(t *testing.T) {
	iphone, cloud := iphoneArchive(t), cloudArchive(t)
	tsI, tsC := (&server{body: iphone}).start(t), (&server{body: cloud}).start(t)
	c := newCache(t)
	if _, err := c.Get(context.Background(), SupportedPair.IPhone, &Source{Identity: SupportedPair.IPhone, URL: tsI.URL}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Get(context.Background(), SupportedPair.CloudOS, &Source{Identity: SupportedPair.CloudOS, URL: tsC.URL}, nil); err != nil {
		t.Fatal(err)
	}
	a, b, err := c.Resolve(context.Background(), SupportedPair, nil)
	if err != nil || a.SHA256 != digest(iphone) || b.SHA256 != digest(cloud) {
		t.Fatalf("Resolve: %v", err)
	}
}

func itoa(n int) string { return strconv.Itoa(n) }
