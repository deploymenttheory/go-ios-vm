# Firmware acquisition and the IPSW cache

`internal/firmware` acquires the supported pair into the IPSW cache, `~/Library/Caches/go-ios-vm/ipsw` by default (`--ipsw-cache` overrides it). The pair is iPhone17,3 iOS 27.0.1 (24A446) + cloudOS 26.4 (23E5207q).

## Identity

An archive's identity comes only from its `BuildManifest.plist`. File names, URLs and provider metadata never establish it.

The archive's root must hold exactly one manifest, read within a 32 MiB bound. That manifest must name the exact version and build, and:
- **iOS:** list `iPhone17,3`.
- **cloudOS:** carry the `vphone600ap` device class.

Nothing newer or similar is substituted.

## Sources

`KnownSources` holds the attributed source for each build:

| Build | Source | Size and digest | Attribution |
| --- | --- | --- | --- |
| iPhone 24A446 | Apple CDN URL | 12,343,857,610 bytes; SHA-256 `fc87bb9d…b9469` | ipsw.me v4 and Apple CDN `HEAD`, recorded 2026-10-19 in `docs/research-artifacts/verification` |
| cloudOS 23E5207q | Apple's `private-cloud-compute` URL | Not known in advance; recorded at acquisition | vphone-cli's firmware catalog (`cloud264`) |

`SelectIPSWMe` and `FetchIPSWMe` also select from an ipsw.me v4 device response.
- **Board shapes:** the object-shaped `boards` (`boardconfig`, `platform`, `cpid`, `bdid`) are decoded, and the older string form is accepted too.
- **Exact match only:** zero matching firmwares, or more than one, is an error.
- **Provenance:** the raw selected record and the response's SHA-256 are kept.

A local archive is verified in place and never copied into the cache.

## Cache layout

| Path | Contents |
| --- | --- |
| `<sha256>.ipsw` | A verified archive, named by its digest |
| `<sha256>.json` | Its record: identity, digest and size, retrieval time, source with selection metadata, and the manifest's product, build, devices and device classes |
| `.staging/<build>.part` | An unfinished download, kept so the next attempt can resume |
| `.lock` | The cache's OS lock; one process publishes at a time |

**Publication** happens only after the download completes. The size, the expected digest when known, and the manifest identity must all match. The archive is renamed into place, then the record.

**Interrupted downloads**, and downloads the server ends early, keep their partial file. The next attempt resumes with a range request, or restarts when the server ignores the range.

**Rejected downloads** are discarded and never published. These are downloads with the wrong digest, size or identity, or that aren't an archive.

**Free space** is checked before a download starts.

## Existing entries

Archives the cache didn't publish have no record. On this host that means the iOS 14.0 (18A373) and iOS 15.0 (19A346) images from an earlier tool. Such archives:
- are never opened for writing, renamed or removed;
- never satisfy a selection.

A published entry is re-verified each time it's used. A changed file is reported as `ErrCacheEntry`, never used.

## Checks

| Command | What it covers |
| --- | --- |
| `go test ./internal/firmware` | Generated archives and an HTTP test server: identity, malformed archives, resume, ignored ranges, interruption, cancellation, digest/size/identity rejection, no space, tampered entries and the lock |
| `make test-native` | Inspects the real cache read-only: its existing entries are rejected and left unchanged |
| `make test-live` | Downloads and verifies the real pair. Needs network access and about 13.5 GiB free; it fails, rather than skips, when that's unavailable |
