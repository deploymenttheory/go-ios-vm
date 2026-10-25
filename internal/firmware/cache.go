package firmware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/deploymenttheory/go-ios-vm/internal/flock"
	"github.com/deploymenttheory/go-ios-vm/internal/progress"
)

// Errors from the cache.
var (
	ErrCacheEntry = errors.New("cache entry failed verification")
	ErrCacheBusy  = errors.New("IPSW cache is in use by another process")
)

const (
	recordSuffix  = ".json"
	archiveSuffix = ".ipsw"
	stagingDir    = ".staging"
	cacheLock     = ".lock"
)

// Record is the metadata published beside a verified archive.
type Record struct {
	SchemaVersion int       `json:"schema_version"`
	Identity      Identity  `json:"identity"`
	SHA256        string    `json:"sha256"`
	Size          int64     `json:"size"`
	RetrievedAt   time.Time `json:"retrieved_at"`
	Source        Source    `json:"source"`
	Manifest      Manifest  `json:"manifest"`
	DeviceClasses []string  `json:"device_classes"`
}

// Cache is the IPSW cache directory. Verified archives are stored by digest
// as <sha256>.ipsw with a <sha256>.json record. Files it did not publish,
// including archives from earlier tools, are never opened for writing,
// renamed or removed, and never satisfy a selection.
type Cache struct {
	Dir    string
	Client *http.Client
	// Free reports free space in Dir; nil uses DirFree.
	Free func(dir string) (int64, error)
}

// Resolve returns verified archives for both builds of pair, using a verified
// cache entry, the given local or remote source, or the known source, in that
// order. It implements Resolver.
func (c *Cache) Resolve(ctx context.Context, pair Pair, report progress.Reporter) (Archive, Archive, error) {
	iphone, err := c.Get(ctx, pair.IPhone, nil, report)
	if err != nil {
		return Archive{}, Archive{}, err
	}
	cloud, err := c.Get(ctx, pair.CloudOS, nil, report)
	if err != nil {
		return Archive{}, Archive{}, err
	}
	return iphone, cloud, nil
}

// Get returns a verified archive of exactly id. A verified cache entry is used
// when present; otherwise src (or the known source when src is nil) is
// verified and, for a remote source, published into the cache.
func (c *Cache) Get(ctx context.Context, id Identity, src *Source, report progress.Reporter) (Archive, error) {
	if report == nil {
		report = progress.Discard
	}
	unlock, err := c.lock()
	if err != nil {
		return Archive{}, err
	}
	defer unlock()

	if a, ok, err := c.lookup(id); err != nil || ok {
		return a, err
	}
	if src == nil {
		known, err := KnownSource(id)
		if err != nil {
			return Archive{}, err
		}
		src = &known
	}
	if src.Identity != id {
		return Archive{}, fmt.Errorf("%w: source is for %s, want %s", ErrIdentityMismatch, src.Identity, id)
	}
	if src.Path != "" {
		return verifyLocal(*src)
	}
	return c.fetch(ctx, *src, report)
}

// verifyLocal verifies a local archive in place; its file name is irrelevant.
func verifyLocal(src Source) (Archive, error) {
	size, sum, m, err := inspect(src.Path)
	if err != nil {
		return Archive{}, err
	}
	if err := check(src, size, sum, m); err != nil {
		return Archive{}, err
	}
	return Archive{Identity: src.Identity, Path: src.Path, SHA256: sum}, nil
}

// fetch downloads into staging, verifies size, digest and manifest identity,
// and only then publishes the archive and its record.
func (c *Cache) fetch(ctx context.Context, src Source, report progress.Reporter) (Archive, error) {
	staging := filepath.Join(c.Dir, stagingDir)
	if err := os.MkdirAll(staging, 0o700); err != nil {
		return Archive{}, fmt.Errorf("create staging: %w", err)
	}
	part := filepath.Join(staging, src.Identity.Build+".part")
	client := c.Client
	if client == nil {
		client = http.DefaultClient
	}
	measure := c.Free
	if measure == nil {
		measure = DirFree
	}
	free := func() (int64, error) { return measure(c.Dir) }
	size, sum, err := download(ctx, client, src, part, free, report)
	if err != nil {
		if errors.Is(err, ErrSize) {
			_ = os.Remove(part) //nolint:gosec // the staging file this call owns
		}
		return Archive{}, err
	}
	m, err := manifestOf(part, size)
	if err == nil {
		err = check(src, size, sum, m)
	}
	if err != nil {
		_ = os.Remove(part) //nolint:gosec // the staging file this call owns
		return Archive{}, err
	}
	rec := Record{
		SchemaVersion: 1, Identity: src.Identity, SHA256: sum, Size: size, RetrievedAt: time.Now().UTC(),
		Source: src, Manifest: m, DeviceClasses: m.DeviceClasses(),
	}
	path, err := c.publish(part, rec)
	if err != nil {
		return Archive{}, err
	}
	return Archive{Identity: src.Identity, Path: path, SHA256: sum}, nil
}

func (c *Cache) publish(part string, rec Record) (string, error) {
	archive := filepath.Join(c.Dir, rec.SHA256+archiveSuffix)
	if _, err := os.Stat(archive); err == nil {
		// The same bytes are already present; keep that file.
		_ = os.Remove(part) //nolint:gosec // the staging file this call owns
	} else if err := os.Rename(part, archive); err != nil { //nolint:gosec // inside the cache
		return "", fmt.Errorf("publish archive: %w", err)
	}
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encode cache record: %w", err)
	}
	tmp := filepath.Join(c.Dir, stagingDir, rec.SHA256+recordSuffix)
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return "", fmt.Errorf("write cache record: %w", err)
	}
	if err := os.Rename(tmp, filepath.Join(c.Dir, rec.SHA256+recordSuffix)); err != nil { //nolint:gosec // inside the cache
		return "", fmt.Errorf("publish cache record: %w", err)
	}
	return archive, nil
}

// lookup finds a published entry for id and re-verifies it: size, digest and
// manifest identity must still match its record.
func (c *Cache) lookup(id Identity) (Archive, bool, error) {
	for _, rec := range c.records() {
		if rec.Identity != id {
			continue
		}
		path := filepath.Join(c.Dir, rec.SHA256+archiveSuffix)
		size, sum, m, err := inspect(path)
		if err != nil {
			return Archive{}, false, fmt.Errorf("%w: %s: %w", ErrCacheEntry, path, err)
		}
		if size != rec.Size || sum != rec.SHA256 {
			return Archive{}, false, fmt.Errorf("%w: %s is %d bytes with SHA-256 %s, recorded %d bytes with %s",
				ErrCacheEntry, path, size, sum, rec.Size, rec.SHA256)
		}
		if err := m.Match(id); err != nil {
			return Archive{}, false, fmt.Errorf("%w: %s: %w", ErrCacheEntry, path, err)
		}
		return Archive{Identity: id, Path: path, SHA256: sum}, true, nil
	}
	return Archive{}, false, nil
}

// records reads every well-formed record. Unreadable records are skipped: they
// cannot make an entry verified.
func (c *Cache) records() []Record {
	entries, err := os.ReadDir(c.Dir)
	if err != nil {
		return nil
	}
	var out []Record
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, recordSuffix) || !e.Type().IsRegular() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(c.Dir, name)) //nolint:gosec // inside the cache
		if err != nil {
			continue
		}
		var rec Record
		if json.Unmarshal(data, &rec) != nil || rec.SchemaVersion != 1 || rec.SHA256+recordSuffix != name {
			continue
		}
		out = append(out, rec)
	}
	return out
}

// Entry is one archive file in the cache.
type Entry struct {
	Name string
	Size int64
	// Record is set only for entries this cache published.
	Record *Record
}

// Entries lists the cache's archives without reading their contents.
func (c *Cache) Entries() ([]Entry, error) {
	des, err := os.ReadDir(c.Dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read IPSW cache: %w", err)
	}
	records := map[string]Record{}
	for _, r := range c.records() {
		records[r.SHA256+archiveSuffix] = r
	}
	var out []Entry
	for _, d := range des {
		if !strings.HasSuffix(d.Name(), archiveSuffix) || !d.Type().IsRegular() {
			continue
		}
		info, err := d.Info()
		if err != nil {
			return nil, fmt.Errorf("read IPSW cache: %w", err)
		}
		e := Entry{Name: d.Name(), Size: info.Size()}
		if r, ok := records[d.Name()]; ok {
			e.Record = &r
		}
		out = append(out, e)
	}
	return out, nil
}

func inspect(path string) (size int64, sum string, m Manifest, err error) {
	f, err := os.Open(path) //nolint:gosec // a cache entry or a user-selected archive, opened read-only
	if err != nil {
		return 0, "", Manifest{}, fmt.Errorf("open archive: %w", err)
	}
	defer f.Close() //nolint:errcheck // read-only
	info, err := f.Stat()
	if err != nil {
		return 0, "", Manifest{}, fmt.Errorf("stat archive: %w", err)
	}
	if !info.Mode().IsRegular() {
		return 0, "", Manifest{}, fmt.Errorf("%w: %s is not a regular file", ErrNotArchive, path)
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return 0, "", Manifest{}, fmt.Errorf("hash archive: %w", err)
	}
	m, err = ReadManifest(f, info.Size())
	if err != nil {
		return 0, "", Manifest{}, err
	}
	return info.Size(), hex.EncodeToString(h.Sum(nil)), m, nil
}

func manifestOf(path string, size int64) (Manifest, error) {
	f, err := os.Open(path) //nolint:gosec // the staging file this call owns
	if err != nil {
		return Manifest{}, fmt.Errorf("open archive: %w", err)
	}
	defer f.Close() //nolint:errcheck // read-only
	return ReadManifest(f, size)
}

// check compares an archive with its source's expectations and identity.
func check(src Source, size int64, sum string, m Manifest) error {
	if src.Size > 0 && size != src.Size {
		return fmt.Errorf("%w: %s is %d bytes, want %d", ErrSize, src.Identity, size, src.Size)
	}
	if src.SHA256 != "" && !strings.EqualFold(sum, src.SHA256) {
		return fmt.Errorf("%w: %s has SHA-256 %s, want %s", ErrDigest, src.Identity, sum, src.SHA256)
	}
	return m.Match(src.Identity)
}

var _ Resolver = (*Cache)(nil)

// lock takes the cache's OS lock so that only one process publishes at a time.
func (c *Cache) lock() (func(), error) {
	if err := os.MkdirAll(c.Dir, 0o700); err != nil {
		return nil, fmt.Errorf("create IPSW cache: %w", err)
	}
	f, err := flock.TryLock(filepath.Join(c.Dir, cacheLock))
	if errors.Is(err, flock.ErrHeld) {
		return nil, fmt.Errorf("%w: %s", ErrCacheBusy, c.Dir)
	}
	if err != nil {
		return nil, fmt.Errorf("lock IPSW cache: %w", err)
	}
	return func() { _ = f.Close() }, nil
}
