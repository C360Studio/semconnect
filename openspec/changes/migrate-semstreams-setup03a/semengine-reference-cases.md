# SemEngine consumer reference handoff

## Scope and status

SemConnect supplies three compact executable cases for SETUP 03A extraction planning. The consumer owns their
OGC meaning, mapping decisions and qualification. SemEngine implements its own future adapter; these cases are
not permission to add it to this SemStreams binary or to move conformance ownership.

Handoff destination: [SemEngine issue 8](https://github.com/C360Studio/semengine/issues/8).
Posted comment URL: **pending**. This document is ready for the authorized coordination comment.
Reviewed SemConnect implementation: `2d45b651b3bfac8ad4f96bceab7f5a0b3cf225c1`.
Baseline checkpoint: `55ea4121aba8658aeac9d70bdfe80f19c15fd4cd`.

- Baseline SemStreams: `v1.0.0-beta.160`, `8403a2218000e45a31c5132fbfe01af42ed04f14`.
- Frozen target: `v1.0.0-beta.162.0.20260930150212-8b99efe9c66a`,
  `8b99efe9c66a4faa4fa509f9f62cc6bad8392128`, tree `605f83cb8492eda3bd347ac30a211b9babf3931f`.
- All three synchronized cases pass at both pins. Full migration is **DRAFT / UNQUALIFIED** because the separate
  foreign Datastream expected-201 assertion fails at the target with HTTP 400 `authority_foreign`.

## Run the cases

From the SemConnect checkout, with Go and localhost listener permission:

```bash
go test -race -tags=integration ./gateway/cs-api -run '^TestSetup03AReference' -count=1
```

The cases use fresh embedded NATS, real graph-ingest and ObjectStore where applicable. They do not need the
external ETS stack. JSON expectations and small inputs are under `gateway/cs-api/testdata/setup03a/`;
`setup03a_reference_test.go` and `setup03a_reference_integration_test.go` are executable specifications.
[Paired JSONL and source hash](evidence/reference-synchronized/summary.json) preserve synchronized reruns.

## 1. SensorML triples and vocabulary mappings

`TestSetup03AReferenceSensorML` reads `system.json` and `expectations.json`. A PhysicalSystem with UID
`setup03a-root` yields root type, label, original UID and a hosted-child reference. Every triple subject is the
root; the child-side inverse is absent. The child target is deliberately not materialized.

Assertions pin standard RDF/DC/SOSA/CS API IRIs, inverse predicate mapping, and relationship datatype.
Target declaration metadata uses neutral `entity_id` under ADR-107; the actual relationship triple retains `@id`.
The paired expectation delta is attributed in `evidence/target-red/attribution.json`.
This compact fixture contains no position. Separate real bbox/polygon and restart evidence qualifies spatial behavior.

## 2. Typed exact-revision mutations and absent target

`TestSetup03AReferenceTypedMutations` creates the projected root through the real typed adapter, reads its nonzero
revision, reconciles once using that exact revision, rejects stale reconcile and delete, then deletes using the
current exact revision. A duplicate create conflicts. Stale operations leave authority unchanged; the child remains
absent throughout. No relationship target stub, reread or hidden retry is introduced.

Each create/reconcile/delete also receives controlled conflict and lost-reply injection over real NATS. The reply
handler signals receipt before the test cancels the request; a watchdog only bounds deadlock. Assertions retain
one request, HTTP 409 for conflict, and HTTP 503 plus uncertainty and correlation headers for lost reply.
This tests consumer ambiguity handling; it does not claim to induce upstream durable-storage ambiguity.

## 3. Immutable bytes and graph StorageReference

`TestSetup03AReferenceImmutableArtifact` uses `schema.json`, canonical SWE JSON bytes and a SHA-256 artifact ID/key.
The graph artifact carries a final `StorageReference`: `storage_instance=objectstore`, digest JSON key,
`content_type=application/json`, and exact canonical byte size. The configured provider owns bucket
`CS_API_ARTIFACTS`; bucket identity is composition metadata, not an extra StorageReference field.

The test verifies exact graph revision and bytes through actual graph/ObjectStore access. Equivalent canonical
inputs reuse the same identity without rewriting object metadata or graph revision. A changed unit produces a
new artifact; original bytes remain unchanged. Duplicate-create integrity checks do not reconcile the artifact.

## Blocking gap and extraction closure

The independent federation case remains active in the full integration suite:

```bash
go test -race -tags=integration ./gateway/cs-api -run '^TestSetup03AFederatedDatastreamCreate$' -count=1
```

It expects HTTP 201 for `foreign.remote.systems.csapi.datastream.setup03a-reference` under explicit
`c360.semconnect` authority. Beta.160 passes; the target returns 400 `authority_foreign`. The intended ADR-102/104
local-authority fence conflicts with the existing consumer contract. [Paired
reproduction](evidence/federation-gap/summary.json)
is the admission gap; no authority bypass, weakened assertion or independently newer pin is supplied.

The full combined framework closure is 63 production/99 including retained tests at baseline and 67/113 at target.
The stable bounded backend alone is 61/97 to 63/110; actual consumer scope is 39/55 to 67/67. These scopes differ
intentionally. [Inventory and dispositions](inventory.md) identify added broad service/builtin transitive packages.
Carry, adapt, repair or exclusion decisions belong to a separate SemEngine extraction contract, not automatic
admission.

The unchanged external OGC suite remains SemConnect-owned and passes 137/0/0 at both pins. Final fresh-storage
persistence and spatial restarts pass. Scoped independent implementation review is approved; full migration and
merge qualification remain withheld solely for federation. [Qualification](qualification.md) records limits and
evidence.
