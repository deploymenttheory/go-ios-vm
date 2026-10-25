package native

import (
	"errors"
	"fmt"
	"sync"

	"github.com/deploymenttheory/go-bindings-macosplatform/bindings/runtime/errkit"
	"github.com/deploymenttheory/go-bindings-macosplatform/bindings/runtime/purego"
)

const virtualizationPath = "/System/Library/Frameworks/Virtualization.framework/Virtualization"

var (
	frameworkOnce sync.Once
	frameworkErr  error

	selInstancesRespond = purego.RegisterName("instancesRespondToSelector:")
	selRespond          = purego.RegisterName("respondsToSelector:")
)

// LoadVirtualization loads Virtualization.framework so its classes, including
// private ones, are registered with the Objective-C runtime.
func LoadVirtualization() error {
	frameworkOnce.Do(func() {
		if _, err := purego.Dlopen(virtualizationPath, purego.RTLD_GLOBAL|purego.RTLD_LAZY); err != nil {
			frameworkErr = fmt.Errorf("load Virtualization.framework: %w", err)
		}
	})
	return frameworkErr
}

// Check reports whether c is available, returning a *MissingError when it is
// not. Only the runtime's introspection messages are sent, so a missing
// selector cannot raise an Objective-C exception.
func Check(c Capability) error {
	if err := LoadVirtualization(); err != nil {
		return err
	}
	cls := purego.GetClass(c.Class)
	if cls == 0 {
		return &MissingError{Capability: c, ClassMissing: true}
	}
	if c.Selector == "" {
		return nil
	}
	sel := purego.RegisterName(c.Selector)
	var ok bool
	if c.ClassMethod {
		ok = purego.Send[bool](purego.ID(cls), selRespond, sel)
	} else {
		ok = purego.Send[bool](purego.ID(cls), selInstancesRespond, sel)
	}
	if !ok {
		return &MissingError{Capability: c}
	}
	return nil
}

// Report is the availability of every inventoried capability on this host.
type Report struct {
	Available []Capability
	Missing   []*MissingError
}

// RequiredMissing returns the missing capabilities the first release needs.
func (r Report) RequiredMissing() []*MissingError {
	var out []*MissingError
	for _, m := range r.Missing {
		if m.Capability.Required {
			out = append(out, m)
		}
	}
	return out
}

// Inventory checks every entry of Capabilities.
func Inventory() (Report, error) {
	if err := LoadVirtualization(); err != nil {
		return Report{}, err
	}
	var r Report
	for _, c := range Capabilities {
		err := Check(c)
		var missing *MissingError
		switch {
		case err == nil:
			r.Available = append(r.Available, c)
		case errors.As(err, &missing):
			r.Missing = append(r.Missing, missing)
		default:
			return Report{}, err
		}
	}
	return r, nil
}

// CompletionBlock returns a block for a completion handler of type
// void (^)(NSError *) that completes op. The bridge owns one reference to the
// block until the handler has run, so neither an abandoned wait nor garbage
// collection can free the Go callback while the framework may still call it.
// The native caller copies the block as it normally would.
func CompletionBlock(complete func(error)) purego.Block {
	var (
		mu    sync.Mutex
		block purego.Block
		ran   bool
	)
	mu.Lock()
	defer mu.Unlock()
	block = purego.NewBlock(func(_ purego.Block, nsErr purego.ID) {
		complete(errkit.FromObjC(purego.NSErrorToError(nsErr)))
		mu.Lock()
		first := !ran
		ran = true
		mu.Unlock()
		if first {
			// Drop the bridge's own reference once the handler has returned; the
			// caller's copy keeps the block valid for the rest of its call.
			go block.Release()
		}
	})
	return block
}

// Request queues a message to target on q whose final argument is a
// void (^)(NSError *) completion handler, and returns its operation without
// waiting. c is checked before anything is sent. target is retained until the
// completion has run; the caller keeps args alive for the call.
func Request(q *Queue, target purego.ID, c Capability, args ...any) (*Op, error) {
	if c.Selector == "" || c.ClassMethod {
		return nil, fmt.Errorf("%w: %s is not an instance method", ErrMissingCapability, c)
	}
	if err := Check(c); err != nil {
		return nil, err
	}
	sel := purego.RegisterName(c.Selector)
	purego.Retain(target)
	var release sync.Once
	return Submit(q, c.String(), func(complete func(error)) {
		block := CompletionBlock(func(err error) {
			complete(err) // duplicates still reach the operation, which counts them
			release.Do(func() { purego.Release(target) })
		})
		purego.Send[purego.ID](target, sel, append(args, block)...)
	}), nil
}
