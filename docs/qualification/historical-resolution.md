# Historical restore resolution — iPhone12,1

Checked 2026-10-22 for the agreed exact requests, iOS 14.0 and 15.0. The current Apple GDMF and iTunes restore feeds did not return either release. The [IPSW Downloads device index](https://ipsw.me/iPhone12%2C1/) supplied historical Apple CDN URLs through its [device API](https://api.ipsw.me/v4/device/iPhone12%2C1?type=ipsw). That index is a **locator**, not Apple evidence of archive identity or compatibility. No other version or build was substituted.

| Request | Build | Apple restore archive | Bytes declared by index and Apple CDN | Index SHA-1 | BuildManifest SHA-256 |
| --- | --- | --- | ---: | --- | --- |
| iPhone12,1 / 14.0 | 18A373 | [iPhone11,8,iPhone12,1_14.0_18A373_Restore.ipsw](https://updates.cdn-apple.com/2020SummerFCS/fullrestores/001-46828/6A00C15C-8AEB-490E-A468-04E28C68E7C9/iPhone11,8,iPhone12,1_14.0_18A373_Restore.ipsw) | 5,574,812,591 | `9d9bc29986dd302e26ccbf9cba069262bd3b2295` | `5254a90343946ab4c8c4e1970a251763348648ea14b346d824458741b559615a` |
| iPhone12,1 / 15.0 | 19A346 | [iPhone11,8,iPhone12,1_15.0_19A346_Restore.ipsw](https://updates.cdn-apple.com/2021FallFCS/fullrestores/002-02965/E21A0F85-CE8B-410B-96E1-E367D5619112/iPhone11,8,iPhone12,1_15.0_19A346_Restore.ipsw) | 6,959,737,743 | `464c911e83a7783138c6b58df35de9fa961d04cd` | `82fd2380701459c5816061d2f8ce52c8305123a5e16e69f44e634d3cc147c8d4` |

Apple CDN `HEAD` responses supplied the stated lengths, strong ETags and `Accept-Ranges: bytes`. An ETag-fenced request for the first 1 KiB returned HTTP 206 with the exact content range. A bounded ZIP range probe then read each `BuildManifest.plist` directly from Apple's archive: 21 requests/65,535 bytes for iOS 14.0 and 22 requests/73,511 bytes for iOS 15.0. Both manifests claim the requested product, version and build. Both list board identities `n104ap` and `n841ap` and the customer erase/upgrade and research customer erase/upgrade variants. The probe returns each board and variant **paired with its identity index**; the manifest's global product list does not by itself map a product to one board.

With PostgreSQL configured, repeat the bounded check using:

```sh
go run ./cmd/go-ios-vm --format json firmware probe --product iPhone12,1 --version 14.0 --build 18A373
go run ./cmd/go-ios-vm --format json firmware probe --product iPhone12,1 --version 15.0 --build 19A346
```

These checks verify the *retrieved manifest bytes and CDN response behavior*. They do not verify the index SHA-1, compute a full-archive SHA-256, pin an IPSW, establish signing status, or demonstrate bootability. The full archive verification and exact `n104ap` identity selection belong to the two image-qualification issues, [iOS 14](https://github.com/deploymenttheory/go-ios-vm/issues/10) and [iOS 15](https://github.com/deploymenttheory/go-ios-vm/issues/11). The original catalog response and probed manifest are retained in PostgreSQL when the CLI commands run.

## Full-image transfer constraint

An exact iOS 14 full fetch was attempted on this Mac after the manifest probe.
The first roughly 70 MiB transferred too slowly for a useful single-session
qualification, so it was stopped and the disposable stage was removed. A separate
8 MiB Apple CDN range took 21.6 seconds; four simultaneous 8 MiB ranges took
roughly 70–90 seconds each, with two timing out before completion. Parallel
requests did not improve aggregate throughput in this environment. This is a
local observation, not a claim about Apple's normal CDN speed.

The full-fetch path now uses a URL-and-ETag-keyed managed partial, accounts for
that partial in the cache budget, and resumes only when Apple returns the exact
remaining range for the same ETag. It still requires the declared checksum (when
available), a whole-image SHA-256 and a local BuildManifest identity check before
publishing a completed IPSW. Resume makes a long transfer durable; it does not
make the network faster. Only one image should be acquired at a time within the
8 GiB cache and the configured disk reserve.
