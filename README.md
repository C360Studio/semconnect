# semconnect

**OGC API Connected Systems (CS API) gateway built on
[semstreams](https://github.com/C360Studio/semstreams).**

`semconnect` is the HTTP gateway and reference server half of the
SemStreams CS API split. `semstreams` owns the framework primitives:
graph, NATS request/reply, ObjectStore, SOSA/SWE/OMS/SensorML/GeoJSON
vocabularies and parsers. `semconnect` composes those primitives into an
[OGC API Connected Systems v1.0](https://www.ogc.org/standards/ogc-api-connected-systems/)
REST surface.

The repository is no longer a scaffold. As of the 2026-07-06 SemStreams pin
refresh, `cmd/cs-api-server` builds, the conformance harness runs end to end,
and the pinned CS API ETS is green:

```text
total=137 passed=137 failed=0 skipped=0
```

## Current Status

- Framework pin: `github.com/c360studio/semstreams v1.0.0-beta.141`.
- ETS pin: Botts CS API ETS `0.1-SNAPSHOT` at commit `d9caf33`.
- Reference binary: `cmd/cs-api-server`.
- Gateway package: `gateway/cs-api`.
- Conformance harness: `conformance/run.sh`.
- Open semstreams asks are non-blocking and tracked in
  [docs/upstream-asks/README.md](docs/upstream-asks/README.md).

## What This Repo Owns

- HTTP routing, content negotiation, request validation, response shapes, and
  OGC conformance declarations.
- CS API write helpers that turn HTTP request bodies into semantic graph
  entities and ObjectStore artifacts.
- Observation publish/readback over JetStream.
- Auth/audit seams for trusted reverse-proxy deployments.
- The local and CI conformance harness around NATS, `semstreams-backend`,
  `cs-api-server`, and Team Engine.

It does not fork framework-shaped primitives. Missing vocabulary, parser,
ObjectStore, graph, or NATS-client behavior should be filed upstream on
`semstreams` unless the need is clearly gateway-specific.

## Implemented Surface

The gateway exposes OGC Common discovery plus CS API Part 1 and Part 2 read
and fixture-write surfaces:

- Common: `GET /`, `GET /api`, `GET /conformance`, `GET /collections`,
  `GET /health`.
- Feature resources: Systems, Procedures, Deployments, Sampling Features,
  Properties, Datastreams, and Areas.
- System relations: subsystem and subdeployment collection reads.
- Dynamic data: Observations, ControlStreams, Commands, SystemEvents, and
  Command Feasibility metadata.
- Encodings: JSON, GeoJSON, SensorML JSON, OMS JSON, SWE values, JSON-LD, and
  OpenAPI YAML/JSON where the resource family supports them.
- Write semantics: create/replace/delete and update where claimed by the
  conformance set; fixture-only writes for read-side Part 2 resources that do
  not execute commands or evaluate feasibility at v0.1.

For the endpoint-by-endpoint primitive mapping, read [AGENTS.md](AGENTS.md).
For the historical stage log, read
[docs/000-getting-started.md](docs/000-getting-started.md).

## Framework Dependencies

This repo relies on semstreams for:

| Area | Semstreams surface |
|---|---|
| Graph reads/writes | `graph.query.*`, `graph.index.query.*`, `graph.query.batch`, `graph.mutation.entity.*` |
| Spatial queries | `graph.spatial.query.bounds`, `graph.spatial.query.polygon` |
| Observations | `message.BaseMessage`, OMS payload helpers, JetStream client helpers |
| Artifacts | ObjectStore, `StorageReference`, `ContentStorable`, artifact entity patterns |
| Schemas | `pkg/swecommon` canonicalization and validation |
| Vocabularies | `vocabulary/sosa`, `vocabulary/ssn`, `vocabulary/swe`, `vocabulary/oms`, `vocabulary/csapi` |
| Encoders | `parser/sensorml`, GeoJSON types, JSON-LD export |
| NATS boundary | `natsclient` request/reply classification and test helpers |

### Graph Governance Posture

SemConnect writes CS API resources through SemStreams entity mutation subjects.
At the current framework pin, System SensorML writes also stamp the
`c360.csapi.system.v1` projection producer, bind a `NoBirthStub`
`sensorml.PredIsHostedBy` foreign-edge claim for the configured System ID
prefix, and forward child/foreign-edge triples through the mutation lane.
SemStreams normalizes those projected triples at graph-ingest, routes
foreign-subject edges onto their own entities, and meters unclaimed
`(message_type, predicate)` pairs.

That is the first governed graph-state integration point for this gateway. It is
not endpoint authorization yet: SemStreams ownership is still in an observe-only
bake window for these foreign edges, so operators should treat it as provenance,
write-semantics, and future enforcement readiness rather than as a hard security
boundary.

Ownership claims themselves are not modeled as ordinary domain triples. Those
claims live in SemStreams' ownership substrate. Entity triples may still carry
provenance about graph materialization, source, audit, or lineage; for example,
SemStreams referential stubs can record which producer caused the stub to exist.
That is provenance on the entity, not ownership arbitration.

The framework/sister-repo boundary is documented in
[ADR-044](https://github.com/C360Studio/semstreams/blob/main/docs/adr/044-ogc-connected-systems-framework-split.md)
and the
[framework primitives reference](https://github.com/C360Studio/semstreams/blob/main/docs/operations/21-adr044-framework-primitives-reference.md).

## Build And Test

```bash
openspec validate --all --strict
go test ./...
go build ./...
```

OpenSpec CLI 1.5 currently reports `No items found to validate` because this
repo does not carry a repo-owned `openspec/` project tree; the command remains
in the local verification set so future spec artifacts fail fast.

Run the reference server against a local NATS:

```bash
go run ./cmd/cs-api-server
```

or provide a JSON config:

```bash
go run ./cmd/cs-api-server --config ./cs-api.json
```

The default config binds HTTP on `:8080` and connects to
`nats://localhost:4222`.

Run the full conformance harness:

```bash
./conformance/run.sh
```

The harness writes TestNG XML, logs, seed output, and a summary into
`conformance/output/`.

## Demo UI

The telemetry graph demo lives under `ui/`. It can run with local fixture data
for quick UI review, or against a full SemStreams + CS API stack through the
comparison runner:

```bash
cd ui
npm run compare:full-stack -- --profile both
```

See [docs/demo-telemetry-graph.md](docs/demo-telemetry-graph.md) for the
sponsor and early-adopter runbook, including Caddy proxying, expected counts,
semantic-vs-statistical comparison notes, and the CS API ID mapping.

## Documentation

- [docs/demo-telemetry-graph.md](docs/demo-telemetry-graph.md) - telemetry
  graph demo runbook for sponsors and early adopters.
- [docs/adr/001-cs-api-server-scope.md](docs/adr/001-cs-api-server-scope.md) -
  CS API server scope and conformance stance.
- [docs/adr/002-cs-api-artifact-storage.md](docs/adr/002-cs-api-artifact-storage.md) -
  graph-vs-ObjectStore storage pattern.
- [conformance/README.md](conformance/README.md) - local conformance runner,
  pins, and bump procedure.
- [docs/upstream-asks/README.md](docs/upstream-asks/README.md) - current
  semstreams asks.

## External References

- [OGC API Connected Systems standard](https://www.ogc.org/standards/ogc-api-connected-systems/)
- [CS API Part 1 - Feature Resources](https://docs.ogc.org/DRAFTS/23-001r0.html)
- [CS API Part 2 - Dynamic Data](https://docs.ogc.org/DRAFTS/23-002r0.html)
- [OGC CS API GitHub](https://github.com/opengeospatial/ogcapi-connected-systems)
- [OGC Team Engine](https://github.com/opengeospatial/teamengine)
- [semstreams repository](https://github.com/C360Studio/semstreams)
