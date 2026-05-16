# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Repository status

**Stages 2 + 3 + 4 + 5 of the bootstrap playbook are landed; Stage 6 conformance harness is wired.** What works:

- `cmd/cs-api-server/` — reference binary, builds and runs.
- `gateway/cs-api/` — `Component` implementing `component.Discoverable + LifecycleComponent + gateway.Gateway`.
- Endpoints:
  - `GET /systems` — lists `ssn:System` entities via NATS `graph.index.query.predicate`. JSON only (collection has no SensorML wrapper).
  - `GET /systems/{id}` — fetches an entity via `graph.query.entity`, renders as `application/json` (CS API §7.2 subset), `application/sensorml+json` (via triple→sensorml reverse mapping in `gateway/cs-api/sensorml.go`), or `application/ld+json` (via `vocabulary/export.Serialize(JSONLD)`). Lossy reconstruction is signalled via `X-CS-Reconstructed-Lossy: true`.
  - `POST /datastreams/{datastreamID}/observations` — accepts `application/om+json`, wraps in `message.BaseMessage`, publishes to JetStream subject `cs-api.observations.{datastreamID}` with audit + W3C trace headers.
  - `GET /areas` — spatial filtering via `?bbox=minLon,minLat,maxLon,maxLat` or `?polygon=<GeoJSON Polygon>` (exactly one required). Optional `?limit`. Returns a GeoJSON `FeatureCollection`; Features carry `geometry: null` because the framework's `SpatialResult` only returns entity IDs (`X-CS-Geometry-Available: false` signals this). Clients drill via `GET /systems/{id}` for precise coordinates.
  - `GET /conformance` — declares all v0.1 classes wired (core + json + oms + sensorml + json-ld + geojson). Stage 5 closes the gap between the ADR-S001 v0.1 claim and the runtime declaration.
  - `GET /health`.
  - All read endpoints accept `HEAD`. Routes use Go 1.22+ method+path patterns (`GET /systems` / `HEAD /systems`); 405 is enforced by the mux.
- Auth seam: `IdentityMiddleware` populates `Identity` in every request context. Anonymous-by-default; `X-Forwarded-User` / `X-Forwarded-Email` from a trusted reverse proxy flow onto every publish as `X-CS-Forwarded-*` NATS headers for audit. No verification at v0.1.
- Content negotiation via `Accept`; per-family supported sets in `negotiation.go`. JSON for everything; SensorML + JSON-LD for `GET /systems/{id}` only at Stage 4; collection `GET /systems` honestly 406s on non-JSON Accept (no SensorML "SystemCollection" type).
- Body-size limit middleware (`MaxRequestBytes`) enforces `413` on POSTs.
- JetStream: `cs-api.observations.>` stream is EnsureStream'd at component Start() with 30-day file retention. A failure to provision the stream surfaces as a `Start()` error, not a 503-orphan.
- Error classification: `errEntityNotFound` sentinel → 404; `pkg/errs.IsInvalid / IsTransient` → 400 / 503; raw `nats.ErrNoResponders` / `nats.ErrTimeout` / `context.DeadlineExceeded` / `nats.ErrConnectionClosed` wrapped to Transient at the boundary on both Request and PublishMsg paths. Unclassified → 500 with a generic body (full error logged).
- **`classifyEntityQueryError`** wraps the framework's unstructured request-reply error format (raw `"error: <msg>"` byte prefix from `natsclient.SubscribeForRequests`) into pkg/errs classes + the 404 sentinel. Upstream issue filed with `C360Studio/semstreams`; when structured errors ship (NATS headers + classified JSON body), this function becomes a no-op.
- **Stage 6 conformance harness** (`conformance/run.sh` + `.github/workflows/conformance.yml`) boots NATS + cs-api-server + OGC Team Engine with the [Botts CS API ETS](https://github.com/Botts-Innovative-Research/ets-ogcapi-connectedsystems10) via docker compose, hits TE's REST API, and archives a TestNG XML report. The ETS is pinned by commit SHA in `conformance/.ets-pin`; bumping is intentional. **Calibration reality**: the pinned ETS is `0.1-SNAPSHOT` (scaffold only) — a zero-failure run today validates harness wiring, not spec conformance. When real CS API tests land, re-running surfaces the actual conformance picture. The harness runs on push to `main`, on `workflow_dispatch`, and on PRs labelled `conformance` — **not a PR-blocking gate** at this stage.
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
| `GET /systems` | `graph.index.query.predicate` (rdf:type = ssn:System) → JSON SystemCollection. 406 for non-JSON Accept. |
| `GET /systems/{id}` | `graph.query.entity` → `EntityState` → `reconstructProcessFromTriples` (JSON / SensorML) or `export.Serialize(JSONLD)`. Lossy fields documented via `X-CS-Reconstructed-Lossy: true`. |
| `POST /systems` | `parser/sensorml` decode → `graph-ingest` publish *(not yet wired)* |
| `GET /datastreams/{id}/observations` | KV watch on entity-keyed subject → `message/oms` marshal *(not yet wired)* |
| `POST /datastreams/{id}/observations` | `message/oms` decode → `message.NewBaseMessage` → `js.PublishMsg` on `cs-api.observations.{id}` with `X-CS-*` audit headers + W3C trace context (raw `js.PublishMsg`, not `natsclient.PublishToStream`, so we can attach headers — trace is re-injected via `natsclient.InjectTrace` to match framework convention) |
| `GET /areas?bbox=` / `?polygon=` | `graph.spatial.query.bounds` / `.polygon` → bare `[]SpatialResult` → `geojson.FeatureCollection` with `geometry: null` Features (`X-CS-Geometry-Available: false` signals the framework gap — `SpatialResult` lacks coordinates) |

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
