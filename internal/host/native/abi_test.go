package native

import (
	"errors"
	"strings"
	"testing"
)

func TestCapabilityInventoryIsWellFormed(t *testing.T) {
	seen := map[string]bool{}
	for _, c := range Capabilities {
		key := c.String()
		if seen[key] {
			t.Errorf("duplicate capability %s", key)
		}
		seen[key] = true
		if c.Class == "" || c.Use == "" || c.Purpose == "" {
			t.Errorf("%s: class, use and purpose are required", key)
		}
		if strings.HasPrefix(c.Class, "_") || strings.HasPrefix(c.Selector, "_") {
			if !c.Private {
				t.Errorf("%s: private interface not marked private", key)
			}
		}
		if !c.Private && !c.Required {
			t.Errorf("%s: public interfaces are inventoried because they are required", key)
		}
	}
	for _, want := range []string{
		"-[VZMacOSBootLoader _setROMURL:]",
		"-[_VZSEPCoprocessorConfiguration setRomBinaryURL:]",
		"-[VZMacOSVirtualMachineStartOptions _setForceDFU:]",
		"+[VZMacHardwareModel _hardwareModelWithDescriptor:]",
		"-[VZMacMachineIdentifier _ECID]",
		"_VZPL011SerialPortConfiguration",
		"_VZUSBTouchScreenConfiguration",
	} {
		if !seen[want] {
			t.Errorf("inventory lacks %s", want)
		}
	}
}

func TestMissingErrorNamesTheCapability(t *testing.T) {
	c := Capability{Class: "VZMacOSBootLoader", Selector: "_setROMURL:", Use: "HOST-02", Purpose: "booting the vendored OpenAVB ROM"}
	err := error(&MissingError{Capability: c})
	if !errors.Is(err, ErrMissingCapability) || !strings.Contains(err.Error(), "selector -[VZMacOSBootLoader _setROMURL:], needed for booting the vendored OpenAVB ROM (HOST-02)") {
		t.Fatalf("selector error: %v", err)
	}
	err = &MissingError{Capability: c, ClassMissing: true}
	if !strings.Contains(err.Error(), "class VZMacOSBootLoader, needed for") {
		t.Fatalf("class error: %v", err)
	}
}
