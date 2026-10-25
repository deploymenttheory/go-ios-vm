# Native Go port of vphone-cli, with a GitHub implementation backlog

Plan revision: **1**. Agreed in conversation on **24 October 2026**.
Repository baseline: `cbe8cd92d5672544fb97629ab0753c3eeb2cd176`.

## Summary

Build `go-ios-vm` as a native Go application that creates, restores and runs an iOS guest with a graphical window and input.

The first supported pairing is **iPhone17,3 iOS 27.0.1 (`24A446`) + cloudOS 26.4 (`23E5207q`)**. Use vphone-cli revision `b2804adb45a653ff3981f0705108975414dc8d5d` as the porting reference. [Pinned compatibility matrix](https://github.com/Lakr233/vphone-cli/blob/b2804adb45a653ff3981f0705108975414dc8d5d/Documents/Guides/compatibility.md)

Application logic, firmware preparation, restore orchestration and the guest daemon will be Go. Explicit exceptions cover boot ROMs, Apple guest drivers and required guest hooks. Existing Swift host programs and third-party C host restore libraries are outside that exception.

Before implementation, use the `plan-to-github-issues` skill to turn this plan into a reviewed, dependency-linked backlog. The [accuracy audit](research-accuracy-audit.md) remains the evidence baseline: source claims, compilation and mocks do not prove guest compatibility.

## GitHub backlog workflow

- Target **deploymenttheory/go-ios-vm**, using stable namespace **ios-runner-v1**.
- Inventory existing open and closed issues, labels, milestones and relationships before drafting. Reuse matching work and repository conventions.
- Produce a baseline index and epics covering foundation, native hosting/window, restore, firmware preparation, guest integration and end-to-end verification. Decompose each into bounded implementation tickets.
- Give every implementation ticket the skill's eight required sections, concrete acceptance criteria, verification steps, applicable decisions and direct blockers. Preserve source revisions, firmware identities and the agreed Go/native boundary.
- Use outcome milestones: native host/DFU foundation; Go restore; prepared firmware and guest integration; complete interactive create-and-run.
- Save the complete preview in `docs/github-backlog.md`, including issue bodies, stable IDs, labels, milestone descriptions, requirement coverage and the dependency graph. Validate acyclic direct blockers and identify work ready to start.
- **Obtain approval of that concrete preview before GitHub writes**, including labels and milestones. This is an explicit requirement of the invoked skill, not an inferred permission boundary.
- Publish approved issues with native sub-issue and blocker relationships. Resolve draft references, retain the issue mapping, and read back the published backlog to verify completeness.
- Implement against published tickets. Reconcile subsequent changes by stable identity, preserving human edits and completion history rather than creating duplicates.

## Architecture and implementation

### 1. Runnable foundation

Initialize module `github.com/deploymenttheory/go-ios-vm` using Go 1.27.1. Keep packages internal initially, separating machine storage, firmware, patching, restore, native integration and guest communication.

Implement:

- `host preflight`
- `vm create`, `launch`, `stop`, `list`, `info`
- `fw prepare`, `fw patch`, and `restore`

Use `context.Context`, wrapped errors, explicit resource ownership, structured progress, durable stage records and machine process locking. Distinguish native VM state from guest readiness.

Firmware downloads belong in `/Users/dafyddwatkins/Library/Caches/go-ios-vm/ipsw`. VM bundles default to `~/Library/Application Support/go-ios-vm/machines`; both locations support overrides. Use the corresponding operating-system user directories as portable defaults, resolving to those paths on the current host.

Inspect manifests before selecting cached files. Preserve the existing iOS 14/15 images and reject them for the selected pairing.

### 2. Native virtualization and window

Use Deployment Theory's macOS bindings at revision `d5b3d0ecb48ed106ddac46095bd91ec945b2833c`. Isolate private Apple interfaces behind a small adapter using its public runtime facilities. [Pinned consumer guide](https://github.com/deploymenttheory/go-bindings-macosplatform/blob/d5b3d0ecb48ed106ddac46095bd91ec945b2833c/examples/README.md)

Port the research hardware model, persistent identity, NVRAM, SEP storage, custom ROMs, storage, PL011 serial, entropy, NAT networking, graphics, touch and keyboard configuration.

Run AppKit on the locked main thread. Submit VM operations on their configured queue and await completion outside that queue. Retain native objects and callbacks through completion.

Implement normal and forced-DFU starts, serial capture, delegate errors and shutdown. Verify native configuration and DFU discovery before relying on this layer for firmware preparation.

### 3. Go restore stack

Build the host with `CGO_ENABLED=0`. Do not invoke upstream host executables or link third-party C restore libraries.

Implement recovery transport through Apple IOKit bindings, with Go implementations of discovery, USB transfers, personalization, recovery transitions, restore messaging and filesystem transfer.

Match the created VM by ECID and expected identity. Preserve complete signing responses, propagate the first fatal error and permit one active restore at a time. Port upstream's required OpenAVB-specific behavior.

Use Go standard-library facilities alongside pinned pure-Go property-list, Mach-O and ARM64 decoding packages. Verify that host dependency paths build without CGo.

### 4. Firmware preparation and guest installation

Implement verified downloads, manifest selection, extraction and hybrid restore-tree construction for the supported pairing. Preserve original inputs and record component hashes.

Port applicable boot-chain and userspace patches into Go, including the required TXM admission change. Require expected patch matches and produce receipts; unexpected bytes stop preparation.

Extract the matching GPU bundle through a temporary cloudOS restore, then perform the hybrid restore and custom-firmware installation. Explicit calls to Apple-supplied disk, signing and sealing tools are permitted.

Port the guest daemon's initial readiness, device-information and shutdown capabilities to Go, allowing its required external linking. Inventory and pin ROMs and required native guest hooks as documented exceptions.

### 5. Complete create-and-run

Connect acquisition → preparation → patching → DFU restore → guest installation → graphical first boot.

Creation succeeds only after guest readiness and boot validation. Stop the validation VM afterward; `vm launch` opens the persistent interactive session.

Preserve failed-stage diagnostics and support restarting from validated stage boundaries. An interrupted restore must never be recorded as completed.

## Validation and acceptance

- Fixture tests cover manifest selection, malformed archives, interrupted downloads, hash failures, patch mismatches, restore failures and stage recovery.
- Native tests cover missing selectors, queue completion, cancellation with late callbacks, object lifetime, repeated lifecycle operations and GUI shutdown.
- Verify the host builds without CGo and executes without upstream host binaries or C restore libraries.
- End-to-end acceptance requires creating the selected pairing, reaching the graphical guest, demonstrating input and daemon readiness, shutting down and booting successfully again.
- Retain tool versions, firmware hashes, patch receipts and serial logs. Compilation and mocks do not satisfy boot acceptance.
- Every ticket requires completion evidence; epics additionally require integrated acceptance. Do not automatically close epics because their children are closed.

## Defaults and boundaries

- Initial host: physical Apple Silicon Mac running macOS 27+.
- Default VM: eight CPUs, 8 GiB memory, 64 GB sparse disk and NAT networking, subject to framework validation.
- Host preparation remains diagnostic and manual. Do not automatically change SIP, research-guest policy or AMFI configuration.
- Defer Launchpad, templates, snapshots, HTTP automation, package management, camera, audio and sensor parity.
- Approximately 15 GiB was free during planning. Full restore validation requires sufficient additional storage; preflight checks actual cache and working-volume requirements rather than trusting that historical observation.
- Start implementation with the foundation and native adapter. Track incomplete capabilities explicitly, with no placeholder presented as a working restore or boot.

## Execution context discovered during backlog preparation

The current local and remote `main` contain research documents only. GitHub retains earlier inspection/UI issues and other branches contain prior code. Earlier issue completion is not evidence that its implementation exists in the current checkout. The runner does not require the earlier PostgreSQL evidence store or browser UI.

Keep the earlier backlog and milestones unchanged. Reference applicable historical work as supporting material; verify and adapt any reused code to this plan's pure-Go host and exact firmware requirements. In particular, the native C decoder discussed in issue #52 is not an eligible dependency of this host.
