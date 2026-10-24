# GitHub projects attempting to run iOS virtual machines

Research date: **24 October 2026**.

Accuracy review: see the [adversarial audit](research-accuracy-audit.md) and its [project/source checks](research-verification-projects.md). Project-reported guest behavior remains unreproduced locally.

See the [virtualization reference map](virtualization-reference-map.md) for supporting Apple documentation, technical blogs and implementation sources, and the [firmware-analysis report](firmware-analysis-research.md) for the proposed analysis workflow and Go host findings.

This survey prioritizes projects that execute Apple's iOS kernel and userspace, including incomplete attempts. Related app emulators and simulator tools appear separately. Language was not a selection criterion.

The evidence is project-owned repositories, documentation, source files, and author writeups. **No guest was built or booted for this survey.** “Reported” and “documented” capabilities are upstream claims or demonstrations, not independently reproduced results. Linked branches, release pages, and compatibility matrices can change after this date. This is a targeted survey, not an exhaustive GitHub inventory.

## Comparison

“Full-system emulation” describes an approach, not a guarantee that every device or app works. A kernel reaching a shell, an interactive SpringBoard session, and complete iPhone hardware compatibility are different milestones.

| Project | Approach / implementation family | Documented guest target | Host / preparation requirements | Evidence of progress |
| --- | --- | --- | --- | --- |
| [Lakr233/vphone-cli](https://github.com/Lakr233/vphone-cli) | Apple Virtualization.framework + PCC research VM infrastructure; Swift host tooling | Modern iOS; exact iPhone/cloudOS pairings matter | Physical Apple Silicon Mac, macOS 15+, research-guest security configuration | Graphical guest and automation; firmware matrix records lock-screen and guest-daemon checks |
| [wh1te4ever/super-tart-vphone](https://github.com/wh1te4ever/super-tart-vphone) | Tart-derived virtualization; Swift | iOS 26.1 research setup | Apple Silicon Mac, patched firmware and host security changes | Historical implementation; archived, with missing graphics dependency |
| [ChefKissInc/Inferno](https://github.com/ChefKissInc/Inferno) | QEMU-derived Apple ARM hardware emulation; C-based | Apple ARM guests; iOS 14 documented by its iPhone port | Custom emulator build and guest preparation; see qualification below | Continuing qemu-t8030 lineage; upstream setup guide could not be inspected |
| [TrungNguyen1909/qemu-t8030](https://github.com/TrungNguyen1909/qemu-t8030) | QEMU iPhone 11 / A13 emulation; C-based | iOS 14; status checklist also marks iOS 15 support | Linux/macOS build instructions; some preparation requires macOS | Shell and iOS 14 restore documented; original checklist lacks framebuffer, multitouch, SEP and GPU |
| [jprx/darwin-vm](https://github.com/jprx/darwin-vm) | qemu-sptm with Python/Shell preparation scripts | iOS 26.6 and 27.0 on A14–A19 device configurations | Mac for upstream firmware preparation; runtime wherever its QEMU build runs | Root shell, custom programs and kernel debugging; explicitly no graphical iPhone environment |
| [alephsecurity/xnu-qemu-arm64](https://github.com/alephsecurity/xnu-qemu-arm64) | Patched QEMU / iPhone 6s Plus model; C-based | iOS 12.1 | Firmware preparation plus QEMU; ARM/KVM acceleration is a separate path | launchd, bash, TCP-tunneled SSH and kernel debugging |
| [devos50/qemu-ios](https://github.com/devos50/qemu-ios) | QEMU hardware emulation; C-based | iPod touch 1G and 2G, legacy iPhone OS | Firmware, boot ROM, NOR/NAND images and emulator build | Author demonstrates 1G boot through SpringBoard with display and touch |
| [j0shua-SYSON/S5LBox](https://github.com/j0shua-SYSON/S5LBox) | Independent ARMv6 / S5L8900 emulator; portable C core and Objective-C app | iPhone 3G / iPhone OS 3.1.3 | README specifies 64-bit iPhone on iOS 13+, signed app and firmware | Maintainer reports graphical guest operation; architecture contains both older limitations and later validation |
| [MakrSas/Inferno-iOS](https://github.com/MakrSas/Inferno-iOS) | Inferno embedded as a library; SwiftUI frontend | Emulated iPhone 11 / iOS 14 | JIT-enabled iPhone app and separately prepared jailbroken guest image | README and screenshots report framebuffer, touch, app installation and networking |
| [chrisspankroy/ipod-touch-1g](https://github.com/chrisspankroy/ipod-touch-1g) | QEMU S5L8900 model; C-based | Early iPod touch boot firmware | Extracted firmware and custom build; hardcoded local paths | Bootloader experiment only; kernel boot and general console dispatch remain unfinished |

Implementation families describe the host/core split, not GitHub language percentages. Host support above is limited to the inspected project instructions; generic QEMU portability does not establish that a particular fork works on every host.

## Projects in detail

### vphone-cli

The README documents graphical interaction, screenshots/recording, custom firmware, cloning/export, and an optional HTTP/WebSocket automation API. It requires a physical Apple Silicon Mac on macOS 15+, online restore tickets, and adjusted host security settings; nested macOS VMs are excluded. Runtime does not require Xcode, Python or Homebrew. [README](https://github.com/Lakr233/vphone-cli#readme)

The compatibility matrix records iPhone17,3 iOS 26.6.2 and 27.0 paired with cloudOS 26.4 reaching the lock screen and answering guest-daemon pings. Later entries describe additional pairings, including iOS 18. Treat these as specific tested combinations, not blanket compatibility. [Firmware compatibility](https://github.com/Lakr233/vphone-cli/blob/main/Documents/Guides/compatibility.md)

License: MIT. The release history includes 2.12.2 on 24 October, providing recent maintenance evidence. [License](https://github.com/Lakr233/vphone-cli/blob/main/LICENSE), [releases](https://github.com/Lakr233/vphone-cli/releases)

### super-tart-vphone

This historical Tart derivative exposes custom boot ROM, serial, DFU, kernel and SEP debugging. The repository was archived on 25 February 2026. [Repository](https://github.com/wh1te4ever/super-tart-vphone)

Its author writeup describes combining iOS 26.1 userspace with PCC/cloudOS VM components and restoring graphical operation. This is useful background for understanding the modern virtualization route. [Author writeup](https://github.com/wh1te4ever/super-tart-vphone-writeup)

The README explicitly warns of hardcoded patches, incomplete instructions and an omitted Apple graphics plugin. Its inherited Tart installation instructions should not be read as a complete iOS setup guide. [README](https://github.com/wh1te4ever/super-tart-vphone#readme)

License: the checked-in license is **Fair Source 0.9**, with a usage limitation; do not classify it as MIT or assume identical terms to other vphone projects. [License](https://github.com/wh1te4ever/super-tart-vphone/blob/main/LICENSE)

### ChefKiss Inferno

Inferno identifies itself as a QEMU derivative supporting Apple ARM guests. GitHub records its fork relationship to qemu-t8030. Its build workflows provide evidence of continued development beyond the archived original. [Repository and README](https://github.com/ChefKissInc/Inferno), [build activity](https://github.com/ChefKissInc/Inferno/actions)

The README delegates setup and platform details to the project's website. Both the older linked guide and the newer setup URL returned HTTP 403 during this survey. Consequently, this report does not assert a complete current upstream host/guest compatibility matrix. The separately inspected Inferno-iOS README documents an iOS 14 use of this emulator, but does not prove equivalent behavior on every upstream host. [README](https://github.com/ChefKissInc/Inferno/blob/master/README.md), [linked upstream guide](https://chefkiss.dev/applehax/inferno/), [newer setup guide](https://chefkiss.dev/guides/inferno/), [port documentation](https://github.com/MakrSas/Inferno-iOS#readme)

License: the README describes the derivative as GPL-3.0, with ChefKiss's own code under AGPL-3.0 and additional per-file licensing qualifications. Preserve those distinctions when assessing reuse. [Licensing statement](https://github.com/ChefKissInc/Inferno/blob/master/README.md#legal-disclaimers)

### qemu-t8030

The original iPhone 11 emulator was archived on 14 April 2023. Its general QEMU README is less useful than its project wiki for understanding iOS-specific progress. The README describes QEMU as GPL version 2 overall, subject to its full license file. [Repository and README](https://github.com/TrungNguyen1909/qemu-t8030)

The status checklist marks userspace boot, launchd, console shell, iOS 15 support and iOS 14 restore complete. It leaves iOS 15+ restore, framebuffer, multitouch, SEP and GPU incomplete. These are the original project's milestones, not the status of later forks. [Status checklist, preserving checkbox values](https://raw.githubusercontent.com/wiki/TrungNguyen1909/qemu-t8030/Project-status.md)

The bring-up guide uses an iOS 14 beta IPSW, modified ramdisk and NVMe namespaces. It supplies Linux and macOS build instructions, while requiring macOS for some guest preparation. [Bring-up guide](https://github.com/TrungNguyen1909/qemu-t8030/wiki/Bringing-up-the-emulator)

### darwin-vm

This project wraps qemu-sptm to run a recovery-style Darwin environment. Its matrix reports iOS 26.6 and 27.0 booting on A14–A19 configurations; success means reaching a root shell and running commands. It also documents SSH/TCP over UART and debugging kernel/SPTM/TXM/userspace. Upstream preparation requires a Mac, while the prepared guest can run on a different QEMU host without an ARM CPU. [README and configuration matrix](https://github.com/jprx/darwin-vm#readme)

The README explicitly excludes screen, graphics, GUI apps, full SpringBoard, Wi-Fi and Bluetooth. It is a kernel/command-line research environment, not evidence of a usable graphical iPhone. [Limitations](https://github.com/jprx/darwin-vm#what-this-is-not)

License: MIT for the orchestration repository; that does not relicense its QEMU dependency or Apple firmware. Maintenance evidence here is the published configuration matrix and setup scripts, not a verified release cadence. [Repository](https://github.com/jprx/darwin-vm), [qemu-sptm dependency](https://github.com/jprx/qemu-sptm)

### xnu-qemu-arm64

Aleph Security's project documents running iOS 12.1 with launchd, interactive bash, secondary writable storage, arbitrary binaries, tunneled SSH, GDB helpers and KVM support. Its patched environment disables or bypasses several security mechanisms, so it should not be assumed to reproduce stock-device behavior. [README](https://github.com/alephsecurity/xnu-qemu-arm64/blob/master/README.md)

The author's 2019 boot writeups and 2020 KVM article explain the implementation. KVM acceleration is a distinct ARM-host configuration, not evidence that an x86 host can hardware-virtualize an ARM guest. [Initial boot research](https://alephsecurity.com/2019/06/17/xnu-qemu-arm64-1/), [KVM research](https://alephsecurity.com/2020/07/19/xnu-qemu-kvm/)

Maintenance: the README's “active development” wording is historical evidence only; current development was not established. License: a repository license link exists, but its contents could not be retrieved during this survey, so exact terms remain unverified. [Repository](https://github.com/alephsecurity/xnu-qemu-arm64)

### QEMU-iOS

The repository supports iPod touch 1G and 2G models. For 1G, the author's 2022 writeup demonstrates iBoot, XNU and SpringBoard, display rendering and multitouch, and states that the loaded bootloader/kernel binaries were unmodified. The article calls its guest “iPhoneOS 1.0, build 3A101a”; retain the exact build identifier when reproducing it rather than relying on that version label alone. [README](https://github.com/devos50/qemu-ios#readme), [author's demonstration and technical account](https://devos50.github.io/blog/2022/ipod-touch-qemu/)

The 1G setup was tested on macOS. The 2G instructions describe an experimental release, list macOS/Linux/Windows dependencies and warn that Linux compatibility is unstable. Boot ROM and prepared NOR/NAND images are required. [1G setup](https://devos50.github.io/blog/2022/ipod-touch-qemu-pt2/), [2G setup](https://github.com/devos50/qemu-ios/blob/ipod_touch_2g/RUNNING.md)

License: GPL-2.0 overall for the emulator, with component-specific terms. Maintenance: the inspected sources establish functionality and experimental limitations, not a current release cadence. [License](https://github.com/devos50/qemu-ios/blob/ipod_touch_2g/LICENSE)

### S5LBox

The README reports an iPhone OS 3.1.3 guest, app installation, snapshots, networking and experimental audio inside a signed iPhone app. It specifies iOS 13+ on a 64-bit iPhone and no host jailbreak/JIT setup. The earlier description of it as preparing for its first release was incorrect: the release page lists v1.1.0 on 19 October 2026. Release publication does not independently verify the guest capabilities. License: MIT, with an APSL notice for Apple Csu code in a guest helper. [README](https://github.com/j0shua-SYSON/S5LBox#readme), [releases](https://github.com/j0shua-SYSON/S5LBox/releases)

The architecture document needs careful reading: its opening diagram describes a synthetic app guest and missing I/O, while later sections report physical-device validation of guest pixels, slide-to-unlock and checkpoint restore. It also says full SecureROM/iBoot execution remains future work, with a synthesized handoff into XNU. These internal inconsistencies prevent treating every architecture statement as a synchronized feature matrix. Graphical operation is maintainer-reported; a complete original boot chain is not established. [Architecture and validation notes](https://github.com/j0shua-SYSON/S5LBox/blob/main/docs/ARCHITECTURE.md)

### Inferno-iOS

This unofficial SwiftUI app embeds Inferno as a library to emulate an iPhone 11 running iOS 14. Its README and screenshots report guest framebuffer/touch, buttons, IPA installation, bidirectional file transfer, a shell and networking without a companion VM. [README](https://github.com/MakrSas/Inferno-iOS#readme)

It requires JIT and a separately prepared jailbroken guest with bootstrap. Documented limitations include slow translated execution, a 2 GB guest configuration, encrypted App Store binaries failing to launch and experimental sound. Its memory/performance figures describe the author's setup, not universal iOS platform limits or independent benchmarks. [Requirements and limitations](https://github.com/MakrSas/Inferno-iOS#readme)

License: the repository identifies GPL-3.0; its embedded upstream has additional AGPL-3.0 code as described above. Maintenance: the public README, screenshots and releases document an evolving port, but broad host-device compatibility has not been independently checked. [Repository and license](https://github.com/MakrSas/Inferno-iOS), [releases](https://github.com/MakrSas/Inferno-iOS/releases)

### ipod-touch-1g boot-chain experiment

This separate QEMU attempt runs early iPod touch firmware and reports reaching an iBoot console sequence. Its README qualifies that result: QEMU hooks produce the `help` output from a genuine firmware string, general command dispatch is unfinished, kernel boot is not working, and execution halts after `help`. This is not evidence of booting the iOS operating system. [README](https://github.com/chrisspankroy/ipod-touch-1g#readme)

Setup uses extracted firmware and hardcoded absolute paths in the C device model. Host portability, a release cadence and the exact fork licensing were not verified; the repository lists multiple license files. Include it as an incomplete attempt, not as a substitute for the more advanced legacy projects. [Repository and source layout](https://github.com/chrisspankroy/ipod-touch-1g)

## Wrappers and supporting projects

| Project | Relationship and findings |
| --- | --- |
| [34306/vphone-aio](https://github.com/34306/vphone-aio) | Shell-based packaging of an already-jailbroken iOS 26.1 vphone, with VNC access. Its instructions require substantial storage and host security changes. The maintainer explicitly recommends vphone-cli for better updates/support. No license was verified. This is distribution/setup work, not an independent virtualization engine. [README](https://github.com/34306/vphone-aio#readme) |
| [thecybersandeep/darwin-vm-windows](https://github.com/thecybersandeep/darwin-vm-windows) | Shell wrapper and Linux preparation port around jprx/darwin-vm, claiming an iOS 27 iPhone 16 kernel/root shell on x86 Windows via WSL2. Its README reports Ubuntu 24.04 testing and replaces Mac filesystem/signing preparation with Linux tools. It explicitly provides no graphical iPhone. Wrapper scripts are MIT; upstream dependencies retain their own terms. No independent reproduction or maintenance cadence was established. [README](https://github.com/thecybersandeep/darwin-vm-windows#readme) |
| [OpenAVB](https://github.com/owngoal-dev/OpenAVB) / [OpenSEP](https://github.com/owngoal-dev/OpenSEP) | Supporting boot-ROM replacements, not complete VMs. vphone-cli says new VMs use them in place of AVPBooter/AVPSEPBooter and supplies pinned builds. [Integration documentation](https://github.com/Lakr233/vphone-cli#boot-roms) |

## Related approaches that do not establish an iOS VM

| Project | What it does | Why it is separate |
| --- | --- | --- |
| [touchHLE](https://github.com/touchHLE/touchHLE) | Rust high-level emulator for selected early iOS apps; reimplements frameworks. README targets iPhone OS 2/3 and iOS 4.0.x, with limited compatibility; never targets 64-bit iOS. MPL-2.0, with public releases. | Executes app code while replacing iOS functionality; does not boot Apple's kernel or emulate a whole iPhone. [README](https://github.com/touchHLE/touchHLE#readme), [releases](https://github.com/touchHLE/touchHLE/releases) |
| [ipasimulator/ipasim](https://github.com/ipasimulator/ipasim) | Windows app emulator, using application machine-code emulation and translation of iOS system functionality to Windows. C++ implementation; MIT. README limits working examples to simple apps. | Does not execute a complete iOS guest. Current development cadence was not established. [README and source tree](https://github.com/ipasimulator/ipasim) |
| [facebook/idb](https://github.com/facebook/idb) | Command-line automation for iOS simulators and devices. | Control tooling, not a replacement iOS VM or emulator engine. [Repository](https://github.com/facebook/idb) |
| [ios-control/ios-sim](https://github.com/ios-control/ios-sim) | Command-line launcher for Apple's iOS Simulator. | Depends on the simulator environment; does not establish iPhone firmware virtualization. [Repository](https://github.com/ios-control/ios-sim) |
| [utmapp/UTM](https://github.com/utmapp/UTM) | QEMU-based VM/emulator host for iOS and macOS, with additional macOS virtualization features. | “For iOS” describes a host platform. Its README does not establish support for booting iOS guests. [README](https://github.com/utmapp/UTM#readme) |
| [QEMU vmapple](https://github.com/qemu/qemu/blob/master/docs/system/arm/vmapple.rst) | Upstream Apple virtual-machine platform documentation describes macOS guests. | Relevant underlying technology, but the inspected documentation alone is not evidence of an iOS guest implementation. [Platform documentation](https://github.com/qemu/qemu/blob/master/docs/system/arm/vmapple.rst) |

## Evaluation priorities

These are judgments from the documented evidence, not benchmark results:

1. **Modern graphical iOS on an Apple Silicon Mac:** investigate vphone-cli first, using its exact compatibility matrix and current host setup instructions.
2. **Modern kernel and command-line research:** investigate darwin-vm; use the Windows wrapper as a separately validated preparation option if no Mac is available.
3. **Legacy full-system behavior:** study QEMU-iOS for an author-documented graphical boot and S5LBox for an independent portable implementation with ongoing documentation inconsistencies.
4. **iPhone 11 emulation and iOS-hosted guests:** trace qemu-t8030 into Inferno, then assess Inferno-iOS separately. A frontend's success does not imply complete hardware emulation or compatibility across hosts.

Keep guest firmware requirements, emulator licenses and host security prerequisites separate when comparing projects. No firmware, binaries, dependencies or host settings were installed or changed during this research.
