# Inspection architecture

Status: accepted after interview answer 20. Initial tooling is implemented; qualification and known gaps are tracked in [implementation status](implementation-status.md). This document describes the target contract, not a claim that every requirement is complete.

## Scope and first deliverable

Ship a Go inspection CLI using Cobra and Viper on macOS and Linux. Windows is out of scope. The separate Hypervisor.framework runner targets Apple Silicon macOS. Its eventual acceptance remains an interactive iOS UI, input, persistence, restart, and SIP-enabled operation.

The first tooling deliverable is one complete workflow:

1. Discover an exact requested device/release through GDMF and resolve its restore IPSW using Apple-owned catalogs where necessary.
2. Download into a managed cache, verify the archive's device/build identity, and record acquisition evidence in PostgreSQL.
3. Select a build identity; inspect components, available device-tree structure, kernel/driver metadata, and declared dependencies.
4. Inspect each finding's source, supporting bytes, interpretation and coverage from PostgreSQL, including after restarting the CLI.
5. Compare a second exact target, showing changes and unresolved matches without equating filenames or treating failed decoding as absence.

Initial qualification uses iOS 14 and 15 on matching iPhone hardware. Later qualification adds iOS 26/27 and different hardware generations. Version support is earned separately for each capability and format; an inventory result is not a claim of complete inspection or VM compatibility.

## Candidate image pair

Propose iPhone 11: `iPhone12,1`, board `n104ap`, platform `t8030`, architecture `arm64e`. That mapping appears in [blacktop's Xcode-derived device list](https://blacktop.github.io/ipsw/docs/guides/device_list/). Initial release requests are iOS 14.0 and 15.0; `18A373` and `19A346` are candidate build labels, not verified pins from this research pass.

The acquisition contract must resolve the actual Apple URL and verify each archive's BuildManifest product, board, variant and component identities. Neither URL availability nor either manifest has been verified. An unavailable requested build is an explicit acquisition result; do not silently select a different build. These are proposed inspection fixtures, not a commitment to the runner's final machine profile.

## CLI workflow

The following command families define the public interface; exact flag spelling can be refined during implementation without changing their ownership.

| Command family | User-visible result |
| --- | --- |
| `firmware discover` | Apple catalog candidates with device/version/build and source; distinguishes restore IPSWs from OTA assets |
| `firmware fetch` | An exact verified cached image, or a precise unresolved/interrupted/failed acquisition result |
| `target list` | Build identities actually present in a selected image |
| `inspect` | Target-scoped component inventory and supported static observations with component-by-component coverage |
| `finding list` / `finding show` | Queryable findings, revisions, rationale, source chain, and retained evidence |
| `compare` | Differences between two exact targets, explicit matching rules and any incomparable or unexamined fields |
| `cache status` / `cache prune` | Accounted disk use and cleanup of eligible cache entries; preview before deletion |

Human-readable and versioned structured outputs expose the same results. Structured output contains a schema version, relevant IDs, terminal state, coverage, results, and diagnostics. Progress goes to stderr, keeping structured stdout consumable. Exit 0 means the requested operation completed; exit 2 means an inspection completed only partially; exit 1 covers failure or invalid invocation; cancellation uses exit 130. Error details distinguish missing sources, unsupported formats, corrupt input, missing credentials, invalid selection and storage failures.

Commands that retain evidence require a configured PostgreSQL connection. A database failure cannot be hidden by reporting a successful durable inspection or by writing an alternative report archive. Help and configuration validation do not require a running database.

## Go package boundaries

Create packages as their implementations become necessary, not as empty placeholders.

| Package boundary | Owns | Excludes |
| --- | --- | --- |
| `internal/model` | Target identities, typed source coordinates, observations, finding revisions, applicability and coverage | Cobra, Viper, SQL, network and vendor parser types |
| `internal/acquisition` | GDMF/Apple catalog adapters, exact target resolution, managed downloads and verified cache entries | Hardware interpretation and substitution of requested identities |
| `internal/inspection` | Reader/parser adapters, target-scoped extraction, source locations and comparison | SQL schema, CLI formatting and guest execution |
| `internal/workflow` | Use cases, cancellation, batch publication and resource ownership through small consumer-defined interfaces | Codec implementations and vendor parser internals |
| `internal/postgres` | Schema/migrations, retained evidence bytes, atomic publication, queries and reference-aware cleanup | Device semantics and VM persistence |
| `cmd/go-ios-vm` | Cobra commands, Viper configuration, rendering and progress | Extraction and independently maintained finding state |

Model types use the standard library. Adapters translate dependency types at their boundary. Workflows depend on the model and the interfaces they consume; SQL and parser details remain in adapters. Cobra/Viper configuration resolves once into typed options before worker execution. Keep credentials out of provenance. A dependency-boundary check will catch forbidden imports; behavioral integration checks verify that adapters honor their contracts.

The machine-profile boundary is an explicit reviewed, versioned export. It will be designed with the complete machine/subsystem contracts before runner implementation. The runner must not depend on PostgreSQL, parser internals, experimental probes or inspection-only flags.

## Reuse and new Go work

- Reuse Cobra/Viper and suitable public firmware parser packages. [Viper's documentation](https://github.com/spf13/viper) supports Cobra flag integration; avoid sharing mutable configuration among workers.
- Use PostgreSQL through a dedicated adapter, with `pgx/v5/pgxpool` as the proposed driver. [pgx](https://pkg.go.dev/github.com/jackc/pgx/v5) documents pooling, transactions and PostgreSQL types. Reuse an existing migration engine; tern is a candidate, pending version/API verification.
- Evaluate public `blacktop/ipsw/pkg/*` packages individually on macOS and Linux. Its [metadata parser](https://github.com/blacktop/ipsw/blob/master/pkg/info/info.go) can log a device-tree failure and still return metadata without an error. Our adapter must preserve that gap explicitly. Upstream `internal/download` packages cannot be imported directly; reuse an available public client or implement a small Go adapter for the documented acquisition protocol.
- Use `go-apfs-v3` image/filesystem readers, retaining their ownership and logical file identities. Add required missing disk-image/filesystem codec capabilities there. Do not promise physical extent provenance where its public API exposes only logical reads.
- New Go code owns the common evidence model, source-coordinate integration, hardware-oriented extraction gaps, target-aware comparison, and the eventual machine contracts. Supporting Python tools use the same model and publication path.

Dependencies are pinned in `go.mod`/`go.sum`. A dedicated PostgreSQL 17.7 service runs through `compose.yaml`; pgx and tern provide database access and migrations. The implementation status records the environments actually checked.

## Source and publication lifecycle

A source owner keeps parent readers alive until borrowed image/partition/file views finish. Cancellation closes owned resources. Downloads transition through resolution, staging, verification and publication; partial files cannot appear as verified artifacts. Cache entries in use are leased against eviction. A changed source invalidates work derived from the changed bytes.

A run moves from queued to running and then to complete, partial, failed or cancelled. Capture progress is distinct from published evidence. Completed observation batches publish their supporting database evidence and references atomically. Interrupted blobs remain incomplete and cannot satisfy a finding. Only a completed coverage ledger can support a complete result.

Reruns retain their own relevant configuration and coverage while reusing identical verified evidence. Repetition is not independent corroboration. Interpretations change by linked revisions; changed evidence requires reassessing dependent findings. Unsupported or unexamined fields remain distinct from absent fields and explicit zero values.

PostgreSQL is the durable evidence store. Use a dedicated local development instance with project-owned storage and configurable local/remote connection settings. Cache eviction must leave retained evidence inspectable. Database migrations and backup/restore must preserve supporting bytes and relationships.

## Retention and resource limits

IPSW downloads and reproducible decoded components live in a managed cache; retained observations and required source bytes live in PostgreSQL. Download staging counts toward cache use. Active sources and evidence supporting retained findings are protected from routine cleanup. A lost original can prevent further analysis without destroying already retained evidence.

Expose bounded cache capacity, temporary-disk capacity, evidence capture capacity, retained-database capacity, decompression memory and concurrency. Derive initial numerical defaults from the selected images and available host resources before acquisition; show estimated required space. Never inherit limits from the former project or allocate without accounting. A limit result records incomplete coverage and a reason. It does not invent missing bytes or delete essential evidence to continue.

## Qualification and acceptance

The first release must demonstrate the complete real-image workflow on macOS and Linux, using the same selected device identity across iOS 14/15. Persist findings, restart the CLI, retrieve their evidence, compare targets, and prove disposable-cache cleanup leaves retained evidence readable. Verify PostgreSQL backup/restore with those records. Failures in decoding, acquisition, or publication must be visible at the appropriate scope.

Use small format fixtures for malformed input, source-coordinate conversion, unsigned values, cancellation and retry. Real images verify representative interoperability; synthetic fixtures isolate edge cases. Required behavior includes a parser upgrade exposing contradictory readings, a database interruption leaving no falsely complete records, and a comparison preserving unknown or incomparable fields. See the [evidence model](evidence-model.md) for the linked conformance scenarios.

Implement and qualify the model/publication contracts, acquisition and source-reader contracts, and inspection/comparison contracts as cohesive components, then qualify the integrated workflow. Progress is reported through those completed capabilities. No general-purpose campaign platform or unrelated capture framework is required by this design.

## Review boundary

The first-tooling scope, operating choices and shared understanding were accepted. Initial implementation is authorized. Concrete image availability and real-image interoperability remain qualification tasks; their outcomes must be recorded honestly. The VM runner remains an active project objective, with its full machine/subsystem design required before its implementation.
