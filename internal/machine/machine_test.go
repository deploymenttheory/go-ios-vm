package machine

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/deploymenttheory/go-ios-vm/internal/guest"
	"github.com/deploymenttheory/go-ios-vm/internal/host"
)

var digest = strings.Repeat("ab", 32)

func TestDefaultLibraryDir(t *testing.T) {
	got := DefaultLibraryDir("/Users/u/Library/Application Support")
	want := filepath.Join("/Users/u/Library/Application Support", "go-ios-vm", "machines")
	if got != want {
		t.Fatalf("DefaultLibraryDir = %q, want %q", got, want)
	}
}

func TestStageRecordValidate(t *testing.T) {
	tests := []struct {
		name   string
		record StageRecord
		valid  bool
	}{
		{"completed with evidence", StageRecord{Stage: StageRestore, Status: StatusCompleted, Evidence: []Evidence{{Name: "restore-log", SHA256: digest, Size: 1}}}, true},
		{"completed without evidence", StageRecord{Stage: StageRestore, Status: StatusCompleted}, false},
		{"completed with error", StageRecord{Stage: StagePatch, Status: StatusCompleted, Evidence: []Evidence{{Name: "txm", SHA256: digest, Size: 1}}, Error: "late failure"}, false},
		{"evidence with uppercase digest", StageRecord{Stage: StagePatch, Status: StatusCompleted, Evidence: []Evidence{{Name: "txm", SHA256: strings.ToUpper(digest), Size: 1}}}, false},
		{"evidence with short digest", StageRecord{Stage: StagePatch, Status: StatusCompleted, Evidence: []Evidence{{Name: "txm", SHA256: "abcd", Size: 1}}}, false},
		{"evidence without name", StageRecord{Stage: StagePatch, Status: StatusCompleted, Evidence: []Evidence{{Name: "", SHA256: digest, Size: 1}}}, false},
		{"failed with cause", StageRecord{Stage: StageInstall, Status: StatusFailed, Error: "unmount failed"}, true},
		{"failed without cause", StageRecord{Stage: StageInstall, Status: StatusFailed}, false},
		{"running", StageRecord{Stage: StageAcquire, Status: StatusRunning}, true},
		{"canceled", StageRecord{Stage: StageRestore, Status: StatusCanceled, Error: "interrupted"}, true},
		{"unknown stage", StageRecord{Stage: "launchpad", Status: StatusPending}, false},
		{"unknown status", StageRecord{Stage: StageAcquire, Status: "done"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.record.Validate()
			if tt.valid && err != nil {
				t.Fatalf("Validate() = %v, want nil", err)
			}
			if !tt.valid && !errors.Is(err, ErrInvalidRecord) {
				t.Fatalf("Validate() = %v, want ErrInvalidRecord", err)
			}
		})
	}
}

func TestObservationKeepsStatesSeparate(t *testing.T) {
	o := Observation{
		Native: host.StateRunning,
		Guest:  guest.ReadinessUnknown,
		Stages: []StageRecord{
			{Stage: StageRestore, Status: StatusCompleted, Evidence: []Evidence{{Name: "restore-log", SHA256: digest, Size: 1}}},
			{Stage: StageInstall, Status: StatusRunning},
			{Stage: StageFirstBoot, Status: StatusCompleted},
		},
	}
	if !o.Completed(StageRestore) {
		t.Error("restore with evidence is not reported completed")
	}
	if o.Completed(StageInstall) {
		t.Error("running install is reported completed")
	}
	if o.Completed(StageFirstBoot) {
		t.Error("first boot without evidence is reported completed although the machine is running")
	}
	if o.Completed(StageAcquire) {
		t.Error("stage without a record is reported completed")
	}
}
