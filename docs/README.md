# Project design

- [Scope and current stage](../README.md)
- [Implementation status and next work](implementation-status.md)
- [Historical iPhone12,1 restore resolution and bounded manifest probes](qualification/historical-resolution.md)
- [Verified iPhone12,1 iOS 14.0 image and partial inspection qualification](qualification/iphone12-1-ios14.md)
- [Essential references retained before legacy deletion](research/retired-spike-notes.md)
- [Glossary](../GLOSSARY.md)
- [Fresh implementation with on-demand historical reference](adr/0001-fresh-implementation.md)
- [Disk-image and filesystem codec ownership](adr/0002-codec-capabilities-in-go-apfs.md)
- [PostgreSQL evidence ownership](adr/0003-postgresql-evidence-store.md)
- [Evidence model and proposed schema](evidence-model.md)
- [Consolidated inspection architecture](inspection-architecture.md)

## Settled requirements

- Fresh Go project, supporting Python permitted; no inherited implementation or evidence archive.
- Two deliverables: firmware inspection tools first, then an iOS runner using macOS Hypervisor.framework.
- Tools qualify on macOS and Linux. The runner targets Apple Silicon macOS. Windows is out of scope.
- iOS 14 first; 15, 26 and 27 also in scope. First comparison uses matching hardware on iOS 14/15, followed by newer releases and different hardware generations.
- Cobra/Viper CLI, readable and structured output, reuse suitable dependencies, new Go tooling where required.
- GDMF discovery plus Apple-owned restore catalog resolution; exact IPSWs downloaded on demand into a managed cache.
- PostgreSQL owns findings, provenance and supporting evidence bytes. Provide a dedicated local instance with configurable local/remote connectivity.
- Bounded disposable caches, protected essential evidence, explicit unknowns and source coordinates.
- Findings reach the runner through a reviewed versioned machine profile. Usable-VM acceptance includes interaction, persistence, restart and SIP-enabled operation.

## Design tree status

The first-tooling interview and shared-understanding review are complete. The initial CLI implementation is merged; see the implementation status for demonstrated behavior and remaining gaps.

Dependencies, local PostgreSQL startup and initial resource budgets are established. The exact iOS 14 archive has been verified and inspected on macOS and Linux with partial coverage. The iOS 15 full-image qualification and remaining adapters are outstanding.

The runner's exact machine profile and full subsystem contracts remain a later design branch before runner implementation. Inspection success does not establish VM feasibility. The old repository and wider Weave products remain unchanged.
