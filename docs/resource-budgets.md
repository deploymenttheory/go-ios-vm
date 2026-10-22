# Resource budgets for the pinned iPhone12,1 pair

These figures come from the verified iOS 14.0 (`18A373`) and iOS 15.0
(`19A346`) qualification runs on 2026-10-22. The [iOS 14](qualification/iphone12-1-ios14.md)
and [iOS 15](qualification/iphone12-1-ios15.md) records contain the host,
command, run ID and digest for each measurement. Both OS members are stored ZIP
entries and both inspections stop at the upstream UDIF LZFSE codec gap, so the
measurements do not cover real-image APFS traversal.

| Pinned image | IPSW bytes | OS member bytes | ZIP staging | Highest observed process RSS | Representative retained source bytes |
| --- | ---: | ---: | ---: | ---: | ---: |
| iOS 14.0, `18A373` | 5,574,812,591 | 5,223,921,616 | 0 | 145,457,152 (macOS) | 16,451,916 |
| iOS 15.0, `19A346` | 6,959,737,743 | 6,544,616,394 | 0 | 207,708,160 (Linux, 202,840 KiB) | 18,440,909 |

The source-byte column sums the three representative Manifest, DeviceTree and
KernelCache objects verified against each original ZIP. It is not a measurement
of all PostgreSQL tables, indexes or WAL. The CLI retains one complete source
object for each of those components and deduplicates supporting bytes by digest.
The exact iOS 14 and iOS 15 archives together exceed the default 8 GiB managed
cache limit; the iOS 14 cache object was pruned after its evidence was checked
and before the iOS 15 fetch. Cache pruning does not remove PostgreSQL evidence.

## Current limits and checks

| Resource | Default | Enforcement |
| --- | ---: | --- |
| Managed IPSW cache | 8 GiB | Pinned fetch checks declared size, existing managed bytes and available disk before transfer. |
| Free disk after fetch | 2 GiB | Checked before fetch and again against actual cache state under the exclusive cache lease. |
| Deflated OS member staging | 8 GiB expanded, 2 GiB free reserve | Declared size/free space before staging; measured bytes and free space on each write; temporary stage removed on failure or cancellation. |
| Retained distinct source bytes | 2 GiB | PostgreSQL rejects a publication that would exceed the quota; database overhead is excluded. |
| Manifest, DeviceTree, Kernel payload input | 32 MiB, 16 MiB, 128 MiB | ZIP member reads and decoded payloads have fixed caps; additional tree, prelink and APFS traversal caps apply. |
| Concurrent inspection per cache directory | 1 | Inspection takes the exclusive cache lease; cancellation while waiting leaves no new run. Cache prune/fetch wait for active inspection. |

The cache limit is intentionally one pinned archive at a time. The larger iOS 15
archive is 1,630,196,849 bytes below the 8 GiB cap; actual free disk must also leave
the 2 GiB reserve. A Deflate version of either selected OS member would fit the
8 GiB declared staging cap, but would need the listed expanded bytes on the
temporary filesystem in addition to the IPSW and reserve. The current public
pair requires no ZIP staging. The compressed-member adapter retains an encoded
source only up to its separate 32 MiB evidence cap; a larger encoded member
reports partial disk coverage.

Run `go-ios-vm resources status` for current free space, managed usage and
configured limits. `--format json` gives byte counts for scripts. After the
BuildManifest probe, `firmware fetch` prints its pinned remaining-byte estimate
before downloading; it refuses a plan that already exceeds either cache or free
disk capacity. It rechecks both constraints during the actual fetch. `inspect`
prints the declared expanded OS size and temporary-space estimate before a
Deflate member is staged. When the estimate cannot fit, other components can
still publish and disk coverage remains partial.

The published peak RSS is a baseline for the current parsers, not a memory cap.
For a compressed KernelCache, the bounded ZIP input and LZFSE output buffers
can each reach about 128 MiB (roughly 256 MiB together), before parser objects,
runtime overhead and native decoder allocations. DeviceTree uses 16 MiB caps;
OS Deflate staging streams through a 1 MiB copy buffer. These are allocation
estimates, not a measured maximum for a future image.
Allow at least 512 MiB for a single current-image inspection plus PostgreSQL and
OS headroom; remeasure after the UDIF codec opens real APFS content. There is
no hard whole-process RSS/CPU limit. The cgo LZFSE payload call cannot be
preempted while it is executing; cancellation is observed at surrounding Go
read/parse boundaries. **Release blocker:** [#52](https://github.com/deploymenttheory/go-ios-vm/issues/52)
tracks a hard native-decode cancellation bound and a real APFS-path memory
measurement before first-tooling acceptance [#20](https://github.com/deploymenttheory/go-ios-vm/issues/20).
