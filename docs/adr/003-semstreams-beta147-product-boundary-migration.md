# ADR-S003 - SemStreams beta.147 product-boundary migration

- **Status**: Accepted for implementation (2026-07-17)
- **Repo**: `semconnect`
- **Framework target**: `github.com/c360studio/semstreams v1.0.0-beta.147`
- **Source provenance**: SemStreams commit `c8f0b92edf5ad5b491d5f4e81891bec817fae3cd`
- **Execution contract**: `openspec/changes/migrate-semstreams-beta147/`

## Context

SemStreams beta.147 makes a deliberate pre-v1 product-boundary break. The framework removes its OGC Connected
Systems packages, enforces canonical predicate and entity identities, and changes the persisted graph-index layout.
Semconnect cannot consume the release by changing a module version alone: its imports disappear, several current
writers fail closed, and beta.141 graph state is incompatible with beta.147 replay.

The change also corrects an ownership mistake. OMS, SensorML, SWE Common, SOSA/SWE, and CS API behavior belongs with
the product that implements and conforms the OGC API, while SemStreams continues to own graph, mutation/query, NATS,
JetStream, ObjectStore, ownership, and projection primitives.

## Decision

### 1. Semconnect owns the complete OGC package bundle

Semconnect SHALL own equivalent packages at these module-relative paths:

- `message/oms`
- `parser/sensorml`
- `pkg/swecommon`
- `vocabulary/csapi`
- `vocabulary/oms`
- `vocabulary/sosa`
- `vocabulary/swe`

The transfer source is the exact tree at SemStreams commit
`c8f0b92edf5ad5b491d5f4e81891bec817fae3cd`, the reviewed pre-deletion provenance. The transfer includes public
types, implementations, registrations, tests, fixtures, and license obligations. Semconnect package paths replace
the removed SemStreams paths; compatibility packages under the old module path are forbidden.

`vocabulary/csapi` becomes the single owner of CS API graph predicates and boundary IRIs, including Feasibility,
association/composition, and scalar metadata currently declared in gateway files. `ogc.oms.v3` registration becomes
an explicit responsibility of every semconnect composition root or test decoder that decodes OMS envelopes.

### 2. Internal semantic identity is canonical and product-owned

Stored predicates SHALL use exactly three lower-kebab ASCII segments. The migration includes all 19 transferred
package renames, 12 semconnect camelCase-to-lower-kebab corrections, and the distinct correction of the full SOSA
`observedProperty` IRI currently used as an internal predicate. External OGC JSON member names and RDF/JSON-LD IRIs
remain standards-shaped and are not derived by exposing the internal dotted name.

Graph entity IDs SHALL be exact six-part SemStreams identities, at most 256 bytes. Every triple subject and every
object marked with datatype `@id` SHALL satisfy the same contract. Entity relationships SHALL be marked `@id`;
external IRIs and ordinary link hrefs SHALL remain literal boundary values.

Server minting SHALL preserve the current valid identity when it fits. An over-budget suffix SHALL use a stable,
versioned SHA-256 digest token, while the original source identifier remains stored in its semantic UID field. Prefix
configuration SHALL reserve enough space for the digest form. Validators never trim, rewrite, or normalize caller
input.

### 3. There is no graph compatibility mode

Semconnect SHALL NOT add predicate aliases, dual reads or writes, permissive validation, old import facades, or an
in-place beta-state translator. Legacy `cs-api.system.uid` and `cs-api.system.position` read fallbacks SHALL be
removed because incompatible graph state is wiped and beta.147 refuses poisoned state before the gateway can use a
fallback.

The deployment migration is one stop, manifest-derived wipe, restart, and canonical reseed. A reviewed manifest is
a P0 prerequisite and SHALL resolve exact NATS account/context, configured bucket names, writers, retained product
state, reseed source, and verification commands. Framework graph state and derived indexes are disposable;
authoritative product/source data is not.

The observation stream, schema-artifact ObjectStore, and unrelated product or operational state SHALL be preserved
unless the manifest proves that a separate identity migration is necessary. Readiness requires the graph index to
reach the captured post-reseed `ENTITY_STATES` revision, followed by query parity and a no-write restart/replay parity
proof. A fixed delay is not readiness evidence.

### 4. External behavior and conformance do not regress

Public CS API paths, media types, JSON members, JSON-LD/RDF mappings, NATS observation subjects, OMS envelope type,
and UI semantic labels SHALL remain compatible. The SemStreams Go module pin and conformance backend commit SHALL be
the same release revision.

Release requires fresh external evidence of `137 passed, 0 failed, 0 skipped`. Tests, fixtures, and claimed
conformance classes SHALL NOT be weakened, skipped, or removed to obtain that result. Semconnect has no rule-processor
configuration or `graph.events.*` consumer; beta.147 rule/event migration is therefore an explicitly recorded
not-applicable audit, not an inferred omission.

## Amendments to earlier decisions

This ADR supersedes ADR-S001's statements that SemStreams owns SOSA/SWE/OMS vocabularies, SensorML, and SWE Common,
and that semconnect reuses them without owning those packages. ADR-S001's HTTP scope, conformance claims, auth
posture, binary shape, API versioning, and SemStreams graph-backend decision remain in force. Its beta.141 pin and
137-test result become historical baselines.

This ADR amends ADR-S001's upstream-ask discipline: issues that concern the transferred OGC bundle are now
semconnect-owned product work. Graph/NATS/ObjectStore/ownership gaps remain SemStreams asks.

This ADR amends ADR-S002 only for ownership. The graph-versus-artifact storage pattern remains accepted, but CS API
vocabulary, SensorML, and SWE Common implementations now come from semconnect. Artifact graph entities and
relationships are rebuilt during the cutover; retained ObjectStore content remains product state.

## Consequences

Semconnect gains authority to evolve the OGC bundle and closes the ownership delay behind issues #69, #70, and #71.
It also accepts the maintenance and parity burden for those packages. The cutover is intentionally destructive for
derived graph state and has no old-state rollback reader.

Before the wipe, rollback is a normal deployment rollback to beta.141. After the wipe begins, the only recovery is
to stop writers, correct the migrated sources or deployment, repeat the reviewed clean rebuild, and prove readiness
again. Starting beta.141 against beta.147 state, or beta.147 against retained beta.141 graph state, is prohibited.

## Architect sign-off

The architecture is approved for implementation subject to the OpenSpec acceptance gates. Production cutover
authorization remains blocked until the exact manifest, fresh-volume rehearsal, retained-state proof, and rollback
owner are reviewed and signed by the product owner and operator.

