# ADR-S001 - CS API server scope

- **Status**: Accepted (2026-06-02); ownership and current pin amended by ADR-S003
- **Repo**: `semconnect`
- **Companion**: [ADR-044 (semstreams)](https://github.com/C360Studio/semstreams/blob/main/docs/adr/044-ogc-connected-systems-framework-split.md)
- **Historical framework pin**: `github.com/c360studio/semstreams v1.0.0-beta.141`
- **Amendment**: [ADR-S003](003-semstreams-beta147-product-boundary-migration.md)

> ADR-S003 supersedes this ADR's OGC package-ownership statements and current
> framework pin. The HTTP scope, conformance claims, auth posture, binary shape,
> API versioning, and SemStreams graph-backend decision remain in force. The
> beta.141 pin and 137-test result below are historical baseline evidence.

## Context

ADR-044 originally split framework-shaped primitives (`semstreams`) from
deployment-shaped concerns (`semconnect`). At this ADR's beta.141 baseline,
SemStreams still hosted SOSA/SWE/OMS vocabularies, SensorML and SWE parsers,
GeoJSON helpers, graph mutation/query subjects, NATS client helpers,
ObjectStore, and artifact storage primitives. ADR-S003 later moves the OGC
product bundle into semconnect while leaving graph, NATS, JetStream,
ObjectStore, ownership, and projection in SemStreams.

This ADR records the v0.1 server scope: conformance classes, content
negotiation, auth posture, conformance-test ownership, Part 3 stance, binary
shape, versioning, backend assumptions, and upstream-ask discipline.

Stage 55 closed the original bootstrap target: the pinned CS API ETS reports
`total=137 passed=137 failed=0 skipped=0`.

## Decisions

### 1. Conformance Classes Claimed At v0.1

The running `/conformance` endpoint declares the full v0.1 class set that the
pinned ETS validates:

- OGC API Common Part 1: core, JSON, and OAS 3.0.
- CS API Part 1: core, JSON, OMS, SensorML, JSON-LD, GeoJSON, System,
  Advanced Filtering, Subsystem, Subdeployment, create/replace/delete, update,
  Procedure, Deployment, Sampling Feature, and Property.
- CS API Part 2: API Common, ControlStream, SystemEvent, Datastream, Advanced
  Filtering, and Command Feasibility.

HTML, XML, GML, and CS API Part 3 pub/sub bindings remain out of scope for
v0.1.

**Rationale**: The gateway now has enough resource coverage to claim the
classes that matter for OSH-shaped CS API compatibility while still keeping
non-JSON encodings and Part 3 out of the first release.

### 2. Content Negotiation Policy

`Accept` and OGC Common `?f=` query overrides drive response encoding. An
explicit unsupported `?f=` or unsupported `Accept` returns `406 Not
Acceptable` instead of silently falling back.

Resource-family defaults:

| Resource family | Default | Additional encodings where supported |
|---|---|---|
| Common discovery, collections, and conformance | `application/json` | OpenAPI YAML/JSON on `/api` |
| Systems and Procedures | `application/json` | `application/sml+json`, `application/sensorml+json`, `application/ld+json`, GeoJSON collections |
| Deployments and Sampling Features | `application/json` | GeoJSON collections/items where spatially meaningful |
| Datastreams, ControlStreams, Commands, SystemEvents, Feasibility | `application/json` | SWE/OMS subresources where specified |
| Observations | `application/json` | `application/om+json`, `application/swe+json`, `application/swe+csv`, `application/swe+binary` |
| Areas | `application/geo+json` | JSON only where the handler explicitly supports it |

No XML, HTML, or GML encodings are emitted at v0.1.

### 3. Auth Posture

At v0.1, deployments are anonymous-by-default and reverse-proxy-mediated. The
gateway does not verify tokens itself. Trusted proxy identity headers
(`X-Forwarded-User`, `X-Forwarded-Email`) flow into request context and onto
publish headers for audit.

In-process JWT verification is deferred until a deployment needs it. A future
ADR should cover JWKS discovery, scope-to-endpoint authorization, and token
introspection if that need appears.

### 4. Conformance-Test Ownership

`conformance/` owns a reproducible local and CI harness, not a vendored copy
of Team Engine or the ETS.

- Upstream pins live in `conformance/.ets-pin`.
- The ETS is built from the pinned Botts commit until an official OGC image is
  available.
- The semstreams backend image is built from the pinned semstreams commit and
  must match the Go module pin.
- The harness seeds fixtures through gateway HTTP endpoints, then invokes the
  ETS through Team Engine's REST API.
- TestNG XML, service logs, build logs, and seed logs are archived under
  `conformance/output/`.

**Current result**: The 2026-07-06 SemStreams pin refresh reports
`total=137 passed=137 failed=0 skipped=0` against Botts ETS `d9caf33` and
semstreams `v1.0.0-beta.141`.

### 5. CS API Part 3 Pub/Sub Binding Stance

Defer the external binding decision. Observation publish/readback already uses
stable JetStream subjects (`cs-api.observations.{datastream_id}`), so a future
Part 3 binding can subscribe to the same substrate.

Native NATS remains the likely first binding for SemStreams operators. MQTT or
WebSocket bindings should wait for a concrete consumer.

### 6. Binary Shape

v0.1 ships a single reference binary: `cmd/cs-api-server`.

The component can still be embedded under semstreams service management later,
but one deployable server is the clearest reference shape for the first CS API
release.

### 7. API Versioning

Paths are unprefixed at v0.1: `/systems`, `/datastreams`,
`/controlstreams`, `/systemEvents`, `/feasibility`, and peers.

Major incompatible future surfaces should introduce a prefix such as `/v2`.
Minor OGC-compatible additions should remain on the unprefixed surface.

### 8. Graph Backend

The v0.1 backend is semstreams over NATS and JetStream. Gateway code uses the
framework request/reply and ObjectStore contracts directly:

- entity mutations for graph writes
- predicate, entity, batch, and spatial queries for reads
- JetStream for observations
- ObjectStore-backed artifact entities for SWE schemas and command schemas

At the current semstreams pin, the System SensorML write path also exercises
the governed semantic-state projection lane. SemConnect stamps the
`c360.csapi.system.v1` System projection producer type, binds a `NoBirthStub`
`sensorml.PredIsHostedBy` foreign-edge claim for the configured System ID
prefix, forwards child foreign-edge triples, and relies on graph-ingest to
partition and route those edges onto their own subjects. At this pin,
foreign-edge ownership classification is observe-only: unclaimed edges are
metered and warned but still routed. Treat this as write-semantics governance and
provenance, not as an endpoint authorization boundary.

Ownership arbitration is not encoded as domain triples. It belongs to the
SemStreams ownership substrate and its owner-claim registry. Domain triples may
still record provenance about graph materialization, source, audit, or lineage;
for example, a SemStreams referential-integrity stub may carry a
`core.identity.stub_owner` triple naming the producer that caused the stub to be
created. That is provenance on the entity, not an ownership claim.

Adding a second backend is a future design decision, not a v0.1 abstraction
requirement.

### 9. Upstream-Ask Discipline

Graph, NATS, JetStream, ObjectStore, ownership, and projection gaps are filed
upstream on SemStreams. OMS, SensorML, SWE Common, SOSA/SWE, CS API vocabulary,
and other transferred OGC package work is semconnect-owned product work.
Gateway-local shims remain acceptable only when narrow, non-blocking, and easy
to retire.

Current asks and transferred history are tracked in
`docs/upstream-asks/README.md`. The 2026-07-06 statement that vocabulary asks
belonged upstream is historical; ADR-S003 transfers that ownership to
semconnect.

## Consequences

**Enables:**

- A complete pinned-ETS green CS API v0.1 reference server.
- Reuse of SemStreams graph and storage primitives while semconnect owns its
  OGC parser, vocabulary, and schema behavior.
- Clear operator deployment behind existing identity infrastructure.
- A practical case study for when CS API data belongs in graph and when it
  belongs in ObjectStore-backed artifacts.

**Defers:**

- HTML, XML, and GML encodings.
- CS API Part 3 pub/sub bindings.
- In-process JWT verification.
- Command execution and real feasibility evaluation.
- SWE Common Phase 2 and other separately planned OGC product expansions.

## Closure

This ADR's original bootstrap target is closed.

- Code-side: the reference server and gateway package are implemented.
- Harness-side: `conformance/run.sh` and CI workflow boot the full stack.
- Validation-side: the 2026-07-06 SemStreams pin refresh reports
  `total=137 passed=137 failed=0 skipped=0` against the pinned ETS.

Future changes to conformance scope should be recorded as new ADRs or explicit
updates to this one.

## References

- [ADR-044 - framework / sister split](https://github.com/C360Studio/semstreams/blob/main/docs/adr/044-ogc-connected-systems-framework-split.md)
- [framework-primitives reference](https://github.com/C360Studio/semstreams/blob/main/docs/operations/21-adr044-framework-primitives-reference.md)
- [CS API Part 1 (23-001)](https://docs.ogc.org/DRAFTS/23-001r0.html)
- [CS API Part 2 (23-002)](https://docs.ogc.org/DRAFTS/23-002r0.html)
- [OGC Team Engine](https://github.com/opengeospatial/teamengine)
- [Botts CS API ETS](https://github.com/Botts-Innovative-Research/ets-ogcapi-connectedsystems10)
- [conformance/README.md](../../conformance/README.md)
- [docs/upstream-asks/README.md](../upstream-asks/README.md)
- [ADR-S003 - SemStreams beta.147 product-boundary migration](003-semstreams-beta147-product-boundary-migration.md)
