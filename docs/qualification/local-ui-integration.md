# Integrated local UI qualification — 2026-10-24

This is the reproducible verification record for [QA-01](https://github.com/deploymenttheory/go-ios-vm/issues/48). It covers the read-only Browse, Monitor and Graph UI on the exact `iPhone12,1` public iOS 14.0/15.0 pair. It does not establish a usable VM or filesystem-wide APFS coverage.

## Pins and environment

- Consumer base: `go-ios-vm` main `1cf0c7d8c4f90ed9f2b23ebf442958da0813edd7`, plus this QA change. The Go module and extractor identities pin `go-apfs-v3` main `7566f1030c628e950564dd29b0e0fc3339cca6e4` as `v0.1.1-0.20261024140912-7566f1030c62`. This is an **unreleased pseudo-version**, not a claim about `go-apfs-v3 v0.1.0`.
- Host: macOS 27.0.1 (26A434), arm64, Go 1.27.1, Chrome 155.0.8059.85, Docker Compose PostgreSQL 17.7. The earlier [resource trials](../resource-budgets.md) measured both images on macOS and Linux against a local upstream codec composite; this UI run used upstream main on macOS only. CI provides the project Linux test gate, not a Linux real-image browser run.
- Both archives were independently verified as described in the [iOS 14](iphone12-1-ios14.md) and [iOS 15](iphone12-1-ios15.md) records. This qualification reused those complete local archives; it did not fetch them again. `inspect` hashed each complete archive before publishing. No image or retained source bytes are committed.
- PostgreSQL databases `qa48_20261024` and `qa48_restored_20261024` were isolated from the development database. The original browser process served the first on `127.0.0.1:8768`; a separate process served the restored copy on `127.0.0.1:8769`. These names and ports are local trial details, not defaults.

| Image | Archive SHA-256 | Target ID | Run ID | Findings and observations | Batches |
| --- | --- | --- | --- | ---: | ---: |
| iOS 14.0 `18A373`, identity 1, `n104ap` | `a30f2fa8d4dbdcd2a8cf0f1644de3879019ba6f8be8fba49806a9c82dec09759` | `3df055e7c606da2101bbe75b90acdbb33063deeec14c36906ad1e5a71a354f1a` | `e52c1a1da8b206dc6af961669a68967e` | 5,135 | 43 |
| iOS 15.0 `19A346`, identity 1, `n104ap` | `13d574566eec7be3b9e23ec93c2374cabf7f150be63aafd7f8479d7d162480e0` | `3b2aded7287d38bc6d86992621986a9d27c0ba9ade20266cbcabe007aec99f1d` | `99816e1d4d49cb2aa73ee39ffefa00b4` | 5,369 | 44 |

Both CLI invocations exited 2 with a versioned `partial` result. Manifest, DeviceTree, KernelCache and disk metadata were `complete`; filesystem content was `partial` because the selected bounded search and two truncated files cannot prove filesystem-wide absence. PostgreSQL counts of run observations, findings and batches matched the CLI results. The UI showed published batches while inspection was still running, followed by the same terminal run IDs, counts and coverage. The new extractor string identifies the pinned APFS module; a focused test compares that string with Go's linked build information so a later module update cannot silently retain an old version label.

## Browser and relationship checks

Headless Chrome loaded actual Browse, Monitor and Graph screens for each exact target and run. The eight final browser checks included a finding review detail and a bounded acquisition attempt. They passed again after cache cleanup, a UI restart and database restore. Chrome's rendered DOM showed the target build, partial coverage, batch count, truncation notice and selected details; fixture browser tests cover keyboard navigation and narrow layout.

For **each** image, the graph API returned 25 edges with `truncated=true` at the default neighborhood limit. Bounded expansions reached a device-tree property with a source location, a driver declaration with six dependency edges in the selected neighborhood, an APFS path and file properties, and a finding → revision → basis chain. The iOS 14/iOS 15 sampled device neighborhoods each had 19 edges. An opaque node ID from iOS 14 was rejected for the iOS 15 target with HTTP 400. Every returned edge had a stored record ID. The table and graph use that same response; client interaction tests verify their selection and sorting agree. The partial filesystem gap remained visible.

One isolated QA-only assessment marked an observed iOS 14 revision `disputed` with the reason `UI verification only; no technical conclusion`. The browser showed the review history after restore. This is a display fixture, not an assessment of the firmware. The fixture browser suite also exercised exact zero, absent, unknown, unexamined/unsupported, failed, partial and stale/crashed states. The PostgreSQL recovery tests exercised crash reconciliation and exact-input resume without invented terminal coverage; the real-image runs themselves were not deliberately crashed.

A bounded `firmware fetch` attempt for iOS 14 used an isolated cache with a 1,024-byte limit. A second process observed its `resolving` and `probing` phases in the UI. The CLI refused the 5,574,812,591-byte download before transfer; the UI and CLI both reported `failed` / `acquisition_failure`. No archive bytes were downloaded for this fixture. A separate bounded `firmware probe` completed, but only `fetch` creates a durable acquisition-progress operation.

## Cache cleanup and restore

The two original archives were preserved. For each image, `cp -c` made an APFS clone in an isolated managed cache; `cache prune --apply` listed and removed that regular digest-named copy. The QA database's two registered artifact locations were then pointed to those deleted clone paths. This avoids giving the UI an alternate live source while preserving the original archives outside the QA database. Both referenced paths were absent during the final UI and restore checks.

After stopping and restarting the UI, Browse, Monitor and Graph still loaded for both images. The first retained evidence object from each target returned `digest_verified=true` and a 256-byte preview. The SHA-256 of those preview bytes was `9f3defbe76cf917d2d1aa14ce0e31152e823cdeab84db09d608df5c913fd1bfa` for iOS 14 and `c338e235ac9ba5c2ba53eaa6391d5873941f36a6f3b10327900cb651e3d74632` for iOS 15. These are preview checksums, not archive or full evidence digests.

`python3 scripts/check-backup.py --database qa48_20261024` streamed a dump into a temporary database, compared exact evidence chunks, targets, runs, findings, revisions, basis, assessment events, acquisition records, progress events, batches and coverage, then removed that temporary database. A separate streamed restore to `qa48_restored_20261024` supported the same eight Chrome views. Original and restored UI APIs matched for selected target, run, graph, finding, observation, acquisition-progress and evidence-preview responses, including the two preview checksums. No dump file was retained.

## Repeat the checks

Use a dedicated PostgreSQL database and two preverified archives. Set `GOIOSVM_DATABASE_URL` to that database and `GOIOSVM_TEST_DATABASE_URL` for tests. Keep the original archives outside any cache used for prune verification.

```sh
go build -o bin/go-ios-vm ./cmd/go-ios-vm
bin/go-ios-vm db migrate
bin/go-ios-vm ui serve --listen 127.0.0.1:8768
bin/go-ios-vm --format json inspect --image "$IOS14" --identity 1
bin/go-ios-vm --format json inspect --image "$IOS15" --identity 1
python3 scripts/check-backup.py --database YOUR_ISOLATED_DATABASE
GOIOSVM_BROWSER=/path/to/chrome go test ./internal/webui -count=1
GOIOSVM_BROWSER=/path/to/chrome go test ./... -count=1
go test -race ./... -count=1
go vet ./...
golangci-lint run ./...
```

Run the server, inspections, and browser checks in separate processes. The `inspect` exit status 2 is expected for the documented partial content scope. The backup script now accepts an explicit source database, while its default remains `goiosvm`; it creates and removes its own restore database. The temporary source-path substitution and the manually served restored copy are specific to this isolated trial.

## Remaining limits

This verifies the requested local read-only UI for the pinned pair. The consumer pins unreleased upstream main, so a tagged `go-apfs-v3` update and CI review remain a separate release decision. The selected APFS paths do not establish whole-filesystem coverage, and the graph does not infer runtime hardware behavior or cross-image equivalence. First-tooling acceptance [#20](https://github.com/deploymenttheory/go-ios-vm/issues/20) also retains its independent comparison, resource and format-gap gates.
