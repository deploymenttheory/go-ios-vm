---
status: accepted
---

# Disk-image and filesystem codec capabilities live in go-apfs-v3

When an inspection task exposes missing disk-image or filesystem codec support, add the required capability to `go-apfs-v3` and consume it through that dependency. The user chose this boundary to reuse and improve the existing Go image/filesystem project. `go-ios-vm` owns firmware inspection workflows, source-location integration, and hardware findings.

Verify the exact layer and required format before declaring a gap. An unsupported UDIF chunk type and an unsupported compressed filesystem file are separate capabilities. A bounded read of the public iPhone12,1 iOS 14.0 restore image subsequently confirmed a missing LZFSE UDIF chunk decoder (`0x80000007`); [go-apfs-v3 #146](https://github.com/deploymenttheory/go-apfs-v3/issues/146) tracks it. The full archive qualification remains separate from that bounded format observation.
