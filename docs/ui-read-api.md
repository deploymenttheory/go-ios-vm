# Local inspection read API

`go-ios-vm ui serve` exposes `GET` and `HEAD` routes on numeric loopback only. `GET /api/v1` lists available endpoints. Responses use a versioned envelope with `api_version: "v1"`, `state`, `read_only: true`, and either `data` or a safe `diagnostic`. There are no write routes. Each browse request has a three-second database deadline.

| Route | Exact filters | Detail |
| --- | --- | --- |
| `/api/v1/artifacts` | `sha256` | `/api/v1/artifacts/{id}` |
| `/api/v1/targets` | `artifact_id`, `product`, `build`, `identity` | `/api/v1/targets/{id}` |
| `/api/v1/acquisitions` | `product`, `build`, `status`, `from`, `before` | `/api/v1/acquisitions/{numeric-id}` |
| `/api/v1/runs` | `target_id`, `state`, `from`, `before` | `/api/v1/runs/{id}` |

List routes accept `limit` (default 25, maximum 100) and `cursor`. Lists return `data.items` and an optional `data.next_cursor`. Pass that cursor back with the same filters to request the next page. Artifact and target lists order by ID ascending; acquisition lists order by ID descending; run lists order by start time then ID descending. The cursor anchors the next page in that order, so newly added rows before it are not repeated. A cursor is scoped to its filters and is not a snapshot of later database changes. `from` is inclusive and `before` is exclusive, both RFC 3339 timestamps. Unknown, duplicate, malformed, or overlong query values receive HTTP 400.

An artifact response gives its digest, size, and number of registered source locations. `registered_unverified` means a path was recorded when the artifact was published; it does not claim that cached IPSW bytes still exist. The API omits local cache paths. A target embeds the exact firmware identity and artifact it belongs to. Two identities in one IPSW and builds for the same product remain separate records.

An acquisition response gives the recorded source status, selected product/version/build when present, retained evidence ID and its completeness flag, and a URL origin when parseable. The API omits URL credentials, path, and query. A failed source includes `partial_reason` because its search scope is incomplete; an empty result from such an attempt does not establish absence.

Run lists show durable run state, target, input digest/size, extractor, and planned components. A run detail adds committed component checkpoints and batch counts. `final_coverage_available` is true only when the final coverage ledger has records; `final_coverage` then gives each recorded state and reason. Running, failed, cancelled, or interrupted runs can have completed checkpoints without final coverage. Uncheckpointed work is not presented as progress, and the API does not infer a completion percentage. All run detail reads use one read-only database snapshot. Each detail caps planned components, checkpoints, coverage rows, and batch groups at 128; oversized data returns a safe service error.
