package acquisition

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"math/bits"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// CacheBudget is a point-in-time estimate. Fetch rechecks both limits while
// holding the exclusive cache lease; this snapshot does not reserve space.
type CacheBudget struct {
	LimitBytes     int64 `json:"limit_bytes"`
	UsedBytes      int64 `json:"used_bytes"`
	FreeBytes      int64 `json:"free_bytes"`
	ReserveBytes   int64 `json:"reserve_bytes"`
	AvailableBytes int64 `json:"available_bytes"`
}

type FetchPlan struct {
	CacheBudget
	ImageBytes      int64  `json:"image_bytes"`
	ResumedBytes    int64  `json:"resumed_bytes"`
	AdditionalBytes int64  `json:"additional_bytes"`
	Fits            bool   `json:"fits"`
	Reason          string `json:"reason,omitempty"`
}

func (c Cache) Budget(ctx context.Context) (CacheBudget, error) {
	release, err := c.Lease(ctx, false)
	if err != nil {
		return CacheBudget{}, err
	}
	defer release()
	return c.budgetLocked()
}

func (c Cache) PlanFetch(ctx context.Context, selected Candidate) (FetchPlan, error) {
	if selected.Size <= 0 || selected.ETag == "" {
		return FetchPlan{}, errUnpinnedFetch
	}
	release, err := c.Lease(ctx, false)
	if err != nil {
		return FetchPlan{}, err
	}
	defer release()
	budget, err := c.budgetLocked()
	if err != nil {
		return FetchPlan{}, err
	}
	plan := FetchPlan{CacheBudget: budget, ImageBytes: selected.Size}
	stage := filepath.Join(c.Dir, stageName(selected))
	info, err := os.Lstat(stage)
	if err == nil {
		if !info.Mode().IsRegular() || info.Size() > selected.Size {
			return FetchPlan{}, errPartialNotRegular
		}
		plan.ResumedBytes = info.Size()
	} else if !errors.Is(err, os.ErrNotExist) {
		return FetchPlan{}, fmt.Errorf("PlanFetch: stage: %w", err)
	}
	plan.AdditionalBytes = selected.Size - plan.ResumedBytes
	plan.Fits = selected.Size <= c.Limit && plan.AdditionalBytes <= budget.AvailableBytes
	if !plan.Fits {
		switch {
		case selected.Size > c.Limit:
			plan.Reason = "image exceeds cache limit"
		case plan.AdditionalBytes > c.Limit-budget.UsedBytes:
			plan.Reason = "managed cache limit would be exceeded"
		default:
			plan.Reason = "free disk reserve would be crossed"
		}
	}
	return plan, nil
}

func (c Cache) budgetLocked() (CacheBudget, error) {
	entries, err := c.Entries()
	if err != nil {
		return CacheBudget{}, err
	}
	budget := CacheBudget{LimitBytes: c.Limit, ReserveBytes: c.Reserve}
	for _, entry := range entries {
		if entry.Size < 0 || entry.Size > math.MaxInt64-budget.UsedBytes {
			return CacheBudget{}, errInvalidCacheBudget
		}
		budget.UsedBytes += entry.Size
	}
	budget.FreeBytes, err = availableDiskBytes(c.Dir)
	if err != nil {
		return CacheBudget{}, err
	}
	cacheAvailable := max(int64(0), budget.LimitBytes-budget.UsedBytes)
	diskAvailable := max(int64(0), budget.FreeBytes-budget.ReserveBytes)
	budget.AvailableBytes = min(cacheAvailable, diskAvailable)
	return budget, nil
}

func availableDiskBytes(dir string) (int64, error) {
	var space unix.Statfs_t
	if err := unix.Statfs(dir, &space); err != nil {
		return 0, fmt.Errorf("cache budget: Statfs: %w", err)
	}
	if space.Bsize <= 0 {
		return 0, errInsufficientFreeSpace
	}
	hi, lo := bits.Mul64(uint64(space.Bavail), uint64(space.Bsize))
	if hi > 0 || lo > math.MaxInt64 {
		return math.MaxInt64, nil
	}
	return int64(lo), nil //nolint:gosec // The upper bound is checked above.
}

func stageName(selected Candidate) string {
	stageID := sha256.Sum256([]byte(selected.URL + "\x00" + selected.ETag))
	return hex.EncodeToString(stageID[:]) + ".part"
}
