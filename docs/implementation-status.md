# Implementation status — 2026-10-22

The design interview concluded with approval of item 20. This is a fresh implementation in `go-ios-vm`; the first inspection CLI is merged into `main`. No code, images or reports have been imported from the previous spike. The usable iOS VM remains the project objective; there is no runner or demonstrated guest boot in this repository yet.

## Demonstrated capabilities

- The Cobra/Viper CLI builds on macOS arm64 and Linux arm64. The exact iOS 14 qualification used Go 1.27.1 on both hosts. Cgo is required by the reused LZFSE payload decoder. The historical-resolution PR passed CI; the streaming-transfer fix in [PR #22](https://github.com/deploymenttheory/go-ios-vm/pull/22) passed its initial CI checks.
- Synthetic IPSWs with two build identities exercise manifest selection, IM4P device-tree decoding, exact unsigned values, source-byte recovery, Mach-O prelinked driver metadata and dependency extraction. Distinct identities are not collapsed. Wrong versions, duplicate ZIP members, invalid structure and decompression output over budget are rejected.
- PostgreSQL-backed integration exercises atomic publication, rollback on invalid target ownership, deduplication across reruns, evidence-budget rejection, new CLI instances reading retained findings, and comparison after the synthetic source files have been deleted.
- A shared cache lease prevents pruning an active source. Cleanup only selects regular digest-named images in the configured cache; external originals and failed download staging are preserved or removed according to their ownership.
- Package-boundary checks prohibit transport/database dependencies in the model, adapter dependencies in the workflow, and cross-adapter/CLI imports in the adapters.
- A streaming PostgreSQL dump/restore check preserved supporting bytes, observations, findings and run relationships. The temporary restored database was removed. The test used synthetic findings and live catalog evidence, not real-image findings.

`go test -race ./...` with the dedicated PostgreSQL instance passed on macOS. `go test ./...` with PostgreSQL passed on Linux. `go vet ./...` passed on macOS. Tests use synthetic data for format and lifecycle isolation; these results do not establish support for real iOS firmware or a usable VM.

## Live acquisition evidence

Current [Apple GDMF release metadata](https://gdmf.apple.com/v2/pmv) and
[Apple's iTunes restore catalog](https://itunes.apple.com/WebObjects/MZStore.woa/wa/com.apple.jingle.appserver.client.MZITunesClientCheck/version/)
do not yield the exact `iPhone12,1` iOS 14.0/15.0 public restore pair. A bounded
historical-index fallback now supplies Apple-hosted URL locators. When run with
PostgreSQL, the CLI retains the response with source attribution; an Apple CDN
byte-range probe checks actual ZIP BuildManifest identity before any full download.

The CLI can now list and search restore IPSW locators for a product before an
exact fetch. A live `firmware list --product iPhone12,1 --search 14.0` check on
2026-10-22 returned two restore rows, including public build `18A373`, with
all three sources responding. These are unverified catalog locators; the list
also reports source failures and ambiguous exact selections explicitly.

Both public builds were probed successfully: 14.0/`18A373` and
15.0/`19A346`. The exact URLs, declared checksums, measured sizes, manifest
digests, boards, identities and source limitations are in the
[public iPhone 11 pair record](qualification/historical-resolution.md). The
old 14.0 seed build was not substituted for the public one. The full iOS 14
archive has since been downloaded, verified, and inspected on macOS and Linux;
its SHA-256, selected identity, run IDs, retained-byte checks, resource use, and
partial coverage are in the [iOS 14 qualification record](qualification/iphone12-1-ios14.md).
The iOS 15 archive remains a bounded remote-manifest observation only.

The first long iOS 14 transfer encountered the HTTP client's 30-minute total
timeout despite continuing progress. [PR #22](https://github.com/deploymenttheory/go-ios-vm/pull/22)
removes that total deadline from a progressing stream while retaining response
and idle timeouts. Its ETag-pinned partial resumed and completed; the cache
entry was published only after full checksum and local manifest verification.

## Real-image adapter limits

- The 5,574,812,591-byte iOS 14 restore archive completed the managed fetch and independent SHA-1/SHA-256/BuildManifest checks. Synthetic failure, cancellation, identity-mismatch, and lease tests still cover the refusal paths.
- The device-tree adapter produced 3,520 direct findings on the selected real image. A follow-up bounded IOKit `IDREF` decoder recovered 938 driver findings across 246 bundles on macOS and Linux, with matching observation IDs and KernelCache coverage `complete`; see the [qualification follow-up](qualification/iphone12-1-ios14.md). LZFSE payload decompression uses upstream `DecodeBufferInto` with a fixed output cap.
- Disk/APFS metadata uses `go-apfs-v3 v0.1.0` through a borrowed read-only ZIP member view. Stored OS members are read directly. Deflate OS members now use bounded disposable staging, with the full encoded member retained as evidence when it fits the existing 32 MiB disk source-capture cap. Larger compressed members report partial disk coverage without findings. Local iOS 14 inspection reaches unsupported LZFSE UDIF chunk kind `0x80000007` before any disk finding is emitted; [go-apfs-v3 #146](https://github.com/deploymenttheory/go-apfs-v3/issues/146) owns that decoder. Filesystem decompression is a separate layer, and physical extent provenance for decoded APFS file contents is not claimed.

## Remaining contract gaps

The first-tooling release is not complete:

- The exact iOS 14 image has been qualified with partial coverage. The exact iOS 15 image has not been acquired or inspected in full, so the matching-hardware pair has not been compared. iOS 26/27 and cross-generation comparisons remain separate work.
- Filesystem-content inspection, Mach-O fileset drivers and a bounded LZSS adapter remain unimplemented. Coverage reports these gaps; an inspection currently ends `partial` because filesystem-content inspection is unexamined. ZIP-compressed disk staging has fixed 8 GiB expanded and 2 GiB free-space reserve limits; it has synthetic-fixture coverage but no real compressed OS-member qualification.
- Findings have a stable exact target/subject/property identity and retain linked observed, inferred, and unknown revisions with rationale and typed supporting or contradicting references. Direct observed values come from published observations; conflicting parser readings coexist. Assessment and dependency invalidation are still outstanding. Comparison reports multiple direct readings as conflicts; it does not adjudicate them.
- Inspection publishes at most 128 observations per transaction and stores completed component checkpoints. Explicit `inspect --resume RUN_ID` rehashes the full archive and requires the same target, size, extractor and component plan; interrupted runs have no final coverage ledger. Crash recovery for abandoned `running` runs and idempotent finalization after an ambiguous commit response remain outstanding in issue #17. Graceful cancellation records a terminal state when PostgreSQL remains reachable.
- The retained-byte quota bounds distinct supporting objects, not PostgreSQL table/index/WAL overhead or the total size of an externally managed database. Capture and read limits are bounded, but a full process memory/CPU budget and cancellation during native decompression are not provided. The initial command processes one target at a time.
- Successful, partial, invocation-failure, fatal-failure, and cancellation results use one versioned JSON envelope on stdout. Diagnostic codes classify missing input, invalid selection, unsupported or corrupt input, storage and acquisition failures, and cancellation. Terminal text shows concise command summaries, while progress remains on stderr.
- Exact subject/property matching is implemented. Cross-hardware equivalence and a reviewed machine-profile export are not. The profile and complete machine contracts must precede runner implementation.

## Next work, in order

1. Acquire and verify the exact iOS 15 candidate in full within the current free-disk budget. Its bounded manifest probe is not a full archive pin.
2. Advance the iOS 14 disk adapter through [go-apfs-v3 #146](https://github.com/deploymenttheory/go-apfs-v3/issues/146), then check for further format gaps. [Issue #13](https://github.com/deploymenttheory/go-ios-vm/issues/13) owns filesystem content. Requalify kernel metadata on the iOS 15 image when acquired.
3. Complete the remaining finding/publication contracts and compare the matching-hardware pair after cache cleanup and database restore. This is the first-tooling acceptance gate.
4. Design the bounded whole-machine profile and complete lifecycle contracts, then implement the Hypervisor.framework runner. Static observations alone do not choose runtime behavior.

This work does not create a general campaign harness, retain disposable report trees or alter the wider Weave products.
