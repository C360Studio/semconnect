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

**Wired** — see `conformance/README.md` for full details.

`conformance/run.sh` brings up NATS + `cs-api-server` + OGC Team Engine
(with the [Botts CS API ETS](https://github.com/Botts-Innovative-Research/ets-ogcapi-connectedsystems10)
baked in via Docker git-URL build) on a shared compose network, invokes
the suite via Team Engine's REST API, and archives the TestNG XML
report. The ETS is pinned by commit SHA in `conformance/.ets-pin` per
ADR-S001 §4. `.github/workflows/conformance.yml` runs the same harness
on push to `main` and on PRs labelled `conformance`.

**Calibration reality at v0.1**: the pinned Botts ETS is `0.1-SNAPSHOT`
— scaffold only, real CS API conformance test classes deferred to
follow-up sprints upstream. A green TestNG report today validates
**harness wiring**, not spec coverage. When upstream lands real tests
(or the OGC org publishes an official ETS image), re-running
`conformance/run.sh` lights up the conformance picture without further
plumbing. The two known sister-side deferrals (`X-CS-Reconstructed-Lossy`
on `GET /systems/{id}`; `X-CS-Geometry-Available: false` on `GET /areas`)
will surface as Team Engine assertion failures once tests for those
resources land — track upstream on `semstreams` per ADR-S001 §9.

### Stage 9 — Conformance backend + fixture seed

Stage 6 brought up `nats` + `cs-api-server` + `teamengine` on a shared
compose network. That stack has no responder for the NATS subjects
`cs-api-server` publishes to: `graph.index.query.predicate`,
`graph.query.entity`, `graph.spatial.query.bounds/.polygon`,
`graph.mutation.triple.add_batch`. Every read endpoint returns 503,
which the Botts ETS surfaces as `fetchSensorMlInputs` /
`fetchGeoJsonInputs` `@BeforeClass` assertion failures (expected 200,
got 503) — and SKIPs 122 dependent tests via TestNG group dependencies.

The Stage 7 outcome (`passed=13 failed=2 skipped=122`) bottoms out
here. Until a graph backend responds, the harness is conclusively
infrastructure-only.

**What lands:**

1. **A fourth compose service: `semstreams-backend`.** Builds from the
   framework's own multi-stage `docker/Dockerfile` against a pinned
   commit SHA in `conformance/.ets-pin` (new vars
   `SEMSTREAMS_GIT_URL` + `SEMSTREAMS_COMMIT`). `run.sh`'s
   `ensure_ets_vendor` gains a sibling `ensure_semstreams_vendor` and
   clones into `.vendor/semstreams/`.
2. **`conformance/compose.semstreams.config.json`** — slim graph-only
   config. Five processors: `graph-ingest`, `graph-index`,
   `graph-index-spatial`, `graph-index-temporal`, `graph-query`. The
   non-graph noise (`udp`, `iot_sensor`, `document_processor`,
   `objectstore`, `rule`, `graph-gateway`, file inputs/outputs,
   `message-logger`, metrics forwarders) is stripped. `service-manager`
   re-ports to 8090 to avoid cs-api-server's 8080.
3. **Pre-suite seed step in `run.sh`.** After readiness gates and
   before invoking TestNG: `curl -XPOST /systems` with
   `fixtures/system.sml.json`, `curl -XPOST /datastreams` with a
   matching datastream body. Existing fixtures, write endpoints wired
   in Stage 8.
4. **Capture per-container logs alongside `teamengine`.** Reworked
   `on_exit` trap: success path captures `teamengine` +
   `cs-api-server` + `semstreams-backend`; failure path additionally
   captures `nats` and runs *before* `teardown_silent` (refactored
   `die()` to no longer tear down inline, so trap-captured logs
   survive). Triaging a 503 or healthcheck timeout now grep-able
   from `$OUTPUT_DIR/*-container-*.log`, not "stack already gone."

**Feasibility probe (2026-05-16, framework `v1.0.0-beta.73`):** booted
the slim config against a running NATS, all five required handler
subjects registered (`graph.mutation.triple.{add,add_batch,remove}`,
`graph.index.query.predicate` and siblings, `graph.spatial.query.{bounds,polygon}`,
`graph.query.entity` and siblings). With the slim backend running and
the existing conformance cs-api-server pointed at the same NATS,
`GET /systems` returned `HTTP 200 SystemCollection[empty]` and
`GET /areas?bbox=...` returned `HTTP 200 FeatureCollection[empty]` —
both 503s eliminated at the source.

**Required-port caveat:** `graph-ingest.Config.Validate()` requires
`len(Ports.Inputs) >= 1`. CS-API write flow uses
`graph.mutation.triple.add_batch` request/reply (auto-wired by
`setupMutationHandlers`, not a port), so we don't need an actual feed.
Declare a benign `{"name": "unused_in", "subject": "_semconnect.unused.ingest", "type": "nats"}`
input — the framework subscribes, nothing publishes, no behavior
change. (Filed as a candidate upstream relaxation when other gateways
hit the same wall — but a `nats`-type no-op satisfies validation
today, so not blocking.)

### Stage 10+ — Botts ETS pin bumps (open-ended)

The sponsor has confirmed Botts CS API ETS as the conformance target
through v1.0. Each pin bump (`conformance/.ets-pin: ETS_COMMIT`)
surfaces new assertion failures; triage is per-bump work. Track the
TestNG delta in the bump PR description so the reviewer sees what
conformance picture moved. ADR-S001 §4 documents the pin policy;
`conformance/README.md` documents the procedure.

## What lives where (recap)

```
semconnect/
├── cmd/cs-api-server/main.go    # Stage 1+
├── gateway/cs-api/
│   ├── systems.go               # Stage 2 (GET) + Stage 4 (POST/GET single)
│   ├── observations.go          # Stage 3
│   ├── spatial.go               # Stage 5
│   └── conformance.go           # conformance-class declaration
├── conformance/                 # Stage 6 (harness) + Stage 9 (backend + seed)
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
