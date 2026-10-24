# Reference map for virtualized iOS on Apple Silicon

Research date: **24 October 2026**. Primary direction: modern iOS guests on Apple Silicon Macs, with emulation and physical-hardware research as supporting evidence.

The [adversarial accuracy audit](research-accuracy-audit.md) reviews the evidence boundaries and limitations of this source map. HTTP success establishes reachability, not factual correctness.

This guide complements the [project survey](research.md) and [firmware-analysis and Go-host research](firmware-analysis-research.md). It connects sources to implementation questions rather than repeating their project inventories. All references were reviewed during this research pass; publication dates and revision boundaries are noted where established. Moving documentation and branches are dated observations, not permanent compatibility guarantees.

The [Simulator Clang AST assessment](simulator-clang-ast-research.md) contains local compiler evidence and a checked-in probe for a complementary userspace/API research path.

## Reading order

1. **Apple's VRE documentation and implementation:** understand the existing research platform and its host configuration.
2. **OpenAVB/OpenSEP contracts:** identify concrete interfaces between the host, boot ROMs and firmware.
3. **Host lifecycle implementations:** study queue ownership, asynchronous completion and state reporting before choosing a Go adapter.
4. **Graphics and service dependencies:** separate early framebuffer output from an interactive graphical guest.
5. **Tracing and historical experiments:** turn inferred firmware behavior into observations and small implementation tests.

Evidence labels below mean **official documentation**, **implementation source**, **author experiment/report**, or **reverse-engineered notes**. None means that this project reproduced the result. No firmware, dependencies or guests were installed or executed for this guide.

## Reference index

### Apple research platform and boot contracts

| Reference and provenance | Evidence / version boundary | Practical use and limitation |
| --- | --- | --- |
| **1. Apple: [PCC source-code guide](https://security.apple.com/documentation/private-cloud-compute/sourcecode)** and [security-pcc repository](https://github.com/apple/security-pcc) | Official documentation and selected implementation source; reviewed 24 October 2026. | Establishes the provenance of published PCC components, including VRE tooling. Its limited-use terms and selected-component scope matter: this is not a complete public iPhone implementation or a blanket permission to reuse every component. |
| **2. Apple: [VRE architecture](https://security.apple.com/documentation/private-cloud-compute/virtualresearchenvironment)** and [setup](https://security.apple.com/documentation/private-cloud-compute/vresetup) | Official documentation for the PCC research environment. | Explains AP/SEP virtualization, custom firmware and cryptex support, host prerequisites and research-guest configuration. PCC is the reference platform; retail iOS compatibility must be established separately. |
| **3. Apple: [VRE VM configuration source](https://github.com/apple/security-pcc/blob/main/srd_tools/vre/vrevm/VM/VM+Config.swift)** | Implementation source; moving `main` inspected on the research date. | Shows research hardware-model construction, ROM override, SEP attachment, serial and debugger configuration. Includes private interfaces and notes that parts of configuration serialization are not stable. Use it to inventory host requirements, not to infer a supported public API contract. |
| **4. OwnGoal: [OpenAVB external contract](https://github.com/owngoal-dev/OpenAVB/blob/main/docs/external-contract.md)** and [README](https://github.com/owngoal-dev/OpenAVB#readme) | Implementation contract and maintainer-reported compatibility; moving `main`. | Documents the virtual memory map, platform registers, block/USB devices, host boot arguments, patch-table identity and boot-stage markers. These are useful candidate integration tests. The documented addresses belong to this virtual platform, not every physical iPhone SoC. |
| **5. OwnGoal: [OpenSEP external contract](https://github.com/owngoal-dev/OpenSEP/blob/main/docs/external-contract.md)** | Implementation contract; moving `main`. | Documents ROM/RAM placement, mailbox register semantics, protocol phases, image publication, event delivery and fatal-error reporting. The ROM explicitly waits for events; implementing a buffer without its wakeup behavior would miss part of the contract. This is a virtual SEP boot interface, not complete physical SEP equivalence. |
| **6. OwnGoal: [OpenSEP platform handoffs](https://github.com/owngoal-dev/OpenSEP/blob/main/docs/platforms.md)** | Source-linked reverse engineering and author boot reports; includes 18 October 2026 validation. | Compares `vresearch1` and `t8110` entry registers, memory aliases, translation tables and warm-boot/DART differences. It cites binary addresses and tests, illustrating the desired chain from firmware observation to a platform-specific implementation. Reported SpringBoard results do not validate every SEP service. |

These ROM contracts are particularly relevant to the proposed firmware-analysis approach: they already state interfaces that can be compared against code and traces. Their compatibility matrices distinguish tested, untested and unverified combinations; preserve those categories when selecting a baseline. OpenAVB also records third-party/APSL licensing qualifications, while OpenSEP describes its own provenance and embedded data. [OpenAVB provenance and license](https://github.com/owngoal-dev/OpenAVB#license), [OpenSEP compatibility and provenance](https://github.com/owngoal-dev/OpenSEP#readme)

**Latest-firmware constraint:** OpenSEP's current contract states that it has no GID key and instead includes publicly documented SEP decryption keys for selected iPhone14,4 builds. It also distinguishes research and production trust roots and its default handling of failed trust evaluation. This does not establish decryption or stock trust behavior for a newly selected retail build. Record the selected SEP image, key availability and modified trust policy explicitly; successful ROM startup is insufficient. [OpenSEP image contract](https://github.com/owngoal-dev/OpenSEP/blob/main/docs/external-contract.md#sep-firmware-image)

### Host orchestration and Go integration

| Reference and provenance | Evidence / version boundary | Practical use and limitation |
| --- | --- | --- |
| **7. Apple: [containerization VM helpers](https://github.com/apple/containerization/blob/72298def6ceadaa969303c620c2619af6eb8a51b/Sources/Containerization/VZVirtualMachine+Helpers.swift)** | Implementation source pinned to `72298def6ceadaa969303c620c2619af6eb8a51b`, committed 24 October 2026. | The start/stop/pause/resume helpers submit native work on the VM queue and bridge completion to an asynchronous continuation. This is a concrete lifecycle pattern to compare with blocking Go wrappers. The project implements Linux container VMs, not iOS guests. |
| **8. Code-Hex: [vz](https://github.com/Code-Hex/vz)** and [VM lifecycle implementation](https://github.com/Code-Hex/vz/blob/master/virtualization.go) | Go binding documentation and implementation; moving `master` reviewed on the research date. | Provides Linux/macOS VM examples, lifecycle/state interfaces and explicit OS/build-target availability handling. Its implementation states that VM operations use an internally managed serial queue. Evaluate its current bridge and requirements at a pinned revision; supported Linux/macOS guests do not establish an iOS boot path. |
| **9. crc-org: [vfkit](https://github.com/crc-org/vfkit)** | Implementation source and project documentation; moving `main`. | Offers a CLI using Apple's Virtualization framework and a Go configuration package that generates its command line. Useful for studying a process boundary between a Go controller and a VM host. It is not evidence that an ordinary iPhone IPSW can boot through its existing device configurations. |

These references provide alternatives and patterns to evaluate alongside [Deployment Theory's macOS bindings](firmware-analysis-research.md). They do not settle the dependency choice. A future host adapter should make queue ownership, cancellation versus native-operation completion, object lifetime and state notifications explicit. This is a proposed design criterion; no adapters were compared through runtime tests here.

### Graphics, input and service dependencies

| Reference and provenance | Evidence / version boundary | Practical use and limitation |
| --- | --- | --- |
| **10. Apple: [ParavirtualizedGraphics](https://developer.apple.com/documentation/paravirtualizedgraphics)** | Official framework documentation; reviewed on the research date. | Describes a guest-driver/host-framework relationship, virtual graphics-device advertisement, device descriptors, memory callbacks and display callbacks. Its stated guest scope is macOS. It supplies a reference interface for investigation, not a promise that retail iOS frameworks contain a compatible driver stack. |
| **11. Georges Gagnerot / eShard: [Emulating an iPhone in QEMU, Part I](https://www.eshard.com/blog/emulating-ios-14-with-qemu)** | Author technical account, **4 April 2025**; iOS 14 and QEMU 8.2.1 lineage. | Connects reverse engineering, logs and patches to graphics bring-up, SEP/keybag dependencies, software rendering and AMX-related paths. Useful as an investigation method and dependency inventory. It does not establish modern iOS compatibility or a faithful Apple GPU implementation. |
| **12. Axel Cohen / eShard: [Emulating an iPhone in QEMU, Part II](https://www.eshard.com/blog/emulating-ios-14-with-qemu-part2)** | Author technical account, **4 June 2025**; iOS 14. | Reports home-screen, touch, networking, pairing and non-encrypted app execution after adaptations. Explains why service initialization and rendering dependencies persist after early display output. Metal and missing-device limitations remain; do not promote the demonstrated result to unrestricted app compatibility. |

The practical lesson is to measure **framebuffer output**, **SpringBoard**, **input**, **software-rendered UI** and **Metal-dependent content** separately. A boot logo or restoration screen does not establish the later milestones. This evaluation structure is inferred from the documented differences between the experiments, rather than from a locally executed graphics test.

### Hardware observation and device-state research

| Reference and provenance | Evidence / version boundary | Practical use and limitation |
| --- | --- | --- |
| **13. Asahi Linux: [m1n1 hypervisor guide](https://asahilinux.org/docs/sw/m1n1-hypervisor/)** | Project documentation; reviewed on the research date. The guide pins macOS targets including 13.5 on M1/M2 and 14.8.3 on M1–M3. | Explains physical-machine instrumentation, symbol use and interactive tracing/debugging. Useful for learning how to observe Apple device behavior. This is a macOS/hardware research environment, not an iOS VM backend or a universal newest-Mac recipe. |
| **14. Asahi Linux: [Apple Interrupt Controller notes](https://asahilinux.org/docs/hw/soc/aic/)** | Reverse-engineered hardware notes; exact controller generation must be selected. | Documents masking, acknowledgment, targeting and reassertion behavior, with uncertainties retained. Provides a model for writing a stateful device contract instead of copying a register-address list. Do not apply one generation's offsets or semantics to every Apple SoC. |
| **15. Asahi Linux: [DCP tracer source](https://github.com/AsahiLinux/m1n1/blob/main/proxyclient/hv/trace_dcp.py)** | Instrumentation implementation; moving `main`. | Shows how display-coprocessor communication becomes structured trace events. Useful for designing protocol decoders and identifying shared-memory dependencies. It is not an iPhone display model; match firmware, SoC and tracer revision before interpreting output. |
| **16. Asahi Linux: [Linux 7.2 progress report](https://asahilinux.org/2026/08/progress-report-7-2/)** | Project author report, **August 2026**. | Discusses SPTM and GXF/SPRR-related work needed for newer-hardware observation. Read alongside the supported-target guide: progress on a tracing mechanism is different evidence from an established setup recipe. Changes to security architecture can affect the research environment itself. |

Asahi's material supports a methodology: observe registers and messages, identify state transitions and test the smallest corresponding model. It does not authorize assuming that similar-looking Mac and iPhone peripherals are identical. Before copying code, inspect the selected revision's license and provenance; an investigation reference is not automatically a dependency recommendation.

### Firmware inspection, recovery and restoration

| Reference and provenance | Evidence / version boundary | Practical use and limitation |
| --- | --- | --- |
| **17. blacktop: [go-macho](https://github.com/blacktop/go-macho)** | Go implementation for reading/creating Mach-O objects; moving repository reviewed on the research date. | A candidate building block for a Go-based artifact-inspection layer. Test the actual kernelcache/fileset and fixup shapes needed by the project. Parsing an executable does not implement a boot environment or prove that its decompiled behavior is correct. |
| **18. libimobiledevice: [idevicerestore](https://github.com/libimobiledevice/idevicerestore)** | Restore-tool implementation; moving `master`. | Reference for restore orchestration and interactions among firmware identities, recovery stages and device communication. A virtual device must expose the expected protocols to use comparable workflows. Restore code is not a signing bypass or a VM engine. |
| **19. libimobiledevice: [libirecovery](https://github.com/libimobiledevice/libirecovery)** | Library/tool implementation for iBoot/iBSS USB communication on macOS, Linux and Windows. | Useful when comparing DFU/recovery identities and host-visible protocol behavior with OpenAVB and a virtual USB path. API availability on a host does not establish that a VM provides a compatible USB transport. |
| **20. libimobiledevice: [libtatsu](https://github.com/libimobiledevice/libtatsu)** | Library implementation for Apple's Tatsu Signing Server communication. | Makes signing-service responsibilities concrete and separate from GDMF and asset indexes. Use it as protocol/implementation reference; successful metadata lookup or a third-party signing-status field does not establish that a particular restore request will be authorized. |

The existing [firmware report](firmware-analysis-research.md) covers GDMF, the Go Apple-services SDK and `ipsw` extraction. These additional sources fill different layers: binary structure, recovery transport, restore sequencing and signing communication. Preserve their separate error domains in any future acquisition/restore pipeline.

### Debugging, tracing and historical boot experiments

| Reference and provenance | Evidence / version boundary | Practical use and limitation |
| --- | --- | --- |
| **21. QEMU: [GDB usage](https://www.qemu.org/docs/master/system/gdb.html)** | Official upstream documentation; moving `master`. | Reference for stopped-start execution, memory/register inspection, breakpoints, relocation and multicore debugging. Support depends on the machine and accelerator. A system stub's ability to inspect memory is different from full userspace-debugger semantics. |
| **22. QEMU: [tracing](https://www.qemu.org/docs/master/devel/tracing.html)** | Official upstream trace-event documentation. | Describes memory-region read/write traces with addresses, values and access sizes, plus custom device events. Useful for specifying evidence to collect from an emulator. Such traces reflect that implementation; compare them with another trustworthy baseline before treating them as physical-hardware truth. |
| **23. QEMU: [record/replay](https://www.qemu.org/docs/master/system/replay.html)** | Official upstream deterministic-execution documentation. | Potential mechanism for repeatable investigations, replay and reverse debugging. Record/replay has machine, event and I/O configuration requirements. Its upstream existence does not prove compatibility with an iPhone fork or hardware-accelerated research guest. |
| **24. Zhuowei Zhang: [Almost booting an iOS kernel in QEMU](https://worthdoingbadly.com/xnuqemu/)** | Author experiment, **15 July 2018**; iOS 12 beta/iPhone X context. | Explains early kernel placement, boot arguments, device-tree handoff and console investigation. Useful for understanding the layers before userspace. It is a historical modified-machine experiment, not a modern iPhone boot recipe. |
| **25. Zhuowei Zhang: [XNU through launchd/userspace](https://worthdoingbadly.com/xnuqemu2/)** and [companion scripts](https://github.com/zhuowei/XNUQEMUScripts) | Author experiment/tutorial, **22 July 2018**; iOS 12 beta 4. | Shows why missing platform properties and drivers that validate register values matter; mapping empty memory was insufficient. It explicitly lacks graphics, internal storage and most peripherals. Reaching userspace with drivers excluded is not proof that their hardware contracts were implemented. |

## What the expanded evidence changes

- **Start from stated contracts where possible.** OpenAVB/OpenSEP document interfaces that would otherwise have to be inferred entirely from firmware. Verify each against the exact chosen virtual platform and component build.
- **Use implementation examples to test the Go host seam.** Apple's asynchronous helpers provide a reference for submitting work on the VM queue without blocking completion delivery. Compare this with the lifecycle risk identified in the earlier bindings research.
- **Treat graphics and services as separate dependency chains.** eShard's accounts show why early display success cannot stand in for an interactive, broadly compatible guest.
- **Keep the observation environment versioned.** Asahi's guide and progress report provide different levels of support evidence. A recent progress article is not sufficient to select hardware or firmware for a reproducible lab.

These are research conclusions and evaluation priorities. They do not select a VM backend or establish a locally working guest.

## Project concern map

| Future project concern | Read first | Evidence still needed from an experiment |
| --- | --- | --- |
| Virtual machine identity and initial boot | Apple VRE configuration; OpenAVB contract | Exact firmware identity, accepted host configuration, handoff state and early-console trace |
| SEP initialization and image handoff | OpenSEP contract/platforms; Apple VRE configuration | Version-matched mailbox sequence, memory aliases and observed service readiness |
| Go host lifecycle and UI integration | Apple containerization helpers; Code-Hex/vz; existing bindings report | Queue-correct start/stop, cancellation/late callbacks, object lifetime and state transitions |
| Graphics and input | Apple ParavirtualizedGraphics; eShard Parts I/II; DCP tracer methodology | Guest driver presence, framebuffer/display lifecycle, input response and renderer-specific behavior |
| Storage and restore | OpenAVB contract; idevicerestore; libirecovery; libtatsu | Recovery transport, selected build identity, storage readiness and restore outcome |
| Networking and guest communication | Code-Hex/vz/vfkit host patterns; eShard Part II | Which guest driver/service is active and whether the proposed host interface is supported |
| Interrupts and coprocessor state | Asahi AIC notes and tracing examples | Correct generation, mask/acknowledgment ordering, wakeup/completion and failure cases |
| Debugging and regression investigations | QEMU GDB/tracing/replay; virtual-platform debugger configuration | Actual fork/accelerator support, address/slide mapping and repeatable trace capture |

## Next evidence to collect

A useful first result would be a **single build-pinned early-console, storage or SEP interaction** traced on an exact known-working virtual guest. Record the artifact hashes, platform identity, code addresses, messages/register accesses, ordering, completion conditions and errors. Translate that observation into a small test before extending the machine model. This is proposed work, not a completed experiment.

For each implementation decision, distinguish:

1. What an official document promises.
2. What inspected source implements at a named revision.
3. What an author reports observing on a particular configuration.
4. What this project independently reproduces and tests.

No source in this guide alone closes all four levels. Keep moving-source observations dated, preserve licensing qualifications when considering reuse, and record missing or conflicting evidence rather than broadening a compatibility claim.

Link verification: 29 of the original 32 external URLs passed automated HTTP HEAD checks. The three Apple Security documentation pages returned HTTP 403 to that check; they remain canonical publisher links supported by the documentation content reviewed during research. A rejected automated request was not treated as proof that the page no longer exists.
