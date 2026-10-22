package workflow

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/deploymenttheory/go-ios-vm/internal/model"
)

var ErrResumeInputMismatch = errors.New(
	"resume requires the same verified archive, target, extractor and component plan",
)

var (
	errInvalidBatchPlan = errors.New(
		"inspection requires a verified input and component plan",
	)
	errCoverageComponentMismatch = errors.New("component coverage mismatch")
	errComponentOutsidePlan      = errors.New("component outside run plan")
)

type BatchRepository interface {
	Register(context.Context, model.Artifact, []model.Target) error
	Start(context.Context, model.Run) error
	Run(context.Context, string) (model.Run, error)
	Resume(context.Context, string, string) error
	Heartbeat(context.Context, string, string) error
	Checkpoints(context.Context, string) (map[string]model.Coverage, error)
	PublishBatch(context.Context, model.Run, model.ObservationBatch) error
	Finish(context.Context, string, string) (model.Run, error)
	FailBatched(context.Context, string, string, string) error
}

type BatchInspectFunc func(
	context.Context,
	map[string]bool,
	func(string, []model.Observation, model.Coverage) error,
) error

// InspectBatched durably publishes bounded slices and only finalizes coverage
// after every component checkpoint exists. Resume is explicit and always
// checks the full archive identity established by inspection.Open.
//
//nolint:gocyclo // The run lifecycle keeps lease, cancellation and publication handling in one place.
func InspectBatched(
	ctx context.Context,
	store BatchRepository,
	artifact model.Artifact,
	targets []model.Target,
	identity int,
	resumeID string,
	extractor string,
	plan []string,
	inspect BatchInspectFunc,
	onStart ...func(model.Run) error,
) (model.Run, error) {
	if identity < 0 || identity >= len(targets) {
		return model.Run{}, errInvalidIdentity
	}
	if len(plan) == 0 || extractor == "" || artifact.SHA256 == "" {
		return model.Run{}, errInvalidBatchPlan
	}
	run, err := beginBatchedRun(ctx, store, artifact, targets, identity, resumeID, extractor, plan)
	if err != nil {
		return run, err
	}
	if run.State == "complete" || run.State == "partial" {
		return run, nil
	}
	if len(onStart) > 0 && onStart[0] != nil {
		if err := onStart[0](run); err != nil {
			return run, fmt.Errorf("InspectBatched: start observer: %w", err)
		}
	}
	workerCtx, stopWorker := context.WithCancel(ctx)
	heartbeatDone := make(chan struct{})
	defer func() { stopWorker(); <-heartbeatDone }()
	heartbeatErr := make(chan error, 1)
	go func() {
		defer close(heartbeatDone)
		ticker := time.NewTicker(20 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-workerCtx.Done():
				return
			case <-ticker.C:
				if err := store.Heartbeat(workerCtx, run.ID, run.LeaseToken); err != nil {
					heartbeatErr <- err
					stopWorker()
					return
				}
			}
		}
	}()
	checkpoints, err := store.Checkpoints(workerCtx, run.ID)
	if err != nil {
		return run, fmt.Errorf("InspectBatched: Checkpoints: %w", err)
	}
	done := make(map[string]bool, len(checkpoints))
	for component := range checkpoints {
		done[component] = true
	}
	emit := func(component string, observations []model.Observation, coverage model.Coverage) error {
		if done[component] {
			return nil
		}
		if component != coverage.Component {
			return errCoverageComponentMismatch
		}
		if !slices.Contains(plan, component) {
			return errComponentOutsidePlan
		}
		for offset, ordinal := 0, 0; ; ordinal++ {
			if err := workerCtx.Err(); err != nil {
				return fmt.Errorf("InspectBatched: %w", err)
			}
			end := min(offset+model.MaxObservationsPerBatch, len(observations))
			batch := model.ObservationBatch{
				Component: component, Ordinal: ordinal,
				Observations: observations[offset:end],
			}
			if end == len(observations) {
				batch.Final = &coverage
			}
			if err := store.PublishBatch(workerCtx, run, batch); err != nil {
				return fmt.Errorf("InspectBatched: PublishBatch: %w", err)
			}
			if end == len(observations) {
				break
			}
			offset = end
		}
		done[component] = true
		return nil
	}
	inspectErr := inspect(workerCtx, done, emit)
	stopWorker()
	<-heartbeatDone
	if err := inspectErr; err != nil {
		select {
		case leaseErr := <-heartbeatErr:
			return run, fmt.Errorf("InspectBatched: lease: %w", leaseErr)
		default:
		}
		run.State = "failed"
		if ctx.Err() != nil {
			run.State = "cancelled"
		}
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if e := store.FailBatched(cleanup, run.ID, run.State, run.LeaseToken); e != nil {
			return run, fmt.Errorf("InspectBatched: %w; terminal state: %w", err, e)
		}
		return run, fmt.Errorf("InspectBatched: %w", err)
	}
	select {
	case leaseErr := <-heartbeatErr:
		return run, fmt.Errorf("InspectBatched: lease: %w", leaseErr)
	default:
	}
	run, err = store.Finish(ctx, run.ID, run.LeaseToken)
	if err != nil {
		return run, fmt.Errorf("InspectBatched: Finish: %w", err)
	}
	return run, nil
}

func beginBatchedRun(
	ctx context.Context,
	store BatchRepository,
	artifact model.Artifact,
	targets []model.Target,
	identity int,
	resumeID string,
	extractor string,
	plan []string,
) (model.Run, error) {
	var run model.Run
	if resumeID != "" {
		prior, err := store.Run(ctx, resumeID)
		if err != nil {
			return model.Run{}, fmt.Errorf("InspectBatched: Run: %w", err)
		}
		if prior.TargetID != targets[identity].ID || prior.InputSHA256 != artifact.SHA256 ||
			prior.InputSize != artifact.Size || prior.Extractor != extractor ||
			!slices.Equal(prior.Planned, plan) {
			return prior, ErrResumeInputMismatch
		}
		if prior.State == "complete" || prior.State == "partial" {
			return prior, nil
		}
		var token [16]byte
		if _, err := rand.Read(token[:]); err != nil {
			return prior, fmt.Errorf("InspectBatched: lease nonce: %w", err)
		}
		prior.LeaseToken = hex.EncodeToString(token[:])
		if err := store.Resume(ctx, resumeID, prior.LeaseToken); err != nil {
			// Finalization may commit between the initial read and this claim.
			// Return that committed result instead of treating the retry as a failure.
			if current, readErr := store.Run(ctx, resumeID); readErr == nil &&
				(current.State == "complete" || current.State == "partial") &&
				current.TargetID == targets[identity].ID && current.InputSHA256 == artifact.SHA256 &&
				current.InputSize == artifact.Size && current.Extractor == extractor &&
				slices.Equal(current.Planned, plan) {
				return current, nil
			}
			return prior, fmt.Errorf("InspectBatched: Resume: %w", err)
		}
		run = prior
		run.State = "running"
	} else {
		if err := store.Register(ctx, artifact, targets); err != nil {
			return model.Run{}, fmt.Errorf("InspectBatched: Register: %w", err)
		}
		var nonce [16]byte
		if _, err := rand.Read(nonce[:]); err != nil {
			return model.Run{}, fmt.Errorf("InspectBatched: nonce: %w", err)
		}
		run = model.Run{
			ID: hex.EncodeToString(nonce[:]), TargetID: targets[identity].ID,
			State: "running", Started: time.Now().UTC(), InputSHA256: artifact.SHA256,
			InputSize: artifact.Size, Extractor: extractor, Planned: slices.Clone(plan),
		}
		var token [16]byte
		if _, err := rand.Read(token[:]); err != nil {
			return run, fmt.Errorf("InspectBatched: lease nonce: %w", err)
		}
		run.LeaseToken = hex.EncodeToString(token[:])
		if err := store.Start(ctx, run); err != nil {
			return run, fmt.Errorf("InspectBatched: Start: %w", err)
		}
	}
	return run, nil
}
