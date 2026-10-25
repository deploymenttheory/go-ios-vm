package vz

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/deploymenttheory/go-bindings-macosplatform/bindings/frameworks/foundation"
	"github.com/deploymenttheory/go-bindings-macosplatform/bindings/frameworks/virtualization"
	"github.com/deploymenttheory/go-bindings-macosplatform/bindings/runtime/obj"
	"github.com/deploymenttheory/go-bindings-macosplatform/bindings/runtime/purego"
	"github.com/deploymenttheory/go-bindings-macosplatform/bindings/runtime/rt"

	"github.com/deploymenttheory/go-ios-vm/internal/assets"
	"github.com/deploymenttheory/go-ios-vm/internal/host/native"
	"github.com/deploymenttheory/go-ios-vm/internal/machine"
)

// ErrInvalidConfiguration reports a configuration Virtualization rejects.
var ErrInvalidConfiguration = errors.New("Virtualization rejects the machine configuration") //nolint:staticcheck // Virtualization is a proper noun

// Configuration is a validated native configuration for one machine and the
// serial pipes attached to it.
type Configuration struct {
	VZ   *virtualization.VirtualMachineConfiguration
	ECID uint64
	// SerialOutput carries the guest's PL011 output; SerialInput feeds it.
	SerialOutput *os.File
	SerialInput  *os.File
	// guestSide holds the pipe ends Virtualization uses, open for as long as
	// the configuration lives.
	guestSide []*os.File
}

// Close closes the serial pipes. Call it once the machine has stopped.
func (c *Configuration) Close() {
	for _, f := range append([]*os.File{c.SerialOutput, c.SerialInput}, c.guestSide...) {
		if f != nil {
			_ = f.Close()
		}
	}
}

func check(caps ...native.Capability) error {
	for _, c := range caps {
		if err := native.Check(c); err != nil {
			return err
		}
	}
	return nil
}

func private(class, selector string) native.Capability {
	return native.Capability{Class: class, Selector: selector, Private: true, Use: "HOST-02"}
}

// Build creates the native configuration for a provisioned machine and has
// Virtualization validate it. A rejected configuration returns an error
// wrapping ErrInvalidConfiguration and Virtualization's own error.
func Build(m *machine.Machine, screen Screen) (*Configuration, error) {
	c := m.Config
	path := func(name string) string { return filepath.Join(m.Dir, name) }
	if err := verifyStorage(m); err != nil {
		return nil, err
	}
	if err := check(
		private("VZMacOSBootLoader", "_setROMURL:"),
		private("_VZSEPCoprocessorConfiguration", "initWithStorageURL:"),
		private("_VZSEPCoprocessorConfiguration", "setRomBinaryURL:"),
		private("VZVirtualMachineConfiguration", "_setCoprocessors:"),
		private("_VZPL011SerialPortConfiguration", ""),
		private("_VZUSBTouchScreenConfiguration", ""),
		private("VZVirtualMachineConfiguration", "_setMultiTouchDevices:"),
	); err != nil {
		return nil, err
	}
	model, err := supportedResearchModel()
	if err != nil {
		return nil, err
	}
	id, err := identifier(c.Identifier)
	if err != nil {
		return nil, err
	}
	ecid, err := ECID(c.Identifier)
	if err != nil {
		return nil, err
	}

	cfg := &Configuration{ECID: ecid}
	ok := false
	defer func() {
		if !ok {
			cfg.Close()
		}
	}()

	platform := virtualization.NewMacPlatformConfiguration().
		WithHardwareModel(model).
		WithMachineIdentifier(id).
		WithAuxiliaryStorage(virtualization.NewMACAuxiliaryStorageWithURL(path(c.Files.NVRAM)))
	boot := virtualization.NewMacOSBootLoader()

	disk, err := virtualization.NewDiskImageStorageDeviceAttachmentWithURLReadOnlyCachingModeSynchronizationMode(
		path(c.Files.Disk), false, virtualization.DiskImageCachingModeCached, virtualization.DiskImageSynchronizationModeFull)
	if err != nil {
		return nil, fmt.Errorf("%w: disk: %w", ErrStorage, err)
	}

	serial, err := cfg.serialPort()
	if err != nil {
		return nil, err
	}
	network := virtualization.NewVirtioNetworkDeviceConfiguration().
		WithAttachment(virtualization.NewNATNetworkDeviceAttachment()).
		WithMACAddress(virtualization.NewMACAddressWithString(MACAddress(c.Identifier).String()))

	vz := virtualization.NewVirtualMachineConfiguration().
		WithBootLoader(boot).
		WithPlatform(platform).
		WithCPUCount(int(c.Hardware.CPUs)). //nolint:gosec // validated CPU counts fit in int
		WithMemorySize(c.Hardware.MemoryMiB << 20).
		WithGraphicsDevices(virtualization.NewMacGraphicsDeviceConfiguration().WithDisplays(
			virtualization.NewMACGraphicsDisplayConfigurationWithWidthInPixelsHeightInPixelsPixelsPerInch(screen.Width, screen.Height, screen.PixelsPerInch))).
		WithStorageDevices(virtualization.NewVirtioBlockDeviceConfigurationWithAttachment(
			virtualization.StorageDeviceAttachmentFromID(purego.ID(obj.ID(disk))))).
		WithNetworkDevices(network).
		WithSerialPorts(serial).
		WithEntropyDevices(virtualization.NewVirtioEntropyDeviceConfiguration()).
		WithKeyboards(virtualization.NewUSBKeyboardConfiguration()).
		WithSocketDevices(virtualization.NewVirtioSocketDeviceConfiguration())

	var sendErr error
	err = native.Autoreleased(func() {
		purego.Send[purego.ID](purego.ID(obj.ID(boot)), purego.RegisterName("_setROMURL:"), rt.FileURL(path(c.Files.ROM)))

		sepAlloc := purego.Send[purego.ID](purego.ID(purego.GetClass("_VZSEPCoprocessorConfiguration")), purego.RegisterName("alloc"))
		sep := purego.Send[purego.ID](sepAlloc, purego.RegisterName("initWithStorageURL:"), rt.FileURL(path(c.Files.SEPStorage)))
		if sep == 0 {
			sendErr = fmt.Errorf("%w: SEP coprocessor configuration", ErrStorage)
			return
		}
		defer purego.Release(sep)
		purego.Send[purego.ID](sep, purego.RegisterName("setRomBinaryURL:"), rt.FileURL(path(c.Files.SEPROM)))
		purego.Send[purego.ID](purego.ID(obj.ID(vz)), purego.RegisterName("_setCoprocessors:"), array(sep))

		touch := purego.Send[purego.ID](purego.ID(purego.GetClass("_VZUSBTouchScreenConfiguration")), purego.RegisterName("new"))
		defer purego.Release(touch)
		purego.Send[purego.ID](purego.ID(obj.ID(vz)), purego.RegisterName("_setMultiTouchDevices:"), array(touch))
	})
	if err == nil {
		err = sendErr
	}
	if err != nil {
		return nil, err
	}

	if err := vz.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidConfiguration, err)
	}
	cfg.VZ = vz
	ok = true
	return cfg, nil
}

// serialPort attaches a PL011 port to a pair of pipes: the guest writes to
// one whose read end is SerialOutput and reads from one fed by SerialInput.
func (c *Configuration) serialPort() (*virtualization.SerialPortConfiguration, error) {
	outR, outW, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("serial output pipe: %w", err)
	}
	inR, inW, err := os.Pipe()
	if err != nil {
		_ = outR.Close()
		_ = outW.Close()
		return nil, fmt.Errorf("serial input pipe: %w", err)
	}
	c.SerialOutput, c.SerialInput = outR, inW
	c.guestSide = []*os.File{outW, inR}
	attachment := virtualization.NewFileHandleSerialPortAttachmentWithFileHandleForReadingFileHandleForWriting(
		foundation.NewFileHandleWithFileDescriptor(int(inR.Fd())),  //nolint:gosec // file descriptors fit in int
		foundation.NewFileHandleWithFileDescriptor(int(outW.Fd())), //nolint:gosec // file descriptors fit in int
	)
	id := purego.Send[purego.ID](purego.ID(purego.GetClass("_VZPL011SerialPortConfiguration")), purego.RegisterName("new"))
	port := virtualization.SerialPortConfigurationFromID(id) // retains
	purego.Release(id)
	return port.WithAttachment(attachment), nil
}

func array(items ...purego.ID) purego.ID {
	return purego.SliceToNSArray(items, func(id purego.ID) purego.ID { return id })
}

// verifyStorage checks the machine's storage before any native object is
// built: the disk and NVRAM must be regular files and both ROM copies must
// match their pinned digests. SEP storage may be absent before first start.
func verifyStorage(m *machine.Machine) error {
	c := m.Config
	for _, name := range []string{c.Files.Disk, c.Files.NVRAM} {
		exists, err := regularIfPresent(filepath.Join(m.Dir, name))
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("%w: %s has no %s; provision the machine first", ErrStorage, m.Name, name)
		}
	}
	if _, err := regularIfPresent(filepath.Join(m.Dir, c.Files.SEPStorage)); err != nil {
		return err
	}
	lock, err := assets.Load()
	if err != nil {
		return err
	}
	for _, rom := range []struct{ file, asset string }{{c.Files.ROM, "openavb-rom"}, {c.Files.SEPROM, "opensep-rom"}} {
		a, err := lock.Asset(rom.asset)
		if err != nil {
			return err
		}
		if err := assets.VerifyFile(a, filepath.Join(m.Dir, rom.file)); err != nil {
			return err
		}
	}
	return nil
}
