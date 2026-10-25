package native

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/deploymenttheory/go-bindings-macosplatform/bindings/runtime/purego"
)

var (
	dispatchOnce sync.Once
	dispatchErr  error

	dispatchQueueCreate      func(label string, attr uintptr) uintptr
	dispatchAsyncF           func(queue, ctx, work uintptr)
	dispatchSyncF            func(queue, ctx, work uintptr)
	dispatchQueueSetSpecific func(queue, key, ctx, destructor uintptr)
	dispatchGetSpecific      func(key uintptr) uintptr
	autoreleasePoolPush      func() uintptr
	autoreleasePoolPop       func(pool uintptr)
	trampoline               uintptr

	work     sync.Map // map[uintptr]func()
	nextWork atomic.Uintptr
	nextKey  atomic.Uintptr
)

func loadDispatch() error {
	dispatchOnce.Do(func() {
		libSystem, err := purego.Dlopen("/usr/lib/libSystem.B.dylib", purego.RTLD_GLOBAL|purego.RTLD_LAZY)
		if err != nil {
			dispatchErr = fmt.Errorf("load libdispatch: %w", err)
			return
		}
		libobjc, err := purego.Dlopen("/usr/lib/libobjc.A.dylib", purego.RTLD_GLOBAL|purego.RTLD_LAZY)
		if err != nil {
			dispatchErr = fmt.Errorf("load libobjc: %w", err)
			return
		}
		purego.RegisterLibFunc(&dispatchQueueCreate, libSystem, "dispatch_queue_create")
		purego.RegisterLibFunc(&dispatchAsyncF, libSystem, "dispatch_async_f")
		purego.RegisterLibFunc(&dispatchSyncF, libSystem, "dispatch_sync_f")
		purego.RegisterLibFunc(&dispatchQueueSetSpecific, libSystem, "dispatch_queue_set_specific")
		purego.RegisterLibFunc(&dispatchGetSpecific, libSystem, "dispatch_get_specific")
		purego.RegisterLibFunc(&autoreleasePoolPush, libobjc, "objc_autoreleasePoolPush")
		purego.RegisterLibFunc(&autoreleasePoolPop, libobjc, "objc_autoreleasePoolPop")
		trampoline = purego.NewCallback(runWork)
	})
	return dispatchErr
}

// runWork is the dispatch_function_t for every queued work item. Each item runs
// inside its own autorelease pool, so objects autoreleased by native calls are
// drained when the item finishes.
func runWork(key uintptr) uintptr {
	fn, ok := work.LoadAndDelete(key)
	if !ok {
		return 0
	}
	pool := autoreleasePoolPush()
	defer autoreleasePoolPop(pool)
	fn.(func())() //nolint:forcetypeassert // only funcs are stored
	return 0
}

// Queue is a serial libdispatch queue. Virtualization objects created with it
// must be used only from it. A Queue lives for the rest of the process, like
// the machine bound to it.
type Queue struct {
	q   uintptr
	key uintptr
}

// NewQueue creates a serial dispatch queue.
func NewQueue(label string) (*Queue, error) {
	if err := loadDispatch(); err != nil {
		return nil, err
	}
	q := dispatchQueueCreate(label, 0) // a NULL attribute creates a serial queue
	if q == 0 {
		return nil, fmt.Errorf("dispatch_queue_create %q failed", label) //nolint:err113 // native failure without detail
	}
	// The specific's key only has to be unique; its value marks this queue.
	key := nextKey.Add(1)
	dispatchQueueSetSpecific(q, key, key, 0)
	return &Queue{q: q, key: key}, nil
}

// Async queues fn and returns without waiting.
func (q *Queue) Async(fn func()) {
	key := nextWork.Add(1)
	work.Store(key, fn)
	dispatchAsyncF(q.q, key, trampoline)
}

// Sync runs fn on the queue and waits for it. Called on the queue, it runs fn
// inline rather than deadlocking. fn must not wait for a completion that the
// queue delivers; use Submit and Op.Wait for that.
func (q *Queue) Sync(fn func()) {
	if q.OnQueue() {
		fn()
		return
	}
	key := nextWork.Add(1)
	work.Store(key, fn)
	dispatchSyncF(q.q, key, trampoline)
}

// OnQueue reports whether the caller is running on q.
func (q *Queue) OnQueue() bool { return dispatchGetSpecific(q.key) == q.key }

// Handle returns the dispatch_queue_t for APIs that bind an object to a queue.
func (q *Queue) Handle() uintptr { return q.q }
