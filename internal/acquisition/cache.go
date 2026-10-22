package acquisition

import (
	"context"
	"crypto/sha1" //nolint:gosec // Apple publishes SHA-1; content identity and storage use SHA-256.
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/unix"

	"github.com/deploymenttheory/go-ios-vm/internal/model"
)

var (
	errInvalidCacheBudget       = errors.New("invalid cache budget")
	errCacheDirectoryIsRequired = errors.New("cache directory is required")
	errInvalidImageSize         = errors.New(
		"image size unknown or exceeds cache budget",
	)
	errCacheBudgetExhausted  = errors.New("cache budget exhausted")
	errInsufficientFreeSpace = errors.New(
		"insufficient free space for image and reserve",
	)
	errDownloadLengthMismatch   = errors.New("download length mismatch")
	errCatalogSizeMismatch      = errors.New("catalog size differs from Apple CDN")
	errCDNObjectChanged         = errors.New("apple CDN object changed after manifest probe")
	errDeclaredChecksumMismatch = errors.New("declared image checksum mismatch")
	errMissingVerifier          = errors.New("archive identity verifier is required")
	errUnpinnedFetch            = errors.New("full fetch requires probed size and ETag")
	errPartialNotRegular        = errors.New("managed partial is not a regular file")
	errRangeResumeMismatch      = errors.New("resumed image range does not match pinned object")
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
		if e.Type()&os.ModeSymlink != 0 || e.IsDir() || len(name) != 69 ||
			(!strings.HasSuffix(name, ".ipsw") && !strings.HasSuffix(name, ".part")) {
			continue
		}
		if _, err := hex.DecodeString(name[:64]); err != nil {
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

func (c Cache) ensureSpace(remaining int64) error {
	if remaining < 0 || c.Reserve < 0 {
		return errInvalidImageSize
	}
	entries, err := c.Entries()
	if err != nil {
		return err
	}
	var used int64
	for _, entry := range entries {
		used += entry.Size
	}
	if remaining > c.Limit-used {
		return fmt.Errorf(
			"%w: need %d bytes; prune inactive images before fetching",
			errCacheBudgetExhausted, remaining,
		)
	}
	var space unix.Statfs_t
	if err := unix.Statfs(c.Dir, &space); err != nil {
		return fmt.Errorf("ensureSpace: unix.Statfs: %w", err)
	}
	blockSize := int64(space.Bsize)
	if blockSize <= 0 {
		return errInsufficientFreeSpace
	}
	required := uint64(remaining) + uint64(c.Reserve)
	blocks := required / uint64(blockSize)
	if required%uint64(blockSize) != 0 {
		blocks++
	}
	if blocks > space.Bavail {
		return errInsufficientFreeSpace
	}
	return nil
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
	if selected.Size <= 0 || selected.ETag == "" {
		return model.Artifact{}, errUnpinnedFetch
	}
	if verify == nil {
		return model.Artifact{}, errMissingVerifier
	}
	if selected.Size > cache.Limit || cache.Reserve < 0 {
		return model.Artifact{}, errInvalidImageSize
	}
	release, err := cache.Lease(ctx, true)
	if err != nil {
		return model.Artifact{}, err
	}
	defer release()
	stageID := sha256.Sum256([]byte(selected.URL + "\x00" + selected.ETag))
	stage := filepath.Join(cache.Dir, hex.EncodeToString(stageID[:])+".part")
	fd, err := unix.Open(stage, unix.O_CREAT|unix.O_RDWR|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		return model.Artifact{}, fmt.Errorf("Fetch: open stage: %w", err)
	}
	f := os.NewFile(uintptr(fd), stage)
	defer f.Close()
	defer func() {
		if info, statErr := os.Stat(stage); statErr == nil && info.Size() == 0 {
			_ = os.Remove(stage)
		}
	}()
	stat, err := f.Stat()
	if err != nil {
		return model.Artifact{}, fmt.Errorf("Fetch: stage Stat: %w", err)
	}
	if !stat.Mode().IsRegular() || stat.Size() > selected.Size {
		return model.Artifact{}, errPartialNotRegular
	}
	completed := stat.Size()
	remaining := selected.Size - completed
	if err := cache.ensureSpace(remaining); err != nil {
		return model.Artifact{}, err
	}
	if remaining > 0 {
		if err = c.appendPinned(ctx, selected, f, completed); err != nil {
			if syncErr := f.Sync(); syncErr != nil {
				return model.Artifact{}, fmt.Errorf(
					"Fetch: sync partial after %w: %w",
					err,
					syncErr,
				)
			}
			return model.Artifact{}, err
		}
	}
	if err = f.Sync(); err != nil {
		return model.Artifact{}, fmt.Errorf("Fetch: stage Sync: %w", err)
	}
	if _, err = f.Seek(0, io.SeekStart); err != nil {
		return model.Artifact{}, fmt.Errorf("Fetch: stage Seek: %w", err)
	}
	h := sha256.New()
	legacy := sha1.New() //nolint:gosec // Additional historical-index check; SHA-256 is our identity.
	n, err := io.Copy(io.MultiWriter(h, legacy), f)
	if err != nil {
		return model.Artifact{}, fmt.Errorf("Fetch: hash stage: %w", err)
	}
	if n != selected.Size {
		return model.Artifact{}, errDownloadLengthMismatch
	}
	if selected.SHA1 != "" &&
		!strings.EqualFold(selected.SHA1, hex.EncodeToString(legacy.Sum(nil))) {
		_ = os.Remove(stage)
		return model.Artifact{}, errDeclaredChecksumMismatch
	}
	if err = ctx.Err(); err != nil {
		return model.Artifact{}, fmt.Errorf("Fetch: context: %w", err)
	}
	if err = verify(ctx, stage, selected); err != nil {
		_ = os.Remove(stage)
		return model.Artifact{}, fmt.Errorf("verify downloaded image: %w", err)
	}
	digest := hex.EncodeToString(h.Sum(nil))
	name := filepath.Join(cache.Dir, digest+".ipsw")
	if err = os.Rename(stage, name); err != nil {
		return model.Artifact{}, fmt.Errorf("Fetch: os.Rename: %w", err)
	}
	return model.Artifact{ID: digest, SHA256: digest, Size: n, Path: name}, nil
}

func (c *Client) appendPinned(
	ctx context.Context,
	selected Candidate,
	stage *os.File,
	offset int64,
) error {
	if _, err := stage.Seek(offset, io.SeekStart); err != nil {
		return fmt.Errorf("appendPinned: Seek: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, selected.URL, nil)
	if err != nil {
		return fmt.Errorf("appendPinned: NewRequestWithContext: %w", err)
	}
	req.Header.Set("If-Match", selected.ETag)
	req.Header.Set("Accept-Encoding", "identity")
	if offset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("appendPinned: Do: %w", err)
	}
	defer res.Body.Close()
	wantStatus := http.StatusOK
	if offset > 0 {
		wantStatus = http.StatusPartialContent
		expected := fmt.Sprintf("bytes %d-%d/%d", offset, selected.Size-1, selected.Size)
		if res.Header.Get("Content-Range") != expected {
			return errRangeResumeMismatch
		}
	}
	if res.Header.Get("ETag") != selected.ETag {
		return errCDNObjectChanged
	}
	if res.ContentLength != selected.Size-offset {
		return fmt.Errorf(
			"%w: expected %d, got %d",
			errCatalogSizeMismatch,
			selected.Size-offset,
			res.ContentLength,
		)
	}
	if res.StatusCode != wantStatus {
		return errRangeResumeMismatch
	}
	written, err := io.Copy(stage, io.LimitReader(res.Body, selected.Size-offset+1))
	if err != nil {
		return fmt.Errorf("appendPinned: Copy: %w", err)
	}
	if written != selected.Size-offset {
		return errDownloadLengthMismatch
	}
	return nil
}
