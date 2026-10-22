# iPhone12,1 iOS 15.0 qualification

This record follows the exact public restore request `iPhone12,1` / iOS 15.0 /
build `19A346`. The [historical resolution record](historical-resolution.md)
documents the source route and the bounded remote BuildManifest probe. The
historical index locates an Apple CDN archive; its metadata is not independent
proof of the complete archive's identity.

The live `firmware list --product iPhone12,1 --search 15.0` result included
the exact `19A346` restore row with an `unverified_catalog_locator` status and
a copyable exact fetch command. The list is a way to choose the requested
release; the bounded probe and complete-image checks establish progressively
stronger source evidence.

## Source and selected identity

| Field | Recorded value |
| --- | --- |
| Locator | [IPSW Downloads iPhone12,1 device API](https://api.ipsw.me/v4/device/iPhone12%2C1?type=ipsw) |
| Apple restore object | [iPhone11,8,iPhone12,1_15.0_19A346_Restore.ipsw](https://updates.cdn-apple.com/2021FallFCS/fullrestores/002-02965/E21A0F85-CE8B-410B-96E1-E367D5619112/iPhone11,8,iPhone12,1_15.0_19A346_Restore.ipsw) |
| Declared and CDN length | 6,959,737,743 bytes |
| Complete IPSW SHA-256 | `13d574566eec7be3b9e23ec93c2374cabf7f150be63aafd7f8479d7d162480e0` |
| Complete IPSW SHA-1, matching locator | `464c911e83a7783138c6b58df35de9fa961d04cd` |
| Local and remote BuildManifest SHA-256 | `82fd2380701459c5816061d2f8ce52c8305123a5e16e69f44e634d3cc147c8d4` |
| Selected BuildIdentity index | 1 |
| Selected board and variant | `n104ap`, Customer Erase Install (IPSW) |

The bounded probe on 2026-10-22 read 73,511 bytes in 22 Apple CDN range
requests. The remote manifest was 869,419 bytes and claimed product
`iPhone12,1`, version `15.0`, and build `19A346`. It paired identity 1 with
`n104ap` and Customer Erase. The [hardware mapping](../inspection-architecture.md)
identifies `iPhone12,1` as `n104ap`; the global product list in either release's
manifest does not assign a board to a product. This selection matches the
[verified iOS 14.0 image](iphone12-1-ios14.md) on product and board, while each
release has its own source and archive identity.

The bounded ZIP directory read found 148 archive members and the selected
components below. The complete local ZIP independently has 148 members.

| Component | ZIP member | ZIP method | Uncompressed bytes |
| --- | --- | ---: | ---: |
| OS disk | `018-80101-001.dmg` | 0 (stored) | 6,544,616,394 |
| KernelCache | `kernelcache.release.iphone12b` | 8 (deflate) | 17,528,372 |
| DeviceTree | `Firmware/all_flash/DeviceTree.n104ap.im4p` | 8 (deflate) | 43,118 |

## Qualification hosts and procedure

- macOS 27.0.1 (26A434), Darwin arm64, 10 CPUs, 24 GiB RAM, Go 1.27.1.
- Alpine Linux 3.22.4, kernel 6.12.81-0-virt, aarch64, Go 1.27.1. The
  disposable Lima 2.2.1 guest has four CPUs, 4 GiB RAM, and a 16 GiB sparse
  disk. It reads the same host IPSW through a read-only mount and connects to
  the dedicated PostgreSQL instance through `host.lima.internal`; it does not
  copy the archive into the guest disk.

The macOS managed cache has an 8 GiB image limit and a 2 GiB free-disk reserve.
The previously qualified iOS 14 image was pruned through `cache prune --apply`
after its macOS and Linux parser validation and retained-byte checks finished.
This leaves one large image in the cache during iOS 15 acquisition. The iOS 14
run records and their supporting bytes remain in PostgreSQL.

With `GOIOSVM_DATABASE_URL` supplied through the environment, the qualification
commands are:

```sh
go-ios-vm --format json firmware list --product iPhone12,1 --search 15.0
go-ios-vm --format json firmware probe --product iPhone12,1 --version 15.0 --build 19A346
go-ios-vm --format json firmware fetch --product iPhone12,1 --version 15.0 --build 19A346
go-ios-vm --format json target list --image "$IMAGE"
go-ios-vm --format json inspect --image "$IMAGE" --identity 1
go-ios-vm --format json finding list --run "$RUN_ID"
go-ios-vm --format json finding show --evidence "$EVIDENCE_SHA256"
```

The Linux guest uses `--cache-dir /tmp/go-ios-vm-cache` for its local command
configuration while reading the host's verified image path through the mount.
Both host runs use the same selected identity index, and each run has its own
run ID. The archive and representative retained source objects are checked
independently against their recorded SHA-256 values; the archive check also
checks the historical locator's declared SHA-1.

The acquisition binary was built from merged PR #24 at commit `e51fc1f`.
The macOS and Linux inspection binaries use Go 1.27.1 with cgo and the same
merged source revision `e8e201194fdc2f0425aca12aada56f6caed5d654`,
including the kernel `IDREF` parser from PR #27 and finding revision schema
002 from PR #28. The two host inspections are separate trials of that revision;
one is not an independent firmware source.

## Complete archive and inspection results

The ETag-pinned managed fetch completed on 2026-10-22 and published the
SHA-256-named cache object only after full length, declared SHA-1, computed
SHA-256, and local BuildManifest identity checks. An independent streaming
read on **each** host reproduced the archive's length and both hashes above.
Reading `BuildManifest.plist` from each complete ZIP reproduced its remote
digest and 869,419-byte length. Both `target list` calls returned the same six
build identities; index 1 pairs board `n104ap` with Customer Erase Install
(IPSW), version `15.0`, and build `19A346`. The manifest's global product list
includes `iPhone12,1` and `iPhone11,8`. The product-to-board selection also
uses the separate hardware mapping, as in the [iOS 14 record](iphone12-1-ios14.md).

The dedicated database was migrated from schema 001 to 002 **after** the
acquisition process completed. Both inspections used schema 002. Each `inspect`
command returned exit status 2 with a versioned `partial` result. Wall time
includes archive hashing and database publication:

| Host | Run ID | Wall time | Peak resident memory | Findings |
| --- | --- | ---: | ---: | ---: |
| macOS arm64 | `cd0726e72ad4cc80f2d9e81018e1ee89` | 38.75 s | 167,903,232 bytes | 4,612 |
| Linux arm64 | `b184111188f89ce37814c63724fb7ecc` | 34.09 s | 202,840 KiB | 4,612 |

The two hosts published the same target ID and coverage. `compare --left
cd0726e72ad4cc80f2d9e81018e1ee89 --right
b184111188f89ce37814c63724fb7ecc` reported 4,612 equal exact
subject/property observations, with no changed or one-sided observations.
Their separate run IDs preserve the host trials. Both runs contain one selected
target, 82 component-member, 3,571 DeviceTree, one kernel header, and 957
prelinked-driver findings. The driver findings cover 251 bundle identifiers:
251 `CFBundleIdentifier`, 251 `CFBundleVersion`, 243 `OSBundleLibraries`, and
212 `IOKitPersonalities` properties. All 4,612 observations point to this
complete archive's artifact ID. Their source coordinates comprise 83 manifest
selectors and 4,529 decoded-payload references.

| Component | Both hosts | Observed result and follow-up |
| --- | --- | --- |
| Manifest | complete | Selected identity and 82 component members retained |
| DeviceTree | complete | 3,571 direct property findings |
| KernelCache | complete | Header and 251 prelinked-driver bundles parsed with the [IDREF fix](https://github.com/deploymenttheory/go-ios-vm/issues/25) |
| Disk metadata | partial | Stored OS member reaches unsupported UDIF LZFSE chunk `0x80000007`; [go-apfs-v3 #146](https://github.com/deploymenttheory/go-apfs-v3/issues/146) owns that decoder |
| Filesystem content | unexamined | The inspection revision used here predates the bounded content path in [go-ios-vm #13](https://github.com/deploymenttheory/go-ios-vm/issues/13) |

The stored OS member can be borrowed by the disk adapter without ZIP staging.
The UDIF decoder stops before a disk finding is emitted. The same codec gap
appears in the iOS 14 image, but each observation here comes from its own
verified iOS 15 archive. A later isolated-schema trial of the bounded APFS
content path from [PR #32](https://github.com/deploymenttheory/go-ios-vm/pull/32)
also stopped at this codec, with partial disk and filesystem coverage and zero
filesystem observations. These static findings do not establish bootability or
a usable VM.

## Retained source checks and same-hardware comparison

On each host, `finding show --evidence` recovered these three retained objects.
Their IDs matched the SHA-256 of the retrieved bytes, and the complete bytes
matched the corresponding uncompressed ZIP members in the original archive:

| Representative source | Coordinate | Evidence bytes | Evidence SHA-256 |
| --- | --- | ---: | --- |
| `BuildManifest.plist` | `plist-selector` | 869,419 | `82fd2380701459c5816061d2f8ce52c8305123a5e16e69f44e634d3cc147c8d4` |
| `Firmware/all_flash/DeviceTree.n104ap.im4p` | `decoded-payload-byte` | 43,118 | `eecc5bdadff63b8e561858e0a64f03bf1af0b692e6773b90050d3977cd597904` |
| `kernelcache.release.iphone12b` | `decoded-payload-byte` | 17,528,372 | `55d7d9c5ba8ec98082b98a937da7277a70c33f2401ec8daab7c96a798841d11c` |

For every finding, its ZIP entry index resolves to its recorded member name
among the 148 members. Only these representative retained objects were
compared byte-for-byte with source members; individual decoded coordinates
were not independently replayed for every property. There is no disk finding
or disk evidence to recover while the UDIF decoder refuses the chunk. No
firmware or extracted payload is committed to this repository.

The verified [iOS 14.0 build `18A373`](iphone12-1-ios14.md) and this iOS 15.0
build `19A346` have distinct complete archive hashes and separately verified
BuildManifests. Both selected identity index 1 for `iPhone12,1` / `n104ap`
Customer Erase. The iOS 14 archive was pruned from the managed cache after its
own qualification, leaving its findings and supporting bytes in PostgreSQL.
An exact subject/property `compare` between its later fixed-parser macOS run
`50b8ca73b4098faa7a5764ade7ced655` and this iOS 15 macOS run returned
3,852 equal, 604 changed, and 230 unexamined entries. This is a retained
observation comparison across releases on the same hardware profile; it does
not infer semantic or boot compatibility.
