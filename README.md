# semconnect

**OGC API Connected Systems (CS API) gateway built on
[semstreams](https://github.com/C360Studio/semstreams).**

`semconnect` is the HTTP gateway and reference server half of the
SemStreams CS API split. SemStreams owns the non-product framework primitives:
graph, NATS request/reply, JetStream, ObjectStore, and projection contracts.
Semconnect owns its OGC product bundle: OMS, SensorML, SWE Common, SOSA/SWE,
CS API vocabulary, GeoJSON boundary behavior, and the HTTP gateway that composes them into an
[OGC API Connected Systems v1.0](https://www.ogc.org/standards/ogc-api-connected-systems/)
REST surface.

The active SETUP 03A migration is **DRAFT / UNQUALIFIED**. The frozen target passes
all 137 external OGC tests and isolated persistence/spatial restart checks, but
rejects an existing federated Datastream create that beta.160 accepts. Independent
review approves the bounded implementation and evidence; full migration and merge
approval are withheld. See [the migration record](docs/semstreams-setup03a-migration.md).

## Current Status

- Shared target: SemStreams `v1.0.0-beta.162.0.20260930150212-8b99efe9c66a`.
  Commit `8b99efe9c66a4faa4fa509f9f62cc6bad8392128`, source tree
  `605f83cb8492eda3bd347ac30a211b9babf3931f`.
- SETUP 03A froze that main commit because beta.163 was unavailable. Reuse the
  frozen pin; a later tag or commit is not independently selected here.
- Fresh beta.160 baseline checkpoint: `55ea4121aba8658aeac9d70bdfe80f19c15fd4cd`.
  Both pins produce `137 passed / 0 failed / 0 skipped` with the unchanged ETS.
- The HTTP binary is `cmd/cs-api-server`; the new consumer-owned graph host is
  `cmd/cs-graph-backend`. Both use the same exact SemStreams dependency.
- Unit/race, vet and build pass. Full integration/race retains one failing
  assertion: a foreign six-part Datastream ID must return HTTP 201, but the
  target returns HTTP 400 with graph reason `authority_foreign`.
- Exact revisions, one-attempt mutations, root-only SensorML, immutable schema
  bytes, observation delivery and local spatial/restart parity remain verified.
  No assertion, retry, authority bypass or conformance waiver hides the blocker.
- NATS remains exactly 2.14.4. Baseline and target use separate fresh volumes;
  never open either revision's volume with another revision. The beta.160
  production authorization from 2026-08-12 does not authorize this target.
- OGC ownership remains in SemConnect. A later SemEngine switch requires its
  own qualified contract; no binary mixes both frameworks.

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

It does not fork framework-shaped graph, NATS, JetStream, ObjectStore, or
projection primitives. OGC package work is local product work;
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
| Graph reads/writes | SemStreams exact/batch queries and typed `semstreams.graph.mutation/v1` operations |
| Spatial queries | SemStreams: `graph.spatial.query.bounds`, `graph.spatial.query.polygon` |
| Message substrate | SemStreams: `message.BaseMessage`; semconnect: `message/oms` payloads |
| Artifacts | SemStreams ObjectStore and `StorageReference`; semconnect artifact roles and schemas |
| Schemas | Semconnect: `pkg/swecommon` canonicalization and validation |
| Product vocabularies | Semconnect: `vocabulary/{csapi,oms,sosa,swe}` |
| Sensor encodings | Semconnect SensorML parser; SemStreams generic JSON-LD/RDF export substrate |
| NATS boundary | SemStreams: `natsclient` request/reply classification and test helpers |

### Graph Governance Posture

Semconnect writes eleven resource families through local registered projection
contracts and the typed `semstreams.graph.mutation/v1` family. Create validates
the message type, entity pattern, and declared predicates before I/O. Replace,
PATCH, and delete use the exact nonzero KV revision read from the entity; a
revision mismatch returns HTTP 409 with no hidden retry.

SensorML mutation is root-only. Root facts and root-to-child references may be
retained, but foreign-subject inverse facts are dropped and embedded children
are not materialized. Schema artifacts are immutable, content-addressed values
in provider `objectstore`, bucket `CS_API_ARTIFACTS`; orphan garbage collection
is deferred. Graph mutations emit structured gateway audit records rather than
claiming custom identity-header propagation on the mutation wire.

The historical framework/sister-repo boundary is documented in SemStreams
[ADR-044](https://github.com/C360Studio/semstreams/blob/main/docs/adr/044-ogc-connected-systems-framework-split.md);
ADR-S003 records the current boundary.

## Build And Test

```bash
openspec validate --all --strict
go test ./...
go build ./...
```

The repository preserves prior migrations as historical evidence. Active SETUP 03A
contracts and evidence are under `openspec/changes/migrate-semstreams-setup03a/`.
Strict OpenSpec validation remains a release gate; passing it does not clear the
recorded federation compatibility blocker.

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
- [beta.149 OpenSpec change](openspec/changes/qualify-semstreams-beta149/) -
  historical dependency, shutdown, replay, and conformance qualification.
- [beta.151 OpenSpec change](openspec/changes/qualify-semstreams-beta151/) -
  historical structural, retained-state, replay, and conformance baseline.
- [beta.153 OpenSpec change](openspec/changes/qualify-semstreams-beta153/) -
  historical bug/performance release and greenfield deployment evidence.
- [beta.159 OpenSpec change](openspec/changes/qualify-semstreams-beta159/) -
  historical readiness, storage-bound, graph-semantics, and clean-volume qualification.
- [ADR-S004](docs/adr/004-semstreams-beta160-typed-graph-migration.md) -
  typed graph, revision, projection, artifact, audit, and fresh-state decisions.
- [beta.160 OpenSpec change](openspec/changes/migrate-semstreams-beta160/) -
  active migration, evidence, and accepted-risk production decision.
- [beta.160 operations](docs/semstreams-beta160-operations.md) - fresh-volume,
  restart, rollback, conflict, uncertainty, and artifact runbook.
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
