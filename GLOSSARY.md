# go-ios-vm

Tools for understanding iOS firmware and a runner for iOS virtual machines on a Mac.

## Language

**IPSW**:
An Apple restore package containing operating-system and firmware components for one or more device identities.

**Inspection tools**:
The suite used to examine IPSWs, firmware, and execution to answer questions about what a virtual iPhone model must provide.

**VM runner**:
The program that starts and manages an iOS virtual machine on a Mac.

**Virtual iPhone model**:
A software representation of an iPhone hardware generation, including the devices and behavior required by its firmware.

**Static firmware inspection**:
Examination of firmware contents and structure without executing the guest.

**Live execution observation**:
Examination of a running guest's execution, device accesses, interrupts, and failures.

**Observed finding**:
A statement directly supported by identified firmware contents or a recorded execution event. Its scope is limited to what was actually inspected.

**Finding**:
The enduring question about one exact target, subject, and property. Different readings of that question belong to the same finding.

**Finding revision**:
One recorded answer or unanswered state for a finding, with its basis and rationale. A correction links to the earlier revision while preserving it.

**Basis reference**:
An explicit supporting or contradicting link to a retained observation or another finding revision.

**Inferred finding**:
An interpretation drawn from identified evidence and an explicit reasoning step.

**Unknown**:
A question about the target for which the available evidence does not establish an answer. Unknown does not mean absent or zero.

**Retained evidence**:
The observations and supporting material kept to justify and inspect a finding, including the necessary source bytes or recorded events.

**Machine profile**:
An explicit, versioned specification of a virtual machine target, reflecting reviewed implementation decisions and their supporting findings.

**Acquisition**:
Discovery and retrieval of the restore package for a particular device and operating-system build, together with its origin and identity.

**IPSW cache**:
Managed local copies of downloaded restore packages that can be reused for inspection or reacquired when the exact source is still available. Retained evidence is independent of these cached copies.
