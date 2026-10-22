package workflow

import (
	"context"
	"errors"
	"testing"

	"github.com/deploymenttheory/go-ios-vm/internal/model"
)

type fakeStore struct {
	published   bool
	failed      string
	cleanupLive bool
}

func (*fakeStore) Register(context.Context, model.Artifact, []model.Target) error { return nil }
func (*fakeStore) Start(context.Context, model.Run) error                         { return nil }
func (s *fakeStore) Publish(context.Context, model.Run, []model.Observation) error {
	s.published = true
	return nil
}

func (s *fakeStore) Fail(ctx context.Context, _ string, state string) error {
	s.failed = state
	s.cleanupLive = ctx.Err() == nil
	return nil
}

func TestCancellationRecordsTerminalStateWithFreshCleanupContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	s := &fakeStore{}
	run, err := Inspect(
		ctx,
		s,
		model.Artifact{},
		[]model.Target{{ID: "target"}},
		0,
		func(context.Context) ([]model.Observation, []model.Coverage, error) {
			cancel()
			return nil, nil, ctx.Err()
		},
	)
	if !errors.Is(err, context.Canceled) || run.State != "cancelled" || s.failed != "cancelled" ||
		s.published ||
		!s.cleanupLive {
		t.Fatalf("cancel lifecycle: %+v %+v %v", run, s, err)
	}
}
