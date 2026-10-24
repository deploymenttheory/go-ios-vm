# Adversarial accuracy audit of the iOS VM research

Audit date: **24 October 2026**.

## Verdict

**The research supports a plausible investigation, but does not yet establish a working implementation or compatibility with the latest retail firmware.** The strongest local evidence is the repeatable SDK/compiler experiment. Most VM capability claims remain attributed upstream reports. Two factual statements needed correction, and several evidence gaps prevent stronger conclusions.

All four reports were read: [project survey](research.md), [firmware/Ghidra/GDMF/Go-host analysis](firmware-analysis-research.md), [reference map](virtualization-reference-map.md), and [Simulator assessment](simulator-clang-ast-research.md). The saved probe and observation JSON were also inspected. An independent source review covers the project and firmware claims in [the supporting verification report](research-verification-projects.md). This audit prioritizes claims that affect backend selection, firmware selection and implementation feasibility; it is not a certification of every linked release, license or line of source.

“Confirmed” below means confirmed within the stated scope: source inspection confirms what a project documents or implements, not that its guest works on this machine. “Reproduced” is reserved for the local compiler checks. An unverified historical observation is not automatically a false observation.

## Findings requiring correction or qualification

### 1. High impact: newest firmware availability does not establish a usable VM target

The argument to challenge is: *we can retrieve the latest firmware, inspect it, and therefore implement the necessary virtual machine*. The first two steps can produce useful evidence; the conclusion is not established. A retail image can lack the virtual drivers required by the chosen research platform. Recovering driver accesses also leaves reset state, ordering, concurrency, DMA and coprocessor behavior to validate. The original firmware report generally preserves these distinctions, but its proposed workflow must not be read as completed recovery of a machine specification.

OpenSEP adds a concrete acquisition constraint: its current contract describes selected publicly available decryption keys, rather than possession of the hardware GID key, and a trust policy that differs from production verification. It is not evidence that an arbitrary newest retail SEP image can be decrypted and run. The reference map now makes this constraint explicit. [OpenSEP image contract](https://github.com/owngoal-dev/OpenSEP/blob/main/docs/external-contract.md#sep-firmware-image)

**Decision:** retain the newest applicable firmware as an analysis target. Establish one exact working virtual guest separately before treating it as an implementation baseline. A reproducible boot with component hashes and patch inventory would strengthen the claim; an asset URL or decompilation alone would not.

### 2. High impact: boot progress is weak evidence of hardware fidelity

The modern virtualization and QEMU/emulation paths are different implementation strategies. A patched recovery shell, an interactive graphical guest and faithful physical-device behavior answer different questions. Existing implementations make the overall idea credible, but do not show that a fresh Go host with generated public bindings can reproduce their behavior. The independent review confirms upstream descriptions, not a local boot. [Project and firmware verification](research-verification-projects.md)

The historical boot work itself illustrates the danger: it bypasses or excludes portions of the original environment to reach kernel/userspace milestones. eShard's later account also documents adaptations before a usable UI. These are useful engineering results, with narrower scope than stock-device equivalence. [2018 userspace experiment](https://worthdoingbadly.com/xnuqemu2/), [eShard Part II](https://www.eshard.com/blog/emulating-ios-14-with-qemu-part2)

**Decision:** the project has not selected and validated a backend. Require a versioned baseline and separate boot, storage, SEP, GUI, input and networking outcomes. A patch that suppresses a failure must remain distinguishable from implementing the missing behavior.

### 3. Medium impact: GhidrAssistMCP compatibility was overstated

The firmware report said 11.4+ support tested with 12.1. The current prerequisites specify **Ghidra 12.0 or 12.1 Public with Java 25+** and version-specific extension packages. The original row is corrected. Those requirements do not establish compatibility with the report's latest Ghidra 12.2 candidate. No MCP extension has been installed or exercised here. [Current prerequisites](https://github.com/symgraph/GhidrAssistMCP#prerequisites)

### 4. Medium impact: S5LBox's release status was wrong

The survey said it was preparing for its first release. The release page lists **1.0.0 on 14 October and 1.1.0 on 19 October 2026**. The survey is corrected. Its architecture-document inconsistencies and the absence of independent local guest validation remain relevant; a downloadable release does not prove every advertised capability. [Release history](https://github.com/j0shua-SYSON/S5LBox/releases)

### 5. Medium impact: historical metadata cannot be fully audited from a digest

The firmware report retained a PMV digest and selected fields, but not the original response bytes. The independent fresh read agrees on the consequential release rows and response size, while producing a different digest. This does not falsify the earlier hash: JSON order, serialization or changed fields could alter it, and the cause was not established. It does mean the original snapshot cannot be reconstructed or independently authenticated from the notes. Keep it labeled as a historical recorded observation. [Fresh metadata evidence and limitations](research-verification-projects.md)

Likewise, agreement between an index and CDN HEAD metadata does not validate downloaded component bytes, BuildManifest identity, restore personalization or boot. The report already says this; the audit preserves that boundary. GDMF release applicability, asset location and signing authorization remain separate evidence layers.

### 6. Medium impact: reference breadth is stronger than implementation evidence

Many citations are moving branches. Dates in prose do not pin the inspected file content. A successful HTTP check establishes reachability; it cannot validate claims or establish compatibility. The few pinned Go and Apple source revisions are a better pattern, but no complete archive of all reviewed sources exists.

Apple's pinned containerization helper does support the proposed queue pattern: it dispatches the native asynchronous request and returns from the queue closure while a continuation awaits completion. That source does not prove the proposed Go adapter is correct. The source-level deadlock/cancellation concerns remain hypotheses requiring native runtime tests. [Pinned Apple helper](https://github.com/apple/containerization/blob/72298def6ceadaa969303c620c2619af6eb8a51b/Sources/Containerization/VZVirtualMachine%2BHelpers.swift), [independent Go wrapper checks](research-verification-projects.md)

## Local Clang findings: challenged and reproduced

The [checked-in verifier](research-artifacts/simulator-clang-ast/verify.py) independently regenerated the core textual AST and object results. Its [saved output](research-artifacts/simulator-clang-ast/verification.json) records the observed toolchain and explicit coverage. It invokes no Simulator services and performs no downloads.

| Claim | Adversarial check | Result |
| --- | --- | --- |
| Report used the installed SDK inputs | Recomputed probe, SDKSettings and selected header digests for both SDKs | All matched the recorded evidence |
| Counts were actual direct definitions rather than name-filter noise | Framed concatenated JSON; selected exact name/kind and largest direct definition | Counts reproduced for all six class/protocol definitions |
| `UIWindow` / `CAMetalLayer` have the reported direct members | Compared direct property/method counts | `7/24` and `14/27` on both targets |
| `MTLDevice` differs across targets | Recomputed both counts and exact selector sets | Simulator `35/106`; device `36/132`; the exact 26 device-only selectors reproduced |
| ARM64 objects still distinguish build platforms | Recompiled both targets; inspected `LC_BUILD_VERSION` | Simulator `7`, device `2`; both minimum/SDK `27.1` |
| A valid JSON document is enough to parse a filtered dump | Parsed the whole output as a document stream | `UIWindow` has 75 roots; `MTLDevice` has 28 Simulator roots and 30 device roots |

These checks strengthen confidence in the reported static results. They do **not** establish runtime export availability, ABI interchangeability, graphics behavior or firmware compatibility. Direct method counts include implicit property accessors; the 26-selector difference is not a count of independently tested features.

Apple's Metal documentation independently supports the key limitation: Simulator translates work to the Mac GPU and can expose different features and limits from the selected phone. Apple's WWDC19 explanation supports the historical Mac-kernel/userspace distinction. Neither is a current complete CoreSimulator implementation specification. [Apple Metal documentation](https://developer.apple.com/documentation/metal/developing-metal-apps-that-run-in-simulator), [WWDC19 architecture](https://developer.apple.com/videos/play/wwdc2019/418/)

The rerun deliberately did not repeat the module-enabled omission, selected-shape normalization, availability dump, target-macro inspection or runtime enumeration. Those remain original-session observations, rather than independently rerun findings. The selected-shape comparison is also not a full annotation/ABI comparison; desugared type strings must not be treated as lossless preservation of every source annotation.

To reproduce the covered checks from the repository root:

```sh
python3 docs/research-artifacts/simulator-clang-ast/verify.py
```

The verifier fails on mismatches and requires the recorded SDK inputs to still be installed. Changed SDKs should produce a new, separately versioned observation rather than silently replacing the baseline. It does not validate its own parsing strategy against every possible Clang AST shape.

## Which conclusions survive the stress test?

| Conclusion | Audit judgment |
| --- | --- |
| Existing projects provide plausible routes to executing iOS code and guests | Supported by primary project reports; no local guest reproduction |
| Ghidra/MCP and firmware inspection can help infer software-visible contracts | Reasonable method; no such contract recovered and validated in this repository yet |
| GDMF provides a complete firmware download/signing/virtualization solution | Rejected; the original report correctly separates these services |
| Public macOS bindings alone implement the current iOS virtualization path | Rejected; private interface and lifecycle gaps remain |
| Simulator headers are useful API/binding research inputs | Supported, including reproduced target-specific differences |
| Simulator headers reveal the hardware model a retail iOS kernel requires | Unsupported; headers describe the public source interface, not the device implementation |
| The latest retail image will boot once analyzed | Unsupported; no chosen latest-image guest has been booted here |
| The source map is a sufficient implementation specification | Unsupported; it is a research guide with explicit unknowns |

## Evidence needed before an implementation commitment

These are recommended experiments, not work completed by this audit:

1. **Reproduce a baseline guest:** freeze host/tool/backend revisions, exact original and modified firmware components, patches, hashes and serial output. Record which milestones are actually reached.
2. **Validate the Go host independently:** use a supported guest to test queue submission, callback completion, cancellation, native object lifetime and state transitions. Do not attribute bridge failures to iOS firmware.
3. **Validate one guest contract:** tie a static driver path to a runtime trace, then test reset, completion and error behavior. Compare emulator-generated traces with an independent baseline to avoid circular validation.
4. **Challenge forward compatibility:** compare that contract and its required drivers/security components with the newest applicable retail build. Missing keys, drivers or a valid handoff are real blockers to resolve, rather than evidence that more AST extraction will solve them.

The original Simulator enumeration's automatic install/service-refresh messages remain disclosed in its report. This audit performed static compilation and read-only source/metadata checks; it did not boot a guest, install analysis tools, or change host security settings. No conclusion here should be read as independent validation of stock iPhone security or timing behavior.
