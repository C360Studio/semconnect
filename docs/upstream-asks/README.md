# Semstreams Upstream Asks

This directory tracks framework asks that surfaced while implementing
the CS API gateway in semconnect. Keep open asks short and actionable;
move resolved asks to `RESOLVED-...` files so we preserve the history
without making semstreams review stale blockers.

## Open asks

- [#171](https://github.com/C360Studio/semstreams/issues/171): CS API
  typed artifact entities for ObjectStore-backed source documents,
  Datastream result schemas, and ControlStream command schemas. The
  upstream recommendation is no new framework primitive: use first-class
  artifact entities with their own singular `StorageRef`, related from
  parent resources by CS API vocabulary predicates. See
  `docs/adr/002-cs-api-artifact-storage.md`.
- [#172](https://github.com/C360Studio/semstreams/issues/172): graph
  batch entity hydration or predicate-query projections to reduce N+1
  gateway collection reads.
- [#173](https://github.com/C360Studio/semstreams/issues/173):
  natsclient test-client helper documentation for gateway integration
  tests.

## Resolved in current pins

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

- Migrate gateway writes from `graph.mutation.triple.add_batch` plus
  delete fan-out to `graph.mutation.entity.*`. This is no longer an
  upstream ask now that #120 is closed.
- Migrate Datastream and ControlStream schema storage from local JSON
  predicates to typed artifact entities once semstreams ships the CS API
  vocabulary constants tracked in #171.
- Keep `conformance/nats.conf` for local server limits; semstreams now
  validates/warns, but the harness still owns the NATS server config.
