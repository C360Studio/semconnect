# ADR-S004 - SemStreams beta.160 typed graph migration

- **Status**: Implemented and operationally qualified; production GO with accepted rollback risk (2026-08-12)
- **Repo**: `semconnect`
- **Framework target**: `github.com/c360studio/semstreams v1.0.0-beta.160`
- **Release commit**: `8403a2218000e45a31c5132fbfe01af42ed04f14`
- **Source tree**: `9ed5dd3792bca63ce87ebf449a180add918f59ed`
- **Execution contract**: `openspec/changes/migrate-semstreams-beta160/`
- **Amends**: ADR-S003; its product boundary and greenfield-only rule remain in force.

## Context

SemStreams beta.160 replaces raw graph mutation subjects with a typed mutation family, makes exact-read revisions
part of the concurrency contract, removes ownership bindings in favor of projection contracts, and closes component
port definitions. It also makes storage provider resolution exact and aligns its supported NATS proof with 2.14.

Semconnect currently depends on the removed raw mutation subjects, unmarshals an exact entity response directly into
entity state, binds ownership projections, and declares flat or custom ports. A dependency-only bump would therefore
either fail compilation or, for the exact-read shape, risk accepting a zero-value entity without an obvious error.

The beta.160 migration guide also requires newly provisioned NATS storage. It expressly provides no compatibility
reader, in-place migration, legacy alias, or mixed-version path. This is compatible with ADR-S003's greenfield rule,
but the concurrency, SensorML projection, artifact, and audit decisions require a new durable record.

## Decision

### 1. Adoption uses an exact release and newly provisioned state

The module, backend image, Compose configuration, tests, and evidence SHALL identify beta.160 commit
`8403a2218000e45a31c5132fbfe01af42ed04f14` and source tree
`9ed5dd3792bca63ce87ebf449a180add918f59ed` without a mixed framework revision.

Every beta.160 environment SHALL start on newly provisioned NATS storage. Conformance uses disposable volumes.
Deployment qualification uses a new volume, proves an ordinary stop and restart on that same beta.160 volume, and
makes no claim that a beta.159 volume can be opened. Discovery of retained deployed state stops adoption and requires
a separate owner-reviewed design.

Rollback means routing to a separately preserved beta.159 deployment and its untouched storage. A beta.159 binary
SHALL NOT open beta.160 state, and beta.160 SHALL NOT open retained beta.159 state.

### 2. Graph mutation is typed, revision-fenced, and single-attempt

Semconnect SHALL introduce a local domain-shaped graph state and operation interface at the gateway boundary. The
production adapter SHALL use beta.160's typed `semstreams.graph.mutation/v1` family and exact entity reads. Existing
handler tests may retain fakes behind that interface.

Create uses `entity.create`. Full replacement and PATCH use `entity.reconcile` with the exact nonzero KV revision
read before the desired state is computed. Delete uses `entity.delete` with the exact nonzero revision. No handler or
adapter silently retries a mutation.

The public projection client performs its own exact read before reconcile. That sequencing is unsuitable for the
gateway's read-merge-write PATCH. A semconnect-local revision-fenced reconcile adapter is therefore approved. It
SHALL use the exported reconcile request and response types and derive the operation subject from the declared typed
mutation family; it SHALL NOT copy or revive the removed raw subject table.

HTTP classification is fixed as follows:

- Create conflict and `revision_mismatch` return 409 Conflict.
- A missing PATCH target returns 404 Not Found.
- An idempotent DELETE whose exact read finds no entity returns 204 No Content.
- PUT against a missing target uses strict create; a concurrent create returns 409 and is not auto-reconciled.
- `commit_unknown` returns 503 Service Unavailable with an explicit uncertainty marker and correlation identifier.
- Unavailable, invalid, and internal outcomes retain their 503, 400, and 500 classes respectively.

The `commit_unknown` response instructs a client to verify state before deciding whether to retry. The gateway never
turns an uncertain commit into a second write.

### 3. Exact reads and batch reads preserve authoritative response shapes

Exact entity reads SHALL decode `graph.ExactEntity`, use its `Entity` value, and carry `KVRevision` into any guarded
mutation. A missing or zero revision cannot authorize reconcile or delete. Batch reads SHALL continue to consume
`entities[]` and SHALL account for beta.160's explicit `missing[]` member rather than inferring absence from position.

Graph query result-size failure remains distinct from transport timeout. In particular, `response_too_large` is not
classified as a transient timeout.

### 4. Projection contracts are local and resource-specific

The removed ownership package and projection binding SHALL be replaced with local `pkg/projection.Contract` intent.
Semconnect SHALL declare a contract for System, Datastream, Procedure, Deployment, SamplingFeature, Property,
ControlStream, Command, SystemEvent, Feasibility, and SchemaArtifact.

Each contract SHALL use a stable, valid, resource-specific message type. Entity type is birth-only. Mutable Systems
and Datastreams use one atomic `representation` reconcile group containing only predicates the endpoint owns.
Create-only resource predicates are birth-only. The Datastream contract accepts the existing federated six-part ID
surface through one six-token wildcard entity pattern while its message type keeps the resource family explicit.

Create facts are supplied only through `CreateMutation.Triples`; embedded `Entity.Triples` remains empty. Every
create triple subject SHALL equal the created entity ID. Missing relationship targets remain valid references and do
not create stubs.

### 5. SensorML embedded children are not independently projected

A SensorML request creates or reconciles only its primary root entity. Root-subject facts, including references from
the root to embedded children, may be retained when they satisfy the resource contract. Foreign-subject inverse
facts emitted for embedded children, including child `isHostedBy` facts, SHALL be dropped.

Creating embedded SensorML children as graph entities is out of scope because beta.160 does not provide a
multi-entity transaction for this request. A child posted separately as its own System may still carry its own
root-subject parent relationship.

### 6. Schema artifacts are immutable and use an exact storage provider

The canonical SWE schema bytes SHALL be hashed. The artifact entity ID and object key SHALL be content-addressed by
that digest. The immutable artifact entity is created with its final `StorageReference`; a create conflict triggers
an exact read and verification that the existing digest and reference are identical, never an artifact metadata
update.

The storage provider instance name is exactly `objectstore`. Its NATS ObjectStore bucket is exactly
`CS_API_ARTIFACTS`. Configuration and code SHALL keep the provider instance and bucket as separate concepts and
perform exact provider lookup without bucket-name, unnamed-provider, or fallback resolution.

Changing a Datastream schema reconciles the parent link to a new digest artifact. Orphan artifact garbage collection
is explicitly deferred; this migration neither deletes nor scans unreferenced artifacts.

### 7. Mutation audit is structured gateway evidence

Structured gateway audit is accepted; there is no durable requirement to propagate custom identity headers through
the typed graph mutation wire. After every mutation attempt, the gateway SHALL record request ID, trace ID, available
HTTP identity or trusted forwarded hints, operation, entity ID, commit state, KV revision, and classified error.

Observation publish headers remain a separate existing product contract. Tests and documentation SHALL remove any
claim that beta.160 graph mutations preserve custom forwarded headers on the internal request.

### 8. Ports and declarations use the closed beta.160 model

All SemStreams component definitions SHALL use canonical port envelopes with `name`, `required`, `description`, and
typed `config`. Flat port fields, kind aliases, service names, and custom `http` or `objectstore` port kinds are
removed. Port resolution errors are handled, not ignored.

The gateway declares the required typed mutation interface and typed graph query family. Direct admitted index or
spatial request paths use canonical NATS-request ports when retained. Observation I/O uses canonical JetStream
configuration with explicit stream name and subjects. Artifact storage is a configured provider dependency, not a
fabricated custom port. The standalone gateway's directly composed HTTP routes are not internal flow ports and do
not need a synthetic HTTP input port.

Graph ingest SHALL resolve exactly one typed mutation input provider. Top-level configuration versions SHALL be
regenerated and bumped as required by the beta.160 schema. Factory admission is not applicable to the current direct
standalone composition; using a registry later reopens that decision.

### 9. NATS aligns to the supported beta.160 proof

Semconnect SHALL align development, conformance, and deployment configuration to NATS 2.14. The qualification
evidence SHALL record the exact image digest and server version used; a floating major/minor image is not sufficient
release evidence.

## External behavior and verification

No CS API conformance class, route, media type, fixture intent, or ordinary success representation is intentionally
removed. The visible concurrency 409 and uncertain-commit 503 marker are deliberate correctness behavior and SHALL
be reflected in OpenAPI and tests.

Release requires exact dependency proof, generated configuration validation, failing-first unit and real-NATS tests,
full Go test/race/vet/build/module verification, clean-volume restart parity, and an unchanged external result of
`137 passed, 0 failed, 0 skipped`. Independent review SHALL confirm that no test, fixture, OpenAPI path, or declared
conformance class was weakened to obtain the result.

Trajectories, tool discovery, agent-run milestones, metric server changes, GraphQL similarity search, aggregate graph
clients, component status, and context or structural persistence are not used by semconnect and are recorded as not
applicable, not silently omitted.

## Consequences

- Concurrent writers receive deterministic conflicts instead of silent last-write-wins behavior.
- PATCH and DELETE gain one exact-read round trip and carry the revision into the mutation.
- Embedded SensorML child inverse facts no longer appear accidentally; independently managed children remain valid.
- Content-addressed schema artifacts can accumulate until a separately designed garbage collector is approved.
- Internal graph mutation audit moves from assumed wire-header propagation to explicit gateway records.
- Beta.160 adoption needs newly provisioned storage and an operational rollback environment, not an in-place upgrade.

## Non-goals

- No beta.159 state migration, compatibility shim, raw mutation alias, dual format, or mixed-version reader.
- No automatic retry for revision conflict or uncertain commit.
- No embedded SensorML multi-entity transaction.
- No artifact orphan garbage collector.
- No new trajectory, tool, agent-run, metric, GraphQL, aggregate-client, or persistence feature.

## Architect sign-off

The architecture is approved for TDD implementation on 2026-08-12. The binding execution detail is the associated
OpenSpec change. Any compatibility path, retained-volume support, hidden mutation retry, alternate storage lookup, or
public conformance reduction reopens architecture review.

## Qualification closeout

Implementation, independent Go review, frontend N/A review, fresh beta.160
startup, full same-volume no-write restart parity, and external `137/0/0`
conformance pass. The restart proof covers the System, schema-backed Datastream,
schema endpoint, and global and scoped Observation reads at graph readiness
3/3. The strict SensorML bake proves the root entity readable and its embedded
child absent. Exact evidence is archived under the associated OpenSpec change.

The initial closeout decision was **NO-GO**. The operator's local Docker inventory contained no preserved beta.159
rollback deployment and volume for cross-version isolation proof, and product-owner authorization was then
outstanding. That result did not invalidate the beta.160 implementation or fresh-volume evidence. The risk-acceptance
amendment below supersedes only that production decision; it does not convert missing rollback proof into completed
evidence.

## Production risk-acceptance amendment

On 2026-08-12, after reviewing the completed beta.160 technical evidence, the product owner authorized proceeding:
"roll back would have been nice but we have evidence that our 160 work is solid so let's proceed".

Production is therefore **GO WITH ACCEPTED RISK**. OpenSpec task 9.5 remains unproven and SHALL be recorded as waived
by explicit product-owner risk acceptance, never marked complete. There is no demonstrated beta.159 rollback
deployment or volume. Recovery planning SHALL assume forward repair or reseed into fresh beta.160 state, not binary
rollback onto beta.160 storage.

The accepted technical basis is the independent Go approval, frontend N/A approval, unchanged external
`137 passed, 0 failed, 0 skipped`, no-weakening review, and authoritative r5 persistence proof. The r5 archive records
an empty first start, active graph readiness at target/indexed revision `3/3` before and after restart, clean process
shutdown, and byte-identical System, schema-backed Datastream, schema, and global and scoped Observation results with
canonical proof SHA-256 `4544c19899ccf1c9b461146be542c47dd85d4201aad5aeda2c9cf48946b63e32`.

The production GO is conditional on these guardrails:

- Provision a uniquely named, empty beta.160 NATS volume. Verify zero streams, consumers, messages, and bytes before
  first write. Any retained state, ambiguous volume identity, or beta.159 mount aborts the deployment.
- Never attach beta.159 state to beta.160 or beta.160 state to beta.159. A beta.159 volume discovered later is isolated
  for separate owner-reviewed handling, not used as a fallback target.
- Preserve the complete `evidence/persistence-r5/` archive and its recorded hashes without overwrite. Before production
  writes, capture and verify recoverable backups of authoritative reseed inputs, rendered configuration, deployment
  metadata, and any product data that is not disposable. A restore SHALL target newly provisioned beta.160 storage.
- During cutover and for at least 60 minutes after traffic begins, actively poll authoritative health, graph target and
  indexed revisions, critical CS API read/write probes, JetStream observations, and schema artifact reads every
  30--60 seconds. Logs supplement these checks; silence is not success.
- Stop new writes and remove traffic immediately on a nonempty preflight, version or image mismatch, failed canonical
  probe, storage error, revision regression, or lack of forward index progress for more than twice the expected step
  duration. Do not attempt cross-version recovery. Preserve incident evidence and choose forward repair or a clean
  beta.160 rebuild from the verified authoritative backup.

This amendment accepts reduced recovery optionality; it does not weaken the fresh-state rule, the r5 evidence, any
technical or conformance gate, or the prohibition on cross-version state access.
