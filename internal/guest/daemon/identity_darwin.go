package daemon

import (
	"fmt"
	"os"
	"syscall"

	"howett.net/plist"

	"github.com/deploymenttheory/go-ios-vm/internal/guest"
)

// systemVersion is the guest's SystemVersion.plist.
const systemVersion = "/System/Library/CoreServices/SystemVersion.plist"

// ReadIdentity reads the product type from hw.machine and the version and
// build from SystemVersion.plist.
func ReadIdentity() (guest.DeviceInfo, error) {
	machine, err := syscall.Sysctl("hw.machine")
	if err != nil {
		return guest.DeviceInfo{}, fmt.Errorf("sysctl hw.machine: %w", err)
	}
	data, err := os.ReadFile(systemVersion)
	if err != nil {
		return guest.DeviceInfo{}, fmt.Errorf("read system version: %w", err)
	}
	var v struct {
		ProductVersion      string `plist:"ProductVersion"`
		ProductBuildVersion string `plist:"ProductBuildVersion"`
	}
	if _, err := plist.Unmarshal(data, &v); err != nil {
		return guest.DeviceInfo{}, fmt.Errorf("parse system version: %w", err)
	}
	return guest.DeviceInfo{ProductType: machine, Version: v.ProductVersion, Build: v.ProductBuildVersion}, nil
}
