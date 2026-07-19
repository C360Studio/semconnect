# Public compatibility matrix

This matrix freezes the product boundary for `migrate-semstreams-beta147`. Internal package ownership, graph
predicates, entity validation, and index storage change. Public CS API behavior remains stable unless a section
explicitly states otherwise. Compatibility never means accepting beta.141 graph state, aliases, or dual reads.

## HTTP and JSON

- **Contract**: Routes, methods, status classification, headers, link relations, media negotiation, and documented
  JSON fields remain unchanged.
- **Treatment**: Builders use owned packages and canonical graph identities; invalid graph input fails before I/O.
- **Evidence**: Handler goldens, OpenAPI parity, Go integration tests, and the external ETS.

## OMS

- **Contract**: Posted OMS JSON and observation collection/item behavior remain unchanged.
- **Treatment**: Semconnect owns `message/oms`; typed decoders explicitly register `ogc.oms.v3`; raw-return paths
  remain registry-free.
- **Evidence**: Package round trips, decoder-composition tests, and observation replay parity.

## SensorML

- **Contract**: Accepted and emitted SensorML media types and supported resource shapes remain unchanged.
- **Treatment**: Semconnect owns `parser/sensorml`; graph relationships use canonical predicates and `@id`.
- **Evidence**: Parser fixtures, item round trips, nested-child minting goldens, and HTTP goldens.

## SWE Common

- **Contract**: DataRecord schemas and JSON, CSV, and binary observation encodings remain unchanged.
- **Treatment**: Semconnect owns `pkg/swecommon`; SWE Common Phase 2 remains out of scope.
- **Evidence**: Package round trips, artifact schema reads, and observation encoding tests.

## GeoJSON

- **Contract**: Feature and FeatureCollection shapes, geometry, and association links remain unchanged.
- **Treatment**: Canonical predicates replace camelCase storage identities; external hrefs remain literals.
- **Evidence**: GeoJSON goldens and spatial/association query parity.

## JSON-LD

- **Contract**: Public contexts and SOSA/OGC IRIs remain standards-shaped.
- **Treatment**: Internal dotted predicates map through owned registries; full IRIs are prohibited as graph
  predicates.
- **Evidence**: JSON-LD goldens proving `sosa:observedProperty` boundary output.

## RDF

- **Contract**: Exported RDF terms remain standards IRIs and entity relationships remain resources.
- **Treatment**: Graph storage uses canonical three-part predicates and explicit `@id` relationships.
- **Evidence**: RDF serialization goldens and foreign-edge integration tests.

## OpenAPI

- **Contract**: Paths, operations, schemas, media types, and declared response headers remain unchanged.
- **Treatment**: No ownership or graph-storage details enter the public schema.
- **Evidence**: Semantic OpenAPI diff plus external discovery checks.

## UI labels

- **Contract**: User-facing CS API names and descriptions remain stable and accessible.
- **Treatment**: Labels are explicit product metadata. Neither old camelCase nor new lower-kebab identities leak into
  UI text.
- **Evidence**: Svelte component tests, accessibility review, check, build, and end-to-end tests.

## NATS subjects

- **Contract**: Observation subjects remain `cs-api.observations.{datastreamID}`. Framework graph subjects follow
  beta.147 contracts.
- **Treatment**: Go and backend revisions move together; no local graph-subject compatibility facade exists.
- **Evidence**: Real-NATS mutation/query and observation subject parity tests.

## Envelope types

- **Contract**: Observation envelopes remain `ogc.oms.v3`; the existing base-message structure remains compatible.
- **Treatment**: Typed decoder registration becomes explicit at composition roots.
- **Evidence**: Envelope golden tests and JetStream replay evidence.

## Audit headers

- **Contract**: Forwarded-user, forwarded-email, W3C trace, and existing audit semantics remain unchanged.
- **Treatment**: Header propagation is independent of owned-package and predicate changes.
- **Evidence**: Publish spies and real-NATS header assertions.

## Conformance claims

- **Contract**: Advertised CS API classes remain unchanged and the external Botts ETS remains authoritative.
- **Treatment**: No tests, fixtures, declarations, filters, or skips may be weakened for migration.
- **Evidence**: Reviewed conformance diff and fresh `137 passed, 0 failed, 0 skipped`.

Retained observation stream messages and schema ObjectStore bytes are preserved. Their graph references must pass the
identity-impact gate before production; a mismatch is a cutover no-go, not grounds for implicit compatibility.
