# ADR-S001 — CS API server scope

- **Status**: Proposed (2026-05-15)
- **Repo**: `semconnect`
- **Companion**: [ADR-044 (semstreams)](https://github.com/C360Studio/semstreams/blob/main/docs/adr/044-ogc-connected-systems-framework-split.md) — framework/sister split
- **Framework pin**: `github.com/c360studio/semstreams v1.0.0-beta.73`

## Context

ADR-044 settled the split between framework-shaped primitives (`semstreams`) and deployment-shaped concerns (this repo). Phase 7 — the sister-repo launch — is the work this ADR scopes. The framework half (Phases 2–6) is shipped: SOSA/SWE/OMS vocabularies, GeoJSON + spatial extension, HTTP polling input, SensorML parser + Graphable bridge, OMS Observation payload. The `framework-primitives reference` in semstreams `docs/operations/21-adr044-framework-primitives-reference.md` is the authoritative inventory.

This ADR records what `semconnect` does at v0.1 — which CS API v1.0 conformance classes we claim, how content negotiation works, what auth looks like, where the conformance test runner lives, and how we'll handle the still-draft CS API Part 3 (pub/sub).

The playbook in `docs/000-getting-started.md` requires this ADR before any Go code beyond `go.mod`. The decisions below are proposed defaults — each is independently revisitable.

## Decisions

### 1. Conformance classes claimed at v0.1

The v0.1 *release* claims the minimum end-to-end-meaningful set:

- **Core** (CS API Part 1 §6) — required floor.
- **JSON** — `application/json` for every resource.
- **GeoJSON** — `application/geo+json` for spatial resources (`Feature`, `FeatureCollection`, `Geometry`).
- **SensorML** — `application/sensorml+json` for `System` / `Procedure` resources.
- **OMS** — `application/om+json` for `Observation` resources.
- **JSON-LD** — `application/ld+json` for every resource, emitted via `vocabulary/export`.

**A class is only declared from the running `/conformance` endpoint once its
encoder is wired.** Claiming a class Team Engine cannot validate against
fails conformance. The per-stage wiring schedule:

| Stage | Endpoint added | Classes that move from claimed-roadmap to wired |
|---|---|---|
| 2 | `GET /systems`, `/conformance`, `/health` | core, json |
| 3 | `POST /datastreams/{id}/observations` | + oms (consume) |
| 4 | `GET /systems/{id}` | + sensorml, + json-ld |
| 5 | `GET /areas` | + geojson |

Stages 2 + 3 + 4 + 5 are merged. The runtime `/conformance` declaration
now aligns with this ADR's v0.1 claim; Stage 6 wires the OGC Team Engine
conformance harness in CI to validate each claim.

**Stage 4 reconstruction is lossy by design.** Triples emitted via
`sensorml.Asset.Triples()` drop SensorML fields that the SOSA/SSN vocabulary
does not carry (inputs/outputs/parameters, keywords, connections,
identifier metadata beyond Value). `GET /systems/{id}` documents this via
the `X-CS-Reconstructed-Lossy: true` response header so Team Engine
assertions can account for it. Lossless round-trip requires fetching the
original SensorML JSON from `EntityState.StorageRef` (deferred to a
follow-up tag — graph-ingest's storage seam exists but is not wired
through cs-api yet).

**Stage 5 GeoJSON Features carry `geometry: null`** because the framework's
`SpatialResult` only returns entity IDs, not their indexed points. RFC 7946
§3.2 permits null geometry, but a client that wants precise coordinates
must drill via `GET /systems/{id}`. The `X-CS-Geometry-Available: false`
response header signals this. A follow-up upstream change to extend
`SpatialResult` with lat/lon/alt would close this gap; track as a Stage 6+
refinement if Team Engine flags it.

**Deferred to v0.2+**:

- Collections (CS API Part 1 §13) — landed once we have >1 datastream-shaped resource flowing.
- Sampling features, Properties (CS API Part 1 §12, §14) — wait for a real consumer.
- HTML encoding — out of scope (no SSR pipeline planned).
- XML / GML — explicitly out of scope.
- CS API Part 3 (pub/sub binding) — see §5.

**Rationale**: Core + the four JSON-shape conformance classes is the smallest set that lets the OGC Team Engine run a meaningful pass on this server. Each deferred class is additive — claiming it later is a tag bump, not a breaking change.

### 2. Content negotiation policy

`Accept` header drives response encoding. Server-side decision table:

| Resource family | Default (no Accept) | Negotiable via Accept |
|---|---|---|
| `System` (collection + item) | `application/json` | `application/sensorml+json`, `application/ld+json` |
| `Observation` (collection + item) | `application/json` | `application/om+json`, `application/ld+json` |
| Spatial result (`/areas`, `Feature*`) | `application/geo+json` | `application/json`, `application/ld+json` |
| `/conformance`, `/`, `/api` | `application/json` | `application/ld+json` |

- Unsupported `Accept`: respond `406 Not Acceptable` with a JSON body listing supported types.
- `q=` quality values honored only for tiebreaking inside the supported set.
- Wildcard `*/*` resolves to the row's default.
- No XML, no HTML, no GML.

**Rationale**: A small explicit matrix is auditable. The OGC Team Engine drives `Accept` on every assertion; ambiguous behavior here is the cheapest place to fail conformance.

### 3. Auth posture

**At v0.1: anonymous, reverse-proxy-mediated.** The server itself does no auth. Deployments terminate TLS and enforce identity at a reverse proxy (nginx / Envoy / Cloud Run / API Gateway) — the same shape semstreams' generic HTTP gateway README recommends (`gateway/http/README.md` §Security).

**Server-side hooks reserved but not implemented**: middleware can read `Authorization: Bearer <jwt>` and `X-Forwarded-User` headers; both are passed through to handlers and into the `BaseMessage.Headers` envelope on publishes for audit, but no token verification happens at v0.1.

**Deferred to a later ADR**: in-process JWT verification (OIDC discovery, JWKS rotation, per-scope authorization). When a deployment needs single-tenant auth without a proxy fronting it, file ADR-S00X covering JWKS source, scope→endpoint mapping, and token introspection.

**Rationale**: Every CS API operator at v0.1 will be standing up either a private deployment (no auth needed) or a public deployment behind a proxy that already handles identity for their org. Building in-process JWT validation before either pattern surfaces is YAGNI. The pass-through hooks keep audit trails honest from day one without committing to a verification strategy.

**Delivery semantics**: Observation publishes are **at-least-once**. `js.PublishMsg` can time out waiting for the JetStream ack after the server-side commit succeeded; the gateway treats `nats.ErrTimeout` as transient and returns 503, leaving retry policy to the client. A retried POST will write the observation again under a new envelope ID. Downstream consumers (graph-ingest etc.) must therefore be idempotent on observation ingest — the framework's `payloadregistry` round-trip discipline supports this because BaseMessages carry deterministic content-addressable IDs. Promoting to exactly-once is a Stage 6+ decision tied to deduplication in graph-ingest.

### 4. Conformance-test ownership

**Vendor a thin CI runner**, not the Team Engine binary.

- `conformance/` holds: a CI script that pulls the latest OGC Team Engine container on each run, a `fixtures/` directory of CS-API-specific inputs (minimal `System` doc, a 10-row `Observation` feed, a small polygon area), and a `run.sh` that boots `cs-api-server` + a local NATS + the Team Engine container against it.
- We do **not** vendor `teamengine` source or its WAR; CI fetches a tagged container.
- Pinned Team Engine version lives in `conformance/.teamengine-version`. Bump explicitly.

**Rationale**: Vendoring Team Engine source drags ~80 MB of Java into the repo for no gain — the OGC project ships container images. Pinning the image keeps CI reproducible; bumping the pin is an intentional change because it can surface new failures.

### 5. CS API Part 3 (pub/sub binding) stance

**Defer the binding decision; design the publish path to be binding-neutral.**

CS API Part 3 is still draft. The three live options are native NATS (best fit with semstreams substrate), MQTT proxy (most consumer SDKs), and WebSocket (most browser-friendly). v0.1 does **not** expose any pub/sub endpoint.

What we *do* do at v0.1:

- Every observation `POST` publishes a `message.BaseMessage` to JetStream subject `cs-api.observations.{datastream_id}`.
- Every spatial / system mutation publishes to a `cs-api.mutations.*` subject (final shape TBD per binding choice).
- These subjects are stable; any Part 3 binding implementation sits as a subscriber on the same streams.

**Leaning**: native NATS first (cheapest given the substrate; semstreams operators already speak NATS). MQTT proxy second when a non-NATS consumer surfaces. WebSocket third — likely a Part 4 binding, not Part 3.

**Trigger to revisit**: Part 3 leaves draft, OR a CS API consumer asks for pub/sub. Whichever comes first.

### 6. Binary shape — single vs. modular

**Single binary at v0.1: `cmd/cs-api-server/`.**

The framework's component model already supports running a subset of components in one process — if we later need an observation-ingester-only deployment or a spatial-query-only deployment, that's a configuration choice (`config.json` selecting components), not a separate binary. Splitting the binary up front is premature.

**Rationale**: Operators deploy one container at v0.1. The component-config split inside the binary is the right boundary if a deployment ever wants to scale endpoints independently.

### 7. API versioning

**Unprefixed paths at v0.1**: `/systems`, `/datastreams/{id}/observations`, `/areas`, `/conformance`.

OGC's own versioning is loose — CS API documents don't dictate a URL prefix. Major-version bumps (v1 → v2) will introduce `/v2/...` paths and keep the v1 surface as long as a consumer needs it. Minor versions (v1.0 → v1.1) are backward-compatible by spec; no prefix change.

**Rationale**: A `/v1/` prefix telegraphs an upgrade path nobody is asking for yet, and CS API's own examples are unprefixed. We'll add the prefix the moment we ship a v2 surface; until then it's noise.

### 8. Graph backend

**Fixed to semstreams-NATS at v0.1.**

The framework abstracts via Go interfaces, so making this pluggable later is a refactor, not a redesign. Building plug-in points before any second backend exists is YAGNI.

### 9. OMS deferred-feature posture

When a CS API consumer asks for an OMS feature semstreams hasn't shipped (typed results, `ResultQuality`, intervals, `Parameter`, `ValidTime`, `RelatedObservation`) — **file an issue upstream first**. Sister-side workarounds only land if upstream declines or schedules past our consumer's deadline. The framework-primitives reference §Scope-cut is the authoritative list of what's deferred and why.

This applies symmetrically to SensorML (`Mode`, `Algorithm`, `Configuration`, `DeployedSystem`), SWE 3.0, and HTTP-input SSE — see the same §Scope-cut.

## Consequences

**Enables:**

- A small, conformant v0.1 surface that the OGC Team Engine can validate end-to-end.
- Reusing every framework primitive without forking or shimming.
- Operators standing up the server behind their existing identity layer with no in-server config.

**Defers (acceptable):**

- Typed observation results (Quantity / Category / TimeSeries) — first OMS conformance asks may need these; track upstream.
- `ResultQuality` — flagged as most-likely first ask by framework reviewer; same.
- Cursor pagination for system listings — `graph-gateway` README documents the limitation; CS API has `next` / `prev` link relations to design around eventually.
- In-process auth — fine until a single-tenant operator without a proxy surfaces.

**Closes**

**Code-side: closed.** Stages 1–5 of the bootstrap playbook are merged. All v0.1 endpoint surface (`GET /systems`, `GET /systems/{id}`, `POST /datastreams/{id}/observations`, `GET /areas`, `GET /conformance`, `GET /health`) is wired against a local semstreams NATS, and the runtime `/conformance` declaration matches §1's full v0.1 class list (core + json + oms + sensorml + json-ld + geojson).

**Validation-side: pending Stage 6.** This ADR closes formally when the OGC Team Engine conformance harness reports green against each declared class. Documented limitations (`X-CS-Reconstructed-Lossy` on SensorML reconstruction; `X-CS-Geometry-Available: false` on `/areas` Features) may surface as Team Engine assertion failures; address per the deferred-features list above as they arise.

## References

- [ADR-044 — framework / sister split](https://github.com/C360Studio/semstreams/blob/main/docs/adr/044-ogc-connected-systems-framework-split.md)
- [framework-primitives reference](https://github.com/C360Studio/semstreams/blob/main/docs/operations/21-adr044-framework-primitives-reference.md)
- [CS API Part 1 (23-001)](https://docs.ogc.org/DRAFTS/23-001r0.html)
- [CS API Part 2 (23-002)](https://docs.ogc.org/DRAFTS/23-002r0.html)
- [OGC Team Engine](https://github.com/opengeospatial/teamengine)
- `docs/000-getting-started.md` — bootstrap playbook
