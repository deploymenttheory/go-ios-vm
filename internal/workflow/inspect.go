// Package workflow owns use-case sequencing and publication, not SQL or codecs.
package workflow

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/deploymenttheory/go-ios-vm/internal/model"
)

var errInvalidIdentity = errors.New("select an explicit valid build identity")

type Repository interface {
	Register(context.Context, model.Artifact, []model.Target) error
	Start(context.Context, model.Run) error
	Publish(context.Context, model.Run, []model.Observation) error
	Fail(context.Context, string, string) error
}
type InspectFunc func(context.Context) ([]model.Observation, []model.Coverage, error)

func Inspect(
	ctx context.Context,
	store Repository,
	artifact model.Artifact,
	targets []model.Target,
	identity int,
	inspect InspectFunc,
) (model.Run, error) {
	if identity < 0 || identity >= len(targets) {
		return model.Run{}, errInvalidIdentity
	}
	if err := store.Register(ctx, artifact, targets); err != nil {
		return model.Run{}, fmt.Errorf("Inspect: Register: %w", err)
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return model.Run{}, fmt.Errorf("Inspect: rand.Read: %w", err)
	}
	run := model.Run{
		ID:       hex.EncodeToString(nonce[:]),
		TargetID: targets[identity].ID,
		State:    "running",
		Started:  time.Now().UTC(),
	}
	if err := store.Start(ctx, run); err != nil {
		return run, fmt.Errorf("Inspect: Start: %w", err)
	}
	observations, coverage, err := inspect(ctx)
	run.Coverage = coverage
	run.State = "complete"
	for _, c := range coverage {
		if c.State != "complete" {
			run.State = "partial"
		}
	}
	if err == nil {
		err = store.Publish(ctx, run, observations)
	}
	if err != nil {
		run.State = "failed"
		if ctx.Err() != nil {
			run.State = "cancelled"
		}
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if e := store.Fail(cleanup, run.ID, run.State); e != nil {
			return run, fmt.Errorf("%w; could not record terminal run state: %w", err, e)
		}
	}
	if err != nil {
		return run, fmt.Errorf("inspect run: %w", err)
	}
	return run, nil
}
