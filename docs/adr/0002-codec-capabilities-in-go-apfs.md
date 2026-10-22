---
status: accepted
---

# Disk-image and filesystem codec capabilities live in go-apfs-v3

When an inspection task exposes missing disk-image or filesystem codec support, add the required capability to `go-apfs-v3` and consume it through that dependency. The user chose this boundary to reuse and improve the existing Go image/filesystem project. `go-ios-vm` owns firmware inspection workflows, source-location integration, and hardware findings.

Verify the exact layer and required format before declaring a gap. An unsupported UDIF chunk type and an unsupported compressed filesystem file are separate capabilities. The current source review has not established that any target IPSW requires a missing codec. No codec implementation work has been started by this decision.
