# Essential references retained before retiring the previous spike

Captured 22 October 2026. The previous repository's entire research directory is being removed. This note is the deliberately small handoff to the fresh `go-ios-vm` project. No executable, parser, patch, firmware image, disk, capture corpus or report tree is required from that directory.

Historical source checkpoint: `weaveplatform/guestweave-cli-macos` commit `06bb9ed`. The consulted records were the IPSW index, T8030 image profile, machine decision summary, CPU-domain review, route comparison, UI workload and provider/compiler decision. Their implementation decisions are not adopted here. The one untracked compiler-transport inquiry was an unfinished investigation plan, not an additional result to preserve.

## Image identity: retain the distinction between seed and release

The previous image was **iOS 14.0 seed build 18A5351d**, not public release 18A373. It identified `iPhone12,1`, board `n104ap`, T8030/A13, and a Research Developer Erase Install identity. This is a historical reference candidate; it does not silently replace the new project's still-unqualified iOS 14/15 comparison pair.

- Original filename: `iPhone11,8,iPhone12,1_14.0_18A5351d_Restore.ipsw`.
- Recorded archive length: `5507405643` bytes.
- Previously recorded archive SHA-256: `706742ed60327a5247c29ccc5bf7e6ce9bbf11dd30a468f8f6fc9da76bdca96d`.
- Previously recorded BuildManifest SHA-256: `b21b2cedefcd2f0bc76cabbe6475d8f0fa8ff0ea94ad749d23218eb96f2d85e0`.
- [Original Apple CDN URL](https://updates.cdn-apple.com/2020SummerSeed/fullrestores/001-35886/5FE9BE2E-17F8-41C8-96BB-B76E2B225888/iPhone11,8,iPhone12,1_14.0_18A5351d_Restore.ipsw).

An HTTP HEAD request on 22 October returned 200 and the recorded byte length. No full download or fresh digest/BuildManifest verification was performed. The recorded hashes are identification hints until checked against a newly acquired archive. Availability today does not guarantee future availability; losing the old local copy can require another qualified image if Apple later removes it. The fresh project does not require preserving that copy.

The [qemu-t8030 setup guide](https://github.com/TrungNguyen1909/qemu-t8030/wiki/Bringing-up-the-emulator) is the public starting point for this image family. Current GDMF release metadata is not a historical restore archive. Image resolution must retain the source route, exact build and independently checked manifest; a seed and a public release cannot be treated as interchangeable because both say 14.0.

## Lessons worth retaining, with their limits

| Subject | Useful constraint for future work | What remains to demonstrate here |
| --- | --- | --- |
| Execution | Public Hypervisor.framework exposes CPU/memory execution primitives; it does not provide an emulated iPhone machine. TCG device-model behavior is not proof of equivalent native-HVF CPU semantics. | Whole-machine profile, execution compatibility and SIP-enabled operation on the intended host. |
| CPU authentication | Task/key domains, address geometry, context switching and authentication-failure behavior must be treated together. Newer host semantics can differ from an older guest's assumptions. | Correct success/failure and restoration behavior for the chosen image; do not inherit old PC-specific adjustments or assume a displayed CPU identity establishes compatibility. |
| Graphics | A boot framebuffer or Apple logo does not establish app rendering. Device discovery, driver/provider ABI, shader/compiler inputs, surface ownership, completion and display feedback form one contract. | Usable home screen and app content, input effects, cold compilation, reset and sustained use. |
| Shader compilation | Source text, binary libraries and CoreImage filter/DAG inputs are different requirements. A working binary translator alone is not a complete guest frontend. | Whether a selected original frontend can operate independently of its original GPU service; provider installation and actual workload requirements remain open. |
| Input | A host-enqueued contact does not prove guest consumption. Interrupt delivery, coordinates, contact identity, cancellation, disconnect and reset need a coherent device contract. | Correct touch, scrolling and guest-keyboard input with visible effects. |
| Persistence | Storage completion, filesystem durability, identity/key state and clock policy must agree across reboot and recovery. Saving a few bytes is not a complete protected-data lifecycle. | Same-instance settings/app data surviving cold reopens and guest reboot. |
| Inspection | ZIP, IM4P, decoded Mach-O, disk-image, filesystem and dyld-cache offsets are different coordinate spaces. Unsupported decoding is not evidence of absence. | Source-recoverable findings on the exact new image pair. |
| APFS | The old reader encountered a resource-fork padding case; that historical report does not establish a gap in today's `go-apfs-v3`. Live volumes and snapshots require distinct views. | Reproduce any actual incompatibility with the current dependency; put required codec fixes upstream and preserve strict length/checksum checks. |

Historical notes reported installed-root userspace and Apple-logo frames. They did **not** establish a usable home screen, complete input/persistence or fully enabled SIP. With their raw traces retired, these remain historical reports, not fresh acceptance evidence. The new project starts with no demonstrated guest capability.

## Public references to consult directly

- [Apple Hypervisor.framework](https://developer.apple.com/documentation/hypervisor) and [hypervisor entitlement](https://developer.apple.com/documentation/bundleresources/entitlements/com.apple.security.hypervisor): public host execution boundary.
- [Apple XNU PAC documentation at bb611c8](https://github.com/apple-oss-distributions/xnu/blob/bb611c8fecc755a0d8e56e2fa51513527c5b7a0e/doc/pac.md): authentication domains and task/context distinctions. Reconcile exact release behavior rather than transplanting current code.
- [qemu-t8030 at fd4b0f7](https://github.com/TrungNguyen1909/qemu-t8030/tree/fd4b0f790903044d90b8a35fcf03758401252063) and [project status](https://github.com/TrungNguyen1909/qemu-t8030/wiki/Project-status): T8030 board/device and boot references, not a qualification of our runner.
- [Inferno at 6541861](https://github.com/ChefKissInc/Inferno/tree/6541861f4d033a0de327b5ccc5180ebac20bf461), particularly `hw/input/mt-spi.c`, `hw/arm/t8030.c` and CPU initialization: additional device-model references; check accelerator and authentication policies explicitly.
- [vphone at 2a0d3ac](https://github.com/Lakr233/vphone-cli/tree/2a0d3aca9fb79344d939365cc337b915ae9f2f97): an alternative Virtualization/PCC route. Recheck its firmware/security requirements; do not equate it with the proposed iOS 14/HVF target.
- [OpenSEP at 74b2359](https://github.com/owngoal-dev/OpenSEP/tree/74b2359e606c0a70ee0284d0f0c4bcd32c79fb09): ROM/platform handoff reference; distinguish ROM execution from the complete persistent service contract and verify the actual hardware/OS matrix.
- [Apple File System Reference](https://developer.apple.com/support/downloads/Apple-File-System-Reference.pdf) and [go-apfs-v3](https://github.com/deploymenttheory/go-apfs-v3): image/filesystem interpretation and dependency ownership.
- [blacktop/ipsw](https://github.com/blacktop/ipsw), [Apple Objective-C runtime](https://github.com/apple-oss-distributions/objc4) and [Apple XNU](https://github.com/apple-oss-distributions/xnu): firmware/container, runtime metadata and kernel reference sources. Symbols and imports locate questions; they do not prove runtime reachability.

These links replace reliance on copies in the retired tree. They are research references, not approved implementation plans, newly qualified capabilities or a requirement to preserve every historical source revision locally.

## Deletion readiness

The new repository's Go module, CLI, tests, local database setup and CI contain no path, symlink, build replacement, import or fixture dependency on the legacy research tree. This note and the fresh design documents contain the material needed to continue independently. There is no remaining project requirement to keep the old tree until a runner works.

The reviewed legacy allocation was approximately 17 GiB of mobile assets, 5.1 GiB of IPSWs, 993 MiB of machine research and 35 MiB of the original spike. Deleting those local files and removing tracked research from the old repository is a separate cleanup operation; it was not performed by this handoff. Git history and APFS snapshots may mean filesystem free-space recovery differs from the directory-size total.
