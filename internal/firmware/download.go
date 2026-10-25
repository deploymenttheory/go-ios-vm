package firmware

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"hash"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/deploymenttheory/go-ios-vm/internal/progress"
)

// Errors from downloading.
var (
	ErrIncomplete = errors.New("download incomplete")
	ErrSize       = errors.New("archive size does not match")
	ErrDigest     = errors.New("archive digest does not match")
	ErrNoSpace    = errors.New("not enough free space")
)

// downloadOp names download progress events.
const downloadOp = "download"

// download fetches src.URL into the staging file at part, resuming a previous
// partial download when the server honours a range request and restarting
// when it does not. It returns the archive's size and SHA-256. A canceled or
// failed download leaves the partial file for the next attempt; nothing is
// published here.
func download(ctx context.Context, client *http.Client, src Source, part string, free func() (int64, error), report progress.Reporter) (int64, string, error) {
	f, err := os.OpenFile(part, os.O_RDWR|os.O_CREATE, 0o600) //nolint:gosec // staging file inside the cache
	if err != nil {
		return 0, "", fmt.Errorf("open staging file: %w", err)
	}
	defer f.Close() //nolint:errcheck // closed explicitly on success
	have, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return 0, "", fmt.Errorf("staging file: %w", err)
	}
	if src.Size > 0 && have > src.Size {
		if err := restart(f); err != nil {
			return 0, "", err
		}
		have = 0
	}
	if src.Size > 0 && free != nil {
		avail, err := free()
		if err != nil {
			return 0, "", fmt.Errorf("measure free space: %w", err)
		}
		if need := src.Size - have; avail < need {
			return 0, "", fmt.Errorf("%w: %s needs %d more bytes, %d free", ErrNoSpace, src.Identity, need, avail)
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src.URL, nil)
	if err != nil {
		return 0, "", fmt.Errorf("download request: %w", err)
	}
	if have > 0 {
		req.Header.Set("Range", "bytes="+strconv.FormatInt(have, 10)+"-")
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, "", fmt.Errorf("%w: %s: %w", ErrIncomplete, src.Identity, err)
	}
	defer resp.Body.Close() //nolint:errcheck // read-only

	var total int64
	switch {
	case resp.StatusCode == http.StatusPartialContent && have > 0:
		start, size, ok := parseContentRange(resp.Header.Get("Content-Range"))
		if !ok || start != have {
			return 0, "", fmt.Errorf("%w: %s: unexpected Content-Range %q", ErrIncomplete, src.Identity, resp.Header.Get("Content-Range"))
		}
		total = size
	case resp.StatusCode == http.StatusOK:
		// The server ignored the range or there was none: start over.
		if err := restart(f); err != nil {
			return 0, "", err
		}
		have = 0
		total = resp.ContentLength
	default:
		return 0, "", fmt.Errorf("%w: %s: server returned %s", ErrIncomplete, src.Identity, resp.Status)
	}
	if src.Size > 0 && total >= 0 && total != src.Size {
		return 0, "", fmt.Errorf("%w: %s: server reports %d bytes, want %d", ErrSize, src.Identity, total, src.Size)
	}

	h, err := hashPrefix(f, have)
	if err != nil {
		return 0, "", err
	}
	w := &progressWriter{dst: io.MultiWriter(f, h), done: have, total: total, what: src.Identity.String(), report: report}
	n, err := io.Copy(w, resp.Body)
	have += n
	if err != nil {
		return 0, "", fmt.Errorf("%w: %s after %d bytes: %w", ErrIncomplete, src.Identity, have, err)
	}
	if total >= 0 && have != total {
		return 0, "", fmt.Errorf("%w: %s: %d of %d bytes", ErrIncomplete, src.Identity, have, total)
	}
	if err := f.Sync(); err != nil {
		return 0, "", fmt.Errorf("sync staging file: %w", err)
	}
	if err := f.Close(); err != nil {
		return 0, "", fmt.Errorf("close staging file: %w", err)
	}
	return have, fmt.Sprintf("%x", h.Sum(nil)), nil
}

func restart(f *os.File) error {
	if err := f.Truncate(0); err != nil {
		return fmt.Errorf("reset staging file: %w", err)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("reset staging file: %w", err)
	}
	return nil
}

// hashPrefix hashes the first n bytes of f and leaves f positioned at n.
func hashPrefix(f *os.File, n int64) (hash.Hash, error) {
	h := sha256.New()
	if n > 0 {
		if _, err := io.Copy(h, io.NewSectionReader(f, 0, n)); err != nil {
			return nil, fmt.Errorf("hash partial download: %w", err)
		}
	}
	if _, err := f.Seek(n, io.SeekStart); err != nil {
		return nil, fmt.Errorf("staging file: %w", err)
	}
	return h, nil
}

// parseContentRange parses "bytes start-end/size".
func parseContentRange(v string) (start, size int64, ok bool) {
	rest, found := strings.CutPrefix(v, "bytes ")
	if !found {
		return 0, 0, false
	}
	span, total, found := strings.Cut(rest, "/")
	if !found {
		return 0, 0, false
	}
	first, _, found := strings.Cut(span, "-")
	if !found {
		return 0, 0, false
	}
	s, err1 := strconv.ParseInt(first, 10, 64)
	t, err2 := strconv.ParseInt(total, 10, 64)
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return s, t, true
}

type progressWriter struct {
	dst          io.Writer
	done, total  int64
	what         string
	report       progress.Reporter
	lastReported int64
}

func (w *progressWriter) Write(p []byte) (int, error) {
	n, err := w.dst.Write(p)
	w.done += int64(n)
	if w.done-w.lastReported >= 64<<20 || (w.total > 0 && w.done == w.total) {
		w.lastReported = w.done
		w.report.Report(progress.Event{Operation: downloadOp, Step: w.what, Done: w.done, Total: max(w.total, 0), Unit: "bytes"})
	}
	return n, err //nolint:wrapcheck // io.Writer contract
}
