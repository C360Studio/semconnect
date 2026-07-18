# Conformance deployment inventory

- **Deployment**: disposable local `conformance/` Docker Compose stack
- **Compose project default**: `semconnect-conformance`
- **NATS account**: `$G` (unauthenticated default global account)
- **In-network server**: `nats://nats:4222`
- **NATS context**: direct server; no named NATS CLI context
- **JetStream storage volume**: `semconnect-conformance_nats-data`
- **Target framework**: SemStreams `v1.0.0-beta.147`
- **Target commit**: `5cc22c109594e48b7f1cec04bcaaf0106d85495a`
- **Status**: inventory complete; execution no-go pending immutable manifest

## Sources used

| Source | Values derived |
|---|---|
| `conformance/compose.yml` | project name, service names, NATS network address, named volume, config mounts |
| `conformance/nats.conf` | one unauthenticated account and JetStream store at `/data` |
| `conformance/compose.semstreams.config.json` | graph writers and configured KV bucket names |
| `conformance/compose.cs-api.config.json` | NATS URL; default gateway resource names apply because no overrides exist |
| `gateway/cs-api/config.go` | `CS_API_OBSERVATIONS` and `CS_API_ARTIFACTS` defaults |
| SemStreams beta.141 source at `d46c07a3a32a28259fd1c571a0445b140c8405e8` | legacy `PREDICATE_CATALOG` creation |
| SemStreams beta.147 module | revision readiness and ownership bucket contracts |
| `conformance/run.sh` | fixture writers, seed source, collection probes, external ETS invocation |

The harness chooses a free host port unless `NATS_HOST_PORT` is set. Any
rehearsal must freeze it explicitly. This inventory uses
`NATS_HOST_PORT=14222`, `NATS_MON_HOST_PORT=18222`, and
`TE_HOST_PORT=18181`; literal NATS CLI commands therefore use
`nats://127.0.0.1:14222`.

## Complete writer inventory

All four services or activities below must be stopped before a destructive
command:

1. `teamengine`, which can issue write requests through the gateway while a
   suite is active.
2. `conformance/run.sh` fixture seeding and any human HTTP client attached to
   the compose network.
3. `cs-api-server`, which issues graph mutations and writes observations and
   artifacts.
4. `semstreams-backend`, whose `graph-ingest` component writes
   `ENTITY_STATES` and whose index processors write derived buckets.

The NATS service stays running so the operator can inspect and remove exact
resources. A rehearsal must verify that no unlisted client is connected to the
disposable account before deletion.

Literal stop command for the frozen project:

```bash
docker compose -p semconnect-conformance -f conformance/compose.yml stop teamengine cs-api-server semstreams-backend
```

Literal service-state check:

```bash
docker compose -p semconnect-conformance -f conformance/compose.yml ps
```

Expected state before deletion: only `nats` is running. A different result is
no-go.

## Incompatible graph resources

The exact deletion candidates are:

| Kind | Rendered name | Writer | Why incompatible |
|---|---|---|---|
| KV | `ENTITY_STATES` | `graph-ingest` | authoritative beta.141 rows can violate beta.147 identity contracts |
| KV | `OUTGOING_INDEX` | `graph-index` | derived from incompatible authoritative state |
| KV | `INCOMING_INDEX` | `graph-index` | beta.147 uses different relationship replacement/key semantics |
| KV | `ALIAS_INDEX` | `graph-index` | derived index rebuilt from canonical state |
| KV | `PREDICATE_INDEX` | `graph-index` | beta.147 predicate representation is incompatible |
| KV | `SPATIAL_INDEX` | `graph-index-spatial` | derived index rebuilt from canonical state |
| KV | `TEMPORAL_INDEX` | `graph-index-temporal` | derived index rebuilt from canonical state |
| KV | `PREDICATE_CATALOG` | beta.141 `graph-index` | legacy hash-to-name catalog removed in beta.147 |

`PREDICATE_CATALOG` is included because beta.141 creates it independently of
the rendered port list; this deployment's historical backend is exactly
beta.141. No wildcard or copied framework-default bucket is included.

Pre-rehearsal probes, one per exact candidate:

```bash
nats --server nats://127.0.0.1:14222 kv info ENTITY_STATES --json
nats --server nats://127.0.0.1:14222 kv info OUTGOING_INDEX --json
nats --server nats://127.0.0.1:14222 kv info INCOMING_INDEX --json
nats --server nats://127.0.0.1:14222 kv info ALIAS_INDEX --json
nats --server nats://127.0.0.1:14222 kv info PREDICATE_INDEX --json
nats --server nats://127.0.0.1:14222 kv info SPATIAL_INDEX --json
nats --server nats://127.0.0.1:14222 kv info TEMPORAL_INDEX --json
nats --server nats://127.0.0.1:14222 kv info PREDICATE_CATALOG --json
```

Candidate literal deletion commands, to be copied into the immutable manifest
only after actual counts and approvals are recorded:

```bash
nats --server nats://127.0.0.1:14222 kv rm ENTITY_STATES --force
nats --server nats://127.0.0.1:14222 kv rm OUTGOING_INDEX --force
nats --server nats://127.0.0.1:14222 kv rm INCOMING_INDEX --force
nats --server nats://127.0.0.1:14222 kv rm ALIAS_INDEX --force
nats --server nats://127.0.0.1:14222 kv rm PREDICATE_INDEX --force
nats --server nats://127.0.0.1:14222 kv rm SPATIAL_INDEX --force
nats --server nats://127.0.0.1:14222 kv rm TEMPORAL_INDEX --force
nats --server nats://127.0.0.1:14222 kv rm PREDICATE_CATALOG --force
```

These commands are documentation, not authorization. The immutable JSON
manifest remains absent until runtime counts and reviewers bind the exact
commands to one rehearsal.

## Preserve set

| Kind | Rendered name | Owner | Disposition |
|---|---|---|---|
| Stream | `CS_API_OBSERVATIONS` | semconnect | preserve by default; identity proof required |
| ObjectStore | `CS_API_ARTIFACTS` | semconnect | preserve content; rebuild graph artifact entities and links |
| KV | `OWNER_CLAIMS` | SemStreams ownership substrate | retain; beta.147 registration replaces the active epoch |
| KV | `OWNER_PRESENCE` | SemStreams ownership substrate | retain; liveness TTL and new registration refresh presence |

`PENDING_EDGES` is not listed because beta.141 deliberately does not create it
and the rendered deployment has no component that does. Any other runtime
resource discovered during preflight is unrelated state and defaults to retain
until separately classified.

## Seed source and expected inputs

The authoritative disposable source is `conformance/run.sh::seed_fixtures`
plus `conformance/fixtures/system.sml.json` and
`conformance/fixtures/system-hosted.sml.json`. It submits 13 graph-bearing HTTP
requests, one observation, and two schema artifacts. The hosted SensorML input
also exercises one claimed foreign child edge.

These are expected input counts, not fabricated KV-entry counts. Actual
`ENTITY_STATES`, index, observation, and object counts must be captured during
the rehearsal and written into the immutable manifest and execution evidence.

## Current blockers

- Stable seed identity remediation must be signed before retained-state proof.
- The candidate semconnect revision and built image digest do not exist until
  the migration change is committed and built.
- Runtime pre-cutover counts have not been captured.
- The exact post-seed `ENTITY_STATES` target revision has not been captured.
- Query and no-write replay parity have not run.
- Destructive reviewers and operator have not approved a manifest checksum.

Accordingly, this inventory is sufficient for documentation task 8.5 but not
for manifest task 8.3 or rehearsal task 9.1.
