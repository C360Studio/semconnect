## Context

Semconnect currently compiles against SemStreams beta.141 and imports an OGC bundle that beta.147 deletes. The same
release makes predicates and entity IDs authoritative, changes predicate-index representation and replacement
semantics, and rejects incompatible replayed graph state. ADR-S003 governs the durable ownership, identity, and
cutover decisions; this design explains how the implementation satisfies those decisions.

The stakeholders are the semconnect product owner, Go and Svelte developers, reviewers, technical writer, the
operator responsible for the target NATS account, and users who depend on the claimed CS API behavior. The external
Botts ETS remains the conformance authority.

## Goals / Non-Goals

**Goals:**

- Make semconnect compile and operate against beta.147 with an explicit owned OGC package boundary.
- Enforce canonical graph identity before side effects while preserving standards-shaped external behavior.
- Consume one destructive maintenance window for entity, predicate, and graph-index changes.
- Preserve observation, artifact, and unrelated product state unless an exact identity audit proves otherwise.
- Produce reproducible package, semantic, NATS, UI, build, cutover, and external conformance evidence.

**Non-Goals:**

- SWE Common Phase 2, Part 3 pub/sub, a second graph backend, or public CS API versioning changes.
- Compatibility imports, predicate aliases, dual graph formats, or in-place beta-state migration.
- New rule/event behavior; semconnect's bounded audit currently classifies that beta.147 surface as not applicable.

## Decisions

### Transfer from one exact pre-deletion revision

The seven package trees come from SemStreams commit `c8f0b92edf5ad5b491d5f4e81891bec817fae3cd`. A generated
manifest records source path, destination path, and content hash. Initial differences are import ownership changes;
canonical migration changes are separate commits or reviewable diff sections.

This is preferred over copying beta.141 because the named provenance includes the reviewed pre-v1 contract work. It
is preferred over inventing replacement models because the existing gateway and fixtures already encode the needed
package behavior.

### Preserve package topology under the semconnect module

The destination paths mirror the removed paths beneath `github.com/c360studio/semconnect`. This keeps package names
and cohesion while making ownership visible at every import. Moving everything under the gateway package was
rejected because OMS, vocabularies, parsers, and SWE codecs are reusable product primitives rather than HTTP details.

The owned `vocabulary/csapi` absorbs gateway-local Feasibility, association/composition, and scalar graph constants.
Boundary IRI constants remain distinct from dotted storage predicates. The registry owns display metadata used by
JSON-LD/RDF export and the UI; runtime graph persistence still enforces syntax independently of registry metadata.

### Apply one explicit semantic rename ledger

The canonical-semantic spec is the source of truth for 19 transferred renames, 12 local camelCase corrections, and
the separate full-IRI-as-predicate correction. Producers, exact queries, registrations, fixtures, and seed paths move
together. No lookup checks both identities.

The `sosa.ObservedProperty` bug is handled as a boundary violation, not a spelling change: graph state uses
`csapi.datastream.observed-property`, registered to the SOSA IRI for export. This prevents an HTTP IRI from entering
the fixed-arity predicate index.

Legacy UID and position fallback predicates are deleted. They cannot recover beta.141 graph state because beta.147
rejects incompatible authoritative state before a gateway fallback could execute.

### Use framework validators and mark entity references

Graph-facing IDs use the beta.147 public validators rather than a semconnect regex copy. Opaque observation resource
IDs and HTTP path tokens use a separately named validator so callers cannot accidentally treat them as graph IDs.

All graph relationships set datatype `message.EntityReferenceDatatype` (`@id`). This moves reference validation into
the authoritative final-state seam. HTTP hrefs, external IRIs, and scalar strings remain unmarked literals.

Every handler validates request-derived IDs and the complete final entity candidate before NATS, JetStream, or
ObjectStore calls. Spy tests prove zero side effects for invalid input.

### Bound minting without changing valid existing IDs

A shared minting helper accepts a validated five-part prefix and exact source bytes. It applies the existing token
mapping when the resulting six-part ID fits. If it does not fit, the instance token becomes `h-` plus the lowercase
full SHA-256 hex digest of the exact source bytes. The prefix validator reserves 67 bytes for the separator and
digest token. The original UID remains in its semantic field.

Using the full digest avoids a new collision budget. Hashing every identity was rejected because it would needlessly
change stable externally visible IDs and retained observation subjects. Truncation was rejected because it creates
collision and normalization ambiguity.

Nested SensorML child IDs and schema artifacts use domain-separated source bytes that include the parent identity,
role, and local identifier. Domain separators are fixed constants covered by golden tests.

### Make decoder composition explicit

The owned OMS package exposes `RegisterPayloads`. Production or test composition roots call it only when they create
a typed decoder. The observation read path that intentionally returns raw posted OMS JSON can remain registry-free.
This makes payload ownership inspectable and avoids ambient behavior from SemStreams builtins.

### Align NATS wire and dependency revisions

The Go module and conformance backend pin move together to beta.147's commit. Graph request subjects remain framework
contracts. Observation stream names, `cs-api.observations.{datastreamID}` subjects, `ogc.oms.v3` envelope identity,
and audit/trace headers remain product wire contracts and receive integration parity tests.

The graph-index layout change is server-owned and does not alter the predicate-query request API. It does require the
clean derived-state rebuild and revision readiness proof below.

### Use a manifest-driven one-window cutover

The operator renders the exact deployment configuration and creates an immutable manifest before stopping anything.
The manifest resolves the NATS context/account, active writers, bucket overrides, framework graph buckets,
graph-ingest guard buckets, any legacy catalog, preserved streams/stores, reseed source, expected counts, probe set,
and commands. Two-person review is required for destructive scope.

After all writers stop, only listed incompatible graph resources are removed. The observation stream, schema
ObjectStore, and unrelated state remain. Beta.147 starts against empty graph state, migrated producers reseed, and the
operator captures the resulting `ENTITY_STATES` revision.

Readiness is active polling of graph-index status until indexed revision reaches that target. Query probes then prove
live parity. A no-write restart proves replay parity at the same or later revision. This is preferred over a fixed
wait because silence and process health do not prove the index consumed authoritative state.

### Fail closed instead of offering post-wipe rollback

Before deletion, any gate failure leaves beta.141 untouched. After deletion begins, writers remain stopped on any
failure. The old binary is never pointed at the changed account, and no mixed-format rollback is attempted. Recovery
means correcting the candidate or source and repeating a reviewed clean rebuild from authoritative data.

### Preserve external semantics and UI terminology

Wire golden tests cover public JSON, SensorML, OMS, SWE, GeoJSON, JSON-LD, OpenAPI, links, and error behavior. The UI
uses explicit CS API display labels rather than rendering internal predicate strings. TypeScript generation and UI
tests prove removed SemStreams ownership does not leak into labels or types.

The final gate is a fresh external ETS result of 137 passed, 0 failed, and 0 skipped. A diff of tests, fixture intent,
and conformance declarations is reviewed alongside the result so green cannot be manufactured by weakening scope.

## Risks / Trade-offs

- [Transferred code diverges from provenance] -> Record hashes and review import-only versus semantic diffs separately.
- [A hidden predicate bypasses source grep] -> Validate every final builder output with authoritative marshal logic.
- [Long-ID minting changes retained references] -> Preserve fitting IDs and block cutover on an identity-impact report.
- [An `@id` marker changes foreign-edge behavior] -> Run real-NATS ownership/projection and issue #65 post-break tests.
- [Wrong NATS account or bucket is deleted] -> Require rendered names, literal commands, context capture, and two-person
  manifest approval; prohibit wildcards and unresolved variables.
- [ObjectStore data becomes orphaned] -> Preserve content, reseed artifact entities, and prove key/read parity.
- [Index appears healthy before caught up] -> Gate on captured authoritative revision and replay parity.
- [Package parity passes while wire behavior changes] -> Require both package fixtures and external boundary goldens.
- [Fresh ETS is made green by reducing scope] -> Review ETS, fixture, and conformance-claim diffs as a release gate.

## Migration Plan

1. Transfer and verify package provenance without changing the dependency pin.
2. Move imports and explicit OMS registration; establish package parity.
3. Land predicate, entity/reference, minting, and legacy-fallback changes with final-state contract tests.
4. Prove Go, NATS, UI, wire, and external conformance behavior against a fresh local stack.
5. Align module and backend pins and generate the exact cutover manifest.
6. Rehearse stop/wipe/reseed/readiness/query/replay on disposable fresh volumes.
7. Approve and execute the production maintenance window using the reviewed manifest.
8. Archive revisions, counts, readiness, parity, retained-state, and ETS evidence.

## Open Questions

No architectural questions remain. Deployment-specific bucket names, source counts, NATS context, maintenance owner,
and schedule are required manifest values and block production execution until supplied and approved.
