package acquisition

import (
	"errors"
	"io"
	"testing"
	"time"
)

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}

func TestLargeTransferProgressIsCoalesced(t *testing.T) {
	const size = 512 << 20
	reports := 0
	w := &coalescingWriter{
		writer: io.Discard,
		report: func(n int64) error {
			reports++
			if n > size {
				t.Fatalf("counter exceeded transfer: %d", n)
			}
			return nil
		},
		lastAt: time.Now(),
	}
	n, err := io.CopyN(w, zeroReader{}, size)
	if err != nil || n != size || w.written != size || reports < 16 || reports > 32 {
		t.Fatalf("512 MiB write volume: bytes=%d reports=%d err=%v", n, reports, err)
	}
	t.Logf("512 MiB transfer: %d progress writes for %d buffered writes", reports, size/(32<<10))
}

func TestProgressThrottleAndWriteFailure(t *testing.T) {
	failure := errors.New("database unavailable")
	reports := 0
	w := &coalescingWriter{
		writer: io.Discard,
		report: func(int64) error {
			reports++
			return failure
		},
		lastAt: time.Now(),
	}
	if _, err := w.Write(make([]byte, 64<<10)); err != nil || reports != 0 {
		t.Fatalf("small write was not throttled: reports=%d err=%v", reports, err)
	}
	w.lastAt = time.Now().Add(-2 * time.Second)
	if _, err := w.Write(make([]byte, 64<<10)); !errors.Is(err, failure) || reports != 1 {
		t.Fatalf("progress write failure was hidden: reports=%d err=%v", reports, err)
	}
}
