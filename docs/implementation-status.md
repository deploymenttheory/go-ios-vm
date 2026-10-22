# Implementation status — 2026-10-22

The design interview concluded with approval of item 20. This is a fresh implementation in `go-ios-vm`, on `feat/firmware-inspection`. No code, images or reports have been imported from the previous spike. The usable iOS VM remains the project objective; there is no runner or demonstrated guest boot in this repository yet.

## Demonstrated capabilities

- The Cobra/Viper CLI builds on macOS arm64 and Linux arm64. Local verification used Go 1.27.1 on macOS and Go 1.27.2 in an already-cached Linux container. Cgo is required by the reused LZFSE decoder. GitHub CI is defined but has not been run for this unpublished branch.
- Synthetic IPSWs with two build identities exercise manifest selection, IM4P device-tree decoding, exact unsigned values, source-byte recovery, Mach-O prelinked driver metadata and dependency extraction. Distinct identities are not collapsed. Wrong versions, duplicate ZIP members, invalid structure and decompression output over budget are rejected.
- PostgreSQL-backed integration exercises atomic publication, rollback on invalid target ownership, deduplication across reruns, evidence-budget rejection, new CLI instances reading retained findings, and comparison after the synthetic source files have been deleted.
- A shared cache lease prevents pruning an active source. Cleanup only selects regular digest-named images in the configured cache; external originals and failed download staging are preserved or removed according to their ownership.
- Package-boundary checks prohibit transport/database dependencies in the model, adapter dependencies in the workflow, and cross-adapter/CLI imports in the adapters.
- A streaming PostgreSQL dump/restore check preserved supporting bytes, observations, findings and run relationships. The temporary restored database was removed. The test used synthetic findings and live catalog evidence, not real-image findings.

`go test -race ./...` with the dedicated PostgreSQL instance passed on macOS. `go test ./...` with PostgreSQL passed on Linux. `go vet ./...` passed on macOS. Tests use synthetic data for format and lifecycle isolation; these results do not establish support for real iOS firmware or a usable VM.

## Live acquisition evidence

Successful HTTPS requests were made to:

- [GDMF release metadata](https://gdmf.apple.com/v2/pmv).
- [Apple's iTunes restore catalog](https://itunes.apple.com/WebObjects/MZStore.woa/wa/com.apple.jingle.appserver.client.MZITunesClientCheck/version/).

For `iPhone12,1`, neither catalog adapter returned an exact `14.0` or `15.0` match. The broad device query returned current GDMF release metadata and an iTunes restore candidate labelled `27.0.1 / 24A446`. That candidate has not been downloaded or verified through BuildManifest and is not substituted for the agreed initial comparison pair.

The six catalog-query records reference four distinct response blobs (6,545,498 logical source bytes at the end of the initial pass). Responses changed between requests; they are retained as separate captured versions. The PostgreSQL database was approximately 9.3 MiB after removing temporary synthetic test records. There are no real IPSW downloads or decoded image archives in this project. Cache cleanup does not delete database evidence.

No match in a current feed is not proof that a historical Apple CDN image is unavailable. Historical restore URL resolution is the next acquisition gap to resolve explicitly, preserving its source and validating the downloaded manifest before publication.

## Implemented but not yet qualified on real images

- Downloading a catalog-resolved IPSW, validating declared length/SHA-1 where supplied, computing SHA-256 and checking BuildManifest product/version/build before publishing the managed cache file. Failure, cancellation, identity mismatch and lease behavior have synthetic tests; a real multi-gigabyte acquisition has not run.
- Device-tree and prelinked-kernel adapters reuse pinned `blacktop/ipsw` packages. LZFSE uses upstream `DecodeBufferInto` with a fixed maximum destination. This is firmware payload decompression; no new filesystem codec was written.
- Disk/APFS metadata uses `go-apfs-v3 v0.1.0` through a borrowed read-only ZIP member view. Its evidence captures the encoded-disk read ranges used by the library. It does not claim physical extent provenance for decoded APFS file contents. No missing codec required by the target images has been demonstrated.

## Remaining contract gaps

The first-tooling release is not complete:

- Real iOS 14/15 images on matching hardware have not been acquired, pinned or compared. iOS 26/27 and cross-generation comparisons remain separate qualification work.
- Filesystem-content inspection, ZIP-compressed disk-image staging, Mach-O fileset drivers and a bounded LZSS adapter remain unimplemented. Coverage reports these gaps; an inspection currently ends `partial` because filesystem-content inspection is unexamined.
- Findings currently expose direct, unreviewed static observations. The schema has revision/basis fields, but user-driven inference, assessment, linked corrections and dependency invalidation are not yet implemented. Comparison reports multiple readings as conflicts; it does not adjudicate them.
- Publication is one bounded transaction per inspection, after parsing. Per-batch restart/resume, recovery of runs left `running` after process crashes, and idempotent finalization after an ambiguous commit response remain to be implemented. Graceful cancellation records a terminal state when PostgreSQL remains reachable.
- The retained-byte quota bounds distinct supporting objects, not PostgreSQL table/index/WAL overhead or the total size of an externally managed database. Capture and read limits are bounded, but a full process memory/CPU budget and cancellation during native decompression are not provided. The initial command processes one target at a time.
- Structured successful/partial results are versioned. Invocation and fatal errors currently go to stderr; a fully structured diagnostic envelope is still outstanding. Terminal text currently presents indented structured values.
- Exact subject/property matching is implemented. Cross-hardware equivalence and a reviewed machine-profile export are not. The profile and complete machine contracts must precede runner implementation.

## Next work, in order

1. Resolve exact historical Apple restore URLs for the agreed hardware/version pair. Record the source route and any catalog limitations; retain the distinction between discovery and verified image identity.
2. Acquire and inspect one image at a time within the current free-disk budget. Exercise actual device-tree/kernel/disk formats and recover every reported source location from retained bytes. Qualify only capabilities demonstrated by those inputs.
3. Complete the concrete inspection gaps exposed by those images, including required file-content/driver dependencies. Confirm any disk/filesystem codec gap in `go-apfs-v3` before changing that library.
4. Finish the remaining finding/publication contracts and compare the matching-hardware pair after cache cleanup and database restore. This is the first-tooling acceptance gate.
5. Design the bounded whole-machine profile and complete lifecycle contracts, then implement the Hypervisor.framework runner. Static observations alone do not choose runtime behavior.

This work does not create a general campaign harness, retain disposable report trees or alter the wider Weave products.
