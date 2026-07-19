## ADDED Requirements

### Requirement: Exact OGC package transfer
Semconnect SHALL own the seven transferred package trees `message/oms`, `parser/sensorml`, `pkg/swecommon`,
`vocabulary/csapi`, `vocabulary/oms`, `vocabulary/sosa`, and `vocabulary/swe`. The initial source SHALL be the exact
SemStreams tree at commit `c8f0b92edf5ad5b491d5f4e81891bec817fae3cd`, including public code, tests, fixtures,
registrations, and applicable license material.

#### Scenario: Provenance is reproducible
- **WHEN** a reviewer compares each transferred tree with the named SemStreams commit
- **THEN** every difference is limited to module import paths or a separately reviewed migration change
- **AND** the evidence records a file manifest and content diff

#### Scenario: Transfer scope is complete
- **WHEN** the migrated repository is searched for removed SemStreams OGC imports
- **THEN** no source, test, fixture, generated artifact, or conformance path imports any of the seven removed paths
- **AND** each replacement package resolves under the semconnect module

### Requirement: Product ownership has no compatibility facade
Semconnect MUST use its owned package paths directly. It MUST NOT publish an old SemStreams import alias, copied
compatibility package, replace directive, or dual registration intended to preserve a removed beta import path.

#### Scenario: Removed imports fail the ownership audit
- **WHEN** an implementation introduces an import under a removed SemStreams OGC path
- **THEN** the package-boundary contract test fails before integration

### Requirement: Transferred behavior retains package parity
The owned packages SHALL preserve the public type, parser, encoder, decoder, schema, vocabulary, JSON, graphable,
and fixture behavior on which semconnect currently depends, except for changes required by the canonical semantic
contract. Parity SHALL be demonstrated at package, semantic, and wire boundaries.

#### Scenario: Package and fixture parity is green
- **WHEN** the transferred package tests and canonical fixture suites run against the beta.147 dependency
- **THEN** all tests pass with the same external OMS, SensorML, SWE Common, SOSA/SWE, and CS API behavior

#### Scenario: Intentional semantic difference is reviewed
- **WHEN** canonical identity enforcement changes an internal graph value
- **THEN** the difference appears in the predicate or entity migration ledger
- **AND** an external wire-parity test proves that the OGC representation did not change unintentionally

### Requirement: OMS payload registration is explicit
Every semconnect binary or test registry that decodes a `message.BaseMessage` with type `ogc.oms.v3` MUST call the
owned OMS registration function before decoding. Code that only unwraps and returns the raw OMS payload SHALL NOT
depend on ambient framework registration.

#### Scenario: Registered OMS envelope decodes
- **WHEN** a decoder registry is explicitly composed with the owned OMS registration
- **THEN** an `ogc.oms.v3` envelope round-trips to the owned typed Observation

#### Scenario: Missing registration fails honestly
- **WHEN** a decoder registry omits the owned OMS registration
- **THEN** decoding fails as an unregistered payload type
- **AND** no test helper or framework builtin silently supplies it

### Requirement: OGC backlog is product-owned
Feasibility vocabulary, association/composition vocabulary, and deferred SWE Common expansion SHALL be tracked as
semconnect work. The beta.147 migration SHALL include issues #70 and #71, while issue #69 SHALL remain a non-goal
unless an independently approved consumer requirement expands the scope.

#### Scenario: Migration closes ownership work only
- **WHEN** the package transfer is accepted
- **THEN** Feasibility and association/composition terms are declared in the owned CS API vocabulary
- **AND** SWE Common Phase 2 behavior is not added merely as part of the dependency bump

