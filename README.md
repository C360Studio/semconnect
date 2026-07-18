# semconnect

**OGC API Connected Systems (CS API) gateway built on
[semstreams](https://github.com/C360Studio/semstreams).**

`semconnect` is the HTTP gateway and reference server half of the
SemStreams CS API split. SemStreams owns the non-product framework primitives:
graph, NATS request/reply, JetStream, ObjectStore, ownership, and projection.
Semconnect owns its OGC product bundle: OMS, SensorML, SWE Common, SOSA/SWE,
CS API vocabulary, GeoJSON boundary behavior, and the HTTP gateway that composes them into an
[OGC API Connected Systems v1.0](https://www.ogc.org/standards/ogc-api-connected-systems/)
REST surface.

The repository is no longer a scaffold. A fresh disposable migration run on
2026-07-18 against SemStreams beta.147 produced:

```text
total=137 passed=137 failed=0 skipped=0
```

## Current Status

- Migration candidate: SemStreams `v1.0.0-beta.147` at
  `5cc22c109594e48b7f1cec04bcaaf0106d85495a`.
- ETS pin: Botts CS API ETS `0.1-SNAPSHOT` at commit `d9caf33`.
- Reference binary: `cmd/cs-api-server`.
- Gateway package: `gateway/cs-api`.
- Conformance harness: `conformance/run.sh`.
- Go and UI implementation reviews are approved. The fresh beta.147 ETS run,
  revision readiness, and no-write query replay gates pass. Production remains
  no-go pending the immutable cutover rehearsal, shutdown-error disposition,
  and deployment approvals. Independent review found no conformance weakening.
- Open product and framework asks are tracked in
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
- Product packages at `message/oms`, `parser/sensorml`, `pkg/swecommon`, and
  `vocabulary/{csapi,oms,sosa,swe}`.

It does not fork framework-shaped graph, NATS, JetStream, ObjectStore,
ownership, or projection primitives. OGC package work is local product work;
gaps in the remaining framework substrate belong upstream in SemStreams.

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

The ownership boundary after ADR-S003 is:

| Area | Owner and surface |
|---|---|
| Graph reads/writes | SemStreams graph query, index query, batch, and entity mutation subjects |
| Spatial queries | SemStreams: `graph.spatial.query.bounds`, `graph.spatial.query.polygon` |
| Message substrate | SemStreams: `message.BaseMessage`; semconnect: `message/oms` payloads |
| Artifacts | SemStreams ObjectStore and `StorageReference`; semconnect artifact roles and schemas |
| Schemas | Semconnect: `pkg/swecommon` canonicalization and validation |
| Product vocabularies | Semconnect: `vocabulary/{csapi,oms,sosa,swe}` |
| Sensor encodings | Semconnect SensorML parser; SemStreams generic JSON-LD/RDF export substrate |
| NATS boundary | SemStreams: `natsclient` request/reply classification and test helpers |

### Graph Governance Posture

SemConnect writes CS API resources through SemStreams entity mutation subjects.
At the current framework pin, System SensorML writes also stamp the
`c360.csapi.system.v1` projection producer, bind a `NoBirthStub`
`sensorml.PredIsHostedBy` foreign-edge claim for the configured System ID
prefix, and forward child/foreign-edge triples through the mutation lane.
SemStreams normalizes those projected triples at graph-ingest, routes
foreign-subject edges onto their own entities, and meters unclaimed
`(message_type, predicate)` pairs.

That is the first governed graph-state integration point for this gateway.
Beta.147 requires the registered foreign-edge claim for hosted-child
resolution; conformance evidence must show the lane fired with zero unclaimed
or dropped edges. This is write-semantics governance and provenance, not HTTP
endpoint authorization.

Ownership claims themselves are not modeled as ordinary domain triples. Those
claims live in SemStreams' ownership substrate. Entity triples may still carry
provenance about graph materialization, source, audit, or lineage; for example,
SemStreams referential stubs can record which producer caused the stub to exist.
That is provenance on the entity, not ownership arbitration.

The historical framework/sister-repo boundary is documented in SemStreams
[ADR-044](https://github.com/C360Studio/semstreams/blob/main/docs/adr/044-ogc-connected-systems-framework-split.md);
ADR-S003 records the current boundary.

## Build And Test

```bash
openspec validate --all --strict
go test ./...
go build ./...
```

The repository carries the beta.147 migration under
`openspec/changes/migrate-semstreams-beta147/`; strict OpenSpec validation is a
release gate.

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
- [ADR-S003](docs/adr/003-semstreams-beta147-product-boundary-migration.md) -
  product ownership, semantic identity, and cutover decision.
- [beta.147 OpenSpec change](openspec/changes/migrate-semstreams-beta147/) -
  detailed beta.147 migration specification, tasks, and evidence.
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
