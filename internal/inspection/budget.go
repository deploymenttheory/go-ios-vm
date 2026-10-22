package inspection

import (
	"archive/zip"
	"fmt"
	"os"
)

type StagingBudget struct {
	LimitBytes     uint64 `json:"limit_bytes"`
	ReserveBytes   uint64 `json:"reserve_bytes"`
	FreeBytes      uint64 `json:"free_bytes"`
	AvailableBytes uint64 `json:"available_bytes"`
}

type StagingPlan struct {
	StagingBudget
	Member          string `json:"member,omitempty"`
	RequiresStaging bool   `json:"requires_staging"`
	DeclaredBytes   uint64 `json:"declared_bytes,omitempty"`
	Fits            bool   `json:"fits"`
	Reason          string `json:"reason,omitempty"`
}

// DefaultStagingBudget reports the temporary filesystem used by disk staging.
// stageDiskMember rechecks free space before and during writes.
func DefaultStagingBudget() (StagingBudget, error) {
	free, err := diskStageFree(os.TempDir())
	if err != nil {
		return StagingBudget{}, err
	}
	budget := StagingBudget{
		LimitBytes:   diskStageLimit,
		ReserveBytes: diskStageReserve,
		FreeBytes:    free,
	}
	if free > diskStageReserve {
		budget.AvailableBytes = min(diskStageLimit, free-diskStageReserve)
	}
	return budget, nil
}

func (a *Archive) DiskStagingPlan(identity int) (StagingPlan, error) {
	if identity < 0 || identity >= len(a.Targets) {
		return StagingPlan{}, errIdentityIndexOutsideBuildManifest
	}
	budget, err := DefaultStagingBudget()
	if err != nil {
		return StagingPlan{}, err
	}
	plan := StagingPlan{StagingBudget: budget, Fits: true}
	member, ok := a.Targets[identity].Components["OS"]
	if !ok {
		plan.Reason = "selected identity has no OS image"
		return plan, nil
	}
	plan.Member = member
	index, ok := a.members[member]
	if !ok {
		plan.Reason = "selected OS image is missing from ZIP"
		return plan, nil
	}
	f := a.ZIP.File[index]
	if f.Method == zip.Store {
		return plan, nil
	}
	if f.Method != zip.Deflate {
		plan.Fits = false
		plan.Reason = fmt.Sprintf("unsupported ZIP method %d", f.Method)
		return plan, nil
	}
	plan.RequiresStaging = true
	plan.DeclaredBytes = f.UncompressedSize64
	plan.Fits = plan.DeclaredBytes <= budget.AvailableBytes
	if !plan.Fits {
		if plan.DeclaredBytes > budget.LimitBytes {
			plan.Reason = "declared OS image exceeds staging limit"
		} else {
			plan.Reason = "temporary disk reserve would be crossed"
		}
	}
	return plan, nil
}
