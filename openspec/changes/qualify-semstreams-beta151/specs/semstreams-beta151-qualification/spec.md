## ADDED Requirements

### Requirement: The requested release is resolved and pinned exactly

The qualification SHALL record that `v1.0.0-beta.515` did not exist when queried on 2026-07-18 and that the request
was resolved to latest release `v1.0.0-beta.151`. Semconnect SHALL align its module and conformance source to beta.151
peeled commit `ac75c322140fb2a6b55759d07a79874b4cb4d9cc`. Mixed tag, commit, source, image, or module
identities are forbidden.

#### Scenario: A requested or executable identity diverges
- **WHEN** a pin resolves to beta.515, another version, or a commit other than `ac75c322...`
- **THEN** automated alignment fails before stack startup
- **AND** qualification and production approval remain blocked

### Requirement: The complete beta.149-to-beta.151 delta has an explicit disposition

Evidence SHALL enumerate all nine commits from beta.149 through beta.151, including structural gate PR #554, rule
shutdown PR #561, and trusted owner-RMW PR #567. Source, configuration, and subject audits SHALL distinguish live
semconnect exposure from unconfigured rule, agentic, research, Graphable, documentation, and test-only changes.

#### Scenario: An audited subsystem is unexpectedly live
- **WHEN** an enabled component, publisher, subscriber, or subject consumes a delta classified as not applicable
- **THEN** qualification stops
- **AND** a behavior contract and tests are required before work continues

### Requirement: No legacy or compatibility behavior is introduced

Beta.151 qualification SHALL NOT add aliases, predicate rewriting, relaxed identity parsing, dual reads/writes,
compatibility branches, cleanup lanes, migration commands, or test-only bypasses. Production SHALL target only a new,
empty NATS namespace. Pre-existing target state or required behavior change SHALL reopen architecture review.

#### Scenario: Green requires compatibility code
- **WHEN** any local, structural, replay, or external gate requires a legacy path or relaxed assertion
- **THEN** the candidate is rejected
- **AND** no later green result may waive the rejection

### Requirement: Structural mutations fail closed without changing state

Every beta.151 graph-ingest write exposed to semconnect SHALL preserve the six-part entity-ID, three-part predicate,
canonical reference, and foreign-edge contracts. A malformed direct mutation MUST be rejected atomically with a
classified validation error and MUST NOT change entity bytes, entity revision, bucket revision, or indexes.

#### Scenario: Invalid create is rejected atomically
- **WHEN** `entity.create_with_triples` carries an invalid ID, predicate, or entity reference
- **THEN** no entity or partial triple is persisted
- **AND** rejection logs/metrics identify the lane and structural reason

#### Scenario: Invalid update cannot partially modify a valid entity
- **GIVEN** a valid retained entity with captured bytes and revision
- **WHEN** `entity.update_with_triples` contains valid changes plus one structurally invalid triple
- **THEN** the whole update fails and captured bytes/revision remain unchanged

#### Scenario: A canonical foreign edge still routes
- **WHEN** a hosted-child fixture emits a valid foreign-subject `isHostedBy` edge
- **THEN** the edge remains claimed and routed under the inherited canonical contract
- **AND** unclaimed and dropped foreign-edge counters remain zero with the lane positively exercised

### Requirement: Trusted owner RMW decode cannot launder resident poison

Owner-only RMW reads MAY use beta.151 trusted decode, but every committed candidate MUST pass authoritative
`MarshalEntityState` validation. External readers MUST retain validated decode. Resident malformed JSON or structural
poison MUST fail with reset-required classification and MUST NOT be rewritten, merged, or revision-bumped.

#### Scenario: Resident poison reaches an RMW attempt
- **GIVEN** adversarially injected noncanonical stored state in an isolated upstream test bucket
- **WHEN** graph-ingest attempts merge, add, batch-add, update-with-triples, or remove
- **THEN** authoritative validation prevents commit and attributes the failure to stored-state poison
- **AND** stored bytes and revision remain unchanged

#### Scenario: A valid remove is a true no-op
- **GIVEN** canonical state without the requested predicate
- **WHEN** remove is requested
- **THEN** it succeeds without a KV write, revision bump, or watcher re-fire

### Requirement: Production first start is greenfield only

The operator SHALL resolve the production NATS server, account, domain, and persistent volume and prove that the target
contains no semconnect graph, index, observation, artifact, or guard resource before first start. The production bundle
SHALL contain no delete, wipe, translation, migration reseed, or old-state compatibility operation.

#### Scenario: Target state already exists
- **WHEN** the preflight finds any target semconnect resource or cannot prove the namespace empty
- **THEN** production remains no-go and no resource is changed
- **AND** a separate architecture decision is required

### Requirement: Local and focused upstream verification is green

The aligned revision MUST pass downstream unit, race, real-NATS integration, vet, integration vet, and build gates.
It MUST also pass focused upstream structural gate, trusted-RMW, rule persistence, graph-index synchronization, and
service lifecycle tests. Tests SHALL demonstrate failing-then-passing TDD evidence where beta.149 lacks the new proof.

#### Scenario: A required gate fails or is skipped
- **WHEN** any required command fails, is filtered out, or needs an undocumented exception
- **THEN** beta.151 is not qualified
- **AND** external conformance cannot waive the failure

### Requirement: The production Compose bundle is pinned and persistent

Production SHALL use a checked-in Compose bundle containing NATS with JetStream, SemStreams beta.151, and semconnect.
Committed source revisions, immutable image digests or reproducible build inputs, rendered non-secret configuration,
and the Compose model SHALL be hash-bound. NATS file storage SHALL use an explicit named persistent volume.

#### Scenario: A production input is mutable or storage is ephemeral
- **WHEN** an image uses only a mutable tag, source is dirty/uncommitted, rendered configuration drifts, or NATS uses an
  anonymous/ephemeral volume
- **THEN** static validation fails and the bundle cannot start

### Requirement: Configuration has an internal-only security boundary

Checked-in Compose and service configuration SHALL contain only non-secret values. NATS SHALL publish no host port and
SHALL be reachable only on the private Compose network. This topology has no credential or secret interface.

#### Scenario: NATS is exposed outside the private network
- **WHEN** the rendered Compose model publishes a NATS port or adds an external NATS endpoint
- **THEN** this greenfield bundle is invalid and requires a separate security design

### Requirement: Seed, readiness, and restart prove greenfield persistence

After empty-namespace proof, the operator SHALL start the exact bundle, run a hash-bound canonical seed, and capture
expected resource counts, entity ID, and normalized queries. Readiness MUST prove NATS health and JetStream
availability, SemStreams health, semconnect health, and discovery of the canonical entity through the
graph-index-backed collection and direct item query. The accepted `137/0/0` result remains the broad API proof.

The operator SHALL then stop the Compose services by normal SIGTERM without removing the volume, restart the identical
bundle over that volume, and prove equivalent collection/item counts and normalized results.

#### Scenario: Clean restart preserves the seeded deployment
- **WHEN** the exact production bundle stops and restarts over its named volume
- **THEN** SemStreams and semconnect exit zero, NATS records complete JetStream shutdown without OOM, and seeded/query
  evidence is equivalent

#### Scenario: Readiness or persistence evidence fails
- **WHEN** a service is unhealthy, JetStream is unavailable, the indexed entity stays undiscoverable, exit is
  forced/nonzero, the volume changes, or any count/query differs
- **THEN** production remains no-go
- **AND** fixed sleeps, health alone, volume recreation, or compatibility behavior cannot waive the failure

### Requirement: External conformance remains complete and unweakened

A fresh-volume beta.151 run against the unchanged Botts authority MUST report exactly
`137 passed, 0 failed, 0 skipped`. Independent review SHALL prove the ETS pin/tests, fixtures, OpenAPI, conformance
claims, filters, skips, and result parsing were not weakened from the accepted beta.149 evidence.

#### Scenario: Green changes scope or authority
- **WHEN** any test authority, fixture intent, claim, filter, skip, or parser changes to obtain green
- **THEN** the result is rejected
- **AND** beta.151 remains unqualified

### Requirement: One immutable bundle manifest and two role decisions gate production

Task 6.3 SHALL produce an approval-free immutable manifest binding deployment values, committed source and image
identities, Compose/config hashes, the internal-only/no-secret boundary, named volume, empty-namespace proof,
seed/readiness/restart commands and evidence, and the accepted external `137/0/0` qualification. It SHALL contain no
destructive, migration, retained-state compatibility, or old-deployment rollback requirement.

Task 6.4 SHALL record explicit product-owner and operator `go` or `no-go` decisions over the same manifest SHA-256. One
person MAY hold both roles only through two role-specific attestations. Separate architect and destructive-reviewer
production approvals are not required.

#### Scenario: Both roles approve the unchanged bundle
- **WHEN** tasks 6.1-6.3 pass and product owner and operator each record `go` over the same current manifest hash
- **THEN** the stated first greenfield deployment is authorized

#### Scenario: An approval or bundle input is missing or stale
- **WHEN** a role is absent/no-go, hashes differ, or any environment, source, image, config, network boundary, volume,
  seed, readiness, restart, or evidence input changes
- **THEN** production remains unauthorized and new task 6.4 decisions are required
