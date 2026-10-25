package native

import (
	"errors"
	"fmt"
)

// Capability is one Objective-C class, or one selector of it, that the runner
// sends to. Private entries come from vphone-cli's VPhoneVirtualMachine.swift
// and VPhoneVirtualMachineHardwareModel.swift at the pinned revision.
type Capability struct {
	Class string
	// Selector is empty when only the class is needed.
	Selector string
	// ClassMethod marks a selector sent to the class rather than an instance.
	ClassMethod bool
	// Private marks an interface outside the public SDK.
	Private bool
	// Required marks interfaces the first release cannot do without; the rest
	// are optional and are skipped when unavailable.
	Required bool
	// Use names the ticket that relies on the capability.
	Use     string
	Purpose string
}

func (c Capability) String() string {
	switch {
	case c.Selector == "":
		return c.Class
	case c.ClassMethod:
		return "+[" + c.Class + " " + c.Selector + "]"
	default:
		return "-[" + c.Class + " " + c.Selector + "]"
	}
}

// ErrMissingCapability reports a class or selector this host's Virtualization
// framework does not provide.
var ErrMissingCapability = errors.New("native capability unavailable")

// MissingError names an unavailable capability.
type MissingError struct {
	Capability Capability
	// ClassMissing distinguishes a missing class from a missing selector.
	ClassMissing bool
}

func (e *MissingError) Error() string {
	what := "selector"
	if e.ClassMissing || e.Capability.Selector == "" {
		what = "class " + e.Capability.Class
		return fmt.Sprintf("%s: %s, needed for %s (%s), is not available on this host", ErrMissingCapability, what, e.Capability.Purpose, e.Capability.Use)
	}
	return fmt.Sprintf("%s: %s %s, needed for %s (%s), is not available on this host", ErrMissingCapability, what, e.Capability, e.Capability.Purpose, e.Capability.Use)
}

func (e *MissingError) Unwrap() error { return ErrMissingCapability }

func public(class, selector string, classMethod bool, use, purpose string) Capability {
	return Capability{Class: class, Selector: selector, ClassMethod: classMethod, Required: true, Use: use, Purpose: purpose}
}

func private(class, selector string, classMethod, required bool, use, purpose string) Capability {
	return Capability{Class: class, Selector: selector, ClassMethod: classMethod, Private: true, Required: required, Use: use, Purpose: purpose}
}

// Capabilities is the class and selector inventory of the runner's
// Virtualization use.
var Capabilities = []Capability{
	// Machine lifecycle (public).
	public("VZVirtualMachine", "initWithConfiguration:queue:", false, "HOST-01", "creating a machine bound to its serial queue"),
	public("VZVirtualMachine", "startWithOptions:completionHandler:", false, "HOST-01", "starting a machine"),
	public("VZVirtualMachine", "stopWithCompletionHandler:", false, "HOST-04", "forced native stop"),
	public("VZVirtualMachine", "state", false, "HOST-01", "native state observation"),
	public("VZVirtualMachine", "setDelegate:", false, "HOST-04", "delegate stop and error reports"),
	public("VZVirtualMachineConfiguration", "validateWithError:", false, "HOST-02", "native configuration validation"),
	public("VZMacOSVirtualMachineStartOptions", "", false, "HOST-02", "start options"),
	public("VZMacPlatformConfiguration", "", false, "HOST-02", "the research platform"),
	public("VZMacMachineIdentifier", "initWithDataRepresentation:", false, "HOST-02", "reloading the stored identity"),
	public("VZMacAuxiliaryStorage", "initWithURL:", false, "HOST-02", "NVRAM reuse"),
	public("VZMacAuxiliaryStorage", "initCreatingStorageAtURL:hardwareModel:options:error:", false, "HOST-02", "NVRAM creation"),
	public("VZMacMachineIdentifier", "dataRepresentation", false, "HOST-02", "storing a new identity"),
	public("VZMacOSBootLoader", "", false, "HOST-02", "the boot loader"),
	public("VZMacGraphicsDeviceConfiguration", "", false, "HOST-02", "the display device"),
	public("VZVirtioBlockDeviceConfiguration", "", false, "HOST-02", "the disk"),
	public("VZVirtioSocketDeviceConfiguration", "", false, "GUEST-03", "the host to guest daemon channel"),
	public("VZFileHandleSerialPortAttachment", "", false, "HOST-04", "serial capture"),

	// Research hardware model (private): platform version 3, board 0x90, ISA 2.
	private("_VZMacHardwareModelDescriptor", "setPlatformVersion:", false, true, "HOST-02", "the PV=3 research hardware model"),
	private("_VZMacHardwareModelDescriptor", "setBoardID:", false, true, "HOST-02", "the PV=3 research hardware model"),
	private("_VZMacHardwareModelDescriptor", "setISA:", false, true, "HOST-02", "the PV=3 research hardware model"),
	private("VZMacHardwareModel", "_hardwareModelWithDescriptor:", true, true, "HOST-02", "the PV=3 research hardware model"),
	public("VZMacHardwareModel", "isSupported", false, "FND-03", "checking research hardware model support"),
	private("VZMacMachineIdentifier", "_ECID", false, true, "HOST-02", "matching DFU to the stored identity"),

	// Custom ROMs and SEP (private).
	private("VZMacOSBootLoader", "_setROMURL:", false, true, "HOST-02", "booting the vendored OpenAVB ROM"),
	private("_VZSEPCoprocessorConfiguration", "initWithStorageURL:", false, true, "HOST-02", "SEP storage"),
	private("_VZSEPCoprocessorConfiguration", "setRomBinaryURL:", false, true, "HOST-02", "booting the vendored OpenSEP ROM"),
	private("VZVirtualMachineConfiguration", "_setCoprocessors:", false, true, "HOST-02", "attaching the SEP"),

	// Devices (private).
	private("_VZPL011SerialPortConfiguration", "", false, true, "HOST-04", "PL011 serial output"),
	private("_VZUSBTouchScreenConfiguration", "", false, true, "HOST-03", "touch input"),
	private("VZVirtualMachineConfiguration", "_setMultiTouchDevices:", false, true, "HOST-03", "touch input"),
	private("VZVirtualMachine", "_multiTouchDevices", false, true, "HOST-03", "sending touch events"),
	private("_VZMultiTouchEvent", "", false, true, "HOST-03", "touch events"),
	private("_VZTouch", "", false, true, "HOST-03", "touch events"),

	// Forced DFU (private).
	private("VZMacOSVirtualMachineStartOptions", "_setForceDFU:", false, true, "HOST-02", "starting in DFU"),
	private("VZMacOSVirtualMachineStartOptions", "_setStopInIBootStage1:", false, false, "HOST-02", "keeping iBoot stage 1 running"),
	private("VZMacOSVirtualMachineStartOptions", "_setStopInIBootStage2:", false, false, "HOST-02", "keeping iBoot stage 2 running"),

	// Optional upstream devices (private).
	private("_VZMacVideoToolboxDeviceConfiguration", "", false, false, "HOST-02", "accelerator devices"),
	private("_VZMacNeuralEngineDeviceConfiguration", "", false, false, "HOST-02", "accelerator devices"),
	private("_VZMacScalerAcceleratorDeviceConfiguration", "", false, false, "HOST-02", "accelerator devices"),
	private("VZVirtualMachineConfiguration", "_setAcceleratorDevices:", false, false, "HOST-02", "accelerator devices"),
	private("_VZMacSyntheticBatterySource", "", false, false, "HOST-02", "a synthetic battery"),
	private("_VZMacBatteryPowerSourceDeviceConfiguration", "", false, false, "HOST-02", "a synthetic battery"),
	private("VZVirtualMachineConfiguration", "_setPowerSourceDevices:", false, false, "HOST-02", "a synthetic battery"),
	private("_VZGDBDebugStubConfiguration", "", false, false, "HOST-02", "kernel debugging"),
	private("VZVirtualMachineConfiguration", "_setDebugStub:", false, false, "HOST-02", "kernel debugging"),
}

// HardwareModelDescriptor describes a Virtualization hardware model.
type HardwareModelDescriptor struct {
	PlatformVersion uint32
	BoardID         uint32
	ISA             int64
}

// ResearchHardwareModel is vphone-cli's PV=3 research model: platform version
// 3, board 0x90 and ISA 2, matching vresearch101.
var ResearchHardwareModel = HardwareModelDescriptor{PlatformVersion: 3, BoardID: 0x90, ISA: 2}
