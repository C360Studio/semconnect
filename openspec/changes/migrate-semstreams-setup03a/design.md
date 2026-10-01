# SETUP 03A migration contract

## Status and handoff

Architect contract sign-off: **APPROVED for implementation and reference tests**, 2026-10-01. This approves the
bounded design below, not target qualification or release. The baseline was archived before dependencies changed.
Independent Go review now approves the scoped implementation and comparison evidence at
`2d45b651b3bfac8ad4f96bceab7f5a0b3cf225c1`. Full migration and merge approval remain withheld for the
federated Datastream regression. See [qualification](qualification.md) and [review](review.md). The approved pin
and owner ruling are in [proposal.md](proposal.md).

## Product boundary

SemConnect owns OGC HTTP semantics, OpenAPI and conformance claims, the ETS qualification workload, local
SensorML/OMS/SWE representations, and vocabulary mappings. SemStreams supplies graph, messaging, storage, and
lifecycle primitives. Reference cases communicate the consumer contract to SemEngine; they do not transfer
conformance ownership or admit SemEngine into this binary. A future framework switch requires a separate proposal,
contract qualification, dependency closure, and reviewer approval.

## Preserved contracts

| Contract | Required outcome |
| --- | --- |
| OGC surface | Preserve discovery, negotiation, IDs, encodings, status codes and links. |
| Exact reads | Preserve authoritative entity plus nonzero KV revision; batch missing IDs stay explicit. |
| Create | One typed create; facts belong only to the requested subject; duplicate creates conflict. |
| Reconcile | Use precisely the revision from the read that produced desired state; never reread/retry secretly. |
| Delete | Exact revision; missing is idempotent; observation purge failure remains explicit. |
| Failure classification | Conflict maps to 409; uncertainty to 503 with correlation/header. |
| Root SensorML | Preserve root-to-child facts, drop foreign-subject inverse facts, and create no embedded children. |
| Missing relationship target | Reference is legal and does not synthesize a stub or infer target existence. |
| Schema artifacts | Immutable digest bytes; conflict verifies bytes and exact storage reference. |
| Observation delivery | Preserve envelope, subject, publish ack, audit/trace, cursor and purge scope. |
| Spatial | Preserve bbox/polygon query behavior, result geometry, and graph-index readiness. |
| Lifecycle | Controlled stop drains/joins under the caller's bounded authority before cancelling runtime ownership. |
| Persistence | Isolated fresh storage; normal no-write restart; revision, bytes and delivery parity. |

## Required compatibility decisions

### Register consumer resource types in the graph host

At the frozen target, [upstream ADR-103][adr103] makes the payload registry the type authority. Typed create now
rejects unregistered message types. The [upstream SemConnect migration instruction][migration] explicitly requires
registration for all eleven `c360.csapi-*.v1` types and a SemConnect-owned host composition root. Continuing to run
the unmodified stock `cmd/semstreams` backend cannot satisfy this contract.

Export a registration function from `gateway/cs-api`. Each type has a round-trippable Graphable payload, its existing
`content` indexing floor, and its local projection contract. Contract `MessageType` fields carry the structured type,
not the dotted key. The host creates one registry, registers builtins and the consumer resources, and injects it into
graph-ingest. Both binaries load the same gateway configuration so custom ID prefixes bind the same contracts.
Registration rejection remains fail-closed; there is no permissive unknown-type mode.

Create a narrow `cmd/cs-graph-backend` composition root for the same configured graph-ingest, graph-index,
graph-index-spatial, graph-index-temporal, and graph-query processors. Retain the ObjectStore provider needed by
schema storage references. Use public component/config/service lifecycle APIs and explicit component registrations.
Avoid importing the full component registry merely to recover boot convenience. The payload builtin registry is an
explicit measured dependency; narrowing it further is a separate extraction decision, not assumed in these results.

The graph host and HTTP gateway stay on the same exact SemStreams revision. This is a consumer composition change;
it does not move OGC handlers into the generic framework.

### Honor context ownership at shutdown

[Upstream ADR-095][adr095] and the frozen `component/lifecycle.go` replace `Stop(time.Duration)` with
`Stop(context.Context)`. The exact stop context bounds admission fencing, drain, join, and cleanup. Keep the accepted
Start context live during controlled shutdown; cancelling it first selects abort behavior. Reject nil contexts before
action and retain completed-stop no-op behavior. Do not invent detached cleanup or promise rejoin after a stop
deadline.

The implemented CLI bridges shutdown cancellation only during incomplete startup, then detaches and joins the
callback before readiness. This preserves signal authority while stream/ObjectStore provisioning blocks, without
cancelling admitted work before controlled Stop. A signal racing successful resource acquisition triggers bounded
Stop before returning and closing NATS. Independent review found and verified this HTTP startup repair. The frozen
configuration manager still accepts a duration; the backend passes its remaining caller-bounded shutdown budget.

### Keep entity strings stable while adopting identity semantics

[Upstream ADR-102][adr102] defines segment order `org.platform.system.domain.type.instance`. Existing externally
visible SemConnect IDs are six-token opaque resource identifiers and must not be silently renamed. Test parsing,
validation, absent-target references, and spatial/index outputs against existing fixtures. Platform authority and
configuration-bucket changes under ADR-104 require fresh target storage and a stable declared deployment stem; any
config or derived identity difference must be recorded against that upstream decision.

The implemented one-shot `-provision-identity semconnect` atomically creates the identity after empty-storage
preflight. It accepts an identical existing value and refuses to overwrite a different one. This explicit opt-out
from automatic suffix creation preserves the deployment identity; it does not bypass the mutation fence.
The target still rejects the foreign Datastream ID that beta.160 accepts. The expected HTTP 201 remains active,
and this reproduced ADR-102/104 consumer incompatibility blocks qualification.

### Keep semantic-web mappings at the export boundary

[Upstream ADR-107][adr107] canonicalizes declaration datatypes to neutral spellings, while per-triple RDF-style
markers retain their meaning. Use `entity_id` in predicate declarations where the target requires it; retain the
relationship marker in triples and the correct IRI mapping in JSON-LD/Turtle. A metadata spelling difference is an
intended upstream change only with an explicit golden/attribution update; a changed relationship or OGC field is a
defect.

## Reference-case handoff

The executable fixtures are owned by SemConnect under `gateway/cs-api/testdata/setup03a/` and the adjacent
`setup03a_reference*_test.go` tests. Keep the handoff small enough to run without the external ETS stack.

1. **SensorML and vocabulary:** one root System with a hosted absent child; expected root triples, original UID,
   type, relationship datatype, standard IRI and inverse mappings. Foreign-subject facts never reach create.
   This compact fixture contains no position; separate bbox/polygon and restart evidence covers spatial behavior.
2. **Typed mutation:** create, exact read, reconcile, stale-revision rejection, and exact delete on real NATS; assert
   nonzero and advancing revisions, absent target remains absent, and a request counter proves one mutation attempt.
   Include controlled conflict and uncertain-commit injection with unchanged HTTP classification.
3. **Immutable artifact:** canonical schema bytes and digest; typed artifact entity with final `StorageReference`
   (`objectstore`, digest key, content type and byte size); the provider owns the configured bucket. Assert
   byte-for-byte lookup, duplicate integrity verification, and changed
   schema linking a new value. Never reconcile an existing artifact to hide a mismatch.

No reference case claims that SemEngine already implements the contract. Its extraction owner can reuse these
expectations, attribute differences, and report admission gaps without changing SemConnect's product semantics.

## Qualification evidence

Run the same consumer workload at both pins. Keep NATS exactly 2.14.4 unless an observed upstream requirement forces
an explicitly documented change, in which case isolate and attribute the NATS difference separately. Record actual
images, commits, versions, volume identities, timestamps, readiness revisions, and response bytes.

Final baseline and target r3 evidence both record exactly 137 passed, 0 failed, 0 skipped at the existing ETS commit.
Normal persistence and a second spatial restart pass on separate fresh storage; manifests show zero source drift.
The full Go integration suite fails only the active foreign Datastream assertion. No external CI run is inferred
from local evidence; the PR conformance job was skipped by its existing label trigger.

The external gate remains exactly 137 passed, 0 failed, 0 skipped at the existing ETS commit. Preserve assertions,
filters, fixture intent, claims, parser behavior, and OAS semantics. Poll authoritative readiness and test state while
long-running operations execute; log silence does not prove progress. Fresh storage is mandatory at both revisions,
with a normal stop/no-write restart on each revision's own volume. Never open another revision's volume.

[adr103]: https://github.com/C360Studio/semstreams/blob/8b99efe9c66a4faa4fa509f9f62cc6bad8392128/docs/adr/103-payload-registry-is-the-single-type-authority.md
[adr095]: https://github.com/C360Studio/semstreams/blob/8b99efe9c66a4faa4fa509f9f62cc6bad8392128/docs/adr/095-one-shot-running-lifecycle-with-retained-failed-start-cleanup-authority.md
[adr102]: https://github.com/C360Studio/semstreams/blob/8b99efe9c66a4faa4fa509f9f62cc6bad8392128/docs/adr/102-entity-id-segment-semantics.md
[adr107]: https://github.com/C360Studio/semstreams/blob/8b99efe9c66a4faa4fa509f9f62cc6bad8392128/docs/adr/107-semantic-web-vocabulary-lives-at-the-export-edge.md
[migration]: https://github.com/C360Studio/semstreams/blob/8b99efe9c66a4faa4fa509f9f62cc6bad8392128/docs/operations/migration-beta162-to-beta163.md#semconnect--pinned-d0d06e0
