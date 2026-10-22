package acquisition

import (
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/deploymenttheory/go-ios-vm/internal/model"
	"golang.org/x/sys/unix"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
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
		return nil, err
	}
	if c.Limit <= 0 || c.Reserve < 0 {
		return nil, fmt.Errorf("invalid cache budget")
	}
	if c.Dir == "" {
		return nil, fmt.Errorf("cache directory is required")
	}
	if err := os.MkdirAll(c.Dir, 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(c.Dir, ".lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
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
		if err != unix.EWOULDBLOCK {
			f.Close()
			return nil, err
		}
		select {
		case <-ctx.Done():
			f.Close()
			return nil, ctx.Err()
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
		return nil, err
	}
	out := []Entry{}
	for _, e := range items {
		name := e.Name()
		if e.Type()&os.ModeSymlink != 0 || e.IsDir() || !strings.HasSuffix(name, ".ipsw") || len(name) != 69 {
			continue
		}
		if _, err := hex.DecodeString(strings.TrimSuffix(name, ".ipsw")); err != nil {
			continue
		}
		s, err := e.Info()
		if err != nil {
			return nil, err
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
				return nil, err
			}
		}
	}
	return items, nil
}
func (c *Client) Fetch(ctx context.Context, selected Candidate, cache Cache, verify func(context.Context, string, Candidate) error) (model.Artifact, error) {
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
		return model.Artifact{}, err
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return model.Artifact{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return model.Artifact{}, fmt.Errorf("image HTTP %d", res.StatusCode)
	}
	size := res.ContentLength
	if size <= 0 || size > cache.Limit {
		return model.Artifact{}, fmt.Errorf("image size unknown or exceeds cache budget")
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
		return model.Artifact{}, fmt.Errorf("cache needs %d bytes; prune inactive images before fetching", size)
	}
	var st unix.Statfs_t
	if err = unix.Statfs(cache.Dir, &st); err != nil {
		return model.Artifact{}, err
	}
	if uint64(size+cache.Reserve) > st.Bavail*uint64(st.Bsize) {
		return model.Artifact{}, fmt.Errorf("insufficient free space for image and reserve")
	}
	f, err := os.CreateTemp(cache.Dir, "download-*.part")
	if err != nil {
		return model.Artifact{}, err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	h := sha256.New()
	legacy := sha1.New() // Apple catalog SHA-1 is an additional declared checksum, not our identity.
	n, err := io.Copy(io.MultiWriter(f, h, legacy), io.LimitReader(res.Body, size+1))
	if err != nil {
		return model.Artifact{}, err
	}
	if n != size {
		return model.Artifact{}, fmt.Errorf("download length mismatch: got %d expected %d", n, size)
	}
	if selected.SHA1 != "" && !strings.EqualFold(selected.SHA1, hex.EncodeToString(legacy.Sum(nil))) {
		return model.Artifact{}, fmt.Errorf("Apple catalog checksum mismatch")
	}
	digest := hex.EncodeToString(h.Sum(nil))
	name := filepath.Join(cache.Dir, digest+".ipsw")
	if err = f.Sync(); err != nil {
		return model.Artifact{}, err
	}
	if err = f.Close(); err != nil {
		return model.Artifact{}, err
	}
	if verify == nil {
		return model.Artifact{}, fmt.Errorf("archive identity verifier is required")
	}
	if err = verify(ctx, f.Name(), selected); err != nil {
		return model.Artifact{}, fmt.Errorf("verify downloaded image: %w", err)
	}
	if err = os.Rename(f.Name(), name); err != nil {
		return model.Artifact{}, err
	}
	return model.Artifact{ID: digest, SHA256: digest, Size: n, Path: name}, nil
}
