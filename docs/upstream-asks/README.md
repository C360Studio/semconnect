# Semstreams Upstream Asks

This directory tracks framework asks that surfaced while implementing
the CS API gateway in semconnect. Keep open asks short and actionable;
move resolved asks to `RESOLVED-...` files so we preserve the history
without making semstreams review stale blockers.

## Open asks

None currently blocking semconnect. As of Stage 55, the CS API gateway is
green against the pinned ETS (`total=137 passed=137 failed=0 skipped=0`).

Non-blocking vocabulary asks now filed upstream:

- [C360Studio/semstreams#200](https://github.com/C360Studio/semstreams/issues/200)
  — add CS API Command Feasibility class/predicates so semconnect can
  retire `FeasibilityTypeIRI` and the `cs-api.feasibility.*` local
  predicates introduced in Stage 55.
- [C360Studio/semstreams#201](https://github.com/C360Studio/semstreams/issues/201)
  — add CS API association/composition predicates for Deployment
  deployed-systems evidence, Subdeployment parent composition, and
  SamplingFeature hosted-procedure evidence.
- [C360Studio/semstreams#202](https://github.com/C360Studio/semstreams/issues/202)
  — proposal to decide which CS API scalar metadata predicates belong in
  `vocabulary/csapi` versus remaining gateway-local representation
  details.

## Resolved in current pins

- `v1.0.0-beta.108`: ADR-056 ownership/projection substrate plus the shared
  projection-normalization seam now covers mutation-lane writes. semconnect
  uses this for SensorML System projections by stamping the `sensorml.asset.v1`
  producer and forwarding child foreign-edge triples. SemStreams classifies
  unclaimed foreign edges in observe-only mode at this pin.
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
