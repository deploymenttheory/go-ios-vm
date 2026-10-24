# Clang AST assessment of Xcode's iOS Simulator SDK

Research date: **24 October 2026**. This extends the [firmware-analysis research](firmware-analysis-research.md) and [virtualization reference map](virtualization-reference-map.md).

Accuracy review: the [adversarial audit](research-accuracy-audit.md) independently reran the six direct class/protocol counts, exact Metal selector difference, input hashes and object build-platform checks. The checked-in [verifier](research-artifacts/simulator-clang-ast/verify.py) and [rerun results](research-artifacts/simulator-clang-ast/verification.json) make those checks repeatable. Other original observations are identified separately from the rerun coverage.

## Assessment

**The Simulator SDK is useful for a target-aware API inventory and future userspace comparisons. Its AST does not describe an iPhone's virtual hardware.** This assessment includes actual compiler experiments against the installed Simulator and device SDKs, plus Apple/LLVM primary references. It does not include executing an app, booting a Simulator, or analyzing a retail firmware image.

The strongest local findings are:

- Both probe objects are ARM64, but Simulator and device objects carry different Mach-O platform IDs: **7 and 2** respectively.
- A module-enabled filtered AST dump omitted most class members. Textual header parsing recovered complete direct class declarations for the selected samples.
- The device SDK's `MTLDevice` protocol has **132 direct method declarations**, compared with **106** in the Simulator SDK: **26 device-only selectors** in this sample.
- Some selected declaration shapes match even though the targets and broader APIs differ. Equality of a small API sample is not runtime or hardware equivalence.

The [probe source](research-artifacts/simulator-clang-ast/probe.m) and [machine-readable observations](research-artifacts/simulator-clang-ast/observations.json) are stored in the repository. Full SDK AST dumps and object files were kept in temporary storage, not added to Git. Raw AST IDs and paths can change between invocations; dump hashes are provenance, not expected deterministic outputs.

## 1. What Simulator represents

Apple's WWDC19 architectural explanation describes Simulator as an isolated iOS userspace with its own launchd, daemons, application/framework processes and Mach bootstrap domain, using the **Mac kernel**. This is a useful architectural reference with an explicit 2019 date; it is not a complete specification of current CoreSimulator internals. It differs from the project's goal of running an actual iOS kernel with an appropriate virtual platform. [Apple: Getting the Most Out of Simulator](https://developer.apple.com/videos/play/wwdc2019/418/)

Apple explicitly distinguishes an arm64 iOS device binary from an arm64 Simulator binary. Shared instruction architecture is insufficient to make them interchangeable; separate variants belong in an XCFramework when distributing libraries for both. [Apple TN3117](https://developer.apple.com/documentation/technotes/tn3117-resolving-build-errors-for-apple-silicon), [XCFramework documentation](https://developer.apple.com/documentation/xcode/creating-a-multi-platform-binary-framework-bundle)

Clang's AST represents parsed declarations and source constructs for a translation unit. For SDK headers, it can expose interfaces and types without exposing Apple's proprietary implementations. Inference from this boundary: it cannot independently reveal MMIO behavior, interrupt ordering, boot-ROM handoffs, SEP implementation, DMA state or hardware timing. Those still require firmware/source inspection and runtime evidence. [LLVM AST introduction](https://clang.llvm.org/docs/IntroductionToTheClangAST.html)

## 2. Local environment and experiment design

| Item | Observed value |
| --- | --- |
| Xcode | `27.1`, build `27A9275` |
| Compiler | Apple clang `21.0.0`, build `clang-2100.3.34.2` |
| Developer directory | `/Applications/Xcode.app/Contents/Developer` |
| Simulator SDK | `iPhoneSimulator27.1.sdk` |
| Device SDK | `iPhoneOS27.1.sdk` |
| Simulator target | `arm64-apple-ios27.1-simulator` |
| Device target | `arm64-apple-ios27.1` |
| Language / common flags | Objective-C; ARC and blocks enabled |
| Executable/object scope | Compiled object files only; no linking or execution |

The same small source imports UIKit, Metal and CAMetalLayer and refers to a window's root view controller, a layer's device, and drawable size. It was parsed under both real target/sysroot combinations. No platform macro was manually overridden.

The pilot used eight module-enabled JSON AST invocations and eight textual-header JSON AST invocations. The latter covered `UIWindow`, `CAMetalLayer`, `MTLCreateSystemDefaultDevice` and `MTLDevice` on both targets. One additional text dump checked availability details; two preprocessing runs checked target macros, and two object compilations checked Mach-O metadata. All these compiler invocations completed successfully without diagnostics. This is a bounded sample, not a census of the SDK.

SDK metadata also declares distinct build-platform IDs and target environments. UIKit, Foundation, Metal and QuartzCore header/module directories were present in both inspected SDKs. The inspected UIKit/Metal/QuartzCore bundles contained `.tbd` linker stubs; these files are not the runtime implementations. Public Virtualization.framework and Hypervisor.framework header directories were absent at the corresponding SDK paths. That filesystem observation does not establish the absence of every private interface or binary elsewhere.

## 3. Observed AST and object results

### Platform identity survives a common CPU architecture

`otool -hv` identified both objects as ARM64 relocatable objects. `otool -l` reported:

| Field | Simulator object | Device object |
| --- | --- | --- |
| `LC_BUILD_VERSION.platform` | `7` | `2` |
| Minimum OS / SDK | `27.1` / `27.1` | `27.1` / `27.1` |
| `TARGET_OS_SIMULATOR` | `1` | `0` |
| `TARGET_OS_IOS` / `TARGET_OS_IPHONE` | `1` / `1` | `1` / `1` |
| `TARGET_OS_OSX` / `TARGET_OS_MACCATALYST` | `0` / `0` | `0` / `0` |
| `TARGET_OS_MAC` | `1` | `1` |

The last row is a practical warning for classifiers: the broad legacy macro is not enough to distinguish these environments. Preserve target triple, sysroot and Mach-O platform together. Apple's loader definitions name platform 2 `PLATFORM_IOS` and platform 7 `PLATFORM_IOSSIMULATOR`. [Apple Mach-O definitions](https://github.com/apple-oss-distributions/xnu/blob/main/EXTERNAL_HEADERS/mach-o/loader.h)

This experiment used **arm64**, not arm64e or arm64e.x1. It therefore does not assess pointer-authentication ABI behavior or compatibility with the CPU features required by a selected retail kernel.

### Imported modules can make a successful dump incomplete

With `-fmodules`, the filtered `CAMetalLayer` dump exposed interface references/attributes but **zero direct property or method declarations**. The `UIWindow` filter also produced many forward/interface references and a small number of declarations from related names. Successful parsing and nonempty JSON were insufficient evidence of a complete class inventory.

Repeating the sample with **`-fno-modules`** recovered definitions from the textually included headers:

| Declaration | Simulator direct properties / methods | Device direct properties / methods |
| --- | --- | --- |
| `UIWindow` main interface | `7` / `24` | `7` / `24` |
| `CAMetalLayer` main interface | `14` / `27` | `14` / `27` |
| `MTLDevice` protocol | `35` / `106` | `36` / `132` |

These are counts of direct AST children in the selected definition. Method counts include compiler-generated property accessors and exclude inherited members and separate categories. They are not counts of every callable API on an instance.

The name filter is a substring filter: `UIWindow` also matches names such as `UIWindowScene`. Its output contained **75 concatenated top-level JSON documents**, rather than one JSON array/document. A scanner must frame each document, select the exact symbol/kind, and distinguish definitions from forward references. It must not inflate counts by accumulating every matching root.

### Similar declaration shapes coexist with real API gaps

For `UIWindow`, `CAMetalLayer` and `MTLCreateSystemDefaultDevice`, a selected normalized shape matched across targets. The comparison retained class/superclass names, type strings, instance/class method classification, parameter/return types and property ownership. It excluded AST IDs, locations, availability payloads, record layout and runtime behavior. It therefore establishes only equality of those selected fields in this sample.

The sampled UIKit and CAMetalLayer headers had identical file hashes across SDKs; `MTLDevice.h` did not. Inspecting the actual `MTLDevice` protocol exposed **26 device-only selectors** and no Simulator-only selectors. Examples include `newMTL4CommandQueue`, `newCommandAllocator`, `newSharedTextureWithDescriptor:`, `newSharedTextureWithHandle:` and `supportsPlacementSparse`. The complete observed selector difference is in the [observations file](research-artifacts/simulator-clang-ast/observations.json).

This is a declaration-coverage result, not proof that all declared device methods work on every iPhone, or that no private equivalent exists in Simulator. It does demonstrate why a single shared function signature cannot justify treating whole frameworks as interchangeable.

### Ownership is visible; availability payloads need more work

The textual AST preserved `UIWindow.rootViewController` as a strong, nonatomic, readwrite property and `CAMetalLayer.device` as a retained, atomic, readwrite property. `MTLCreateSystemDefaultDevice`'s desugared type retained nullable-return and retained-result information. These are useful inputs to a binding generator's ownership policy.

However, the sampled JSON `AvailabilityAttr` nodes carried their kind and source ranges without explicit platform/version fields. A text dump of the Metal factory function showed macOS 10.11 and iOS 8.0 availability. A JSON-only extractor must not silently discard this information or invent a version from the presence of an attribute node. Use compiler-supported metadata access, source/preprocessor correlation or a separately validated extractor. Availability remains a declaration/deployment constraint, not a runtime feature probe. [Clang availability semantics](https://clang.llvm.org/docs/AttributeReference.html#availability)

## 4. Opportunities for this project

| Opportunity | Useful result | Required boundary |
| --- | --- | --- |
| Target-aware API inventory | A device/Simulator map of classes, protocols, selectors, types, ownership and availability | Normalize compiler artifacts, verify completeness and preserve source/target provenance |
| Go binding metadata | Inputs for guest-side probes or supported host interfaces; identify retained results and callback shapes | An Objective-C header scan does not capture every Swift-only API, actor requirement or private selector |
| Userspace comparison baseline | Compare selected API results, launch/service dependencies and rendering behavior with a real iOS VM | Requires an installed runtime and executed probes; no such comparisons were performed here |
| Graphics capability matrix | Separate common Metal entry points from device-only APIs and identify which features a test app requires | Do not equate API translation to the host GPU with emulating an iPhone GPU or its kernel drivers |
| Binary-analysis annotations | Correlate declarations with symbols/types during firmware analysis | A declaration or linker-stub entry does not prove an export exists in the selected firmware build |
| SDK change tracking | Review API and annotation changes between future SDK snapshots | SDK version, firmware version and runtime version must remain separate dimensions |

Apple currently documents hardware-accelerated Metal in Simulator, translating work to the host GPU. Older archived statements that Simulator Metal consists only of stubs are not an appropriate description of the current feature. Conversely, hardware acceleration does not make the selected virtual iPhone's GPU identical to the Mac GPU. [Apple: Developing Metal apps that run in Simulator](https://developer.apple.com/documentation/metal/developing-metal-apps-that-run-in-simulator)

The most plausible contribution to the full iOS VM project is an **API/ABI research sidecar and userspace comparison harness**. It can help identify framework requirements and build better probes. It does not replace Apple's research-platform boot path, private host adapters, guest drivers or the firmware-derived device contracts described in the other reports. No such harness or VM integration was implemented here.

## 5. Reproducing the static pilot

The following commands reproduce one textual AST and one object for each target using the checked-in probe. They write only to a newly created temporary directory. They do not enumerate or boot Simulator services.

```sh
probe=docs/research-artifacts/simulator-clang-ast/probe.m
out_dir=$(mktemp -d /private/tmp/go-ios-vm-simulator-repro.XXXXXX)
clang_bin=$(xcrun --find clang)

for platform in iphonesimulator iphoneos; do
    sdk_path=$(xcrun --sdk "$platform" --show-sdk-path)
    target=arm64-apple-ios27.1
    if [ "$platform" = iphonesimulator ]; then
        target=arm64-apple-ios27.1-simulator
    fi

    "$clang_bin" -target "$target" -isysroot "$sdk_path" \
        -fobjc-arc -fblocks -fno-modules -fsyntax-only "$probe" \
        -Xclang -ast-dump=json -Xclang -ast-dump-filter=MTLDevice \
        > "$out_dir/$platform-MTLDevice.json" \
        2> "$out_dir/$platform-MTLDevice.stderr"

    "$clang_bin" -target "$target" -isysroot "$sdk_path" \
        -fobjc-arc -fblocks -fno-modules -c "$probe" \
        -o "$out_dir/$platform-probe.o"
    otool -hv "$out_dir/$platform-probe.o"
    otool -l "$out_dir/$platform-probe.o"
done
```

Use the other filter names above to examine their declarations. Keep diagnostics with each output and check compiler exit status before parsing. The commands use deployment target 27.1 for this installed SDK experiment; that is not a recommendation to target every currently available retail iPhone with a 27.1 executable.

For sustained tooling, record full arguments in a compilation database and define a stable project-owned output schema. Clang's frontend AST-dump interface is not a promised stable interchange API. Strip ephemeral AST IDs from comparisons, keep target-relative source locations, and retain aliases/ownership/availability separately rather than flattening them into display strings. Evaluate libclang's relatively stable but intentionally incomplete C API or LibTooling when richer AST access is required. [Compilation database specification](https://clang.llvm.org/docs/JSONCompilationDatabase.html), [Clang FAQ](https://clang.llvm.org/docs/FAQ.html), [libclang](https://clang.llvm.org/docs/LibClang.html), [LibTooling](https://clang.llvm.org/docs/LibTooling.html)

Swift interfaces need a separate path: generated Objective-C-facing headers expose only the compatible surface. Pair AST work with Swift interfaces/symbol graphs and binary inspection when relevant; do not infer complete Swift or private CoreSimulator coverage from an Objective-C AST. [Apple: Importing Swift into Objective-C](https://developer.apple.com/documentation/swift/importing-swift-into-objective-c)

## 6. Runtime enumeration and session limits

The first `simctl list` attempt failed inside the sandbox with CoreSimulator service/log-access errors. That did not establish missing runtimes. A retry outside the sandbox completed and reported **an empty runtime list and an empty device map**. Consequently there was no installed runtime available through that enumeration for this session's dynamic comparison work.

The listing command unexpectedly emitted **“Install Started” / “Install Succeeded”**, then reported a CoreSimulator framework/job version mismatch (`1174.9.2` versus `1171.7`) and an attempted service refresh. This was an automatic side effect of enumeration; no explicit runtime download or install command was issued. Its exact installation payload was not audited, so this report does not claim that enumeration left all host state unchanged. No Simulator was created or booted.

No proprietary binary was reverse-engineered, no VM host setting was changed, and no firmware was downloaded. The completed evidence is SDK inspection, AST/preprocessor output and relocatable object metadata. Runtime API results, symbol resolution, GPU rendering, service tracing and full iOS VM compatibility remain untested.
