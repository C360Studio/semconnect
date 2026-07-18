## Why

SemStreams `v1.0.0-beta.147` removes the OGC packages that semconnect imports and begins rejecting the graph
identities that the current gateway emits. Semconnect must take explicit ownership of the OGC bundle and perform one
coordinated graph-state cutover before it can consume the new framework without compile failures, poisoned replay, or
silent loss of CS API relationships.

## What Changes

- **BREAKING** Re-home the seven-package OMS, SensorML, SWE Common, SOSA/SWE, and CS API bundle in semconnect from
  the reviewed pre-deletion SemStreams source revision.
- **BREAKING** Replace every noncanonical internal predicate and IRI-as-predicate use with registered, three-part,
  lower-kebab product vocabulary. External OGC member names and RDF/JSON-LD IRIs do not change.
- **BREAKING** Enforce canonical six-part graph entity IDs and `@id` references before backend I/O, and bound all
  server-minted IDs deterministically while preserving the source UID as semantic data.
- Register `ogc.oms.v3` explicitly in every binary or test decoder that consumes OMS envelopes.
- Remove legacy UID/position predicate fallbacks and prohibit aliases, dual reads, permissive validators, or in-place
  rewrites of incompatible beta graph state.
- Require one reviewed stop, manifest-derived graph-bucket wipe, restart, canonical reseed, index-revision readiness
  proof, query parity proof, and replay-parity proof while preserving unrelated observation and artifact state.
- Preserve the public CS API wire contract, UI semantic labels, NATS subjects, payload envelopes, and the full claimed
  conformance set.
- Re-run package, semantic, graph-wire, NATS, Go, TypeScript, race, vet, build, and external ETS evidence gates.

Non-goals are adding a second graph backend, changing public CS API paths or member names, expanding SWE Common Phase
2, implementing rule/event flows that semconnect does not use, or weakening a conformance claim to make the migration
pass. Compatibility risk is concentrated in persisted graph identity: old predicate/index layouts are intentionally
unreadable after the cutover and have no rollback reader.

## Capabilities

### New Capabilities

- `owned-ogc-package-parity`: Product ownership, source provenance, registration, and package/API behavior parity for
  the transferred OGC bundle.
- `canonical-semantic-graph-contracts`: Canonical predicates, entity IDs, references, deterministic minting, and
  boundary separation between internal graph names and external standards-shaped names.
- `graph-state-cutover-readiness`: Manifest-controlled destructive cutover, retained product state, reseed,
  revision-based index readiness, query parity, replay parity, and no-go/rollback behavior.
- `external-cs-api-conformance`: Public wire/UI/NATS compatibility and the immutable external ETS and build evidence
  required to release the migration.

### Modified Capabilities

None. This repository has no established OpenSpec capability whose normative requirements cover this pre-v1
migration.

## Impact

- Go ownership moves into semconnect for `message/oms`, `parser/sensorml`, `pkg/swecommon`, and
  `vocabulary/{csapi,oms,sosa,swe}`.
- Gateway graph builders, validators, tests, fixtures, JSON-LD/RDF registration, and OMS decoder composition change.
- SemStreams module and conformance-backend pins move together to `v1.0.0-beta.147`.
- Existing framework-owned graph and derived-index KV state is incompatible and requires a controlled maintenance
  window. The CS API observation stream, schema-artifact ObjectStore, and unrelated product state are retained unless
  the reviewed manifest proves an identity change requires separate treatment.
- The external Botts ETS remains the conformance authority. Release requires a fresh `137 passed, 0 failed, 0 skipped`
  result with unchanged tests, fixtures, and conformance claims.
