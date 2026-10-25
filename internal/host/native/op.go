// Package native is the bridge to Apple's Virtualization runtime. It submits
// native operations on a machine's serial dispatch queue, waits for their
// completions outside that queue, keeps completion callbacks alive until they
// run, and checks the classes and selectors it needs before sending them.
//
// The bridge builds with CGO_ENABLED=0: native calls go through the pinned
// go-bindings-macosplatform runtime (purego), never a Swift helper. An
// Objective-C exception is not a Go panic and cannot be recovered; capability
// checks keep the bridge from sending selectors that would raise one.
//
// The bindings' generated lifecycle wrappers (VirtualMachine.Start, Stop and
// friends) wait for their completion where they are called. Called on the
// machine's queue they deadlock, and called elsewhere they break queue
// confinement, so the bridge uses Request instead.
package native

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// Executor runs work on one serial native queue.
type Executor interface {
	// Async queues fn to run on the queue and returns without waiting.
	Async(fn func())
	// OnQueue reports whether the caller is running on the queue.
	OnQueue() bool
}

// ErrWaitOnQueue reports a wait attempted on the queue that has to deliver the
// completion, which would deadlock.
var ErrWaitOnQueue = errors.New("waiting on the native queue that delivers the completion would deadlock")

// ErrAbandoned reports a wait that ended before the native completion arrived.
// The native operation continues; its eventual result is delivered to the
// operation's late handler and the machine's state must be reconciled.
var ErrAbandoned = errors.New("stopped waiting before the native operation completed")

// Op is one native asynchronous request whose completion arrives on its queue.
type Op struct {
	name string
	q    Executor
	done chan struct{}

	mu         sync.Mutex
	err        error
	completed  bool
	abandoned  bool
	duplicates int
	late       func(error)
}

// pending holds every incomplete operation, so an operation stays reachable
// until its completion has run even if every caller has dropped it.
var pending sync.Map // map[*Op]struct{}

// Submit queues start on q and returns immediately. start runs on the queue; it
// issues the native request and arranges for complete to be called with the
// result when the native completion arrives. Submit never waits, so the queue
// is free to deliver that completion.
func Submit(q Executor, name string, start func(complete func(error))) *Op {
	op := &Op{name: name, q: q, done: make(chan struct{})}
	pending.Store(op, struct{}{})
	q.Async(func() { start(op.complete) })
	return op
}

// complete records the native result. Only the first call counts; later calls
// are counted as duplicates.
func (op *Op) complete(err error) {
	op.mu.Lock()
	if op.completed {
		op.duplicates++
		op.mu.Unlock()
		return
	}
	op.completed, op.err = true, err
	late := op.abandoned
	handler := op.late
	op.mu.Unlock()
	close(op.done)
	pending.Delete(op)
	if late && handler != nil {
		handler(err)
	}
}

// Wait blocks until the native completion arrives or ctx ends, and must be
// called off the queue. When ctx ends first, Wait returns an error wrapping
// ErrAbandoned and ctx's error; the native operation is not canceled.
func (op *Op) Wait(ctx context.Context) error {
	if op.q.OnQueue() {
		return fmt.Errorf("%s: %w", op.name, ErrWaitOnQueue)
	}
	select {
	case <-op.done:
		op.mu.Lock()
		defer op.mu.Unlock()
		if op.err != nil {
			return fmt.Errorf("%s: %w", op.name, op.err)
		}
		return nil
	case <-ctx.Done():
		op.mu.Lock()
		defer op.mu.Unlock()
		if op.completed {
			// The completion won the race; report it rather than the cancellation.
			if op.err != nil {
				return fmt.Errorf("%s: %w", op.name, op.err)
			}
			return nil
		}
		op.abandoned = true
		return fmt.Errorf("%s: %w: %w", op.name, ErrAbandoned, ctx.Err())
	}
}

// OnLate sets the handler for a completion that arrives after Wait abandoned
// the operation. It runs on the queue that delivers the completion.
func (op *Op) OnLate(handler func(error)) {
	op.mu.Lock()
	defer op.mu.Unlock()
	op.late = handler
}

// Result reports whether the native completion has arrived and its result.
func (op *Op) Result() (completed bool, err error) {
	op.mu.Lock()
	defer op.mu.Unlock()
	return op.completed, op.err
}

// Duplicates counts completions delivered after the first. A nonzero count is
// a native contract violation worth diagnosing.
func (op *Op) Duplicates() int {
	op.mu.Lock()
	defer op.mu.Unlock()
	return op.duplicates
}

// Pending counts operations whose completion has not arrived.
func Pending() int {
	n := 0
	pending.Range(func(any, any) bool { n++; return true })
	return n
}
