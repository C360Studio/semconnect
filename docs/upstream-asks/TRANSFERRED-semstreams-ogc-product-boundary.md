# Transferred upstream asks - SemStreams OGC product boundary

- **Decision**: ADR-S003
- **Transfer release**: SemStreams `v1.0.0-beta.147`
- **SemStreams commit**: `5cc22c109594e48b7f1cec04bcaaf0106d85495a`
- **Reviewed source provenance**: `c8f0b92edf5ad5b491d5f4e81891bec817fae3cd`
- **Status**: Product ownership transferred to semconnect

## Why these asks moved

SemStreams beta.147 deliberately trims OGC Connected Systems product packages
from its non-product framework core. Semconnect now owns `message/oms`,
`parser/sensorml`, `pkg/swecommon`, and
`vocabulary/{csapi,oms,sosa,swe}`. Graph, NATS, JetStream, ObjectStore,
ownership, and projection remain SemStreams responsibilities.

This is an ownership transfer, not a claim that the original requests were
mistakes. They were the correct integration path while SemStreams hosted the
packages. ADR-S003 and OpenSpec change `migrate-semstreams-beta147` now govern
their implementation and evidence.

## Issue disposition

- [SemStreams #200](https://github.com/C360Studio/semstreams/issues/200),
  Command Feasibility vocabulary, moved to
  [semconnect #70](https://github.com/C360Studio/semconnect/issues/70).
  It is implemented in owned `vocabulary/csapi`; release evidence is pending.
- [SemStreams #201](https://github.com/C360Studio/semstreams/issues/201),
  association/composition vocabulary, moved to
  [semconnect #71](https://github.com/C360Studio/semconnect/issues/71).
  It is implemented in owned `vocabulary/csapi`; release evidence is pending.
- [SemStreams #202](https://github.com/C360Studio/semstreams/issues/202),
  scalar-metadata ownership, is resolved by ADR-S003. Canonical predicates are
  included in the 32-correction ledger.
- SWE Common Phase 2 remains
  [semconnect #69](https://github.com/C360Studio/semconnect/issues/69). It is
  explicitly outside the beta.147 migration.

## Closure rule

Issues #70 and #71 are implementation-complete only within the beta.147
migration candidate. They are release-complete after the full quality,
cutover-rehearsal, retained-state, and external `137/0/0` gates pass. Issue #69
must not be closed or folded into this migration merely because semconnect now
owns `pkg/swecommon`.
