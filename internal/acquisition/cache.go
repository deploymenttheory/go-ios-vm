package acquisition

import (
	"context"
	"crypto/sha1" //nolint:gosec // Apple publishes SHA-1; content identity and storage use SHA-256.
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/unix"

	"github.com/deploymenttheory/go-ios-vm/internal/model"
)

var (
	errInvalidCacheBudget        = errors.New("invalid cache budget")
	errCacheDirectoryIsRequired  = errors.New("cache directory is required")
	errUnexpectedImageHTTPStatus = errors.New("unexpected image HTTP status")
	errInvalidImageSize          = errors.New(
		"image size unknown or exceeds cache budget",
	)
	errCacheBudgetExhausted  = errors.New("cache budget exhausted")
	errInsufficientFreeSpace = errors.New(
		"insufficient free space for image and reserve",
	)
	errDownloadLengthMismatch       = errors.New("download length mismatch")
	errAppleCatalogChecksumMismatch = errors.New("apple catalog checksum mismatch")
	errMissingVerifier              = errors.New("archive identity verifier is required")
)

type Cache struct {
	Dir     string
	Limit   int64
	Reserve int64
}
type Entry struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

func (c Cache) Lease(ctx context.Context, exclusive bool) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("Lease: Err: %w", err)
	}
	if c.Limit <= 0 || c.Reserve < 0 {
		return nil, errInvalidCacheBudget
	}
	if c.Dir == "" {
		return nil, errCacheDirectoryIsRequired
	}
	if err := os.MkdirAll(c.Dir, 0o700); err != nil {
		return nil, fmt.Errorf("Lease: os.MkdirAll: %w", err)
	}
	f, err := os.OpenFile(filepath.Join(c.Dir, ".lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("Lease: os.OpenFile: %w", err)
	}
	op := unix.LOCK_SH
	if exclusive {
		op = unix.LOCK_EX
	}
	for {
		err = unix.Flock(int(f.Fd()), op|unix.LOCK_NB)
		if err == nil {
			break
		}
		if !errors.Is(err, unix.EWOULDBLOCK) {
			f.Close()
			return nil, fmt.Errorf("acquire cache lease: %w", err)
		}
		select {
		case <-ctx.Done():
			f.Close()
			return nil, fmt.Errorf("wait for cache lease: %w", ctx.Err())
		case <-time.After(100 * time.Millisecond):
		}
	}
	return func() { _ = unix.Flock(int(f.Fd()), unix.LOCK_UN); _ = f.Close() }, nil
}

func (c Cache) Entries() ([]Entry, error) {
	items, err := os.ReadDir(c.Dir)
	if os.IsNotExist(err) {
		return []Entry{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("Entries: os.ReadDir: %w", err)
	}
	out := []Entry{}
	for _, e := range items {
		name := e.Name()
		if e.Type()&os.ModeSymlink != 0 || e.IsDir() || !strings.HasSuffix(name, ".ipsw") ||
			len(name) != 69 {
			continue
		}
		if _, err := hex.DecodeString(strings.TrimSuffix(name, ".ipsw")); err != nil {
			continue
		}
		s, err := e.Info()
		if err != nil {
			return nil, fmt.Errorf("Entries: Info: %w", err)
		}
		if !s.Mode().IsRegular() {
			continue
		}
		out = append(out, Entry{filepath.Join(c.Dir, name), s.Size()})
	}
	return out, nil
}

func (c Cache) Prune(ctx context.Context, apply bool) ([]Entry, error) {
	release, err := c.Lease(ctx, true)
	if err != nil {
		return nil, err
	}
	defer release()
	items, err := c.Entries()
	if err != nil {
		return nil, err
	}
	if apply {
		for _, e := range items {
			if err = os.Remove(e.Path); err != nil {
				return nil, fmt.Errorf("Prune: os.Remove: %w", err)
			}
		}
	}
	return items, nil
}

func (c *Client) Fetch(
	ctx context.Context,
	selected Candidate,
	cache Cache,
	verify func(context.Context, string, Candidate) error,
) (model.Artifact, error) {
	if err := AppleURL(selected.URL); err != nil {
		return model.Artifact{}, err
	}
	release, err := cache.Lease(ctx, true)
	if err != nil {
		return model.Artifact{}, err
	}
	defer release()
	req, err := http.NewRequestWithContext(ctx, "GET", selected.URL, nil)
	if err != nil {
		return model.Artifact{}, fmt.Errorf("Fetch: http.NewRequestWithContext: %w", err)
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return model.Artifact{}, fmt.Errorf("Fetch: Do: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return model.Artifact{}, fmt.Errorf("%w: %d", errUnexpectedImageHTTPStatus, res.StatusCode)
	}
	size := res.ContentLength
	if size <= 0 || size == math.MaxInt64 || size > cache.Limit || cache.Reserve < 0 {
		return model.Artifact{}, errInvalidImageSize
	}
	entries, err := cache.Entries()
	if err != nil {
		return model.Artifact{}, err
	}
	var used int64
	for _, e := range entries {
		used += e.Size
	}
	if size > cache.Limit-used {
		return model.Artifact{}, fmt.Errorf(
			"%w: need %d bytes; prune inactive images before fetching",
			errCacheBudgetExhausted,
			size,
		)
	}
	var st unix.Statfs_t
	if err = unix.Statfs(cache.Dir, &st); err != nil {
		return model.Artifact{}, fmt.Errorf("Fetch: unix.Statfs: %w", err)
	}
	blockSize := int64(st.Bsize)
	if blockSize <= 0 {
		return model.Artifact{}, errInsufficientFreeSpace
	}
	// Divide with rounding up instead of multiplying filesystem block counts.
	required := uint64(size) + uint64(cache.Reserve)
	blocks := required / uint64(blockSize)
	if required%uint64(blockSize) != 0 {
		blocks++
	}
	if blocks > st.Bavail {
		return model.Artifact{}, errInsufficientFreeSpace
	}
	f, err := os.CreateTemp(cache.Dir, "download-*.part")
	if err != nil {
		return model.Artifact{}, fmt.Errorf("Fetch: os.CreateTemp: %w", err)
	}
	defer os.Remove(f.Name())
	defer f.Close()
	h := sha256.New()
	legacy := sha1.New() //nolint:gosec // Additional Apple catalog check only; SHA-256 is our identity.
	n, err := io.Copy(io.MultiWriter(f, h, legacy), io.LimitReader(res.Body, size+1))
	if err != nil {
		return model.Artifact{}, fmt.Errorf("Fetch: io.Copy: %w", err)
	}
	if n != size {
		return model.Artifact{}, fmt.Errorf(
			"%w: got %d expected %d",
			errDownloadLengthMismatch,
			n,
			size,
		)
	}
	if selected.SHA1 != "" &&
		!strings.EqualFold(selected.SHA1, hex.EncodeToString(legacy.Sum(nil))) {
		return model.Artifact{}, errAppleCatalogChecksumMismatch
	}
	digest := hex.EncodeToString(h.Sum(nil))
	name := filepath.Join(cache.Dir, digest+".ipsw")
	if err = f.Sync(); err != nil {
		return model.Artifact{}, fmt.Errorf("Fetch: Sync: %w", err)
	}
	if err = f.Close(); err != nil {
		return model.Artifact{}, fmt.Errorf("Fetch: Close: %w", err)
	}
	if verify == nil {
		return model.Artifact{}, errMissingVerifier
	}
	if err = verify(ctx, f.Name(), selected); err != nil {
		return model.Artifact{}, fmt.Errorf("verify downloaded image: %w", err)
	}
	if err = os.Rename(f.Name(), name); err != nil {
		return model.Artifact{}, fmt.Errorf("Fetch: os.Rename: %w", err)
	}
	return model.Artifact{ID: digest, SHA256: digest, Size: n, Path: name}, nil
}
