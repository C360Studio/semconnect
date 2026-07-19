# Getting Started — semconnect Bootstrap Playbook

This is the historical bootstrap playbook and stage log for `semconnect`.
The repository is no longer freshly initialized: Stages 2 through 55 have
landed, and the pinned conformance harness is green at
`total=137 passed=137 failed=0 skipped=0`. Keep this file as the narrative of
how the gateway was built; use [README.md](../README.md),
[AGENTS.md](../AGENTS.md), and [conformance/README.md](../conformance/README.md)
for the current operating picture.

That green result is the 2026-07-06 beta.141 historical baseline. ADR-S003 and
`openspec/changes/migrate-semstreams-beta147/` record the signed beta.147
product-boundary migration foundation. In particular, references below to SemStreams
owning OMS, SensorML, SWE Common, SOSA/SWE, or CS API vocabulary describe the
historical build sequence; those packages are now semconnect-owned.

The historical disposable beta.147 run on 2026-07-18 passed the external ETS at
`137 passed, 0 failed, 0 skipped`, revision readiness, foreign-edge, and
archived no-write query replay gates. Independent review found no conformance
weakening. It also exposed the heartbeat shutdown error that beta.149 subsequently
closed; it did not authorize production.

The dependent beta.149 qualification subsequently passed `137/0/0` and closed
the beta.147 heartbeat shutdown blocker. It remains signed historical evidence.
Beta.151 then passed its authoritative post-review fresh-volume `137/0/0` run,
retained-state, normal-SIGTERM, readiness, no-write replay, and foreign-edge
gates. It is the current qualified historical baseline.

The active OpenSpec change,
`openspec/changes/qualify-semstreams-beta153/`, qualifies beta.153's graph-ingest
bug and performance fixes. Its exact pin, live per-entity structural regression,
full Go test/race/vet/build, focused upstream, clean-volume Compose persistence,
and unchanged external `137/0/0` gates pass. Independent review found no
legacy/compatibility code or conformance weakening. Frontend/Svelte is N/A
because the public CS API and UI did not change. The beta.153 Compose bundle is
production-ready for standard startup on clean NATS; there is no migration,
runtime manifest, or product-owner hash approval.

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

**Historical calibration note:** at Stage 6, the Botts ETS was still a
scaffold and a green TestNG report validated harness wiring rather than full
spec coverage. Subsequent stages closed that gap. As of Stage 55, the same
pinned ETS reports `total=137 passed=137 failed=0 skipped=0`; see
`conformance/README.md` for the current harness picture.

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

### Stage 12 — OAS3 service definition + `oas30` conformance class

PR #12 (landing on top of Stages 9+10+11). The cascade-unblocker that
took us from "infrastructure verified, two upstream-ETS bugs blocking
real CS API tests" to "every assertion against our claimed conformance
set passes."

**What lands:**

1. **Vendored OGC OAS3 source** at `api/upstream/` from
   [`opengeospatial/ogcapi-connected-systems`](https://github.com/opengeospatial/ogcapi-connected-systems)
   at pinned commit. License-compliant (OGC permissive license, see
   `api/upstream/LICENSE-OGC.txt` + `NOTICE-OF-MODIFICATIONS.md`).
   The vendored copy is not served — it's the source-of-truth we adapt
   from.
2. **Hand-authored `gateway/cs-api/openapi.yaml`** — single-file OAS3
   served at `GET /api` (and HEAD). v0.1 paths inline with honest
   shapes (`X-CS-*` response headers as spec contract elements,
   `items` field per CS API §7.13); roadmap paths from OGC vendored
   inline with `x-not-implemented-at-v01: true` extension and an
   `x-cs-upstream-source` pointer back into `api/upstream/`.
3. **`/api` handler** — `application/vnd.oai.openapi+json;version=3.0`
   default (boot-time YAML→JSON conversion via `gopkg.in/yaml.v3` +
   `encoding/json` indent), `application/vnd.oai.openapi;version=3.0`
   alt returns raw YAML. `?f=yaml`/`?f=openapi` short names.
4. **Landing page** — `rel=service-desc` link pointing at `/api`,
   `rel=systems`/`rel=datastreams` resource-specific links.
   Hrefs ABSOLUTE (was relative) via `absoluteBase(r)` helper —
   REST-Assured-shaped clients (the Botts ETS) don't auto-resolve
   relative URIs.
5. **`/conformance`** — claims `.../conf/oas30` honestly.
6. **Predicate-index lookup fix** — `predicateRDFType` constant in
   `systems.go` was misnamed AND wrong-valued: pointed at
   `"rdf.type"` but sensorml emits triples under
   `"sensorml.process.type"`. Renamed to `predicateClassType =
   sensorml.PredType`. Hidden bug since Stage 2 because we never had
   data in the graph during a probe; Stage 12 cascade-unblock
   surfaced it.
7. **`rel=canonical` link** on `/systems/{id}` and `/datastreams/{id}`
   per CS API §7 `/req/system/canonical-url`.
8. **`negotiation.go` MIME-parameter stripping** — accept-match was
   treating `application/vnd.oai.openapi;version=3.0`'s `version`
   parameter as part of the subtype. Now strips parameters from the
   supported side before matching.
9. **`conformance/run.sh` poll-until-visible** after seed — POST
   writes to ENTITY_STATES synchronously but graph-index updates
   PREDICATE_INDEX via async KV-watch. Poll `/systems` until
   `numberReturned > 0` before invoking the suite.

**Outcome:** `total=137 passed=20 failed=0 skipped=117`. From
Stage 11's `passed=13 failed=2`. The 117 SKIPs are tests gated on
conformance classes / resources we haven't claimed at v0.1 (Part 2
write side, Update, Advanced Filtering, subsystems / samplingFeatures
/ procedures / sub-resource item GETs).

**What's NOT in scope (deferred to follow-up stages):** every path
marked `x-not-implemented-at-v01: true` in `openapi.yaml`. Each lands
as its own stage with the upstream OGC OAS path block as the starting
point — copy from `api/upstream/`, flip the extension off, point at
a real handler.

### Stage 13 — semstreams pin bump to v1.0.0-beta.75 + retire two `X-CS-*` deferral headers

Small infrastructure cleanup stage. semstreams v1.0.0-beta.74 + v1.0.0-beta.75
landed two changes that retire two of the three honest-deferral headers
cs-api was carrying:

1. **`feat(graph-index-spatial): SpatialResult carries Lat/Lon/Alt`** (semstreams 6def801) — `GET /areas` now emits real Point geometry built from `SpatialResult.Lat/Lon/Alt`; `X-CS-Geometry-Available: false` header retired.
2. **`feat(vocabulary/csapi): add OGC Connected Systems v1.0 Datastream package`** (semstreams b3f705e) — `DatastreamTypeIRI` now aliases `csapi.Datastream` (spec-rooted) instead of our locally-minted HTTPS IRI; `X-CS-Datastream-Subset: true` header retired; `docs/upstream-asks/semstreams-datastream-vocabulary.md` renamed to `RESOLVED-…`.

`go.mod` bumped, `conformance/.ets-pin` `SEMSTREAMS_COMMIT` bumped to match. `gateway/cs-api/openapi.yaml` schemas updated to reflect real Point geometry + drop the retired headers from response shapes.

**Outcome:** `total=137 passed=20 failed=0 skipped=117`. Identical to Stage 12 — no regressions, no new PASSes. The headline numbers didn't move because the lone systemfeatures MAY-priority test (`systemItemHasGeometryOrValidTime`) still blocks the sensorml + geojson cascade groups; that test asserts geometry OR validTime on `/systems/{id}` and the framework's sensorml emitter still doesn't preserve the `position` field through the triple round-trip. That's the Stage 14 target.

The third deferral header — `X-CS-Reconstructed-Lossy: true` on `GET /systems/{id}` — stays in place; it's a property of our triple-round-trip reconstruction and won't retire until the framework's triple emitter preserves more fields (or we switch to a different storage strategy).

### Stage 14 — SensorML position-preservation sister-side workaround + `sml+json` media type alias

The chokepoint cascade unblocker. Pre-Stage 14, one MAY-priority ETS test (`systemItemHasGeometryOrValidTime`) emitted `SkipException` because `/systems/{id}` lacked geometry, keeping the entire `systemfeatures` group "not successfully finished" → sensorml + geojson groups cascade-SKIPped (~26 tests).

The framework's `parser/sensorml` has no `Position` field on `AbstractProcess` and emits no position triple (verified at framework v1.0.0-beta.75). Upstream ask drafted at `docs/upstream-asks/RESOLVED-semstreams-sensorml-position-preservation.md`. Until upstream lands, sister-side workaround:

1. **POST /systems** (`gateway/cs-api/systems_post.go`): after `sensorml.NewAsset(...).Triples()`, peek the raw POST body for a top-level `position` field (`extractPositionTriple`). If present + not literal `null`, append a triple `(entityID, "cs-api.system.position", <raw GeoJSON bytes as string>)` to the batch. `PredSystemPosition` const documents the workaround + retire-on-upstream-fix path.
2. **GET /systems/{id}** (`gateway/cs-api/systems.go` `systemFromState`): look for the `cs-api.system.position` triple; if present, populate a new `Geometry json.RawMessage` field on the System struct (omitempty). RawMessage preserves the exact GeoJSON bytes from POST — no re-marshal-through-interface{} precision loss.
3. **OAS schema** (`gateway/cs-api/openapi.yaml`): `System.geometry` documented as the GeoJSON-shaped recovered position, with retire-on-upstream-fix note.

Also: **`application/sml+json` media type alias.** CS API §11.7 + the Botts ETS use the spec form `application/sml+json`, not the longer `application/sensorml+json` we'd been serving. Stage 14 made `MediaSensorML = "application/sml+json"` (spec form, primary) and added `MediaSensorMLLegacy = "application/sensorml+json"` (backward-compat). Both `Accept` and POST `Content-Type` honor either; `Accept-Post` advertises both. New `requireMediaTypeAny()` helper in `observations.go` for multi-candidate Content-Type matching.

**Outcome:** `total=137 passed=29 failed=1 skipped=107`. From Stage 12 baseline (`passed=20 failed=0 skipped=117`): +9 newly passing tests (the entire cascade unblock), +1 failure (the next layer surfaced — `geoJsonMediaTypeRead` wants `application/geo+json` on `/systems` collection, deferred to Stage 15), -10 SKIPs.

The 9 newly-passing tests: `systemItemHasGeometryOrValidTime` (the chokepoint) + 6 sensorml + 2 geojson cascade-runtime tests.

### Stage 15 — GeoJSON FeatureCollection on `/systems` collection

Closes the lone Stage 14 failure (`geoJsonMediaTypeRead`). `FamilySystemCollection.supported()` extended with `MediaGeoJSON`; new `writeSystemsGeoJSON` branch in `handleSystems` fetches each entity's state to recover the `cs-api.system.position` triple (Stage 14), builds a Feature with that as `geometry` and the System's reconstructed fields as `properties`, returns an RFC 7946 FeatureCollection.

Failure mode at Stage 15: transient backend errors on the FIRST entity → 503 (subsequent entities would fail identically); transient errors after the first → log + degrade to null-geometry Feature (one bad row doesn't poison the page). Stage 40 later replaces per-entity requests with batch hydration; malformed position triples still log + null geometry.

Implementation note: Stage 40 later moves hydrated collection reads to
`graph.query.batch`.

**Outcome:** `total=137 passed=32 failed=0 skipped=105`. From Stage 14 (29/1/107): +3 newly passing (`geoJsonMediaTypeRead`, `systemFeatureHasGeoJsonShapeAndProperties`, `systemsCollectionIsGeoJsonFeatureCollection`), -1 failure, -2 SKIPs.

### Stage 16 — CS API §7.6 create-replace-delete on `/systems`

Closes the largest single-class gap left after Stage 15: the CRD
conformance class. Three new verbs land on `/systems`:

- **POST /systems** with `application/json` / `application/geo+json`
  (GeoJSON Feature body). Joins the existing `application/sml+json` +
  `application/sensorml+json` accept set. `Content-Type` selects the
  branch in `handleSystemPost`; the JSON Feature builder
  (`buildSystemTriplesFromFeature`) mints the entity ID from
  `properties.uid`, emits `rdf:type ssn:System` + `PredLabel` +
  `PredDescription` + (when geometry present) the Stage 14
  `cs-api.system.position` triple. PUT re-uses this builder so the
  same body works on both verbs.
- **PUT /systems/{id}** with the same GeoJSON Feature body. **No
  SensorML on PUT** — the lossy reverse-mapping would mismatch the
  read-back shape and surprise clients. The handler verifies the
  body's uid mints to the path ID *before* any destructive operation
  (mismatch → 400; no remove called). Replace semantics are
  implemented as `deleteAllEntityTriples` + `ingestTriples`. N
  per-predicate round-trips per call. Stage 30 confirms semstreams
  beta.87 now exposes entity-level mutation subjects, so replacing
  this fan-out is local semconnect cleanup.
- **DELETE /systems/{id}** — idempotent (errEntityNotFound is
  swallowed). 204 No Content.
- **OPTIONS /systems** + **OPTIONS /systems/{id}** — advertise the
  `Allow` header so the ETS confirms write-side readiness without
  exercising the verbs. PATCH is intentionally absent.

The conformance harness gains two opt-in flags
(`mutation-tests-enabled=true` + `mutation-iut-policy=dedicated-mutable-iut`)
because the harness's stack is ephemeral per run — `compose down -v`
at start makes the IUT honestly dedicated and mutable. Without these
flags the ETS skips the CRD lifecycle tests via
`ensureMutationEnabledOrSkip` and the conformance picture misses the
real evidence of POST/PUT/DELETE round-trip.

`stageConformanceClasses` claims
`http://www.opengis.net/spec/ogcapi-connectedsystems-1/1.0/conf/create-replace-delete`.
`update` (PATCH) is intentionally NOT claimed at v0.1.

**Outcome:** `total=137 passed=38 failed=2 skipped=97`. From Stage 15
(32/0/105): +6 newly passing tests (the CRD lifecycle group), -8
SKIPs, +2 failures — both are read-back uid-preservation gaps the
mutation opt-in surfaced (Stage 18 target). NOTE: this number
materialized only after PR #19 (`chore/conformance-compose-wait-fix`,
2026-05-17) fixed a GHA `docker compose up --wait` regression that
was making every main-branch probe FATAL since 2026-05-16. Pre-fix
the harness couldn't even start the stack.

### Stage 17 — CS API §10.6 create-replace-delete on `/datastreams`

Brings `/datastreams` to parity with Stage 16's `/systems` CRD set so
the `create-replace-delete` conformance class claim is honest across
every resource type the IUT implements.

- **PUT /datastreams/{id}** — `application/json` only (no SensorML on
  datastreams; CS API §10 doesn't define one). Re-validates required
  `system` (6-part SemStreams strict) + `observedProperty` (non-empty
  IRI). Body `id` (if present) must match path; mismatch yields 400
  *before* any destructive remove. Re-uses `datastreamToTriples` +
  `deleteAllEntityTriples` + `ingestTriples` (same partial-erasure
  window + same audit-headers symmetry as Stage 16 /systems).
- **DELETE /datastreams/{id}** — idempotent (errEntityNotFound
  swallowed → 204). Stage 17 only deleted graph triples; Stage 36
  adds subject-scoped JetStream purge for
  `cs-api.observations.{datastreamID}` after graph deletion succeeds.
- **OPTIONS** on collection + item, same shape as /systems.

The `conformance.go` claim comment is updated to note both resource
types now serve the full CRD verb set — no more partial-claim
disclaimer.

**Outcome:** `total=137 passed=38 failed=2 skipped=97` (unchanged
from Stage 16's headline numbers). Stage 17's contribution was
making the `create-replace-delete` claim *honest* across both
resource types — the ETS's CRD lifecycle tests already passed when
exercised against /systems alone at Stage 16, so the additional
/datastreams verbs didn't surface new tests. The 2 failures
(read-back uid preservation) carried over and are Stage 18's
target.

### Stage 18 — uid preservation on read-back

Closes the 2 remaining failures Stage 16's mutation opt-in surfaced:
`sensorMlMediaTypeWriteParsesSystemBodyWhenMutationEnabled` and
`geoJsonMediaTypeWriteParsesSystemBodyWhenMutationEnabled`. Both
assert that a POST → GET round-trip preserves the
client-submitted `uniqueId` / `properties.uid` on the response, via
any of three field-name fallbacks: top-level `uid`, top-level
`uniqueId`, nested `properties.uid`.

Sister-side workaround mirroring Stage 14's
`cs-api.system.position` pattern:

- New predicate constant `PredSystemUID = "cs-api.system.uid"` in
  `systems_post.go`.
- `buildSystemTriplesFromSensorML` appends the triple when
  `process.Base().UniqueID != ""` (absent uniqueId leaves no
  synthetic value — the entity ID would mislead the read-back).
- `buildSystemTriplesFromFeature` appends unconditionally
  (`properties.uid` is required by the Feature builder).
- `systemFromState` surfaces the preserved value on top-level
  `uid`, top-level `uniqueId`, AND nested `properties.uid` via a
  new `featureProperties` container — belt-and-suspenders so each
  client family finds the spelling it expects on the same response.
- `buildAbstractProcess` (SensorML reverse-mapping) reads the
  triple and writes it back onto `process.Base().UniqueID`.

**Breaking field rename**: the JSON System subset's `properties`
field (which always carried SensorML characteristics, lossily
reconstructed) is renamed to `characteristics`. The previous name
was semantically wrong — SensorML characteristics ≠ GeoJSON Feature
properties — and freeing the `properties` JSON key was a
prerequisite for adding the Feature-shape container. Documented in
the OAS3 schema and via the long-standing
`X-CS-Reconstructed-Lossy: true` deferral header.

Upstream ask drafted at
`docs/upstream-asks/RESOLVED-semstreams-sensorml-uid-preservation.md`. When
upstream lands the emission natively, the workaround triple +
write/read code on this side retires the same way Stage 13 retired
the `X-CS-Geometry-Available` header.

**Outcome:** `total=137 passed=40 failed=0 skipped=97` (confirmed
post-merge 2026-05-17). Both uid-preservation failures flipped
PASS. **Zero failures against the claimed conformance set** — every
assertion the harness can run now passes. The 97 SKIPs are gated
on conformance classes / resources we haven't claimed at v0.1
(Part 2 write side, `conf/update` (PATCH), Advanced Filtering, and
all sub-resource item GETs).

### Stage 19 — CS API `conf/update`: PATCH /systems/{id}

Closes the `conf/update` conformance class with PATCH partial-update
semantics on /systems. The ETS's `UpdateTests` scenario POSTs a
Feature, PATCHes only `properties.name`, GETs back, asserts the new
name is present and the other fields are unchanged.

Implementation:

- New `handleSystemPatch` in `systems_patch.go`. Body shape is the
  same `SystemFeature` POST/PUT accept, with permissive validation:
  `type: "Feature"` enforced only when present; all
  `properties.*` fields optional; the path `{id}` is authoritative.
- `mergePatchSystemTriples` reads the existing triple set and walks
  it, replacing the triple under each predicate the body addresses
  (`name` → `PredLabel`, `description` → `PredDescription`,
  `geometry` → `PredSystemPosition`). Fields the body doesn't
  address survive untouched. Fields the body addresses but the
  entity didn't have are appended fresh.
- Body-uid-vs-existing-uid safety gate runs *before* any
  destructive operation (same shape as PUT).
- 404 on missing entity — PATCH is strict, NOT upsert (PUT remains
  the upsert path).
- Re-uses Stage 16's `deleteAllEntityTriples` + `ingestTriples`
  two-step replace, so the same partial-erasure window applies
  (surfaced via `X-CS-Partial-Delete: true` on the add-batch
  failure path).
- `handleSystemOptions` Allow header gains `PATCH`.
- `conformance.go` claims
  `http://www.opengis.net/spec/ogcapi-connectedsystems-1/1.0/conf/update`,
  scoped to /systems at v0.1 (/datastreams PATCH is a follow-up,
  same partial-claim precedent as Stage 16 used for CRD).

**No JSON Merge Patch null-as-delete** (RFC 7396) at v0.1 — a
`null` body field is treated as a no-op rather than a remove. The
ETS doesn't exercise it; the conservative stance avoids surprising
existing clients.

**Expected outcome:** the ETS's `update` group (currently SKIPped)
runs. Probe projection: ~4 newly passing tests from the
`updateSystemPatchLifecycleOptIn` + readiness + dependency-cascade
assertions; `passed=44 / failed=0 / skipped=93` from current
40/0/97. The conformance-declaration test will need its expected
list updated to include `conf/update`.

### Stage 20 — CS API §6 Procedures resource (OSH bar)

Sponsor set the new bar 2026-05-17: "at least as compliant as
OpenSensorHub." OSH's public IUT at `api.georobotix.io` declares
**34 conformance classes** vs our **11**. The gap is dominated by
resource types (procedures, deployments, samplingFeatures,
properties, controlStreams, system-history, system-event) plus SWE
Common encodings plus HTML plus Part 3 protocols. Stages 20-27
address the read-side resource-type gap; Stage 28+ defers HTML and
Part 3.

Stage 20 ships `/procedures`:

- `GET /procedures` (collection) — predicate-query on
  `rdf:type = sosa.Procedure`; JSON `ProcedureCollection`.
- `GET /procedures/{id}` — JSON Procedure subset (id, type, label,
  description, definition, uid/uniqueId/properties.uid). Per
  `/req/procedure/location`, procedures MUST NOT carry geometry —
  the JSON shape omits the field entirely.
- `POST /procedures` — same four media types `POST /systems`
  accepts (sml+json, sensorml+json, json, geo+json). SensorML path
  feeds `buildProcedureTriplesFromSensorML` which OVERRIDES the
  emitted rdf:type to `sosa.Procedure` (so a PhysicalSystem
  mistakenly POSTed still lands correctly). Feature path feeds
  `buildProcedureTriplesFromFeature` — same minimum shape as
  `/systems`, no position triple emitted.
- `OPTIONS /procedures` (`GET, HEAD, POST, OPTIONS`) and
  `OPTIONS /procedures/{id}` (`GET, HEAD, OPTIONS`). PUT/DELETE/
  PATCH intentionally absent — the ETS CRD/update test groups only
  exercise them against /systems, so the existing
  `conf/create-replace-delete` + `conf/update` claims stay honest
  at /systems-only.
- `conformance.go` claims
  `http://www.opengis.net/spec/ogcapi-connectedsystems-1/1.0/conf/procedure`.
- Conformance harness gains a seed Procedure fixture so the ETS
  procedures test group has non-empty `/procedures` to exercise.
- New config field `ProcedureIDPrefix` (default
  `c360.semconnect.systems.csapi.procedure`).

**Expected outcome:** the ETS `procedures` group runs and passes
its 3-4 assertions (collection-returns-200,
items-have-no-geometry, item-has-id-type-links,
item-has-canonical-link). Probe projection: `passed=49 / failed=0 /
skipped=88` from current 45/0/92 (+4 procedures-group tests, -4 SKIPs).

### Stage 21 — CS API §8 Deployments resource (OSH bar)

Stage 21 ships `/deployments`:

- `GET /deployments` (collection) — predicate-query on
  `rdf:type = ssn:Deployment`; JSON `DeploymentCollection` or
  `application/geo+json` FeatureCollection with per-deployment
  geometry recovered from the position triple.
- `GET /deployments/{id}` — JSON Deployment subset with geometry
  when present. Stage 53 later adds SensorML item reads.
- `POST /deployments` — `application/json` / `application/geo+json`
  Feature body only. SensorML is intentionally absent; no CS API
  encoding pairs SensorML with Deployment.
- `OPTIONS /deployments` (`GET, HEAD, POST, OPTIONS`) and
  `OPTIONS /deployments/{id}` (`GET, HEAD, OPTIONS`).
- `conformance.go` claims
  `http://www.opengis.net/spec/ogcapi-connectedsystems-1/1.0/conf/deployment`.
- Conformance harness gains a seed Deployment fixture with Point
  geometry.
- New config field `DeploymentIDPrefix` (default
  `c360.semconnect.systems.csapi.deployment`).

**Outcome:** rolled into the Stage 22 conformance probe below. The
Stage 22 run includes the landed Stage 21 deployment group and keeps
the claimed conformance set at zero failures.

### Stage 22 — CS API Sampling Features resource (OSH bar)

Stage 22 ships `/samplingFeatures`:

- `GET /samplingFeatures` (collection) — predicate-query on
  `rdf:type = sosa:Sample`; JSON `SamplingFeatureCollection` or
  `application/geo+json` FeatureCollection. Sampling Feature geometry
  is treated as first-class GeoJSON resource data.
- `GET /samplingFeatures/{id}` — JSON SamplingFeature subset with
  `uid` / `uniqueId` / nested `properties.uid` and geometry when
  present.
- `POST /samplingFeatures` — `application/json` /
  `application/geo+json` Feature body; mints from `properties.uid`,
  stores label/description, and preserves optional geometry.
- `OPTIONS /samplingFeatures` (`GET, HEAD, POST, OPTIONS`) and
  `OPTIONS /samplingFeatures/{id}` (`GET, HEAD, OPTIONS`).
- `conformance.go` claims
  `http://www.opengis.net/spec/ogcapi-connectedsystems-1/1.0/conf/sf`.
- Conformance harness gains a seed SamplingFeature fixture with
  Polygon geometry.
- New config field `SamplingFeatureIDPrefix` (default
  `c360.semconnect.systems.csapi.samplingfeature`).

**Outcome:** `total=137 passed=58 failed=0 skipped=79` (confirmed
2026-05-18 after adding Stage 22 and hydrating GeoJSON Feature
properties for procedures, deployments, and sampling features with
`uid` / `name` / `description`). The TeamEngine host-port readiness
check now polls because Tomcat can briefly reset connections after
Docker starts the container but before `/teamengine/` is serving.

### Stage 23 — CS API Properties resource (OSH bar)

Stage 23 ships `/properties`:

- `GET /properties` (collection) — predicate-query on
  `rdf:type = sosa:ObservableProperty`; JSON `PropertyCollection`.
- `GET /properties/{id}` — JSON Property subset with
  `uid` / `uniqueId`, label, description, definition, and optional
  baseProperty recovered from triples. Stage 53 later adds SensorML
  item reads.
- `POST /properties` — accepts `application/sml+json`,
  `application/sensorml+json`, or `application/json` SensorML
  DerivedProperty-shaped JSON. The v0.1 subset stores `uniqueId`,
  label/name, description, definition, and baseProperty.
- `OPTIONS /properties` (`GET, HEAD, POST, OPTIONS`) and
  `OPTIONS /properties/{id}` (`GET, HEAD, OPTIONS`).
- `conformance.go` claims
  `http://www.opengis.net/spec/ogcapi-connectedsystems-1/1.0/conf/property`.
- Conformance harness gains a seed Property fixture.
- New config field `PropertyIDPrefix` (default
  `c360.semconnect.systems.csapi.property`).

**Outcome:** `total=137 passed=62 failed=0 skipped=75` (confirmed
2026-05-19). From Stage 22 (58/0/79): +4 newly passing
properties-group tests, -4 SKIPs, zero failures.

### Stage 24 — CS API Part 2 Control Streams read-side (OSH bar)

Stage 24 ships the ControlStream read subset:

- `GET /controlstreams` — predicate-query on
  `vocabulary/csapi.ControlStream`, then entity hydration so collection
  `items` are full ControlStream resources. Stage 40 later moves this to
  `graph.query.batch`.
- `GET /controlstreams/{id}` — JSON ControlStream subset with system
  reference, inputName, controlledProperties, issue/execution time
  placeholders, formats, live/async flags, and command links. Stage 47
  later adds canonical Part 2 alias `GET /controls/{id}` to the same
  handler.
- `GET /controlstreams/{id}/schema` — stored command schema subset
  with `commandFormat` and `parametersSchema`.
- `GET /controlstreams/{id}/commands` — readable empty Command
  collection at Stage 24. Stage 51 later populates it from read-side
  Command metadata while keeping command execution out of scope at v0.1.
- `GET /systems/{id}/controlstreams` — system-scoped collection,
  filtered by `vocabulary/csapi.ControlsSystem`.
- `POST /controlstreams` — JSON fixture helper for the conformance
  harness. It creates read-side metadata only; it is not a command
  execution path.
- `conformance.go` claims
  `http://www.opengis.net/spec/ogcapi-connectedsystems-2/1.0/conf/controlstream`.
- New config field `ControlStreamIDPrefix` (default
  `c360.semconnect.systems.csapi.controlstream`).

**Outcome:** `total=137 passed=62 failed=0 skipped=75` (confirmed
2026-05-19). No regression from Stage 23. The pinned ETS fetches
`/controlstreams?limit=2` during fixture setup, but all Part 2
ControlStream assertions remain SKIPPED because the TestNG methods
depend on the existing `common` group being fully successful. This
stage therefore lands the endpoint surface and conformance fixture
without changing headline counts.

### Stage 25 — CS API Part 2 System Events read-side (OSH bar)

Stage 25 ships the SystemEvent read subset:

- `GET /systemEvents` — predicate-query on
  `vocabulary/csapi.SystemEvent`, then entity hydration so collection
  `items` are full SystemEvent resources. Stage 40 later moves this to
  `graph.query.batch`; Stage 47 later exposes the same collection at
  `/collections/all_system_events/items` for ETS Part 2 discovery.
- `GET /systemEvents/{id}` — JSON SystemEvent subset with
  `time` / `eventTime`, `eventType`, message, system reference,
  source, severity, keywords, optional payload, and links.
- `GET /systems/{id}/events` — normative Requirement 43
  system-scoped collection path, filtered by
  `vocabulary/csapi.EventForSystem`.
- `GET /systems/{id}/events/{eventID}` — system-scoped item alias;
  404s if the event is not associated with the path system.
- `POST /systemEvents` and `POST /systems/{id}/events` — JSON fixture
  helpers for the conformance harness. They create read-side event
  facts only; streaming/SSE event delivery remains out of scope.
- `conformance.go` claims Part 2 `conf/api-common` and
  `conf/system-event`. Landing-page links now advertise the Part 2
  `/controlstreams` and `/systemEvents` resource collections so the
  API Common claim has discovery evidence.
- New config field `SystemEventIDPrefix` (default
  `c360.semconnect.systems.csapi.systemevent`).

**Outcome:** `total=137 passed=62 failed=0 skipped=75` (confirmed
2026-05-19). No regression from Stage 24. The pinned ETS fetches
Part 2 setup inputs, and the harness seeds a SystemEvent, but Part 2
API Common / ControlStream / SystemEvent assertion methods remain
SKIPPED because TestNG declares them dependent on the existing
`common` group being fully successful. This stage lands the endpoint
surface and declarations without changing headline counts.

### Stage 26 — System History read-side (OSH-bar vendor extension)

Stage 26 ships the OSH-compatible System History read subset:

- `GET /systems/{id}/history` — returns a `SystemCollection`
  containing the current system description as the single available
  revision. The response sets `X-CS-History-Current-Only: true`.
- `GET /systems/{id}/history/current` — returns the same JSON System
  representation as `/systems/{id}`. Unknown revision IDs return 404
  before a backend lookup.
- `OPTIONS /systems/{id}/history` and
  `OPTIONS /systems/{id}/history/{revID}` advertise `GET, HEAD,
  OPTIONS`.
- The OpenAPI description marks both paths `x-cs-vendor-extension:
  true` and points back to the upstream Part 2 history path files.

No conformance class is claimed. The pinned Botts ETS explicitly
documents `/conf/system-history` as a GeoRobotix vendor extension, not
an OGC 23-002 Annex A class, so claiming it would make `/conformance`
less honest.

**Outcome:** `total=137 passed=62 failed=0 skipped=75` (confirmed
2026-05-21). No headline movement from Stage 25. This stage is
API-surface parity work for the OSH bar, not an active ETS assertion
unlock in the pinned suite.

### Stage 27 — SWE Common observation value encodings (Part 2 subset)

Stage 27 starts the SWE Common encoding work on the observation read
side:

- `GET /datastreams/{id}/observations` now negotiates
  `application/swe+json`, `application/swe+csv`, and
  `application/swe+binary` in addition to the existing
  `application/json` and `application/om+json`.
- `application/swe+json` returns a JSON object with `items[]`, where
  each item carries the stored observation `time` and `result` value
  recovered from the OMS payload.
- `application/swe+csv` returns a `time,result` CSV value stream.
- `application/swe+binary` returns the same value records as bytes.
  This is intentionally labelled with `X-CS-SWE-Subset:
  observation-values`; full schema-bound SWE binary layouts remain
  deferred until observation schema binding lands.
- No SWE Common conformance class is claimed yet. The pinned ETS still
  marks the SWE Common suites deferred, and v0.1 has not completed the
  command/schema side of those classes.

**Outcome:** `total=137 passed=62 failed=0 skipped=75` (confirmed
2026-05-21). No TestNG headline movement from Stage 26 because the
pinned ETS still defers SWE Common suites and v0.1 does not claim a SWE
Common conformance class for this observation-value subset.

### Stage 28 — OGC API Common Part 2 Collections metadata

Stage 28 keeps the work on the read/discovery path while upstream
semstreams works the framework issues filed from Stages 14-27:

- `GET /collections` returns an OGC API Common Part 2-style
  `collections[]` document.
- Entries cover resource families semconnect already reads:
  `all_systems`, `all_procedures`, `all_deployments`,
  `all_sampling_features`, `all_properties`, and `all_datastreams`.
- Feature collections carry the CS API marker fields the spec and ETS
  look for (`itemType: "feature"` plus `featureType`, e.g.
  `sosa:System`). Property collections use `itemType:
  "sosa:Property"`.
- `items` links point at canonical endpoints such as
  `/systems?f=geojson`; Stage 47 later adds the narrow
  `/collections/all_system_events/items` facade for SystemEvent
  discovery only. No write-side aliases are added.
- No new Common Part 2 conformance class is claimed yet.

**Outcome:** `total=137 passed=79 failed=0 skipped=58` (confirmed
2026-05-21). `/collections` unblocked the Common Part 2 collections
assertion and the resource-specific collections cascade for already
implemented read-side resources. No write-side or SWE Common classes
were added.

### Stage 29 — semstreams pin bump to v1.0.0-beta.79 + retired closed shims

Stage 29 pins semconnect to semstreams `v1.0.0-beta.79` and adopts the
framework fixes that landed from our upstream issue queue:

- SensorML `uniqueId` now emits as `sensorml.process.uid`; the gateway
  no longer appends its own `cs-api.system.uid` triple on SensorML POST.
- SensorML `position` now emits as `sensorml.process.position`; the
  gateway no longer peeks raw SensorML bodies for a position workaround.
- Feature-shaped resources (systems, deployments, sampling features)
  use the same framework uid/position predicates. Read paths still
  recognize the old `cs-api.system.uid` and `cs-api.system.position`
  predicates during the migration window.
- Deployments use `sosa.SSNDeployment`.
- ControlStream and SystemEvent type / relationship triples use
  `vocabulary/csapi.ControlStream`, `vocabulary/csapi.SystemEvent`,
  `vocabulary/csapi.ControlsSystem`, and
  `vocabulary/csapi.EventForSystem`.

The write path still uses `graph.mutation.triple.add_batch` and the
existing delete fan-out; Stage 30 confirms semstreams#120 is closed, so
moving onto `graph.mutation.entity.*` is now local semconnect cleanup.
The read path now uses beta.87's header-classified request/reply error
surface via `natsclient.ClassifyReply`.

**Outcome:** `total=137 passed=79 failed=0 skipped=58` (confirmed
2026-05-27). Headline conformance is unchanged from Stage 28. The
harness now actively polls `/controlstreams` and `/systemEvents` after
seeding because their predicate-index visibility can lag the POST by a
few seconds under beta.79; the confirmed run needed five attempts for
`/controlstreams`.

### Stage 30 — semstreams pin bump to v1.0.0-beta.87 + upstream ask triage

Stage 30 pins semconnect to semstreams `v1.0.0-beta.87` and re-triages
the upstream ask queue for semstreams review.

Resolved upstream asks now covered by the pin:

- #100: the `-health-port` flag is wired to a dedicated `/health` and
  `/healthz` listener, disabled by default with `0`.
- #101: `nats.jetstream` limits are documented and validated against
  server-side `AccountInfo`; enforcement remains a nats-server config
  concern.
- #120: entity mutation responses now expose `Degraded` semantics so a
  gateway can distinguish committed-write/read-back-failed from an
  uncommitted mutation.
- #93 Phase 1+2+3: request/reply handler failures now carry
  `X-Status` / `X-Error-Class` headers while preserving the legacy
  `error: ...` body during the dual-encoding window.

Open semstreams asks that still matter for CS API coverage:

- #116: schema-bound SWE Common JSON/text/binary encodings for
  observations and commands. semconnect keeps the Stage 27
  observation-value subsets and does not claim SWE Common conformance.

The remaining write-path cleanup is local to semconnect: move POST,
PUT/PATCH, and DELETE off `graph.mutation.triple.add_batch` plus
delete fan-out and onto `graph.mutation.entity.*`. That work is no
longer an upstream blocker.

**Outcome:** `total=137 passed=79 failed=0 skipped=58` (confirmed
2026-05-29). Headline conformance is unchanged from Stage 29 on the
beta.87 backend pin. `/controlstreams` predicate-index readiness was
immediate in the confirmed run; `/systemEvents` needed five poll
attempts.

### Stage 31 — semstreams pin bump to v1.0.0-beta.88 + SWE unblock

Stage 31 pins semconnect to semstreams `v1.0.0-beta.88`, which ships
ADR-050 / #116: schema-bound SWE Common `DataRecord` values and
JSON/text/binary encoders and decoders in `pkg/swecommon`.

This closes the last semstreams-side upstream ask tracked by
`docs/upstream-asks`. Semconnect still intentionally returns
`X-CS-SWE-Subset: observation-values` on SWE observation responses until
a local gateway stage wires datastream result schemas and controlstream
command schemas through `pkg/swecommon`.

Remaining post-unblock local work as of Stage 31:

- Replace the hand-rolled observation SWE JSON/CSV/binary projection
  with schema-bound `pkg/swecommon` encoders.
- Bind datastream result schemas to observation values so the gateway
  can remove `X-CS-SWE-Subset`.
- Add command payload/schema parity for controlstreams.

**Outcome:** `total=137 passed=79 failed=0 skipped=58` (confirmed
2026-05-29). Headline conformance is unchanged from Stage 30 on the
beta.88 backend pin. `/controlstreams` predicate-index readiness needed
five poll attempts in the confirmed run; `/systemEvents` was ready on
the first attempt.

### Stage 32 — adopt semstreams SWE encoders on observation read path

Stage 32 replaces the hand-rolled SWE observation projection with
semstreams `pkg/swecommon` encoders:

- `application/swe+json` now emits SWE Common JSON rows from
  `swecommon.MarshalJSONRows`.
- `application/swe+csv` now emits SWE TextEncoding rows from
  `swecommon.MarshalTextRows`; the v0.1 response keeps the
  `time,result` header because the Datastream schema is not yet
  advertised separately.
- `application/swe+binary` now emits real SWE BinaryEncoding packed
  primitive rows from `swecommon.MarshalBinaryRows`, replacing the
  former comma-separated byte stream.
- The gateway infers a v0.1 `{time,result}` `DataRecord` from the OMS
  page. Numeric results map to `Quantity`, boolean results to
  `Boolean`, and mixed/object/string results degrade to `Text`.
- `X-CS-SWE-Subset: observation-values` remains intentionally present:
  the framework encoder is now in use, but the schema is still inferred
  per page rather than stored on and advertised by the Datastream
  resource.

Remaining local work:

- Bind Datastream result schemas to observation values so clients can
  discover the schema and the gateway can remove `X-CS-SWE-Subset`.
- Add command payload/schema parity for controlstreams.

**Outcome:** `total=137 passed=79 failed=0 skipped=58` (confirmed
2026-05-29). Headline conformance is unchanged from Stage 31; the ETS
still does not exercise the deferred SWE Common suites while semconnect
does not claim a SWE Common conformance class.

### Stage 33 — Datastream SWE result schemas

Stage 33 makes Datastream result schemas first-class in the gateway:

- `POST /datastreams` and `PUT /datastreams/{id}` accept an optional
  `schema` field containing a SWE Common `DataRecord` JSON schema.
- The schema is validated and canonicalized with semstreams
  `pkg/swecommon` before any graph mutation.
- The schema is stored on a gateway-local `cs-api.datastream.schema`
  predicate. Stage 42 later retires this bridge in favor of
  `csapi:SWESchemaDocument` artifact entities related by
  `csapi.HasResultSchema`.
- `GET /datastreams/{id}` returns the schema and a `rel=schema` link
  when one is stored.
- `GET /datastreams/{id}/schema` returns the stored `DataRecord`
  schema as JSON.
- SWE observation reads use the stored schema when present and omit
  `X-CS-SWE-Subset`. Legacy Datastreams without a schema keep the
  Stage 32 inferred `{time,result}` fallback and still carry
  `X-CS-SWE-Subset: observation-values`.

Remaining local work:

- Add command payload/schema parity for controlstreams.
- Replace the gateway-local Datastream schema predicate with a typed
  SWE schema artifact entity. Completed in Stage 42 after semstreams
  shipped the CS API vocabulary constants.

**Outcome:** `total=137 passed=79 failed=0 skipped=58` (confirmed
2026-05-29). Headline conformance is unchanged from Stage 32; schema
binding is a local capability and the pinned ETS still does not exercise
the deferred SWE Common suites while semconnect does not claim a SWE
Common conformance class.

### Stage 34 — ControlStream SWE command schema parity

Stage 34 makes ControlStream command schemas use the same semstreams
`pkg/swecommon` `DataRecord` contract as Datastream result schemas:

- `POST /controlstreams` validates `schema.parametersSchema` as a SWE
  Common `DataRecord` and canonicalizes it before storing the schema
  triple.
- `GET /controlstreams/{id}/schema` returns the canonical command
  schema when the stored schema is valid.
- `controlledProperties` are still derived from the schema fields when
  omitted, but now from a validated DataRecord.
- `ControlStream.formats` reflects the stored `commandFormat`.
- Command execution remains intentionally out of scope at v0.1; Stage 51
  later lets `/controlstreams/{id}/commands` return read-side Command
  metadata when graph evidence exists.

Remaining local work:

- Implement command execution only when the product scope asks for it;
  the schema/read side is now SWE-backed.
- Replace gateway-local command schema storage with a typed SWE schema
  artifact entity once semstreams ships the CS API vocabulary constants
  tracked in upstream #171.

**Outcome:** `total=137 passed=79 failed=0 skipped=58` (confirmed
2026-05-29). Headline conformance is unchanged from Stage 33; the
pinned ETS still does not exercise the deferred SWE Common suites while
semconnect does not claim a SWE Common conformance class.

### Stage 35 — Datastream PATCH parity

Stage 35 extends the `conf/update` surface from systems to datastreams:

- `PATCH /datastreams/{id}` accepts `application/json` Datastream-shaped
  partial bodies.
- Non-empty `name`, `description`, `system`, `observedProperty`, and
  `schema` fields replace the corresponding triples; absent/empty fields
  preserve existing state.
- Body `id` mismatch, invalid system refs, invalid SWE Common schemas,
  and `schema: null` fail before any destructive operation.
- Missing entities return 404 rather than upserting; PUT remains the
  create-or-replace path.
- `OPTIONS /datastreams/{id}` now advertises
  `GET, HEAD, PUT, PATCH, DELETE, OPTIONS`.

Remaining local work:

- Command execution only if v0.1 scope expands beyond read-side
  ControlStream metadata.

**Outcome:** `total=137 passed=79 failed=0 skipped=58` (confirmed
2026-05-29). Headline conformance is unchanged from Stage 34; the
pinned ETS already exercises the claimed update surface through systems,
and Datastream PATCH parity does not unlock a new ETS branch.

### Stage 36 — Datastream DELETE observation purge

Stage 36 closes the observation orphan note on Datastream DELETE:

- `DELETE /datastreams/{id}` still deletes the graph entity first and
  keeps the same idempotent 204 behavior when the graph entity is
  missing.
- After graph deletion succeeds, the gateway purges messages on the exact
  JetStream subject `cs-api.observations.{id}`.
- The purge is subject-scoped; it does not touch observations for any
  other datastream in the shared `cs-api.observations.>` stream.
- If graph deletion succeeds but purge fails, the handler returns 503
  with `X-CS-Partial-Delete: true` and
  `X-CS-Observation-Purge-Failed: true`. Retrying DELETE finishes the
  cleanup.

The implementation adds a narrow `streamCleaner` seam over
`jetstream.Stream.Purge(WithPurgeSubject(...))`, matching the existing
`streamReader` seam for observation reads.

Remaining local work:

- Command execution only if v0.1 scope expands beyond read-side
  ControlStream metadata.

**Outcome:** `total=137 passed=79 failed=0 skipped=58` (confirmed
2026-05-29). Headline conformance is unchanged from Stage 35; the
pinned ETS does not seed or assert observation cleanup.

### Stage 37 — Entity mutation write path

Stage 37 retires the legacy write-path shims now that semstreams'
entity mutation subjects are available in the pinned beta:

- POST resource creates use `graph.mutation.entity.create_with_triples`
  through `ingestTriples`. Duplicate creates now map to 409 Conflict
  instead of silently upserting through `add_batch`.
- PUT create-or-replace paths fetch the current entity, then use
  `graph.mutation.entity.update_with_triples` for replacements or
  `create_with_triples` when the entity is missing.
- PATCH paths keep their existing precondition checks, merge into a
  full desired triple set, then use `update_with_triples`.
- DELETE paths use `graph.mutation.entity.delete`, which is idempotent
  for missing entities.
- The old delete-all + add-batch partial-erasure window is gone from
  Systems and Datastreams. Datastream DELETE still reports
  `X-CS-Partial-Delete: true` when observation stream purge fails after
  graph deletion, because that split-brain state is still real.

Remaining local work:

- Command execution only if v0.1 scope expands beyond read-side
  ControlStream metadata.

**Outcome:** `total=137 passed=79 failed=0 skipped=58` (confirmed
2026-05-31). Headline conformance is unchanged from Stage 36; this
stage retires local write-path debt rather than unlocking a new ETS
branch.

### Stage 38 — semstreams pin bump to v1.0.0-beta.90 + upstream ask closure

Stage 38 pins semconnect to semstreams `v1.0.0-beta.90`, which closes
the three semstreams asks filed from the CS API graph/store fit pass:

- #171 ships `vocabulary/csapi` artifact classes and CS API IRI
  constants for `hasSource`, `hasResultSchema`, and
  `hasCommandSchema`. The accepted pattern is first-class artifact
  entities with their own singular `StorageRef`, related from parent CS
  API entities by vocabulary predicates. semconnect must still preserve
  SemStreams' internal three-level dotted predicate contract and map to
  CS API IRIs only at export/import boundaries; follow-up #182 tracks
  dotted CS API predicate constants for direct graph use.
- #172 exposes public `graph.query.batch`; semconnect adopts it for
  hydrated collection reads in Stage 40.
- #173 documents the existing `natsclient.TestClient` helper patterns
  for gateway-style integration tests.

No semstreams ask currently blocks semconnect. The remaining work after
this pin is local adoption:

- Migrate Datastream result schemas and ControlStream command schemas
  from gateway-local JSON predicates to `csapi:SWESchemaDocument`
  artifact entities with dotted relationship predicates. Completed in
  Stage 42.
- Use `natsclient.TestClient` when a real NATS-backed integration test
  is more valuable than the current in-memory fakes.

**Outcome:** `total=137 passed=79 failed=0 skipped=58` (confirmed
2026-05-31). Headline conformance is unchanged from Stage 37; the beta.90
pin closes upstream ask triage and unlocks local follow-up work rather
than claiming new ETS branches.

### Stage 39 — semstreams pin bump to v1.0.0-beta.91 + CS API dotted predicates

Stage 39 pins semconnect to semstreams `v1.0.0-beta.91`, which closes
#182. `vocabulary/csapi` relationship predicates are now split into the
two shapes semconnect needs:

- dotted internal graph predicate constants, such as
  `csapi.ProducedBy`, `csapi.ControlsSystem`, `csapi.EventForSystem`,
  `csapi.HasResultSchema`, and `csapi.HasCommandSchema`
- `*IRI` boundary constants for JSON-LD/RDF export, such as
  `csapi.ProducedByIRI` and `csapi.HasResultSchemaIRI`

Local cleanup in this stage:

- New writes for Datastream system links, ControlStream system links,
  and SystemEvent system links now use beta.91's dotted CS API predicate
  constants automatically.
- Because semconnect is greenfield, relationship reads use the same
  dotted beta.91 predicates directly rather than carrying beta.90
  compatibility fallbacks.
- The upstream ask queue now has no open semstreams blocker for the CS
  API gateway.

**Outcome:** `total=137 passed=79 failed=0 skipped=58` (confirmed
2026-05-31). Headline conformance is unchanged from Stage 38; the beta.91
pin repairs the CS API predicate contract and retires the upstream
dotted-predicate blocker rather than unlocking a new ETS branch.

### Stage 39+ — Continue OSH-bar resource buildout

Subsequent stages from the OSH-bar memory:

- Command execution, if/when v0.1 scope expands beyond read-side
  ControlStream metadata.

### Stage 40 — Batch entity hydration for collection reads

Stage 40 adopts semstreams #172's public `graph.query.batch` passthrough
for collection handlers that predicate-query IDs and then hydrate entity
state for response rendering:

- `/systems?f=geojson`
- `/procedures?f=geojson`
- `/deployments?f=geojson`
- `/samplingFeatures?f=geojson`
- `/controlstreams`
- `/systems/{id}/controlstreams`
- `/systemEvents`
- `/systems/{id}/events`

Hydration is chunked at 100 IDs per semstreams guidance to avoid oversized
NATS replies. Batch-level transport/classification failures still map to
gateway backend errors; partial-success replies that omit one entity keep
the existing collection behavior by skipping the full-resource item or
degrading a GeoJSON Feature to null geometry / minimal properties.

**Outcome:** `total=137 passed=79 failed=0 skipped=58` (confirmed
2026-06-02). Headline conformance is unchanged from Stage 39; this is a
read-path performance/shape cleanup that adopts the semstreams #172
primitive rather than unlocking a new ETS branch.

### Stage 41 — Schema artifact ObjectStore substrate

Stage 41 prepares the typed schema artifact migration unlocked by
semstreams beta.91:

- `Start()` now ensures a dedicated JetStream ObjectStore bucket for CS
  API schema artifacts (`schema_artifacts_bucket`, default
  `CS_API_ARTIFACTS`) alongside the observations stream.
- `schema_artifact_id_prefix` gives SWE schema artifact entities their
  own 5-part SemStreams namespace.
- A shared helper canonicalizes SWE Common schema JSON through
  `pkg/swecommon`, writes the bytes to ObjectStore, creates a typed
  `csapi:SWESchemaDocument` graph entity with `StorageRef`, and returns
  the parent relationship triple using the dotted beta.91 predicates
  (`csapi.HasResultSchema` or `csapi.HasCommandSchema`).

The Datastream and ControlStream HTTP handlers still read/write the
gateway-local JSON predicates in this stage. That keeps the migration
small enough to verify independently; the next local stage can switch
those call sites onto the artifact helper and retire the bridge
predicates.

**Outcome:** `total=137 passed=79 failed=0 skipped=58` (confirmed
2026-06-02). Headline conformance is unchanged from Stage 40; this stage
adds storage substrate for the schema artifact migration rather than
claiming a new ETS branch.

### Stage 42 — Schema artifact handler migration

Stage 42 retires the gateway-local schema JSON predicates:

- Datastream result schemas are stored as canonical SWE Common bytes in
  ObjectStore. The Datastream entity carries `csapi.HasResultSchema`
  pointing at the typed `csapi:SWESchemaDocument` artifact entity.
- ControlStream command `parametersSchema` follows the same artifact
  pattern through `csapi.HasCommandSchema`; the small command media
  format remains a scalar graph fact (`cs-api.controlstream.commandFormat`)
  so item/collection reads can advertise `formats` without fetching
  artifact content.
- `GET /datastreams/{id}`, `GET /datastreams/{id}/schema`,
  observation SWE encoders, and `GET /controlstreams/{id}/schema` follow
  the parent relationship to the artifact entity, then fetch bytes from
  ObjectStore via `StorageRef`.
- Schema writes use deterministic artifact IDs and upsert the artifact
  entity before attaching the parent relationship, so PUT/PATCH retries
  converge on the same artifact.

**Outcome:** `total=137 passed=79 failed=0 skipped=58` (confirmed
2026-06-02). Headline conformance is unchanged from Stage 41; this stage
retires local storage debt rather than claiming a new ETS branch.

### Stage 43 — Global Commands read collection

Stage 43 implements the optional canonical `GET /commands` endpoint as a
readable empty Command collection:

- `/commands` supports `GET`, `HEAD`, and `OPTIONS`.
- The JSON body mirrors `/controlstreams/{id}/commands` with `items: []`
  and a `self` link.
- `?limit=` is parsed and validated for collection consistency, but no
  Command lifecycle or execution semantics are introduced at v0.1.
- `gateway/cs-api/openapi.yaml` no longer carries any
  `x-not-implemented-at-v01` paths.
- Stage 51 later populates `/commands` from graph-backed
  `csapi.Command` metadata and adds the conformance fixture `POST`
  helper; execution remains out of scope.

**Outcome:** `total=137 passed=80 failed=0 skipped=57` (confirmed
2026-06-02). The ETS now passes the optional canonical Commands endpoint
check; Advanced Filtering command tests still SKIP because `/commands`
is empty. Stage 43 itself does not claim Advanced Filtering; Stage 48
later claims the Part 1 Systems filter slice only.

### Stage 44 — Part 2 Datastream read-side closure

Stage 44 declares the CS API Part 2 Datastreams/Observations conformance
class and wires the read-only surface the pinned ETS checks:

- `/datastreams` now batch-hydrates full Datastream resources in
  `items[]` instead of lightweight refs.
- Datastream JSON includes Part 2 members: `system@id`,
  `system@link`, `outputName`, `observedProperties`, `formats`, and
  `resultType`.
- `/datastreams/{id}/schema` wraps the stored SWE Common DataRecord as
  `{obsFormat,resultSchema}`.
- `/systems/{id}/datastreams` filters Datastreams by the stored
  `csapi.ProducedBy` relationship.
- `/observations` is a readable global Observation collection; Stage 45
  makes it populated through the existing JetStream observation stream.
- `conformance/run.sh` now seeds a schema-backed Datastream and actively
  polls `/datastreams` so Team Engine does not race the predicate index.

**Outcome:** `total=137 passed=89 failed=0 skipped=48` (confirmed
2026-06-02). The remaining Part 2 Datastream SKIPs are observation item
and reference checks that require populated global/canonical Observation
resources.

### Stage 45 — canonical Observation read resources

Stage 45 closes the populated Observation checks without introducing a
new graph index:

- `GET /observations` now reads the existing `cs-api.observations.>`
  JetStream stream and returns JSON Observation resources.
- `GET /observations/{obsID}` scans the same stream and returns the first
  matching canonical Observation item.
- JSON Observation resources add the Part 2-required `datastream@id`,
  recovered from the JetStream subject. The `application/om+json` nested
  read still returns the original OMS payload bytes.
- `conformance/run.sh` now posts one OMS observation after Datastream
  seed, then polls both `/observations` and
  `/datastreams/{id}/observations` before invoking Team Engine.

**Outcome:** `total=137 passed=91 failed=0 skipped=46` (confirmed
2026-06-02). The Part 2 Datastream block is now fully PASSING in the
pinned ETS. Remaining skips are outside the Datastream read-side slice
(for example richer relation-type mapping, command execution/status, and
Advanced Filtering).

### Stage 46 — GeoJSON association evidence

Stage 46 closes the GeoJSON relation-type and non-system mapping skips
that were waiting for concrete association evidence:

- System JSON items now expose allowlisted `links[]` association rels for
  `datastreams` and `controlstreams`, pointing at existing system-scoped
  resources.
- Procedure JSON items expose an allowlisted `implementingSystems` link.
- Deployment JSON items expose allowlisted `samplingFeatures` and
  `datastreams` links, and Deployment Feature POST/GET round-trips
  `properties.deployedSystems@link[]` hrefs through gateway-local dotted
  predicate `cs-api.deployment.deployedSystems`.
- SamplingFeature JSON items expose allowlisted `datastreams` and
  `controlstreams` links, and SamplingFeature Feature POST/GET
  round-trips `properties.hostedProcedure@link.href` through
  gateway-local dotted predicate `cs-api.samplingfeature.hostedProcedure`.
- `conformance/run.sh` now seeds those two property-level `@link`
  mappings using the concrete System and Procedure IDs minted earlier in
  the fixture sequence.

This stage does not claim new conformance classes and does not add
SensorML representations for Deployments, Procedures, or Properties. Stage
52 later adds Procedure item SensorML reads, and Stage 53 adds Deployment
and Property item SensorML reads.

**Outcome:** `total=137 passed=97 failed=0 skipped=40` (confirmed
2026-06-02). Newly passing checks are
`deploymentFeatureHasGeoJsonSchemaAndMapping`,
`samplingFeatureHasGeoJsonSchemaAndMapping`, and the four GeoJSON
links-member relation-type checks for system, deployment, procedure, and
samplingFeature.

### Stage 47 — Part 2 canonical read aliases

Stage 47 closes three read-side Part 2 skips that were about discovery
shape rather than new domain behavior:

- `conformance.go` now declares the explicit CS API `conf/system` URI.
  The existing System surface already satisfied the ETS prerequisite;
  the declaration was the missing signal.
- `GET /controls/{id}` is registered as the canonical CS API Part 2
  ControlStream item URL alias and routes to the existing
  `/controlstreams/{id}` handler.
- `GET /collections` still returns the Common Part 2 `collections[]`
  document, and now mirrors that array under `items[]` for the ETS Part
  2 resource-collection discovery shape.
- The discovery document includes `all_system_events` with
  `itemType: "SystemEvent"` and an `items` link to
  `/collections/all_system_events/items`.
- `GET /collections/all_system_events/items` is a narrow facade backed
  by the existing `GET /systemEvents` collection. Other
  `/collections/{id}/items` IDs still return 404 at v0.1.

This stage keeps command execution/status lifecycle out of scope and
does not introduce a graph index for observations or commands.

**Outcome:** `total=137 passed=100 failed=0 skipped=37` (confirmed
2026-06-02). Newly passing checks are
`controlStreamCanonicalUrlReadableWhenControlsPathAvailable`,
`systemEventCollectionsCheckedWhenAdvertised`, and
`systemEventPrerequisitesVisibleForFullClosure`.

### Stage 48 — Part 1 Advanced Filtering for Systems

Stage 48 declares the Part 1 Advanced Filtering conformance class and
implements the `/systems` filter slice exercised by the pinned ETS:

- `GET /systems?id=<id-list>` filters by System entity ID or preserved
  UID. The ID list parser accepts homogeneous local-ID lists or URI UID
  lists and rejects empty or mixed lists.
- `GET /systems?q=<keyword>` filters over hydrated System collection
  evidence: entity ID, preserved UID, label/name, description, and
  definition.
- `GET /systems?geom=POLYGON(...)` accepts WKT polygons and returns
  Systems whose stored GeoJSON Point position is inside the polygon.
- JSON `SystemCollection.items[]` now include hydrated `name` and
  `description` when available, so clients and the ETS can discover
  keyword evidence before applying `?q`.
- Filtered requests hydrate candidate entity state through
  `graph.query.batch`; no new graph index subject is introduced.

This is intentionally a Part 1 Systems slice, not the full Part 2
Advanced Filtering surface. Stage 54 later adds the Part 2 read-only
filter checks for Datastream, Observation, ControlStream, Command, and
SystemEvent collections.

**Outcome:** `total=137 passed=106 failed=0 skipped=31` (confirmed
2026-06-02). Newly passing checks are
`advancedFilteringConformanceDeclared`,
`advancedFilteringDependencyCascadeRuntime`,
`advancedFilteringIdListSchema`, `systemsFilterById`,
`systemsFilterByGeomSmoke`, and `systemsFilterByKeyword`.

### Stage 49 — Part 1 Subsystems read side

Stage 49 declares the Part 1 Subsystems conformance class and implements
the parent-scoped read surface the pinned ETS exercises:

- `POST /systems` Feature bodies now accept optional `properties.parent@id`
  or `properties.parent@link`. The gateway stores the relation on the
  child System using semstreams' `sensorml.PredIsHostedBy` predicate.
- `GET /systems/{id}/subsystems` validates the parent System, lists
  Systems via the existing predicate index, batch-hydrates entity state,
  and filters children whose `PredIsHostedBy` points to the parent.
- `GET /systems/{id}/subsystems/{subsystemID}` verifies the child is a
  System hosted by the parent, then serves the same item encodings as the
  canonical `/systems/{subsystemID}` resource. The JSON form carries both
  `rel=canonical` and `rel=parent` links.
- The conformance fixture seed now creates a child System linked to the
  primary seeded System and waits for `/systems/{id}/subsystems` to become
  non-empty before Team Engine starts.

This stage intentionally does not implement recursive subsystem search or
time-filtered subcollections; those are outside the pinned ETS assertions
and can follow once a client or newer suite asks for them.

**Outcome:** `total=137 passed=110 failed=0 skipped=27` (confirmed
2026-06-02). Newly passing checks are
`subsystemsCollectionReturns200`, `subsystemItemHasIdTypeLinks`,
`subsystemItemHasCanonicalLink`, and `subsystemHasParentSystemLink`.

### Stage 50 — Part 1 Subdeployments read side

Stage 50 declares the Part 1 Subdeployments conformance class and implements
the parent-scoped read surface the pinned ETS exercises:

- `POST /deployments` Feature bodies now accept optional
  `properties.parent@id` or `properties.parent@link`. The gateway stores
  the relation on the child Deployment using the three-part gateway-local
  dotted predicate `cs-api.deployment.parent`.
- `GET /deployments/{id}/subdeployments` validates the parent Deployment,
  lists Deployments via the existing predicate index, batch-hydrates entity
  state, and filters children whose `cs-api.deployment.parent` points to
  the parent.
- Collection items are normal Deployment resources with canonical
  `/deployments/{childID}` links; no nested item alias is required by the
  pinned ETS for this group.
- The conformance fixture seed now creates a child Deployment linked to the
  primary seeded Deployment and waits for `/deployments/{id}/subdeployments`
  to become non-empty before Team Engine starts.

`cs-api.deployment.parent` exists because semstreams currently has SOSA/SSN
deployment vocabulary for deployed systems, but no canonical CS API
deployment-composition predicate. The shim is one constant plus the
Deployment write/read call sites; once semstreams adds a canonical term, the
replacement should be a small predicate swap rather than a structural
rewrite.

**Outcome:** `total=137 passed=114 failed=0 skipped=23` (confirmed
2026-06-02). Newly passing checks are
`subdeploymentsDependencyCascadeRuntime`,
`subdeploymentsCollectionReturns200`, `subdeploymentItemHasIdTypeLinks`,
and `subdeploymentItemHasCanonicalLink`.

### Stage 51 — ControlStream command reference evidence

Stage 51 closes the remaining ControlStream command-reference skip without
expanding into command execution:

- `GET /commands` now lists graph-backed `csapi.Command` entities instead
  of always returning an empty collection.
- `GET /controlstreams/{id}/commands` validates the parent ControlStream,
  lists Command entities through the existing predicate index,
  batch-hydrates entity state, and filters Commands whose semstreams
  `csapi.PartOfControlStream` predicate points at the selected
  ControlStream.
- `GET /commands/{id}` serves the canonical read-only Command metadata
  item so collection links resolve to a real resource.
- `POST /commands` is a JSON fixture helper for the conformance harness.
  It writes Command metadata triples only: required `controlstream@id`
  plus optional issue/execution time, status, sender, and params.
- The conformance seed now creates one Command metadata resource for the
  seeded ControlStream and waits for the nested command collection to
  become non-empty before Team Engine starts.

This stage uses semstreams' framework vocabulary (`csapi.Command` and
`csapi.PartOfControlStream`), so no gateway-local predicate shim is added.
Command execution, device interaction, asynchronous status transitions, and
actuation side effects remain out of v0.1 scope.

**Outcome:** `total=137 passed=115 failed=0 skipped=22` (confirmed
2026-06-02). Newly passing check:
`commandsReferenceSelectedControlStreamWhenNestedCollectionPopulated`.

### Stage 52 — Procedure SensorML read side

Stage 52 closes the Procedure SensorML read-side gap without adding new graph
vocabulary:

- `GET /procedures/{id}` now negotiates `application/sml+json` and
  `application/sensorml+json`, reusing the existing semstreams
  SimpleProcess/AggregateProcess reverse mapper for `sosa.Procedure`.
- Procedure JSON item links advertise the SensorML alternate representation.
- Procedure Feature POST now preserves `properties.definition` on
  `sensorml.PredDefinition`, giving SensorML read-back non-identity mapping
  evidence.
- System and Procedure SensorML responses append the same CS API `links[]`
  association evidence their JSON resources already expose. This is a
  representation-layer addition: no new graph predicate or storage shim.
- The conformance seed adds a concrete Procedure definition so the ETS can
  verify the mapping instead of honestly skipping an identity-only fixture.

**Outcome:** `total=137 passed=118 failed=0 skipped=19` (confirmed
2026-06-02). Newly passing checks:
`procedureSensorMlHasSchemaAndMapping`,
`sensorMlProcedureLinksMemberAssociationRelsUseResourceSpecificNames`, and
`sensorMlLinksMemberAssociationRelsUseResourceSpecificNames`.

### Stage 53 — Deployment and Property SensorML read side

Stage 53 closes the remaining non-process SensorML read-side skips without
adding graph vocabulary or storage:

- `GET /deployments/{id}` now negotiates `application/sml+json` and
  `application/sensorml+json`, emitting a Deployment-shaped SensorML JSON
  representation from the existing Deployment triples.
- Deployment SensorML includes `deployedSystems[]` from the stored
  `properties.deployedSystems@link` evidence plus the same links-member
  association rels already exposed on the JSON resource.
- `GET /properties/{id}` now negotiates both SensorML media types, emitting
  the existing DerivedProperty subset (`uniqueId`, label, description,
  definition, and optional baseProperty).
- Deployment and Property JSON item links advertise the SensorML alternate
  representation.

This is intentionally representation-layer work. Deployment and Property
POSTs keep their existing request media-type contracts, and no gateway-local
predicate is added for this stage.

**Outcome:** `total=137 passed=121 failed=0 skipped=16` (confirmed
2026-06-02). Newly passing checks:
`deploymentSensorMlHasSchemaAndMapping`,
`propertySensorMlHasSchemaAndMapping`, and
`sensorMlDeploymentLinksMemberAssociationRelsUseResourceSpecificNames`.
The remaining skips are Part 2 feasibility checks and Part 2 advanced
filtering checks. Stage 54 later closes the Part 2 advanced-filtering
block; Stage 55 later closes the Feasibility block.

### Stage 54 — Part 2 Advanced Filtering read side

Stage 54 declares CS API Part 2 `conf/advanced-filtering` and implements
the read-only filter slice exercised by the pinned ETS:

- `GET /datastreams?observedProperty=...` filters over
  `observedProperties[].definition`. `observedProperties` now emits the
  Part 2 object shape while preserving the scalar `observedProperty`
  breadcrumb.
- `GET /datastreams?phenomenonTime=...` and `?resultTime=...` filter
  over optional Datastream time evidence stored as small scalar triples.
- `GET /observations?phenomenonTime=...` and `?resultTime=...` filter
  normalized JSON Observation resources after BaseMessage unwrap; no graph
  observation index is introduced.
- `GET /controlstreams?controlledProperty=...`, `?issueTime=...`, and
  `?executionTime=...` filter over hydrated ControlStream resources.
- `GET /commands?issueTime=...`, `?executionTime=...`, `?statusCode=...`,
  and `?sender=...` filter read-side Command metadata. `currentStatus`
  mirrors the stored status code for Part 2 clients.
- `GET /systemEvents?eventType=...` filters hydrated SystemEvent
  resources.

The implementation deliberately keeps filtering at the HTTP resource layer:
predicate-query finds candidate entities, batch hydration builds the public
resource shape, then the filter is applied against that shape. The only new
stored predicates are small gateway-local scalar time values for Datastream
and ControlStream fixtures (`cs-api.datastream.*`,
`cs-api.controlstream.*`), using the existing three-part dotted convention.

At Stage 54, Part 2 Command Feasibility remained deferred as the only
skipped family; it required the explicit product decision to model
Feasibility resources rather than treating feasibility as another read filter
pass. Stage 55 closes that block.

**Outcome:** `total=137 passed=130 failed=0 skipped=7` (confirmed
2026-06-02). Newly passing checks:
`part2AdvancedFilteringConformanceDeclared`,
`advancedFilteringPrerequisitesVisibleForFullClosure`,
`datastreamTimeFiltersVerifyReturnedPredicates`,
`datastreamObservedPropertyFilterVerifiesReturnedPredicates`,
`observationTimeFiltersVerifyReturnedPredicates`,
`controlStreamTimeFiltersVerifyReturnedPredicates`,
`controlStreamControlledPropertyFilterVerifiesReturnedPredicates`,
`commandFiltersVerifyReturnedPredicatesWhenEndpointAvailable`, and
`systemEventTypeFilterVerifiesReturnedPredicatesWhenEndpointAvailable`.

### Stage 55 — Part 2 Command Feasibility read side

Stage 55 declares CS API Part 2 `conf/feasibility` and adds the read-side
Feasibility surface exercised by the pinned ETS:

- `GET /feasibility` lists Feasibility resources from graph state.
- `GET /feasibility/{id}` returns the canonical Feasibility resource with
  `status`, `params` / `parameters`, optional `result`, and ControlStream
  reference evidence.
- `GET /feasibility/{id}/status` and `/result` return readable `items[]`
  collections for status and result metadata.
- `GET /controlstream/{id}/feasibility` implements the normative singular
  ControlStream-scoped Feasibility path and filters resources by
  `controlstream@id`.
- `GET /collections/all_feasibility/items` backs the advertised
  `itemType=Feasibility` collection metadata.
- `POST /feasibility` is a fixture helper for the conformance harness only.
  It records metadata and does not evaluate feasibility or execute commands.

semstreams does not yet expose canonical CS API Feasibility vocabulary terms,
so Stage 55 uses a single gateway-local Feasibility type IRI plus
three-part dotted predicates:

- `cs-api.feasibility.controlstream`
- `cs-api.feasibility.status`
- `cs-api.feasibility.params`
- `cs-api.feasibility.result`

This should be retired when semstreams grows canonical Feasibility vocabulary
terms; the gateway-local predicates are isolated behind constants in
`gateway/cs-api/feasibility.go`.

**Outcome:** `total=137 passed=137 failed=0 skipped=0` (confirmed
2026-06-02). Newly passing checks:
`feasibilityConformanceDeclared`,
`feasibilityControlStreamPrerequisiteVisibleForFullClosure`,
`controlStreamScopedFeasibilityEndpointUsesNormativeSingularPath`,
`feasibilityCanonicalResourceReadableWhenAvailable`,
`feasibilityStatusEndpointReadableWhenResourceAvailable`,
`feasibilityResultEndpointReadableWhenResourceAvailable`, and
`feasibilityCollectionsCheckedWhenAdvertised`.

Also pending: HTML + Part 3 (`websocket`, `mqtt`) if product scope
expands in that direction.

The sponsor has confirmed Botts CS API ETS as the conformance target
through v1.0. Each pin bump (`conformance/.ets-pin: ETS_COMMIT`)
surfaces new assertion failures; triage is per-bump work. Track the
TestNG delta in the bump PR description so the reviewer sees what
conformance picture moved. ADR-S001 §4 documents the pin policy;
`conformance/README.md` documents the procedure.

Future CS API surface expansion should continue the established pattern:
inline schema + handler + tests; update `gateway/cs-api/openapi.yaml`;
verify conformance delta.

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
