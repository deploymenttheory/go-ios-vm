package native

import (
	"context"
	"errors"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// serialQueue is a controllable stand-in for a native serial dispatch queue:
// one goroutine runs submitted work in order.
type serialQueue struct {
	work chan func()
	gid  chan uint64
	id   uint64
	stop chan struct{}
}

func newSerialQueue(t *testing.T) *serialQueue {
	q := &serialQueue{work: make(chan func(), 64), gid: make(chan uint64), stop: make(chan struct{})}
	go func() {
		q.gid <- goid()
		for {
			select {
			case fn := <-q.work:
				fn()
			case <-q.stop:
				return
			}
		}
	}()
	q.id = <-q.gid
	t.Cleanup(func() { close(q.stop) })
	return q
}

func (q *serialQueue) Async(fn func()) { q.work <- fn }
func (q *serialQueue) OnQueue() bool   { return goid() == q.id }

// run runs fn on the queue and waits for it, like dispatch_sync.
func (q *serialQueue) run(fn func()) {
	done := make(chan struct{})
	q.Async(func() { fn(); close(done) })
	<-done
}

func goid() uint64 {
	buf := make([]byte, 64)
	buf = buf[:runtime.Stack(buf, false)]
	id, _ := strconv.ParseUint(strings.Fields(string(buf))[1], 10, 64)
	return id
}

// nativeRequest models a framework call that queues its completion handler on
// the machine queue when the framework finishes.
type nativeRequest struct {
	q        *serialQueue
	finish   chan error
	issuedOn bool
}

func (r *nativeRequest) issue(complete func(error)) {
	r.issuedOn = r.q.OnQueue()
	go func() {
		err := <-r.finish
		r.q.Async(func() { complete(err) })
	}()
}

func TestSubmitReturnsControlToTheQueueBeforeWaiting(t *testing.T) {
	q := newSerialQueue(t)
	req := &nativeRequest{q: q, finish: make(chan error)}
	op := Submit(q, "start", req.issue)

	// The queue must be free while the request is outstanding: other work runs
	// on it before the completion is delivered.
	ran := false
	q.run(func() { ran = true })
	if !ran || !req.issuedOn {
		t.Fatalf("queue blocked (ran %v) or request not issued on the queue (%v)", ran, req.issuedOn)
	}
	req.finish <- nil
	if err := op.Wait(context.Background()); err != nil {
		t.Fatalf("Wait: %v", err)
	}
}

func TestWaitOnTheQueueIsRefused(t *testing.T) {
	q := newSerialQueue(t)
	req := &nativeRequest{q: q, finish: make(chan error, 1)}
	op := Submit(q, "start", req.issue)
	var err error
	q.run(func() { err = op.Wait(context.Background()) })
	if !errors.Is(err, ErrWaitOnQueue) {
		t.Fatalf("Wait on the queue = %v, want ErrWaitOnQueue rather than a deadlock", err)
	}
	req.finish <- nil
	if err := op.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestNativeErrorIsReturnedWrapped(t *testing.T) {
	q := newSerialQueue(t)
	nativeErr := errors.New("VZErrorDomain 2: invalid configuration")
	req := &nativeRequest{q: q, finish: make(chan error, 1)}
	op := Submit(q, "start", req.issue)
	req.finish <- nativeErr
	if err := op.Wait(context.Background()); !errors.Is(err, nativeErr) || !strings.HasPrefix(err.Error(), "start: ") {
		t.Fatalf("Wait = %v", err)
	}
}

func TestCanceledWaitKeepsTheCallbackAndReportsTheLateCompletion(t *testing.T) {
	q := newSerialQueue(t)
	req := &nativeRequest{q: q, finish: make(chan error)}
	op := Submit(q, "stop", req.issue)
	late := make(chan error, 1)
	op.OnLate(func(err error) {
		if !q.OnQueue() {
			t.Error("late handler ran off the queue")
		}
		late <- err
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := op.Wait(ctx)
	if !errors.Is(err, ErrAbandoned) || !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled Wait = %v, want ErrAbandoned and context.Canceled", err)
	}
	if done, _ := op.Result(); done {
		t.Fatal("cancellation reported as completion")
	}

	// Drop the caller's reference: the pending registry keeps the operation.
	op = nil
	runtime.GC()
	if Pending() == 0 {
		t.Fatal("an incomplete operation was released")
	}
	req.finish <- errors.New("stop failed")
	select {
	case err := <-late:
		if err == nil || err.Error() != "stop failed" {
			t.Fatalf("late completion %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("late completion never delivered")
	}
}

func TestCompletionBeatingCancellationIsReported(t *testing.T) {
	q := newSerialQueue(t)
	op := Submit(q, "start", func(complete func(error)) { complete(nil) })
	q.run(func() {}) // the completion has run
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := op.Wait(ctx); err != nil {
		t.Fatalf("Wait after completion with a canceled context = %v, want the completion", err)
	}
}

func TestDuplicateCompletionsAreCountedNotDelivered(t *testing.T) {
	q := newSerialQueue(t)
	op := Submit(q, "start", func(complete func(error)) {
		complete(nil)
		complete(errors.New("second"))
		complete(errors.New("third"))
	})
	if err := op.Wait(context.Background()); err != nil {
		t.Fatalf("Wait = %v, want the first completion", err)
	}
	if n := op.Duplicates(); n != 2 {
		t.Fatalf("Duplicates = %d, want 2", n)
	}
}

func TestConcurrentWaitersAndCancellations(t *testing.T) {
	q := newSerialQueue(t)
	const n = 50
	reqs := make([]*nativeRequest, n)
	ops := make([]*Op, n)
	for i := range n {
		reqs[i] = &nativeRequest{q: q, finish: make(chan error, 1)}
		ops[i] = Submit(q, "op", reqs[i].issue)
	}
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(i%5)*time.Millisecond)
			defer cancel()
			_ = ops[i].Wait(ctx)
		}()
		go func() { reqs[i].finish <- nil }()
	}
	wg.Wait()
	deadline := time.Now().Add(5 * time.Second)
	for _, op := range ops {
		for {
			if done, _ := op.Result(); done || time.Now().After(deadline) {
				break
			}
			time.Sleep(time.Millisecond)
		}
		if done, err := op.Result(); !done || err != nil || op.Duplicates() != 0 {
			t.Fatalf("operation not completed exactly once: %v %v %d", done, err, op.Duplicates())
		}
	}
}
