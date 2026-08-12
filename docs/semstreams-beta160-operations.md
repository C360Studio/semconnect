# SemStreams beta.160 operations

This is the current operator guide for the breaking migration from SemStreams
beta.159 to beta.160. ADR-S004 and the OpenSpec change remain the binding
technical contract.

## Exact release set

- SemStreams tag: `v1.0.0-beta.160`
- Release commit: `8403a2218000e45a31c5132fbfe01af42ed04f14`
- Source tree: `9ed5dd3792bca63ce87ebf449a180add918f59ed`
- NATS server: `2.14.4`
- NATS image:
  `nats:2.14.4-alpine@sha256:f2123f533c2b0cada0a5c5ec434fb2b8cfe1cf220215ef9d7517e1372917ad66`
- `nats.go`: `v1.52.0`

Do not mix module, backend, configuration, or server revisions from a different
release set.

## State and rollback boundary

Beta.160 starts only on newly provisioned NATS storage. It has no beta.159
reader, importer, alias, or in-place conversion. A retained volume is a stop
condition, not permission to clear or reuse it.

Rollback means routing clients to a separate beta.159 deployment and its
untouched beta.159 volume. Never open beta.159 state with beta.160, or beta.160
state with beta.159.

The authoritative 2026-08-12 r5 qualification proved fresh beta.160 startup,
readiness target/indexed `3/3`, and byte-identical no-write restart parity for
the System, schema-backed Datastream, schema endpoint, and global and scoped
Observation reads. It used volume
`semconnect-beta160-qualification-20260812-r5`. The earlier r3 System-only proof
is retained as historical evidence.

Local Docker inventory contained no preserved beta.159 rollback volume. The
product owner explicitly accepted that residual risk on 2026-08-12 and issued
**GO WITH ACCEPTED RISK**. Task 9.5 is waived, not proven. This decision does
not authorize an in-place upgrade or any cross-version volume access.

## Accepted-risk rollout guardrails

Before enabling traffic:

- allocate and record a never-before-used beta.160 volume identity;
- prove the volume is empty with the same authoritative preflight used by r5;
- record the exact images, rendered Compose, configuration, and input hashes;
- verify recoverable backups of authoritative reseed inputs, rendered
  configuration, deployment metadata, and all nondisposable product data; and
- retain the r5, conformance, review, and product-owner decision evidence.

During cutover and for at least 60 minutes after traffic begins, actively poll
every 30–60 seconds: `/health`, `GRAPH_STATUS` target/indexed revision parity,
critical CS API read/write probes, JetStream observations, schema artifact
reads, NATS capacity, service restart/OOM state, HTTP 5xx rates, revision
conflicts, and uncertain-commit audit records. Silence is not success; compare
timestamps and revisions to wall clock.

Abort the rollout if the volume is not empty, exact pins differ, readiness
regresses or stops advancing, post-start parity fails, a service panics or is
OOM-killed, storage limits are violated, or commit uncertainty cannot be
reconciled. On abort, stop new traffic, stop writes, preserve logs and the
beta.160 volume, then choose forward repair or a clean beta.160 rebuild from
verified authoritative backups. Never attempt a beta.159 binary downgrade over
beta.160 state.

## Mutation behavior

The gateway uses typed `semstreams.graph.mutation/v1` create, reconcile, and
delete operations. Replace, PATCH, and delete carry the exact nonzero KV
revision returned by the read that authorized the mutation. The gateway makes
one mutation attempt and does not silently retry.

| Outcome | HTTP behavior | Caller action |
|---|---|---|
| Create conflict or revision mismatch | 409 | Read current state and resolve the conflict. |
| Missing PATCH target | 404 | Use PUT only when create-or-replace is intended. |
| Missing DELETE target | 204 | Treat as successful idempotent deletion. |
| Invalid mutation | 400 | Correct the request. |
| Internal graph failure | 500 | Correlate server logs; do not treat it as a retry hint. |
| Backend unavailable | 503 | Retry according to the client's transient-error policy. |
| Commit outcome unknown | 503 plus uncertainty headers | Verify resource state before any retry. |

An uncertain commit sets `X-CS-Commit-Uncertain: true` and a non-empty
`X-CS-Correlation-ID`. The response body instructs the caller to verify state
before retrying. The correlation ID matches the structured mutation audit
request ID.

## Structured audit

Every graph mutation attempt emits one `graph mutation audit` record containing:

- request and trace IDs;
- operation and entity ID;
- attempted and resulting KV revision evidence;
- commit state;
- authenticated identity and trusted forwarded hints when available; and
- classified error detail.

Typed graph mutations do not promise custom forwarded identity headers on the
internal NATS wire. Observation publishing is a separate existing product path
and continues to attach its audit and trace headers.

## SensorML projection limit

A SensorML write mutates only its primary root entity. Root facts and
root-to-child references may remain. Foreign-subject inverse facts are dropped,
and an embedded child is not independently materialized. Post the child as its
own System when it needs an entity lifecycle.

The external qualification includes a strict bake that reads the root and
proves the inline child absent.

## Schema artifacts

Canonical SWE schema bytes determine the immutable artifact entity ID and
ObjectStore key. The storage provider instance is exactly `objectstore`; the
bucket is exactly `CS_API_ARTIFACTS`. Existing content and metadata are verified
against the digest before reuse. A parent schema change links a new digest
artifact instead of updating an existing one.

Object durability precedes graph visibility. A later graph failure may leave a
content-addressed orphan. Orphan scanning and garbage collection are deferred;
this migration performs neither.

## Qualification commands

Run static and Go gates:

```sh
openspec validate migrate-semstreams-beta160 --strict
go mod verify
go vet ./...
go vet -tags=integration ./...
go test ./... -count=1
go test -race ./... -count=1
go test -tags=integration ./gateway/cs-api -count=1
go build ./...
```

Run a fresh persistence proof with a never-before-used volume name:

```sh
SEMCONNECT_NATS_VOLUME=semconnect-beta160-rehearsal \
  EVIDENCE_DIR=/tmp/semconnect-beta160-evidence \
  ./deploy/verify-persistence.sh
```

Run external conformance from disposable volumes:

```sh
./conformance/run.sh
```

The required external result is exactly
`total=137 passed=137 failed=0 skipped=0`, plus a passing root-only SensorML
bake. Evidence from 2026-08-12 is archived under
`openspec/changes/migrate-semstreams-beta160/evidence/`.

## Production approval checklist

- Exact SemStreams commit/tree and NATS 2.14.4 digest match the release record.
- The beta.160 volume is new and empty before first start.
- Startup readiness and same-volume no-write restart parity pass.
- Schema artifact and observation persistence pass.
- External conformance is exactly `137/0/0` with the strict root-only bake.
- Independent Go and no-weakening reviews are approved.
- Rollback isolation is proved, or its absence is explicitly accepted and
  waived by the product owner with fresh-volume and abort guardrails retained.
- The architect, product owner, and operator explicitly issue the production
  decision.

The 2026-08-12 decision is **GO WITH ACCEPTED RISK**. It may be revoked if any
guardrail fails or the deployed inputs differ from the qualified release set.
