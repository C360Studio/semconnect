# Semstreams Upstream Asks

This directory tracks framework asks that surfaced while implementing the CS
API gateway in semconnect. ADR-S003 changes the ownership boundary: graph,
NATS, JetStream, ObjectStore, ownership, and projection gaps remain SemStreams
asks, while OMS, SensorML, SWE Common, SOSA/SWE, CS API vocabulary, and related
OGC package work is semconnect-owned. Keep asks short and actionable; retain
resolved or transferred history without presenting it as an upstream blocker.

## Open asks

No SemStreams framework ask currently blocks the beta.153 dependency
pin. Exact alignment, the live per-entity structural regression, full Go
test/race/vet/build, focused upstream, clean-volume Compose persistence, and
unchanged external `137/0/0` gates pass. Independent review found no
legacy/compatibility code or conformance weakening. Beta.151 remains a
qualified historical baseline. The beta.153 greenfield bundle is
production-ready for standard Compose on clean NATS and has no migration or
runtime-unused manifest approval gate.

Transferred product-boundary history:

- [C360Studio/semstreams#200](https://github.com/C360Studio/semstreams/issues/200)
  — CS API Command Feasibility vocabulary. Transferred into semconnect issue
  [#70](https://github.com/C360Studio/semconnect/issues/70) and implemented by
  the beta.147 migration in owned `vocabulary/csapi`.
- [C360Studio/semstreams#201](https://github.com/C360Studio/semstreams/issues/201)
  — CS API association/composition vocabulary. Transferred into semconnect
  issue [#71](https://github.com/C360Studio/semconnect/issues/71) and
  implemented by the beta.147 migration in owned `vocabulary/csapi`.
- [C360Studio/semstreams#202](https://github.com/C360Studio/semstreams/issues/202)
  — scalar-metadata ownership proposal. Its ownership question is resolved by
  ADR-S003: CS API product vocabulary belongs in semconnect.

The complete transfer record is
[TRANSFERRED-semstreams-ogc-product-boundary.md](TRANSFERRED-semstreams-ogc-product-boundary.md).

Semconnect issue [#69](https://github.com/C360Studio/semconnect/issues/69), SWE
Common Phase 2, remains deferred and is not part of the dependency migration.

## Resolved in current pins

- `v1.0.0-beta.141`: includes the SemStreams ENTITY_STATES TTL guardrail fix
  from gh#484/gh#485. The conformance backend pin now matches this tag so the
  graph-ingest boot path and gateway module dependency stay aligned.
- `v1.0.0-beta.116`: ADR-060 removes in-body mutation/query error signalling
  from the current wire contract. semconnect now branches on classified NATS
  replies and reads mutation/query failure codes from `*errs.ClassifiedError`
  instead of legacy response-body `Success`, `Error`, or `ErrorCode` fields.
- `v1.0.0-beta.111`: ADR-056 ownership/projection substrate plus the shared
  projection-normalization seam now covers mutation-lane writes. semconnect
  uses this for SensorML System projections by stamping the
  `c360.csapi.system.v1` producer, forwarding child foreign-edge triples, and
  binding a `NoBirthStub` `sensorml.PredIsHostedBy` foreign-edge claim for its
  configured System ID prefix. SemStreams classifies unclaimed foreign edges in
  observe-only mode at this pin.
- `v1.0.0-beta.91`: #182 split `vocabulary/csapi` relationship
  predicates into dotted internal constants plus `*IRI` boundary
  constants. semconnect now writes dotted CS API relationship predicates
  and reads that same dotted shape directly because the gateway is
  greenfield.
- `v1.0.0-beta.90`: #171 CS API typed artifact classes and
  first-class artifact-entity guidance, #172 public `graph.query.batch`
  passthrough for batch entity hydration, and #173 natsclient test-client
  helper documentation for gateway integration tests.
- `v1.0.0-beta.88`: #116 schema-bound SWE Common JSON/text/binary
  encoders and decoders. semconnect Stage 32 adopts them on the
  observation read path, and Stage 33 binds Datastream schemas locally;
  Stage 34 validates ControlStream command schemas with `pkg/swecommon`.
- `v1.0.0-beta.87`: #93 Phase 1+2+3 header-classified request/reply
  errors. semconnect uses `natsclient.ClassifyReply` on entity reads;
  #93 remains open upstream only for deferred breaking cleanup and
  follow-ups that do not block semconnect.
- `v1.0.0-beta.81`: #100 health-port wiring, #101 JetStream limit
  validation/documentation, and #120 entity mutation degraded read-back
  semantics.
- `v1.0.0-beta.79`: #99 graph-only config example, #114 SensorML
  position preservation, and #115 SensorML uid preservation.
- `v1.0.0-beta.75`: datastream vocabulary moved into semstreams.

## semconnect-local follow-ups

- Maintain the transferred OGC product packages under semconnect and route
  their detailed development through ADRs plus OpenSpec changes.
- Keep SWE Common Phase 2 in issue #69 as separate product scope; owning
  `pkg/swecommon` does not silently expand the beta.147 migration.

- Datastream and ControlStream schema call sites now use the Stage 41
  schema artifact helper (Stage 42). Local JSON schema predicates are
  retired in favor of `csapi.SWESchemaDocument` entities with
  `StorageRef` and beta.91 dotted relationship triples
  (`csapi.HasResultSchema`, `csapi.HasCommandSchema`).
- Use the documented `natsclient.TestClient` helpers for gateway
  integration tests when we next replace in-memory request fakes with a
  real NATS-backed harness.
- Keep `conformance/nats.conf` for local server limits; semstreams now
  validates/warns, but the harness still owns the NATS server config.
- Track SemStreams ADR-056 follow-ups for hard foreign-edge rejection,
  pending-edge buffering, owner-token write leases, and projection-contract
  boot binding before documenting graph ownership as hard enforcement.
- Keep the user-facing distinction clear: ownership claims live in the
  SemStreams ownership substrate, while entity triples may still describe
  provenance such as referential-stub materialization (`core.identity.stub_owner`).
