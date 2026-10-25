package vz

import (
	"context"
	"fmt"

	"github.com/deploymenttheory/go-bindings-macosplatform/bindings/frameworks/virtualization"
	"github.com/deploymenttheory/go-bindings-macosplatform/bindings/runtime/obj"
	"github.com/deploymenttheory/go-bindings-macosplatform/bindings/runtime/purego"

	"github.com/deploymenttheory/go-ios-vm/internal/host"
	"github.com/deploymenttheory/go-ios-vm/internal/host/native"
)

var (
	capStart = native.Capability{Class: "VZVirtualMachine", Selector: "startWithOptions:completionHandler:", Use: "HOST-02", Purpose: "starting a machine"}
	capStop  = native.Capability{Class: "VZVirtualMachine", Selector: "stopWithCompletionHandler:", Use: "HOST-04", Purpose: "forced native stop"}
	capInit  = native.Capability{Class: "VZVirtualMachine", Selector: "initWithConfiguration:queue:", Use: "HOST-01", Purpose: "creating a machine bound to its queue"}
	capDFU   = native.Capability{Class: "VZMacOSVirtualMachineStartOptions", Selector: "_setForceDFU:", Private: true, Use: "HOST-02", Purpose: "starting in DFU"}
)

// VM is a native virtual machine bound to its own serial queue. Every native
// call on it runs on that queue; waits happen off it.
type VM struct {
	q   *native.Queue
	vm  *virtualization.VirtualMachine
	cfg *Configuration
}

var _ host.Machine = (*VM)(nil)

// NewVM creates the native machine for a validated configuration.
func NewVM(cfg *Configuration, label string) (*VM, error) {
	if err := check(capInit, capStart, capStop, capDFU); err != nil {
		return nil, err
	}
	q, err := native.NewQueue(label)
	if err != nil {
		return nil, err
	}
	var vm *virtualization.VirtualMachine
	q.Sync(func() {
		alloc := purego.Send[purego.ID](purego.ID(purego.GetClass("VZVirtualMachine")), purego.RegisterName("alloc"))
		id := purego.Send[purego.ID](alloc, purego.RegisterName("initWithConfiguration:queue:"), purego.ID(obj.ID(cfg.VZ)), q.Handle())
		if id != 0 {
			vm = virtualization.VirtualMachineFromID(id) // retains
			purego.Release(id)
		}
	})
	if vm == nil {
		return nil, fmt.Errorf("%w: Virtualization did not create the machine", ErrInvalidConfiguration)
	}
	return &VM{q: q, vm: vm, cfg: cfg}, nil
}

// Start starts the machine, in DFU when opts.ForceDFU is set, and waits for
// Virtualization's completion. Canceling ctx stops the wait, not the start.
func (v *VM) Start(ctx context.Context, opts host.StartOptions) error {
	options := virtualization.NewMacOSVirtualMachineStartOptions()
	optionsID := purego.ID(obj.ID(options))
	v.q.Sync(func() {
		purego.Send[purego.ID](optionsID, purego.RegisterName("_setForceDFU:"), opts.ForceDFU)
		for _, sel := range []string{"_setStopInIBootStage1:", "_setStopInIBootStage2:"} {
			if native.Check(native.Capability{Class: "VZMacOSVirtualMachineStartOptions", Selector: sel}) == nil {
				purego.Send[purego.ID](optionsID, purego.RegisterName(sel), false)
			}
		}
	})
	op, err := native.Request(v.q, purego.ID(obj.ID(v.vm)), capStart, optionsID)
	if err != nil {
		return err
	}
	if err := op.Wait(ctx); err != nil {
		return fmt.Errorf("start machine: %w", err)
	}
	return nil
}

// Stop stops the machine immediately, as pulling the power would.
func (v *VM) Stop(ctx context.Context) error {
	op, err := native.Request(v.q, purego.ID(obj.ID(v.vm)), capStop)
	if err != nil {
		return err
	}
	if err := op.Wait(ctx); err != nil {
		return fmt.Errorf("stop machine: %w", err)
	}
	return nil
}

// State reads the native state on the machine's queue.
func (v *VM) State(context.Context) (host.State, error) {
	var s virtualization.VirtualMachineState
	v.q.Sync(func() { s = v.vm.State() })
	return nativeState(s), nil
}

func nativeState(s virtualization.VirtualMachineState) host.State {
	switch s {
	case virtualization.VirtualMachineStateStopped:
		return host.StateStopped
	case virtualization.VirtualMachineStateRunning:
		return host.StateRunning
	case virtualization.VirtualMachineStatePaused:
		return host.StatePaused
	case virtualization.VirtualMachineStateError:
		return host.StateError
	case virtualization.VirtualMachineStateStarting:
		return host.StateStarting
	case virtualization.VirtualMachineStatePausing:
		return host.StatePausing
	case virtualization.VirtualMachineStateResuming:
		return host.StateResuming
	case virtualization.VirtualMachineStateStopping:
		return host.StateStopping
	default:
		return host.StateUnknown
	}
}

// Configuration returns the machine's configuration and serial pipes.
func (v *VM) Configuration() *Configuration { return v.cfg }
