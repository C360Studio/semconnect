# Complete beta.149-to-beta.151 delta and exposure audit

All nine commits were reviewed in order:

| Commit | Upstream change | Semconnect disposition |
|---|---|---|
| `d058bddf` | PR #554 structural gate | Shared graph-ingest gate is live; directly qualified |
| `fa8e90ed` | Archive/sync structural OpenSpec | Contract/docs only |
| `58893068` | Await authoritative graph-index/rule state in tests | Test-only; focused graph-index test run |
| `cb74a6de` | Atomic rule test watcher stop | Test-only; rule not configured |
| `ce130e34` | PR #561 detached rule-state persistence | Rule not configured; focused regression run |
| `31f16c98` | Graph-retention adversarial review | Documentation only |
| `c5b29a3b` | Graph-index CI smoke headroom | Test-only; focused smoke run |
| `38697908` | Canonical agent skills/adapters | Repository tooling, not runtime |
| `ac75c322` | PR #567 trusted graph-ingest owner-RMW decode | Live on update/RMW; directly qualified |

The exact runtime configuration contains:

```json
{
  "tier": "rules",
  "services": ["component-manager", "service-manager"],
  "components": [
    "graph-index", "graph-index-spatial", "graph-index-temporal",
    "graph-ingest", "graph-query"
  ],
  "subjects": [
    "ALIAS_INDEX", "ENTITY_STATES", "INCOMING_INDEX", "OUTGOING_INDEX",
    "PREDICATE_INDEX", "SPATIAL_INDEX", "TEMPORAL_INDEX",
    "_semconnect.unused.ingest", "graph.query.>"
  ]
}
```

`tier: rules` is placement metadata. There is no configured rule, agentic-loop,
agentic-tools, research-graph, clustering, or example producer. A guarded source
and config search found no direct import or executable configuration for those
changed subsystems.

Semconnect's production graph mutation surface is exactly:

```text
graph.mutation.entity.create_with_triples
graph.mutation.entity.update_with_triples
graph.mutation.entity.delete
```

Create and update are exposed to PR #554. Update is also exposed to PR #567.
Delete has no predicate payload. The configured graph-ingest stream input is
`_semconnect.unused.ingest`; semconnect has no publisher to that subject.

Every semconnect graph write is centralized through the create/update helpers.
All created or updated final states pass `validateProjectedTriples`, which calls
the authoritative `graph.MarshalEntityState` contract before NATS. The full
resource-builder suite covers Systems, Procedures, Deployments, Sampling
Features, Properties, Datastreams, schema artifacts, ControlStreams, Commands,
SystemEvents, and Feasibility. The beta.151 live-NATS test independently bypasses
that local guard to prove the backend rejects malformed direct callers.

No alias, predicate rewrite, relaxed parser, dual read/write, cleanup lane, or
legacy compatibility branch was added.
