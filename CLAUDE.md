# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Repository status

**Stage 2 of the bootstrap playbook is landed.** What works:

- `cmd/cs-api-server/` — reference binary, builds and runs.
- `gateway/cs-api/` — `Component` implementing `component.Discoverable + LifecycleComponent + gateway.Gateway`.
- Endpoints: `GET /systems` (lists `ssn:System` entities via NATS `graph.index.query.predicate`), `GET /conformance`, `GET /health`. `HEAD` is supported on the read endpoints.
- Auth seam: `IdentityMiddleware` populates `Identity` in every request context. Anonymous-by-default; `X-Forwarded-User` / `X-Forwarded-Email` from a trusted reverse proxy are captured for audit only.
- Content negotiation via `Accept`; only `application/json` is wired at Stage 2 (other media types 406 honestly — SensorML / GeoJSON / OMS / JSON-LD land per stage; see ADR-S001 §1 schedule).
- Body-size limit middleware (`MaxRequestBytes`) — seam ready for Stage 3 POST endpoints.
- Error classification: `pkg/errs.IsInvalid / IsTransient` → 400 / 503; raw `nats.ErrNoResponders` / `nats.ErrTimeout` / `context.DeadlineExceeded` wrapped to Transient at the boundary. Unclassified → 500 with a generic body (full error logged).

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
| `GET /systems` | `graph-query` over `ssn:System` entities → JSON / JSON-LD via `vocabulary/export` |
| `GET /systems/{id}` | `graph-query` full entity → reconstruct `sensorml.PhysicalSystem` from triples (sister-repo reverse mapping) |
| `POST /systems` | `parser/sensorml` decode → `graph-ingest` publish |
| `GET /datastreams/{id}/observations` | KV watch on entity-keyed subject → `message/oms` marshal |
| `POST /datastreams/{id}/observations` | `message/oms` decode → `message.NewBaseMessage` → `natsclient.PublishToStream("cs-api.observations.{id}", wire)` |
| `GET /areas?bbox=` / `?polygon=` | `graph.spatial.query.bounds` / `.polygon` |

The triple → SensorML reverse mapping (`GET /systems/{id}`) is gateway domain code and the one place real reconstruction logic lives in this repo. Plan a `gateway/cs-api/sensorml.go` with `FromEntityState(state graph.EntityState) (sensorml.Process, error)`.

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

No `Taskfile` or `Makefile` yet — intentional until Stage 6 wires the conformance harness.

Running the binary needs a NATS server reachable at `nats://localhost:4222` (configurable via `--config`):

```bash
/tmp/cs-api-server                                  # default config
/tmp/cs-api-server -config ./cs-api.json            # JSON config file
```

A config-less run binds `:8080` and connects to local NATS. With nothing on either, it fails fast with a clear NATS-connect error — by design.

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
