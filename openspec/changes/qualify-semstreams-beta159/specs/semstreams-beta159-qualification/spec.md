## ADDED Requirements

### Requirement: Beta.159 is pinned by exact identity

Semconnect SHALL align every executable SemStreams pin to `v1.0.0-beta.159`, tag object
`ba2c4f8e03bb42c56319b2363ca89f4ab0f9ccec`, peeled commit
`8813270c5ba441286d9120cba82fbf72bdcf9a6c`, and source tree
`c9a8014cbf6b91837769c8fb8a3dea051f6f11d9`.

#### Scenario: Any pin differs

- **WHEN** the module, conformance source, Compose build, labels, or tests identify another release or commit
- **THEN** alignment fails before runtime qualification

### Requirement: Readiness uses fresh GRAPH_STATUS updates

The conformance gate SHALL watch new graph-index updates in `GRAPH_STATUS`, capture two consecutive equal non-zero
targets, and proceed only when bootstrap is complete, the readiness gate is healthy, and indexed revision covers the
captured and currently observed targets. It MUST NOT use removed `graph.index.query.status` or accept retained
pre-subscription state.

#### Scenario: A fresh caught-up status is published

- **WHEN** two live updates report the same target with bootstrap complete, healthy ready state, and full coverage
- **THEN** the gate records the captured/final revisions and allows conformance to start

#### Scenario: Readiness evidence is unsafe

- **WHEN** the key is deleted, an update is malformed, the target regresses, state is degraded/reset-required, the
  watcher closes, or the timeout expires
- **THEN** the gate fails closed and archives the reason

### Requirement: The observations stream has explicit capacity and age bounds

The CS API observations stream SHALL use file storage and `LimitsPolicy`, retain observations for at most 30 days,
cap stored bytes at 1 GiB, and use `DiscardOld`. Configuration MUST reject a zero or negative byte cap.

#### Scenario: The component starts with defaults

- **WHEN** semconnect ensures the observations stream on clean NATS
- **THEN** JetStream reports 1 GiB, 30 days, and `DiscardOld` without changing observation HTTP behavior

#### Scenario: An unbounded stream is configured

- **WHEN** `observations_max_bytes` is zero or negative
- **THEN** configuration validation fails before the component starts

### Requirement: Live graph mutation behavior remains canonical and fail-closed

Beta.159 SHALL preserve semconnect's six-part entity IDs, three-part predicates, canonical references, foreign-edge
routing, and classified mutation/query behavior. Invalid direct input MUST cause no entity bytes, entity revision,
bucket revision, or index change. Resident poison MUST remain scoped and recover after out-of-band repair. A repeated
identical six-field add and an absent remove MUST NOT write or advance revisions.

#### Scenario: Canonical resources are created and updated

- **WHEN** semconnect writes canonical System and hosted-child fixtures through beta.159 mutation subjects
- **THEN** exact triples persist and the claimed foreign edge is queryable

#### Scenario: An invalid mutation or poisoned entity is encountered

- **WHEN** direct input violates the graph contract or resident bytes are invalid
- **THEN** beta.159 rejects or scopes the failure with the expected classification and preserves unrelated state

#### Scenario: An add is duplicated or a remove has no match

- **WHEN** the same six-field triple is added twice or an absent predicate is removed
- **THEN** the response honestly reports deduplication/removal and no entity or bucket revision advances

### Requirement: No legacy or old-volume behavior is introduced

Qualification SHALL NOT add compatibility aliases, dual reads or writes, predicate rewriting, relaxed validation,
cleanup lanes, migration commands, or old-state support. Production targets a clean pre-v1 NATS volume, and the
evidence SHALL NOT claim an in-place beta.153 volume migration.

#### Scenario: Green requires compatibility behavior

- **WHEN** any required gate needs legacy handling, an old-volume assumption, or a weakened assertion
- **THEN** the candidate is rejected and architecture review reopens

### Requirement: Local and focused upstream verification is green

The exact beta.159 pin MUST pass full downstream Go test, race, vet, module verification, build, and live-NATS
structural integration. Focused upstream race tests MUST cover readiness, NATS client, config, service, graph-ingest,
graph-index, and graph-query. Unused feature additions SHALL remain absent by configuration/import audit.

#### Scenario: A required test fails or is skipped

- **WHEN** a required command fails, is filtered out, or needs an undocumented exception
- **THEN** beta.159 remains unqualified and later smoke or conformance results cannot waive the failure

### Requirement: Clean-volume Compose startup and persistence remain simple

The checked-in bundle SHALL remain a standard Compose deployment of NATS with JetStream, SemStreams, and semconnect.
On a clean named volume it MUST pass empty preflight, canonical seed, health plus query readiness, normal stop,
same-volume restart, and persistence parity. The deployment SHALL NOT require a runtime manifest or role-specific
hash approval.

#### Scenario: A clean stack restarts over its beta.159 data

- **WHEN** the exact beta.159 bundle is started, seeded, stopped normally, and restarted over the same named volume
- **THEN** service readiness returns and canonical counts and normalized queries match before and after restart

#### Scenario: The initial target is not clean

- **WHEN** preflight finds existing resources or cannot prove the namespace empty
- **THEN** startup fails without deleting or translating anything

### Requirement: External conformance remains complete and unweakened

A fresh-volume beta.159 run against the unchanged Botts authority MUST report exactly
`137 passed, 0 failed, 0 skipped`. The ETS, fixtures, OpenAPI, declarations, filters, skips, parser, and harness
authority MUST NOT be weakened to obtain green.

#### Scenario: Green changes the authority or scope

- **WHEN** a conformance input or result-processing rule is weakened
- **THEN** the result is rejected and beta.159 remains unqualified
