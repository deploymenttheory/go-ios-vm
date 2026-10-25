# GitHub backlog preview: native Go iOS runner

**Draft awaiting approval — no GitHub writes made.** Target: `https://github.com/deploymenttheory/go-ios-vm`. Namespace: `ios-runner-v1`.

Source: [agreed implementation plan](implementation-plan.md), revision 1, SHA-256 `8ad1f8d9e932a758c411ac75d6d00c29853518a011e6cf72edb1d2248124feba`. Research baseline: `cbe8cd92d5672544fb97629ab0753c3eeb2cd176`. This complete preview includes the final proposed issue bodies and metadata; accepting a general plan did not approve an unseen issue breakdown.

## Proposed changes

- Create **34 issues**: one baseline, 6 epics and 27 implementation tickets.
- Create **11 labels**, reuse 8 existing labels unchanged, and create four outcome milestones with no due dates.
- Add **27 native parent relationships** and **41 native direct blocker relationships** after resolving database IDs.
- Update/close/reopen/delete **zero existing issues**. Preserve all 31 earlier inspection/UI issues, both existing milestones and their native relationships.
- Record no assignees, Project status fields or synthetic blocked labels. Do not publish code or implement tickets as part of the skill's publication step.

Machine-readable desired state: [backlog.json](backlog/ios-runner-v1/backlog.json). Read-only source inventory: [remote-inventory.json](backlog/ios-runner-v1/remote-inventory.json). Neither file contains authentication credentials.

## Preview validation

Run `python3 docs/backlog/ios-runner-v1/validate.py` from the repository root. The [validator](backlog/ios-runner-v1/validate.py) checks source/body fingerprints, all eight task sections, unique identities, requirement coverage, parent membership, milestone ordering, acyclic direct blockers, absence of redundant transitive edges, unchanged reused labels and the initial ready frontier. The [saved validation result](backlog/ios-runner-v1/validation.json) fingerprints this exact preview and desired-state file.

These are backlog checks. No runner, native VM, restore or graphical guest was implemented or tested by preparing this draft.

## Existing work and reconciliation

The current local and remote main are the research-only commit above. Historical feature branches and earlier merged PRs contain inspection/UI code; their existence does not put that code in this checkout. The new runner is neither a replacement browser UI nor a PostgreSQL-backed analysis tool. The existing namespace is `go-ios-vm-inspection-ui`; no exact `ios-runner-v1` issue marker was found across all paginated open/closed issues.

Issues [#9](https://github.com/deploymenttheory/go-ios-vm/issues/9) and [#23](https://github.com/deploymenttheory/go-ios-vm/issues/23) document historical firmware selection/download work and can inform FW-02, but target iPhone12,1 iOS 14/15 and are not the runner's exact-pair/cache implementation in current main. Issue [#52](https://github.com/deploymenttheory/go-ios-vm/issues/52) describes a C-based native decoder; it is supporting evidence of a limitation, not an allowed host dependency. [#34](https://github.com/deploymenttheory/go-ios-vm/issues/34), [#20](https://github.com/deploymenttheory/go-ios-vm/issues/20) and their children keep their existing scope and remain unchanged. No existing issue is proposed for adoption, obsolescence or reassignment.

Native sub-issue/blocker reads are recorded in the inventory. Publication permission exists at the repository level, but write capability is not exercised until approval. Before publishing, refresh remote state, recheck duplicate identities and resolve any new overlap instead of overwriting it.

## Metadata preview

Existing label descriptions/colors remain unchanged. `priority:P0` retains its integrity/first-tooling-acceptance meaning; runner integrity and acceptance gates use it. `priority:P1` remains required capability within the phase. The existing `component:frontend` is browser-specific and `area:ui` covers inspection result browsing, so neither is repurposed for AppKit.

| Label | Action | Color | Description |
| --- | --- | --- | --- |
| type:backlog | reuse | `#ededed` | Agreed backlog baseline |
| type:epic | reuse | `#5319e7` | Coherent behavior with integrated acceptance |
| type:task | reuse | `#0075ca` | Bounded implementation or verification work |
| priority:P0 | reuse | `#b60205` | Integrity or first-tooling acceptance gate |
| priority:P1 | reuse | `#d93f0b` | Required capability within this phase |
| area:fnd | create | `#1d76db` | Go runner foundation and machine integrity |
| area:host | create | `#0052cc` | Native research VM hosting and lifecycle |
| area:restore | create | `#5319e7` | Pure-Go virtual recovery and restore protocols |
| area:firmware | create | `#006b75` | Verified runner firmware preparation and installation |
| area:guest | create | `#0e8a16` | Go guest daemon and host communication |
| area:qa | reuse | `#fbca04` | Integrated qualification |
| component:backend | reuse | `#c5def5` | Go server, query or persistence work |
| component:verification | reuse | `#f9d0c4` | Cross-component acceptance evidence |
| component:runtime | create | `#bfd4f2` | Go native VM runtime and lifecycle |
| component:protocol | create | `#d4c5f9` | Go recovery, restore and guest protocols |
| component:firmware | create | `#c2e0c6` | Firmware format, patch and payload preparation |
| component:native-ui | create | `#bfdadc` | AppKit VM presentation and native input |
| component:infrastructure | create | `#d4c5f9` | Go build, dependency and CI infrastructure |
| component:documentation | create | `#0075ca` | Implementation and release documentation |

### New milestones

| ID / title | Scope and exit conditions |
| --- | --- |
| M1 — Runner M1 - Native host and DFU foundation | Go CLI, durable machine state, exact manifest/cache inspection, read-only host diagnostics, pinned ROM inventory, native configuration/lifecycle and AppKit window are testable. A created virtual identity is observed in DFU on a configured physical host. No claim of an installed iOS guest; unavailable live-host evidence keeps this gate open. |
| M2 — Runner M2 - Go restore | Pure-Go recovery transport, personalization, restored/FDR messaging and ASR transfer restore the selected cloudOS virtual guest without upstream executables or third-party C restore libraries. Preserve first-error and cancellation behavior and record an actual restore result on the target host. |
| M3 — Runner M3 - Prepared firmware and guest integration | The exact paired firmware is prepared with verified GPU resources, boot/userspace patch receipts and an explicitly installed Go guest daemon/pinned exceptions. Host/guest readiness and shutdown protocols are implemented; retain original artifacts and stop on mismatches. Native live evidence is distinguished from fixture coverage. |
| M4 — Runner M4 - Interactive create and repeat boot | A fresh create completes preparation, patching, DFU restore, custom-firmware installation and graphical boot with working input and daemon readiness; validation shuts the guest down and a later launch boots it again. Release documentation includes actual evidence, dependencies, native exceptions, resource needs and remaining limitations. |

All proposed epics/tasks have a milestone. The baseline targets M4. FW spans M1–M3 and targets M3 for integrated acceptance; other epic gates follow their integrated outcomes. No task is assigned before an unresolved predecessor's gate.

## Hierarchy and execution readiness

| Stable ID / issue | Kind | Milestone | Parent | Direct blockers |
| --- | --- | --- | --- | --- |
| [INDEX](#index) — go-ios-vm: implementation backlog and product baseline | baseline | M4 | None | None |
| [FND-01](#fnd-01) — [FND-01] Establish the Go module, CLI and package contracts | task | M1 | [EPIC-FND](#epic-fnd) | None |
| [FND-02](#fnd-02) — [FND-02] Persist machine identity, stage records and exclusive ownership | task | M1 | [EPIC-FND](#epic-fnd) | [FND-01](#fnd-01) |
| [FND-03](#fnd-03) — [FND-03] Diagnose host capabilities and storage without changing policy | task | M1 | [EPIC-FND](#epic-fnd) | [FND-02](#fnd-02), [HOST-01](#host-01) |
| [FND-04](#fnd-04) — [FND-04] Add Go build and test gates without privileged CI assumptions | task | M1 | [EPIC-FND](#epic-fnd) | [FND-01](#fnd-01) |
| [HOST-01](#host-01) — [HOST-01] Implement the native bridge and queue-safe lifecycle primitives | task | M1 | [EPIC-HOST](#epic-host) | [FND-01](#fnd-01) |
| [HOST-02](#host-02) — [HOST-02] Create the research VM configuration and observe forced DFU | task | M1 | [EPIC-HOST](#epic-host) | [HOST-01](#host-01), [FND-02](#fnd-02), [FW-01](#fw-01) |
| [HOST-03](#host-03) — [HOST-03] Display the guest with AppKit graphics and input | task | M1 | [EPIC-HOST](#epic-host) | [HOST-04](#host-04) |
| [HOST-04](#host-04) — [HOST-04] Own VM sessions, serial logs and stop requests | task | M1 | [EPIC-HOST](#epic-host) | [HOST-02](#host-02) |
| [REST-01](#rest-01) — [REST-01] Implement virtual recovery USB transport through IOKit | task | M2 | [EPIC-REST](#epic-rest) | [HOST-02](#host-02) |
| [REST-02](#rest-02) — [REST-02] Implement signing requests and component personalization in Go | task | M2 | [EPIC-REST](#epic-rest) | [REST-01](#rest-01), [FW-03](#fw-03) |
| [REST-03](#rest-03) — [REST-03] Implement restore-mode messaging and required service channels | task | M2 | [EPIC-REST](#epic-rest) | [REST-02](#rest-02) |
| [REST-04](#rest-04) — [REST-04] Implement ASR and restore payload transfer in Go | task | M2 | [EPIC-REST](#epic-rest) | [REST-03](#rest-03) |
| [REST-05](#rest-05) — [REST-05] Orchestrate and verify a cloudOS restore with OpenAVB | task | M2 | [EPIC-REST](#epic-rest) | [REST-04](#rest-04), [HOST-04](#host-04) |
| [FW-01](#fw-01) — [FW-01] Pin upstream port sources and native firmware exceptions | task | M1 | [EPIC-FW](#epic-fw) | None |
| [FW-02](#fw-02) — [FW-02] Acquire and inspect the exact firmware pair in the requested cache | task | M1 | [EPIC-FW](#epic-fw) | [FND-02](#fnd-02) |
| [FW-03](#fw-03) — [FW-03] Implement firmware containers and binary patch primitives | task | M2 | [EPIC-FW](#epic-fw) | [FW-02](#fw-02) |
| [FW-04](#fw-04) — [FW-04] Port the selected boot-chain patches and receipts | task | M3 | [EPIC-FW](#epic-fw) | [FW-03](#fw-03), [FW-01](#fw-01) |
| [FW-05](#fw-05) — [FW-05] Extract and cache the matching GPU resources using a temporary VM | task | M3 | [EPIC-FW](#epic-fw) | [REST-05](#rest-05) |
| [FW-06](#fw-06) — [FW-06] Prepare the hybrid restore tree for the single supported pair | task | M3 | [EPIC-FW](#epic-fw) | [FW-04](#fw-04), [FW-05](#fw-05) |
| [FW-08](#fw-08) — [FW-08] Port required userspace and dyld cache modifications | task | M3 | [EPIC-FW](#epic-fw) | [FW-03](#fw-03), [FW-01](#fw-01) |
| [FW-07](#fw-07) — [FW-07] Install custom firmware and the Go guest payload explicitly | task | M3 | [EPIC-FW](#epic-fw) | [FW-06](#fw-06), [FW-08](#fw-08), [GUEST-02](#guest-02) |
| [GUEST-01](#guest-01) — [GUEST-01] Implement the minimal guest daemon in Go | task | M3 | [EPIC-GUEST](#epic-guest) | [FND-01](#fnd-01) |
| [GUEST-02](#guest-02) — [GUEST-02] Build and package the signed guest payload and exceptions | task | M3 | [EPIC-GUEST](#epic-guest) | [GUEST-01](#guest-01), [FW-01](#fw-01) |
| [GUEST-03](#guest-03) — [GUEST-03] Connect the host to guest readiness and graceful shutdown | task | M3 | [EPIC-GUEST](#epic-guest) | [GUEST-02](#guest-02), [HOST-04](#host-04) |
| [E2E-01](#e2e-01) — [E2E-01] Compose create, launch and recovery into the Go CLI | task | M4 | [EPIC-E2E](#epic-e2e) | [FW-07](#fw-07), [GUEST-03](#guest-03), [HOST-03](#host-03), [FND-03](#fnd-03) |
| [E2E-02](#e2e-02) — [E2E-02] Prove creation, graphical input and a repeat boot on the target host | task | M4 | [EPIC-E2E](#epic-e2e) | [E2E-01](#e2e-01), [FND-04](#fnd-04) |
| [E2E-03](#e2e-03) — [E2E-03] Document and package the verified first runner release | task | M4 | [EPIC-E2E](#epic-e2e) | [E2E-02](#e2e-02) |
| [EPIC-FND](#epic-fnd) — [Epic FND] Foundation and machine integrity | epic | M1 | None | None |
| [EPIC-HOST](#epic-host) — [Epic HOST] Native research VM hosting and window | epic | M1 | None | None |
| [EPIC-REST](#epic-rest) — [Epic REST] Pure-Go virtual restore | epic | M2 | None | None |
| [EPIC-FW](#epic-fw) — [Epic FW] Verified paired firmware preparation and installation | epic | M3 | None | None |
| [EPIC-GUEST](#epic-guest) — [Epic GUEST] Go guest readiness and shutdown integration | epic | M3 | None | None |
| [EPIC-E2E](#epic-e2e) — [Epic E2E] Interactive creation and repeat-boot qualification | epic | M4 | None | None |

Ready after publication and verification: **FND-01, FW-01**. Parent membership does not itself block a task. Live acceptance additionally requires adequate storage, an authorized signed research host and the exact artifacts; no issue is marked complete merely because those checks were skipped.

The direct blocker sets above are the complete proposed native edges. Supporting/transitive context in bodies is not an additional edge. [Requirement coverage](#requirement-coverage) is reproduced in the baseline body below.

## Publication and recovery instructions

The invoked skill explicitly requires: “Ask for approval of this exact backlog before GitHub writes, including metadata writes.” Approval must cover these bodies, labels, milestones and relationships. No publishing, application implementation, credential changes, commits or pushes have been performed by preparing this preview.

After approval, read the skill's publication instructions; refresh remote identities and metadata; create only approved missing metadata and issues (baseline, epics, tasks in topological order); persist stable ID → number/database ID/URL mappings; create native sub-issues and direct blockers; and resolve every draft issue reference. Record and read back each write in the publication journal. Re-read on an ambiguous response before retrying. Unsupported relationships or a failed write leave publication explicitly incomplete; preserve successful work and pending actions.

Use the actual numeric database ID for native relationship payloads. The baseline is an index, not the native parent of every epic. Preserve unrelated relationships and checked criteria. Essential plan decisions are included in the baseline, so a plan permalink must not be fabricated while the source file is local only.

## Full proposed issue bodies

Each block below is the exact body to publish, except for replacing draft issue anchors with resolved GitHub URLs. Metadata is in the hierarchy table and backlog.json. No implementation or verification is claimed complete.

<a id="index"></a>

### go-ios-vm: implementation backlog and product baseline

<!-- weave-backlog:ios-runner-v1:v1:INDEX -->
Backlog ID: `INDEX` · Source: agreed implementation plan revision 1, 24 October 2026; local `docs/implementation-plan.md`, SHA-256 `8ad1f8d9e932a758c411ac75d6d00c29853518a011e6cf72edb1d2248124feba`. Repository baseline: `cbe8cd92d5672544fb97629ab0753c3eeb2cd176`.

## Purpose and scope

Deliver a native Go port of the selected vphone-cli create/restore/run path with a graphical iOS guest and input. The current repository main contains research only. Essential agreed decisions are reproduced here so this backlog remains usable before the local planning documents are committed/published.

Target repository: `https://github.com/deploymenttheory/go-ios-vm`. Namespace: `ios-runner-v1`. Upstream port source: `https://github.com/Lakr233/vphone-cli/tree/b2804adb45a653ff3981f0705108975414dc8d5d`. This is distinct from the existing firmware-inspection/browser-UI backlog; it does not add PostgreSQL or depend on that UI.

## Product baseline

**R01.** Port the host application to idiomatic Go; do not execute vphone-cli, vphone-vm, Swift host helpers, or third-party C restore programs/libraries.

**R02.** Target physical Apple Silicon Macs on macOS 27+ with Go 1.27.1. Pin macOS bindings to d5b3d0ecb48ed106ddac46095bd91ec945b2833c; the host must build with CGO_ENABLED=0.

**R03.** Support only iPhone17,3 iOS 27.0.1 build 24A446 plus cloudOS 26.4 build 23E5207q initially. Check manifests; never substitute another device/build or select an arbitrary latest release.

**R04.** Use the requested user cache directory, resolving here to /Users/dafyddwatkins/Library/Caches/go-ios-vm/ipsw, and a separate ~/Library/Application Support/go-ios-vm/machines library, with overrides. Preserve unrelated cached images and stable per-machine identity/NVRAM/SEP/disk state.

**R05.** Native VM state and guest readiness are different observations. Persist completed stages only after their evidence exists; failures, cancellation and incomplete work must not look successful.

**R06.** AppKit uses the locked main thread. Native VM operations use their configured queue; wait for completion outside that queue and retain callback/object ownership through late completion. Canceling a Go wait does not cancel native work.

**R07.** Restore only the created virtual device matched by ECID and expected identity, serialize active restores, retain the complete signing response, and preserve the first fatal error. An interrupted restore is not resumable at an arbitrary byte offset.

**R08.** Keep original firmware immutable, record input/output component digests and source revisions, enforce expected patch matches, and stop on identity/hash/patch disagreement.

**R09.** Port the guest daemon to Go with readiness, device-information and shutdown operations. External linking is permitted for that daemon. Pinned ROMs, Apple guest drivers and required native guest hooks are explicit documented exceptions, not a host C-library exception.

**R10.** Apple-supplied disk/signing/sealing tools may be invoked explicitly. Keep privileged disk installation explicit; host preflight is read-only and must not change SIP, research-guest policy or AMFI.

**R11.** Default to eight CPUs, 8 GiB RAM, a 64 GB sparse disk and NAT, subject to native validation. Check current space and actual staging needs; the approximately 15 GiB observed during planning is not sufficient proof that a restore can run.

**R12.** First-release acceptance is create, graphical boot, input, daemon readiness, shutdown and repeat boot. Defer Launchpad, templates, snapshots, HTTP automation, package management, camera, audio and sensor parity. Do not commit proprietary firmware or signing credentials.

Source: agreed plan revision 1; local source `docs/implementation-plan.md` with digest recorded above.

## Glossary

- **Pairing:** the exact iPhone build and cloudOS virtual-platform build, not a general OS version promise.
- **Native running:** Virtualization reports the machine running; **guest ready:** the intended Go guest daemon has answered its bounded readiness request.
- **Stage receipt:** durable completion evidence tied to input/resource/output digests; not a progress percentage.
- **Restore:** a device-specific, personalized protocol operation that may erase the selected virtual disk; not a cache download.
- **Native exception:** a pinned ROM, Apple guest driver or required guest hook. It does not authorize a Swift host program or C host restore library.
- **DFU:** the created virtual device's observed recovery endpoint, matched by ECID; not an arbitrary attached phone.

## Agreed architecture decisions

Use Go 1.27.1 and the pinned Deployment Theory binding revision, typed internal packages, context/error/progress boundaries, file-backed machine state and explicit native resource ownership. Port the restore stack to Go over Apple system APIs. AppKit uses the locked main thread and native VM completions must not be blocked by a waiting wrapper. The Go guest daemon may use iOS external linking; the allowed native exceptions remain inventoried. No runner ADR files exist at the baseline; these are agreed decisions, not invented ADR IDs.

Port source: [vphone-cli pinned revision](https://github.com/Lakr233/vphone-cli/tree/b2804adb45a653ff3981f0705108975414dc8d5d); binding evidence: [firmware/Go-host research](https://github.com/deploymenttheory/go-ios-vm/blob/cbe8cd92d5672544fb97629ab0753c3eeb2cd176/docs/firmware-analysis-research.md); confidence boundaries: [accuracy audit](https://github.com/deploymenttheory/go-ios-vm/blob/cbe8cd92d5672544fb97629ab0753c3eeb2cd176/docs/research-accuracy-audit.md). Earlier inspection issues and branch code are supporting context, not evidence of a runner already in this checkout.

## Delivery map

| Milestone | Exit conditions | Epics |
| --- | --- | --- |
| M1 — Runner M1 - Native host and DFU foundation | Go CLI, durable machine state, exact manifest/cache inspection, read-only host diagnostics, pinned ROM inventory, native configuration/lifecycle and AppKit window are testable. A created virtual identity is observed in DFU on a configured physical host. No claim of an installed iOS guest; unavailable live-host evidence keeps this gate open. | [EPIC-FND](#epic-fnd), [EPIC-HOST](#epic-host) |
| M2 — Runner M2 - Go restore | Pure-Go recovery transport, personalization, restored/FDR messaging and ASR transfer restore the selected cloudOS virtual guest without upstream executables or third-party C restore libraries. Preserve first-error and cancellation behavior and record an actual restore result on the target host. | [EPIC-REST](#epic-rest) |
| M3 — Runner M3 - Prepared firmware and guest integration | The exact paired firmware is prepared with verified GPU resources, boot/userspace patch receipts and an explicitly installed Go guest daemon/pinned exceptions. Host/guest readiness and shutdown protocols are implemented; retain original artifacts and stop on mismatches. Native live evidence is distinguished from fixture coverage. | [EPIC-FW](#epic-fw), [EPIC-GUEST](#epic-guest) |
| M4 — Runner M4 - Interactive create and repeat boot | A fresh create completes preparation, patching, DFU restore, custom-firmware installation and graphical boot with working input and daemon readiness; validation shuts the guest down and a later launch boots it again. Release documentation includes actual evidence, dependencies, native exceptions, resource needs and remaining limitations. | [EPIC-E2E](#epic-e2e) |

## Requirement coverage

| Requirement | Delivery tickets | Rule / rationale |
| --- | --- | --- |
| Native Go application and dependency/build boundary | [FND-01](#fnd-01), [FND-04](#fnd-04), [HOST-01](#host-01), [REST-01](#rest-01), [REST-05](#rest-05) | R01, R02 |
| Command tree, diagnostics, contexts and honest progress | [FND-01](#fnd-01), [FND-03](#fnd-03), [HOST-04](#host-04), [E2E-01](#e2e-01) | R05, R10 |
| User cache path, existing cache preservation and VM library | [FND-02](#fnd-02), [FW-02](#fw-02) | R04 |
| Durable stages, atomic state, stable identity and ownership | [FND-02](#fnd-02), [HOST-02](#host-02), [HOST-04](#host-04), [E2E-01](#e2e-01) | R04, R05 |
| Read-only host prerequisites and measured resources | [FND-03](#fnd-03), [E2E-02](#e2e-02) | R02, R10, R11 |
| Main-thread UI and queue/callback lifetime | [HOST-01](#host-01), [HOST-03](#host-03), [HOST-04](#host-04), [GUEST-03](#guest-03) | R06 |
| Research machine model, ROMs, NVRAM/SEP, devices and DFU | [FW-01](#fw-01), [HOST-02](#host-02), [REST-01](#rest-01) | R04, R09, R11 |
| Graphics, touch, keyboard and owned shutdown | [HOST-03](#host-03), [HOST-04](#host-04), [GUEST-03](#guest-03), [E2E-02](#e2e-02) | R06, R12 |
| Exact pair, source provenance and verified downloads | [FW-01](#fw-01), [FW-02](#fw-02), [FW-06](#fw-06) | R03, R08 |
| Containers, AEA/compression, Mach-O, device tree and ARM64 | [FW-03](#fw-03) | R01, R08 |
| Pure-Go USB, personalization, complete tickets and device matching | [REST-01](#rest-01), [REST-02](#rest-02) | R07 |
| Restored/FDR/ASR, first fatal error and serialized real restore | [REST-03](#rest-03), [REST-04](#rest-04), [REST-05](#rest-05) | R05, R07 |
| TXM and selected boot/userspace patch receipts | [FW-04](#fw-04), [FW-08](#fw-08) | R03, R08 |
| Temporary cloudOS restore and matching GPU resource extraction | [FW-05](#fw-05) | R08, R09, R10 |
| Hybrid preparation and explicit privileged installation | [FW-06](#fw-06), [FW-07](#fw-07) | R03, R08, R10 |
| Go guest daemon, external-link build and native exceptions | [GUEST-01](#guest-01), [GUEST-02](#guest-02), [FW-01](#fw-01) | R09 |
| Guest readiness/device info/shutdown and bounded host channel | [GUEST-03](#guest-03), [HOST-04](#host-04) | R05, R06, R09 |
| Create success, stopped validation VM and persistent launch | [E2E-01](#e2e-01), [E2E-02](#e2e-02) | R05, R12 |
| Failure recovery and invalidated receipts | [FND-02](#fnd-02), [FW-06](#fw-06), [FW-07](#fw-07), [REST-05](#rest-05), [E2E-01](#e2e-01) | R05, R08 |
| Fixture/native/live checks, evidence and repeat boot | [FND-04](#fnd-04), [E2E-02](#e2e-02) | R05, R12 |
| Build/sign/package guide and supported-feature claims | [FW-01](#fw-01), [GUEST-02](#guest-02), [E2E-03](#e2e-03) | R02, R09, R12 |
| Backlog identity, native hierarchy/blockers and verified publication | Explicit non-ticket scope | This preview and its publication workflow; no implementation ticket or sync service is required. |
| Launchpad/templates/snapshots/HTTP/package/camera/audio/sensors | Explicit non-ticket scope | Explicitly deferred by R12; existing inspection/UI backlog is unchanged. |

## Execution conventions

Use stable marker identity, type/priority/area/component labels, native task→epic parent relationships and only genuine native direct blockers. Baseline belongs to M4 and is not the native parent of every epic. Epics require integrated acceptance independently of child closure; no dates or assignees are imposed. P0 preserves the repository's integrity/acceptance-gate meaning; P1 is required capability in the phase.

Initially ready after approved publication: [FND-01](#fnd-01), [FW-01](#fw-01). All other task readiness comes from completed direct blockers and verified required contracts. Prior closure as not-planned is not completion evidence.

Before GitHub writes, approve the exact full preview including metadata. Publish baseline, epics, then tasks in blocker-first order, substitute stable draft references with real issue URLs, add native relationships and read everything back. Preserve IDs/journal for restart. If source documents are still unpublished, retain the full decisions and source digest here rather than creating a broken plan permalink. Do not alter the older inspection/UI issues, metadata or relationships, and do not introduce a continuous status-sync service.

---

<a id="fnd-01"></a>

### [FND-01] Establish the Go module, CLI and package contracts

<!-- weave-backlog:ios-runner-v1:v1:FND-01 -->
Backlog ID: `FND-01` · Epic: [EPIC-FND](#epic-fnd) · Product baseline: [INDEX](#index)

## Outcome

Establish the Go module, CLI and package contracts. This is planned runner work; the current main baseline contains research documents, not an existing runner implementation.

## Implementation

Initialize github.com/deploymenttheory/go-ios-vm and the command entrypoint. Define internal machine, firmware, patching, restore, host and guest boundaries using context.Context, typed progress and wrapped errors. Add the agreed command tree, --library and --ipsw-cache overrides and clear unsupported-platform diagnostics. Use standard flag parsing; reserve stdout for requested command results/progress and stderr for diagnostics. Pin pure-Go plist/Mach-O/ARM64 dependencies and the agreed native bindings. Unimplemented operations must return a nonzero unsupported/not-implemented result, never simulated success.

Porting reference: [pinned upstream component](https://github.com/Lakr233/vphone-cli/tree/b2804adb45a653ff3981f0705108975414dc8d5d/Documents/Guides). Planned package/file locations must be introduced by the implementation; do not assume historical branch code exists in the current checkout.

## Acceptance criteria

- [ ] The host command builds on darwin/arm64 with CGO_ENABLED=0 and reports its version and source pins.
- [ ] Help exposes the agreed command tree and actual default directories without claiming unimplemented operations work.
- [ ] Internal interfaces distinguish native running state, guest readiness and pipeline-stage completion.

## Verification

Planned checks: CGO_ENABLED=0 go build ./cmd/go-ios-vm; go test ./...; go vet ./.... Add command tests for help, invalid options, unsupported platform and nonzero incomplete-operation results. Verify dependency paths contain no required host CGo imports.

These are required future checks, not test results from backlog preparation. All implementation PRs must identify what was actually executed and what remains unverified.

## Dependencies

None; ready to start after the backlog publication/verification gate.

## Applicable product rules

**R01.** Port the host application to idiomatic Go; do not execute vphone-cli, vphone-vm, Swift host helpers, or third-party C restore programs/libraries.

**R02.** Target physical Apple Silicon Macs on macOS 27+ with Go 1.27.1. Pin macOS bindings to d5b3d0ecb48ed106ddac46095bd91ec945b2833c; the host must build with CGO_ENABLED=0.

**R05.** Native VM state and guest readiness are different observations. Persist completed stages only after their evidence exists; failures, cancellation and incomplete work must not look successful.

Source: agreed plan revision 1, reproduced in [INDEX](#index); local source `docs/implementation-plan.md`.

## Planning

Milestone: `M1` — Runner M1 - Native host and DFU foundation.
Priority: `priority:P0` — integrity or acceptance gate.
Component / scope owner: `component:infrastructure`; adjacent ticket outcomes are dependencies, not implicit extra scope.
Source rationale: agreed plan revision 1, FND subsystem and rules R01, R02, R05. Source SHA-256: `8ad1f8d9e932a758c411ac75d6d00c29853518a011e6cf72edb1d2248124feba`.

## Architecture decision alignment

No runner ADR files exist at the repository baseline. The agreed decisions reproduced in [INDEX](#index) govern this task (R01, R02, R05). Earlier inspection/UI ADRs are supporting historical context, not a requirement to introduce PostgreSQL or a browser UI. Source claims remain subject to the [accuracy audit](https://github.com/deploymenttheory/go-ios-vm/blob/cbe8cd92d5672544fb97629ab0753c3eeb2cd176/docs/research-accuracy-audit.md).

---

<a id="fnd-02"></a>

### [FND-02] Persist machine identity, stage records and exclusive ownership

<!-- weave-backlog:ios-runner-v1:v1:FND-02 -->
Backlog ID: `FND-02` · Epic: [EPIC-FND](#epic-fnd) · Product baseline: [INDEX](#index)

## Outcome

Persist machine identity, stage records and exclusive ownership. This is planned runner work; the current main baseline contains research documents, not an existing runner implementation.

## Implementation

Implement the file-backed machine store, versioned configuration, atomic writes, stable identifier bytes, artifact references, native-state observations and independently recorded stage receipts. Provide list/info through this store. Use an OS-held per-machine lock for mutation and running sessions; reject competing launches and incompatible or corrupt configurations instead of generating a replacement identity. Keep disk, NVRAM and SEP storage attached to the same machine. Record failed or interrupted attempts separately from verified completed boundaries.

Porting reference: [pinned upstream component](https://github.com/Lakr233/vphone-cli/tree/b2804adb45a653ff3981f0705108975414dc8d5d/Documents/Guides). Planned package/file locations must be introduced by the implementation; do not assume historical branch code exists in the current checkout.

## Acceptance criteria

- [ ] Restarting the CLI preserves machine identity and completed-stage receipts.
- [ ] Concurrent launches/mutations cannot own the same machine; process exit releases the OS lock.
- [ ] Corrupt records and missing or changed artifacts produce actionable errors and no fabricated completed stage.

## Verification

Use temporary directories and subprocess tests for competing owners, process death, interrupted atomic writes, identity reload, malformed configuration and changed artifact hashes. Do not allocate real VM disks in unit tests.

These are required future checks, not test results from backlog preparation. All implementation PRs must identify what was actually executed and what remains unverified.

## Dependencies

Direct blockers:
- [FND-01](#fnd-01) — requires its establish the go module, cli and package contracts outcome.

## Applicable product rules

**R04.** Use the requested user cache directory, resolving here to /Users/dafyddwatkins/Library/Caches/go-ios-vm/ipsw, and a separate ~/Library/Application Support/go-ios-vm/machines library, with overrides. Preserve unrelated cached images and stable per-machine identity/NVRAM/SEP/disk state.

**R05.** Native VM state and guest readiness are different observations. Persist completed stages only after their evidence exists; failures, cancellation and incomplete work must not look successful.

**R08.** Keep original firmware immutable, record input/output component digests and source revisions, enforce expected patch matches, and stop on identity/hash/patch disagreement.

Source: agreed plan revision 1, reproduced in [INDEX](#index); local source `docs/implementation-plan.md`.

## Planning

Milestone: `M1` — Runner M1 - Native host and DFU foundation.
Priority: `priority:P0` — integrity or acceptance gate.
Component / scope owner: `component:backend`; adjacent ticket outcomes are dependencies, not implicit extra scope.
Source rationale: agreed plan revision 1, FND subsystem and rules R04, R05, R08. Source SHA-256: `8ad1f8d9e932a758c411ac75d6d00c29853518a011e6cf72edb1d2248124feba`.

## Architecture decision alignment

No runner ADR files exist at the repository baseline. The agreed decisions reproduced in [INDEX](#index) govern this task (R04, R05, R08). Earlier inspection/UI ADRs are supporting historical context, not a requirement to introduce PostgreSQL or a browser UI. Source claims remain subject to the [accuracy audit](https://github.com/deploymenttheory/go-ios-vm/blob/cbe8cd92d5672544fb97629ab0753c3eeb2cd176/docs/research-accuracy-audit.md).

---

<a id="fnd-03"></a>

### [FND-03] Diagnose host capabilities and storage without changing policy

<!-- weave-backlog:ios-runner-v1:v1:FND-03 -->
Backlog ID: `FND-03` · Epic: [EPIC-FND](#epic-fnd) · Product baseline: [INDEX](#index)

## Outcome

Diagnose host capabilities and storage without changing policy. This is planned runner work; the current main baseline contains research documents, not an existing runner implementation.

## Implementation

Implement host preflight using native capability results plus OS/architecture, signature/entitlement observations, required Apple-tool availability, memory and filesystem free-space checks for the actual cache/library/scratch volumes. Make unresolved research-guest authorization explicitly unknown rather than supported. Explain required manual preparation and report measured staging requirements; do not modify host policy, install a helper, delete cached files or trigger firmware downloads.

Porting reference: [pinned upstream component](https://github.com/Lakr233/vphone-cli/tree/b2804adb45a653ff3981f0705108975414dc8d5d/Documents/Guides). Planned package/file locations must be introduced by the implementation; do not assume historical branch code exists in the current checkout.

## Acceptance criteria

- [ ] Preflight is read-only and separates supported, unsupported and unverified checks.
- [ ] The configured cache/library paths and current disk demand are reported, including the planning-host space constraint.
- [ ] Create can use its typed result to fail before destructive work when a known prerequisite is unmet.

## Verification

Inject host/tool/filesystem probes for unsupported OS, absent private API, unknown authorization, missing Apple tools, low space, shared-volume accounting and existing valid cache entries. Run a real read-only preflight and retain its observations without treating it as boot proof.

These are required future checks, not test results from backlog preparation. All implementation PRs must identify what was actually executed and what remains unverified.

## Dependencies

Direct blockers:
- [FND-02](#fnd-02) — requires its persist machine identity, stage records and exclusive ownership outcome.
- [HOST-01](#host-01) — requires its implement the native bridge and queue-safe lifecycle primitives outcome.

Supporting contracts already covered transitively: [FND-01](#fnd-01). These are context, not additional direct blocker edges.

## Applicable product rules

**R02.** Target physical Apple Silicon Macs on macOS 27+ with Go 1.27.1. Pin macOS bindings to d5b3d0ecb48ed106ddac46095bd91ec945b2833c; the host must build with CGO_ENABLED=0.

**R04.** Use the requested user cache directory, resolving here to /Users/dafyddwatkins/Library/Caches/go-ios-vm/ipsw, and a separate ~/Library/Application Support/go-ios-vm/machines library, with overrides. Preserve unrelated cached images and stable per-machine identity/NVRAM/SEP/disk state.

**R10.** Apple-supplied disk/signing/sealing tools may be invoked explicitly. Keep privileged disk installation explicit; host preflight is read-only and must not change SIP, research-guest policy or AMFI.

**R11.** Default to eight CPUs, 8 GiB RAM, a 64 GB sparse disk and NAT, subject to native validation. Check current space and actual staging needs; the approximately 15 GiB observed during planning is not sufficient proof that a restore can run.

Source: agreed plan revision 1, reproduced in [INDEX](#index); local source `docs/implementation-plan.md`.

## Planning

Milestone: `M1` — Runner M1 - Native host and DFU foundation.
Priority: `priority:P0` — integrity or acceptance gate.
Component / scope owner: `component:runtime`; adjacent ticket outcomes are dependencies, not implicit extra scope.
Source rationale: agreed plan revision 1, FND subsystem and rules R02, R04, R10, R11. Source SHA-256: `8ad1f8d9e932a758c411ac75d6d00c29853518a011e6cf72edb1d2248124feba`.

## Architecture decision alignment

No runner ADR files exist at the repository baseline. The agreed decisions reproduced in [INDEX](#index) govern this task (R02, R04, R10, R11). Earlier inspection/UI ADRs are supporting historical context, not a requirement to introduce PostgreSQL or a browser UI. Source claims remain subject to the [accuracy audit](https://github.com/deploymenttheory/go-ios-vm/blob/cbe8cd92d5672544fb97629ab0753c3eeb2cd176/docs/research-accuracy-audit.md).

---

<a id="fnd-04"></a>

### [FND-04] Add Go build and test gates without privileged CI assumptions

<!-- weave-backlog:ios-runner-v1:v1:FND-04 -->
Backlog ID: `FND-04` · Epic: [EPIC-FND](#epic-fnd) · Product baseline: [INDEX](#index)

## Outcome

Add Go build and test gates without privileged CI assumptions. This is planned runner work; the current main baseline contains research documents, not an existing runner implementation.

## Implementation

Provide repeatable build, test and vet commands and a macOS CI workflow for the Go host. Separate portable/fixture tests from native capability and opt-in live-VM tests. Make absence of an authorized research host an explicit unmet integration condition, not a passing boot test. Preserve the host CGO_ENABLED=0 build constraint; guest external-link builds are separate. Follow the current checkout rather than copying obsolete inspection CI/DB requirements.

Porting reference: [pinned upstream component](https://github.com/Lakr233/vphone-cli/tree/b2804adb45a653ff3981f0705108975414dc8d5d/Documents/Guides). Planned package/file locations must be introduced by the implementation; do not assume historical branch code exists in the current checkout.

## Acceptance criteria

- [ ] CI builds the darwin/arm64 host without CGo and runs meaningful unit/fixture tests.
- [ ] Ordinary CI never requests restore tickets, downloads IPSWs, alters host security policy or boots a guest.
- [ ] Live integration and guest-build commands have explicit prerequisites and do not claim execution when skipped.

## Verification

Run the documented Go build/test/vet commands; inspect CI configuration and dependency closure. Test that opt-in live checks fail or report unavailable prerequisites distinctly when the host/artifacts are missing.

These are required future checks, not test results from backlog preparation. All implementation PRs must identify what was actually executed and what remains unverified.

## Dependencies

Direct blockers:
- [FND-01](#fnd-01) — requires its establish the go module, cli and package contracts outcome.

## Applicable product rules

**R01.** Port the host application to idiomatic Go; do not execute vphone-cli, vphone-vm, Swift host helpers, or third-party C restore programs/libraries.

**R02.** Target physical Apple Silicon Macs on macOS 27+ with Go 1.27.1. Pin macOS bindings to d5b3d0ecb48ed106ddac46095bd91ec945b2833c; the host must build with CGO_ENABLED=0.

**R05.** Native VM state and guest readiness are different observations. Persist completed stages only after their evidence exists; failures, cancellation and incomplete work must not look successful.

**R12.** First-release acceptance is create, graphical boot, input, daemon readiness, shutdown and repeat boot. Defer Launchpad, templates, snapshots, HTTP automation, package management, camera, audio and sensor parity. Do not commit proprietary firmware or signing credentials.

Source: agreed plan revision 1, reproduced in [INDEX](#index); local source `docs/implementation-plan.md`.

## Planning

Milestone: `M1` — Runner M1 - Native host and DFU foundation.
Priority: `priority:P0` — integrity or acceptance gate.
Component / scope owner: `component:infrastructure`; adjacent ticket outcomes are dependencies, not implicit extra scope.
Source rationale: agreed plan revision 1, FND subsystem and rules R01, R02, R05, R12. Source SHA-256: `8ad1f8d9e932a758c411ac75d6d00c29853518a011e6cf72edb1d2248124feba`.

## Architecture decision alignment

No runner ADR files exist at the repository baseline. The agreed decisions reproduced in [INDEX](#index) govern this task (R01, R02, R05, R12). Earlier inspection/UI ADRs are supporting historical context, not a requirement to introduce PostgreSQL or a browser UI. Source claims remain subject to the [accuracy audit](https://github.com/deploymenttheory/go-ios-vm/blob/cbe8cd92d5672544fb97629ab0753c3eeb2cd176/docs/research-accuracy-audit.md).

---

<a id="host-01"></a>

### [HOST-01] Implement the native bridge and queue-safe lifecycle primitives

<!-- weave-backlog:ios-runner-v1:v1:HOST-01 -->
Backlog ID: `HOST-01` · Epic: [EPIC-HOST](#epic-host) · Product baseline: [INDEX](#index)

## Outcome

Implement the native bridge and queue-safe lifecycle primitives. This is planned runner work; the current main baseline contains research documents, not an existing runner implementation.

## Implementation

Use the bindings consumer/runtime APIs, not internal raw packages, for typed wrappers and the missing research/private selectors. Inventory class/selector ABI and availability against the pinned upstream host. Provide native submission, completion, error and capability primitives with explicit retained references, autorelease handling and Go callback lifetime. Never call the blocking generated Start wrapper inside the completion queue. Keep AppKit work on the locked main-thread run loop; do not assume Go recover catches NSException.

Porting reference: [pinned upstream component](https://github.com/Lakr233/vphone-cli/tree/b2804adb45a653ff3981f0705108975414dc8d5d/VPhoneExecutable/VPhoneVirtualization). Planned package/file locations must be introduced by the implementation; do not assume historical branch code exists in the current checkout.

## Acceptance criteria

- [ ] Required classes/selectors are checked before use and missing capabilities return typed errors.
- [ ] A lifecycle request returns control to its native queue before waiting and completion/cancellation cannot free live callbacks.
- [ ] The bridge builds with CGO_ENABLED=0 and does not invoke a Swift helper.

## Verification

Use a controllable serial-queue/completion fixture to detect waits on the completion queue, duplicate/late callbacks and cancellation races. On macOS run nonprivileged runtime lookup/lifetime checks; record any unavailable native operation instead of substituting success.

These are required future checks, not test results from backlog preparation. All implementation PRs must identify what was actually executed and what remains unverified.

## Dependencies

Direct blockers:
- [FND-01](#fnd-01) — requires its establish the go module, cli and package contracts outcome.

## Applicable product rules

**R01.** Port the host application to idiomatic Go; do not execute vphone-cli, vphone-vm, Swift host helpers, or third-party C restore programs/libraries.

**R02.** Target physical Apple Silicon Macs on macOS 27+ with Go 1.27.1. Pin macOS bindings to d5b3d0ecb48ed106ddac46095bd91ec945b2833c; the host must build with CGO_ENABLED=0.

**R06.** AppKit uses the locked main thread. Native VM operations use their configured queue; wait for completion outside that queue and retain callback/object ownership through late completion. Canceling a Go wait does not cancel native work.

Source: agreed plan revision 1, reproduced in [INDEX](#index); local source `docs/implementation-plan.md`.

## Planning

Milestone: `M1` — Runner M1 - Native host and DFU foundation.
Priority: `priority:P0` — integrity or acceptance gate.
Component / scope owner: `component:runtime`; adjacent ticket outcomes are dependencies, not implicit extra scope.
Source rationale: agreed plan revision 1, HOST subsystem and rules R01, R02, R06. Source SHA-256: `8ad1f8d9e932a758c411ac75d6d00c29853518a011e6cf72edb1d2248124feba`.

## Architecture decision alignment

No runner ADR files exist at the repository baseline. The agreed decisions reproduced in [INDEX](#index) govern this task (R01, R02, R06). Earlier inspection/UI ADRs are supporting historical context, not a requirement to introduce PostgreSQL or a browser UI. Source claims remain subject to the [accuracy audit](https://github.com/deploymenttheory/go-ios-vm/blob/cbe8cd92d5672544fb97629ab0753c3eeb2cd176/docs/research-accuracy-audit.md).

---

<a id="host-02"></a>

### [HOST-02] Create the research VM configuration and observe forced DFU

<!-- weave-backlog:ios-runner-v1:v1:HOST-02 -->
Backlog ID: `HOST-02` · Epic: [EPIC-HOST](#epic-host) · Product baseline: [INDEX](#index)

## Outcome

Create the research VM configuration and observe forced DFU. This is planned runner work; the current main baseline contains research documents, not an existing runner implementation.

## Implementation

Port the pinned PV research hardware model and native platform configuration, persistent machine identity, ROM overrides, NVRAM, SEP coprocessor/storage, sparse disk, PL011 pipes, entropy, vsock and NAT configuration. Validate resources through the native framework. Implement forced-DFU startup and ECID resolution, with configuration ownership and serial output tied to the machine store. Use the pinned ROM assets from FW-01; do not replace the hardware model with a public Linux/macOS guest profile.

Porting reference: [pinned upstream component](https://github.com/Lakr233/vphone-cli/tree/b2804adb45a653ff3981f0705108975414dc8d5d/VPhoneExecutable/VPhoneVirtualization). Planned package/file locations must be introduced by the implementation; do not assume historical branch code exists in the current checkout.

## Acceptance criteria

- [ ] A new configuration persists identity and correctly reuses its disk/NVRAM/SEP state on reopening.
- [ ] Native validation and startup errors are reported without marking the machine ready.
- [ ] On a configured physical host, the created VM starts in DFU and its observed ECID matches the stored identity; serial evidence is retained.

## Verification

Test configuration mapping, defaults, missing asset, invalid disk/path and identity reload with fixtures. Run an explicit live DFU spike on an authorized host, recording host signature/capabilities, ROM digests, ECID and serial result. Keep this live acceptance open if prerequisites are unavailable.

These are required future checks, not test results from backlog preparation. All implementation PRs must identify what was actually executed and what remains unverified.

## Dependencies

Direct blockers:
- [HOST-01](#host-01) — requires its implement the native bridge and queue-safe lifecycle primitives outcome.
- [FND-02](#fnd-02) — requires its persist machine identity, stage records and exclusive ownership outcome.
- [FW-01](#fw-01) — requires its pin upstream port sources and native firmware exceptions outcome.

Supporting contracts already covered transitively: [FND-01](#fnd-01). These are context, not additional direct blocker edges.

## Applicable product rules

**R02.** Target physical Apple Silicon Macs on macOS 27+ with Go 1.27.1. Pin macOS bindings to d5b3d0ecb48ed106ddac46095bd91ec945b2833c; the host must build with CGO_ENABLED=0.

**R04.** Use the requested user cache directory, resolving here to /Users/dafyddwatkins/Library/Caches/go-ios-vm/ipsw, and a separate ~/Library/Application Support/go-ios-vm/machines library, with overrides. Preserve unrelated cached images and stable per-machine identity/NVRAM/SEP/disk state.

**R06.** AppKit uses the locked main thread. Native VM operations use their configured queue; wait for completion outside that queue and retain callback/object ownership through late completion. Canceling a Go wait does not cancel native work.

**R09.** Port the guest daemon to Go with readiness, device-information and shutdown operations. External linking is permitted for that daemon. Pinned ROMs, Apple guest drivers and required native guest hooks are explicit documented exceptions, not a host C-library exception.

**R11.** Default to eight CPUs, 8 GiB RAM, a 64 GB sparse disk and NAT, subject to native validation. Check current space and actual staging needs; the approximately 15 GiB observed during planning is not sufficient proof that a restore can run.

Source: agreed plan revision 1, reproduced in [INDEX](#index); local source `docs/implementation-plan.md`.

## Planning

Milestone: `M1` — Runner M1 - Native host and DFU foundation.
Priority: `priority:P0` — integrity or acceptance gate.
Component / scope owner: `component:runtime`; adjacent ticket outcomes are dependencies, not implicit extra scope.
Source rationale: agreed plan revision 1, HOST subsystem and rules R02, R04, R06, R09, R11. Source SHA-256: `8ad1f8d9e932a758c411ac75d6d00c29853518a011e6cf72edb1d2248124feba`.

## Architecture decision alignment

No runner ADR files exist at the repository baseline. The agreed decisions reproduced in [INDEX](#index) govern this task (R02, R04, R06, R09, R11). Earlier inspection/UI ADRs are supporting historical context, not a requirement to introduce PostgreSQL or a browser UI. Source claims remain subject to the [accuracy audit](https://github.com/deploymenttheory/go-ios-vm/blob/cbe8cd92d5672544fb97629ab0753c3eeb2cd176/docs/research-accuracy-audit.md).

---

<a id="host-03"></a>

### [HOST-03] Display the guest with AppKit graphics and input

<!-- weave-backlog:ios-runner-v1:v1:HOST-03 -->
Backlog ID: `HOST-03` · Epic: [EPIC-HOST](#epic-host) · Product baseline: [INDEX](#index)

## Outcome

Display the guest with AppKit graphics and input. This is planned runner work; the current main baseline contains research documents, not an existing runner implementation.

## Implementation

Implement an AppKit window and native Virtualization view in the Go host, including the selected guest display configuration, touch/pointer mapping, keyboard and basic Home/power controls. Use the native devices from the VM configuration and maintain main-thread ownership. Connect close/quit to HOST-04 lifecycle control. Do not port Launchpad, the browser UI or camera/audio/sensor features.

Porting reference: [pinned upstream component](https://github.com/Lakr233/vphone-cli/tree/b2804adb45a653ff3981f0705108975414dc8d5d/VPhoneExecutable/VPhoneVirtualization). Planned package/file locations must be introduced by the implementation; do not assume historical branch code exists in the current checkout.

## Acceptance criteria

- [ ] The signed Go process can own a window/view and attach it to its native VM without a Swift UI executable.
- [ ] Input coordinates and keyboard events are delivered to the configured guest devices, with UI operations confined to the main thread.
- [ ] Window close/quit invokes the owned-machine stop path and leaves no abandoned session; visible iOS/input acceptance is completed in E2E-02.

## Verification

Test coordinate/scale transformations and event routing independently. On macOS verify main-thread creation, resizing, close/quit and repeated window sessions. Record native UI evidence; final guest interaction must be demonstrated by E2E-02, not inferred from these fixtures.

These are required future checks, not test results from backlog preparation. All implementation PRs must identify what was actually executed and what remains unverified.

## Dependencies

Direct blockers:
- [HOST-04](#host-04) — requires its own vm sessions, serial logs and stop requests outcome.

Supporting contracts already covered transitively: [FND-01](#fnd-01), [FND-02](#fnd-02), [FW-01](#fw-01), [HOST-01](#host-01), [HOST-02](#host-02). These are context, not additional direct blocker edges.

## Applicable product rules

**R02.** Target physical Apple Silicon Macs on macOS 27+ with Go 1.27.1. Pin macOS bindings to d5b3d0ecb48ed106ddac46095bd91ec945b2833c; the host must build with CGO_ENABLED=0.

**R06.** AppKit uses the locked main thread. Native VM operations use their configured queue; wait for completion outside that queue and retain callback/object ownership through late completion. Canceling a Go wait does not cancel native work.

**R12.** First-release acceptance is create, graphical boot, input, daemon readiness, shutdown and repeat boot. Defer Launchpad, templates, snapshots, HTTP automation, package management, camera, audio and sensor parity. Do not commit proprietary firmware or signing credentials.

Source: agreed plan revision 1, reproduced in [INDEX](#index); local source `docs/implementation-plan.md`.

## Planning

Milestone: `M1` — Runner M1 - Native host and DFU foundation.
Priority: `priority:P1` — required capability within this phase.
Component / scope owner: `component:native-ui`; adjacent ticket outcomes are dependencies, not implicit extra scope.
Source rationale: agreed plan revision 1, HOST subsystem and rules R02, R06, R12. Source SHA-256: `8ad1f8d9e932a758c411ac75d6d00c29853518a011e6cf72edb1d2248124feba`.

## Architecture decision alignment

No runner ADR files exist at the repository baseline. The agreed decisions reproduced in [INDEX](#index) govern this task (R02, R06, R12). Earlier inspection/UI ADRs are supporting historical context, not a requirement to introduce PostgreSQL or a browser UI. Source claims remain subject to the [accuracy audit](https://github.com/deploymenttheory/go-ios-vm/blob/cbe8cd92d5672544fb97629ab0753c3eeb2cd176/docs/research-accuracy-audit.md).

---

<a id="host-04"></a>

### [HOST-04] Own VM sessions, serial logs and stop requests

<!-- weave-backlog:ios-runner-v1:v1:HOST-04 -->
Backlog ID: `HOST-04` · Epic: [EPIC-HOST](#epic-host) · Product baseline: [INDEX](#index)

## Outcome

Own VM sessions, serial logs and stop requests. This is planned runner work; the current main baseline contains research documents, not an existing runner implementation.

## Implementation

Implement launch/session ownership, serial log capture, native state events and a local control path for vm stop. Keep UI/run-loop operations in an owned Go VM worker when process isolation is needed; only the project's own binary may be used. Protect the control endpoint with user-only filesystem permissions and the machine lock. Route stop through a guest-shutdown capability when available, then a bounded native-stop fallback; an unavailable guest or second interrupt can force stop. Do not signal unrelated processes or release the session while native work remains outstanding.

Porting reference: [pinned upstream component](https://github.com/Lakr233/vphone-cli/tree/b2804adb45a653ff3981f0705108975414dc8d5d/VPhoneExecutable/VPhoneVirtualization). Planned package/file locations must be introduced by the implementation; do not assume historical branch code exists in the current checkout.

## Acceptance criteria

- [ ] A second CLI can stop only the intended owned VM session.
- [ ] Serial output and native errors are retained and remain distinct from guest readiness.
- [ ] Cancellation, signals and duplicate stop requests reconcile actual native state and release callbacks/files/locks after completion.

## Verification

Use worker/control-endpoint fixtures for competing owners, stale records, duplicate stops, delayed native completion and unrelated PIDs. On a live DFU VM verify serial capture and native stop. Guest-graceful behavior is exercised with GUEST-03 and E2E-02.

These are required future checks, not test results from backlog preparation. All implementation PRs must identify what was actually executed and what remains unverified.

## Dependencies

Direct blockers:
- [HOST-02](#host-02) — requires its create the research vm configuration and observe forced dfu outcome.

Supporting contracts already covered transitively: [FND-01](#fnd-01), [FND-02](#fnd-02), [FW-01](#fw-01), [HOST-01](#host-01). These are context, not additional direct blocker edges.

## Applicable product rules

**R04.** Use the requested user cache directory, resolving here to /Users/dafyddwatkins/Library/Caches/go-ios-vm/ipsw, and a separate ~/Library/Application Support/go-ios-vm/machines library, with overrides. Preserve unrelated cached images and stable per-machine identity/NVRAM/SEP/disk state.

**R05.** Native VM state and guest readiness are different observations. Persist completed stages only after their evidence exists; failures, cancellation and incomplete work must not look successful.

**R06.** AppKit uses the locked main thread. Native VM operations use their configured queue; wait for completion outside that queue and retain callback/object ownership through late completion. Canceling a Go wait does not cancel native work.

**R10.** Apple-supplied disk/signing/sealing tools may be invoked explicitly. Keep privileged disk installation explicit; host preflight is read-only and must not change SIP, research-guest policy or AMFI.

Source: agreed plan revision 1, reproduced in [INDEX](#index); local source `docs/implementation-plan.md`.

## Planning

Milestone: `M1` — Runner M1 - Native host and DFU foundation.
Priority: `priority:P0` — integrity or acceptance gate.
Component / scope owner: `component:runtime`; adjacent ticket outcomes are dependencies, not implicit extra scope.
Source rationale: agreed plan revision 1, HOST subsystem and rules R04, R05, R06, R10. Source SHA-256: `8ad1f8d9e932a758c411ac75d6d00c29853518a011e6cf72edb1d2248124feba`.

## Architecture decision alignment

No runner ADR files exist at the repository baseline. The agreed decisions reproduced in [INDEX](#index) govern this task (R04, R05, R06, R10). Earlier inspection/UI ADRs are supporting historical context, not a requirement to introduce PostgreSQL or a browser UI. Source claims remain subject to the [accuracy audit](https://github.com/deploymenttheory/go-ios-vm/blob/cbe8cd92d5672544fb97629ab0753c3eeb2cd176/docs/research-accuracy-audit.md).

---

<a id="rest-01"></a>

### [REST-01] Implement virtual recovery USB transport through IOKit

<!-- weave-backlog:ios-runner-v1:v1:REST-01 -->
Backlog ID: `REST-01` · Epic: [EPIC-REST](#epic-rest) · Product baseline: [INDEX](#index)

## Outcome

Implement virtual recovery USB transport through IOKit. This is planned runner work; the current main baseline contains research documents, not an existing runner implementation.

## Implementation

Implement Go IOKit discovery/open/close, control and bulk transfers, bounded waits, endpoint transitions and device-info/nonces needed by the pinned virtual DFU/iBoot flow. Use public binding/runtime access to system APIs instead of gousb/libusb/libirecovery. Correlate reconnects to the runner-created ECID and expected platform identifiers; reject other attached devices.

Porting reference: [pinned upstream component](https://github.com/Lakr233/vphone-cli/tree/b2804adb45a653ff3981f0705108975414dc8d5d/VPhoneExecutable/VPhoneCommand/VPhoneRestore). Planned package/file locations must be introduced by the implementation; do not assume historical branch code exists in the current checkout.

## Acceptance criteria

- [ ] The Go transport can discover/open the created virtual DFU device and obtain the required identity/nonces.
- [ ] Transfers handle partial data, disconnect/re-enumeration, cancellation and bounded timeout without leaking IOKit resources.
- [ ] No operation selects a physical or merely first-enumerated device.

## Verification

Create protocol/IOKit call fixtures for multiple devices, incorrect ECID, short transfers, disconnects and cancellation. Verify the transport against HOST-02's live virtual DFU endpoint and record agreement with the stored ECID.

These are required future checks, not test results from backlog preparation. All implementation PRs must identify what was actually executed and what remains unverified.

## Dependencies

Direct blockers:
- [HOST-02](#host-02) — requires its create the research vm configuration and observe forced dfu outcome.

Supporting contracts already covered transitively: [FND-01](#fnd-01), [FND-02](#fnd-02), [FW-01](#fw-01), [HOST-01](#host-01). These are context, not additional direct blocker edges.

## Applicable product rules

**R01.** Port the host application to idiomatic Go; do not execute vphone-cli, vphone-vm, Swift host helpers, or third-party C restore programs/libraries.

**R02.** Target physical Apple Silicon Macs on macOS 27+ with Go 1.27.1. Pin macOS bindings to d5b3d0ecb48ed106ddac46095bd91ec945b2833c; the host must build with CGO_ENABLED=0.

**R07.** Restore only the created virtual device matched by ECID and expected identity, serialize active restores, retain the complete signing response, and preserve the first fatal error. An interrupted restore is not resumable at an arbitrary byte offset.

Source: agreed plan revision 1, reproduced in [INDEX](#index); local source `docs/implementation-plan.md`.

## Planning

Milestone: `M2` — Runner M2 - Go restore.
Priority: `priority:P0` — integrity or acceptance gate.
Component / scope owner: `component:protocol`; adjacent ticket outcomes are dependencies, not implicit extra scope.
Source rationale: agreed plan revision 1, REST subsystem and rules R01, R02, R07. Source SHA-256: `8ad1f8d9e932a758c411ac75d6d00c29853518a011e6cf72edb1d2248124feba`.

## Architecture decision alignment

No runner ADR files exist at the repository baseline. The agreed decisions reproduced in [INDEX](#index) govern this task (R01, R02, R07). Earlier inspection/UI ADRs are supporting historical context, not a requirement to introduce PostgreSQL or a browser UI. Source claims remain subject to the [accuracy audit](https://github.com/deploymenttheory/go-ios-vm/blob/cbe8cd92d5672544fb97629ab0753c3eeb2cd176/docs/research-accuracy-audit.md).

---

<a id="rest-02"></a>

### [REST-02] Implement signing requests and component personalization in Go

<!-- weave-backlog:ios-runner-v1:v1:REST-02 -->
Backlog ID: `REST-02` · Epic: [EPIC-REST](#epic-rest) · Product baseline: [INDEX](#index)

## Outcome

Implement signing requests and component personalization in Go. This is planned runner work; the current main baseline contains research documents, not an existing runner implementation.

## Implementation

Build the selected identity's TSS requests from verified manifest data and device nonces, perform the online HTTPS exchange, preserve the full response and apply IMG4 component personalization using Go container helpers. Validate response/component association; never treat PMV or an index signed boolean as authorization. Retain tickets only in the local machine work area, not Git or logs.

Porting reference: [pinned upstream component](https://github.com/Lakr233/vphone-cli/tree/b2804adb45a653ff3981f0705108975414dc8d5d/VPhoneExecutable/VPhoneCommand/VPhoneRestore). Planned package/file locations must be introduced by the implementation; do not assume historical branch code exists in the current checkout.

## Acceptance criteria

- [ ] Requests bind the exact build identity, virtual device ECID and observed nonces.
- [ ] The full response is retained and the component set is personalized without replacing it with only a bare ticket.
- [ ] Signing refusal, malformed responses, mismatched component identity and network failures return errors before upload.

## Verification

Use sanitized locally constructed plist/ASN.1 fixtures and an HTTP test server for refusal, malformed/partial responses and nonce/build mismatch. An explicit live ticket request is separate evidence and must not be claimed by passing fixtures.

These are required future checks, not test results from backlog preparation. All implementation PRs must identify what was actually executed and what remains unverified.

## Dependencies

Direct blockers:
- [REST-01](#rest-01) — requires its implement virtual recovery usb transport through iokit outcome.
- [FW-03](#fw-03) — requires its implement firmware containers and binary patch primitives outcome.

Supporting contracts already covered transitively: [FND-01](#fnd-01), [FND-02](#fnd-02), [FW-01](#fw-01), [FW-02](#fw-02), [HOST-01](#host-01), [HOST-02](#host-02). These are context, not additional direct blocker edges.

## Applicable product rules

**R03.** Support only iPhone17,3 iOS 27.0.1 build 24A446 plus cloudOS 26.4 build 23E5207q initially. Check manifests; never substitute another device/build or select an arbitrary latest release.

**R07.** Restore only the created virtual device matched by ECID and expected identity, serialize active restores, retain the complete signing response, and preserve the first fatal error. An interrupted restore is not resumable at an arbitrary byte offset.

**R08.** Keep original firmware immutable, record input/output component digests and source revisions, enforce expected patch matches, and stop on identity/hash/patch disagreement.

Source: agreed plan revision 1, reproduced in [INDEX](#index); local source `docs/implementation-plan.md`.

## Planning

Milestone: `M2` — Runner M2 - Go restore.
Priority: `priority:P0` — integrity or acceptance gate.
Component / scope owner: `component:protocol`; adjacent ticket outcomes are dependencies, not implicit extra scope.
Source rationale: agreed plan revision 1, REST subsystem and rules R03, R07, R08. Source SHA-256: `8ad1f8d9e932a758c411ac75d6d00c29853518a011e6cf72edb1d2248124feba`.

## Architecture decision alignment

No runner ADR files exist at the repository baseline. The agreed decisions reproduced in [INDEX](#index) govern this task (R03, R07, R08). Earlier inspection/UI ADRs are supporting historical context, not a requirement to introduce PostgreSQL or a browser UI. Source claims remain subject to the [accuracy audit](https://github.com/deploymenttheory/go-ios-vm/blob/cbe8cd92d5672544fb97629ab0753c3eeb2cd176/docs/research-accuracy-audit.md).

---

<a id="rest-03"></a>

### [REST-03] Implement restore-mode messaging and required service channels

<!-- weave-backlog:ios-runner-v1:v1:REST-03 -->
Backlog ID: `REST-03` · Epic: [EPIC-REST](#epic-rest) · Product baseline: [INDEX](#index)

## Outcome

Implement restore-mode messaging and required service channels. This is planned runner work; the current main baseline contains research documents, not an existing runner implementation.

## Implementation

Port the required normal/recovery-to-restored transport and service messaging to Go, including usbmux/device selection, restored plist framing, progress/status handling and the pinned workflow's FDR interactions. Dispatch file/image requests through explicit handlers for REST-04. Keep protocol state, re-enumeration and final status explicit; a progress message is not a completed restore.

Porting reference: [pinned upstream component](https://github.com/Lakr233/vphone-cli/tree/b2804adb45a653ff3981f0705108975414dc8d5d/VPhoneExecutable/VPhoneCommand/VPhoneRestore). Planned package/file locations must be introduced by the implementation; do not assume historical branch code exists in the current checkout.

## Acceptance criteria

- [ ] Go code reaches and converses with the selected virtual restore service using bounded validated message framing.
- [ ] Required FDR and status exchanges are implemented rather than silently skipped.
- [ ] Unsupported messages and fatal statuses stop the session and preserve their original cause.

## Verification

Replay synthetic protocol transcripts for fragmented/coalesced frames, service reconnects, unknown requests, FDR errors, progress and final/fatal statuses. Verify selected live service transitions as part of REST-05; fixtures do not establish FDR equivalence.

These are required future checks, not test results from backlog preparation. All implementation PRs must identify what was actually executed and what remains unverified.

## Dependencies

Direct blockers:
- [REST-02](#rest-02) — requires its implement signing requests and component personalization in go outcome.

Supporting contracts already covered transitively: [FND-01](#fnd-01), [FND-02](#fnd-02), [FW-01](#fw-01), [FW-02](#fw-02), [FW-03](#fw-03), [HOST-01](#host-01), [HOST-02](#host-02), [REST-01](#rest-01). These are context, not additional direct blocker edges.

## Applicable product rules

**R01.** Port the host application to idiomatic Go; do not execute vphone-cli, vphone-vm, Swift host helpers, or third-party C restore programs/libraries.

**R05.** Native VM state and guest readiness are different observations. Persist completed stages only after their evidence exists; failures, cancellation and incomplete work must not look successful.

**R07.** Restore only the created virtual device matched by ECID and expected identity, serialize active restores, retain the complete signing response, and preserve the first fatal error. An interrupted restore is not resumable at an arbitrary byte offset.

Source: agreed plan revision 1, reproduced in [INDEX](#index); local source `docs/implementation-plan.md`.

## Planning

Milestone: `M2` — Runner M2 - Go restore.
Priority: `priority:P0` — integrity or acceptance gate.
Component / scope owner: `component:protocol`; adjacent ticket outcomes are dependencies, not implicit extra scope.
Source rationale: agreed plan revision 1, REST subsystem and rules R01, R05, R07. Source SHA-256: `8ad1f8d9e932a758c411ac75d6d00c29853518a011e6cf72edb1d2248124feba`.

## Architecture decision alignment

No runner ADR files exist at the repository baseline. The agreed decisions reproduced in [INDEX](#index) govern this task (R01, R05, R07). Earlier inspection/UI ADRs are supporting historical context, not a requirement to introduce PostgreSQL or a browser UI. Source claims remain subject to the [accuracy audit](https://github.com/deploymenttheory/go-ios-vm/blob/cbe8cd92d5672544fb97629ab0753c3eeb2cd176/docs/research-accuracy-audit.md).

---

<a id="rest-04"></a>

### [REST-04] Implement ASR and restore payload transfer in Go

<!-- weave-backlog:ios-runner-v1:v1:REST-04 -->
Backlog ID: `REST-04` · Epic: [EPIC-REST](#epic-rest) · Product baseline: [INDEX](#index)

## Outcome

Implement ASR and restore payload transfer in Go. This is planned runner work; the current main baseline contains research documents, not an existing runner implementation.

## Implementation

Implement the selected restore's ASR validation/data transfer and restored-requested component streaming from verified extracted artifacts. Handle offsets, lengths, checksums/challenges, short writes and reconnect/failure semantics required by the pinned backend. Bound buffering and keep source bytes immutable. Integrate handlers with REST-03 without shelling out to an existing restore client.

Porting reference: [pinned upstream component](https://github.com/Lakr233/vphone-cli/tree/b2804adb45a653ff3981f0705108975414dc8d5d/VPhoneExecutable/VPhoneCommand/VPhoneRestore). Planned package/file locations must be introduced by the implementation; do not assume historical branch code exists in the current checkout.

## Acceptance criteria

- [ ] Requested components and filesystem ranges come from the selected manifest and validated local artifacts.
- [ ] Large transfers stream within bounded buffers and reject out-of-range or truncated requests.
- [ ] Cancellation or failed transfers cannot emit a successful final restore status.

## Verification

Use deterministic generated files and a simulated restore peer to verify complete bytes, challenge/range handling, short transfers, cancellation and missing artifacts. Measure buffer bounds. Full-image delivery is validated by REST-05 on a real virtual guest.

These are required future checks, not test results from backlog preparation. All implementation PRs must identify what was actually executed and what remains unverified.

## Dependencies

Direct blockers:
- [REST-03](#rest-03) — requires its implement restore-mode messaging and required service channels outcome.

Supporting contracts already covered transitively: [FND-01](#fnd-01), [FND-02](#fnd-02), [FW-01](#fw-01), [FW-02](#fw-02), [FW-03](#fw-03), [HOST-01](#host-01), [HOST-02](#host-02), [REST-01](#rest-01), [REST-02](#rest-02). These are context, not additional direct blocker edges.

## Applicable product rules

**R05.** Native VM state and guest readiness are different observations. Persist completed stages only after their evidence exists; failures, cancellation and incomplete work must not look successful.

**R07.** Restore only the created virtual device matched by ECID and expected identity, serialize active restores, retain the complete signing response, and preserve the first fatal error. An interrupted restore is not resumable at an arbitrary byte offset.

**R08.** Keep original firmware immutable, record input/output component digests and source revisions, enforce expected patch matches, and stop on identity/hash/patch disagreement.

Source: agreed plan revision 1, reproduced in [INDEX](#index); local source `docs/implementation-plan.md`.

## Planning

Milestone: `M2` — Runner M2 - Go restore.
Priority: `priority:P0` — integrity or acceptance gate.
Component / scope owner: `component:protocol`; adjacent ticket outcomes are dependencies, not implicit extra scope.
Source rationale: agreed plan revision 1, REST subsystem and rules R05, R07, R08. Source SHA-256: `8ad1f8d9e932a758c411ac75d6d00c29853518a011e6cf72edb1d2248124feba`.

## Architecture decision alignment

No runner ADR files exist at the repository baseline. The agreed decisions reproduced in [INDEX](#index) govern this task (R05, R07, R08). Earlier inspection/UI ADRs are supporting historical context, not a requirement to introduce PostgreSQL or a browser UI. Source claims remain subject to the [accuracy audit](https://github.com/deploymenttheory/go-ios-vm/blob/cbe8cd92d5672544fb97629ab0753c3eeb2cd176/docs/research-accuracy-audit.md).

---

<a id="rest-05"></a>

### [REST-05] Orchestrate and verify a cloudOS restore with OpenAVB

<!-- weave-backlog:ios-runner-v1:v1:REST-05 -->
Backlog ID: `REST-05` · Epic: [EPIC-REST](#epic-rest) · Product baseline: [INDEX](#index)

## Outcome

Orchestrate and verify a cloudOS restore with OpenAVB. This is planned runner work; the current main baseline contains research documents, not an existing runner implementation.

## Implementation

Connect DFU upload, OpenAVB patch-store handling, iBoot/recovery transitions, personalization, restore-mode messaging and ASR into one Go restore state machine. First prove the selected cloudOS image on a temporary runner-owned VM, before hybrid firmware is involved. Serialize restores, preserve the first fatal error across later messages, release session resources and publish a receipt only after the actual terminal success status. Re-enter a failed restore only at a validated whole-stage boundary.

Porting reference: [pinned upstream component](https://github.com/Lakr233/vphone-cli/tree/b2804adb45a653ff3981f0705108975414dc8d5d/VPhoneExecutable/VPhoneCommand/VPhoneRestore). Planned package/file locations must be introduced by the implementation; do not assume historical branch code exists in the current checkout.

## Acceptance criteria

- [ ] A disposable virtual machine completes the selected cloudOS restore through Go code, with status and identity evidence.
- [ ] The first fatal protocol error remains the command failure even when later messages succeed.
- [ ] Canceled/failed restores retain diagnostics and no success receipt; cleanup does not affect other devices or machines.

## Verification

Run error-injection transcripts, cancellation/reconnect cases and two simultaneous restore attempts. On an authorized adequately provisioned host run the real cloudOS restore and retain serial/protocol summaries and artifact hashes. An unavailable live run leaves acceptance incomplete.

These are required future checks, not test results from backlog preparation. All implementation PRs must identify what was actually executed and what remains unverified.

## Dependencies

Direct blockers:
- [REST-04](#rest-04) — requires its implement asr and restore payload transfer in go outcome.
- [HOST-04](#host-04) — requires its own vm sessions, serial logs and stop requests outcome.

Supporting contracts already covered transitively: [FND-01](#fnd-01), [FND-02](#fnd-02), [FW-01](#fw-01), [FW-02](#fw-02), [FW-03](#fw-03), [HOST-01](#host-01), [HOST-02](#host-02), [REST-01](#rest-01), [REST-02](#rest-02), [REST-03](#rest-03). These are context, not additional direct blocker edges.

## Applicable product rules

**R01.** Port the host application to idiomatic Go; do not execute vphone-cli, vphone-vm, Swift host helpers, or third-party C restore programs/libraries.

**R05.** Native VM state and guest readiness are different observations. Persist completed stages only after their evidence exists; failures, cancellation and incomplete work must not look successful.

**R07.** Restore only the created virtual device matched by ECID and expected identity, serialize active restores, retain the complete signing response, and preserve the first fatal error. An interrupted restore is not resumable at an arbitrary byte offset.

**R08.** Keep original firmware immutable, record input/output component digests and source revisions, enforce expected patch matches, and stop on identity/hash/patch disagreement.

Source: agreed plan revision 1, reproduced in [INDEX](#index); local source `docs/implementation-plan.md`.

## Planning

Milestone: `M2` — Runner M2 - Go restore.
Priority: `priority:P0` — integrity or acceptance gate.
Component / scope owner: `component:protocol`; adjacent ticket outcomes are dependencies, not implicit extra scope.
Source rationale: agreed plan revision 1, REST subsystem and rules R01, R05, R07, R08. Source SHA-256: `8ad1f8d9e932a758c411ac75d6d00c29853518a011e6cf72edb1d2248124feba`.

## Architecture decision alignment

No runner ADR files exist at the repository baseline. The agreed decisions reproduced in [INDEX](#index) govern this task (R01, R05, R07, R08). Earlier inspection/UI ADRs are supporting historical context, not a requirement to introduce PostgreSQL or a browser UI. Source claims remain subject to the [accuracy audit](https://github.com/deploymenttheory/go-ios-vm/blob/cbe8cd92d5672544fb97629ab0753c3eeb2cd176/docs/research-accuracy-audit.md).

---

<a id="fw-01"></a>

### [FW-01] Pin upstream port sources and native firmware exceptions

<!-- weave-backlog:ios-runner-v1:v1:FW-01 -->
Backlog ID: `FW-01` · Epic: [EPIC-FW](#epic-fw) · Product baseline: [INDEX](#index)

## Outcome

Pin upstream port sources and native firmware exceptions. This is planned runner work; the current main baseline contains research documents, not an existing runner implementation.

## Implementation

Inventory the pinned upstream host, firmware, restore and guest source surfaces required for the selected pair. Record upstream MIT attribution and the actual per-component provenance/terms without assuming one license covers all assets. Pin OpenAVB/OpenSEP resources and digests from that revision, identify the required guest hooks and Apple GPU components, and distinguish generated/built resources from user-supplied firmware. Define a local asset lock manifest and acquisition/build instructions; do not vendor proprietary IPSWs or silently reuse VPhone.bundle executables.

Porting reference: [pinned upstream component](https://github.com/Lakr233/vphone-cli/tree/b2804adb45a653ff3981f0705108975414dc8d5d/VPhoneExecutable/VPhoneCommand/FirmwarePatcher). Planned package/file locations must be introduced by the implementation; do not assume historical branch code exists in the current checkout.

## Acceptance criteria

- [ ] Every allowed native exception has a role, exact source/build pin, digest policy and provenance notice.
- [ ] ROM assets can be verified before use and absent/mismatched resources fail explicitly.
- [ ] The port inventory covers the exact supported pair and identifies deferred optional features and forbidden host dependencies.

## Verification

Check resource pins/digests against the pinned upstream resource READMEs and local asset verification fixtures. Inspect the inventory for unclassified binaries and ensure no firmware or signing material is committed. This task can begin independently of Go module work.

These are required future checks, not test results from backlog preparation. All implementation PRs must identify what was actually executed and what remains unverified.

## Dependencies

None; ready to start after the backlog publication/verification gate.

## Applicable product rules

**R01.** Port the host application to idiomatic Go; do not execute vphone-cli, vphone-vm, Swift host helpers, or third-party C restore programs/libraries.

**R03.** Support only iPhone17,3 iOS 27.0.1 build 24A446 plus cloudOS 26.4 build 23E5207q initially. Check manifests; never substitute another device/build or select an arbitrary latest release.

**R08.** Keep original firmware immutable, record input/output component digests and source revisions, enforce expected patch matches, and stop on identity/hash/patch disagreement.

**R09.** Port the guest daemon to Go with readiness, device-information and shutdown operations. External linking is permitted for that daemon. Pinned ROMs, Apple guest drivers and required native guest hooks are explicit documented exceptions, not a host C-library exception.

**R12.** First-release acceptance is create, graphical boot, input, daemon readiness, shutdown and repeat boot. Defer Launchpad, templates, snapshots, HTTP automation, package management, camera, audio and sensor parity. Do not commit proprietary firmware or signing credentials.

Source: agreed plan revision 1, reproduced in [INDEX](#index); local source `docs/implementation-plan.md`.

## Planning

Milestone: `M1` — Runner M1 - Native host and DFU foundation.
Priority: `priority:P0` — integrity or acceptance gate.
Component / scope owner: `component:firmware`; adjacent ticket outcomes are dependencies, not implicit extra scope.
Source rationale: agreed plan revision 1, FW subsystem and rules R01, R03, R08, R09, R12. Source SHA-256: `8ad1f8d9e932a758c411ac75d6d00c29853518a011e6cf72edb1d2248124feba`.

## Architecture decision alignment

No runner ADR files exist at the repository baseline. The agreed decisions reproduced in [INDEX](#index) govern this task (R01, R03, R08, R09, R12). Earlier inspection/UI ADRs are supporting historical context, not a requirement to introduce PostgreSQL or a browser UI. Source claims remain subject to the [accuracy audit](https://github.com/deploymenttheory/go-ios-vm/blob/cbe8cd92d5672544fb97629ab0753c3eeb2cd176/docs/research-accuracy-audit.md).

---

<a id="fw-02"></a>

### [FW-02] Acquire and inspect the exact firmware pair in the requested cache

<!-- weave-backlog:ios-runner-v1:v1:FW-02 -->
Backlog ID: `FW-02` · Epic: [EPIC-FW](#epic-fw) · Product baseline: [INDEX](#index)

## Outcome

Acquire and inspect the exact firmware pair in the requested cache. This is planned runner work; the current main baseline contains research documents, not an existing runner implementation.

## Implementation

Implement exact source selection from local paths/URLs or attributed metadata, bounded manifest inspection, streaming downloads to staging, size/digest checks and atomic cache publication. Record raw selection metadata, retrieval time, archive SHA-256, product/build and chosen identity. Use the user cache default and override, preserve unrelated entries, and handle object-shaped board metadata rather than inheriting the researched SDK decoding/parser bugs. Local archive filenames alone do not establish identity.

Porting reference: [pinned upstream component](https://github.com/Lakr233/vphone-cli/tree/b2804adb45a653ff3981f0705108975414dc8d5d/VPhoneExecutable/VPhoneCommand/FirmwarePatcher). Planned package/file locations must be introduced by the implementation; do not assume historical branch code exists in the current checkout.

## Acceptance criteria

- [ ] The selected iPhone/cloudOS identities resolve exactly and both local and downloaded archives are verified before becoming inputs.
- [ ] Existing iOS 14/15 cache entries remain untouched and cannot satisfy the target selection.
- [ ] Interrupted downloads, ambiguous identities, invalid manifests or checksum disagreement never publish a verified cache entry.

## Verification

Use ZIP/plist fixtures and HTTP test servers for missing/ambiguous identity, range fallback, interruption, checksum mismatch and no-space errors. Inspect the existing cache read-only. Live network/download checks remain separate from restore authorization and must not run implicitly in unit tests.

These are required future checks, not test results from backlog preparation. All implementation PRs must identify what was actually executed and what remains unverified.

## Dependencies

Direct blockers:
- [FND-02](#fnd-02) — requires its persist machine identity, stage records and exclusive ownership outcome.

Supporting contracts already covered transitively: [FND-01](#fnd-01). These are context, not additional direct blocker edges.

## Applicable product rules

**R03.** Support only iPhone17,3 iOS 27.0.1 build 24A446 plus cloudOS 26.4 build 23E5207q initially. Check manifests; never substitute another device/build or select an arbitrary latest release.

**R04.** Use the requested user cache directory, resolving here to /Users/dafyddwatkins/Library/Caches/go-ios-vm/ipsw, and a separate ~/Library/Application Support/go-ios-vm/machines library, with overrides. Preserve unrelated cached images and stable per-machine identity/NVRAM/SEP/disk state.

**R08.** Keep original firmware immutable, record input/output component digests and source revisions, enforce expected patch matches, and stop on identity/hash/patch disagreement.

**R11.** Default to eight CPUs, 8 GiB RAM, a 64 GB sparse disk and NAT, subject to native validation. Check current space and actual staging needs; the approximately 15 GiB observed during planning is not sufficient proof that a restore can run.

Source: agreed plan revision 1, reproduced in [INDEX](#index); local source `docs/implementation-plan.md`.

## Planning

Milestone: `M1` — Runner M1 - Native host and DFU foundation.
Priority: `priority:P0` — integrity or acceptance gate.
Component / scope owner: `component:backend`; adjacent ticket outcomes are dependencies, not implicit extra scope.
Source rationale: agreed plan revision 1, FW subsystem and rules R03, R04, R08, R11. Source SHA-256: `8ad1f8d9e932a758c411ac75d6d00c29853518a011e6cf72edb1d2248124feba`.

## Architecture decision alignment

No runner ADR files exist at the repository baseline. The agreed decisions reproduced in [INDEX](#index) govern this task (R03, R04, R08, R11). Earlier inspection/UI ADRs are supporting historical context, not a requirement to introduce PostgreSQL or a browser UI. Source claims remain subject to the [accuracy audit](https://github.com/deploymenttheory/go-ios-vm/blob/cbe8cd92d5672544fb97629ab0753c3eeb2cd176/docs/research-accuracy-audit.md).

---

<a id="fw-03"></a>

### [FW-03] Implement firmware containers and binary patch primitives

<!-- weave-backlog:ios-runner-v1:v1:FW-03 -->
Backlog ID: `FW-03` · Epic: [EPIC-FW](#epic-fw) · Product baseline: [INDEX](#index)

## Outcome

Implement firmware containers and binary patch primitives. This is planned runner work; the current main baseline contains research documents, not an existing runner implementation.

## Implementation

Implement the selected pair's IMG4/IM4P, compression/AEA handling, Mach-O/fileset, device-tree and ARM64 inspection/rewrite primitives needed by the pinned patch pipeline. Use pure-Go parsing/cryptography and Apple system frameworks/tools where allowed; no Capstone or C codec dependency in the host. Preserve offsets, virtual addresses, source byte identity and round-trip structure. Expose bounded match/rewrite and receipt records for boot and userspace patchers; unsupported codecs/instructions remain explicit.

Porting reference: [pinned upstream component](https://github.com/Lakr233/vphone-cli/tree/b2804adb45a653ff3981f0705108975414dc8d5d/VPhoneExecutable/VPhoneCommand/FirmwarePatcher). Planned package/file locations must be introduced by the implementation; do not assume historical branch code exists in the current checkout.

## Acceptance criteria

- [ ] Required selected-pair components can be inspected and rebuilt with verifiable structural invariants.
- [ ] Matchers enforce counts and original bytes; bounds errors and unsupported input never produce a plausible patched file.
- [ ] The complete host format/decoder dependency path builds with CGO_ENABLED=0.

## Verification

Use generated small ASN.1/plist/device-tree/Mach-O fixtures, golden instruction encodings and malformed/truncated cases. Add fuzz targets for bounded format entrypoints and prove unchanged decode/re-encode invariants where the format permits. Validate actual selected component layouts locally without adding proprietary bytes to Git.

These are required future checks, not test results from backlog preparation. All implementation PRs must identify what was actually executed and what remains unverified.

## Dependencies

Direct blockers:
- [FW-02](#fw-02) — requires its acquire and inspect the exact firmware pair in the requested cache outcome.

Supporting contracts already covered transitively: [FND-01](#fnd-01), [FND-02](#fnd-02). These are context, not additional direct blocker edges.

## Applicable product rules

**R01.** Port the host application to idiomatic Go; do not execute vphone-cli, vphone-vm, Swift host helpers, or third-party C restore programs/libraries.

**R02.** Target physical Apple Silicon Macs on macOS 27+ with Go 1.27.1. Pin macOS bindings to d5b3d0ecb48ed106ddac46095bd91ec945b2833c; the host must build with CGO_ENABLED=0.

**R08.** Keep original firmware immutable, record input/output component digests and source revisions, enforce expected patch matches, and stop on identity/hash/patch disagreement.

Source: agreed plan revision 1, reproduced in [INDEX](#index); local source `docs/implementation-plan.md`.

## Planning

Milestone: `M2` — Runner M2 - Go restore.
Priority: `priority:P0` — integrity or acceptance gate.
Component / scope owner: `component:firmware`; adjacent ticket outcomes are dependencies, not implicit extra scope.
Source rationale: agreed plan revision 1, FW subsystem and rules R01, R02, R08. Source SHA-256: `8ad1f8d9e932a758c411ac75d6d00c29853518a011e6cf72edb1d2248124feba`.

## Architecture decision alignment

No runner ADR files exist at the repository baseline. The agreed decisions reproduced in [INDEX](#index) govern this task (R01, R02, R08). Earlier inspection/UI ADRs are supporting historical context, not a requirement to introduce PostgreSQL or a browser UI. Source claims remain subject to the [accuracy audit](https://github.com/deploymenttheory/go-ios-vm/blob/cbe8cd92d5672544fb97629ab0753c3eeb2cd176/docs/research-accuracy-audit.md).

---

<a id="fw-04"></a>

### [FW-04] Port the selected boot-chain patches and receipts

<!-- weave-backlog:ios-runner-v1:v1:FW-04 -->
Backlog ID: `FW-04` · Epic: [EPIC-FW](#epic-fw) · Product baseline: [INDEX](#index)

## Outcome

Port the selected boot-chain patches and receipts. This is planned runner work; the current main baseline contains research documents, not an existing runner implementation.

## Implementation

Port the applicable pinned iBoot, kernel, device-tree, SPTM/TXM and related boot-chain changes for the selected standard pairing using FW-03 primitives. Include the TXM admission fix required by iOS 27.0.1. Preserve per-patch purpose, match preconditions, expected match counts, image/member/address evidence and original/output hashes. Keep OpenAVB patch-store serialization consistent with REST-05. Exclude other device/research-board recipes and optional feature parity.

Porting reference: [pinned upstream component](https://github.com/Lakr233/vphone-cli/tree/b2804adb45a653ff3981f0705108975414dc8d5d/VPhoneExecutable/VPhoneCommand/FirmwarePatcher). Planned package/file locations must be introduced by the implementation; do not assume historical branch code exists in the current checkout.

## Acceptance criteria

- [ ] All required selected-pair boot patches have inspectable preconditions, outputs and receipts.
- [ ] Missing/extra matches or unexpected component identities stop the patch stage without modifying original inputs.
- [ ] Generated OpenAVB patch data agrees with the restore consumer and keeps the exact boot-stage/image association.

## Verification

Use generated/sanitized pattern fixtures for single/missing/ambiguous matches and idempotence/reapplication handling. Inspect actual selected components locally and compare patch records with the pinned source; actual boot correctness remains an E2E-02 requirement.

These are required future checks, not test results from backlog preparation. All implementation PRs must identify what was actually executed and what remains unverified.

## Dependencies

Direct blockers:
- [FW-03](#fw-03) — requires its implement firmware containers and binary patch primitives outcome.
- [FW-01](#fw-01) — requires its pin upstream port sources and native firmware exceptions outcome.

Supporting contracts already covered transitively: [FND-01](#fnd-01), [FND-02](#fnd-02), [FW-02](#fw-02). These are context, not additional direct blocker edges.

## Applicable product rules

**R03.** Support only iPhone17,3 iOS 27.0.1 build 24A446 plus cloudOS 26.4 build 23E5207q initially. Check manifests; never substitute another device/build or select an arbitrary latest release.

**R08.** Keep original firmware immutable, record input/output component digests and source revisions, enforce expected patch matches, and stop on identity/hash/patch disagreement.

**R09.** Port the guest daemon to Go with readiness, device-information and shutdown operations. External linking is permitted for that daemon. Pinned ROMs, Apple guest drivers and required native guest hooks are explicit documented exceptions, not a host C-library exception.

Source: agreed plan revision 1, reproduced in [INDEX](#index); local source `docs/implementation-plan.md`.

## Planning

Milestone: `M3` — Runner M3 - Prepared firmware and guest integration.
Priority: `priority:P0` — integrity or acceptance gate.
Component / scope owner: `component:firmware`; adjacent ticket outcomes are dependencies, not implicit extra scope.
Source rationale: agreed plan revision 1, FW subsystem and rules R03, R08, R09. Source SHA-256: `8ad1f8d9e932a758c411ac75d6d00c29853518a011e6cf72edb1d2248124feba`.

## Architecture decision alignment

No runner ADR files exist at the repository baseline. The agreed decisions reproduced in [INDEX](#index) govern this task (R03, R08, R09). Earlier inspection/UI ADRs are supporting historical context, not a requirement to introduce PostgreSQL or a browser UI. Source claims remain subject to the [accuracy audit](https://github.com/deploymenttheory/go-ios-vm/blob/cbe8cd92d5672544fb97629ab0753c3eeb2cd176/docs/research-accuracy-audit.md).

---

<a id="fw-05"></a>

### [FW-05] Extract and cache the matching GPU resources using a temporary VM

<!-- weave-backlog:ios-runner-v1:v1:FW-05 -->
Backlog ID: `FW-05` · Epic: [EPIC-FW](#epic-fw) · Product baseline: [INDEX](#index)

## Outcome

Extract and cache the matching GPU resources using a temporary VM. This is planned runner work; the current main baseline contains research documents, not an existing runner implementation.

## Implementation

Use the Go cloudOS restore backend to create a temporary VM with networking disabled, restore the selected cloudOS, stop it, mount the restored System volume read-only and obtain its matching paravirtual GPU resources. Incorporate the pinned required compiler resource recorded in FW-01. Verify build/platform identity and hashes, cache by the cloudOS build and clean up temporary native sessions/mounts while preserving failure diagnostics. Permit an explicitly supplied bundle only after the same checks.

Porting reference: [pinned upstream component](https://github.com/Lakr233/vphone-cli/tree/b2804adb45a653ff3981f0705108975414dc8d5d/VPhoneExecutable/VPhoneCommand/FirmwarePatcher). Planned package/file locations must be introduced by the implementation; do not assume historical branch code exists in the current checkout.

## Acceptance criteria

- [ ] The cached driver is tied to the selected cloudOS build and usable platform requirements, not only a directory name.
- [ ] Temporary restore, readonly mount, extraction and cleanup use runner-owned resources and leave no live VM or mount behind.
- [ ] An invalid cache or supplied bundle cannot be reused as matching GPU firmware.

## Verification

Test cache mismatch, absent compiler asset, extraction failure and cleanup fault cases with fixtures. On the live host perform one real temporary restore/extraction and record provenance/hashes; a later request reuses only the verified matching bundle.

These are required future checks, not test results from backlog preparation. All implementation PRs must identify what was actually executed and what remains unverified.

## Dependencies

Direct blockers:
- [REST-05](#rest-05) — requires its orchestrate and verify a cloudos restore with openavb outcome.

Supporting contracts already covered transitively: [FND-01](#fnd-01), [FND-02](#fnd-02), [FW-01](#fw-01), [FW-02](#fw-02), [FW-03](#fw-03), [HOST-01](#host-01), [HOST-02](#host-02), [HOST-04](#host-04), [REST-01](#rest-01), [REST-02](#rest-02), [REST-03](#rest-03), [REST-04](#rest-04). These are context, not additional direct blocker edges.

## Applicable product rules

**R03.** Support only iPhone17,3 iOS 27.0.1 build 24A446 plus cloudOS 26.4 build 23E5207q initially. Check manifests; never substitute another device/build or select an arbitrary latest release.

**R04.** Use the requested user cache directory, resolving here to /Users/dafyddwatkins/Library/Caches/go-ios-vm/ipsw, and a separate ~/Library/Application Support/go-ios-vm/machines library, with overrides. Preserve unrelated cached images and stable per-machine identity/NVRAM/SEP/disk state.

**R08.** Keep original firmware immutable, record input/output component digests and source revisions, enforce expected patch matches, and stop on identity/hash/patch disagreement.

**R09.** Port the guest daemon to Go with readiness, device-information and shutdown operations. External linking is permitted for that daemon. Pinned ROMs, Apple guest drivers and required native guest hooks are explicit documented exceptions, not a host C-library exception.

**R10.** Apple-supplied disk/signing/sealing tools may be invoked explicitly. Keep privileged disk installation explicit; host preflight is read-only and must not change SIP, research-guest policy or AMFI.

Source: agreed plan revision 1, reproduced in [INDEX](#index); local source `docs/implementation-plan.md`.

## Planning

Milestone: `M3` — Runner M3 - Prepared firmware and guest integration.
Priority: `priority:P1` — required capability within this phase.
Component / scope owner: `component:firmware`; adjacent ticket outcomes are dependencies, not implicit extra scope.
Source rationale: agreed plan revision 1, FW subsystem and rules R03, R04, R08, R09, R10. Source SHA-256: `8ad1f8d9e932a758c411ac75d6d00c29853518a011e6cf72edb1d2248124feba`.

## Architecture decision alignment

No runner ADR files exist at the repository baseline. The agreed decisions reproduced in [INDEX](#index) govern this task (R03, R04, R08, R09, R10). Earlier inspection/UI ADRs are supporting historical context, not a requirement to introduce PostgreSQL or a browser UI. Source claims remain subject to the [accuracy audit](https://github.com/deploymenttheory/go-ios-vm/blob/cbe8cd92d5672544fb97629ab0753c3eeb2cd176/docs/research-accuracy-audit.md).

---

<a id="fw-06"></a>

### [FW-06] Prepare the hybrid restore tree for the single supported pair

<!-- weave-backlog:ios-runner-v1:v1:FW-06 -->
Backlog ID: `FW-06` · Epic: [EPIC-FW](#epic-fw) · Product baseline: [INDEX](#index)

## Outcome

Prepare the hybrid restore tree for the single supported pair. This is planned runner work; the current main baseline contains research documents, not an existing runner implementation.

## Implementation

Implement fw prepare and fw patch composition: select exact manifest/component identities, combine the iPhone userspace with the selected cloudOS virtual platform/restore components and verified GPU resources, and produce the patched restore directory consumed by REST-05. Preserve pristine originals, source mapping, patch receipts and stage digests. Expose the result through the internal prepared-firmware contract and durable store; never infer compatibility merely from equal version strings.

Porting reference: [pinned upstream component](https://github.com/Lakr233/vphone-cli/tree/b2804adb45a653ff3981f0705108975414dc8d5d/VPhoneExecutable/VPhoneCommand/FirmwarePatcher). Planned package/file locations must be introduced by the implementation; do not assume historical branch code exists in the current checkout.

## Acceptance criteria

- [ ] The prepared tree contains an explicit selected identity and every required component resolves to its original/patched digest and source.
- [ ] Preparation can be repeated or reused only when recorded input/resource hashes still match.
- [ ] A failed preparation leaves diagnostics and no completed stage or partially published prepared tree.

## Verification

Build synthetic component trees to verify identity joins, path selection, missing/mixed builds, stale receipts and crash-safe publication. Inspect one actual prepared tree and its manifests before invoking restore; fixture completeness is not a boot result.

These are required future checks, not test results from backlog preparation. All implementation PRs must identify what was actually executed and what remains unverified.

## Dependencies

Direct blockers:
- [FW-04](#fw-04) — requires its port the selected boot-chain patches and receipts outcome.
- [FW-05](#fw-05) — requires its extract and cache the matching gpu resources using a temporary vm outcome.

Supporting contracts already covered transitively: [FND-01](#fnd-01), [FND-02](#fnd-02), [FW-01](#fw-01), [FW-02](#fw-02), [FW-03](#fw-03), [HOST-01](#host-01), [HOST-02](#host-02), [HOST-04](#host-04), [REST-01](#rest-01), [REST-02](#rest-02), [REST-03](#rest-03), [REST-04](#rest-04), [REST-05](#rest-05). These are context, not additional direct blocker edges.

## Applicable product rules

**R03.** Support only iPhone17,3 iOS 27.0.1 build 24A446 plus cloudOS 26.4 build 23E5207q initially. Check manifests; never substitute another device/build or select an arbitrary latest release.

**R05.** Native VM state and guest readiness are different observations. Persist completed stages only after their evidence exists; failures, cancellation and incomplete work must not look successful.

**R08.** Keep original firmware immutable, record input/output component digests and source revisions, enforce expected patch matches, and stop on identity/hash/patch disagreement.

Source: agreed plan revision 1, reproduced in [INDEX](#index); local source `docs/implementation-plan.md`.

## Planning

Milestone: `M3` — Runner M3 - Prepared firmware and guest integration.
Priority: `priority:P0` — integrity or acceptance gate.
Component / scope owner: `component:firmware`; adjacent ticket outcomes are dependencies, not implicit extra scope.
Source rationale: agreed plan revision 1, FW subsystem and rules R03, R05, R08. Source SHA-256: `8ad1f8d9e932a758c411ac75d6d00c29853518a011e6cf72edb1d2248124feba`.

## Architecture decision alignment

No runner ADR files exist at the repository baseline. The agreed decisions reproduced in [INDEX](#index) govern this task (R03, R05, R08). Earlier inspection/UI ADRs are supporting historical context, not a requirement to introduce PostgreSQL or a browser UI. Source claims remain subject to the [accuracy audit](https://github.com/deploymenttheory/go-ios-vm/blob/cbe8cd92d5672544fb97629ab0753c3eeb2cd176/docs/research-accuracy-audit.md).

---

<a id="fw-08"></a>

### [FW-08] Port required userspace and dyld cache modifications

<!-- weave-backlog:ios-runner-v1:v1:FW-08 -->
Backlog ID: `FW-08` · Epic: [EPIC-FW](#epic-fw) · Product baseline: [INDEX](#index)

## Outcome

Port required userspace and dyld cache modifications. This is planned runner work; the current main baseline contains research documents, not an existing runner implementation.

## Implementation

Port the pinned selected-pair userspace/custom-firmware edits, including applicable dyld-cache mapping/platform, launch/service and GPU installation prerequisites. Use source-backed byte/layout checks from FW-03 and the guest-hook inventory from FW-01; do not copy unrelated TV/watch/camera/audio/sensor changes. Produce deterministic patch plans and receipts for FW-07, with explicit assumptions about code signing, entitlements and retained native hooks.

Porting reference: [pinned upstream component](https://github.com/Lakr233/vphone-cli/tree/b2804adb45a653ff3981f0705108975414dc8d5d/VPhoneExecutable/VPhoneCommand/FirmwarePatcher). Planned package/file locations must be introduced by the implementation; do not assume historical branch code exists in the current checkout.

## Acceptance criteria

- [ ] Every selected required userspace change has a source mapping, checked target and original/output digest.
- [ ] Missing anchors, incompatible cache layout or a changed hook resource stops installation before publishing a modified system.
- [ ] Deferred optional upstream features remain absent from the claimed first-release capability set.

## Verification

Test dyld/Mach-O offset and identity checks with generated fixtures plus wrong-layout and missing-anchor cases. Inspect actual selected cache/library edits locally and compare their intended effects with pinned upstream source; validate launch/services through E2E-02.

These are required future checks, not test results from backlog preparation. All implementation PRs must identify what was actually executed and what remains unverified.

## Dependencies

Direct blockers:
- [FW-03](#fw-03) — requires its implement firmware containers and binary patch primitives outcome.
- [FW-01](#fw-01) — requires its pin upstream port sources and native firmware exceptions outcome.

Supporting contracts already covered transitively: [FND-01](#fnd-01), [FND-02](#fnd-02), [FW-02](#fw-02). These are context, not additional direct blocker edges.

## Applicable product rules

**R03.** Support only iPhone17,3 iOS 27.0.1 build 24A446 plus cloudOS 26.4 build 23E5207q initially. Check manifests; never substitute another device/build or select an arbitrary latest release.

**R08.** Keep original firmware immutable, record input/output component digests and source revisions, enforce expected patch matches, and stop on identity/hash/patch disagreement.

**R09.** Port the guest daemon to Go with readiness, device-information and shutdown operations. External linking is permitted for that daemon. Pinned ROMs, Apple guest drivers and required native guest hooks are explicit documented exceptions, not a host C-library exception.

**R12.** First-release acceptance is create, graphical boot, input, daemon readiness, shutdown and repeat boot. Defer Launchpad, templates, snapshots, HTTP automation, package management, camera, audio and sensor parity. Do not commit proprietary firmware or signing credentials.

Source: agreed plan revision 1, reproduced in [INDEX](#index); local source `docs/implementation-plan.md`.

## Planning

Milestone: `M3` — Runner M3 - Prepared firmware and guest integration.
Priority: `priority:P0` — integrity or acceptance gate.
Component / scope owner: `component:firmware`; adjacent ticket outcomes are dependencies, not implicit extra scope.
Source rationale: agreed plan revision 1, FW subsystem and rules R03, R08, R09, R12. Source SHA-256: `8ad1f8d9e932a758c411ac75d6d00c29853518a011e6cf72edb1d2248124feba`.

## Architecture decision alignment

No runner ADR files exist at the repository baseline. The agreed decisions reproduced in [INDEX](#index) govern this task (R03, R08, R09, R12). Earlier inspection/UI ADRs are supporting historical context, not a requirement to introduce PostgreSQL or a browser UI. Source claims remain subject to the [accuracy audit](https://github.com/deploymenttheory/go-ios-vm/blob/cbe8cd92d5672544fb97629ab0753c3eeb2cd176/docs/research-accuracy-audit.md).

---

<a id="fw-07"></a>

### [FW-07] Install custom firmware and the Go guest payload explicitly

<!-- weave-backlog:ios-runner-v1:v1:FW-07 -->
Backlog ID: `FW-07` · Epic: [EPIC-FW](#epic-fw) · Product baseline: [INDEX](#index)

## Outcome

Install custom firmware and the Go guest payload explicitly. This is planned runner work; the current main baseline contains research documents, not an existing runner implementation.

## Implementation

Implement the post-hybrid-restore offline custom-firmware installation using the prepared patch plans, Go guest payload and pinned native exceptions. Require the VM stopped and exclusively locked, verify the disk/NVRAM/SEP association, and invoke Apple mounting/signing/sealing tools with explicit arguments and privilege boundaries. Track mounted volumes and scratch artifacts for cleanup. Mark installation complete only after verification and unmount; retain originals/receipts and fail clearly without inventing rollback guarantees.

Porting reference: [pinned upstream component](https://github.com/Lakr233/vphone-cli/tree/b2804adb45a653ff3981f0705108975414dc8d5d/VPhoneExecutable/VPhoneCommand/FirmwarePatcher). Planned package/file locations must be introduced by the implementation; do not assume historical branch code exists in the current checkout.

## Acceptance criteria

- [ ] Only an explicitly selected stopped machine is modified, with the selected guest payload and patch receipts.
- [ ] Privileged operations are explicit and limited to disk installation; host SIP/research/AMFI policy remains untouched.
- [ ] Installation failure, unmount failure or changed inputs produces an incomplete/failed stage with recoverable diagnostics.

## Verification

Test the tool runner with mount/sign/seal/unmount failures, argument/path handling, wrong machine identity, concurrent ownership and changed payloads. Perform an explicit installation on a disposable restored VM and retain verification evidence for E2E-02.

These are required future checks, not test results from backlog preparation. All implementation PRs must identify what was actually executed and what remains unverified.

## Dependencies

Direct blockers:
- [FW-06](#fw-06) — requires its prepare the hybrid restore tree for the single supported pair outcome.
- [FW-08](#fw-08) — requires its port required userspace and dyld cache modifications outcome.
- [GUEST-02](#guest-02) — requires its build and package the signed guest payload and exceptions outcome.

Supporting contracts already covered transitively: [FND-01](#fnd-01), [FND-02](#fnd-02), [FW-01](#fw-01), [FW-02](#fw-02), [FW-03](#fw-03), [FW-04](#fw-04), [FW-05](#fw-05), [GUEST-01](#guest-01), [HOST-01](#host-01), [HOST-02](#host-02), [HOST-04](#host-04), [REST-01](#rest-01), [REST-02](#rest-02), [REST-03](#rest-03), [REST-04](#rest-04), [REST-05](#rest-05). These are context, not additional direct blocker edges.

## Applicable product rules

**R04.** Use the requested user cache directory, resolving here to /Users/dafyddwatkins/Library/Caches/go-ios-vm/ipsw, and a separate ~/Library/Application Support/go-ios-vm/machines library, with overrides. Preserve unrelated cached images and stable per-machine identity/NVRAM/SEP/disk state.

**R05.** Native VM state and guest readiness are different observations. Persist completed stages only after their evidence exists; failures, cancellation and incomplete work must not look successful.

**R08.** Keep original firmware immutable, record input/output component digests and source revisions, enforce expected patch matches, and stop on identity/hash/patch disagreement.

**R09.** Port the guest daemon to Go with readiness, device-information and shutdown operations. External linking is permitted for that daemon. Pinned ROMs, Apple guest drivers and required native guest hooks are explicit documented exceptions, not a host C-library exception.

**R10.** Apple-supplied disk/signing/sealing tools may be invoked explicitly. Keep privileged disk installation explicit; host preflight is read-only and must not change SIP, research-guest policy or AMFI.

Source: agreed plan revision 1, reproduced in [INDEX](#index); local source `docs/implementation-plan.md`.

## Planning

Milestone: `M3` — Runner M3 - Prepared firmware and guest integration.
Priority: `priority:P0` — integrity or acceptance gate.
Component / scope owner: `component:firmware`; adjacent ticket outcomes are dependencies, not implicit extra scope.
Source rationale: agreed plan revision 1, FW subsystem and rules R04, R05, R08, R09, R10. Source SHA-256: `8ad1f8d9e932a758c411ac75d6d00c29853518a011e6cf72edb1d2248124feba`.

## Architecture decision alignment

No runner ADR files exist at the repository baseline. The agreed decisions reproduced in [INDEX](#index) govern this task (R04, R05, R08, R09, R10). Earlier inspection/UI ADRs are supporting historical context, not a requirement to introduce PostgreSQL or a browser UI. Source claims remain subject to the [accuracy audit](https://github.com/deploymenttheory/go-ios-vm/blob/cbe8cd92d5672544fb97629ab0753c3eeb2cd176/docs/research-accuracy-audit.md).

---

<a id="guest-01"></a>

### [GUEST-01] Implement the minimal guest daemon in Go

<!-- weave-backlog:ios-runner-v1:v1:GUEST-01 -->
Backlog ID: `GUEST-01` · Epic: [EPIC-GUEST](#epic-guest) · Product baseline: [INDEX](#index)

## Outcome

Implement the minimal guest daemon in Go. This is planned runner work; the current main baseline contains research documents, not an existing runner implementation.

## Implementation

Port the initial host/guest daemon contract from pinned VPhoneDaemon sources: transport/framing, readiness ping, device information and graceful shutdown. Implement it in Go with bounded requests and explicit unsupported-method errors. Isolate guest-only platform calls from the host bindings; allow the required iOS external linking without treating the macOS binding set as an iOS SDK. Do not claim the full vphoned automation API or injected-hook parity.

Porting reference: [pinned upstream component](https://github.com/Lakr233/vphone-cli/tree/b2804adb45a653ff3981f0705108975414dc8d5d/VPhoneDaemon). Planned package/file locations must be introduced by the implementation; do not assume historical branch code exists in the current checkout.

## Acceptance criteria

- [ ] The Go daemon implements the agreed minimal protocol and reports the actual guest identity/version.
- [ ] Malformed/unsupported requests and shutdown failures return explicit errors; readiness is not a hardcoded response.
- [ ] Guest-specific builds do not introduce C restore/runtime libraries into the host.

## Verification

Use protocol fixtures for framing, request validation, unsupported operations and shutdown acknowledgment/error ordering. Cross-build the guest target with the required Apple toolchain and record linkage/platform metadata; runtime execution remains part of E2E-02.

These are required future checks, not test results from backlog preparation. All implementation PRs must identify what was actually executed and what remains unverified.

## Dependencies

Direct blockers:
- [FND-01](#fnd-01) — requires its establish the go module, cli and package contracts outcome.

## Applicable product rules

**R01.** Port the host application to idiomatic Go; do not execute vphone-cli, vphone-vm, Swift host helpers, or third-party C restore programs/libraries.

**R05.** Native VM state and guest readiness are different observations. Persist completed stages only after their evidence exists; failures, cancellation and incomplete work must not look successful.

**R09.** Port the guest daemon to Go with readiness, device-information and shutdown operations. External linking is permitted for that daemon. Pinned ROMs, Apple guest drivers and required native guest hooks are explicit documented exceptions, not a host C-library exception.

**R12.** First-release acceptance is create, graphical boot, input, daemon readiness, shutdown and repeat boot. Defer Launchpad, templates, snapshots, HTTP automation, package management, camera, audio and sensor parity. Do not commit proprietary firmware or signing credentials.

Source: agreed plan revision 1, reproduced in [INDEX](#index); local source `docs/implementation-plan.md`.

## Planning

Milestone: `M3` — Runner M3 - Prepared firmware and guest integration.
Priority: `priority:P1` — required capability within this phase.
Component / scope owner: `component:protocol`; adjacent ticket outcomes are dependencies, not implicit extra scope.
Source rationale: agreed plan revision 1, GUEST subsystem and rules R01, R05, R09, R12. Source SHA-256: `8ad1f8d9e932a758c411ac75d6d00c29853518a011e6cf72edb1d2248124feba`.

## Architecture decision alignment

No runner ADR files exist at the repository baseline. The agreed decisions reproduced in [INDEX](#index) govern this task (R01, R05, R09, R12). Earlier inspection/UI ADRs are supporting historical context, not a requirement to introduce PostgreSQL or a browser UI. Source claims remain subject to the [accuracy audit](https://github.com/deploymenttheory/go-ios-vm/blob/cbe8cd92d5672544fb97629ab0753c3eeb2cd176/docs/research-accuracy-audit.md).

---

<a id="guest-02"></a>

### [GUEST-02] Build and package the signed guest payload and exceptions

<!-- weave-backlog:ios-runner-v1:v1:GUEST-02 -->
Backlog ID: `GUEST-02` · Epic: [EPIC-GUEST](#epic-guest) · Product baseline: [INDEX](#index)

## Outcome

Build and package the signed guest payload and exceptions. This is planned runner work; the current main baseline contains research documents, not an existing runner implementation.

## Implementation

Produce the Go daemon installation payload, launchd configuration and required signing/entitlement/trust resources for the selected guest. Use the pinned native-hook/ROM exception inventory and separate their source/build/digest records from the Go daemon. Permit external linking for the iOS daemon and explicit Apple signing tools; do not silently package the upstream Swift daemon. Supply FW-07 with a checked payload manifest.

Porting reference: [pinned upstream component](https://github.com/Lakr233/vphone-cli/tree/b2804adb45a653ff3981f0705108975414dc8d5d/VPhoneDaemon). Planned package/file locations must be introduced by the implementation; do not assume historical branch code exists in the current checkout.

## Acceptance criteria

- [ ] Payload metadata identifies the Go guest executable, target platform, launch configuration, signing requirements and exact native exceptions.
- [ ] Missing or mismatched hooks/signing assets fail packaging and cannot be hidden by a valid daemon hash.
- [ ] No proprietary firmware, private signing credential or upstream Swift daemon is committed or bundled as an undisclosed fallback.

## Verification

Verify built Mach-O platform/architecture and payload digests, test missing-resource and mismatched-signature paths, and inspect launchd/trust configuration against the pinned recipe. Record actual build commands and toolchain; do not substitute successful signing for runtime daemon validation.

These are required future checks, not test results from backlog preparation. All implementation PRs must identify what was actually executed and what remains unverified.

## Dependencies

Direct blockers:
- [GUEST-01](#guest-01) — requires its implement the minimal guest daemon in go outcome.
- [FW-01](#fw-01) — requires its pin upstream port sources and native firmware exceptions outcome.

Supporting contracts already covered transitively: [FND-01](#fnd-01). These are context, not additional direct blocker edges.

## Applicable product rules

**R08.** Keep original firmware immutable, record input/output component digests and source revisions, enforce expected patch matches, and stop on identity/hash/patch disagreement.

**R09.** Port the guest daemon to Go with readiness, device-information and shutdown operations. External linking is permitted for that daemon. Pinned ROMs, Apple guest drivers and required native guest hooks are explicit documented exceptions, not a host C-library exception.

**R10.** Apple-supplied disk/signing/sealing tools may be invoked explicitly. Keep privileged disk installation explicit; host preflight is read-only and must not change SIP, research-guest policy or AMFI.

**R12.** First-release acceptance is create, graphical boot, input, daemon readiness, shutdown and repeat boot. Defer Launchpad, templates, snapshots, HTTP automation, package management, camera, audio and sensor parity. Do not commit proprietary firmware or signing credentials.

Source: agreed plan revision 1, reproduced in [INDEX](#index); local source `docs/implementation-plan.md`.

## Planning

Milestone: `M3` — Runner M3 - Prepared firmware and guest integration.
Priority: `priority:P0` — integrity or acceptance gate.
Component / scope owner: `component:firmware`; adjacent ticket outcomes are dependencies, not implicit extra scope.
Source rationale: agreed plan revision 1, GUEST subsystem and rules R08, R09, R10, R12. Source SHA-256: `8ad1f8d9e932a758c411ac75d6d00c29853518a011e6cf72edb1d2248124feba`.

## Architecture decision alignment

No runner ADR files exist at the repository baseline. The agreed decisions reproduced in [INDEX](#index) govern this task (R08, R09, R10, R12). Earlier inspection/UI ADRs are supporting historical context, not a requirement to introduce PostgreSQL or a browser UI. Source claims remain subject to the [accuracy audit](https://github.com/deploymenttheory/go-ios-vm/blob/cbe8cd92d5672544fb97629ab0753c3eeb2cd176/docs/research-accuracy-audit.md).

---

<a id="guest-03"></a>

### [GUEST-03] Connect the host to guest readiness and graceful shutdown

<!-- weave-backlog:ios-runner-v1:v1:GUEST-03 -->
Backlog ID: `GUEST-03` · Epic: [EPIC-GUEST](#epic-guest) · Product baseline: [INDEX](#index)

## Outcome

Connect the host to guest readiness and graceful shutdown. This is planned runner work; the current main baseline contains research documents, not an existing runner implementation.

## Implementation

Implement the host side of the Go daemon protocol over the VM's native host/guest socket channel. Wire connection establishment, bounded ping/device-info requests, readiness timeout and graceful shutdown into session control. Maintain native queue ownership when opening socket connections, preserve callback lifetimes and distinguish transport connection, verified daemon response and guest/native stop. Do not install an HTTP API.

Porting reference: [pinned upstream component](https://github.com/Lakr233/vphone-cli/tree/b2804adb45a653ff3981f0705108975414dc8d5d/VPhoneDaemon). Planned package/file locations must be introduced by the implementation; do not assume historical branch code exists in the current checkout.

## Acceptance criteria

- [ ] The host verifies a daemon response from the intended VM and reports guest readiness separately from native running.
- [ ] A silent, disconnected or incompatible daemon cannot satisfy the readiness gate.
- [ ] Graceful shutdown failure transitions to the documented bounded native-stop path without leaking socket/callback resources.

## Verification

Use a controlled daemon peer for delayed startup, partial frames, wrong identity, timeout, disconnect, shutdown error and late native socket callbacks. Exercise actual guest communication during E2E-02; no fixture ping is recorded as live readiness.

These are required future checks, not test results from backlog preparation. All implementation PRs must identify what was actually executed and what remains unverified.

## Dependencies

Direct blockers:
- [GUEST-02](#guest-02) — requires its build and package the signed guest payload and exceptions outcome.
- [HOST-04](#host-04) — requires its own vm sessions, serial logs and stop requests outcome.

Supporting contracts already covered transitively: [FND-01](#fnd-01), [FND-02](#fnd-02), [FW-01](#fw-01), [GUEST-01](#guest-01), [HOST-01](#host-01), [HOST-02](#host-02). These are context, not additional direct blocker edges.

## Applicable product rules

**R05.** Native VM state and guest readiness are different observations. Persist completed stages only after their evidence exists; failures, cancellation and incomplete work must not look successful.

**R06.** AppKit uses the locked main thread. Native VM operations use their configured queue; wait for completion outside that queue and retain callback/object ownership through late completion. Canceling a Go wait does not cancel native work.

**R09.** Port the guest daemon to Go with readiness, device-information and shutdown operations. External linking is permitted for that daemon. Pinned ROMs, Apple guest drivers and required native guest hooks are explicit documented exceptions, not a host C-library exception.

Source: agreed plan revision 1, reproduced in [INDEX](#index); local source `docs/implementation-plan.md`.

## Planning

Milestone: `M3` — Runner M3 - Prepared firmware and guest integration.
Priority: `priority:P1` — required capability within this phase.
Component / scope owner: `component:protocol`; adjacent ticket outcomes are dependencies, not implicit extra scope.
Source rationale: agreed plan revision 1, GUEST subsystem and rules R05, R06, R09. Source SHA-256: `8ad1f8d9e932a758c411ac75d6d00c29853518a011e6cf72edb1d2248124feba`.

## Architecture decision alignment

No runner ADR files exist at the repository baseline. The agreed decisions reproduced in [INDEX](#index) govern this task (R05, R06, R09). Earlier inspection/UI ADRs are supporting historical context, not a requirement to introduce PostgreSQL or a browser UI. Source claims remain subject to the [accuracy audit](https://github.com/deploymenttheory/go-ios-vm/blob/cbe8cd92d5672544fb97629ab0753c3eeb2cd176/docs/research-accuracy-audit.md).

---

<a id="e2e-01"></a>

### [E2E-01] Compose create, launch and recovery into the Go CLI

<!-- weave-backlog:ios-runner-v1:v1:E2E-01 -->
Backlog ID: `E2E-01` · Epic: [EPIC-E2E](#epic-e2e) · Product baseline: [INDEX](#index)

## Outcome

Compose create, launch and recovery into the Go CLI. This is planned runner work; the current main baseline contains research documents, not an existing runner implementation.

## Implementation

Implement vm create orchestration from host preflight through verified acquisition, preparation, patching, forced-DFU hybrid restore, explicit custom-firmware installation and graphical first-boot validation. Publish a completed creation receipt only after the Go daemon responds and the validation VM stops. Implement persistent vm launch, stop, list and info using actual state. Reuse verified stage boundaries after failure; a failed restore must be restarted through a valid whole-stage sequence. Keep source/config/payload fingerprints in reuse decisions.

Porting reference: [pinned upstream component](https://github.com/Lakr233/vphone-cli/tree/b2804adb45a653ff3981f0705108975414dc8d5d/Documents/Guides). Planned package/file locations must be introduced by the implementation; do not assume historical branch code exists in the current checkout.

## Acceptance criteria

- [ ] A successful create ends with a verified, stopped VM that can later be launched interactively.
- [ ] Each command reports actual native/guest/pipeline states and failures without delegating to upstream host tools.
- [ ] Low space, unavailable authorization, unsupported pairing, canceled restore and stale receipts stop at the appropriate boundary and preserve diagnostics.

## Verification

Inject failures at every stage boundary and verify receipts, cleanup, retry decisions and CLI exit status. Run the complete local workflow only when preflight and privileges are satisfied; retain its evidence for E2E-02. Passing orchestration mocks alone does not complete the live acceptance gate.

These are required future checks, not test results from backlog preparation. All implementation PRs must identify what was actually executed and what remains unverified.

## Dependencies

Direct blockers:
- [FW-07](#fw-07) — requires its install custom firmware and the go guest payload explicitly outcome.
- [GUEST-03](#guest-03) — requires its connect the host to guest readiness and graceful shutdown outcome.
- [HOST-03](#host-03) — requires its display the guest with appkit graphics and input outcome.
- [FND-03](#fnd-03) — requires its diagnose host capabilities and storage without changing policy outcome.

Supporting contracts already covered transitively: [FND-01](#fnd-01), [FND-02](#fnd-02), [FW-01](#fw-01), [FW-02](#fw-02), [FW-03](#fw-03), [FW-04](#fw-04), [FW-05](#fw-05), [FW-06](#fw-06), [FW-08](#fw-08), [GUEST-01](#guest-01), [GUEST-02](#guest-02), [HOST-01](#host-01), [HOST-02](#host-02), [HOST-04](#host-04), [REST-01](#rest-01), [REST-02](#rest-02), [REST-03](#rest-03), [REST-04](#rest-04), [REST-05](#rest-05). These are context, not additional direct blocker edges.

## Applicable product rules

**R01.** Port the host application to idiomatic Go; do not execute vphone-cli, vphone-vm, Swift host helpers, or third-party C restore programs/libraries.

**R03.** Support only iPhone17,3 iOS 27.0.1 build 24A446 plus cloudOS 26.4 build 23E5207q initially. Check manifests; never substitute another device/build or select an arbitrary latest release.

**R04.** Use the requested user cache directory, resolving here to /Users/dafyddwatkins/Library/Caches/go-ios-vm/ipsw, and a separate ~/Library/Application Support/go-ios-vm/machines library, with overrides. Preserve unrelated cached images and stable per-machine identity/NVRAM/SEP/disk state.

**R05.** Native VM state and guest readiness are different observations. Persist completed stages only after their evidence exists; failures, cancellation and incomplete work must not look successful.

**R07.** Restore only the created virtual device matched by ECID and expected identity, serialize active restores, retain the complete signing response, and preserve the first fatal error. An interrupted restore is not resumable at an arbitrary byte offset.

**R10.** Apple-supplied disk/signing/sealing tools may be invoked explicitly. Keep privileged disk installation explicit; host preflight is read-only and must not change SIP, research-guest policy or AMFI.

**R11.** Default to eight CPUs, 8 GiB RAM, a 64 GB sparse disk and NAT, subject to native validation. Check current space and actual staging needs; the approximately 15 GiB observed during planning is not sufficient proof that a restore can run.

**R12.** First-release acceptance is create, graphical boot, input, daemon readiness, shutdown and repeat boot. Defer Launchpad, templates, snapshots, HTTP automation, package management, camera, audio and sensor parity. Do not commit proprietary firmware or signing credentials.

Source: agreed plan revision 1, reproduced in [INDEX](#index); local source `docs/implementation-plan.md`.

## Planning

Milestone: `M4` — Runner M4 - Interactive create and repeat boot.
Priority: `priority:P0` — integrity or acceptance gate.
Component / scope owner: `component:runtime`; adjacent ticket outcomes are dependencies, not implicit extra scope.
Source rationale: agreed plan revision 1, E2E subsystem and rules R01, R03, R04, R05, R07, R10, R11, R12. Source SHA-256: `8ad1f8d9e932a758c411ac75d6d00c29853518a011e6cf72edb1d2248124feba`.

## Architecture decision alignment

No runner ADR files exist at the repository baseline. The agreed decisions reproduced in [INDEX](#index) govern this task (R01, R03, R04, R05, R07, R10, R11, R12). Earlier inspection/UI ADRs are supporting historical context, not a requirement to introduce PostgreSQL or a browser UI. Source claims remain subject to the [accuracy audit](https://github.com/deploymenttheory/go-ios-vm/blob/cbe8cd92d5672544fb97629ab0753c3eeb2cd176/docs/research-accuracy-audit.md).

---

<a id="e2e-02"></a>

### [E2E-02] Prove creation, graphical input and a repeat boot on the target host

<!-- weave-backlog:ios-runner-v1:v1:E2E-02 -->
Backlog ID: `E2E-02` · Epic: [EPIC-E2E](#epic-e2e) · Product baseline: [INDEX](#index)

## Outcome

Prove creation, graphical input and a repeat boot on the target host. This is planned runner work; the current main baseline contains research documents, not an existing runner implementation.

## Implementation

Run the complete signed Go application against the exact selected pair on an authorized physical macOS 27+ Apple Silicon host with sufficient storage. Capture original/modified component hashes, tool/source pins, patch receipts, restore terminal status, serial output, Go daemon identity/readiness and GUI/input observations. Shut down and cold boot again with the same persistent machine state. Record real resource demand and run focused cancellation/error checks on a disposable guest. Leave the gate incomplete if storage, authorization, signing or a protocol is unresolved.

Porting reference: [pinned upstream component](https://github.com/Lakr233/vphone-cli/tree/b2804adb45a653ff3981f0705108975414dc8d5d/Documents/Guides). Planned package/file locations must be introduced by the implementation; do not assume historical branch code exists in the current checkout.

## Acceptance criteria

- [ ] A fresh VM reaches the graphical iOS guest, responds to input and the Go daemon, shuts down and repeats boot successfully.
- [ ] Evidence distinguishes upstream reports, fixture checks and independently executed results.
- [ ] The live host dependency path contains no upstream Swift host executable or third-party C restore library, and all native exceptions are accounted for.

## Verification

Provide reproducible build/sign/create/launch/stop commands and actual observed results, including screenshots or equivalent visual evidence, serial logs and hash/receipt summaries. Run Go build/test/vet plus native lifecycle/error checks. Do not commit IPSWs or ticket secrets; skipped live checks do not pass this ticket.

These are required future checks, not test results from backlog preparation. All implementation PRs must identify what was actually executed and what remains unverified.

## Dependencies

Direct blockers:
- [E2E-01](#e2e-01) — requires its compose create, launch and recovery into the go cli outcome.
- [FND-04](#fnd-04) — requires its add go build and test gates without privileged ci assumptions outcome.

Supporting contracts already covered transitively: [FND-01](#fnd-01), [FND-02](#fnd-02), [FND-03](#fnd-03), [FW-01](#fw-01), [FW-02](#fw-02), [FW-03](#fw-03), [FW-04](#fw-04), [FW-05](#fw-05), [FW-06](#fw-06), [FW-07](#fw-07), [FW-08](#fw-08), [GUEST-01](#guest-01), [GUEST-02](#guest-02), [GUEST-03](#guest-03), [HOST-01](#host-01), [HOST-02](#host-02), [HOST-03](#host-03), [HOST-04](#host-04), [REST-01](#rest-01), [REST-02](#rest-02), [REST-03](#rest-03), [REST-04](#rest-04), [REST-05](#rest-05). These are context, not additional direct blocker edges.

## Applicable product rules

**R02.** Target physical Apple Silicon Macs on macOS 27+ with Go 1.27.1. Pin macOS bindings to d5b3d0ecb48ed106ddac46095bd91ec945b2833c; the host must build with CGO_ENABLED=0.

**R03.** Support only iPhone17,3 iOS 27.0.1 build 24A446 plus cloudOS 26.4 build 23E5207q initially. Check manifests; never substitute another device/build or select an arbitrary latest release.

**R05.** Native VM state and guest readiness are different observations. Persist completed stages only after their evidence exists; failures, cancellation and incomplete work must not look successful.

**R08.** Keep original firmware immutable, record input/output component digests and source revisions, enforce expected patch matches, and stop on identity/hash/patch disagreement.

**R11.** Default to eight CPUs, 8 GiB RAM, a 64 GB sparse disk and NAT, subject to native validation. Check current space and actual staging needs; the approximately 15 GiB observed during planning is not sufficient proof that a restore can run.

**R12.** First-release acceptance is create, graphical boot, input, daemon readiness, shutdown and repeat boot. Defer Launchpad, templates, snapshots, HTTP automation, package management, camera, audio and sensor parity. Do not commit proprietary firmware or signing credentials.

Source: agreed plan revision 1, reproduced in [INDEX](#index); local source `docs/implementation-plan.md`.

## Planning

Milestone: `M4` — Runner M4 - Interactive create and repeat boot.
Priority: `priority:P0` — integrity or acceptance gate.
Component / scope owner: `component:verification`; adjacent ticket outcomes are dependencies, not implicit extra scope.
Source rationale: agreed plan revision 1, E2E subsystem and rules R02, R03, R05, R08, R11, R12. Source SHA-256: `8ad1f8d9e932a758c411ac75d6d00c29853518a011e6cf72edb1d2248124feba`.

## Architecture decision alignment

No runner ADR files exist at the repository baseline. The agreed decisions reproduced in [INDEX](#index) govern this task (R02, R03, R05, R08, R11, R12). Earlier inspection/UI ADRs are supporting historical context, not a requirement to introduce PostgreSQL or a browser UI. Source claims remain subject to the [accuracy audit](https://github.com/deploymenttheory/go-ios-vm/blob/cbe8cd92d5672544fb97629ab0753c3eeb2cd176/docs/research-accuracy-audit.md).

---

<a id="e2e-03"></a>

### [E2E-03] Document and package the verified first runner release

<!-- weave-backlog:ios-runner-v1:v1:E2E-03 -->
Backlog ID: `E2E-03` · Epic: [EPIC-E2E](#epic-e2e) · Product baseline: [INDEX](#index)

## Outcome

Document and package the verified first runner release. This is planned runner work; the current main baseline contains research documents, not an existing runner implementation.

## Implementation

Write the build/signing/run guide, native exception/source notices, supported pairing table, cache/library usage, manual host prerequisites, explicit privilege boundaries and troubleshooting from the actual validated implementation. Provide the Go host/guest packaging commands and artifact manifests. Describe deferred features and any remaining limitations; link real acceptance evidence instead of turning the research survey into a compatibility claim. Do not create a Launchpad, setup helper or continuous issue-sync service.

Porting reference: [pinned upstream component](https://github.com/Lakr233/vphone-cli/tree/b2804adb45a653ff3981f0705108975414dc8d5d/Documents/Guides). Planned package/file locations must be introduced by the implementation; do not assume historical branch code exists in the current checkout.

## Acceptance criteria

- [ ] A new engineer can reproduce the documented build and run process using the published source pins and artifact manifest.
- [ ] Release claims match E2E-02 evidence and clearly identify the allowed native assets and guest external-link requirement.
- [ ] Cache/VM locations, manual host setup, restore failure handling and deferred features are documented without exposing signing secrets or firmware blobs.

## Verification

Follow the documented commands on the validated environment, check artifact hashes/notices and link the resulting evidence. Verify all examples match real CLI flags and nonzero failure behavior; do not mark the release ready while E2E-02 is incomplete.

These are required future checks, not test results from backlog preparation. All implementation PRs must identify what was actually executed and what remains unverified.

## Dependencies

Direct blockers:
- [E2E-02](#e2e-02) — requires its prove creation, graphical input and a repeat boot on the target host outcome.

Supporting contracts already covered transitively: [E2E-01](#e2e-01), [FND-01](#fnd-01), [FND-02](#fnd-02), [FND-03](#fnd-03), [FND-04](#fnd-04), [FW-01](#fw-01), [FW-02](#fw-02), [FW-03](#fw-03), [FW-04](#fw-04), [FW-05](#fw-05), [FW-06](#fw-06), [FW-07](#fw-07), [FW-08](#fw-08), [GUEST-01](#guest-01), [GUEST-02](#guest-02), [GUEST-03](#guest-03), [HOST-01](#host-01), [HOST-02](#host-02), [HOST-03](#host-03), [HOST-04](#host-04), [REST-01](#rest-01), [REST-02](#rest-02), [REST-03](#rest-03), [REST-04](#rest-04), [REST-05](#rest-05). These are context, not additional direct blocker edges.

## Applicable product rules

**R01.** Port the host application to idiomatic Go; do not execute vphone-cli, vphone-vm, Swift host helpers, or third-party C restore programs/libraries.

**R02.** Target physical Apple Silicon Macs on macOS 27+ with Go 1.27.1. Pin macOS bindings to d5b3d0ecb48ed106ddac46095bd91ec945b2833c; the host must build with CGO_ENABLED=0.

**R03.** Support only iPhone17,3 iOS 27.0.1 build 24A446 plus cloudOS 26.4 build 23E5207q initially. Check manifests; never substitute another device/build or select an arbitrary latest release.

**R09.** Port the guest daemon to Go with readiness, device-information and shutdown operations. External linking is permitted for that daemon. Pinned ROMs, Apple guest drivers and required native guest hooks are explicit documented exceptions, not a host C-library exception.

**R10.** Apple-supplied disk/signing/sealing tools may be invoked explicitly. Keep privileged disk installation explicit; host preflight is read-only and must not change SIP, research-guest policy or AMFI.

**R12.** First-release acceptance is create, graphical boot, input, daemon readiness, shutdown and repeat boot. Defer Launchpad, templates, snapshots, HTTP automation, package management, camera, audio and sensor parity. Do not commit proprietary firmware or signing credentials.

Source: agreed plan revision 1, reproduced in [INDEX](#index); local source `docs/implementation-plan.md`.

## Planning

Milestone: `M4` — Runner M4 - Interactive create and repeat boot.
Priority: `priority:P1` — required capability within this phase.
Component / scope owner: `component:documentation`; adjacent ticket outcomes are dependencies, not implicit extra scope.
Source rationale: agreed plan revision 1, E2E subsystem and rules R01, R02, R03, R09, R10, R12. Source SHA-256: `8ad1f8d9e932a758c411ac75d6d00c29853518a011e6cf72edb1d2248124feba`.

## Architecture decision alignment

No runner ADR files exist at the repository baseline. The agreed decisions reproduced in [INDEX](#index) govern this task (R01, R02, R03, R09, R10, R12). Earlier inspection/UI ADRs are supporting historical context, not a requirement to introduce PostgreSQL or a browser UI. Source claims remain subject to the [accuracy audit](https://github.com/deploymenttheory/go-ios-vm/blob/cbe8cd92d5672544fb97629ab0753c3eeb2cd176/docs/research-accuracy-audit.md).

---

<a id="epic-fnd"></a>

### [Epic FND] Foundation and machine integrity

<!-- weave-backlog:ios-runner-v1:v1:EPIC-FND -->
Backlog ID: `EPIC-FND` · Product baseline: [INDEX](#index)

## Outcome

A runnable Go CLI owns durable machine state, exact firmware inputs, read-only preflight and repeatable build/test gates; incomplete operations cannot masquerade as a VM.

## Agreed behavior

**R01.** Port the host application to idiomatic Go; do not execute vphone-cli, vphone-vm, Swift host helpers, or third-party C restore programs/libraries.

**R02.** Target physical Apple Silicon Macs on macOS 27+ with Go 1.27.1. Pin macOS bindings to d5b3d0ecb48ed106ddac46095bd91ec945b2833c; the host must build with CGO_ENABLED=0.

**R04.** Use the requested user cache directory, resolving here to /Users/dafyddwatkins/Library/Caches/go-ios-vm/ipsw, and a separate ~/Library/Application Support/go-ios-vm/machines library, with overrides. Preserve unrelated cached images and stable per-machine identity/NVRAM/SEP/disk state.

**R05.** Native VM state and guest readiness are different observations. Persist completed stages only after their evidence exists; failures, cancellation and incomplete work must not look successful.

**R08.** Keep original firmware immutable, record input/output component digests and source revisions, enforce expected patch matches, and stop on identity/hash/patch disagreement.

**R10.** Apple-supplied disk/signing/sealing tools may be invoked explicitly. Keep privileged disk installation explicit; host preflight is read-only and must not change SIP, research-guest policy or AMFI.

**R11.** Default to eight CPUs, 8 GiB RAM, a 64 GB sparse disk and NAT, subject to native validation. Check current space and actual staging needs; the approximately 15 GiB observed during planning is not sufficient proof that a restore can run.

**R12.** First-release acceptance is create, graphical boot, input, daemon readiness, shutdown and repeat boot. Defer Launchpad, templates, snapshots, HTTP automation, package management, camera, audio and sensor parity. Do not commit proprietary firmware or signing credentials.

Source: agreed plan revision 1, reproduced in [INDEX](#index); local source `docs/implementation-plan.md`.

## Implementation tickets

- [ ] [FND-01](#fnd-01) — component:infrastructure; priority:P0; Establish the Go module, CLI and package contracts
- [ ] [FND-02](#fnd-02) — component:backend; priority:P0; Persist machine identity, stage records and exclusive ownership
- [ ] [FND-03](#fnd-03) — component:runtime; priority:P0; Diagnose host capabilities and storage without changing policy
- [ ] [FND-04](#fnd-04) — component:infrastructure; priority:P0; Add Go build and test gates without privileged CI assumptions

## Epic acceptance

- [ ] Demonstrate the integrated outcome above with all applicable native/guest behavior observed rather than inferred from closed child tickets.
- [ ] Verify the failure, cancellation and ownership boundaries shared by these children; retain source/build pins and distinguish fixture results from executed native evidence.
- [ ] Account for each child's required acceptance evidence and any unresolved integration/environment blocker. Child closure alone does not close this epic.

## Planning

Milestone: `M1` — Runner M1 - Native host and DFU foundation; this is the gate for integrated epic acceptance. Earlier-gate children remain assigned to their own delivery gates.
Priority: `priority:P0`.
Source rationale: agreed plan revision 1; foundation and machine integrity.

## Architecture decision alignment

No runner ADR files exist at the repository baseline. The agreed decisions reproduced in [INDEX](#index) govern this task (R01, R02, R04, R05, R08, R10, R11, R12). Earlier inspection/UI ADRs are supporting historical context, not a requirement to introduce PostgreSQL or a browser UI. Source claims remain subject to the [accuracy audit](https://github.com/deploymenttheory/go-ios-vm/blob/cbe8cd92d5672544fb97629ab0753c3eeb2cd176/docs/research-accuracy-audit.md).

---

<a id="epic-host"></a>

### [Epic HOST] Native research VM hosting and window

<!-- weave-backlog:ios-runner-v1:v1:EPIC-HOST -->
Backlog ID: `EPIC-HOST` · Product baseline: [INDEX](#index)

## Outcome

A queue-correct Go host creates the research platform, observes its own DFU identity, owns lifecycle/serial control and displays the native VM view. Integrated iOS boot/input acceptance belongs to E2E-02.

## Agreed behavior

**R01.** Port the host application to idiomatic Go; do not execute vphone-cli, vphone-vm, Swift host helpers, or third-party C restore programs/libraries.

**R02.** Target physical Apple Silicon Macs on macOS 27+ with Go 1.27.1. Pin macOS bindings to d5b3d0ecb48ed106ddac46095bd91ec945b2833c; the host must build with CGO_ENABLED=0.

**R04.** Use the requested user cache directory, resolving here to /Users/dafyddwatkins/Library/Caches/go-ios-vm/ipsw, and a separate ~/Library/Application Support/go-ios-vm/machines library, with overrides. Preserve unrelated cached images and stable per-machine identity/NVRAM/SEP/disk state.

**R05.** Native VM state and guest readiness are different observations. Persist completed stages only after their evidence exists; failures, cancellation and incomplete work must not look successful.

**R06.** AppKit uses the locked main thread. Native VM operations use their configured queue; wait for completion outside that queue and retain callback/object ownership through late completion. Canceling a Go wait does not cancel native work.

**R09.** Port the guest daemon to Go with readiness, device-information and shutdown operations. External linking is permitted for that daemon. Pinned ROMs, Apple guest drivers and required native guest hooks are explicit documented exceptions, not a host C-library exception.

**R10.** Apple-supplied disk/signing/sealing tools may be invoked explicitly. Keep privileged disk installation explicit; host preflight is read-only and must not change SIP, research-guest policy or AMFI.

**R11.** Default to eight CPUs, 8 GiB RAM, a 64 GB sparse disk and NAT, subject to native validation. Check current space and actual staging needs; the approximately 15 GiB observed during planning is not sufficient proof that a restore can run.

**R12.** First-release acceptance is create, graphical boot, input, daemon readiness, shutdown and repeat boot. Defer Launchpad, templates, snapshots, HTTP automation, package management, camera, audio and sensor parity. Do not commit proprietary firmware or signing credentials.

Source: agreed plan revision 1, reproduced in [INDEX](#index); local source `docs/implementation-plan.md`.

## Implementation tickets

- [ ] [HOST-01](#host-01) — component:runtime; priority:P0; Implement the native bridge and queue-safe lifecycle primitives
- [ ] [HOST-02](#host-02) — component:runtime; priority:P0; Create the research VM configuration and observe forced DFU
- [ ] [HOST-03](#host-03) — component:native-ui; priority:P1; Display the guest with AppKit graphics and input
- [ ] [HOST-04](#host-04) — component:runtime; priority:P0; Own VM sessions, serial logs and stop requests

## Epic acceptance

- [ ] Demonstrate the integrated outcome above with all applicable native/guest behavior observed rather than inferred from closed child tickets.
- [ ] Verify the failure, cancellation and ownership boundaries shared by these children; retain source/build pins and distinguish fixture results from executed native evidence.
- [ ] Account for each child's required acceptance evidence and any unresolved integration/environment blocker. Child closure alone does not close this epic.

## Planning

Milestone: `M1` — Runner M1 - Native host and DFU foundation; this is the gate for integrated epic acceptance. Earlier-gate children remain assigned to their own delivery gates.
Priority: `priority:P0`.
Source rationale: agreed plan revision 1; native research vm hosting and window.

## Architecture decision alignment

No runner ADR files exist at the repository baseline. The agreed decisions reproduced in [INDEX](#index) govern this task (R01, R02, R04, R05, R06, R09, R10, R11, R12). Earlier inspection/UI ADRs are supporting historical context, not a requirement to introduce PostgreSQL or a browser UI. Source claims remain subject to the [accuracy audit](https://github.com/deploymenttheory/go-ios-vm/blob/cbe8cd92d5672544fb97629ab0753c3eeb2cd176/docs/research-accuracy-audit.md).

---

<a id="epic-rest"></a>

### [Epic REST] Pure-Go virtual restore

<!-- weave-backlog:ios-runner-v1:v1:EPIC-REST -->
Backlog ID: `EPIC-REST` · Product baseline: [INDEX](#index)

## Outcome

The selected cloudOS virtual device completes a Go-controlled restore with correct identity, personalization, required service/data transfer and terminal error semantics.

## Agreed behavior

**R01.** Port the host application to idiomatic Go; do not execute vphone-cli, vphone-vm, Swift host helpers, or third-party C restore programs/libraries.

**R02.** Target physical Apple Silicon Macs on macOS 27+ with Go 1.27.1. Pin macOS bindings to d5b3d0ecb48ed106ddac46095bd91ec945b2833c; the host must build with CGO_ENABLED=0.

**R03.** Support only iPhone17,3 iOS 27.0.1 build 24A446 plus cloudOS 26.4 build 23E5207q initially. Check manifests; never substitute another device/build or select an arbitrary latest release.

**R05.** Native VM state and guest readiness are different observations. Persist completed stages only after their evidence exists; failures, cancellation and incomplete work must not look successful.

**R07.** Restore only the created virtual device matched by ECID and expected identity, serialize active restores, retain the complete signing response, and preserve the first fatal error. An interrupted restore is not resumable at an arbitrary byte offset.

**R08.** Keep original firmware immutable, record input/output component digests and source revisions, enforce expected patch matches, and stop on identity/hash/patch disagreement.

Source: agreed plan revision 1, reproduced in [INDEX](#index); local source `docs/implementation-plan.md`.

## Implementation tickets

- [ ] [REST-01](#rest-01) — component:protocol; priority:P0; Implement virtual recovery USB transport through IOKit
- [ ] [REST-02](#rest-02) — component:protocol; priority:P0; Implement signing requests and component personalization in Go
- [ ] [REST-03](#rest-03) — component:protocol; priority:P0; Implement restore-mode messaging and required service channels
- [ ] [REST-04](#rest-04) — component:protocol; priority:P0; Implement ASR and restore payload transfer in Go
- [ ] [REST-05](#rest-05) — component:protocol; priority:P0; Orchestrate and verify a cloudOS restore with OpenAVB

## Epic acceptance

- [ ] Demonstrate the integrated outcome above with all applicable native/guest behavior observed rather than inferred from closed child tickets.
- [ ] Verify the failure, cancellation and ownership boundaries shared by these children; retain source/build pins and distinguish fixture results from executed native evidence.
- [ ] Account for each child's required acceptance evidence and any unresolved integration/environment blocker. Child closure alone does not close this epic.

## Planning

Milestone: `M2` — Runner M2 - Go restore; this is the gate for integrated epic acceptance. Earlier-gate children remain assigned to their own delivery gates.
Priority: `priority:P0`.
Source rationale: agreed plan revision 1; pure-go virtual restore.

## Architecture decision alignment

No runner ADR files exist at the repository baseline. The agreed decisions reproduced in [INDEX](#index) govern this task (R01, R02, R03, R05, R07, R08). Earlier inspection/UI ADRs are supporting historical context, not a requirement to introduce PostgreSQL or a browser UI. Source claims remain subject to the [accuracy audit](https://github.com/deploymenttheory/go-ios-vm/blob/cbe8cd92d5672544fb97629ab0753c3eeb2cd176/docs/research-accuracy-audit.md).

---

<a id="epic-fw"></a>

### [Epic FW] Verified paired firmware preparation and installation

<!-- weave-backlog:ios-runner-v1:v1:EPIC-FW -->
Backlog ID: `EPIC-FW` · Product baseline: [INDEX](#index)

## Outcome

The selected hybrid firmware and installed guest system have verified sources, GPU assets, boot/userspace patch receipts and explicit native exceptions; failed stages remain incomplete.

## Agreed behavior

**R01.** Port the host application to idiomatic Go; do not execute vphone-cli, vphone-vm, Swift host helpers, or third-party C restore programs/libraries.

**R02.** Target physical Apple Silicon Macs on macOS 27+ with Go 1.27.1. Pin macOS bindings to d5b3d0ecb48ed106ddac46095bd91ec945b2833c; the host must build with CGO_ENABLED=0.

**R03.** Support only iPhone17,3 iOS 27.0.1 build 24A446 plus cloudOS 26.4 build 23E5207q initially. Check manifests; never substitute another device/build or select an arbitrary latest release.

**R04.** Use the requested user cache directory, resolving here to /Users/dafyddwatkins/Library/Caches/go-ios-vm/ipsw, and a separate ~/Library/Application Support/go-ios-vm/machines library, with overrides. Preserve unrelated cached images and stable per-machine identity/NVRAM/SEP/disk state.

**R05.** Native VM state and guest readiness are different observations. Persist completed stages only after their evidence exists; failures, cancellation and incomplete work must not look successful.

**R08.** Keep original firmware immutable, record input/output component digests and source revisions, enforce expected patch matches, and stop on identity/hash/patch disagreement.

**R09.** Port the guest daemon to Go with readiness, device-information and shutdown operations. External linking is permitted for that daemon. Pinned ROMs, Apple guest drivers and required native guest hooks are explicit documented exceptions, not a host C-library exception.

**R10.** Apple-supplied disk/signing/sealing tools may be invoked explicitly. Keep privileged disk installation explicit; host preflight is read-only and must not change SIP, research-guest policy or AMFI.

**R11.** Default to eight CPUs, 8 GiB RAM, a 64 GB sparse disk and NAT, subject to native validation. Check current space and actual staging needs; the approximately 15 GiB observed during planning is not sufficient proof that a restore can run.

**R12.** First-release acceptance is create, graphical boot, input, daemon readiness, shutdown and repeat boot. Defer Launchpad, templates, snapshots, HTTP automation, package management, camera, audio and sensor parity. Do not commit proprietary firmware or signing credentials.

Source: agreed plan revision 1, reproduced in [INDEX](#index); local source `docs/implementation-plan.md`.

## Implementation tickets

- [ ] [FW-01](#fw-01) — component:firmware; priority:P0; Pin upstream port sources and native firmware exceptions
- [ ] [FW-02](#fw-02) — component:backend; priority:P0; Acquire and inspect the exact firmware pair in the requested cache
- [ ] [FW-03](#fw-03) — component:firmware; priority:P0; Implement firmware containers and binary patch primitives
- [ ] [FW-04](#fw-04) — component:firmware; priority:P0; Port the selected boot-chain patches and receipts
- [ ] [FW-05](#fw-05) — component:firmware; priority:P1; Extract and cache the matching GPU resources using a temporary VM
- [ ] [FW-06](#fw-06) — component:firmware; priority:P0; Prepare the hybrid restore tree for the single supported pair
- [ ] [FW-08](#fw-08) — component:firmware; priority:P0; Port required userspace and dyld cache modifications
- [ ] [FW-07](#fw-07) — component:firmware; priority:P0; Install custom firmware and the Go guest payload explicitly

## Epic acceptance

- [ ] Demonstrate the integrated outcome above with all applicable native/guest behavior observed rather than inferred from closed child tickets.
- [ ] Verify the failure, cancellation and ownership boundaries shared by these children; retain source/build pins and distinguish fixture results from executed native evidence.
- [ ] Account for each child's required acceptance evidence and any unresolved integration/environment blocker. Child closure alone does not close this epic.

## Planning

Milestone: `M3` — Runner M3 - Prepared firmware and guest integration; this is the gate for integrated epic acceptance. Earlier-gate children remain assigned to their own delivery gates.
Priority: `priority:P0`.
Source rationale: agreed plan revision 1; verified paired firmware preparation and installation.

## Architecture decision alignment

No runner ADR files exist at the repository baseline. The agreed decisions reproduced in [INDEX](#index) govern this task (R01, R02, R03, R04, R05, R08, R09, R10, R11, R12). Earlier inspection/UI ADRs are supporting historical context, not a requirement to introduce PostgreSQL or a browser UI. Source claims remain subject to the [accuracy audit](https://github.com/deploymenttheory/go-ios-vm/blob/cbe8cd92d5672544fb97629ab0753c3eeb2cd176/docs/research-accuracy-audit.md).

---

<a id="epic-guest"></a>

### [Epic GUEST] Go guest readiness and shutdown integration

<!-- weave-backlog:ios-runner-v1:v1:EPIC-GUEST -->
Backlog ID: `EPIC-GUEST` · Product baseline: [INDEX](#index)

## Outcome

A built Go guest daemon and host peer implement readiness, actual device information and graceful shutdown, with a checked install payload and native callbacks owned correctly.

## Agreed behavior

**R01.** Port the host application to idiomatic Go; do not execute vphone-cli, vphone-vm, Swift host helpers, or third-party C restore programs/libraries.

**R05.** Native VM state and guest readiness are different observations. Persist completed stages only after their evidence exists; failures, cancellation and incomplete work must not look successful.

**R06.** AppKit uses the locked main thread. Native VM operations use their configured queue; wait for completion outside that queue and retain callback/object ownership through late completion. Canceling a Go wait does not cancel native work.

**R08.** Keep original firmware immutable, record input/output component digests and source revisions, enforce expected patch matches, and stop on identity/hash/patch disagreement.

**R09.** Port the guest daemon to Go with readiness, device-information and shutdown operations. External linking is permitted for that daemon. Pinned ROMs, Apple guest drivers and required native guest hooks are explicit documented exceptions, not a host C-library exception.

**R10.** Apple-supplied disk/signing/sealing tools may be invoked explicitly. Keep privileged disk installation explicit; host preflight is read-only and must not change SIP, research-guest policy or AMFI.

**R12.** First-release acceptance is create, graphical boot, input, daemon readiness, shutdown and repeat boot. Defer Launchpad, templates, snapshots, HTTP automation, package management, camera, audio and sensor parity. Do not commit proprietary firmware or signing credentials.

Source: agreed plan revision 1, reproduced in [INDEX](#index); local source `docs/implementation-plan.md`.

## Implementation tickets

- [ ] [GUEST-01](#guest-01) — component:protocol; priority:P1; Implement the minimal guest daemon in Go
- [ ] [GUEST-02](#guest-02) — component:firmware; priority:P0; Build and package the signed guest payload and exceptions
- [ ] [GUEST-03](#guest-03) — component:protocol; priority:P1; Connect the host to guest readiness and graceful shutdown

## Epic acceptance

- [ ] Demonstrate the integrated outcome above with all applicable native/guest behavior observed rather than inferred from closed child tickets.
- [ ] Verify the failure, cancellation and ownership boundaries shared by these children; retain source/build pins and distinguish fixture results from executed native evidence.
- [ ] Account for each child's required acceptance evidence and any unresolved integration/environment blocker. Child closure alone does not close this epic.

## Planning

Milestone: `M3` — Runner M3 - Prepared firmware and guest integration; this is the gate for integrated epic acceptance. Earlier-gate children remain assigned to their own delivery gates.
Priority: `priority:P1`.
Source rationale: agreed plan revision 1; go guest readiness and shutdown integration.

## Architecture decision alignment

No runner ADR files exist at the repository baseline. The agreed decisions reproduced in [INDEX](#index) govern this task (R01, R05, R06, R08, R09, R10, R12). Earlier inspection/UI ADRs are supporting historical context, not a requirement to introduce PostgreSQL or a browser UI. Source claims remain subject to the [accuracy audit](https://github.com/deploymenttheory/go-ios-vm/blob/cbe8cd92d5672544fb97629ab0753c3eeb2cd176/docs/research-accuracy-audit.md).

---

<a id="epic-e2e"></a>

### [Epic E2E] Interactive creation and repeat-boot qualification

<!-- weave-backlog:ios-runner-v1:v1:EPIC-E2E -->
Backlog ID: `EPIC-E2E` · Product baseline: [INDEX](#index)

## Outcome

A signed Go runner creates the exact selected iOS guest, demonstrates GUI/input and real Go-daemon readiness, stops it and cold boots again with a reproducible evidence-backed release guide.

## Agreed behavior

**R01.** Port the host application to idiomatic Go; do not execute vphone-cli, vphone-vm, Swift host helpers, or third-party C restore programs/libraries.

**R02.** Target physical Apple Silicon Macs on macOS 27+ with Go 1.27.1. Pin macOS bindings to d5b3d0ecb48ed106ddac46095bd91ec945b2833c; the host must build with CGO_ENABLED=0.

**R03.** Support only iPhone17,3 iOS 27.0.1 build 24A446 plus cloudOS 26.4 build 23E5207q initially. Check manifests; never substitute another device/build or select an arbitrary latest release.

**R04.** Use the requested user cache directory, resolving here to /Users/dafyddwatkins/Library/Caches/go-ios-vm/ipsw, and a separate ~/Library/Application Support/go-ios-vm/machines library, with overrides. Preserve unrelated cached images and stable per-machine identity/NVRAM/SEP/disk state.

**R05.** Native VM state and guest readiness are different observations. Persist completed stages only after their evidence exists; failures, cancellation and incomplete work must not look successful.

**R07.** Restore only the created virtual device matched by ECID and expected identity, serialize active restores, retain the complete signing response, and preserve the first fatal error. An interrupted restore is not resumable at an arbitrary byte offset.

**R08.** Keep original firmware immutable, record input/output component digests and source revisions, enforce expected patch matches, and stop on identity/hash/patch disagreement.

**R09.** Port the guest daemon to Go with readiness, device-information and shutdown operations. External linking is permitted for that daemon. Pinned ROMs, Apple guest drivers and required native guest hooks are explicit documented exceptions, not a host C-library exception.

**R10.** Apple-supplied disk/signing/sealing tools may be invoked explicitly. Keep privileged disk installation explicit; host preflight is read-only and must not change SIP, research-guest policy or AMFI.

**R11.** Default to eight CPUs, 8 GiB RAM, a 64 GB sparse disk and NAT, subject to native validation. Check current space and actual staging needs; the approximately 15 GiB observed during planning is not sufficient proof that a restore can run.

**R12.** First-release acceptance is create, graphical boot, input, daemon readiness, shutdown and repeat boot. Defer Launchpad, templates, snapshots, HTTP automation, package management, camera, audio and sensor parity. Do not commit proprietary firmware or signing credentials.

Source: agreed plan revision 1, reproduced in [INDEX](#index); local source `docs/implementation-plan.md`.

## Implementation tickets

- [ ] [E2E-01](#e2e-01) — component:runtime; priority:P0; Compose create, launch and recovery into the Go CLI
- [ ] [E2E-02](#e2e-02) — component:verification; priority:P0; Prove creation, graphical input and a repeat boot on the target host
- [ ] [E2E-03](#e2e-03) — component:documentation; priority:P1; Document and package the verified first runner release

## Epic acceptance

- [ ] Demonstrate the integrated outcome above with all applicable native/guest behavior observed rather than inferred from closed child tickets.
- [ ] Verify the failure, cancellation and ownership boundaries shared by these children; retain source/build pins and distinguish fixture results from executed native evidence.
- [ ] Account for each child's required acceptance evidence and any unresolved integration/environment blocker. Child closure alone does not close this epic.

## Planning

Milestone: `M4` — Runner M4 - Interactive create and repeat boot; this is the gate for integrated epic acceptance. Earlier-gate children remain assigned to their own delivery gates.
Priority: `priority:P0`.
Source rationale: agreed plan revision 1; interactive creation and repeat-boot qualification.

## Architecture decision alignment

No runner ADR files exist at the repository baseline. The agreed decisions reproduced in [INDEX](#index) govern this task (R01, R02, R03, R04, R05, R07, R08, R09, R10, R11, R12). Earlier inspection/UI ADRs are supporting historical context, not a requirement to introduce PostgreSQL or a browser UI. Source claims remain subject to the [accuracy audit](https://github.com/deploymenttheory/go-ios-vm/blob/cbe8cd92d5672544fb97629ab0753c3eeb2cd176/docs/research-accuracy-audit.md).

---
