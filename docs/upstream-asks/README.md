# Semstreams Upstream Asks

This directory tracks framework asks that surfaced while implementing
the CS API gateway in semconnect. Keep open asks short and actionable;
move resolved asks to `RESOLVED-...` files so we preserve the history
without making semstreams review stale blockers.

## Open asks

None currently blocking semconnect. The asks filed from the CS API
gateway work are closed upstream as of semstreams `v1.0.0-beta.91`.

## Resolved in current pins

- `v1.0.0-beta.91`: #182 split `vocabulary/csapi` relationship
  predicates into dotted internal constants plus `*IRI` boundary
  constants. semconnect now writes dotted CS API relationship predicates
  while retaining read fallbacks for beta.90's IRI-shaped predicate data.
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

- Migrate Datastream and ControlStream schema storage from local JSON
  predicates to typed artifact entities using the beta.91 dotted
  relationship predicates (`csapi.HasResultSchema`,
  `csapi.HasCommandSchema`) and artifact class (`csapi.SWESchemaDocument`).
- Adopt `graph.query.batch` for collection endpoints that currently do
  predicate-query plus N entity-query hydrations; chunk around 100 IDs
  per batch per the semstreams guidance.
- Use the documented `natsclient.TestClient` helpers for gateway
  integration tests when we next replace in-memory request fakes with a
  real NATS-backed harness.
- Keep `conformance/nats.conf` for local server limits; semstreams now
  validates/warns, but the harness still owns the NATS server config.
