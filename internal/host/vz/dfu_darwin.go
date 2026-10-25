package vz

import (
	"fmt"
	"strings"
	"sync"
	"unsafe"

	"github.com/deploymenttheory/go-bindings-macosplatform/bindings/runtime/purego"

	"github.com/deploymenttheory/go-ios-vm/internal/host/native"
)

const iokitPath = "/System/Library/Frameworks/IOKit.framework/IOKit"

var (
	iokitOnce sync.Once
	iokitErr  error

	ioServiceMatching               func(name string) uintptr
	ioServiceGetMatchingServices    func(mainPort uint32, matching uintptr, iterator unsafe.Pointer) int32
	ioIteratorNext                  func(iterator uint32) uint32
	ioObjectRelease                 func(object uint32) int32
	ioRegistryEntryCreateCFProperty func(entry uint32, key uintptr, allocator uintptr, options uint32) uintptr
)

func loadIOKit() error {
	iokitOnce.Do(func() {
		h, err := purego.Dlopen(iokitPath, purego.RTLD_GLOBAL|purego.RTLD_LAZY)
		if err != nil {
			iokitErr = fmt.Errorf("load IOKit: %w", err)
			return
		}
		purego.RegisterLibFunc(&ioServiceMatching, h, "IOServiceMatching")
		purego.RegisterLibFunc(&ioServiceGetMatchingServices, h, "IOServiceGetMatchingServices")
		purego.RegisterLibFunc(&ioIteratorNext, h, "IOIteratorNext")
		purego.RegisterLibFunc(&ioObjectRelease, h, "IOObjectRelease")
		purego.RegisterLibFunc(&ioRegistryEntryCreateCFProperty, h, "IORegistryEntryCreateCFProperty")
	})
	return iokitErr
}

// FindDFU lists the USB devices in DFU mode that the host registry reports.
// It only reads the registry.
func FindDFU() ([]DFUDevice, error) {
	if err := loadIOKit(); err != nil {
		return nil, err
	}
	var devices []DFUDevice
	var scanErr error
	err := native.Autoreleased(func() {
		matching := ioServiceMatching("IOUSBHostDevice")
		if matching == 0 {
			scanErr = fmt.Errorf("IOServiceMatching failed") //nolint:err113 // native failure without detail
			return
		}
		var iter uint32
		// IOServiceGetMatchingServices consumes the matching dictionary.
		if kr := ioServiceGetMatchingServices(0, matching, unsafe.Pointer(&iter)); kr != 0 {
			scanErr = fmt.Errorf("IOServiceGetMatchingServices: kern_return %d", kr) //nolint:err113 // native status
			return
		}
		defer ioObjectRelease(iter)
		product, serial := purego.NSString("USB Product Name"), purego.NSString("USB Serial Number")
		for entry := ioIteratorNext(iter); entry != 0; entry = ioIteratorNext(iter) {
			name := registryString(entry, product)
			if strings.Contains(name, "DFU") {
				if d, err := ParseDFUSerial(registryString(entry, serial)); err == nil {
					devices = append(devices, d)
				}
			}
			ioObjectRelease(entry)
		}
	})
	if err != nil {
		return nil, err
	}
	return devices, scanErr
}

func registryString(entry uint32, key purego.ID) string {
	v := ioRegistryEntryCreateCFProperty(entry, uintptr(key), 0, 0)
	if v == 0 {
		return ""
	}
	defer purego.Release(purego.ID(v)) // the Create rule: this call owns v
	return purego.GoString(purego.ID(v))
}
