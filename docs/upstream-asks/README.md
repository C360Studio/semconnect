# Semstreams Upstream Asks

This directory tracks framework asks that surfaced while implementing
the CS API gateway in semconnect. Keep open asks short and actionable;
move resolved asks to `RESOLVED-...` files so we preserve the history
without making semstreams review stale blockers.

## Open asks

None. As of semstreams `v1.0.0-beta.88`, every semstreams issue this
directory tracks is resolved for semconnect.

## Resolved in current pins

- `v1.0.0-beta.88`: #116 schema-bound SWE Common JSON/text/binary
  encoders and decoders. semconnect still needs local adoption work to
  bind datastream result schemas to observation and command payloads.
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
- Adopt `pkg/swecommon` for full schema-bound observation and command
  payload encodings; Stage 27's observation-value subset remains until
  semconnect wires result schemas to datastream/controlstream resources.
- Keep `conformance/nats.conf` for local server limits; semstreams now
  validates/warns, but the harness still owns the NATS server config.
