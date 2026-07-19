## ADDED Requirements

### Requirement: Public CS API wire behavior is stable
The migration MUST preserve public paths, methods, status mappings, media types, negotiation rules, JSON member names,
JSON-LD/RDF IRIs, links, OpenAPI declarations, and claimed conformance classes. Internal lower-kebab predicates MUST
NOT leak into standards-shaped JSON member names.

#### Scenario: Wire golden parity passes
- **WHEN** representative pre- and post-migration requests are compared
- **THEN** their externally specified status, headers, media type, JSON shape, links, and semantic IRIs are equivalent
- **AND** any intentional difference is approved as a separate public API change

### Requirement: UI presents stable semantic labels
The UI SHALL use explicit product-owned display labels and descriptions for migrated predicates and resource fields.
It MUST NOT derive user-facing text by exposing internal lower-kebab predicate identifiers or stale SemStreams
package metadata.

#### Scenario: Migrated field remains understandable
- **WHEN** the UI renders Datastream, ControlStream, Command, SystemEvent, Feasibility, association, or artifact data
- **THEN** the label matches the existing CS API terminology
- **AND** no camelCase legacy predicate or lower-kebab storage identity appears as unintended user-facing text

### Requirement: NATS and dependency revisions are aligned
The semconnect Go module, conformance backend source, generated evidence, and cutover manifest MUST name the same
SemStreams beta.147 commit. Existing CS API observation subjects and the `ogc.oms.v3` BaseMessage envelope identity
MUST remain unchanged.

#### Scenario: Dependency alignment gate passes
- **WHEN** release evidence is assembled
- **THEN** the Go module version, backend tag, backend commit, and documented migration target resolve to beta.147

#### Scenario: Observation transport parity passes
- **WHEN** an observation is published and read after migration
- **THEN** its JetStream subject follows the existing `cs-api.observations.{datastreamID}` contract
- **AND** its envelope type remains `ogc.oms.v3`
- **AND** audit and trace headers retain their prior meaning

### Requirement: Rule and graph-event migration is explicitly not applicable
The release evidence MUST include a bounded source and configuration audit proving that semconnect has no
rule-processor configuration, rule pack, graph-event constructor, or `graph.events.*` consumer affected by the
beta.147 rule/event break. A future introduction of one SHALL require its own explicit beta.147 contract.

#### Scenario: Not-applicable audit is recorded
- **WHEN** source, generated configuration, deployment configuration, and subscriptions are scanned at the release
  commit
- **THEN** the evidence records the exact scope and zero behavior-bearing matches
- **AND** no default `pack_id` or `enable_graph_integration` assumption is introduced

### Requirement: Complete automated quality gates are green
The migration MUST pass formatting, lint, full Go tests, Go race tests, Go vet, Go build, Svelte/TypeScript checks,
UI unit tests, UI build, package parity, semantic-state contract tests, NATS integration tests, and the conformance
stack. Test behavior and critical paths SHALL be preserved rather than replaced by implementation-only assertions.

#### Scenario: Engineering gate evidence is complete
- **WHEN** reviewers evaluate the release candidate
- **THEN** every required command, revision, environment, result, and artifact is recorded
- **AND** no required suite is skipped, cached as fresh evidence, or silently narrowed

### Requirement: External ETS result is immutable
The final candidate MUST run the externally sourced Botts OGC CS API ETS from fresh volumes and produce exactly
`137 passed, 0 failed, 0 skipped`. The migration MUST NOT modify, filter, disable, or replace ETS tests or weaken
semconnect conformance declarations, seed intent, or assertions to obtain that result.

#### Scenario: Fresh external conformance gate passes
- **WHEN** the final aligned candidate runs the pinned external ETS from a clean stack
- **THEN** the archived TestNG result reports 137 passed, 0 failed, and 0 skipped
- **AND** evidence proves the ETS pin, tests, fixture intent, and claimed conformance set were not weakened

#### Scenario: Any conformance delta blocks release
- **WHEN** the ETS reports a failure or skip, or evidence shows test or claim weakening
- **THEN** the release is no-go regardless of unit-test or local smoke results

