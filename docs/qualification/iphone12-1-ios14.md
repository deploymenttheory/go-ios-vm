# iPhone12,1 iOS 14.0 qualification

This record follows the exact public restore request `iPhone12,1` / iOS 14.0 /
build `18A373`. The historical-index result locates an Apple CDN object; the
index is not the authority for archive identity. The [historical resolution
record](historical-resolution.md) describes the bounded remote BuildManifest
probe and the source limitations.

## Source and selected identity

| Field | Recorded value |
| --- | --- |
| Locator | [IPSW Downloads iPhone12,1 device API](https://api.ipsw.me/v4/device/iPhone12%2C1?type=ipsw) |
| Apple restore object | [iPhone11,8,iPhone12,1_14.0_18A373_Restore.ipsw](https://updates.cdn-apple.com/2020SummerFCS/fullrestores/001-46828/6A00C15C-8AEB-490E-A468-04E28C68E7C9/iPhone11,8,iPhone12,1_14.0_18A373_Restore.ipsw) |
| Declared, CDN, and complete file length | 5,574,812,591 bytes |
| Complete IPSW SHA-256 | `a30f2fa8d4dbdcd2a8cf0f1644de3879019ba6f8be8fba49806a9c82dec09759` |
| Complete IPSW SHA-1, matching locator | `9d9bc29986dd302e26ccbf9cba069262bd3b2295` |
| Local and remote BuildManifest SHA-256 | `5254a90343946ab4c8c4e1970a251763348648ea14b346d824458741b559615a` |
| Selected BuildIdentity index | 1 |
| Selected board and variant | `n104ap`, Customer Erase |

The selected index is paired with `n104ap` and Customer Erase in the actual
BuildManifest. The separate [hardware mapping](../inspection-architecture.md)
identifies `iPhone12,1` as `n104ap`; the manifest's global product list alone
does not assign a board to the product. No seed build or alternate release is
substituted.

A bounded ZIP directory and manifest read also identified 138 members and the
selected components before full acquisition:

| Component | ZIP member | ZIP method | Uncompressed bytes |
| --- | --- | ---: | ---: |
| OS disk | `038-03664-370.dmg` | 0 (stored) | 5,223,921,616 |
| KernelCache | `kernelcache.release.iphone12b` | 8 (deflate) | 15,680,196 |
| DeviceTree | `Firmware/all_flash/DeviceTree.n104ap.im4p` | 8 (deflate) | 42,629 |

The stored OS member can be borrowed by the current disk adapter without ZIP
staging. A bounded remote `diskimage.New` probe of that member returned
`UDIF chunk codec 0x80000007: unsupported feature`. The current `go-apfs-v3`
UDIF reader does not decode that LZFSE chunk kind; see
[go-apfs-v3 #146](https://github.com/deploymenttheory/go-apfs-v3/issues/146).
The full local inspection below reproduces the same error. No disk finding is
published, and the filesystem remains unexamined. These format observations do
not establish VM capability.

## Qualification hosts

- macOS 27.0.1 (26A434), Darwin arm64, 10 CPUs, 24 GiB RAM, Go 1.27.1.
  Approximately 81 GiB was available on the data volume before acquisition.
- Alpine Linux 3.22.4, kernel 6.12.81-0-virt, aarch64, Go 1.27.1. The isolated
  Lima 2.2.1 guest has four CPUs, 4 GiB RAM and a 16 GiB sparse disk. It reads
  the same host IPSW through a read-only mount and connects to the dedicated
  PostgreSQL instance through `host.lima.internal`; it does not copy the IPSW
  into the guest disk.

## Full archive verification and commands

`firmware fetch --product iPhone12,1 --version 14.0 --build 18A373` resumed an
ETag-pinned managed partial and published the SHA-256-named cache file only after
its length, declared SHA-1, SHA-256, and local BuildManifest identity checks
passed. An independent streaming read of the resulting file confirmed the
5,574,812,591-byte length and both archive digests above. Reading
`BuildManifest.plist` from the complete ZIP independently reproduced the remote
manifest digest above. The archive has 138 ZIP members and six actual build
identities. `target list` on both hosts returned identical target records; index
1 pairs `n104ap` with `Customer Erase Install (IPSW)`, version `14.0`, build
`18A373`, and the manifest's global product list including `iPhone12,1`.

The CLI source included the progressing-transfer fix at commit `ac8e909`.
Both inspected binaries used Go 1.27.1 with cgo; the macOS binary was built
from that working tree before the commit (its embedded VCS stamp is
`1093c8b+dirty`), while the Linux binary used `-buildvcs=false` from the same
source. PostgreSQL had the embedded schema migrations applied. The commands
below use an environment-supplied `GOIOSVM_DATABASE_URL`; the Linux guest also
used `--cache-dir /tmp/go-ios-vm-cache` and a guest-reachable database address.

```sh
IMAGE=/path/to/a30f2fa8d4dbdcd2a8cf0f1644de3879019ba6f8be8fba49806a9c82dec09759.ipsw
go-ios-vm --format json target list --image "$IMAGE"
go-ios-vm --format json inspect --image "$IMAGE" --identity 1
go-ios-vm --format json finding list --run RUN_ID
go-ios-vm --format json finding show --evidence EVIDENCE_SHA256
```

Both `inspect` invocations returned exit status 2 with a versioned `partial`
result, not a fatal failure. `/usr/bin/time` measured the whole inspection
command, including archive hashing and publication:

| Host | Run ID | Wall time | Peak resident memory | Findings |
| --- | --- | ---: | ---: | ---: |
| macOS arm64 | `a4901f8609ba049a913e798b84420704` | 12.36 s | 145,457,152 bytes | 3,592 |
| Linux arm64 | `6c3428ae17f4b4ebf78e7e2a59fb2096` | 7.54 s | 141,948 KiB | 3,592 |

The hosts published the same target ID, coverage, sets of observation IDs, and
sets of evidence IDs. The separate run IDs preserve the host trials.

## Coverage and retained source checks

| Component | Both hosts | Observed limit or result |
| --- | --- | --- |
| Manifest | complete | One identity and 70 component-member findings |
| DeviceTree | complete | 3,520 property findings |
| KernelCache | partial | Mach-O header retained; driver parsing stops at IOKit `IDREF` object references. [go-ios-vm #25](https://github.com/deploymenttheory/go-ios-vm/issues/25) |
| Disk metadata | partial | UDIF LZFSE chunk `0x80000007` unsupported by `go-apfs-v3`. [go-apfs-v3 #146](https://github.com/deploymenttheory/go-apfs-v3/issues/146) |
| Filesystem content | unexamined | Adapter is not implemented. [go-ios-vm #13](https://github.com/deploymenttheory/go-ios-vm/issues/13) |

The kernel's 1,121,340-byte `__PRELINK_INFO/__info` section contains 3,383
`<integer IDREF="…"/>` elements. The ordinary property-list decoder reports
`invalid empty <integer/>`; the [Apple IOKit object unserializer](https://github.com/apple-oss-distributions/IOKitUser/blob/main/IOCFUnserialize.tab.c)
defines an `IDREF` token and object-tag table. The evidence supports a
specific object-reference parser gap, not an assumption that those references
mean zero. The kernel header remains a valid direct finding; driver inventory
is incomplete.

On each host, `finding show` recovered the following three distinct retained
objects. Their SHA-256 IDs matched the retrieved bytes, and those bytes matched
the corresponding uncompressed ZIP members in the original archive:

| Representative source | Coordinate | Evidence bytes | Evidence SHA-256 |
| --- | --- | ---: | --- |
| `BuildManifest.plist` | `plist-selector` | 729,091 | `5254a90343946ab4c8c4e1970a251763348648ea14b346d824458741b559615a` |
| `Firmware/all_flash/DeviceTree.n104ap.im4p` | `decoded-payload-byte` | 42,629 | `a28601eae6560fe3e6e49d8309ebb31c7c74ee71f57ef2df8aaac7fa5bfb5715` |
| `kernelcache.release.iphone12b` | `decoded-payload-byte` | 15,680,196 | `beb656073ec8e4ba140a4e08b3fd8b9b0d1fdca39563f0371b15cb79eed36239` |

The 3,592 findings on each host point to the complete archive's artifact ID;
every ZIP entry index resolves to its recorded member name among the 138
members. Their coordinates comprise 3,521 decoded-payload references and 71
manifest selectors. Only the representative retained objects were compared
byte-for-byte with source members; the decoded payload coordinates were not
independently replayed for every property. There is no disk finding or disk
evidence to recover while the UDIF decoder refuses the chunk. No firmware or
extracted payload is committed to this repository.

## IDREF decoder follow-up — 2026-10-22

The bounded IOKit XML decoder in [issue #25](https://github.com/deploymenttheory/go-ios-vm/issues/25)
resolved both integer and string `IDREF` objects from this same verified archive.
The new macOS and Linux binaries used Go 1.27.1 and the same source snapshot.
Each run retained the selected identity and 3,592 earlier findings, then added
938 prelinked driver findings across 246 bundle identifiers:

| Host | Run ID | Total findings | KernelCache coverage |
| --- | --- | ---: | --- |
| macOS arm64 | `50b8ca73b4098faa7a5764ade7ced655` | 4,530 | complete |
| Linux arm64 | `4547bead025371cf4ca33b87fdb995df` | 4,530 | complete |

The driver properties comprise 246 `CFBundleIdentifier`, 246
`CFBundleVersion`, 238 `OSBundleLibraries`, and 208 `IOKitPersonalities`
findings on each host. All 4,530 observation IDs match across hosts. Every
driver finding retains a `decoded-payload-byte` coordinate for
`kernelcache.release.iphone12b` and the same raw kernel evidence ID shown above.
On both hosts, `finding show` recovered 15,680,196 bytes for that ID; the
retrieved bytes hash to its recorded SHA-256. Disk metadata remains partial at
UDIF LZFSE chunk `0x80000007`, and filesystem content remains unexamined.
