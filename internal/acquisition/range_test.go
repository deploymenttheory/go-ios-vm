package acquisition

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestRemoteArchiveRejectsChangedOrUnboundedRanges(t *testing.T) {
	const image = "0123456789"
	for _, behavior := range []string{"valid", "whole", "changed", "wrong-range"} {
		t.Run(behavior, func(t *testing.T) {
			client := NewClient()
			client.HTTP.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
				headers := http.Header{"Etag": []string{`"stable"`}}
				if r.Method == http.MethodHead {
					headers.Set("Accept-Ranges", "bytes")
					return &http.Response{
						StatusCode:    200,
						ContentLength: int64(len(image)),
						Header:        headers,
						Body:          io.NopCloser(strings.NewReader("")),
					}, nil
				}
				if r.Header.Get("If-Match") != `"stable"` || r.Header.Get("Range") != "bytes=2-5" {
					t.Fatalf("unfenced range request: %v", r.Header)
				}
				start, end := 2, 5
				if behavior == "whole" {
					return &http.Response{
						StatusCode:    200,
						ContentLength: int64(len(image)),
						Header:        headers,
						Body:          io.NopCloser(strings.NewReader(image)),
					}, nil
				}
				if behavior == "changed" {
					headers.Set("ETag", `"changed"`)
				}
				headers.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(image)))
				if behavior == "wrong-range" {
					headers.Set("Content-Range", "bytes 1-4/10")
				}
				return &http.Response{
					StatusCode:    206,
					ContentLength: int64(end - start + 1),
					Header:        headers,
					Body:          io.NopCloser(bytes.NewReader([]byte(image[start : end+1]))),
				}, nil
			})
			archive, err := client.OpenRange(
				context.Background(),
				"https://updates.cdn-apple.com/fixture.ipsw",
			)
			if err != nil {
				t.Fatal(err)
			}
			p := make([]byte, 4)
			n, err := archive.ReadAt(p, 2)
			if behavior == "valid" {
				if err != nil || n != 4 || string(p) != "2345" {
					t.Fatal(n, err, string(p))
				}
			} else if !errors.Is(err, errRangeResponse) {
				t.Fatal("unsafe response accepted", behavior, n, err)
			}
			if info := archive.Info(); info.Requests != 1 || info.Bytes != 4 {
				t.Fatal(info)
			}
		})
	}
}

func TestRemoteArchiveBudgetsAndCancellation(t *testing.T) {
	client := NewClient()
	client.HTTP.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodHead {
			t.Fatal("oversized or cancelled read reached transport")
		}
		h := http.Header{"Etag": []string{`"stable"`}, "Accept-Ranges": []string{"bytes"}}
		return &http.Response{
			StatusCode:    200,
			ContentLength: maxRangeBytes + 1,
			Header:        h,
			Body:          io.NopCloser(strings.NewReader("")),
		}, nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	archive, err := client.OpenRange(ctx, "https://updates.cdn-apple.com/fixture.ipsw")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := archive.ReadAt(make([]byte, maxRangeRead+1), 0); !errors.Is(err, errRangeBudget) {
		t.Fatal(err)
	}
	cancel()
	if _, err := archive.ReadAt(make([]byte, 1), 0); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if info := archive.Info(); info.Requests != 0 || info.Bytes != 0 {
		t.Fatal(info)
	}
}
