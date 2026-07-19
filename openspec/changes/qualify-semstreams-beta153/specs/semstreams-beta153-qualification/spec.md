## ADDED Requirements

### Requirement: Beta.153 is pinned by exact identity

Semconnect SHALL align every executable SemStreams pin to `v1.0.0-beta.153`, tag object
`ee011caee8a137b8dfb01d7634e9bb09519818b8`, peeled commit
`d2654e5a027138b8a9056863da5ed463ef767f37`, and source tree
`dc7422aa9fd93ec446dca73a33e0c602b6601111`.

#### Scenario: Any pin differs

- **WHEN** the module, conformance source, Compose build, labels, or tests identify another release or commit
- **THEN** alignment fails before runtime qualification

### Requirement: Live graph mutation behavior remains canonical and fail-closed

Beta.153 SHALL preserve semconnect's six-part entity IDs, three-part predicates, canonical references, foreign-edge
routing, and classified mutation behavior. Invalid direct create or update input MUST cause no entity bytes, entity
revision, bucket revision, or index change. Resident poison MUST NOT be laundered by read-modify-write.

#### Scenario: Canonical resources are created and updated

- **WHEN** semconnect writes canonical System and hosted-child fixtures through the beta.153 mutation subjects
- **THEN** the exact canonical triples persist and the claimed foreign edge is queryable

#### Scenario: An invalid mutation reaches graph-ingest

- **WHEN** a direct mutation contains an invalid entity ID, predicate, or entity reference
- **THEN** it is rejected with the expected classification and no backend state changes

#### Scenario: Resident poison reaches an update

- **WHEN** an isolated test injects invalid resident bytes and submits a canonical update
- **THEN** beta.153 reports graph-state-reset-required and leaves bytes and revisions unchanged

### Requirement: No legacy or migration behavior is introduced

Qualification SHALL NOT add compatibility aliases, dual reads or writes, predicate rewriting, relaxed validation,
cleanup lanes, migration commands, or old-state support. Production targets a clean pre-v1 NATS volume.

#### Scenario: Green requires compatibility behavior

- **WHEN** any required gate needs legacy handling or a weakened assertion
- **THEN** the candidate is rejected and architecture review reopens

### Requirement: Local and focused upstream verification is green

The exact beta.153 pin MUST pass full downstream Go test, race, vet, and build gates plus live-NATS structural
integration. Focused upstream tests MUST cover graph-ingest poison scoping, watcher retirement, validation
memoization, write gates, and the rule health race. Unused rule, agentic-loop, and fusion additions MUST compile and
pass package tests.

#### Scenario: A required test fails or is skipped

- **WHEN** a required command fails, is filtered out, or needs an undocumented exception
- **THEN** beta.153 remains unqualified and later smoke or conformance results cannot waive the failure

### Requirement: Greenfield Compose startup and persistence remain simple

The checked-in bundle SHALL remain a standard Compose deployment of NATS with JetStream, SemStreams, and semconnect.
On a fresh named volume it MUST pass empty preflight, canonical seed, health plus indexed/direct query readiness,
normal stop, same-volume restart, and persistence parity. The deployment SHALL NOT require a runtime-unused manifest
or role-specific hash approval.

#### Scenario: A fresh stack restarts over its data

- **WHEN** the exact beta.153 bundle is started, seeded, stopped normally, and restarted over the same named volume
- **THEN** service readiness returns and canonical counts and normalized queries match before and after restart

#### Scenario: The target is not clean

- **WHEN** preflight finds existing semconnect resources or cannot prove the namespace empty
- **THEN** startup fails without deleting or translating anything

### Requirement: External conformance remains complete and unweakened

A fresh-volume beta.153 run against the unchanged Botts authority MUST report exactly
`137 passed, 0 failed, 0 skipped`. The ETS, fixtures, OpenAPI, declarations, filters, skips, and parser MUST NOT be
weakened to obtain green.

#### Scenario: Green changes the authority or scope

- **WHEN** a conformance input or result-processing rule is weakened
- **THEN** the result is rejected and beta.153 remains unqualified
