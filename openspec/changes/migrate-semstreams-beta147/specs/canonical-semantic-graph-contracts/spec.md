## ADDED Requirements

### Requirement: Transferred predicate rename ledger is complete
The owned package transfer MUST apply these 19 canonical predicate identities and MUST retain an auditable exact
old-to-new ledger:

| Previous | Canonical |
|---|---|
| `oms.observation.hasFeatureOfInterest` | `oms.observation.has-feature-of-interest` |
| `oms.observation.hasSimpleResult` | `oms.observation.has-simple-result` |
| `oms.observation.observedProperty` | `oms.observation.observed-property` |
| `oms.observation.phenomenonTime` | `oms.observation.phenomenon-time` |
| `oms.observation.resultTime` | `oms.observation.result-time` |
| `oms.observation.usedProcedure` | `oms.observation.used-procedure` |
| `sensorml.component.isHostedBy` | `sensorml.component.is-hosted-by` |
| `sensorml.process.attachedTo` | `sensorml.process.attached-to` |
| `sensorml.process.hasSubSystem` | `sensorml.process.has-sub-system` |
| `sensorml.process.usedProcedure` | `sensorml.process.used-procedure` |
| `csapi.command.partOfControlStream` | `csapi.command.part-of-control-stream` |
| `csapi.controlstream.commandSchema` | `csapi.controlstream.command-schema` |
| `csapi.controlstream.controlsSystem` | `csapi.controlstream.controls-system` |
| `csapi.datastream.phenomenonTimeRange` | `csapi.datastream.phenomenon-time-range` |
| `csapi.datastream.producedBy` | `csapi.datastream.produced-by` |
| `csapi.datastream.resultSchema` | `csapi.datastream.result-schema` |
| `csapi.datastream.resultTimeRange` | `csapi.datastream.result-time-range` |
| `csapi.datastream.resultType` | `csapi.datastream.result-type` |
| `csapi.systemevent.forSystem` | `csapi.systemevent.for-system` |

#### Scenario: Transferred predicates pass the canonical parser
- **WHEN** every canonical value in the transferred ledger is parsed by the beta.147 predicate parser
- **THEN** all 19 values pass as exact three-part lower-kebab predicates
- **AND** every previous value is absent from write, query, registration, fixture, and seed paths

### Requirement: Semconnect predicate correction ledger is complete
Semconnect MUST apply these 12 local camelCase-to-lower-kebab corrections:

| Previous | Canonical |
|---|---|
| `cs-api.controlstream.inputName` | `cs-api.controlstream.input-name` |
| `cs-api.controlstream.commandFormat` | `cs-api.controlstream.command-format` |
| `cs-api.controlstream.controlledProperties` | `cs-api.controlstream.controlled-properties` |
| `cs-api.controlstream.issueTime` | `cs-api.controlstream.issue-time` |
| `cs-api.controlstream.executionTime` | `cs-api.controlstream.execution-time` |
| `cs-api.command.issueTime` | `cs-api.command.issue-time` |
| `cs-api.command.executionTime` | `cs-api.command.execution-time` |
| `cs-api.datastream.phenomenonTime` | `cs-api.datastream.phenomenon-time` |
| `cs-api.datastream.resultTime` | `cs-api.datastream.result-time` |
| `cs-api.property.baseProperty` | `cs-api.property.base-property` |
| `cs-api.deployment.deployedSystems` | `cs-api.deployment.deployed-systems` |
| `cs-api.samplingfeature.hostedProcedure` | `cs-api.samplingfeature.hosted-procedure` |

The helper-mediated `deployedSystems` writer MUST be covered even though a direct `Predicate:` source audit does not
discover it.

#### Scenario: All local graph builders pass final-state validation
- **WHEN** every resource builder produces its final `graph.EntityState`
- **THEN** authoritative beta.147 marshal validation accepts every predicate
- **AND** no legacy local predicate is written, queried, registered, or seeded

### Requirement: Full IRI is not used as an internal predicate
The current use of `sosa.ObservedProperty`, whose value is the full SOSA `observedProperty` IRI, MUST be replaced by
the registered internal predicate `csapi.datastream.observed-property`. The full SOSA IRI SHALL remain only the
boundary mapping for RDF/JSON-LD export.

#### Scenario: Datastream observed property separates internal and boundary identity
- **WHEN** a Datastream with an observed property is persisted and exported
- **THEN** graph state contains `csapi.datastream.observed-property`
- **AND** RDF/JSON-LD exposes the registered SOSA `observedProperty` IRI
- **AND** graph state never stores an HTTP IRI in the predicate field

### Requirement: Predicate syntax is fail-closed before backend I/O
Every persisted predicate MUST contain exactly three lower-kebab ASCII segments, each no longer than 64 bytes, with
the complete predicate no longer than 194 bytes. Invalid request-derived graph state MUST fail before any NATS,
JetStream, ObjectStore, or graph-backend I/O.

#### Scenario: Invalid predicate causes no side effect
- **WHEN** a request or builder produces a predicate that violates arity, character, case, hyphen, or byte limits
- **THEN** the gateway returns a classified client error
- **AND** request spies observe zero backend and storage calls

### Requirement: Entity and reference identity is exact
Every graph `EntityState.ID` and triple subject MUST be an exact six-part identity no longer than 256 bytes. Each
part MUST match `[A-Za-z0-9][A-Za-z0-9_-]*`. Every entity relationship object MUST be a string marked with datatype
`@id` and MUST satisfy the same identity contract. External IRIs and OGC link hrefs MUST remain literals and MUST NOT
be marked `@id`.

#### Scenario: Invalid entity ID causes no backend I/O
- **WHEN** a graph resource path, body ID, parent, child, system, control stream, or artifact reference is invalid
- **THEN** the gateway rejects it before mutation, query, stream, or ObjectStore access

#### Scenario: Invalid explicit reference causes no partial write
- **WHEN** a triple set contains an `@id` object that is non-string or noncanonical
- **THEN** the complete entity candidate is rejected atomically
- **AND** no primary or foreign entity write occurs

#### Scenario: Boundary link remains a literal
- **WHEN** a CS API association contains an external HTTP href rather than a graph entity ID
- **THEN** the gateway preserves the href as boundary data without `@id` datatype

### Requirement: Server minting is bounded and deterministic
All server-minted resource, artifact, command, feasibility, and nested SensorML child IDs MUST use one shared minting
contract. A currently valid result SHALL remain byte-identical. If the sanitized suffix exceeds the available budget,
the suffix SHALL become `h-` followed by the lowercase full SHA-256 hex digest of the exact source bytes. The original
source UID SHALL remain in its semantic field, and configuration validation MUST reserve space for the digest form.

#### Scenario: Existing short identity is stable
- **WHEN** a source identifier produces a canonical ID at most 256 bytes
- **THEN** minting returns the current deterministic identity unchanged

#### Scenario: Long identity uses bounded digest
- **WHEN** the ordinary suffix would exceed the remaining ID byte budget
- **THEN** minting returns the six-part digest form within 256 bytes
- **AND** repeated minting of the same exact source bytes returns the same identity
- **AND** the original source identifier remains available through the resource UID

#### Scenario: Prefix cannot support digest form
- **WHEN** configured five-part prefix length leaves insufficient space for `h-` plus a full SHA-256 digest
- **THEN** configuration validation fails before component initialization
- **AND** no NATS connection or backend request is made for the component

### Requirement: Legacy graph fallbacks are removed
The gateway MUST remove legacy reads for `cs-api.system.uid` and `cs-api.system.position`. The migration MUST NOT add
aliases, dual reads, dual writes, permissive predicate handling, or an in-process old-state translator.

#### Scenario: Source contains no legacy fallback
- **WHEN** the migrated source and tests are audited
- **THEN** the legacy UID and position predicates are absent from operational read and write paths
- **AND** compatibility behavior is provided only by canonical reseed from authoritative sources

