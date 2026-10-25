# Go guest daemon

`go-ios-vm-guestd` runs inside the iOS guest and answers the host. It ports the initial part of vphone-cli's vphoned contract at the pinned revision (`VPhoneDaemon/Daemon/APIWire.swift`, `GuestHyperTextHandler.swift`, `GuestAPI*.swift`). It implements none of vphoned's other automation methods or injected hooks.

## Protocol

HTTP/1.1 over vsock, on port 1339 as vphoned uses (`internal/guest/protocol`).

| Request | Reply |
| --- | --- |
| `GET /v1/health` | `{"status": "serving"}`. The server is up; this is not readiness. |
| `POST /v1/rpc` with `{"method", "params"?, "id"?}` | `{"type": "response", "id", "result"}`, or `{"type": "response", "id", "error": {"code", "message"}}` |

Requests are validated:
- **Size and type:** bodies are at most 1 MiB (413 otherwise) and must be `Content-Type: application/json` (415 otherwise).
- **Clients:** only local clients are served, meaning no `Origin` and a loopback `Host`, as vphoned checks (403 otherwise).
- **Shape:**
  - `method` is 1 to 128 bytes;
  - `params` is an object;
  - `id` is a string or number;
  - unknown fields and trailing data are rejected.

| Method | Result |
| --- | --- |
| `ping` | `ready`, true only when the daemon can read a complete guest identity, plus the reasons when it can't, the daemon's version, PID and start time, and the identity |
| `device.info` | `product_type` (from `hw.machine`, for example `iPhone17,3` in an iOS guest), `version` and `build` (from `SystemVersion.plist`) |
| `system.shutdown` | Requires `{"force": true}` and root. It replies `{"accepted": true}`, flushes that reply, and only then halts through `reboot3(RB2_FULLREBOOT \| RB_HALT)`, as vphoned does. A halt that fails after the acknowledgment is logged; the host sees the machine keep running and uses its bounded native stop. |

Any other method returns `unsupported_method`.

## Build

```sh
make guest   # bin/go-ios-vm-guestd
```

The guest daemon is the only component built with cgo and external linking. The build:
- targets `GOOS=ios GOARCH=arm64`;
- uses the iPhoneOS SDK's clang;
- sets a minimum OS of iOS 26.0 (`IOS_MIN`).

The result is an arm64 Mach-O executable whose `LC_BUILD_VERSION` names the iOS platform. It links only Apple system libraries: libSystem, plus CoreFoundation, Security and libresolv, which Go's TLS and resolver code pull in on Apple platforms.

`cmd/go-ios-vm` tests keep the two binaries apart: the host never imports the guest daemon, and the iOS build never imports the macOS bindings. GUEST-02 signs and packages the binary.

For local testing, `go-ios-vm-guestd -listen tcp:127.0.0.1:1339` serves the same protocol over TCP on the host.
