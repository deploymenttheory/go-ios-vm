package acquisition

import (
	"bytes"
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFetchPlanAccountsForOtherImagesAndPinnedPartial(t *testing.T) {
	cache := Cache{Dir: t.TempDir(), Limit: 100, Reserve: 0}
	selected := Candidate{
		URL: "https://updates.cdn-apple.com/test.ipsw", Size: 80, ETag: `"one"`,
	}
	partial := filepath.Join(cache.Dir, stageName(selected))
	if err := os.WriteFile(partial, []byte(strings.Repeat("p", 30)), 0o600); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(cache.Dir, strings.Repeat("a", 64)+".ipsw")
	if err := os.WriteFile(other, []byte(strings.Repeat("x", 40)), 0o600); err != nil {
		t.Fatal(err)
	}
	plan, err := cache.PlanFetch(context.Background(), selected)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Fits || plan.UsedBytes != 70 || plan.ResumedBytes != 30 ||
		plan.AdditionalBytes != 50 || plan.AvailableBytes != 30 ||
		plan.Reason != "managed cache limit would be exceeded" {
		t.Fatalf("incorrect blocked plan: %+v", plan)
	}
	_, err = NewClient().Fetch(context.Background(), selected, cache,
		func(context.Context, string, Candidate) error {
			t.Fatal("blocked fetch reached verifier")
			return nil
		})
	if !errors.Is(err, errCacheBudgetExhausted) {
		t.Fatalf("blocked fetch bypassed budget: %v", err)
	}
	if info, statErr := os.Stat(partial); statErr != nil || info.Size() != 30 {
		t.Fatalf("blocked fetch damaged resumable partial: %+v, %v", info, statErr)
	}
	if err := os.Remove(other); err != nil {
		t.Fatal(err)
	}
	plan, err = cache.PlanFetch(context.Background(), selected)
	if err != nil || !plan.Fits || plan.UsedBytes != 30 || plan.AvailableBytes != 70 {
		t.Fatalf("resumable plan: %+v, %v", plan, err)
	}
	oversize := selected
	oversize.Size = 101
	plan, err = cache.PlanFetch(context.Background(), oversize)
	if err != nil || plan.Fits || plan.Reason != "image exceeds cache limit" {
		t.Fatalf("oversized plan: %+v, %v", plan, err)
	}
	cache.Reserve = math.MaxInt64
	plan, err = cache.PlanFetch(context.Background(), selected)
	if err != nil || plan.Fits || plan.Reason != "free disk reserve would be crossed" {
		t.Fatalf("disk reserve plan: %+v, %v", plan, err)
	}
}

func TestFetchPlanRefusesSymlinkedPartial(t *testing.T) {
	cache := Cache{Dir: t.TempDir(), Limit: 100}
	selected := Candidate{
		URL: "https://updates.cdn-apple.com/test.ipsw", Size: 80, ETag: `"one"`,
	}
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(cache.Dir, stageName(selected))); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.PlanFetch(context.Background(), selected); err != errPartialNotRegular {
		t.Fatalf("symlinked partial accepted: %v", err)
	}
}

func TestDownloadWriterPreservesReserveAndCancellation(t *testing.T) {
	var dst bytes.Buffer
	checks := 0
	writer := budgetedDownloadWriter{
		ctx: context.Background(), dst: &dst, dir: t.TempDir(), reserve: 4,
		free: func(string) (int64, error) {
			checks++
			if checks == 1 {
				return 8, nil
			}
			return 5, nil
		},
	}
	if _, err := writer.Write([]byte("data")); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write([]byte("more")); !errors.Is(err, errInsufficientFreeSpace) {
		t.Fatalf("reserve was crossed: %v", err)
	}
	if dst.String() != "data" {
		t.Fatalf("bytes written after reserve failure: %q", dst.String())
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	writer.ctx = ctx
	if _, err := writer.Write([]byte("last")); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled writer continued: %v", err)
	}
}
