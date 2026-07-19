## ADDED Requirements

### Requirement: Reviewed cutover manifest is a P0 gate
No destructive command SHALL run until an immutable manifest records the exact semconnect and SemStreams commits,
deployment, NATS context and account, rendered component configuration, active writers, resolved bucket names,
deletion set, retained state, authoritative reseed source, readiness target, verification commands, operator,
reviewers, and timestamps.

#### Scenario: Missing manifest blocks cutover
- **WHEN** any required manifest field or approval is absent
- **THEN** the cutover is no-go
- **AND** no writer is stopped and no NATS resource is removed

#### Scenario: Manifest uses rendered deployment values
- **WHEN** configured bucket names differ from framework defaults
- **THEN** the manifest names only the resolved deployed resources
- **AND** no copied default list, wildcard, unresolved variable, or shared-account guess is accepted

### Requirement: Cutover is one stop, wipe, restart, and reseed
The operator SHALL stop every writer to the target graph account, remove only the manifest-approved incompatible
framework graph, guard, and derived-index buckets, start the matching beta.147 deployment against empty graph state,
and reseed from canonical authoritative sources. A legacy `PREDICATE_CATALOG` SHALL be removed only when the manifest
proves that the old deployment created it.

#### Scenario: Writer cannot race the wipe
- **WHEN** the operator begins destructive execution
- **THEN** all listed writers are stopped and verified inactive before the first bucket removal

#### Scenario: Reseed uses only migrated producers
- **WHEN** empty beta.147 graph state is ready for input
- **THEN** only the manifest-listed canonical producers start
- **AND** the evidence records source revision, input count, and resulting `ENTITY_STATES` revision

### Requirement: Product state is preserved by default
The system SHALL preserve the observation JetStream stream, schema-artifact ObjectStore, and unrelated product,
operational, workflow, or source-system state unless the manifest contains a separately reviewed identity-impact
decision.

#### Scenario: Preserved observations remain readable
- **WHEN** canonical graph reseed completes with unchanged resource IDs
- **THEN** retained observation subjects and OMS envelopes remain readable through their existing CS API paths

#### Scenario: Preserved artifacts relink after reseed
- **WHEN** artifact graph entities and relationships are rebuilt
- **THEN** retained ObjectStore content resolves through canonical artifact entities
- **AND** no blanket ObjectStore deletion occurs

#### Scenario: Identity impact blocks automatic preservation
- **WHEN** the minting audit shows that a retained observation or artifact reference would change identity
- **THEN** cutover is no-go until a separate preservation or retirement plan is approved

### Requirement: Readiness is revision-based
After reseed, the operator MUST capture the authoritative `ENTITY_STATES` target revision and poll the graph-index
status endpoint until readiness is true and the indexed revision is at least that target. Silence, a healthy process,
or an arbitrary delay SHALL NOT count as readiness.

#### Scenario: Index has not reached target
- **WHEN** graph-index reports ready false or indexed revision below the captured target
- **THEN** query verification and writer release remain blocked

#### Scenario: State contract poison is fatal
- **WHEN** the deployment reports `graph_state_reset_required`
- **THEN** the operator stops the cutover
- **AND** fixes the producer or injected state before any reviewed rebuild is repeated

### Requirement: Query and replay parity complete the cutover
The migrated deployment MUST prove predicate, relationship, entity, batch, spatial, scoped collection, schema
artifact, and representative CS API query parity after readiness. It MUST then restart without an intervening write
and prove the same readiness revision and results.

#### Scenario: Live rebuild parity passes
- **WHEN** the index reaches the reseed target revision
- **THEN** all manifest query probes match their canonical expected IDs, counts, relationships, and payloads

#### Scenario: Replay parity passes
- **WHEN** the migrated stack restarts with no intervening write
- **THEN** graph readiness returns to the same or later indexed revision
- **AND** the same query probes return equivalent results

### Requirement: Rollback and no-go behavior is explicit
The deployment SHALL permit a normal rollback to beta.141 only before bucket removal. After removal begins, neither
beta.141 against beta.147 state nor beta.147 against retained beta.141 graph state is permitted. The only recovery
SHALL be a stopped, corrected, manifest-reviewed clean rebuild from authoritative sources.

#### Scenario: Pre-destructive failure rolls back normally
- **WHEN** a build, audit, rehearsal, approval, or retained-state gate fails before bucket removal
- **THEN** the existing beta.141 deployment and state remain untouched

#### Scenario: Post-destructive failure fails closed
- **WHEN** startup, reseed, readiness, query parity, replay parity, or external verification fails after removal
- **THEN** writers remain stopped
- **AND** the old binary is not started against the changed account
- **AND** recovery requires a corrected reviewed rebuild
