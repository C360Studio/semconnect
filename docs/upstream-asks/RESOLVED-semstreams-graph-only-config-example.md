# RESOLVED upstream ask — semstreams: ship a "graph-only backend" example config

**Repo:** <https://github.com/C360Studio/semstreams>
**Drafted from:** semconnect Stage 9 conformance backend integration (2026-05-16), framework pin `v1.0.0-beta.73`.
**Status:** **RESOLVED in v1.0.0-beta.79** ([C360Studio/semstreams#99](https://github.com/C360Studio/semstreams/issues/99), commit `7cdf4e3`). semstreams now ships a slim graph-backend example and port-type guide. semconnect keeps its conformance-specific config because the harness owns fixture subjects and local service wiring.

## Summary

For a sister-repo (`semconnect`) that publishes mutations directly
via `graph.mutation.triple.add_batch` request/reply and reads via
`graph.index.query.predicate` / `graph.query.entity` /
`graph.spatial.query.*`, the minimum framework instance needs five
processors and nothing else:

- `graph-ingest` (mutation responder; auto-wires
  `graph.mutation.triple.{add,add_batch,remove}`)
- `graph-index` (entity-state KV watcher → predicate index responder)
- `graph-index-spatial` (entity-state KV watcher → spatial query
  responder)
- `graph-index-temporal` (cheap to include for completeness)
- `graph-query` (`graph.query.>` request/reply responder)

No `udp`, `iot_sensor`, `document_processor`, `objectstore`, `rule`,
`graph-gateway`, file inputs/outputs — those are all part of the
"source data through the JetStream pipeline" demo path.

The closest existing config is `configs/hello-world.json`, which
still ships `udp` + `iot_sensor` (because the demo's whole point is
sourcing entities via the standard pipeline). Authoring the slim
graph-only shape took reading `componentregistry/register.go`,
`component/port.go`, `processor/graph-ingest/component.go` (port
validation), and `processor/graph-ingest/mutations.go` (auto-wired
subjects). That's not light lifting for a downstream consumer.

## Concrete impact

Without a slim graph-only example, every gateway / sister repo that
wants the framework as a *backend* (not as the *whole* pipeline) has
to derive the same minimum from source. Some will overshoot and
include processors they don't need; others will undershoot and miss
a required port.

Two specific landmines we hit:

1. **`graph-ingest.Config.Validate()` requires `len(Ports.Inputs) >= 1`**
   (`processor/graph-ingest/component.go:78`). Cs-api-server writes
   via the auto-wired mutation handlers, not a JetStream input port,
   so we have to declare a benign dummy:
   `{"name": "unused_in", "subject": "_semconnect.unused.ingest", "type": "nats"}`.
   Worth a slim-mode boolean or relaxing the validation when only
   mutation handlers are exposed.

2. **The `nats`-type port (vs `jetstream`-type) was the key insight**
   — declaring `objectstore.stored.entity` as a JetStream input would
   require EnsureStream-ing a stream we never publish to. The `nats`
   subscriber type has no such requirement. Worth surfacing in the
   port-type docs ("when to use `nats` vs `jetstream`").

## Proposed change

### Primary — add `configs/graph-backend.json`

Slim config wiring only the five graph-* processors plus the minimum
services (`service-manager` + `component-manager`). Suitable for
operators who want the framework as a graph backend behind a
custom gateway.

See `conformance/compose.semstreams.config.json` in semconnect for
the working shape (copy-paste-ready, exercised against the conformance
harness 2026-05-16).

### Secondary — port-type docs

`component/port.go` and `processor/graph-ingest/README.md` (or
equivalent) should clarify:

- When to use `nats` vs `jetstream` vs `kv-watch` vs `nats-request`
  port types
- That `graph-ingest`'s `graph.mutation.*` handlers are auto-wired
  by `setupMutationHandlers` regardless of port declarations
- That `graph-ingest` requires `>= 1` input port even if nothing
  actually feeds the JetStream pipeline (with the recommended dummy
  pattern if no slim-mode lands)

### Tertiary — slim-mode for graph-ingest

Either drop the `len(Ports.Inputs) >= 1` validation when the
processor is operating in "mutation-handlers-only" mode (no
hierarchy inference, no JetStream feed), or add an explicit
`mode: "responder-only"` field to the processor config that
skips the port-count check.

## Why this is worth filing

The framework markets itself as composable; a graph-only deployment
is a first-class use case (every gateway wants this, plus any service
that needs a SPARQL-shaped backend without owning the pipeline). Today
that path requires source-reading. A 60-line example config makes the
on-ramp ~10x shorter.
