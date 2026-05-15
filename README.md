# semconnect

**OGC API Connected Systems (CS API) server built on
[semstreams](https://github.com/C360Studio/semstreams).**

`semconnect` is the deployment-shaped sibling of `semstreams`:
where `semstreams` ships the framework primitives constrained by
published standards (SOSA / SWE / OMS / SensorML / GeoJSON),
`semconnect` is the spec-specific HTTP gateway that exposes
those primitives as a [CS API v1.0](https://docs.ogc.org/DRAFTS/23-001r0.html)
RESTful surface.

The split is settled by
[ADR-044](https://github.com/C360Studio/semstreams/blob/main/docs/adr/044-ogc-connected-systems-framework-split.md);
the framework half is complete (Phases 2-6 merged). This
repository is Phase 7 of that ADR.

## What `semconnect` will do

- **Implement the CS API v1.0 RESTful surface** ([Part 1 — Feature Resources](https://docs.ogc.org/DRAFTS/23-001r0.html),
  [Part 2 — Dynamic Data](https://docs.ogc.org/DRAFTS/23-002r0.html))
  by composing the semstreams framework primitives behind HTTP
  endpoints. Conformance class declarations, content negotiation,
  auth, and TLS termination live here.
- **Ship a reference binary** (`cmd/cs-api-server/`) that an
  operator can deploy as the CS API endpoint of a SemStreams
  graph.
- **Run conformance against the [OGC test suite](https://github.com/opengeospatial/teamengine)**
  on CI so every release proves CS API v1.0 compatibility.
- **Provide domain examples** (drone fleet, sensor network,
  robotic platform configs) showing canonical operator
  deployments end-to-end.

## What it will NOT do

- **Re-implement framework-shaped primitives.** SOSA / SWE / OMS
  IRI bindings, GeoJSON parsing, SensorML decode, OMS Observation
  payload are all consumed from `semstreams` as Go module
  dependencies. Sister-repo work is gateway + deployment + tests.
- **Vendor OpenSensorHub** ([OSH](https://www.opensensorhub.org/) is the dominant
  Java CS API server). The Go-server differentiator is part of
  the strategic case in ADR-044.

## Framework dependencies

This repo depends on `github.com/c360studio/semstreams` for:

| Module | What it gives us |
|---|---|
| [`vocabulary/sosa`](https://github.com/C360Studio/semstreams/tree/main/vocabulary/sosa) | SOSA + SSN IRI constants — every RDF response leans on these |
| [`vocabulary/swe`](https://github.com/C360Studio/semstreams/tree/main/vocabulary/swe) | SWE Common v2.1 IRIs for typed observation results |
| [`vocabulary/oms`](https://github.com/C360Studio/semstreams/tree/main/vocabulary/oms) | OMS v3.0 IRIs for Observation document encoding |
| [`graph/geo/geojson`](https://github.com/C360Studio/semstreams/tree/main/graph/geo/geojson) | RFC 7946 GeoJSON types + polygon containment |
| [`input/http`](https://github.com/C360Studio/semstreams/tree/main/input/http) | Generic REST polling input (consumed when proxying external feeds into the graph) |
| [`parser/sensorml`](https://github.com/C360Studio/semstreams/tree/main/parser/sensorml) | SensorML JSON parser + Graphable bridge for `GET /systems/{id}` |
| [`message/oms`](https://github.com/C360Studio/semstreams/tree/main/message/oms) | OMS Observation payload + BaseMessage bridge for observation publish/consume |

Plus pre-existing primitives: `message.BaseMessage`, `message.NewDecoder`,
`graph.Graphable`, `processor/graph-ingest`, `processor/graph-query`,
`vocabulary/export`, `natsclient`, `component`.

The authoritative reference for what each provides and how it
composes for a CS API server is the framework-side doc:

> [docs/operations/21-adr044-framework-primitives-reference.md](https://github.com/C360Studio/semstreams/blob/main/docs/operations/21-adr044-framework-primitives-reference.md)

## CS API endpoint surface (planned)

CS API v1.0 — initial set, mapped to framework primitives:

| Endpoint | Framework primitives |
|---|---|
| `GET /systems` | `graph-query` (lists `ssn:System` entities) → JSON-LD via `vocabulary/export` |
| `GET /systems/{id}` | `graph-query` + `parser/sensorml` round-trip → SensorML JSON |
| `POST /systems` | `parser/sensorml` decode → `graph-ingest` publish |
| `GET /datastreams/{id}/observations` | KV watch on entity-keyed subject → `message/oms` marshal |
| `POST /datastreams/{id}/observations` | `message/oms` decode → BaseMessage publish to `cs-api.observations` JetStream subject |
| `GET /areas?bbox=...` | `graph.spatial.query.bounds` |
| `GET /areas?polygon=...` | `graph.spatial.query.polygon` |

Conformance classes to claim land in `ADR-S001` (see
[docs/000-getting-started.md](docs/000-getting-started.md)).

## Directory layout (planned)

```
semconnect/
├── cmd/
│   └── cs-api-server/        # reference deployment binary
│       └── main.go
├── gateway/
│   └── cs-api/               # HTTP routing + content negotiation + auth
│       ├── systems.go        # Part 1 endpoints
│       ├── observations.go   # Part 2 endpoints
│       └── conformance.go
├── conformance/              # OGC test suite runner + fixtures
├── examples/
│   ├── drone-fleet/
│   ├── sensor-network/
│   └── robotic-platform/
├── docs/
│   ├── 000-getting-started.md
│   └── adr/
│       └── 001-cs-api-server-scope.md   # ADR-S001 — see below
├── deploy/                   # container, Helm, ops docs
└── README.md
```

Nothing in this layout exists yet — directories are placeholders.
The getting-started doc walks through the bootstrap order.

## Bootstrap status

This repository was just initialized. Nothing past `README` /
`LICENSE` / `.gitignore` is present. The bootstrap playbook is:

> [docs/000-getting-started.md](docs/000-getting-started.md)

Open issues to file once the first commit lands:

1. `ADR-S001 — CS API server scope` (this is the sister-side ADR
   ADR-044 anticipates; track conformance classes, deployment
   surface, content-negotiation policy, conformance-test ownership).
2. Initial `go.mod` with `github.com/c360studio/semstreams`
   pinned to the latest framework tag.
3. First endpoint scaffold (`GET /systems`) — smallest end-to-end
   path that proves the framework-to-gateway plumbing works.

## References

### Internal

- [docs/000-getting-started.md](docs/000-getting-started.md) — bootstrap playbook for the sister-repo agent.
- [docs/adr/001-cs-api-server-scope.md](docs/adr/001-cs-api-server-scope.md) — to be filed (ADR-S001).

### External

- [OGC API Connected Systems standard](https://www.ogc.org/standards/ogc-api-connected-systems/)
- [CS API Part 1 — Feature Resources (23-001)](https://docs.ogc.org/DRAFTS/23-001r0.html)
- [CS API Part 2 — Dynamic Data (23-002)](https://docs.ogc.org/DRAFTS/23-002r0.html)
- [OGC CS API GitHub](https://github.com/opengeospatial/ogcapi-connected-systems)
- [OGC Team Engine (conformance test runner)](https://github.com/opengeospatial/teamengine)

### Framework side

- [semstreams repository](https://github.com/C360Studio/semstreams)
- [ADR-044 — Framework / sister-repo split](https://github.com/C360Studio/semstreams/blob/main/docs/adr/044-ogc-connected-systems-framework-split.md)
- [Framework primitives reference](https://github.com/C360Studio/semstreams/blob/main/docs/operations/21-adr044-framework-primitives-reference.md)
