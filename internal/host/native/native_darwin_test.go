package native

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/deploymenttheory/go-bindings-macosplatform/bindings/runtime/errkit"
	"github.com/deploymenttheory/go-bindings-macosplatform/bindings/runtime/purego"

	"github.com/deploymenttheory/go-ios-vm/internal/testenv"
)

func requireNative(t *testing.T) {
	t.Helper()
	testenv.Require(t, testenv.Native, testenv.AppleSilicon(27))
}

func newQueue(t *testing.T) *Queue {
	t.Helper()
	q, err := NewQueue("com.deploymenttheory.go-ios-vm.test")
	if err != nil {
		t.Fatal(err)
	}
	return q
}

func TestNativeQueue(t *testing.T) {
	requireNative(t)
	q := newQueue(t)
	if q.OnQueue() {
		t.Fatal("OnQueue true off the queue")
	}
	var on, nested bool
	q.Sync(func() {
		on = q.OnQueue()
		q.Sync(func() { nested = q.OnQueue() }) // inline, not a deadlock
	})
	if !on || !nested {
		t.Fatalf("Sync ran off the queue (on %v, nested %v)", on, nested)
	}
	order := make(chan int, 3)
	for i := range 3 {
		q.Async(func() { order <- i })
	}
	for want := range 3 {
		if got := <-order; got != want {
			t.Fatalf("Async ran out of order: %d, want %d", got, want)
		}
	}
	if other := newQueue(t); other.OnQueue() {
		t.Fatal("a second queue reports the caller on it")
	}
}

// The framework-style request: the native call returns at once and the
// completion handler is later invoked on the queue.
func TestNativeSubmitWaitsOutsideTheQueue(t *testing.T) {
	requireNative(t)
	q := newQueue(t)
	issuedOnQueue := false
	op := Submit(q, "start", func(complete func(error)) {
		issuedOnQueue = q.OnQueue()
		go func() {
			time.Sleep(20 * time.Millisecond)
			q.Async(func() { complete(nil) })
		}()
	})
	var waitedOnQueue error
	q.Sync(func() { waitedOnQueue = op.Wait(context.Background()) })
	if !errors.Is(waitedOnQueue, ErrWaitOnQueue) {
		t.Fatalf("Wait on the queue = %v, want ErrWaitOnQueue", waitedOnQueue)
	}
	if err := op.Wait(context.Background()); err != nil || !issuedOnQueue {
		t.Fatalf("Wait = %v, issued on queue %v", err, issuedOnQueue)
	}
}

func TestNativeCheckReportsMissingCapabilities(t *testing.T) {
	requireNative(t)
	if err := Check(Capability{Class: "VZVirtualMachine", Selector: "startWithOptions:completionHandler:"}); err != nil {
		t.Fatalf("public selector: %v", err)
	}
	var missing *MissingError
	err := Check(Capability{Class: "VZVirtualMachine", Selector: "_goIOSVMNoSuchSelector:", Use: "test", Purpose: "a test"})
	if !errors.As(err, &missing) || missing.ClassMissing || !errors.Is(err, ErrMissingCapability) {
		t.Fatalf("missing selector: %v", err)
	}
	err = Check(Capability{Class: "_VZGoIOSVMNoSuchClass", Selector: "init"})
	if !errors.As(err, &missing) || !missing.ClassMissing {
		t.Fatalf("missing class: %v", err)
	}
	// Class and instance methods are checked separately.
	classMethod := Capability{Class: "VZMacHardwareModel", Selector: "_hardwareModelWithDescriptor:", ClassMethod: true}
	if err := Check(classMethod); err != nil {
		t.Fatalf("class method: %v", err)
	}
	classMethod.ClassMethod = false
	if err := Check(classMethod); !errors.As(err, &missing) {
		t.Fatalf("class method checked as an instance method: %v", err)
	}
}

// TestNativeInventory records which inventoried interfaces this host provides.
// Missing ones are reported, not treated as available.
func TestNativeInventory(t *testing.T) {
	requireNative(t)
	r, err := Inventory()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range r.Available {
		t.Logf("available: %s", c)
	}
	for _, m := range r.Missing {
		if !errors.Is(m, ErrMissingCapability) {
			t.Errorf("untyped missing capability %v", m)
		}
		t.Logf("missing: %v", m)
		if !m.Capability.Private && m.Capability.Required {
			t.Errorf("public interface missing: %v", m)
		}
	}
	t.Logf("%d available, %d missing, %d required missing", len(r.Available), len(r.Missing), len(r.RequiredMissing()))
}

func TestNativeCompletionBlockOutlivesAbandonedWait(t *testing.T) {
	requireNative(t)
	results := make(chan error, 2)
	block := CompletionBlock(func(err error) { results <- err })
	framework := block.Copy() // what a framework does with a completion handler

	// The waiter gives up and drops its reference; nothing may be freed.
	block = 0
	for range 3 {
		runtime.GC()
	}
	framework.Invoke(purego.ID(0))
	if err := <-results; err != nil {
		t.Fatalf("nil NSError delivered as %v", err)
	}

	nsErr := purego.Send[purego.ID](purego.ID(purego.FoundationClass("NSError")),
		purego.RegisterName("errorWithDomain:code:userInfo:"), purego.NSString("VZErrorDomain"), 2, purego.ID(0))
	framework.Invoke(nsErr) // a duplicate completion still converts its error
	var vzErr *errkit.Error
	if err := <-results; !errors.As(err, &vzErr) || vzErr.Domain() != "VZErrorDomain" || vzErr.Code() != 2 {
		t.Fatalf("NSError converted to %v", err)
	}
	framework.Release()
}

// fakeOperation is an Objective-C class whose method takes a completion
// handler, copies it and invokes it later on the queue, as Virtualization does.
var (
	fakeOnce  sync.Once
	fakeClass purego.Class
	fakeQueue atomic.Pointer[Queue]
	fakeErr   atomic.Pointer[error]
	fakeDelay = make(chan struct{}, 1)
)

func fake(t *testing.T, q *Queue) purego.ID {
	t.Helper()
	fakeQueue.Store(q)
	fakeOnce.Do(func() {
		var err error
		fakeClass, err = purego.RegisterClass("GoIOSVMFakeOperation", purego.GetClass("NSObject"), nil, nil, []purego.MethodDef{{
			Cmd: purego.RegisterName("runWithCompletionHandler:"),
			Fn: func(_ purego.ID, _ purego.SEL, handler purego.Block) {
				handler = handler.Copy()
				go func() {
					<-fakeDelay
					fakeQueue.Load().Async(func() {
						var nsErr purego.ID
						if e := fakeErr.Load(); e != nil && *e != nil {
							nsErr = purego.Send[purego.ID](purego.ID(purego.FoundationClass("NSError")),
								purego.RegisterName("errorWithDomain:code:userInfo:"), purego.NSString("GoIOSVMTest"), 7, purego.ID(0))
						}
						handler.Invoke(nsErr)
						handler.Release()
					})
				}()
			},
		}})
		if err != nil {
			t.Fatal(err)
		}
	})
	obj := purego.Send[purego.ID](purego.ID(fakeClass), purego.RegisterName("new"))
	t.Cleanup(func() { purego.Release(obj) })
	return obj
}

var runCapability = Capability{Class: "GoIOSVMFakeOperation", Selector: "runWithCompletionHandler:", Use: "test", Purpose: "a test"}

func TestNativeRequestCompletesAndReportsLateResults(t *testing.T) {
	requireNative(t)
	q := newQueue(t)
	obj := fake(t, q)

	var noErr error
	fakeErr.Store(&noErr)
	op, err := Request(q, obj, runCapability)
	if err != nil {
		t.Fatal(err)
	}
	fakeDelay <- struct{}{}
	if err := op.Wait(context.Background()); err != nil {
		t.Fatalf("Wait = %v", err)
	}

	failure := errors.New("fail")
	fakeErr.Store(&failure)
	op, err = Request(q, obj, runCapability)
	if err != nil {
		t.Fatal(err)
	}
	late := make(chan error, 1)
	op.OnLate(func(err error) { late <- err })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := op.Wait(ctx); !errors.Is(err, ErrAbandoned) {
		t.Fatalf("abandoned Wait = %v", err)
	}
	op = nil
	for range 3 {
		runtime.GC()
	}
	fakeDelay <- struct{}{}
	var testErr *errkit.Error
	select {
	case err := <-late:
		if !errors.As(err, &testErr) || testErr.Domain() != "GoIOSVMTest" || testErr.Code() != 7 {
			t.Fatalf("late result %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("late completion never arrived")
	}
}

func TestNativeRequestChecksBeforeSending(t *testing.T) {
	requireNative(t)
	q := newQueue(t)
	obj := fake(t, q)
	_, err := Request(q, obj, Capability{Class: "GoIOSVMFakeOperation", Selector: "missingWithCompletionHandler:"})
	if !errors.Is(err, ErrMissingCapability) {
		t.Fatalf("Request with a missing selector = %v", err)
	}
	if _, err := Request(q, obj, Capability{Class: "GoIOSVMFakeOperation"}); !errors.Is(err, ErrMissingCapability) {
		t.Fatalf("Request without a selector = %v", err)
	}
}
