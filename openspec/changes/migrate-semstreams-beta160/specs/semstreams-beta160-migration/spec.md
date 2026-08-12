## ADDED Requirements

### Requirement: Exact beta.160 identity and fresh storage

The migration SHALL align every SemStreams module, backend, test, Compose, and evidence reference to release
`v1.0.0-beta.160`, commit `8403a2218000e45a31c5132fbfe01af42ed04f14`, and source tree
`9ed5dd3792bca63ce87ebf449a180add918f59ed`. Every beta.160 environment SHALL start on newly provisioned NATS
storage and SHALL NOT open, translate, wipe, or reuse retained beta.159 storage.

#### Scenario: A retained deployment volume is discovered

- **WHEN** qualification discovers graph or product state retained from beta.159
- **THEN** beta.160 adoption stops without modifying that state
- **AND** a separate owner-reviewed migration design is required before proceeding

#### Scenario: Same-version persistence is qualified

- **WHEN** a fresh beta.160 deployment is seeded, stopped normally, and restarted without an intervening write
- **THEN** readiness reaches the captured authoritative revision or later
- **AND** graph queries, schema artifacts, and observations retain equivalent results

### Requirement: Typed graph operations are revision-fenced and single-attempt

The gateway SHALL use the typed `semstreams.graph.mutation/v1` family. Create SHALL use strict `entity.create`.
Reconcile and delete SHALL carry the exact nonzero KV revision returned by the entity read that authorized them.
No gateway path SHALL use a removed raw mutation subject or automatically retry a mutation.

#### Scenario: Concurrent reconcile loses its revision race

- **WHEN** `entity.reconcile` returns `revision_mismatch`
- **THEN** the gateway returns HTTP 409 Conflict
- **AND** it sends no second mutation request

#### Scenario: Commit outcome is unknown

- **WHEN** a typed mutation returns `commit_unknown`
- **THEN** the gateway returns HTTP 503 with an explicit uncertainty marker and correlation identifier
- **AND** it does not retry the mutation

#### Scenario: PUT races a concurrent create

- **WHEN** PUT exact-reads a missing entity and its strict create loses to another writer
- **THEN** the gateway returns HTTP 409
- **AND** it does not convert the create into reconcile

#### Scenario: DELETE targets a missing entity

- **WHEN** the exact read for an idempotent DELETE reports that the entity does not exist
- **THEN** the gateway returns HTTP 204 without sending a delete mutation

### Requirement: Exact and batch query shapes are authoritative

Exact reads SHALL decode `graph.ExactEntity`, expose the entity and its KV revision, and reject a missing or zero
revision for guarded writes. Batch reads SHALL map `entities[]` by identity and account for every explicit
`missing[]` entry. `response_too_large` SHALL remain distinct from a transport timeout.

#### Scenario: Exact entity response includes a revision

- **WHEN** the graph returns an exact entity wrapper
- **THEN** the gateway uses the wrapped entity state
- **AND** preserves its KV revision for the next guarded mutation

#### Scenario: Batch response reports missing identities

- **WHEN** a batch query returns hydrated entities and explicit missing IDs
- **THEN** the caller receives the real entities mapped by ID
- **AND** does not manufacture zero-value entities for the missing IDs

### Requirement: Resource-specific projection contracts govern graph facts

Semconnect SHALL declare local projection contracts for System, Datastream, Procedure, Deployment, SamplingFeature,
Property, ControlStream, Command, SystemEvent, Feasibility, and SchemaArtifact. Each contract SHALL use a valid stable
message type, entity pattern, birth predicates, reconcile groups where applicable, and indexing profile.

Entity type SHALL be birth-only. System and Datastream SHALL place endpoint-owned mutable predicates in one atomic
`representation` group. Create-only resources SHALL declare emitted predicates birth-only. Datastream SHALL preserve
valid client-supplied six-part IDs through a six-token wildcard entity pattern.

#### Scenario: A typed resource is created

- **WHEN** a gateway builder creates any supported resource family
- **THEN** `CreateMutation.Entity.Triples` is empty
- **AND** every fact in `CreateMutation.Triples` has the created entity ID as its subject
- **AND** the resource-specific message type and contract validate before I/O

#### Scenario: A relationship target is absent

- **WHEN** a valid root-subject relationship references an entity that has not been created
- **THEN** the relationship remains valid
- **AND** the gateway and graph ingest do not create a stub target entity

### Requirement: SensorML projection is root-entity scoped

A SensorML mutation SHALL retain facts only when their subject is the primary requested entity. It MAY retain valid
root-to-child references. It SHALL drop inverse or other facts whose subject is an embedded child. Creating embedded
child entities in the same request is out of scope.

#### Scenario: SensorML contains an embedded subsystem

- **WHEN** parsing emits root facts, a root-to-child reference, and a child-subject inverse parent fact
- **THEN** the root facts and valid root-to-child reference may enter the root mutation
- **AND** the child-subject inverse fact is absent
- **AND** no child entity mutation is sent

#### Scenario: A child is posted independently

- **WHEN** a child System is posted as the request's primary root and carries a valid parent relation
- **THEN** its own root-subject parent fact is eligible under the System contract

### Requirement: Schema artifacts are immutable and exactly stored

Canonical SWE schema bytes SHALL determine a content-addressed artifact entity ID and object key. An artifact SHALL
be created with its final `StorageReference` and SHALL never be reconciled. A create conflict SHALL exact-read and
verify identical type, digest, provider instance, bucket, and key, or fail as an integrity error.

Storage lookup SHALL use provider instance `objectstore`. That provider SHALL use NATS ObjectStore bucket
`CS_API_ARTIFACTS`. There SHALL be no bucket-name, unnamed-provider, or fallback resolution. Orphan artifact garbage
collection is deferred.

#### Scenario: The same canonical schema already exists

- **WHEN** immutable artifact create returns conflict and exact verification finds the same digest and reference
- **THEN** the parent Datastream may link to the existing artifact
- **AND** no artifact reconcile request is sent

#### Scenario: Artifact metadata conflicts with its digest identity

- **WHEN** the existing artifact's digest or exact storage reference differs
- **THEN** the operation fails as an integrity error
- **AND** neither the artifact nor parent is mutated

### Requirement: Mutation audit is structured at the gateway boundary

After every graph mutation attempt, the gateway SHALL emit structured audit evidence containing request ID, trace ID,
available authenticated or trusted forwarded HTTP identity hints, operation, entity ID, commit state, KV revision,
and classified error. Custom identity-header propagation on the typed mutation wire SHALL NOT be a product
requirement. Existing observation publish headers remain independently governed.

#### Scenario: A mutation fails before a known commit

- **WHEN** a typed graph mutation returns a classified failure
- **THEN** one structured gateway audit record captures the attempt and classification
- **AND** no test or documentation claims that custom identity headers reached the graph mutation wire

### Requirement: Component declarations use canonical beta.160 ports

Every SemStreams component declaration SHALL use canonical envelopes with `name`, `required`, `description`, and a
closed typed `config`. The gateway SHALL declare required typed mutation and graph-query families, canonical NATS
request ports for retained direct queries, and canonical JetStream stream and subjects for observations.

Graph ingest SHALL resolve exactly one typed mutation input provider. Flat fields, aliases, service names, synthetic
HTTP ports, custom objectstore ports, and ignored resolution errors SHALL be absent. Artifact storage SHALL resolve
through the configured exact provider registry.

#### Scenario: Configuration is generated for beta.160

- **WHEN** deployment and conformance configurations are rendered and validated
- **THEN** every port satisfies the beta.160 schema and top-level version
- **AND** graph ingest has exactly one typed mutation provider
- **AND** no removed declaration form remains

### Requirement: NATS 2.14 and external conformance are release gates

Development, conformance, and deployment qualification SHALL use NATS 2.14 and record the exact image digest and
server version. Release SHALL pass formatting, module verification, Go test, race, vet, build, real-NATS integration,
generated schema, clean-volume restart, and external conformance gates.

#### Scenario: External conformance is evaluated

- **WHEN** the aligned beta.160 stack runs the unchanged external ETS from disposable fresh volumes
- **THEN** the result is exactly 137 passed, 0 failed, and 0 skipped
- **AND** independent review confirms no test, fixture intent, OpenAPI path, or claimed conformance class was weakened

### Requirement: Unused beta.160 breaks have explicit disposition

The migration evidence SHALL record trajectories, tool discovery, agent-run milestones, metric server changes,
GraphQL similarity search, aggregate graph clients, component status, and context or structural persistence as not
applicable unless repository usage is discovered. Discovery of usage SHALL reopen its migration task before release.

#### Scenario: The bounded usage audit finds an affected surface

- **WHEN** code or configuration uses a beta.160 surface previously classified as not applicable
- **THEN** release remains blocked
- **AND** the change receives failing-first tests, implementation, review, and documentation disposition
