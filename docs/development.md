# Development

How to build and test `go-ios-vm`, and what each kind of check does and does not prove.

## Requirements

- An Apple Silicon Mac. The supported runtime is macOS 27 or later.
- Go 1.27.1, as declared in `go.mod`.
- golangci-lint v2.14.0, for `make lint` and `make check`.

## Commands

| Command | What it does |
| --- | --- |
| `make build` | Builds `bin/go-ios-vm` with `CGO_ENABLED=0`. |
| `make check` | The CI gate: gofmt, `go mod tidy -diff`, `CGO_ENABLED=0` builds of every package and of the pinned dependencies (`-tags pins`), vet, golangci-lint and the portable tests. |
| `make test` | Portable and fixture tests. |
| `make test-race` | Portable tests with the race detector. |
| `make test-native` | The native tier: nonprivileged macOS runtime checks. |
| `make test-live` | The live tier: tests that need a prepared research host. |
| `make guest` | Cross-builds the Go guest daemon for iOS with external linking (`bin/go-ios-vm-guestd`). It needs the iPhoneOS SDK. See `docs/guest-daemon.md`. |

The host binary must build with `CGO_ENABLED=0`. It must not execute vphone-cli or other upstream host programs, and must not link a third-party C restore library.

`cmd/go-ios-vm` tests enforce this. They build the binary and every pinned dependency without cgo. They also fail when a module outside the reviewed list (`allowedModules` in `cmd/go-ios-vm/main_test.go`) enters the dependency closure.

Some dependencies carry an optional cgo implementation that is excluded when cgo is off; the tests log these. One example is `blacktop/go-macho/pkg/swift`.

The guest daemon is a separate build. It is the one component allowed to use external linking.

## Test tiers

`GO_IOS_VM_TEST_TIER` selects the tier. Each tier includes the ones below it.

| Tier | Runs | Prerequisites |
| --- | --- | --- |
| `portable` (default) | Unit and fixture tests. No native framework calls, network, firmware or privileges. | None |
| `native` | Nonprivileged macOS runtime checks, such as class and selector lookup and object lifetime. | darwin/arm64 on macOS 27+, plus each test's stated prerequisites |
| `live` | Research-host checks: DFU, firmware downloads, restore tickets, restores, guest boot. | A prepared research host, plus each test's stated artifacts and authorization |

Tests opt into a tier with `testenv.Require(t, tier, prerequisites...)` (`internal/testenv`). Its behaviour:

- When the tier is not requested, the test is skipped and the skip names the variable to set.
- When the tier is requested but a prerequisite is unmet, the test **fails** with every unmet prerequisite listed. An unprepared host is never reported as a passing integration check.
- When a test proceeds, it logs `testenv: running in tier <tier>`.

`make test-native` and `make test-live` run through `scripts/test-tier.sh`. The script fails when any test fails, and also when no test ran in the requested tier. An empty or fully skipped tier therefore never reads as verified.

Live checks also need the prerequisites each test names, typically as environment variables that point at artifacts (see `testenv.FileFromEnv`).

Host preparation is manual and diagnostic. No test or command changes SIP, research-guest policy or AMFI configuration.

## Continuous integration

`.github/workflows/go.yml` runs `make check` and `make test-race` on GitHub's `macos-latest` image, with `GO_IOS_VM_TEST_TIER=portable`.

Ordinary CI never downloads IPSWs, requests restore tickets, changes host security policy or boots a guest.

The hosted image currently runs macOS 26 (`macos-26-arm64`), below the macOS 27 runtime floor. Native-tier checks therefore run on a prepared Mac, not in hosted CI. Fixture and compilation results are not evidence that the native runtime, a restore or a guest boot works.

Never commit firmware, restore tickets or signing material. `.gitignore` excludes IPSW and SHSH files.
