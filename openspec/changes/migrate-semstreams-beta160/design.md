## Context

Semconnect's beta.159 gateway creates, updates, and deletes graph entities through removed raw subjects. It reads
exact entities into the old bare state shape, binds ownership projections, declares flat and custom component ports,
and updates deterministic schema artifact entities. Beta.160 replaces each of those contracts and requires fresh
storage for adoption.

ADR-S004 records the durable decisions. This design turns them into implementation boundaries and evidence gates.
The stakeholders are the architect, Go developer and reviewer, technical writer, operator, and product owner.
Frontend work is not applicable unless generated public types or UI behavior actually change.

## Goals / Non-Goals

**Goals:**

- Adopt the exact beta.160 release and its closed declarations on NATS 2.14.
- Preserve atomic read-merge-write semantics with exact revision-fenced reconcile and delete.
- Express each CS API graph producer through a local resource projection contract.
- Preserve external CS API behavior while making concurrency and uncertain-commit outcomes honest.
- Prove greenfield startup, same-version persistence, and unchanged external conformance.

**Non-Goals:**

- In-place beta.159 storage migration, compatibility aliases, mixed revisions, or an old-state reader.
- Automatic write retries, embedded-child transactions, or artifact orphan garbage collection.
- Features from beta.160 surfaces that the bounded repository audit classifies as not applicable.

## Decisions

### Put a domain interface in front of beta.160 graph operations

`Component` receives a semconnect-owned interface expressed as fetch, batch fetch, create, reconcile, and delete
operations. Handler tests keep focused fakes. The production adapter uses `graph.ExactEntityReader` and the typed
mutation request family. No handler constructs a raw mutation subject.

Create may use the framework projection mutation client when it preserves strict single-attempt behavior. PATCH and
full replacement use a local revision-fenced adapter because desired state is computed from a prior exact read. That
adapter serializes the exported reconcile request and derives `entity.reconcile` from the configured typed mutation
family. Delete similarly carries the exact read revision.

The adapter returns domain outcomes, including not found, conflict, revision mismatch, commit unknown, invalid,
unavailable, and internal. HTTP mapping stays at the gateway boundary. `commit_unknown` includes an uncertainty header
and correlation value but never triggers an internal retry.

### Treat exact read revision as part of entity state

Fetch returns both the entity state and nonzero KV revision. Any reconcile candidate is computed from that fetched
state and submitted with that same revision. A second read inside the mutation path would create a race and is
forbidden. PUT-missing uses create, PATCH-missing fails, and DELETE-missing succeeds idempotently.

Batch hydration maps returned entities by ID and records explicit missing IDs. Callers do not assume output order or
manufacture empty entities. Query `response_too_large` remains a result failure rather than a timeout.

### Declare one projection contract per resource family

Local contracts cover System, Datastream, Procedure, Deployment, SamplingFeature, Property, ControlStream, Command,
SystemEvent, Feasibility, and SchemaArtifact. Each has a stable valid message type, expected entity pattern, type as a
birth predicate, and an indexing profile.

System and Datastream put all endpoint-owned mutable predicates in one atomic `representation` group. Create-only
resources declare their emitted predicates birth-only. Datastream keeps its client-supplied federation contract by
using a six-token wildcard entity pattern, with the family-specific message type preventing an anonymous mutation.
Contract validation tests enumerate every emitted predicate and all accepted local and federated IDs.

Typed create requests hold facts only in `CreateMutation.Triples`; the embedded entity fact list is empty. All create
subjects equal the target ID. References to missing targets remain legal and never synthesize entity stubs.

### Project only the requested SensorML entity

The SensorML builder may reconstruct multiple logical subjects from an embedded document. Before mutation, the
gateway retains only facts whose subject is the primary resource ID. Root-to-child references can remain. Inverse
facts whose subject is an embedded child are dropped.

This avoids pretending that one HTTP request is an atomic multi-entity transaction. A child remains independently
creatable through its own System request, including a root-subject parent relation.

### Make schema artifacts immutable content-addressed values

Canonical SWE schema bytes produce the digest used by both artifact entity ID and ObjectStore key. The artifact is
created once with a final exact storage reference. On create conflict, an exact read verifies matching type, digest,
storage instance, bucket, and key. Any mismatch is an integrity error; the artifact is never reconciled.

The provider instance is `objectstore`; the physical bucket is `CS_API_ARTIFACTS`. The ObjectStore component registers
that exact provider instance over that bucket, and CS API configuration names each separately. Parent Datastream
reconcile changes only its link to a new artifact. Orphan cleanup is deferred.

### Record mutation audit at the gateway boundary

Every attempt emits one structured record containing request and trace correlation, available trusted HTTP identity
hints, operation, entity ID, result or commit state, observed KV revision, and classified error. The typed mutation
wire has no product requirement to carry custom forwarded headers. Existing observation publish headers remain
unchanged because they are a different product path.

### Regenerate declarations instead of translating old ports

Gateway definitions use canonical envelopes and declare a required typed mutation interface plus typed graph query
requests. Direct admitted predicate-index and spatial queries use canonical NATS request ports if they remain direct.
Observation input and output use canonical JetStream stream and subject fields. Artifact storage is resolved through
the configured registry, not represented as a custom port.

Directly composed standalone HTTP routes do not declare a synthetic component flow port. Backend graph ingest has
exactly one typed mutation input provider. Removed aliases, service names, flat fields, and ignored resolution errors
are not translated. Configuration schema generation and version bumps are part of the compile gate.

### Use fresh storage and an isolated rollback environment

Conformance already removes its disposable volumes and SHALL continue to prove beta.160 from empty state. Deployment
qualification allocates a new beta.160 volume, seeds it, captures authoritative and indexed revisions, stops normally,
restarts without a write, and proves equal or later readiness and query parity.

Any detected retained production state is a stop condition, not an invitation to wipe it. Rollback routes users to a
separate preserved beta.159 service and volume. No binary is pointed across the version boundary.

### Align NATS and preserve public conformance

All development, conformance, and deployment proofs align on NATS 2.14 and record the exact image digest and server
version. The SemStreams module, backend, tests, Compose, and evidence identify the same beta.160 release.

The public wire surface changes only where correctness requires an explicit 409 concurrency response or an uncertain
commit 503 marker. OpenAPI and golden tests cover those cases. External release evidence remains exactly 137 passed,
0 failed, and 0 skipped, with an independent no-weakening diff review.

## Risks / Trade-offs

- [A stale revision overwrites a concurrent update] -> Reconcile and delete require the exact fetched revision.
- [A helper hides a second mutation] -> Adapter tests count typed requests and reject retries for every outcome.
- [A copied subject drifts from declarations] -> Derive operation subjects from the resolved typed family.
- [A missing batch result becomes a zero entity] -> Consume explicit `missing[]` and map results by ID.
- [Foreign SensorML facts violate create ownership] -> Filter by root subject before contract validation.
- [An artifact conflict hides corruption] -> Exact-read and compare immutable digest and storage reference.
- [Provider name and bucket are conflated] -> Configure and test `objectstore` separately from `CS_API_ARTIFACTS`.
- [Audit identity is lost with headers] -> Emit structured attempt records at the authenticated HTTP boundary.
- [A beta.159 volume is opened accidentally] -> Require a new volume identity and preserve rollback separately.
- [A green ETS conceals weakened scope] -> Review fixture, claim, OpenAPI, and test diffs with the raw result.

## Migration Plan

1. Write failing adapter, concurrency, projection, SensorML, artifact, audit, and declaration tests on beta.159.
2. Align the Go toolchain, SemStreams beta.160 module, `nats.go`, and NATS 2.14 proof pins together.
3. Add the domain adapter, exact read shape, revision-fenced operations, and HTTP classifications.
4. Replace ownership bindings with validated resource contracts and root-only SensorML create facts.
5. Make schema artifacts immutable and wire exact provider instance and bucket configuration.
6. Regenerate canonical ports and deployment/conformance configuration; remove old declarations.
7. Run full Go, real-NATS, clean-volume startup/restart, and external conformance gates.
8. Obtain Go review, N/A frontend review, documentation handoff, and final architect/product-owner release approval.

## Open Questions

No architecture questions remain. Exact production volume names, image digest, deployment owner, rollback endpoint,
and qualification schedule are operational evidence values and block production adoption until supplied.
