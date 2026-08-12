# Greenfield Compose deployment

This bundle starts the pre-v1 semconnect product surface on SemStreams
`v1.0.0-beta.160` and NATS `2.14.4`. It is for a newly provisioned NATS volume
only. It has no state-import, in-place upgrade, compatibility-reader, or
old-state path.

The checked-in NATS image is exactly
`nats:2.14.4-alpine@sha256:f2123f533c2b0cada0a5c5ec434fb2b8cfe1cf220215ef9d7517e1372917ad66`.
The framework tag resolves to commit
`8403a2218000e45a31c5132fbfe01af42ed04f14` and source tree
`9ed5dd3792bca63ce87ebf449a180add918f59ed`.

The authoritative beta.160 r5 qualification proved empty first start, readiness
3/3, normal stop, same-volume no-write restart, and byte-identical System,
Datastream, schema, and global/scoped Observation evidence. The observations
stream remains bounded at 1 GiB/30 days with `DiscardOld`. External conformance
remains `137 passed, 0 failed, 0 skipped`.

Production is **GO WITH ACCEPTED RISK** by product-owner authorization on
2026-08-12. No separate beta.159 rollback environment was proved; task 9.5 is
waived, not passed. Never point beta.160 at beta.159 state, or beta.159 at
beta.160 state.

NATS is internal-only on the private Compose network, publishes no host ports,
and therefore has no NATS credentials in this bundle. The CS API is published
on `${SEMCONNECT_PORT:-8080}`. Publishing NATS outside the network is a
different security design and is not covered here.

## Start

From the repository root:

```sh
docker compose -f deploy/compose.yml up -d --build
```

The named `semconnect-nats-data` volume retains JetStream state across ordinary
Compose stops, starts, and host restarts. Do not point this bundle at an existing
NATS namespace or volume.

## First-start and persistence proof

The verifier proves zero streams before SemStreams starts, then seeds one
versioned System, a schema-backed Datastream that references it, and one
deterministic Observation. It actively polls the System, Datastream schema,
global Observation collection, and Datastream-scoped Observation collection;
stops all three services normally; restarts them over the same volume without
another write; requires the post-restart indexed graph revision to be equal to
or later than the captured pre-restart target; and compares the normalized
resource proof byte-for-byte. Graph readiness is event-driven from
`GRAPH_STATUS`; the verifier does not substitute a fixed sleep for readiness.

```sh
SEMCONNECT_NATS_VOLUME=semconnect-beta160-rehearsal \
  EVIDENCE_DIR=/tmp/semconnect-beta160-evidence \
  ./deploy/verify-persistence.sh
```

Use a newly allocated volume name for every clean qualification. The verifier
leaves services and the named volume in place for operator inspection; it never
removes storage. A clean NATS 2.14.4 stop requires exit code 0,
`OOMKilled=false`, and both JetStream and server shutdown lifecycle markers.
SemStreams and semconnect must also exit zero.

The authoritative 2026-08-12 proof used volume
`semconnect-beta160-qualification-20260812-r5`. Before and after restart,
the normalized resource record had digest
`4544c19899ccf1c9b461146be542c47dd85d4201aad5aeda2c9cf48946b63e32`
and graph readiness was target/indexed `3/3`. All services exited 0 with
`OOMKilled=false`; the NATS log contains the JetStream and server shutdown
lifecycle markers. Evidence is archived under
`openspec/changes/migrate-semstreams-beta160/evidence/persistence-r5/`.

The earlier partial proof used volume
`semconnect-beta160-qualification-20260812-r3`. Before and after restart, the
canonical entity was `c360.semconnect.systems.csapi.system.v1` with item digest
`29aa791b31dfa40c1f14cd5ac8768a4a2cdb9c0e8ca17324cb321f76138e169b`.
Archived evidence is under
`openspec/changes/migrate-semstreams-beta160/evidence/persistence-r3/`.
That historical archive predates the expanded verifier and proves System parity
only; r5 supersedes it as the authoritative persistence record.

The versioned fixtures are [canonical-system.v1.json](canonical-system.v1.json),
[canonical-datastream.v1.json](canonical-datastream.v1.json), and
[canonical-observation.v1.json](canonical-observation.v1.json).

## Rollback boundary

Rollback is a routing operation to a separate beta.159 deployment with its
untouched beta.159 volume. It is not a binary downgrade over beta.160 storage.
Before production approval, record both volume identities and prove that neither
binary opens the other version's state. The local 2026-08-12 Docker inventory
contained no preserved beta.159 volume. The product owner waived this proof on
2026-08-12; task 9.5 remains explicitly unproven.

## Accepted-risk rollout

Before enabling traffic, capture the new volume identity, exact image and input
hashes, empty-state preflight, and readiness evidence. Verify recoverable
backups of authoritative reseed inputs, rendered configuration, deployment
metadata, and all nondisposable product data. Retain these with the r5 and
release-decision archives.

During cutover and for at least 60 minutes after traffic begins, poll every
30–60 seconds: service health, `GRAPH_STATUS` target/indexed parity, critical
CS API probes, observations, artifact reads, capacity, restart/OOM state, 5xx,
conflicts, and uncertain commits. Abort on pin drift, retained state, readiness
regression, parity failure, panic, OOM, capacity breach, or unreconciled commit
uncertainty. Stop traffic and writes, preserve logs and beta.160 state, then use
forward repair or a clean beta.160 rebuild. The accepted risk never authorizes
a beta.159 binary to open beta.160 state.
