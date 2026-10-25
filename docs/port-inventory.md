# Port inventory

This is what the runner ports from vphone-cli and what it takes from elsewhere. It covers the supported pair, iPhone17,3 iOS 27.0.1 (24A446) + cloudOS 26.4 (23E5207q). For each item it records the provenance and terms actually found, and what is deferred or forbidden.

`internal/assets/lock.json` is the machine-readable lock for the native exceptions and firmware inputs. `docs/port-patches.md` lists every upstream patch with its first-release disposition.

## Pins

| Source | Revision | Licence | Use |
| --- | --- | --- | --- |
| [vphone-cli](https://github.com/Lakr233/vphone-cli/tree/b2804adb45a653ff3981f0705108975414dc8d5d) | `b2804adb45a653ff3981f0705108975414dc8d5d` | MIT, © 2026 @Lakr233, except the directories listed under [Licensing findings](#licensing-findings) | Porting reference for the host, firmware preparation, patches, restore and guest daemon |
| [go-bindings-macosplatform](https://github.com/deploymenttheory/go-bindings-macosplatform/tree/d5b3d0ecb48ed106ddac46095bd91ec945b2833c) | `d5b3d0ecb48ed106ddac46095bd91ec945b2833c` | MIT | Native macOS framework access through purego, required in `go.mod` |
| [OpenAVB](https://github.com/owngoal-dev/OpenAVB/tree/65a776e9ae00b2e1de22b18cbbad1359945d7277) | `65a776e9ae00b2e1de22b18cbbad1359945d7277` | MIT, with some APSL-2.0 files | Boot ROM, vendored as built by vphone-cli |
| [OpenSEP](https://github.com/owngoal-dev/OpenSEP/tree/8b1633f7e6606f0bc9e1422c93e8d2e056154d2f) | `8b1633f7e6606f0bc9e1422c93e8d2e056154d2f` | MIT | SEP boot ROM, vendored as built by vphone-cli |

`go-ios-vm version` reports the vphone-cli and binding pins. `go.mod` and `cmd/go-ios-vm` tests enforce the binding pin.

## Firmware inputs

The user supplies both IPSWs. They are acquired into the IPSW cache, which is `~/Library/Caches/go-ios-vm/ipsw` on this host (FW-02). Their identities are checked against their build manifests and their digests verified before use. The originals stay immutable. Nothing from them is committed.

| Asset | Identity | Supplies |
| --- | --- | --- |
| `iphone-ipsw` | iPhone17,3 iOS 27.0.1 (24A446) | Guest userland and the iPhone components of the hybrid restore |
| `cloudos-ipsw` | cloudOS 26.4 (23E5207q) | vphone600 boot chain, kernel, SEP firmware and device tree; the GPU bundle, through a temporary restore |
| `paravirt-gpu-bundle` | From cloudOS 23E5207q | `AppleParavirtGPUMetalIOGPUFamily.bundle`, extracted read-only from the restored System volume (FW-05). Expected `CFBundleVersion=64.4.4` and `DTPlatformVersion=26.4`. Digests are recorded at extraction. |

According to upstream's compatibility notes, cloudOS 23E5207q is the newest cloudOS that still contains `vphone600ap`. iPhone17,3 24A446 needs the TXM `txm-boot-precheck_admission` patch to pass first boot.

## Native exceptions

These are the only non-Go components the runner uses. The host process itself stays pure Go with `CGO_ENABLED=0`.

| Asset | Kind | First release | Pin | Digest policy | Terms |
| --- | --- | --- | --- | --- | --- |
| `openavb-rom` | Boot ROM | Required | OpenAVB `65a776e`, copied from vphone-cli's resource | Pinned SHA-256 `bbcabe73…11cb9c`, 460057 bytes; vendored | MIT + APSL-2.0 notices, `internal/assets/rom/OpenAVB/` |
| `opensep-rom` | SEP boot ROM | Required | OpenSEP `8b1633f`, copied from vphone-cli's resource | Pinned SHA-256 `92690b9c…2b08b12`, 147512 bytes; vendored | MIT notice, `internal/assets/rom/OpenSEP/` |
| `gpu-compiler-plugin` | Guest dylib | Required | vphone-cli `VPhoneGuestComponents/GraphicLoader/main.mm`, byte-identical to its origin at `0xjohnnydev/0xjohnnydev.github.io@0ee9decd` | Source tree pinned; the built digest is recorded in the payload receipt (GUEST-02) | **Unverified**, see below |
| `launchd-hook` | Guest dylib | Candidate | vphone-cli `VPhoneGuestComponents/LaunchHook` + `Shared` | Source tree pinned | MIT (vphone-cli) |
| `system-hook` | Guest dylib | Candidate | vphone-cli `VPhoneGuestComponents/SystemHook` + `FlutterRemapFix` + `Shared` | Source tree pinned | MIT (vphone-cli) |
| `haptics-fix` | Guest dylib | Candidate | vphone-cli `VPhoneGuestComponents/HapticsFix` | Source tree pinned | MIT (vphone-cli) |

The Go guest daemon is not an exception. It is ported to Go (GUEST-01) and may use iOS external linking (R09).

**How assets are verified.**
- `assets.ROM` returns a vendored ROM only after checking its embedded bytes against the lock.
- `assets.VerifyFile` checks a machine's copy before use. An absent file returns `ErrMissing` (wrapping `fs.ErrNotExist`); any size or digest difference returns `ErrMismatch`.
- `assets.SourceTreeDigest` reproduces the source-tree digests. Each is the SHA-256 of `shasum -a 256` lines sorted bytewise by path.

**What a candidate is.** Upstream's standard preset installs these hooks through the boot-tier `system-launchdaemons-boot-environment` patch:
- the launchd hook loads SystemHook at spawn;
- SystemHook loads `libhapticsfix` into SpringBoard, so UIKit doesn't crash on the absent haptic hardware.

The standard device tree also removes the haptics node. Upstream's `minimal` preset installs none of these hooks. Whether the first release needs each candidate is not yet established; FW-08 decides it with evidence, and E2E-02 confirms it on a live guest. A candidate that turns out unnecessary is dropped, not installed by default.

Upstream records that without the GPU compiler plugin, `MTLCompilerService` aborts and the window stays black. That is why it is required.

ROM provenance: the vendored images are vphone-cli's checked-in builds. Their digests match the digests in vphone-cli's resource READMEs. They have not been independently rebuilt from source here; that needs the Homebrew LLVM/LLD 23.1.3 cross toolchain named in the lock.

## Upstream surfaces and where they are ported

| Upstream (at `b2804adb`) | What it does | Ported to |
| --- | --- | --- |
| `VPhoneExecutable/VPhoneCommand/VPhoneCommand` | CLI commands: host preflight, `vm`, `fw`, `restore` | `internal/cli` (FND-01); commands per ticket |
| `VPhoneExecutable/VPhoneVirtualization/UI/VirtualMachine` | VM configuration, private Virtualization selectors (ROM URL, SEP coprocessor, PL011, touch, forced DFU), lifecycle, window | HOST-01 to HOST-04 through the pinned bindings |
| `VPhoneExecutable/VPhoneCommand/VPhoneCommand/Firmware`, `Archive` | Firmware download, source selection, preparation, hybrid tree, CFW installation | FW-02, FW-03, FW-05 to FW-07 |
| `VPhoneExecutable/VPhoneCommand/FirmwarePatcher` | Boot-chain, TXM, kernel, device tree, SEP and userspace patchers; patch sets | FW-03 (containers and primitives), FW-04, FW-08 |
| `VPhoneExecutable/VPhoneCommand/VPhoneRestore/VPhoneRestore` (Swift) | Restore runner, identity, ticket, options, events | REST-02 to REST-05 |
| `VPhoneExecutable/VPhoneCommand/VPhoneRestore/MobileRestoreCore/Recovery/openavb.c` | OpenAVB stage-0 image identity upload before iBSS | REST-05 (OpenAVB behaviour); see licensing |
| `VPhoneExecutable/VPhoneCommand/VPhoneRestore/MobileRestoreCore`, `MobileRecoveryCore` (C) | idevicerestore and libirecovery: DFU/recovery USB, personalization, restored, FDR, ASR | Behaviour reimplemented in Go (REST-01 to REST-04); the C code is never linked or executed |
| `VPhoneExecutable/VPhoneCommand/VPhoneSign` | Signing helpers | Apple-supplied `codesign` invoked explicitly (R10), as needed by GUEST-02 and FW-07 |
| `VPhoneDaemon` (Swift + Objective-C) | vphoned: HTTP/WebSocket API over VSOCK and many guest features | GUEST-01: readiness, device information and shutdown only |
| `VPhoneGuestComponents` | Guest dylibs and helpers | Native exceptions above; everything else deferred |
| `VPhoneExecutable/VPhoneVirtualization/Resources/OpenAVB`, `OpenSEP` | ROM images and licences | Vendored in `internal/assets/rom` |

## Forbidden host dependencies

The host must not execute or link any of these (R01). The `cmd/go-ios-vm` module allowlist and no-cgo build tests enforce the linking side.

- **Upstream host executables:**
  - vphone-cli, vphone-vm, `VPhone.bundle` and its executables, and VPhoneLaunchpad;
  - the `VPhoneEscalator` C privilege helper;
  - the `VirtualMachineProcess` and `LocationHelperProcess` helpers.

  Cached VPhone.bundle executables are not reused either.
- **Third-party C restore libraries:**
  - `MobileRestoreCore` (idevicerestore) and `MobileRecoveryCore` (libirecovery);
  - AppleMobileDeviceLibrary, the libimobiledevice family, pinned by SwiftPM at `553a0bf1b55812b1a08c727b1a3084e88871343b`;
  - openssl-spm `9f3b525d960fe71e534482310e96cd9c4f2faa17`;
  - any cgo wrapper of these.
- **Third-party tools:** the `ipsw` tool and KDK/kmutil kernelcache mixing. The latter applies only to research-board pairings and isn't needed for 23E5207q.

Apple-supplied disk, signing and sealing tools may be invoked explicitly (R10).

## Deferred features

R12 defers the following:
- Launchpad, templates, snapshots, transfer and archive commands.
- HTTP automation (vphoned's HTTP, WebSocket, port-forward and file APIs).
- Package management and bootstraps (Sileo, ElleKit, TweakLoader, RootHide).
- Camera (CamFix, VCamCaptured, VCamDataPlane).
- Audio (VirtIOSound, VirtualAudio).
- Sensors (Gyroscope, Accelerometer, Attitude).
- HomePod, Apple TV, iPad and watch support.
- Virtual Bluetooth.
- Developer tooling (debugserver, Xcode installation persona, developer images).
- MISFix, BatteryHealthFix, DeviceName, DeviceHubFix, PrefsFix, SignInFix, CryptexStagingFix and OTA userland replacement.

`docs/port-patches.md` marks the corresponding patches as deferred, as belonging to another device family, or as outside upstream's standard preset.

## Licensing findings

These follow from what each component actually declares. No single licence is assumed to cover everything.

- **vphone-cli is MIT** at the repository level. It covers the ported Swift, Objective-C and C sources outside the directories below; those carry no file-level licence headers. Porting them requires keeping vphone-cli's copyright and permission notice, which is reproduced in `THIRD_PARTY_NOTICES.md`.
- **The upstream C restore stack is LGPL, not MIT.**
  - `MobileRestoreCore` is idevicerestore. Its file headers say LGPL-2.1-or-later, and its `COPYING` file carries the LGPL-3.0 text.
  - `MobileRecoveryCore` is libirecovery, under LGPL-2.1.
  - The AppleMobileDeviceLibrary package is the libimobiledevice family under LGPL.

  The runner never links these (R01). A Go port that closely translates their source could still be a derivative work under the LGPL. **Before REST-01, the project owner needs to decide one of two approaches:**
  1. implement REST-01 to REST-04 from protocol behaviour, documentation and observed transcripts, without translating that source; or
  2. accept LGPL terms for the restore packages.

  `openavb.c` sits in that directory but states that it is not part of idevicerestore. It is treated as vphone-cli (MIT) code.
- **The GPU compiler plugin source has no declared licence.** Its origin repository, `0xjohnnydev/0xjohnnydev.github.io`, has no licence file. vphone-cli redistributes `main.mm` under its MIT licence. The runner pins the source by digest and does not vendor it. **Before GUEST-02 ships a built plugin, the project owner needs to decide whether to:**
  - obtain the author's terms;
  - rely on vphone-cli's redistribution; or
  - have users build it locally from the pinned source.
- **The ROMs are redistributable.** OpenAVB is MIT with APSL-2.0 notices; the modified APSL source is OpenAVB's public repository. OpenSEP is MIT, and its image carries public Apple certificate and key data. The licence texts are vendored next to the images.
- **Apple firmware content** is never committed or redistributed. That covers IPSWs, the GPU bundle, restore tickets and anything extracted from them.
- **This repository has had no `LICENSE` file since commit `d075e67`.** Before that commit it was MIT, © 2026 Deployment Theory. Restoring the licence is the project owner's decision.
