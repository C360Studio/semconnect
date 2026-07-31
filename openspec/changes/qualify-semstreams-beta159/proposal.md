## Why

SemStreams `v1.0.0-beta.159` removes the request/reply graph-index status endpoint, enforces bounds on ordinary
streams, and changes several graph/readiness contracts used by semconnect. The dependency can move only after the
gateway adopts the live breaking seams and proves that its public CS API remains fully conformant.

## What Changes

- Align every executable SemStreams pin to `v1.0.0-beta.159`, peeled commit
  `8813270c5ba441286d9120cba82fbf72bdcf9a6c`.
- Replace readiness polling on removed `graph.index.query.status` with fresh `GRAPH_STATUS` KV updates.
- Bound CS API observations at 1 GiB and 30 days with `DiscardOld`, rejecting unbounded configuration.
- Qualify the live graph mutation, add-deduplication, remove-no-op, startup, query-envelope, and storage contracts.
- Re-run full downstream Go, focused upstream race, clean-volume Compose persistence, and unchanged external
  `137/0/0` gates.

## Non-goals

- No in-place migration, retained beta.153 volume claim, compatibility alias, dual read/write, or relaxed validation.
- No adoption of GraphQL `graphSummary`, fusion, embedding, clustering, graphview, rule, or agentic features.
- No public CS API, OpenAPI, conformance-claim, fixture-intent, ETS-authority, or frontend change.
- No runtime manifest, product-owner hash ceremony, or separate production authorization workflow.

## Capabilities

### New Capabilities

- `semstreams-beta159-qualification`: exact beta.159 alignment and proportional downstream qualification.

### Modified Capabilities

None.

## Impact

Semconnect consumes graph mutation/query, readiness, framework bucket acquisition, and ordinary JetStream stream
configuration. Those live seams carry the migration risk. The dependency-only change has no Svelte work. If
qualification requires compatibility behavior or a public API change, implementation stops and architecture review
reopens.
