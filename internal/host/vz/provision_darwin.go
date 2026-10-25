package vz

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/deploymenttheory/go-bindings-macosplatform/bindings/frameworks/virtualization"
	"github.com/deploymenttheory/go-bindings-macosplatform/bindings/runtime/obj"
	"github.com/deploymenttheory/go-bindings-macosplatform/bindings/runtime/purego"

	"github.com/deploymenttheory/go-ios-vm/internal/assets"
	"github.com/deploymenttheory/go-ios-vm/internal/host/native"
	"github.com/deploymenttheory/go-ios-vm/internal/machine"
)

// Errors from provisioning and configuration.
var (
	ErrIdentifier = errors.New("invalid machine identifier")
	ErrStorage    = errors.New("machine storage is unusable")
	// ErrUnsupportedModel reports that Virtualization refuses the PV=3 research
	// hardware model to this process. Virtualization raises an Objective-C
	// exception, which cannot be recovered, when NVRAM or a configuration uses
	// an unsupported model, so callers check this first.
	ErrUnsupportedModel = errors.New("Virtualization refuses the research hardware model to this process; it needs the signed VM entitlements on a prepared host") //nolint:staticcheck // Virtualization is a proper noun
)

// NewIdentifier creates a machine identifier through Virtualization and returns
// its data representation, which a machine stores once and never regenerates.
func NewIdentifier() ([]byte, error) {
	if err := native.LoadVirtualization(); err != nil {
		return nil, err
	}
	data := virtualization.NewMacMachineIdentifier().DataRepresentation()
	if len(data) == 0 {
		return nil, fmt.Errorf("%w: Virtualization returned an empty identifier", ErrIdentifier)
	}
	return data, nil
}

func identifier(data []byte) (*virtualization.MacMachineIdentifier, error) {
	if err := native.LoadVirtualization(); err != nil {
		return nil, err
	}
	id := virtualization.NewMACMachineIdentifierWithDataRepresentation(data)
	if id == nil {
		return nil, fmt.Errorf("%w: Virtualization rejects the stored %d bytes", ErrIdentifier, len(data))
	}
	return id, nil
}

var ecidCapability = native.Capability{Class: "VZMacMachineIdentifier", Selector: "_ECID", Private: true, Use: "HOST-02", Purpose: "matching DFU to the stored identity"}

// ECID returns the ECID the stored identifier gives the virtual device.
// Depending on the macOS build, _ECID returns an unsigned integer or an
// NSNumber; the method's return type decides which.
func ECID(data []byte) (uint64, error) {
	id, err := identifier(data)
	if err != nil {
		return 0, err
	}
	if err := native.Check(ecidCapability); err != nil {
		return 0, err
	}
	sel := purego.RegisterName("_ECID")
	sig := purego.Send[purego.ID](purego.ID(purego.GetClass("VZMacMachineIdentifier")),
		purego.RegisterName("instanceMethodSignatureForSelector:"), sel)
	if sig == 0 {
		return 0, fmt.Errorf("%w: no signature for -[VZMacMachineIdentifier _ECID]", ErrIdentifier)
	}
	ret := purego.GoCString(purego.Send[uintptr](sig, purego.RegisterName("methodReturnType")))
	var ecid uint64
	switch ret {
	case "Q", "q":
		ecid = purego.Send[uint64](purego.ID(obj.ID(id)), sel)
	case "@":
		var number purego.ID
		if err := native.Autoreleased(func() { number = purego.Send[purego.ID](purego.ID(obj.ID(id)), sel) }); err != nil {
			return 0, err
		}
		if number == 0 {
			return 0, fmt.Errorf("%w: _ECID returned nil", ErrIdentifier)
		}
		ecid = purego.Send[uint64](number, purego.RegisterName("unsignedLongLongValue"))
	default:
		return 0, fmt.Errorf("%w: _ECID returns %q", ErrIdentifier, ret)
	}
	if ecid == 0 {
		return 0, fmt.Errorf("%w: ECID is zero", ErrIdentifier)
	}
	return ecid, nil
}

// researchModel is vphone-cli's PV=3 research model descriptor: platform
// version 3, board 0x90 and ISA 2, matching vresearch101.
var researchModel = struct {
	PlatformVersion, BoardID uint32
	ISA                      int64
}{PlatformVersion: 3, BoardID: 0x90, ISA: 2}

// researchHardwareModel builds the PV=3 hardware model object. It exists in any
// process; whether it is supported depends on the process's entitlements.
func researchHardwareModel() (*virtualization.MacHardwareModel, error) {
	var model *virtualization.MacHardwareModel
	var buildErr error
	err := native.Autoreleased(func() {
		for _, sel := range []string{"setPlatformVersion:", "setBoardID:", "setISA:"} {
			if buildErr = native.Check(native.Capability{Class: "_VZMacHardwareModelDescriptor", Selector: sel}); buildErr != nil {
				return
			}
		}
		if buildErr = native.Check(native.Capability{Class: "VZMacHardwareModel", Selector: "_hardwareModelWithDescriptor:", ClassMethod: true}); buildErr != nil {
			return
		}
		d := researchModel
		number := purego.ID(purego.FoundationClass("NSNumber"))
		desc := purego.Send[purego.ID](purego.ID(purego.GetClass("_VZMacHardwareModelDescriptor")), purego.RegisterName("new"))
		defer purego.Release(desc)
		purego.Send[purego.ID](desc, purego.RegisterName("setPlatformVersion:"), purego.Send[purego.ID](number, purego.RegisterName("numberWithUnsignedInt:"), d.PlatformVersion))
		purego.Send[purego.ID](desc, purego.RegisterName("setBoardID:"), purego.Send[purego.ID](number, purego.RegisterName("numberWithUnsignedInt:"), d.BoardID))
		purego.Send[purego.ID](desc, purego.RegisterName("setISA:"), purego.Send[purego.ID](number, purego.RegisterName("numberWithLongLong:"), d.ISA))
		id := purego.Send[purego.ID](purego.ID(purego.GetClass("VZMacHardwareModel")), purego.RegisterName("_hardwareModelWithDescriptor:"), desc)
		model = virtualization.MacHardwareModelFromID(id) // retains past the pool
	})
	if err != nil {
		return nil, err
	}
	if buildErr != nil {
		return nil, buildErr
	}
	if model == nil {
		return nil, fmt.Errorf("%w: Virtualization returned no research hardware model", ErrStorage)
	}
	return model, nil
}

// supportedResearchModel returns the research hardware model only when
// Virtualization supports it for this process.
func supportedResearchModel() (*virtualization.MacHardwareModel, error) {
	model, err := researchHardwareModel()
	if err != nil {
		return nil, err
	}
	if !purego.Send[bool](purego.ID(obj.ID(model)), purego.RegisterName("isSupported")) {
		return nil, ErrUnsupportedModel
	}
	return model, nil
}

// Provision creates the machine's missing storage: the ROM copies, the sparse
// disk and NVRAM. Existing files are reused only if they are regular files,
// and existing ROM copies must match their pinned digests; nothing existing is
// overwritten. SEP storage is created by Virtualization on first start.
func Provision(m *machine.Machine, o *machine.Owner) error {
	if err := o.Check(m); err != nil {
		return err
	}
	c := m.Config
	for _, rom := range []struct{ file, asset string }{{c.Files.ROM, "openavb-rom"}, {c.Files.SEPROM, "opensep-rom"}} {
		if err := provisionROM(filepath.Join(m.Dir, rom.file), rom.asset); err != nil {
			return err
		}
	}
	if err := provisionDisk(filepath.Join(m.Dir, c.Files.Disk), int64(c.Hardware.DiskSizeGB)*1_000_000_000); err != nil { //nolint:gosec // disk sizes fit in int64
		return err
	}
	nvram := filepath.Join(m.Dir, c.Files.NVRAM)
	exists, err := regularIfPresent(nvram)
	if err != nil || exists {
		return err
	}
	model, err := supportedResearchModel()
	if err != nil {
		return err
	}
	if _, err := virtualization.NewMACAuxiliaryStorageCreatingStorageAtURLHardwareModelOptions(nvram, model, 0); err != nil {
		_ = os.Remove(nvram) //nolint:gosec // the file this call tried to create
		return fmt.Errorf("%w: create NVRAM: %w", ErrStorage, err)
	}
	return nil
}

func provisionROM(path, asset string) error {
	lock, err := assets.Load()
	if err != nil {
		return err
	}
	a, err := lock.Asset(asset)
	if err != nil {
		return err
	}
	if err := assets.VerifyFile(a, path); !errors.Is(err, assets.ErrMissing) {
		return err // nil for a matching copy, or why the existing copy is unusable
	}
	data, err := assets.ROM(asset)
	if err != nil {
		return err
	}
	tmp := path + ".partial"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("%w: write %s: %w", ErrStorage, filepath.Base(path), err)
	}
	if err := os.Rename(tmp, path); err != nil { //nolint:gosec // inside the owned machine directory
		_ = os.Remove(tmp) //nolint:gosec // the file this call wrote
		return fmt.Errorf("%w: install %s: %w", ErrStorage, filepath.Base(path), err)
	}
	return assets.VerifyFile(a, path)
}

func provisionDisk(path string, size int64) error {
	exists, err := regularIfPresent(path)
	if err != nil || exists {
		return err
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // inside the owned machine directory
	if err != nil {
		return fmt.Errorf("%w: create disk: %w", ErrStorage, err)
	}
	if err := f.Truncate(size); err != nil {
		_ = f.Close()
		_ = os.Remove(path) //nolint:gosec // the file this call created
		return fmt.Errorf("%w: size disk: %w", ErrStorage, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("%w: create disk: %w", ErrStorage, err)
	}
	return nil
}

// regularIfPresent reports whether path exists, failing for anything but a
// regular file.
func regularIfPresent(path string) (bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("%w: %w", ErrStorage, err)
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("%w: %s is not a regular file", ErrStorage, path)
	}
	return true, nil
}
