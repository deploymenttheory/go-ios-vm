# The research virtual machine

`internal/host/vz` turns a machine from the store into a Virtualization machine. It follows vphone-cli's VM layer at the pinned revision (`VPhoneVirtualMachine.swift`, `VPhoneVirtualMachineHardwareModel.swift`).

| Part | How it is configured |
| --- | --- |
| Hardware model | Private `_VZMacHardwareModelDescriptor`: platform version 3, board 0x90, ISA 2 (vresearch101). Virtualization supports it only for a process carrying the private virtualization entitlements. |
| Identity | `VZMacMachineIdentifier`, created once (`NewIdentifier`) and stored as its data representation. The ECID comes from the private `_ECID`, read as an integer or an `NSNumber` depending on the method's return type. |
| ROMs | The vendored OpenAVB (`_setROMURL:`) and OpenSEP (`setRomBinaryURL:`), copied into the machine and verified against `internal/assets` before every use. |
| NVRAM | `VZMacAuxiliaryStorage`, created for the research model on first provision and reused after that. |
| SEP | `_VZSEPCoprocessorConfiguration` with the machine's SEP storage, attached through `_setCoprocessors:`. |
| Disk | A sparse file of the configured size (decimal GB), attached with caching `cached` and synchronization `full`. vphone-cli found that automatic caching on a sparse image corrupts it under memory compression. |
| Devices | PL011 serial over two pipes, NAT networking with a MAC derived from the identifier, the Mac graphics display (1290×2796 at 460 ppi), USB touchscreen, USB keyboard, Virtio entropy and Virtio socket. |
| Lifecycle | A `VM` bound to its own dispatch queue. `Start` sets `_setForceDFU:` when asked and waits for the completion off the queue (`internal/host/native`). `State` and `Stop` run on that queue too. |
| DFU discovery | `FindDFU` reads `IOUSBHostDevice` entries from the I/O Registry. `MatchDFU` accepts only the device whose serial string carries the stored ECID, never another phone in DFU. |

## Exceptions

Some Virtualization initializers raise an Objective-C exception instead of returning an error. An exception aborts the process, and `recover` cannot catch it. The case observed on macOS 27.0.1 is NVRAM creation for an unsupported hardware model.

For that reason, `Provision` and `Build` check that the research model is supported for this process first, and return `ErrUnsupportedModel` when it is not. A machine is created only from a configuration Virtualization has validated.

## Running a live DFU start

```sh
scripts/live-vm-test.sh ./internal/host/vz Live
```

The script builds the package's test binary, signs it ad hoc with `build/vm.entitlements` (vphone-cli's VM entitlements) and runs the live tier.

The host has to admit an ad hoc signed binary with private entitlements, and research guests have to be allowed. On a host that hasn't been prepared, the binary is killed at launch. `go-ios-vm host preflight` lists the manual preparation needed.
