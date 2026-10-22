# Local inspection read API

`go-ios-vm ui serve` exposes `GET` and `HEAD` routes on numeric loopback only. `GET /api/v1` lists available endpoints. Responses use a versioned envelope with `api_version: "v1"`, `state`, `read_only: true`, and either `data` or a safe `diagnostic`. There are no write routes. Artifact, target, acquisition and run requests have a three-second database deadline; finding, observation and evidence requests have a fifteen-second deadline.

| Route | Exact filters | Detail |
| --- | --- | --- |
| `/api/v1/artifacts` | `sha256` | `/api/v1/artifacts/{id}` |
| `/api/v1/targets` | `artifact_id`, `product`, `build`, `identity` | `/api/v1/targets/{id}` |
| `/api/v1/acquisitions` | `product`, `build`, `status`, `from`, `before` | `/api/v1/acquisitions/{numeric-id}` |
| `/api/v1/runs` | `target_id`, `state`, `from`, `before` | `/api/v1/runs/{id}` |
| `/api/v1/progress` | `kind` (`acquisition` or `inspection`), `state` (exact state or `terminal`) | LIVE-02 supplies per-attempt/run snapshots and events |
| `/api/v1/findings` | required `target_id`; `subject`, `property`, `component`, `basis`, `assessment` | `/api/v1/findings/{id}?target_id={id}` |
| `/api/v1/observations` | required `target_id`; `run_id`, `subject`, `property`, `component` | `/api/v1/observations/{id}?target_id={id}` |
| `/api/v1/evidence/{digest}/preview` | required `target_id`; `bytes` (1–4096, default 256) | Verified retained byte sample |

List routes accept `limit` (default 25, maximum 100) and `cursor`. Lists return `data.items` and an optional `data.next_cursor`. Pass that cursor back with the same filters to request the next page. Artifact and target lists order by ID ascending; acquisition lists order by ID descending; run lists order by start time then ID descending. The cursor anchors the next page in that order, so newly added rows before it are not repeated. A cursor is scoped to its filters and is not a snapshot of later database changes. `from` is inclusive and `before` is exclusive, both RFC 3339 timestamps. Unknown, duplicate, malformed, or overlong query values receive HTTP 400.

An artifact response gives its digest, size, and number of registered source locations. `registered_unverified` means a path was recorded when the artifact was published; it does not claim that cached IPSW bytes still exist. The API omits local cache paths. A target embeds the exact firmware identity and artifact it belongs to. Two identities in one IPSW and builds for the same product remain separate records.

An acquisition response gives the recorded source status, selected product/version/build when present, retained evidence ID and its completeness flag, and a URL origin when parseable. The API omits URL credentials, path, and query. A failed source includes `partial_reason` because its search scope is incomplete; an empty result from such an attempt does not establish absence.

The browser uses `product`, `build`, and optional `identity` to find a target. Selecting it pins `target` in the URL; `run`, `finding`, `observation`, and `evidence` identify subsequent detail. The `view` parameter selects findings or observations. Subject and property filters are prefixes. Component, basis, and assessment filters are exact. Pagination uses a cursor in the URL and always offers a return to the first page. Finding results remain scoped to the selected target across retained runs; selecting a run narrows the observation list. Refreshing a detail URL repeats the read from PostgreSQL, so source-cache deletion does not remove retained evidence.

The browser renders basis, assessment, and presence as separate labels. It shows absent, unexamined, zero, failed, and partial coverage distinctly, with recorded reasons. Source chains name the artifact, image member, coordinate and selector; a retained-byte preview is capped at 512 bytes in the browser and checked against its stored digest. The UI does not claim a completed search from an empty result list or an unfinished run.

To repeat the browser smoke flow with a local Chromium binary, set `GOIOSVM_TEST_DATABASE_URL` to the development PostgreSQL URL and run:

```sh
GOIOSVM_BROWSER=/path/to/chromium \
GOIOSVM_BROWSER_SCREENSHOT_DIR=/tmp/go-ios-vm-ui \
go test ./internal/webui -run TestBrowserDeepLinkRetainsExactEvidence -count=1 -v
```

The test creates and removes an isolated schema. It opens a deep link after deleting the fixture archive, checks the retained evidence preview and exact uint64 display, and verifies distinct absent, zero, failed, and unsupported states plus escaped long text. The optional screenshot directory receives desktop and narrow-window PNGs.

Run lists show durable run state, target, input digest/size, extractor, and planned components. A run detail adds committed component checkpoints and batch counts. `final_coverage_available` is true only when the final coverage ledger has records; `final_coverage` then gives each recorded state and reason. Running, failed, cancelled, or interrupted runs can have completed checkpoints without final coverage. Uncheckpointed work is not presented as progress, and the API does not infer a completion percentage. All run detail reads use one read-only database snapshot. Each detail caps planned components, checkpoints, coverage rows, and batch groups at 128; oversized data returns a safe service error.

Finding and observation lists require an exact target ID so identical names in different firmware identities remain separate. `subject` and `property` are prefix filters; `component` matches the observation source member recorded for the selected target. Finding lists return identity, subject, property and revision count. A finding detail includes its full immutable revision chain, typed values, supporting and contradicting references, exact assessment state and append-only history. A new revision never inherits an earlier assessment. Observation lists and details include presence and logical source coordinates. JSON typed values, including unsigned 64-bit integers, are passed through without a floating-point conversion. Finding and observation cursors are opaque and bound to the selected filters.

An evidence preview requires the target that owns an observation referencing the digest. It reads and verifies the complete retained blob's length and SHA-256 while returning at most `bytes` bytes as hex and, when valid UTF-8, text. `state: "complete"` with `declared_bytes: 0` is a verified empty blob; `state: "unavailable"` means the retained blob is incomplete. A missing or cross-target digest returns 404, while inconsistent stored bytes return a safe storage failure. Preview does not read the IPSW cache, so it remains available after cache deletion.

## Monitor local work

Open `/?screen=monitor` from the local UI. The monitor lists running and stale work separately from finished history, with filters for acquisition or inspection. Selecting a row keeps its operation ID and kind in the URL so refresh and reconnect reload the same committed history. The browser polls the LIVE-02 `.../progress/events` route. Each response also carries a current snapshot because measured counters can advance without a new phase event. An expired inspection lease is shown as stale; the browser does not change the persisted run state. Missing totals render indeterminate progress while running, and terminal states never imply a completed inspection merely because bytes were downloaded.

`GET /api/v1/progress` orders rows by committed update time then operation ID descending. It accepts exact `kind`, exact persisted `state`, or `state=terminal` for all non-running states. `state=running` includes expired-lease inspections with `effective_state: "stale"`. Its cursor anchors that order; live updates can move a row between pages, so refresh the first page for newly committed work. An acquisition attempt appears at catalog resolution, before an acquisition record exists. List rows carry measured counters, phase, safe diagnostic code, target/run ID for inspection, and product/version/build for acquisition. They omit source URLs, cache paths, credentials and leases.

The operation detail shows transferred and verified bytes or committed components and retained batches. The phase history includes a visible retention-gap marker when earlier events have expired. Inspection details link to the exact target/run and show committed checkpoints while active, then final component coverage when available. Network errors leave the last committed view visible with a connection warning. There are no browser start or cancel controls.

With `GOIOSVM_TEST_DATABASE_URL` pointing at a local test database and `GOIOSVM_BROWSER` set to Chrome or Chromium, run `go test ./internal/webui -run TestBrowserMonitorCommittedTransitions -count=1 -v`. The opt-in test uses an isolated schema and a separate browser process to check running fetch and inspection counters, stale lease display, retry, cancellation, failure, interruption, and partial final coverage. Before LIVE-02 merges, its test adapter supplies the documented detail/event envelope; in the integration stack, the same test exercises the actual LIVE-02 handler.
