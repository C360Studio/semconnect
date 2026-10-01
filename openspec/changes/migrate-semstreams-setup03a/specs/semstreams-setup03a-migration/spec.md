# Shared SETUP 03A migration qualification

## ADDED Requirements

### Requirement: Reuse the frozen shared framework revision

SemConnect SHALL use SemStreams `v1.0.0-beta.162.0.20260930150212-8b99efe9c66a`, commit
`8b99efe9c66a4faa4fa509f9f62cc6bad8392128`, tree `605f83cb8492eda3bd347ac30a211b9babf3931f`,
as frozen by SemEngine SETUP 03A. It SHALL NOT independently select a newer tag or main revision.
The baseline SHALL remain identified as beta.160, commit `8403a2218000e45a31c5132fbfe01af42ed04f14`.

#### Scenario: A later release becomes available

- **GIVEN** the shared baseline is already frozen
- **WHEN** a newer SemStreams tag or main commit is discovered
- **THEN** the migration retains the frozen revision pending a shared owner decision
- **AND** module, backend, deployment and evidence pins remain aligned

### Requirement: Preserve the baseline before dependency edits

The migration SHALL capture current Go, real-NATS, external conformance, persistence and spatial evidence before
changing dependencies. Each framework revision SHALL use isolated fresh storage and only its own restart volume.
NATS SHALL remain exactly 2.14.4. Cross-version volume reuse SHALL NOT be used for qualification.

#### Scenario: The target is qualified after a baseline run

- **GIVEN** committed beta.160 evidence and a preserved baseline volume
- **WHEN** the target stack is started
- **THEN** its empty storage preflight and distinct volume identity are recorded
- **AND** normal no-write restarts compare graph, artifact, observation and spatial results against the baseline

### Requirement: Adapt framework seams while retaining consumer contracts

The SemConnect-owned graph host SHALL register all eleven resource payloads with their bound local contracts.
Structured message types and neutral vocabulary declarations SHALL preserve wire identity, root-only SensorML
projection, standard IRIs and inverse mappings, immutable artifact bytes and final graph StorageReference.
Typed create/reconcile/delete SHALL retain exact revision fences and one-attempt conflict/uncertainty handling.
Invalid local projection input SHALL fail before backend I/O. Absent relationship targets SHALL remain absent.

#### Scenario: A stale write or uncertain commit occurs

- **GIVEN** a mutation authorized by one exact read revision
- **WHEN** the graph reports conflict or the attempted commit has an unknown outcome
- **THEN** the gateway retains HTTP 409 or HTTP 503 with uncertainty and correlation respectively
- **AND** it does not send a hidden retry

#### Scenario: A projected SensorML root references an absent child

- **WHEN** the gateway creates the validated root
- **THEN** only root-subject facts enter its mutation
- **AND** a valid root-to-child reference may remain without creating child facts or a target stub

### Requirement: Retain lifecycle and platform identity authority

Startup cancellation SHALL abort incomplete initialization. Controlled Stop SHALL drain and join under the caller's
bounded context before runtime cancellation. Stable public deployment identity SHALL use explicit atomic provisioning
and verified adoption; a different existing identity SHALL never be overwritten.

#### Scenario: A signal races successful startup

- **WHEN** shutdown arrives while startup acquires resources
- **THEN** the startup bridge is joined and acquired resources receive bounded cleanup
- **AND** successful ordinary startup detaches that bridge before readiness

### Requirement: Release only with unchanged consumer qualification

Qualification SHALL retain all 137 external ETS assertions, fixtures, filters, parsing, OpenAPI and conformance claims.
It SHALL require 137 passed, zero failed, zero skipped, all required Go/integration gates, real-NATS lifecycle and
restart evidence, difference attribution, and independent Go review. Intended upstream changes SHALL NOT waive
consumer regressions. A failed mandatory assertion SHALL keep the migration draft and merge approval withheld.

#### Scenario: Foreign Datastream creation is rejected at the target

- **GIVEN** beta.160 accepts a valid client-supplied six-part foreign-authority Datastream ID with HTTP 201
- **WHEN** the target returns HTTP 400 with `authority_foreign`
- **THEN** the expected-201 assertion remains active and blocks migration qualification
- **AND** passing external ETS results do not narrow the accepted identity contract or override the blocker

### Requirement: Supply consumer references and measured extraction obligations

SemConnect SHALL supply executable paired reference cases for SensorML/vocabulary, exact-revision typed mutations
with an absent target, and immutable bytes with graph StorageReference. It SHALL measure actual consumer and bounded
backend imports plus every retained framework package's tagged-test closure, with package additions and dispositions.
SemEngine admission decisions SHALL remain separate from measured dependency cost.

#### Scenario: SemEngine evaluates the handoff

- **WHEN** the reference cases and closure are supplied to the retained-contract matrix
- **THEN** OGC semantics and conformance ownership remain in SemConnect
- **AND** a later SemEngine switch requires its own qualified contract and no mixed-framework binary
