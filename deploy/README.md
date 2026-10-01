# Greenfield Compose deployment

This draft bundle uses SemConnect's registered graph host on the frozen SemEngine
SETUP 03A SemStreams pin, `v1.0.0-beta.162.0.20260930150212-8b99efe9c66a`, commit
`8b99efe9c66a4faa4fa509f9f62cc6bad8392128`, source tree
`605f83cb8492eda3bd347ac30a211b9babf3931f`. It contains no SemEngine runtime.
The shared pin was frozen because `v1.0.0-beta.163` was unavailable; this bundle
must not select a newer SemStreams revision independently.

Target qualification and independent Go review are pending. This draft does not
inherit the beta.160 production approval. The target's cross-authority mutation
restriction is a known compatibility concern under investigation; local resource
conformance alone cannot establish that existing federation behavior is preserved.
See the [migration proposal](../openspec/changes/migrate-semstreams-setup03a/proposal.md).

NATS remains exactly
`nats:2.14.4-alpine@sha256:f2123f533c2b0cada0a5c5ec434fb2b8cfe1cf220215ef9d7517e1372917ad66`.
Use newly provisioned NATS storage only. There is no state-import, in-place upgrade,
compatibility-reader, or old-state path. Never open beta.160 storage with this
revision, or this revision's storage with beta.160.

NATS is internal-only on the private Compose network, publishes no host ports,
and therefore has no NATS credentials in this bundle. The CS API is published
on `${SEMCONNECT_PORT:-8080}`. Publishing NATS outside the network requires a
separate security design.

## Registered host and stable identity

The target makes the payload registry the type authority. The stock SemStreams
binary does not register SemConnect's eleven resource projections and cannot
serve this bundle. [backend.Dockerfile](backend.Dockerfile) builds the consumer-owned
`cmd/cs-graph-backend` against the same exact module pin as the HTTP gateway.
Both base images are pinned by digest; the build rejects a different module version.
The host receives the gateway configuration through `-cs-api-config`, so operator
resource prefixes and artifact settings are shared with its registered contracts.

The backend configuration declares `platform.org=c360` and `platform.id=semconnect`
as the deployment stem. The removed `platform.instance_id` field is absent.
Target ConfigManager would normally mint a suffixed platform identity on first
start. To preserve public `c360.semconnect.*` identifiers, the explicit one-shot
`provision-identity` service runs `-provision-identity semconnect` before graph
startup. It creates the framework catalog's `platform_identity` record atomically,
verifies a matching existing record, and refuses a different one without overwrite.
Normal backend startup then uses the framework's identity adoption path.

This provisioning is a distinct operator action in Compose. Empty-storage
qualification runs before provisioning. Ordinary same-volume restarts retain the
identity; no fixture identifiers are rewritten to satisfy conformance.

## Start

After target qualification and release approval, start from the repository root:

```sh
docker compose -f deploy/compose.yml up -d --build
```

The default `semconnect-nats-setup03a-8b99efe9c66a-data` volume is exclusive to this
revision. It retains JetStream state across ordinary stops, starts, and host
restarts. Set `SEMCONNECT_NATS_VOLUME` to a new explicit name for each isolated
qualification; never point the bundle at an existing unrelated NATS namespace.

## First-start and persistence proof

The verifier proves zero streams before identity provisioning or graph startup,
then seeds one versioned System, a schema-backed Datastream, and one deterministic
Observation. It checks the System, schema, global Observation collection, and
scoped Observation collection; stops all three persistent services normally;
restarts them over the same volume without another fixture write; requires the
post-restart indexed graph revision to cover the captured pre-restart target;
and compares the normalized resource proof byte-for-byte. Graph readiness is
event-driven from `GRAPH_STATUS`.

```sh
COMPOSE_PROJECT_NAME=semconnect-setup03a-rehearsal \
  SEMCONNECT_NATS_VOLUME=semconnect-setup03a-8b99efe9c66a-rehearsal-fresh \
  EVIDENCE_DIR=/tmp/semconnect-setup03a-evidence \
  ./deploy/verify-persistence.sh
```

The verifier leaves its services and named volume for inspection; it never removes
storage. A clean NATS 2.14.4 stop requires exit code 0, `OOMKilled=false`, and both
JetStream and server shutdown markers. The graph host and HTTP gateway must also
exit zero. The `provision-identity` service must finish successfully before the
backend starts; rerunning it against a matching identity performs no overwrite.

The supplementary read-only spatial probe asserts exact canonical point geometry
for bbox and polygon hits and no features for a remote bbox. Run the same probe
before and after a clean no-write restart, with distinct output directories:

```sh
python3 scripts/setup03a/spatial-probe.py \
  --url http://localhost:8080 --output /tmp/semconnect-spatial-before
```

The fixtures remain [canonical-system.v1.json](canonical-system.v1.json),
[canonical-datastream.v1.json](canonical-datastream.v1.json), and
[canonical-observation.v1.json](canonical-observation.v1.json). External OGC
qualification remains SemConnect-owned: the existing ETS suite must retain
`137 passed, 0 failed, 0 skipped`, without weakening assertions or skipping failures.

## Historical beta.160 baseline

The 2026-10-01 pre-migration baseline passed all Go race/vet/build/module gates,
unchanged ETS `137/0/0`, real-NATS lifecycle, root-only SensorML bake, and two
same-volume normal restart checks, including spatial response parity. Evidence is
under [the baseline archive](../openspec/changes/migrate-semstreams-setup03a/evidence/baseline/README.md).
Its volume `semconnect-03a-beta160-20261001-fresh` is preserved exclusively for
beta.160 and must never be opened by this target.

The historical 2026-08-12 r5 proof used
`semconnect-beta160-qualification-20260812-r5`. Both r5 and the 2026-10-01 baseline
record the canonical proof digest
`4544c19899ccf1c9b461146be542c47dd85d4201aad5aeda2c9cf48946b63e32`
and target/indexed readiness `3/3`, with all persistent services exiting zero and
no OOM. R5 evidence is under
`openspec/changes/migrate-semstreams-beta160/evidence/persistence-r5/`.
The earlier r3 archive proves System parity only and is superseded by r5.

Beta.160 was **GO WITH ACCEPTED RISK** by product-owner authorization on 2026-08-12.
Its separate beta.159 rollback environment was waived, not proved. That historical
approval and waiver do not authorize this target or cross-version storage reuse.

## Release and rollback boundary

Keep this migration draft until compatibility differences, failures, target
qualification, and independent Go review are recorded. A later SemEngine switch
requires its own qualified contract and must not mix both frameworks in one binary.

Any rollback must route to a separate deployment using its own untouched matching
volume. Never downgrade a binary over another revision's storage. Before an approved
cutover, preserve exact image/input hashes, volume identity, empty-state preflight,
readiness and critical query evidence, and recoverable authoritative inputs.
During an approved rollout, poll service health, graph readiness, observations,
artifact reads, capacity, exit/OOM state, conflicts, and uncertain commits every
30–60 seconds. Preserve failure evidence and stop traffic on a proven regression.
