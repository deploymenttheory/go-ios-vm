# Resource budgets for the pinned iPhone12,1 pair

The baseline figures below come from the verified iOS 14.0 (`18A373`) and iOS 15.0
(`19A346`) qualification runs on 2026-10-22. The [iOS 14](qualification/iphone12-1-ios14.md)
and [iOS 15](qualification/iphone12-1-ios15.md) records contain the host,
command, run ID and digest for each measurement. Both OS members are stored ZIP
entries and both inspections stop at the upstream UDIF LZFSE codec gap, so the
baseline measurements do not cover real-image APFS traversal. A later iOS 15
trial with the still-open upstream codec work is recorded below.

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

## Native worker and post-codec trial

IM4P LZFSE decoding now runs in a child process. Each DeviceTree or KernelCache
decode has a 30-second wall deadline and a 31-second CPU limit. The child reads
at most 128 MiB of compressed bytes and writes at most the selected component's
16 MiB or 128 MiB decoded cap; neither side writes a temporary file. Linux also
limits the child's virtual address space to 2 GiB. Both hosts sample the child's
peak resident set size every 10 ms and stop it above 512 MiB. macOS rejects
`RLIMIT_AS` and `RLIMIT_DATA`, so its resident threshold is a sampled bound,
not a hard allocation limit. The parent kills and reaps the worker on timeout
or user cancellation. Timeout, resource failure and cancellation remain distinct:
the first two leave partial component coverage; cancellation leaves an
interrupted run without final coverage.

The exact cached iOS 15.0 `19A346` archive (6,959,737,743 bytes; SHA-256
`13d574566eec7be3b9e23ec93c2374cabf7f150be63aafd7f8479d7d162480e0`)
was inspected at identity 1 (`iPhone12,1`, `n104ap`) on both arm64 hosts with
this branch based on PR #53 and a local upstream go-apfs-v3 composite commit
`52e6d48`. That composite contains the open [UDIF LZFSE PR #147](https://github.com/deploymenttheory/go-apfs-v3/pull/147)
and [decmpfs type-4 PR #149](https://github.com/deploymenttheory/go-apfs-v3/pull/149).
Upstream [CI limit PR #150](https://github.com/deploymenttheory/go-apfs-v3/pull/150)
and [snapshot unmount PR #151](https://github.com/deploymenttheory/go-apfs-v3/pull/151)
are also open. These are trial measurements, not a claim that the released
`go-apfs-v3 v0.1.0` dependency can traverse this APFS image.

| Host | Run ID | Wall | User CPU | System CPU | Reported peak RSS | Findings | Retained distinct bytes | PostgreSQL table/index bytes | ZIP staging |
| --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| macOS arm64, Go 1.27.1 | `8a08e8be3128af6c35e16f68f3f034ec` | 13.66 s | 3.15 s | 1.95 s | 215,400,448 | 5,369 | 29,562,367 | 51,945,472 | 0 |
| Linux arm64, Go 1.27.1 | `045120fd5833330562dc36b144be0274` | 23.46 s | 4.17 s | 3.46 s | 218,132,480 | 5,369 | 29,562,367 | 51,945,472 | 0 |

Both runs have complete Manifest, DeviceTree, KernelCache and disk-metadata
coverage. Filesystem-content is partial under the existing 65,536-byte per-file
capture cap for two files and the bounded path selection; filesystem-wide
absence remains unexamined. The selected OS ZIP member is stored, so no
expanded-image staging file was needed. PostgreSQL table/index bytes exclude
cluster WAL and shared overhead. `time` reports a peak for the inspected command,
not the simultaneous sum of parent and worker RSS. Allow at least 1 GiB for
the inspection process tree, plus separate PostgreSQL and OS headroom. This is
an operational minimum derived from the observed parent peak and the 512 MiB
worker threshold, not a whole-process hard cap.

The exact iOS 14 archive is absent from the managed cache after the prior
required prune, and the two archives together exceed its 8 GiB limit. Its
post-codec APFS path is still unmeasured; the iOS 14 baseline above must not
be used as a real-APFS peak. Reacquire it sequentially and repeat this trial
before closing the two-image first-tooling gate.

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

The published peak RSS is an observed baseline, not a memory cap. For a
compressed KernelCache, bounded ZIP input and decoded output can each reach
about 128 MiB in different processes, before parser objects and runtime
overhead. DeviceTree uses 16 MiB caps; OS Deflate staging streams through a
1 MiB copy buffer. There is no hard whole-process RSS/CPU limit. First-tooling
acceptance [#20](https://github.com/deploymenttheory/go-ios-vm/issues/20)
still depends on the upstream codec work and the remaining iOS 14 measurement.
