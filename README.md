# go-ios-vm

A fresh Go project with two deliverables:

- A suite of tools for inspecting IPSWs and firmware, to understand and implement virtual models of additional iPhone hardware generations.
- An iOS virtual machine runner for macOS using Hypervisor.framework.

## Agreed direction

Tools are the initial focus. The project language is Go; supporting tools may use Python. The inspection tools target macOS and Linux; Windows is out of scope. The Hypervisor.framework runner targets Apple Silicon macOS. The first guest target is iOS 14. The full version scope is iOS 14, 15, 26, and 27. Scope does not imply demonstrated support.

The inspection suite covers static firmware inspection and live execution observation. Work begins with static inspection: image components, device identities, device trees, kernel drivers and dependencies, and differences between images. Live observation will expose execution, device accesses, interrupts, and failures when a runner is available. Each tool must answer a concrete question and provide a usable output.

The first inspection workflow is: open an IPSW, select a device identity, explore its firmware and hardware descriptions, and compare findings with another image. Unsupported content must be reported explicitly. The CLI uses Cobra for commands and Viper for configuration, with readable terminal output and structured output for consumers. Existing libraries and tools should be reused where suitable; new Go tooling will fill demonstrated gaps. Confirmed missing disk-image/filesystem codec support belongs in `go-apfs-v3`. Hardware findings distinguish direct observations, interpretations, and unknowns, and retain references to their sources.

PostgreSQL stores findings, provenance, and retained supporting evidence, including binary data. Disposable decoded files and bulk traces are subject to bounded retention. IPSWs are acquired on demand using GDMF discovery plus Apple-owned restore catalogs as needed, and held in a managed local cache. When those current feeds lack an exact historical release, a historical index can locate an Apple-hosted restore URL; its claim is checked against the archive's BuildManifest and is not treated as a verified image. Development includes a dedicated local PostgreSQL instance; its connection is configurable through Viper for local or remote use. Findings inform a reviewed, versioned machine profile consumed by the runner.

Usable-VM acceptance includes an interactive iOS UI, input, persistence, restart, and operation with SIP enabled. The first inspection image is pinned; runner hardware and image builds remain to be selected. The wider Weave product family remains unchanged.

This is a fresh implementation. The previous project may be consulted for a specific question; its code, reports, image assets, and architecture are not a migration baseline. No historical material has been imported.

## Current stage

The design interview is accepted. The first CLI implementation is merged into `main`. The [exact iPhone12,1 iOS 14.0 image](docs/qualification/iphone12-1-ios14.md) has been verified and inspected on macOS and Linux with **partial coverage**; the full first-tooling release is not yet accepted. No VM runner has been implemented.

Implemented: Apple catalog discovery, bounded verified downloads, explicit BuildManifest identity selection, source-addressable device-tree and prelinked kernel/driver metadata, read-only disk/APFS metadata through `go-apfs-v3`, PostgreSQL publication/retrieval, run comparison and leased cache cleanup. Unsupported components produce partial coverage. Retained evidence includes the bytes needed to inspect findings after source-cache deletion.

See [implementation status and limitations](docs/implementation-status.md), the [evidence model](docs/evidence-model.md), [inspection architecture](docs/inspection-architecture.md), and [design decisions](docs/README.md).

## Run locally

Requires Go 1.27.1+, a C compiler (Xcode command-line tools on macOS), Python 3 for setup scripts, and Docker Compose for the optional local database. The inspection CLI itself uses Go and C-backed upstream LZFSE; Python is not a runtime dependency.

```sh
python3 scripts/dev-env.py  # once; preserves any existing .env
set -a
source .env
set +a
docker compose up -d --wait postgres
go build -o bin/go-ios-vm ./cmd/go-ios-vm
bin/go-ios-vm db migrate
bin/go-ios-vm firmware list --product iPhone12,1
bin/go-ios-vm firmware list --product iPhone12,1 --search 14.0
bin/go-ios-vm firmware fetch --product iPhone12,1 --version 14.0 --build 18A373
```

`firmware list` requires PostgreSQL (`GOIOSVM_DATABASE_URL` or `--database-url`) and queries Apple release metadata, Apple's restore catalog, and the historical device index. It shows only restore IPSW rows, sorted by version and build. `--search` matches a version or build substring without needing the exact build first. Each row includes the Apple-hosted URL, source, declared size and SHA-1 where available, an explicit **unverified catalog locator** status, and a copyable exact `firmware fetch` command. Release-only metadata cannot be downloaded. If any source fails, the result is `partial` with exit status 2, including the failed source; an empty matching result is not proof that no archive exists. Conflicting locators for one exact version/build are marked ambiguous and do not offer a fetch command.

For automation, use the versioned JSON envelope:

```sh
bin/go-ios-vm --format json firmware list --product iPhone12,1 --search 18A373
```

For the documented iOS 14.0 locator, text includes a row like this (the URL is abbreviated here):

```text
14.0 18A373  unverified_catalog_locator  5574812591 bytes  SHA-1 9d9bc29986dd302e26ccbf9cba069262bd3b2295
  source: https://api.ipsw.me/v4/device/iPhone12%2C1?type=ipsw
  URL: https://updates.cdn-apple.com/.../iPhone11,8,iPhone12,1_14.0_18A373_Restore.ipsw
  next: go-ios-vm firmware fetch --product 'iPhone12,1' --version '14.0' --build '18A373'
```

The versioned JSON shape carries the same row and every source status:

```json
{"schema_version":1,"state":"complete","results":{"product":"iPhone12,1","query":"18A373","sources":[{"source":"https://api.ipsw.me/v4/device/iPhone12%2C1?type=ipsw","status":"complete"}],"restores":[{"product":"iPhone12,1","version":"14.0","build":"18A373","kind":"restore_ipsw","url":"https://updates.cdn-apple.com/.../iPhone11,8,iPhone12,1_14.0_18A373_Restore.ipsw","source":"https://api.ipsw.me/v4/device/iPhone12%2C1?type=ipsw","size":5574812591,"sources":["https://api.ipsw.me/v4/device/iPhone12%2C1?type=ipsw"],"status":"unverified_catalog_locator","fetchable":true,"next_command":"firmware fetch --product 'iPhone12,1' --version '14.0' --build '18A373'"}]}}
```

The actual `results.sources` includes the Apple catalogs as well. Listing records raw catalog responses with source attribution in PostgreSQL. The current Apple catalogs did not resolve the proposed iOS 14.0/15.0 fixtures during the initial checks. A historical locator has since returned exact Apple CDN candidates; see [historical resolution and bounded manifest probes](docs/qualification/historical-resolution.md). Do not substitute another image automatically.

`firmware probe --product PRODUCT --version VERSION --build BUILD` reads only bounded byte ranges to check an Apple CDN archive's BuildManifest. It reports the manifest's identity indices, boards, variants, and digest but **does not verify the whole archive**. For an exact resolved release, `firmware fetch` probes, downloads, verifies the full archive's declared size, checksum where supplied, and BuildManifest identity, then publishes a SHA-256-named cache entry. It refuses an unresolved or ambiguous release, a failed check, or insufficient cache/free-disk capacity. When Apple and the historical index give the same URL, their metadata is combined; conflicting URL, size or checksum declarations for an exact selection are rejected.

```sh
bin/go-ios-vm target list --image /path/to/verified.ipsw
bin/go-ios-vm --format json inspect --image /path/to/verified.ipsw --identity 0
bin/go-ios-vm finding list --run RUN_ID
bin/go-ios-vm finding show --evidence EVIDENCE_SHA256
bin/go-ios-vm compare --left FIRST_RUN_ID --right SECOND_RUN_ID
bin/go-ios-vm cache status
bin/go-ios-vm cache prune         # preview
bin/go-ios-vm cache prune --apply
```

Choose the identity index from `target list`; `0` is only an example. Evidence bytes in `finding show` are base64 within JSON. Present zero, missing observations, and conflicting readings stay distinct in comparisons. Comparisons match exact semantic subjects/properties; they do not establish cross-hardware equivalence.

Viper accepts `--config FILE`, flags, and `GOIOSVM_` environment variables; prefer `GOIOSVM_DATABASE_URL` over passing credentials on the command line. Defaults: 8 GiB image-cache budget, 2 GiB free-disk reserve, 2 GiB distinct retained source bytes in PostgreSQL. The evidence budget excludes database/index/WAL overhead. These limits are configurable using `--cache-limit`, `--disk-reserve`, and `--evidence-limit` (bytes). Credentials and firmware images are ignored by Git.

ZIP Deflate OS members are staged in a disposable temporary directory for read-only disk inspection. Staging requires the declared expanded size to fit within 8 GiB and available space after a 2 GiB reserve; both size and free space are checked again as bytes are written. Disk observations need replayable source evidence. Because a Deflate member has no direct physical mapping from decoded reads, the full encoded member is retained only when it fits the existing 32 MiB disk source-capture cap. Larger compressed members get explicit partial disk coverage without disk findings. The staging file is removed when inspection finishes or fails; the original IPSW is unaffected.

Structured output uses `--format json` and includes `schema_version`, `state` and `results`. Progress goes to stderr. Exit status: 0 complete, 2 partial, 1 failure, 130 cancellation. The current inspector reports partial coverage because filesystem-content inspection remains unimplemented.

## Verify changes

```sh
go test ./...
go vet ./...
GOIOSVM_TEST_DATABASE_URL="$GOIOSVM_DATABASE_URL" go test -race ./...
python3 scripts/check-backup.py
```

Database tests use temporary schemas and remove them afterward. The backup check streams into a temporary database and removes it; no dump archive is kept. It should run while no evidence is being published. It compares retained bytes and publication relationships and does not claim real-image qualification. The dedicated development database remains available across CLI restarts. `docker compose down` stops it without deleting its volume.
