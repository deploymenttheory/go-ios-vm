# Implementation status — 2026-10-24

The design interview concluded with approval of item 20. This is a fresh implementation in `go-ios-vm`; the first inspection CLI is merged into `main`. No code, images or reports have been imported from the previous spike. The usable iOS VM remains the project objective; there is no runner or demonstrated guest boot in this repository yet.

## Demonstrated capabilities

- The Cobra/Viper CLI builds on macOS arm64 and Linux arm64. The exact iOS 14 and 15 qualifications used Go 1.27.1 on both hosts. Cgo is required by the reused LZFSE payload decoder. The historical-resolution PR passed CI; the streaming-transfer fix in [PR #22](https://github.com/deploymenttheory/go-ios-vm/pull/22) passed its initial CI checks.
- Synthetic IPSWs with two build identities exercise manifest selection, IM4P device-tree decoding, exact unsigned values, source-byte recovery, Mach-O prelinked driver metadata and dependency extraction. Distinct identities are not collapsed. Wrong versions, duplicate ZIP members, invalid structure and decompression output over budget are rejected.
- PostgreSQL-backed integration exercises atomic publication, rollback on invalid target ownership, deduplication across reruns, evidence-budget rejection, new CLI instances reading retained findings, and comparison after the synthetic source files have been deleted.
- A shared cache lease prevents pruning an active source. Cleanup only selects regular digest-named images in the configured cache; external originals and failed download staging are preserved or removed according to their ownership.
- Package-boundary checks prohibit transport/database dependencies in the model, adapter dependencies in the workflow, and cross-adapter/CLI imports in the adapters.
- A streaming PostgreSQL dump/restore check preserved supporting bytes, observations, findings and run relationships. The temporary restored database was removed. The test used synthetic findings and live catalog evidence, not real-image findings.

`go test -race ./...` with the dedicated PostgreSQL instance passed on macOS. `go test ./...` with PostgreSQL passed on Linux. `go vet ./...` passed on macOS. Synthetic tests isolate format and lifecycle behavior. The separate [integrated UI qualification](qualification/local-ui-integration.md) records real-image browser, cache and restore checks on macOS; neither result establishes a usable VM.

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
The exact iOS 15 archive has also been downloaded, verified, and inspected on
macOS and Linux. Its independent SHA-1/SHA-256/BuildManifest checks, selected
identity, run IDs, retained-byte checks, and partial coverage are in the
[iOS 15 qualification record](qualification/iphone12-1-ios15.md).

The first long iOS 14 transfer encountered the HTTP client's 30-minute total
timeout despite continuing progress. [PR #22](https://github.com/deploymenttheory/go-ios-vm/pull/22)
removes that total deadline from a progressing stream while retaining response
and idle timeouts. Its ETag-pinned partial resumed and completed; the cache
entry was published only after full checksum and local manifest verification.

## Real-image adapter limits

- The 5,574,812,591-byte iOS 14 restore archive completed the managed fetch and independent SHA-1/SHA-256/BuildManifest checks. Synthetic failure, cancellation, identity-mismatch, and lease tests still cover the refusal paths.
- The 6,959,737,743-byte iOS 15 restore archive also completed managed fetch and independent full-file checks on both hosts. Its SHA-256 is `13d574566eec7be3b9e23ec93c2374cabf7f150be63aafd7f8479d7d162480e0`. Earlier pre-codec macOS/Linux runs each retained 4,612 matching observations. The current upstream-main UI trial retained 5,369, with Manifest, DeviceTree, KernelCache and disk-metadata coverage `complete` and selected filesystem content `partial`.
- The device-tree adapter produced 3,520 direct findings on the selected real image. A follow-up bounded IOKit `IDREF` decoder recovered 938 driver findings across 246 bundles on macOS and Linux, with matching observation IDs and KernelCache coverage `complete`; see the [qualification follow-up](qualification/iphone12-1-ios14.md). IM4P LZFSE payload decoding now runs in a killable, bounded worker.
- Disk/APFS inspection now pins unreleased `go-apfs-v3` main `7566f1030c62` through a Go pseudo-version, including the merged LZFSE UDIF and decmpfs type-4 fixes. Stored OS ZIP members are read directly; Deflate members use bounded disposable staging and report partial coverage when their encoded evidence exceeds the 32 MiB capture limit. The exact iOS 14 and 15 images now reach complete disk metadata and selected APFS firmware/driver paths, with partial filesystem content because of bounded search and per-file capture limits. Logical APFS volume/view/object selectors are retained; physical extents for decoded content are not claimed. The [UI qualification](qualification/local-ui-integration.md) verified retrieval after source-cache deletion and database restore. A tagged upstream codec release has not yet been consumed.

## Remaining contract gaps

The first-tooling release is not complete:

- The exact iOS 14 and 15 images have been qualified with partial coverage on matching `iPhone12,1` / `n104ap` identities. Their retained macOS runs support exact subject/property comparison (3,852 equal, 604 changed, 230 unexamined entries); this does not imply semantic or boot compatibility. iOS 26/27 and cross-generation comparisons remain separate work.
- Filesystem-wide content inspection, Mach-O fileset drivers and a bounded LZSS adapter remain unimplemented. The bounded APFS search reports partial coverage for paths outside its declared scope and for any source, codec or content limits. ZIP-compressed disk staging has fixed 8 GiB expanded and 2 GiB free-space reserve limits; it has synthetic-fixture coverage but no real compressed OS-member qualification.
- Findings have a stable exact target/subject/property identity and retain linked observed, inferred, and unknown revisions with rationale and typed supporting or contradicting references. Direct observed values come from published observations; conflicting parser readings coexist. Append-only assessments attach accepted, rejected, or disputed decisions to an exact revision and basis with reviewer reason; conflicting readings, explicit contradictions, and changed support mark earlier decisions `needs_review` while preserving history. Comparison reports multiple direct readings as conflicts; it does not adjudicate them.
- Inspection publishes at most 128 observations per transaction and stores completed component checkpoints. Explicit `inspect --resume RUN_ID` rehashes the full archive and requires the same target, size, extractor and component plan; interrupted runs have no final coverage ledger. `run recover` discovers expired two-minute claims and marks them `interrupted` while preserving input identity and completed batches. Finalization retries return the committed terminal result. Graceful cancellation records a distinct terminal state when PostgreSQL remains reachable.
- Acquisition attempts now register durable progress before catalog resolution. Their downloaded and hashed-byte counters are coalesced, and phase history is capped at 64 events per operation. Inspection progress shares the exact run identity; published batch and checkpoint counters commit with evidence. A second process can read the latest committed state during work or after a crash. A 512 MiB streaming trial produced 16 progress writes across 16,384 buffered writes. The read-only local browser now shows bounded progress, findings and evidence, and source-backed relationships. The [integrated QA record](qualification/local-ui-integration.md) includes live running phases and truthful partial/failure states.
- The retained-byte quota bounds distinct supporting objects, not PostgreSQL table/index/WAL overhead or the total size of an externally managed database. Capture and read limits are bounded. IM4P LZFSE decoding runs in a killable child with a deadline, output cap and worker memory threshold; there is still no hard whole-process RSS/CPU budget. The initial command processes one target at a time.
- The [pinned-image resource budget](resource-budgets.md) records both archives' sizes, observed peak resident memory and representative retained bytes. It also records post-codec iOS 14 and 15 APFS trials on macOS and Linux against a local upstream codec composite. `resources status` shows live cache/staging/evidence limits, fetch and Deflate staging print preflight estimates, and the cache lease permits one inspection per cache directory. Upstream main now carries the codec fixes and the integrated UI has been qualified on macOS. [Resource gate #52](https://github.com/deploymenttheory/go-ios-vm/issues/52) and [first-tooling gate #20](https://github.com/deploymenttheory/go-ios-vm/issues/20) remain open for their independent acceptance review.
- Successful, partial, invocation-failure, fatal-failure, and cancellation results use one versioned JSON envelope on stdout. Diagnostic codes classify missing input, invalid selection, unsupported or corrupt input, storage and acquisition failures, and cancellation. Terminal text shows concise command summaries, while progress remains on stderr.
- Exact subject/property matching is implemented. Cross-hardware equivalence and a reviewed machine-profile export are not. The profile and complete machine contracts must precede runner implementation.

## Next work, in order

1. Review the pinned upstream-main APFS qualification and [resource gate #52](https://github.com/deploymenttheory/go-ios-vm/issues/52), then adopt a tagged upstream version when published. Keep bounded partial coverage explicit.
2. Complete [first-tooling acceptance #20](https://github.com/deploymenttheory/go-ios-vm/issues/20): verify its independent comparison and resource checks against the exact pair and link the [completed UI qualification](qualification/local-ui-integration.md).
3. Design the bounded whole-machine profile and complete lifecycle contracts, then implement the Hypervisor.framework runner. Static observations alone do not choose runtime behavior.

This work does not create a general campaign harness, retain disposable report trees or alter the wider Weave products.
