package acquisition

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	maxRemoteArchiveSize = 32 << 30
	maxRangeRead         = 8 << 20
	maxRangeBytes        = 64 << 20
	maxRangeRequests     = 512
)

var (
	errInvalidRemoteArchive = errors.New(
		"remote archive lacks a bounded size, byte ranges or strong ETag",
	)
	errRangeBudget   = errors.New("remote byte-range budget exceeded")
	errRangeResponse = errors.New("remote byte-range response changed or violated request")
)

type RangeInfo struct {
	URL      string `json:"url"`
	Size     int64  `json:"size"`
	ETag     string `json:"etag"`
	Bytes    int64  `json:"bytes_read"`
	Requests int    `json:"range_requests"`
}

// RemoteArchive is a bounded, ETag-pinned ReaderAt for ZIP metadata preflight.
// Its ranges do not establish a full-archive checksum or a bootable image.
type RemoteArchive struct {
	ctx    context.Context
	client *http.Client
	url    string
	size   int64
	etag   string
	mu     sync.Mutex
	bytes  int64
	calls  int
}

func (c *Client) OpenRange(ctx context.Context, rawURL string) (*RemoteArchive, error) {
	if err := AppleURL(rawURL); err != nil {
		return nil, err
	}
	requestCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodHead, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("OpenRange: NewRequestWithContext: %w", err)
	}
	req.Header.Set("Accept-Encoding", "identity")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("OpenRange: Do: %w", err)
	}
	defer res.Body.Close()
	etag := res.Header.Get("ETag")
	if res.StatusCode != http.StatusOK || res.ContentLength <= 0 ||
		res.ContentLength > maxRemoteArchiveSize ||
		!strings.EqualFold(res.Header.Get("Accept-Ranges"), "bytes") ||
		!strings.HasPrefix(etag, "\"") || !strings.HasSuffix(etag, "\"") {
		return nil, errInvalidRemoteArchive
	}
	return &RemoteArchive{
		ctx:    ctx,
		client: c.HTTP,
		url:    rawURL,
		size:   res.ContentLength,
		etag:   etag,
	}, nil
}

func (r *RemoteArchive) Size() int64 { return r.size }

func (r *RemoteArchive) Info() RangeInfo {
	r.mu.Lock()
	defer r.mu.Unlock()
	return RangeInfo{r.url, r.size, r.etag, r.bytes, r.calls}
}

func (r *RemoteArchive) ReadAt(p []byte, offset int64) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, fmt.Errorf("ReadAt: context: %w", err)
	}
	if len(p) == 0 {
		return 0, nil
	}
	if offset < 0 || offset >= r.size {
		return 0, io.EOF
	}
	if len(p) > maxRangeRead {
		return 0, errRangeBudget
	}
	length := len(p)
	if int64(length) > r.size-offset {
		length = int(r.size - offset)
	}
	r.mu.Lock()
	if r.calls >= maxRangeRequests || int64(length) > maxRangeBytes-r.bytes {
		r.mu.Unlock()
		return 0, errRangeBudget
	}
	r.calls++
	r.bytes += int64(length)
	r.mu.Unlock()

	requestCtx, cancel := context.WithTimeout(r.ctx, 45*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, r.url, nil)
	if err != nil {
		return 0, fmt.Errorf("ReadAt: NewRequestWithContext: %w", err)
	}
	end := offset + int64(length) - 1
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", offset, end))
	req.Header.Set("If-Match", r.etag)
	req.Header.Set("Accept-Encoding", "identity")
	res, err := r.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("ReadAt: Do: %w", err)
	}
	defer res.Body.Close()
	expected := fmt.Sprintf("bytes %d-%d/%d", offset, end, r.size)
	if res.StatusCode != http.StatusPartialContent || res.Header.Get("Content-Range") != expected ||
		res.Header.Get("ETag") != r.etag || res.ContentLength != int64(length) {
		return 0, errRangeResponse
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, int64(length)+1))
	if err != nil {
		return 0, fmt.Errorf("ReadAt: ReadAll: %w", err)
	}
	if len(data) != length {
		return 0, errRangeResponse
	}
	copy(p, data)
	if length < len(p) {
		return length, io.EOF
	}
	return length, nil
}
