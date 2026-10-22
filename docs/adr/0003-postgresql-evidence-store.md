---
status: accepted
---

# PostgreSQL owns retained evidence

Store findings, their relationships and provenance, and the supporting evidence bytes in PostgreSQL. The user selected a database-owned evidence model in place of the proposed SQLite catalog with supporting bytes in local files. Disposable captures and decoded caches remain bounded; evidence needed to support retained findings must survive their cleanup.

This adds database provisioning and migration responsibilities, but gives retained evidence and its relationships one system of record. It does not imply keeping every trace or every regenerated report. Placement of complete downloaded IPSWs remains a separate decision.

The runner consumes a reviewed, versioned machine profile; PostgreSQL is the inspection system's evidence store, not a runtime dependency or the VM's persistence implementation.
