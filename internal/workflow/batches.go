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
	Resume(context.Context, string) error
	Checkpoints(context.Context, string) (map[string]model.Coverage, error)
	PublishBatch(context.Context, model.Run, model.ObservationBatch) error
	Finish(context.Context, string) (model.Run, error)
	Fail(context.Context, string, string) error
}

type BatchInspectFunc func(
	context.Context,
	map[string]bool,
	func(string, []model.Observation, model.Coverage) error,
) error

// InspectBatched durably publishes bounded slices and only finalizes coverage
// after every component checkpoint exists. Resume is explicit and always
// checks the full archive identity established by inspection.Open.
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
	checkpoints, err := store.Checkpoints(ctx, run.ID)
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
			if err := ctx.Err(); err != nil {
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
			if err := store.PublishBatch(ctx, run, batch); err != nil {
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
	if err := inspect(ctx, done, emit); err != nil {
		run.State = "failed"
		if ctx.Err() != nil {
			run.State = "cancelled"
		}
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if e := store.Fail(cleanup, run.ID, run.State); e != nil {
			return run, fmt.Errorf("InspectBatched: %w; terminal state: %w", err, e)
		}
		return run, fmt.Errorf("InspectBatched: %w", err)
	}
	run, err = store.Finish(ctx, run.ID)
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
		if err := store.Resume(ctx, resumeID); err != nil {
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
		if err := store.Start(ctx, run); err != nil {
			return run, fmt.Errorf("InspectBatched: Start: %w", err)
		}
	}
	return run, nil
}
