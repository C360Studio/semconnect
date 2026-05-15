# Getting Started — semconnect Bootstrap Playbook

You are working in a freshly-initialized repository (`.git`,
`LICENSE`, `.gitignore`, `README.md`, this playbook). This
document walks the sister-repo agent through the bootstrap
sequence — what to scaffold first, in what order, and why.

The framework half of [ADR-044](https://github.com/C360Studio/semstreams/blob/main/docs/adr/044-ogc-connected-systems-framework-split.md)
(Phases 2-6) is complete and merged on `semstreams` main.
Everything `semconnect` builds on consumes those primitives as
a Go module dependency. The
[framework-primitives reference](https://github.com/C360Studio/semstreams/blob/main/docs/operations/21-adr044-framework-primitives-reference.md)
is the authoritative source for what's available.

## Reading order before any code

1. [ADR-044](https://github.com/C360Studio/semstreams/blob/main/docs/adr/044-ogc-connected-systems-framework-split.md) — the framework / sister-repo split decision and why this repo exists.
2. The framework-primitives [reference](https://github.com/C360Studio/semstreams/blob/main/docs/operations/21-adr044-framework-primitives-reference.md) — what each Phase 2-6 package provides, with import paths and composition guidance.
3. [CS API Part 1 (23-001)](https://docs.ogc.org/DRAFTS/23-001r0.html) and [Part 2 (23-002)](https://docs.ogc.org/DRAFTS/23-002r0.html) — the REST surface this server implements.
4. The [`semstreams` README](https://github.com/C360Studio/semstreams) — overall architecture (NATS / KV / Graphable / payload registry).
5. This document.

## Bootstrap order

### Stage 0 — Decisions to record in ADR-S001

Before writing code, file `docs/adr/001-cs-api-server-scope.md`
(this is the **ADR-S001** that
[ADR-044 line 197-200](https://github.com/C360Studio/semstreams/blob/main/docs/adr/044-ogc-connected-systems-framework-split.md)
anticipates) covering:

- **Conformance classes claimed at v0.1.** Realistic MVP target:
  Core + JSON encoding + GeoJSON encoding + SensorML encoding +
  OMS encoding. Defer pub/sub binding (CS API Part 3 is still
  draft).
- **Content negotiation policy.** Which media types are
  primary (`application/json`, `application/geo+json`,
  `application/sensorml+json`, `application/om+json`,
  `application/ld+json` for RDF). Which are optional. Which
  are out of scope (XML / GML).
- **Auth posture.** Bearer JWT at the edge? Per-endpoint
  scopes? OIDC discovery? This repo's stance on auth is
  separate from the framework's `input/http` Auth config.
- **Conformance-test ownership.** Vendor OGC Team Engine in
  the repo or fetch on CI? Where the fixtures live.
- **CS API Part 3 (pub/sub binding) stance.** Native NATS?
  Proxy via MQTT / WebSocket? Defer until the draft stabilizes
  but record the leaning.

ADR-S001 closes once stages 1-3 below are scaffolded — the act
of writing the gateway forces concrete decisions on each.

### Stage 1 — `go.mod` + first dependency pin

```bash
go mod init github.com/c360studio/semconnect
go get github.com/c360studio/semstreams@latest
```

Pin to the latest semstreams tag once ADR-044 Phase 7 merges.
Track upgrades manually — the framework will keep moving as
deferred OMS / SensorML / SWE primitives land in follow-up tags.

### Stage 2 — Smallest end-to-end path: `GET /systems`

Implement this single endpoint first. It exercises:

- HTTP routing (the gateway primitive)
- `graph-query` invocation against the NATS-backed entity store
- Response serialization (JSON for the MVP; JSON-LD via
  `vocabulary/export` as the second mediatype)
- The full deployment chain (config parse → NATS connect →
  HTTP listen → query handler → response)

```
GET /systems
  → gateway/cs-api/systems.go
    → graph-query client: list entities with rdf:type ssn:System
    → for each entity: format as CS API System resource
  → JSON response (or JSON-LD if Accept matches)
```

Aim for this to be the first commit beyond the bootstrap docs.
Everything else slots in after the deployment chain is proven.

### Stage 3 — Add observation publish: `POST /datastreams/{id}/observations`

This is the first **mutation** endpoint and the first place
`message/oms` carries its weight:

```
POST /datastreams/{id}/observations
  Content-Type: application/om+json
  Body: OMS Observation JSON

  → gateway/cs-api/observations.go
    → message/oms.Observation (json.Unmarshal)
    → message.NewBaseMessage(obs.Schema(), &obs, "cs-api-ingest")
    → natsclient.PublishToStream("cs-api.observations.{datastream_id}", wire)
  → 201 Created
```

The framework's `graph-ingest` processor will pick up the
publish and turn the observation's `Triples()` into entity-state
updates automatically — that's the payload registry working as
designed.

### Stage 4 — Add SensorML round-trip: `GET /systems/{id}`

This is where `parser/sensorml` carries the load. The graph
side stores entities as triples; the gateway needs to reverse
that back into a SensorML JSON document:

```
GET /systems/{id}
  → gateway/cs-api/systems.go
    → graph-query: full entity + relationships
    → reconstruct sensorml.PhysicalSystem from triples
    → json.Marshal → SensorML JSON
  → 200 OK
```

The triple-to-SensorML reverse mapping is gateway work — the
framework provides parsing and emission of the JSON shape, but
the entity-state → SensorML reconstruction is sister-repo
domain code. Consider a helper `gateway/cs-api/sensorml.go`
with `FromEntityState(state graph.EntityState) (sensorml.Process, error)`.

### Stage 5 — Spatial queries

`GET /areas?bbox=...` and `?polygon=...` map directly onto the
framework's existing spatial query subjects
(`graph.spatial.query.bounds` and `.polygon`). Thin
content-negotiation wrappers; no new primitives needed.

### Stage 6 — Conformance harness

Wire the [OGC Team Engine](https://github.com/opengeospatial/teamengine)
conformance suite into CI. Decision per ADR-S001 stage 0:
vendor the runner or fetch at CI time. The fixtures the suite
exercises will surface deferred-feature requests faster than
hand-authored tests; treat the first conformance run as a
calibration step, not a pass/fail gate.

## What lives where (recap)

```
semconnect/
├── cmd/cs-api-server/main.go    # Stage 1+
├── gateway/cs-api/
│   ├── systems.go               # Stage 2 (GET) + Stage 4 (POST/GET single)
│   ├── observations.go          # Stage 3
│   ├── spatial.go               # Stage 5
│   └── conformance.go           # conformance-class declaration
├── conformance/                 # Stage 6
├── examples/                    # drone fleet etc. — late, after Stage 4
├── docs/
│   ├── 000-getting-started.md   # this file
│   └── adr/001-cs-api-server-scope.md  # Stage 0
└── deploy/                      # late
```

## Things to consult the framework about

The semstreams repo ships these deferred items per the
framework-primitives reference Scope-Cut section:

- **OMS typed results** (Quantity / Category / TimeSeries). If a
  CS API consumer needs typed observation results before the
  framework ships them, file an issue on `semstreams` — don't
  fork the OMS encoder.
- **OMS ResultQuality field.** Framework reviewer flagged this
  as the most likely first ask. Either wait for the follow-up
  tag, or add a thin sister-side extension layer that wraps
  `oms.Observation` with the extra field and provide a
  one-direction marshal.
- **SensorML Mode / Algorithm / Configuration** — framework
  ships PhysicalSystem / PhysicalComponent / SimpleProcess /
  AggregateProcess. Anything past that, file upstream first.
- **SWE Common 3.0** — framework pins to v2.1. If a CS API v1.1+
  binding requires v3.0, that's a `vocabulary/swe3` sibling
  package upstream.
- **CS API Part 3 (pub/sub binding)** — draft. When it
  finalizes, the binding choice (native NATS vs MQTT proxy vs
  WebSocket) is a `semconnect` decision but the framework's
  facts-vs-requests primitive (`message.BaseMessage` + KV watch
  / JetStream) is the underlying substrate.

## Memory / discipline notes inherited from semstreams

These are not framework features but cross-cutting disciplines
worth honoring in the sister repo from the start:

- **Every NATS publish wraps in `message.BaseMessage`** — even
  when the obvious consumer reads raw. Subjects are shared
  infrastructure; an audit dashboard or sister-of-sister-repo
  will eventually subscribe.
- **Operator-reachable JSON seams need round-trip tests** —
  this is the discipline that caught the Phase 4 / 5 / 6 wire
  drift. Any new gateway envelope shape (CS API auth headers,
  conformance-class advertisement) needs the same coverage.
- **Pre-tag sweep includes build tags** — when this repo ships
  its first tag, run `go vet -tags=integration` and any other
  conditional-build tags before tagging.
- **Never re-tag** — Go module proxy pins on first fetch.
- **E2E required for breaking changes** — when the sister repo
  reaches v1.0 and starts taking on conformance-class breaking
  changes, full e2e (whatever that looks like for an HTTP
  server — likely conformance suite + a smoke binary) must run
  green on the breaking commit before tagging.

## Open questions for the sister-repo agent to answer in ADR-S001

- Single binary or modular? `cmd/cs-api-server/` could compose
  multiple sub-binaries (api-server, observation-ingester,
  spatial-query-frontend) or stay monolithic. Monolithic is
  simpler and the framework's component model lets us split
  later without API breakage.
- Pluggable graph backend, or fixed-to-semstreams-NATS? The
  framework abstracts via interfaces; this repo could in
  principle target other graphs but the value proposition is
  with semstreams.
- API versioning policy. `/v1/systems` vs no prefix? When does
  v2 land? OGC's own versioning is loose; pick a convention
  and stick.

## When to come back here

Once the conformance harness reports the first pass on Core +
JSON encoding, ADR-S001 is effectively closed and this playbook
ages out. The next reference will be `docs/operations/` style
docs — release playbooks, deployment guides, content
negotiation policy.

Good luck. The framework half is solid; the standards-shaped
work is done. What's left is the deployment story, and the
deployment story is where the value lives for operators.
