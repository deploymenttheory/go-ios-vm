package native

import (
	"github.com/deploymenttheory/go-bindings-macosplatform/bindings/runtime/purego"
)

var hardwareModelCapabilities = []Capability{
	{Class: "_VZMacHardwareModelDescriptor", Selector: "setPlatformVersion:"},
	{Class: "_VZMacHardwareModelDescriptor", Selector: "setBoardID:"},
	{Class: "_VZMacHardwareModelDescriptor", Selector: "setISA:"},
	{Class: "VZMacHardwareModel", Selector: "_hardwareModelWithDescriptor:", ClassMethod: true},
	{Class: "VZMacHardwareModel", Selector: "isSupported"},
}

// HardwareModelSupported builds the hardware model d describes and reports
// whether Virtualization supports it for this process. The PV=3 research model
// is reported supported only to a process carrying the private virtualization
// entitlements.
func HardwareModelSupported(d HardwareModelDescriptor) (bool, error) {
	for _, c := range hardwareModelCapabilities {
		if err := Check(c); err != nil {
			return false, err
		}
	}
	if err := loadDispatch(); err != nil {
		return false, err
	}
	pool := autoreleasePoolPush()
	defer autoreleasePoolPop(pool)

	number := purego.ID(purego.FoundationClass("NSNumber"))
	desc := purego.Send[purego.ID](purego.ID(purego.GetClass("_VZMacHardwareModelDescriptor")), purego.RegisterName("new"))
	defer purego.Release(desc)
	purego.Send[purego.ID](desc, purego.RegisterName("setPlatformVersion:"),
		purego.Send[purego.ID](number, purego.RegisterName("numberWithUnsignedInt:"), d.PlatformVersion))
	purego.Send[purego.ID](desc, purego.RegisterName("setBoardID:"),
		purego.Send[purego.ID](number, purego.RegisterName("numberWithUnsignedInt:"), d.BoardID))
	purego.Send[purego.ID](desc, purego.RegisterName("setISA:"),
		purego.Send[purego.ID](number, purego.RegisterName("numberWithLongLong:"), d.ISA))
	model := purego.Send[purego.ID](purego.ID(purego.GetClass("VZMacHardwareModel")),
		purego.RegisterName("_hardwareModelWithDescriptor:"), desc)
	if model == 0 {
		return false, nil
	}
	return purego.Send[bool](model, purego.RegisterName("isSupported")), nil
}
