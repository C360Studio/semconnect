# AGENTS.md

This file provides guidance to Codex (Codex.ai/code) when working with code in this repository.

## Repository status

**Stages 2 + 3 + 4 + 5 + 7 + 8 + 9 + 10 + 11 + 12 + 13 + 14 + 15 + 16 + 17 + 18 + 19 + 20 + 21 + 22 + 23 + 24 + 25 + 26 + 27 + 28 + 29 + 30 + 31 + 32 + 33 + 34 + 35 + 36 of the bootstrap playbook are landed; Stage 6 conformance harness is wired.** What works:

- `cmd/cs-api-server/` — reference binary, builds and runs.
- `gateway/cs-api/` — `Component` implementing `component.Discoverable + LifecycleComponent + gateway.Gateway`.
- Endpoints:
  - `GET /` — OGC API Common Part 1 §7.2 landing page (Stage 7). JSON with `self`, `conformance`, `service-desc` (`/api`), `data` links, resource-specific `systems` / `datastreams` links, and Stage 25 Part 2 discovery links for `controlstreams` and `systemevents`. Uses Go 1.22 `GET /{$}` end-of-path anchor so it doesn't shadow sibling routes.
  - `GET /collections` (Stage 28) — OGC API Common Part 2 collection metadata. Static discovery document with `collections[]` entries for the resource families semconnect already reads (`all_systems`, `all_procedures`, `all_deployments`, `all_sampling_features`, `all_properties`, `all_datastreams`). `items` links point at the canonical CS API endpoints (for example `/systems?f=geojson`) rather than a `/collections/{id}/items` facade at v0.1. No new Common Part 2 conformance class is claimed.
  - `GET /systems` — lists `ssn:System` entities via NATS `graph.index.query.predicate`. Default `application/json` returns CS API SystemCollection wrapper; Stage 15 added `application/geo+json` content negotiation that returns an RFC 7946 FeatureCollection with per-system geometry recovered from the `sensorml.process.position` triple (legacy read fallback: `cs-api.system.position`; N+1: entity-query per item, per-entity failures degrade to null-geometry Features). Still no SensorML "SystemCollection" wrapper — that 406s honestly.
  - `POST /systems` (Stage 8; Stage 14 added position preservation; Stage 16 added JSON Feature body; Stage 18 added uid preservation; Stage 29 moved uid/position to semstreams beta.79 native predicates; Stage 37 moved creates to entity mutations) — accepts four request media types: `application/sml+json` / `application/sensorml+json` (SensorML path; semstreams emits `uniqueId` and `position` triples natively) and `application/json` / `application/geo+json` (CS API §7.6 GeoJSON Feature body — Stage 16; entity ID minted from `properties.uid`, the uid is preserved on `sensorml.process.uid`, optional `geometry` round-trips via `sensorml.process.position`). `Content-Type` selects the branch (`buildSystemTriplesFromSensorML` vs `buildSystemTriplesFromFeature`); both feed the shared `ingestTriples` path, which now calls `graph.mutation.entity.create_with_triples`. Returns 201 Created with Location; duplicate entity creates map to 409 Conflict.
  - `PUT /systems/{id}` (Stage 16; Stage 37 moved replacement to entity mutations) — CS API §7.6 create-replace-delete replace semantics. Accepts `application/json` / `application/geo+json` only (no SensorML on PUT — the lossy reverse-mapping would surprise clients on read-back). Verifies the body's `properties.uid` mints to the same entity ID as the path; mismatch yields 400 before any backend mutation. Existing entities are replaced via `graph.mutation.entity.update_with_triples`; missing entities are created via `graph.mutation.entity.create_with_triples`. Returns 204 No Content.
  - `DELETE /systems/{id}` (Stage 16; Stage 37 moved deletion to entity mutations) — CS API §7.6. Idempotent: a delete against a non-existent ID still returns 204. Deletes via `graph.mutation.entity.delete`.
  - `PATCH /systems/{id}` (Stage 19; Stage 37 moved replacement to entity mutations) — CS API `conf/update` partial-update. Body is the same `SystemFeature` shape PUT accepts (JSON or geo+json), but only `type: "Feature"` is enforced when present and all properties fields are optional. Implementation reads existing entity, merges body fields onto its triple set via `mergePatchSystemTriples`, then replaces via `graph.mutation.entity.update_with_triples`. Body-uid-vs-existing-uid safety gate runs before mutation. PATCH against non-existent entity returns 404 (no upsert — PUT is the upsert path). **No JSON Merge Patch null-as-delete** at v0.1 (ETS doesn't exercise it).
  - `OPTIONS /systems` + `OPTIONS /systems/{id}` (Stage 16; Stage 19 added PATCH to item Allow) — advertise `Allow` headers so the ETS confirms write-side readiness without exercising the verbs. Collection: `GET, HEAD, POST, OPTIONS`. Item: `GET, HEAD, PUT, PATCH, DELETE, OPTIONS`.
  - `GET /systems/{id}` — fetches an entity via `graph.query.entity`, renders as `application/json` (CS API §7.2 subset; `geometry` from `sensorml.process.position` with legacy fallback to `cs-api.system.position`; `uid` + `uniqueId` + `properties.uid` from `sensorml.process.uid` with legacy fallback to `cs-api.system.uid`), `application/sml+json` or `application/sensorml+json` (via triple→sensorml reverse mapping in `gateway/cs-api/sensorml.go`; surfaces `uniqueId` on the reconstructed Process), or `application/ld+json` (via `vocabulary/export.Serialize(JSONLD)`). Lossy reconstruction is signalled via `X-CS-Reconstructed-Lossy: true`. Both SensorML media types are honored on Accept (Stage 14: `sml+json` is the CS API spec form; long form kept as backward-compat alias). **Breaking field rename at Stage 18:** the JSON System subset's `properties` field was renamed to `characteristics` (it always carried SensorML characteristics, not GeoJSON-shape properties — the rename frees the `properties` JSON key for the Feature-shape container).
  - `GET /datastreams` (Stage 8) — predicate-query for `rdf:type = DatastreamTypeIRI` (Stage 13: `csapi.Datastream` from `vocabulary/csapi` since semstreams v1.0.0-beta.75; pre-Stage-13 was a locally-minted HTTPS IRI). JSON DatastreamCollection. `X-CS-Datastream-Subset: true` header retired at Stage 13.
  - `GET /datastreams/{id}` (Stage 8; Stage 33 adds schema field/link) — entity-query → CS API §10 Datastream JSON subset (id, name, description, system-ref, observedProperty, optional SWE Common DataRecord schema). 404 if entity exists but is not a Datastream.
  - `GET /datastreams/{id}/schema` (Stage 33) — returns the stored SWE Common DataRecord JSON schema for observation value encodings. Stored under gateway-local `cs-api.datastream.schema`. Semstreams beta.91 closed #182 with dotted CS API relationship predicates plus `*IRI` boundary constants; migrate schema storage to typed artifact entities when semconnect adopts `csapi.HasResultSchema` / `csapi.HasCommandSchema`.
  - `POST /datastreams` (Stage 8; Stage 33 adds optional schema) — accepts `application/json` (CS API §10 Datastream shape), validates `system` strictly (6-part SemStreams shape) + `observedProperty` (non-empty IRI), validates/canonicalizes optional `schema` with `pkg/swecommon`, mints/honors 6-part entity ID, publishes via the same `ingestTriples` path POST /systems uses. Federation idiom: a client-supplied 6-part `id` is honored verbatim; otherwise minted from `cfg.DatastreamIDPrefix`.
  - `PUT /datastreams/{id}` (Stage 17; Stage 33 validates optional schema; Stage 37 moved replacement to entity mutations) — CS API §10.6 create-replace-delete replace-or-upsert semantics. Accepts `application/json` only. Re-validates required `system` + `observedProperty` fields (same as POST). Body `id` (if present) must match path; mismatch yields 400 before any backend mutation. Existing entities are replaced via `graph.mutation.entity.update_with_triples`; missing entities are created via `graph.mutation.entity.create_with_triples`. Returns 204 No Content.
  - `PATCH /datastreams/{id}` (Stage 35; Stage 37 moved replacement to entity mutations) — CS API `conf/update` partial-update parity with `/systems/{id}`. Accepts `application/json` only. Non-empty `name`, `description`, `system`, `observedProperty`, or `schema` fields replace the corresponding triples; absent/empty fields leave existing triples untouched. Body `id` mismatch, invalid system refs, invalid SWE Common schema, and `schema: null` fail before backend mutation. PATCH against a missing entity returns 404 (no upsert — PUT remains the upsert path). Uses `graph.mutation.entity.update_with_triples`.
  - `DELETE /datastreams/{id}` (Stage 17; Stage 36 purges observation subject; Stage 37 moved graph deletion to entity mutations) — CS API §10.6. Idempotent. Deletes the graph entity via `graph.mutation.entity.delete`, then purges messages on the exact JetStream subject `cs-api.observations.{id}`. If graph deletion succeeds but stream purge fails, returns 503 with `X-CS-Partial-Delete: true` and `X-CS-Observation-Purge-Failed: true` so retrying DELETE can finish cleanup.
  - `OPTIONS /datastreams` + `OPTIONS /datastreams/{id}` (Stage 17; Stage 35 adds PATCH to item Allow) — collection: `GET, HEAD, POST, OPTIONS`. Item: `GET, HEAD, PUT, PATCH, DELETE, OPTIONS`.
  - `POST /datastreams/{datastreamID}/observations` — accepts `application/om+json`, wraps in `message.BaseMessage`, publishes to JetStream subject `cs-api.observations.{datastreamID}` with audit + W3C trace headers.
  - `GET /datastreams/{datastreamID}/observations` (Stage 11; Stage 27 added SWE value encodings; Stage 32 routes them through semstreams `pkg/swecommon`; Stage 33 uses stored Datastream schemas) — reads back via the same JetStream stream the POST writes to. Spins a one-shot ordered consumer filtered on `cs-api.observations.{datastreamID}`, fetches up to `?limit=N` messages with `FetchNoWait` (so an empty stream returns immediately rather than burning the QueryTimeout budget), unwraps each `BaseMessage` to its inner OMS payload, returns CS API §11.3 `ObservationCollection` for `application/json`, a bare JSON array of OMS observations for `application/om+json`, or SWE Common observation-value rows for `application/swe+json`, `application/swe+csv`, and `application/swe+binary`. Schema-backed Datastreams omit `X-CS-SWE-Subset`; legacy Datastreams without schema fall back to inferred `{time,result}` and carry `X-CS-SWE-Subset: observation-values`. Paging via opaque `?after=<stream-seq>` cursor; when the page fills and a sequence was seen, a `next` link is added on the JSON wrapper (`truncated` is a heuristic — proper "remaining count" needs `consumer.Info().NumPending`, deferred follow-up; failure modes documented in `observations_get.go`). Malformed envelopes are skipped (logged) rather than 500-ing the whole request. Structured access log line on success carries the resolved `Identity` forwarded-user/email for read-side audit, mirroring the publish path's NATS-header audit. New `streamReader` interface on `Component` (production: `jetstreamObservationReader` wrapping `OrderedConsumer + FetchNoWait`; tests: fake).
  - `GET /areas` — spatial filtering via `?bbox=minLon,minLat,maxLon,maxLat` or `?polygon=<GeoJSON Polygon>` (exactly one required). Optional `?limit`. Returns a GeoJSON `FeatureCollection`; Features carry real Point geometry (Stage 13: framework v1.0.0-beta.75 added Lat/Lon/Alt echo to `SpatialResult`). `X-CS-Geometry-Available: false` header retired at Stage 13.
  - `GET /conformance` — declares the full v0.1 set: Common Part 1 core + json + **oas30** (Stage 12), CS API core + json + oms + sensorml + json-ld + geojson + **create-replace-delete** (Stage 16/17) + **update** (Stage 19) + **procedure** (Stage 20) + **deployment** (Stage 21) + **sampling feature** (Stage 22) + **property** (Stage 23) + Part 2 **api-common** + **controlstream** (Stage 24) + **system-event** (Stage 25). Stages 20+25 begin closing the OSH-bar resource-type gap (sponsor 2026-05-17 set OSH compliance as the new bar; OSH declares 34 classes).
  - `GET /procedures` (Stage 20) — CS API §6 collection. Predicate-query for `rdf:type = sosa.Procedure`. JSON-only ProcedureCollection.
  - `GET /procedures/{id}` (Stage 20) — JSON Procedure subset. NO `geometry` field per `/req/procedure/location` (procedures are methods, not physical things). Same `X-CS-Reconstructed-Lossy: true` header as /systems/{id}.
  - `POST /procedures` (Stage 20) — accepts the same four media types POST /systems does (sml+json / sensorml+json / json / geo+json). NO position triple appended (procedures forbid location). rdf:type triple object is OVERRIDDEN to `sosa.Procedure` on the SensorML path so a PhysicalSystem mistakenly POSTed to /procedures still lands under the Procedure class for predicate-query collection.
  - `OPTIONS /procedures` + `OPTIONS /procedures/{id}` (Stage 20) — `GET, HEAD, POST, OPTIONS` on collection; `GET, HEAD, OPTIONS` on item. PUT/DELETE/PATCH intentionally absent at v0.1 (ETS CRD/update test groups only target /systems).
  - `GET /deployments` (Stage 21) — CS API §8 collection. Predicate-query on `rdf:type = sosa.SSNDeployment`. JSON `DeploymentCollection` (default) or `application/geo+json` FeatureCollection with per-deployment geometry recovered from the shared `sensorml.process.position` triple (N+1 entity-query, same pattern as Stage 15 /systems).
  - `GET /deployments/{id}` (Stage 21) — JSON Deployment subset; includes geometry field from the position triple when present.
  - `POST /deployments` (Stage 21) — accepts `application/json` / `application/geo+json` Feature body only. NO SensorML — no spec encoding pairs SensorML with Deployment.
  - `OPTIONS /deployments` + `OPTIONS /deployments/{id}` (Stage 21) — same shape as /procedures: collection accepts POST, item is read-only.
  - `GET /samplingFeatures` (Stage 22) — CS API sampling-features collection. Predicate-query on `rdf:type = sosa:Sample`; JSON `SamplingFeatureCollection` (default) or `application/geo+json` FeatureCollection with per-feature geometry recovered from the shared `sensorml.process.position` triple. GeoJSON Feature `properties` include `featureType`, `uid`, `name`, and `description` when the entity state carries those triples.
  - `GET /samplingFeatures/{id}` (Stage 22) — JSON SamplingFeature subset with uid/uniqueId/properties.uid and geometry when present.
  - `POST /samplingFeatures` (Stage 22) — accepts `application/json` / `application/geo+json` Feature body only. Entity ID minted from `properties.uid`; optional geometry round-trips via the shared position triple.
  - `OPTIONS /samplingFeatures` + `OPTIONS /samplingFeatures/{id}` (Stage 22) — same shape as /deployments: collection accepts POST, item is read-only.
  - `GET /properties` (Stage 23) — CS API properties collection. Predicate-query on `rdf:type = sosa:ObservableProperty`; JSON `PropertyCollection`.
  - `GET /properties/{id}` (Stage 23) — JSON Property subset with uid/uniqueId, label, description, definition, and optional baseProperty.
  - `POST /properties` (Stage 23) — accepts `application/sml+json`, `application/sensorml+json`, or `application/json` SensorML DerivedProperty-shaped JSON. Entity ID minted from `uniqueId` (or `uid` alias); representable subset lands as triples.
  - `OPTIONS /properties` + `OPTIONS /properties/{id}` (Stage 23) — same shape as /procedures: collection accepts POST, item is read-only.
  - `GET /controlstreams` (Stage 24) — CS API Part 2 control-stream collection. Predicate-query on `vocabulary/csapi.ControlStream`, then N+1 entity hydration so collection `items` are full ControlStream resources.
  - `GET /controlstreams/{id}` (Stage 24; Stage 34 makes schema SWE-backed) — JSON ControlStream subset with system reference, inputName, controlledProperties, formats, live/async flags, and command links.
  - `GET /controlstreams/{id}/schema` (Stage 24; Stage 34 validates/canonicalizes schema) — returns the stored command schema (`commandFormat`, `parametersSchema` as SWE Common DataRecord).
  - `GET /controlstreams/{id}/commands` (Stage 24) — readable empty Command collection; command execution is intentionally out of scope at v0.1.
  - `GET /systems/{id}/controlstreams` (Stage 24) — system-scoped ControlStream collection, filtered by beta.91's dotted `vocabulary/csapi.ControlsSystem` (legacy read fallbacks: beta.75-beta.90 IRI value and `cs-api.controlstream.system`).
  - `POST /controlstreams` (Stage 24; Stage 34 validates command schema with `pkg/swecommon`) — JSON fixture helper used by the conformance harness to create read-side ControlStreams; not a command execution path.
  - `GET /systemEvents` (Stage 25) — CS API Part 2 SystemEvent collection. Predicate-query on `vocabulary/csapi.SystemEvent`, then N+1 entity hydration so collection `items` are full SystemEvent resources.
  - `GET /systemEvents/{id}` (Stage 25) — JSON SystemEvent subset with time/eventTime, eventType, message, system reference, source/severity, optional payload, and links.
  - `GET /systems/{id}/events` (Stage 25) — normative Requirement 43 system-scoped SystemEvent collection, filtered by beta.91's dotted `vocabulary/csapi.EventForSystem` (legacy read fallbacks: beta.75-beta.90 IRI value and `cs-api.systemevent.system`).
  - `GET /systems/{id}/events/{eventID}` (Stage 25) — system-scoped SystemEvent item alias; 404 if the event is not associated with the path system.
  - `POST /systemEvents` + `POST /systems/{id}/events` (Stage 25) — JSON fixture helpers used by the conformance harness to create read-side SystemEvents; not a streaming/SSE event delivery path.
  - `GET /systems/{id}/history` (Stage 26) — OSH-compatible System History vendor extension. Returns a `SystemCollection` containing the current System description as revision `current`; sets `X-CS-History-Current-Only: true`. No conformance class is claimed because the pinned ETS documents `/conf/system-history` as non-OGC/vendor-only.
  - `GET /systems/{id}/history/current` (Stage 26) — current historical description alias for `/systems/{id}` JSON. Unknown revision IDs 404 before a backend lookup.
  - `GET /api` (Stage 12) — serves the OAS3 service definition embedded at `gateway/cs-api/openapi.yaml`. Default `application/vnd.oai.openapi+json;version=3.0` (boot-time YAML→JSON conversion via `gopkg.in/yaml.v3` + encoding/json indent), alt `application/vnd.oai.openapi;version=3.0` returns the raw embedded YAML. `?f=yaml` and `?f=openapi` short names per Common Part 1 §7. The OAS is hand-authored to reflect cs-api's actual v0.1 behavior — honest `X-CS-*` response headers as spec contract elements, `x-not-implemented-at-v01: true` extension on paths vendored from OGC for roadmap visibility but deferred to follow-up stages (`/commands`). The vendored OGC source-of-truth lives at `api/upstream/` at pinned commit (`api/.oas-pin`, `api/upstream/README.md`).
  - `GET /health`.
  - All read endpoints accept `HEAD`. Routes use Go 1.22+ method+path patterns (`GET /systems` / `HEAD /systems`); 405 is enforced by the mux.
- Auth seam: `IdentityMiddleware` populates `Identity` in every request context. Anonymous-by-default; `X-Forwarded-User` / `X-Forwarded-Email` from a trusted reverse proxy flow onto every publish as `X-CS-Forwarded-*` NATS headers for audit. No verification at v0.1.
- Content negotiation via `Accept` AND the OGC Common Part 1 `?f=<short>` query-parameter override (Stage 7) — `NegotiateRequest` honors both. Short names: `json`, `geojson`, `sensorml`, `om`, `jsonld`. An explicit `?f=` that doesn't map to the family's supported set 406s rather than silently falling through to Accept — the override is a deliberate client signal. Per-family supported sets live in `negotiation.go`. JSON for everything; SensorML + JSON-LD for `GET /systems/{id}` only; collection `GET /systems` honestly 406s on non-JSON Accept (no SensorML "SystemCollection" type).
- Body-size limit middleware (`MaxRequestBytes`) enforces `413` on POSTs.
- JetStream: `cs-api.observations.>` stream is EnsureStream'd at component Start() with 30-day file retention. A failure to provision the stream surfaces as a `Start()` error, not a 503-orphan.
- Error classification: `errEntityNotFound` sentinel → 404; `pkg/errs.IsInvalid / IsTransient` → 400 / 503; raw `nats.ErrNoResponders` / `nats.ErrTimeout` / `context.DeadlineExceeded` / `nats.ErrConnectionClosed` wrapped to Transient at the boundary on both Request and PublishMsg paths. Unclassified → 500 with a generic body (full error logged).
- **`classifyEntityQueryFailure`** handles the one CS API mapping that remains above semstreams beta.87's `natsclient.ClassifyReply`: entity-query `"not found: ..."` inside the Invalid class becomes the local `errEntityNotFound` sentinel so HTTP returns 404 instead of 400. Other handler errors now flow through `X-Status` / `X-Error-Class`.
- **`ingestTriples`** (Stage 8; Stage 37 migrated to entity mutation subjects) is the shared create helper for POST resource fixtures. Publishes via `graph.mutation.entity.create_with_triples` request/reply on the `QueryTimeout` budget (NOT `PublishTimeout` — request/reply lives on the read budget, not the fire-and-forget budget). Duplicate creates map to the local `errEntityConflict` sentinel → 409; invalid graph mutation requests map to 400; transport-layer errors (ErrNoResponders/timeout) map to 503. The `X-CS-Attempted-ID` response header on error paths echoes the minted entity ID so clients can correlate without parsing a Location header that wasn't set. PUT/PATCH replacement paths use `graph.mutation.entity.update_with_triples`; DELETE uses `graph.mutation.entity.delete`.
- **Landing page** (Stage 12 update): hrefs are now ABSOLUTE (was: root-relative) — built by `absoluteBase(r)` from `X-Forwarded-Proto`/`X-Forwarded-Host` (reverse-proxy case) or `r.TLS`/`r.Host` (direct). REST Assured-shaped clients (the Botts ETS) don't auto-resolve relative URIs against the document's own URL, instead bare-fetching them. Stage 12 also added `rel=service-desc` (`/api`), `rel=systems` (`/systems`), `rel=datastreams` (`/datastreams`) link entries alongside the existing `rel=data` ones to satisfy CS API §7.6 / Common §7.4.1.
- **Predicate index lookup** (Stage 12 fix): `gateway/cs-api/systems.go` constant renamed `predicateRDFType` → `predicateClassType = sensorml.PredType`. The framework's sensorml emitter writes type triples under the predicate name `sensorml.process.type`, not `rdf.type`; cs-api-server's POST/GET paths both use `sensorml.PredType` so they agree. The old constant was misnamed AND wrong-valued — surfaced by `systemsCollectionHasItemsArray` ETS assertion when the upstream-ETS core cascade unblocked. Hidden since Stage 2 because we never had data in the graph during a probe.
- **Stage 6 + 9 conformance harness** (`conformance/run.sh` + `.github/workflows/conformance.yml`) boots NATS + `semstreams-backend` (Stage 9) + cs-api-server + OGC Team Engine with the [Botts CS API ETS](https://github.com/Botts-Innovative-Research/ets-ogcapi-connectedsystems10) via docker compose, seeds CS-API fixtures (Stage 9: POST /systems + POST /datastreams via `curlimages/curl` on the compose network; Stages 20-25 add procedure, deployment, sampling-feature, property, controlstream, and system-event fixtures), hits TE's REST API, and archives a TestNG XML report plus per-container logs (on success and on failure — `on_exit` captures before teardown). Both the ETS and the framework are pinned by commit SHA in `conformance/.ets-pin`; bumping each is intentional. NATS JetStream limits are pinned in `conformance/nats.conf` (10GB file / 1GB memory) because nats-server 2.10's CLI doesn't expose `max_file_store`; semstreams beta.81+ now validates/warns against the connected account's actual limits, while the harness still owns server-side enforcement. The harness runs on push to `main`, on `workflow_dispatch`, and on PRs labelled `conformance` — **not a PR-blocking gate** at this stage. **Current outcome (Stage 39, 2026-05-31): `total=137 passed=79 failed=0 skipped=58`.** Zero failures against our claimed conformance set. Stage 28's `/collections` metadata unblocked the Common Part 2 collections assertion and the resource-specific collections cascade for already-implemented read-side resources without adding write-side or SWE Common claims; Stage 29's beta.79 pin left the headline unchanged; Stage 30's beta.87 pin leaves the headline unchanged while closing upstream ask triage for #93/#100/#101/#120; Stage 31's beta.88 pin closes #116 on the semstreams side and leaves the headline unchanged; Stage 32 routes observation SWE responses through `pkg/swecommon`; Stage 33 adds Datastream schema binding; Stage 34 validates ControlStream command schemas with `pkg/swecommon`; Stage 35 adds Datastream PATCH parity; Stage 36 purges per-datastream observation subjects on DELETE; Stage 37 migrates gateway writes to entity mutation subjects; Stage 38 pins beta.90 and closes #171/#172/#173 triage while filing #182 for dotted CS API predicate constants; Stage 39 pins beta.91, closes #182, and moves CS API relationship writes to dotted predicates with IRI legacy read fallbacks; the headline remains unchanged because no new ETS branch is unlocked. The remaining SKIPs are tests gated on conformance classes / resources we haven't claimed at v0.1 (Part 2 write side, Advanced Filtering, sub-resource item GETs: subsystems / commands, plus intentionally unimplemented encodings) or by the ETS dependency chain. Trajectory: Stage 12 (20/0/117) → Stage 14 (29/1/107) → Stage 15 (32/0/105) → Stage 16+17+conformance-fix (38/2/97) → Stage 18 (40/0/97) → Stage 22 (58/0/79) → Stage 23 (62/0/75) → Stage 24 (62/0/75) → Stage 25 (62/0/75) → Stage 26 (62/0/75) → Stage 27 (62/0/75) → Stage 28 (79/0/58) → Stage 29 (79/0/58) → Stage 30 (79/0/58) → Stage 31 (79/0/58) → Stage 32 (79/0/58) → Stage 33 (79/0/58) → Stage 34 (79/0/58) → Stage 35 (79/0/58) → Stage 36 (79/0/58) → Stage 37 (79/0/58) → Stage 38 (79/0/58) → Stage 39 (79/0/58). Eventual-consistency seed-then-query lag is handled by `run.sh` poll-until-visible checks after seed (systems, controlstreams, systemEvents); TeamEngine host readiness is actively polled because Tomcat can briefly reset connections after Docker starts the container.
- **`Dockerfile`** (repo root) — multi-stage build of cs-api-server into a distroless/static-debian12 image. Used by the conformance harness and eventual operator deploys.

**Read order** for orientation:

1. `README.md` — what `semconnect` is and is not.
2. `docs/adr/001-cs-api-server-scope.md` — ADR-S001, the scope decisions in force.
3. `docs/000-getting-started.md` — the bootstrap playbook (stages 0–6).
4. [ADR-044](https://github.com/C360Studio/semstreams/blob/main/docs/adr/044-ogc-connected-systems-framework-split.md) — the framework / sister-repo split this repo implements.
5. The [framework-primitives reference](https://github.com/C360Studio/semstreams/blob/main/docs/operations/21-adr044-framework-primitives-reference.md) — authoritative list of what `semstreams` provides.

## What this repository is

`semconnect` is the **HTTP gateway** half of ADR-044. It exposes [OGC API Connected Systems v1.0](https://docs.ogc.org/DRAFTS/23-001r0.html) (CS API) as a RESTful surface over the `semstreams` framework primitives. It is **not** a re-implementation of those primitives — SOSA/SWE/OMS/SensorML/GeoJSON all come from `github.com/c360studio/semstreams` as a Go module dependency.

Concretely:

- **In scope here**: HTTP routing, content negotiation, auth/TLS, CS API conformance, reference deploy binary (`cmd/cs-api-server/`), OGC Team Engine conformance harness, example operator deployments.
- **Out of scope here**: anything framework-shaped. If a SOSA/SWE/OMS/SensorML primitive is missing, file an issue upstream on `semstreams` — do not fork the encoder into this repo.

## Architecture overview

Request flow once bootstrapped:

```
HTTP request
  → gateway/cs-api/<endpoint>.go (routing, content negotiation, auth)
    → semstreams primitives:
        graph-query    (entity reads → CS API resources)
        graph-ingest   (POST bodies → NATS publishes wrapped in message.BaseMessage)
        parser/sensorml (SensorML JSON ↔ Graphable entities)
        message/oms    (OMS Observation JSON ↔ BaseMessage)
        vocabulary/*   (SOSA/SWE/OMS/SSN IRIs for JSON-LD responses)
        graph.spatial.query.{bounds,polygon} (spatial query subjects)
  → JSON / JSON-LD / GeoJSON / SensorML+JSON / OM+JSON response
```

The deployment substrate underneath is NATS (JetStream + KV) — the framework's facts-vs-requests model is the wire layer. Every NATS publish, **including from gateway handlers**, must wrap in `message.BaseMessage` (see Discipline notes below).

## Endpoint → primitive mapping

| Endpoint | Framework primitive |
|---|---|
| `GET /collections` | Static OGC API Common Part 2 metadata over the canonical CS API resource endpoints. No semstreams call; `items` links target `/systems?f=geojson`, `/procedures?f=geojson`, `/deployments?f=geojson`, `/samplingFeatures?f=geojson`, `/properties`, and `/datastreams`. Stage 28. |
| `GET /systems` | `graph.index.query.predicate` (rdf:type = ssn:System) → JSON SystemCollection (default) OR `geojson.FeatureCollection` with per-system geometry (Stage 15, on Accept `application/geo+json`; N+1 entity-query per item). |
| `GET /systems/{id}` | `graph.query.entity` → `EntityState` → `reconstructProcessFromTriples` (JSON / SensorML) or `export.Serialize(JSONLD)`. Lossy fields documented via `X-CS-Reconstructed-Lossy: true`. |
| `POST /systems` | `parser/sensorml.UnmarshalProcess` (sml+json) **or** `buildSystemTriplesFromFeature` (json / geo+json, Stage 16) → `graph.mutation.entity.create_with_triples` via `ingestTriples`. 201 Created + Location; duplicate ID → 409. |
| `PUT /systems/{id}` | `buildSystemTriplesFromFeature` → existing entity `graph.mutation.entity.update_with_triples` OR missing entity `create_with_triples`. 204 No Content. Stage 16; entity mutation migration Stage 37. |
| `DELETE /systems/{id}` | `graph.mutation.entity.delete`. 204 No Content (idempotent). Stage 16; entity mutation migration Stage 37. |
| `PATCH /systems/{id}` | `mergePatchSystemTriples` over existing entity state → `graph.mutation.entity.update_with_triples`. 204 No Content. 404 if entity doesn't exist (no upsert). Stage 19; entity mutation migration Stage 37. |
| `OPTIONS /systems` / `OPTIONS /systems/{id}` | Static `Allow` header advertisement. 204 No Content. Item Allow includes PATCH from Stage 19. Stage 16. |
| `GET /procedures` | `graph.index.query.predicate` (rdf:type = sosa.Procedure) → JSON ProcedureCollection. Stage 20. |
| `GET /procedures/{id}` | `graph.query.entity` → `EntityState` → `procedureFromState` (CS API §6 JSON subset; no geometry per /req/procedure/location). Stage 20. |
| `POST /procedures` | `buildProcedureTriplesFromSensorML` (sml+json) **or** `buildProcedureTriplesFromFeature` (json / geo+json) → `ingestTriples`. rdf:type override to `sosa.Procedure` on the SensorML path. No position triple. 201 Created + Location. Stage 20. |
| `OPTIONS /procedures` / `OPTIONS /procedures/{id}` | Static `Allow` header. 204 No Content. Stage 20. |
| `GET /deployments` | `graph.index.query.predicate` (rdf:type = `sosa.SSNDeployment`) → JSON DeploymentCollection (default) OR `geojson.FeatureCollection` with per-deployment geometry from `sensorml.process.position` (N+1, on Accept `application/geo+json`). Stage 21. |
| `GET /deployments/{id}` | `graph.query.entity` → `deploymentFromState` (JSON subset with geometry from position triple). Stage 21. |
| `POST /deployments` | `buildDeploymentTriplesFromFeature` (json / geo+json) → `ingestTriples`. Optional geometry → `sensorml.process.position` triple. 201 Created + Location. Stage 21. |
| `OPTIONS /deployments` / `OPTIONS /deployments/{id}` | Static `Allow` header. 204 No Content. Stage 21. |
| `GET /samplingFeatures` | `graph.index.query.predicate` (rdf:type = sosa:Sample) → JSON SamplingFeatureCollection (default) OR GeoJSON FeatureCollection with per-feature geometry from `sensorml.process.position` and hydrated `uid` / `name` / `description` Feature properties. Stage 22. |
| `GET /samplingFeatures/{id}` | `graph.query.entity` → `samplingFeatureFromState` (JSON subset with geometry from position triple). Stage 22. |
| `POST /samplingFeatures` | `buildSamplingFeatureTriplesFromFeature` (json / geo+json) → `ingestTriples`. Optional geometry → `sensorml.process.position` triple. 201 Created + Location. Stage 22. |
| `OPTIONS /samplingFeatures` / `OPTIONS /samplingFeatures/{id}` | Static `Allow` header. 204 No Content. Stage 22. |
| `GET /properties` | `graph.index.query.predicate` (rdf:type = sosa:ObservableProperty) → JSON PropertyCollection. Stage 23. |
| `GET /properties/{id}` | `graph.query.entity` → `propertyFromState` (JSON subset with uid/label/description/definition/baseProperty). Stage 23. |
| `POST /properties` | `buildPropertyTriples` (sml+json / sensorml+json / json DerivedProperty subset) → `ingestTriples`. 201 Created + Location. Stage 23. |
| `OPTIONS /properties` / `OPTIONS /properties/{id}` | Static `Allow` header. 204 No Content. Stage 23. |
| `GET /controlstreams` | `graph.index.query.predicate` (rdf:type = `vocabulary/csapi.ControlStream`) → N+1 `graph.query.entity` hydration → JSON ControlStreamCollection. Stage 24. |
| `GET /controlstreams/{id}` | `graph.query.entity` → `controlStreamFromState` (JSON subset with schema/commands links). Stage 24. |
| `GET /controlstreams/{id}/schema` | `graph.query.entity` → stored command schema with SWE Common DataRecord `parametersSchema`. Stage 34 validates/canonicalizes new schemas with `pkg/swecommon`. |
| `GET /controlstreams/{id}/commands` | `graph.query.entity` kind check → empty Command collection. Stage 24. |
| `GET /systems/{id}/controlstreams` | Predicate-query all ControlStreams, hydrate, filter by dotted `vocabulary/csapi.ControlsSystem` with legacy IRI/local fallback. Stage 24; beta.91 dotted predicate migration Stage 39. |
| `GET /systemEvents` | `graph.index.query.predicate` (rdf:type = `vocabulary/csapi.SystemEvent`) → N+1 `graph.query.entity` hydration → JSON SystemEventCollection. Stage 25. |
| `GET /systemEvents/{id}` | `graph.query.entity` → `systemEventFromState` (JSON subset with system reference and event metadata). Stage 25. |
| `GET /systems/{id}/events` | Predicate-query all SystemEvents, hydrate, filter by dotted `vocabulary/csapi.EventForSystem` with legacy IRI/local fallback. Stage 25; beta.91 dotted predicate migration Stage 39. |
| `GET /systems/{id}/events/{eventID}` | `graph.query.entity` → kind check + system-reference check → JSON SystemEvent. Stage 25. |
| `GET /systems/{id}/history` | `graph.query.entity` → kind check → current-only JSON SystemCollection with `/history/current` link. Stage 26. |
| `GET /systems/{id}/history/{revID}` | `revID=current` → `graph.query.entity` → `systemFromState`; unknown revisions 404. Stage 26. |
| `GET /datastreams` | `graph.index.query.predicate` (rdf:type = `csapi.Datastream` since Stage 13) → JSON DatastreamCollection. |
| `GET /datastreams/{id}` | `graph.query.entity` → `EntityState` → `datastreamFromState` (CS API §10 JSON subset). 404 if not a Datastream kind. |
| `GET /datastreams/{id}/schema` | `graph.query.entity` → gateway-local `cs-api.datastream.schema` triple → SWE Common DataRecord JSON. Stage 33. |
| `POST /datastreams` | JSON decode → optional `pkg/swecommon` schema validation → `datastreamToTriples` → `graph.mutation.entity.create_with_triples` via `ingestTriples`. 201 Created + Location. Honors client-supplied 6-part `id`; duplicate ID → 409. |
| `PUT /datastreams/{id}` | JSON decode → `datastreamToTriples` → existing entity `graph.mutation.entity.update_with_triples` OR missing entity `create_with_triples`. 204 No Content. Stage 17; entity mutation migration Stage 37. |
| `PATCH /datastreams/{id}` | JSON decode → optional `pkg/swecommon` schema validation → merge non-empty fields over existing triples → `graph.mutation.entity.update_with_triples`. 204 No Content; 404 if missing (no upsert). Stage 35; entity mutation migration Stage 37. |
| `DELETE /datastreams/{id}` | `graph.mutation.entity.delete` → JetStream `Purge(WithPurgeSubject("cs-api.observations.{id}"))`. 204 No Content (idempotent). Stage 17; observation purge added Stage 36; entity mutation migration Stage 37. |
| `OPTIONS /datastreams` / `OPTIONS /datastreams/{id}` | Static `Allow` header advertisement. 204 No Content. Stage 17; item Allow includes PATCH from Stage 35. |
| `POST /datastreams/{id}/observations` | `message/oms` decode → `message.NewBaseMessage` → `js.PublishMsg` on `cs-api.observations.{id}` with `X-CS-*` audit headers + W3C trace context (raw `js.PublishMsg`, not `natsclient.PublishToStream`, so we can attach headers — trace is re-injected via `natsclient.InjectTrace` to match framework convention) |
| `GET /datastreams/{id}/observations` (`application/swe+json` / `application/swe+csv` / `application/swe+binary`) | JetStream read → BaseMessage unwrap → OMS payload → stored Datastream `swecommon.DataRecord` when present, otherwise inferred `{time,result}` fallback → `pkg/swecommon` JSON/Text/Binary encoders. Stage 33 removes `X-CS-SWE-Subset` on schema-backed Datastreams. |
| `GET /areas?bbox=` / `?polygon=` | `graph.spatial.query.bounds` / `.polygon` → bare `[]SpatialResult` (now with Lat/Lon/Alt since Stage 13) → `geojson.FeatureCollection` with real Point geometry. |

The triple → SensorML reverse mapping (`gateway/cs-api/sensorml.go`) is intentionally lossy: inputs/outputs/parameters, keywords, connections, and identifier metadata beyond `Value` are dropped because `sensorml.Asset.Triples()` doesn't emit them. The reconstruction emits skeleton refs for hosted children (`{id: "child-id", type: "PhysicalComponent"}`) rather than recursively hydrating them — clients drill via `GET /systems/{childID}`.

## Bootstrap order (do not skip stages)

From `docs/000-getting-started.md`:

- **Stage 0** — File `docs/adr/001-cs-api-server-scope.md` (ADR-S001) **before** any Go code. Decisions to land: conformance classes claimed at v0.1, content negotiation policy, auth posture, conformance-test ownership, CS API Part 3 (pub/sub) stance.
- **Stage 1** — `go mod init github.com/c360studio/semconnect` + `go get github.com/c360studio/semstreams@latest`. Pin to a tag, not a branch.
- **Stage 2** — First endpoint: `GET /systems`. Smallest end-to-end path; proves the whole config → NATS → query → response chain.
- **Stage 3** — `POST /datastreams/{id}/observations`. First mutation; first real use of `message/oms` + `message.BaseMessage`.
- **Stage 4** — `GET /systems/{id}` with SensorML round-trip.
- **Stage 5** — Spatial queries (`/areas`).
- **Stage 6** — Wire OGC Team Engine into CI.

## Commands

Standard Go toolchain. Go 1.26.3 required (auto-selected via `toolchain` directive — `semstreams` requires it).

```bash
go build ./...                          # build everything
go build -o /tmp/cs-api-server ./cmd/cs-api-server
go test ./...                           # full suite (no integration tags yet)
go test -race ./...                     # required before any commit
go test -run TestHandleSystems ./gateway/cs-api    # single test
go vet ./...
```

No `Taskfile` or `Makefile`. The conformance harness is invoked directly via `conformance/run.sh`.

Running the binary needs a NATS server reachable at `nats://localhost:4222` (configurable via `--config`):

```bash
/tmp/cs-api-server                                  # default config
/tmp/cs-api-server -config ./cs-api.json            # JSON config file
```

A config-less run binds `:8080` and connects to local NATS. With nothing on either, it fails fast with a clear NATS-connect error — by design.

Conformance harness (Stage 6):

```bash
./conformance/run.sh                # full run; cold ~6-8 min (ETS Maven build), warm ~1-2 min
./conformance/run.sh --teardown-only

# Override host ports if 4222 / 8081 / 8222 are busy locally:
TE_HOST_PORT=8181 NATS_HOST_PORT=14222 NATS_MON_HOST_PORT=18222 ./conformance/run.sh
```

Outputs land in `conformance/output/` (gitignored). `conformance/README.md` documents the calibration delta and the bump procedure for the pin file `conformance/.ets-pin`.

## Discipline notes (inherited from semstreams)

These are cross-cutting rules the framework side learned the hard way. Honor them from the first commit:

- **Every NATS publish wraps in `message.BaseMessage`** — even when the obvious consumer reads raw. Subjects are shared infrastructure; auditors and sister-of-sister-repos will subscribe.
- **Operator-reachable JSON seams need round-trip tests.** This caught wire drift in framework Phases 4/5/6. Any new gateway envelope (auth headers, conformance-class advertisement, error shapes) needs the same coverage.
- **Pre-tag sweep includes build tags.** Run `go vet -tags=integration` (and any other conditional-build tags) before tagging.
- **Never re-tag.** Go's module proxy pins on first fetch; a re-tag is a footgun.
- **E2E required for breaking changes** once v1.0 ships — conformance suite + smoke binary must run green on the breaking commit before the tag.

## When something feels missing from `semstreams`

The framework deferred several items (see the framework-primitives reference, "Scope-Cut" section). If CS API conformance asks for one of these — OMS typed results, `ResultQuality`, SensorML `Mode` / `Algorithm` / `Configuration`, SWE Common 3.0, CS API Part 3 pub/sub binding — **file an issue on `semstreams` first**. Do not work around it by reimplementing the primitive here.

## Open architectural questions (resolve in ADR-S001)

- Single binary vs. modular `cmd/cs-api-server/` (api-server + observation-ingester + spatial-query-frontend as sub-binaries). Monolithic is the default; the framework's component model lets us split later without API breakage.
- Pluggable graph backend vs. fixed to semstreams-NATS. Framework abstracts via interfaces, but the value proposition is with semstreams.
- API versioning (`/v1/systems` vs. unprefixed). OGC's own versioning is loose; pick a convention and stick.
