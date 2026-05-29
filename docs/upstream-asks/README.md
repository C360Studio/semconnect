# Semstreams Upstream Asks

This directory tracks framework asks that surfaced while implementing
the CS API gateway in semconnect. Keep open asks short and actionable;
move resolved asks to `RESOLVED-...` files so we preserve the history
without making semstreams review stale blockers.

## Open asks

- [#93 — header-classified structured request/reply errors](semstreams-structured-errors.md).
  semconnect still carries `classifyEntityQueryError` to parse the
  current stringly `error: ...` response body convention.
- [#116 — schema-bound SWE Common encodings](semstreams-swe-schema-bound-encodings.md).
  semconnect Stage 27 only exposes observation-value subsets and does
  not claim SWE Common conformance.

## Resolved in current pins

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
- Keep `conformance/nats.conf` for local server limits; semstreams now
  validates/warns, but the harness still owns the NATS server config.
