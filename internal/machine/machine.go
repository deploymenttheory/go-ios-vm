// Package machine defines a virtual machine's durable pipeline records and the
// separate observations made about it. Native state, guest readiness and stage
// completion are distinct; none is inferred from another.
package machine

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"

	"github.com/deploymenttheory/go-ios-vm/internal/guest"
	"github.com/deploymenttheory/go-ios-vm/internal/host"
)

// DefaultLibraryDir returns the machine library under the user's configuration
// directory, as returned by os.UserConfigDir.
func DefaultLibraryDir(userConfigDir string) string {
	return filepath.Join(userConfigDir, "go-ios-vm", "machines")
}

// Stage is one step of the create pipeline.
type Stage string

// Pipeline stages.
const (
	StageAcquire   Stage = "acquire"
	StagePrepare   Stage = "prepare"
	StagePatch     Stage = "patch"
	StageRestore   Stage = "restore"
	StageInstall   Stage = "install"
	StageFirstBoot Stage = "first-boot"
)

// Stages lists the pipeline stages in execution order.
var Stages = []Stage{StageAcquire, StagePrepare, StagePatch, StageRestore, StageInstall, StageFirstBoot}

// Status is the state of one stage.
type Status string

// Stage statuses. Only StatusCompleted means the stage is done.
const (
	StatusPending   Status = "pending"
	StatusRunning   Status = "running"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
	StatusCanceled  Status = "canceled"
)

// Evidence is the digest of an artifact that proves a stage completed.
type Evidence struct {
	// Name is the artifact's path relative to the machine directory.
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

// StageRecord is the durable record of one stage.
type StageRecord struct {
	Stage    Stage      `json:"stage"`
	Status   Status     `json:"status"`
	Evidence []Evidence `json:"evidence,omitempty"`
	// Error describes why a failed or canceled stage stopped.
	Error string `json:"error,omitempty"`
}

// ErrInvalidRecord reports a stage record that must not be persisted.
var ErrInvalidRecord = errors.New("invalid stage record")

// Validate reports whether r may be persisted. A completed stage needs
// evidence, and a failed one needs its cause.
func (r StageRecord) Validate() error {
	if !slices.Contains(Stages, r.Stage) {
		return fmt.Errorf("%w: unknown stage %q", ErrInvalidRecord, r.Stage)
	}
	switch r.Status {
	case StatusPending, StatusRunning, StatusCanceled:
	case StatusCompleted:
		if len(r.Evidence) == 0 {
			return fmt.Errorf("%w: %s completed without evidence", ErrInvalidRecord, r.Stage)
		}
		if r.Error != "" {
			return fmt.Errorf("%w: %s completed with an error", ErrInvalidRecord, r.Stage)
		}
	case StatusFailed:
		if r.Error == "" {
			return fmt.Errorf("%w: %s failed without a cause", ErrInvalidRecord, r.Stage)
		}
	default:
		return fmt.Errorf("%w: unknown status %q for %s", ErrInvalidRecord, r.Status, r.Stage)
	}
	for _, e := range r.Evidence {
		if e.Name == "" || !isSHA256(e.SHA256) {
			return fmt.Errorf("%w: %s evidence %q needs a lowercase hex SHA-256", ErrInvalidRecord, r.Stage, e.Name)
		}
	}
	return nil
}

func isSHA256(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// Observation is what is currently known about a machine. Each field comes from
// its own source and is reported as observed.
type Observation struct {
	Native host.State
	Guest  guest.Readiness
	Stages []StageRecord
}

// Completed reports whether stage has a valid completed record.
func (o Observation) Completed(stage Stage) bool {
	for _, r := range o.Stages {
		if r.Stage == stage {
			return r.Status == StatusCompleted && r.Validate() == nil
		}
	}
	return false
}
