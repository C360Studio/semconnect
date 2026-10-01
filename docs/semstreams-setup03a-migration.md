# SemConnect SETUP 03A SemStreams migration

## Current decision

The migration is **DRAFT / UNQUALIFIED**. Independent Go review approves the scoped implementation and comparison
evidence, while full migration and merge approval remain withheld. The target rejects a federated Datastream ID
that SemConnect already accepts; the expected HTTP 201 remains an active assertion.

The reviewed implementation is `2d45b651b3bfac8ad4f96bceab7f5a0b3cf225c1`.
Full [qualification](../openspec/changes/migrate-semstreams-setup03a/qualification.md) and
[independent review](../openspec/changes/migrate-semstreams-setup03a/review.md) provide evidence and scope.

## Frozen baseline and target

- Pre-change checkpoint: `55ea4121aba8658aeac9d70bdfe80f19c15fd4cd`.
- Baseline dependency: `v1.0.0-beta.160`, commit `8403a2218000e45a31c5132fbfe01af42ed04f14`,
  tree `9ed5dd3792bca63ce87ebf449a180add918f59ed`.
- Shared target: `v1.0.0-beta.162.0.20260930150212-8b99efe9c66a`,
  commit `8b99efe9c66a4faa4fa509f9f62cc6bad8392128`,
  tree `605f83cb8492eda3bd347ac30a211b9babf3931f`.

SemEngine SETUP 03A selected this main commit because beta.163 was unavailable at freeze. SemConnect reuses it;
a later release or main revision is not selected independently. Both binaries stay on SemStreams. A future
SemEngine switch requires a separate qualified contract and must not mix both frameworks in one binary.

## Compatibility work

The consumer-owned `cmd/cs-graph-backend` registers eleven graph payload types, their local contracts and content
indexing floors under ADR-103. It composes explicit graph/index/query/ObjectStore factories using public lifecycle
APIs and receives the same gateway configuration, including custom prefixes. Contract message types are structured.

ADR-095 changes Stop to the exact caller context. Runtime authority stays live through controlled drain and join;
startup-only signal bridges still abort incomplete startup. A signal racing successful startup triggers bounded
cleanup before NATS closes. ADR-107 changes declaration names to `entity_id`, `bool` and `float`; triple `@id`,
standard vocabulary IRIs, root-only SensorML facts and OGC representations remain intact.

Explicit create-only platform provisioning opts out of automatic suffix creation and preserves
`c360.semconnect.*`. It refuses a different existing identity. This preserves deployment identity without weakening
the target's mutation-authority checks. Fresh-storage preflight runs before provisioning; ordinary restart adopts
only that volume's identity. [Deployment instructions](../deploy/README.md) retain the operator details.

Exact-revision create/reconcile/delete, conflict and uncertainty classification, one mutation attempt, immutable
schema bytes, observation delivery and spatial query behavior remain protected. OGC semantics, fixtures, OpenAPI,
conformance assertions and SensorML/OMS/SWE mappings stay in SemConnect.

## Results and blocker

- Final local ETS: 137 passed, zero failed, zero skipped at both pins; unchanged assertions and fixtures.
- Unit/race, vet and build pass. Full integration/race fails only the preserved federated Datastream assertion.
- Three receipt-synchronized consumer references pass on both pins, using real graph/NATS/ObjectStore boundaries.
- Isolated fresh-storage normal restart preserves graph, schema and observation proof bytes. Bbox/polygon responses
  also match after a second normal restart. Source manifests show zero drift during final r3 qualification.
- Baseline volume `semconnect-03a-beta160-20261001-fresh` and target
  `semconnect-03a-8b99efe9-20261001-r3-fresh` remain separate. Never cross-open them.

The blocker is reproducible: client-supplied ID
`foreign.remote.systems.csapi.datastream.setup03a-reference` yields HTTP 201 at beta.160 and HTTP 400 with graph
reason `authority_foreign` at the target. Intended upstream ADR-102/104 fencing explains the difference but does
not satisfy the existing consumer contract. There is no waiver, narrowed assertion, hidden retry or authority bypass.

Optional clustering warnings have identical meanings at both pins. The target's nine composition warning shapes
reflect cross-process consumers and optional outputs; actual readiness and request results qualify those paths.
No production readiness claim relies on quiet logs. The PR's conformance CI job is skipped by its existing label
trigger; the local ETS evidence is the actual executed 137-case proof.

## Coverage and extraction planning

The explicit critical groups achieve 85.19–100% statement coverage. Individual functions and package limits remain
visible: `readSchemaArtifact` is 70.59%; whole gateway/backend/HTTP CLI packages are 74.3%/65.0%/21.5%.
Docker entrypoint execution does not count toward Go instrumented coverage.

Combined framework production/retained-test closure grows 63/99 to 67/113. The stable bounded backend grows
61/97 to 63/110; actual consumer scope grows 39/55 to 67/67. Broad service and builtin-registry dependencies are
measured extraction obligations, not automatic SemEngine admission. See the
[dependency inventory](../openspec/changes/migrate-semstreams-setup03a/inventory.md).

The [three-case SemEngine handoff](../openspec/changes/migrate-semstreams-setup03a/semengine-reference-cases.md)
contains executable commands, fixtures, exact pins and the unresolved contract gap.
The [issue-8 handoff](https://github.com/C360Studio/semengine/issues/8#issuecomment-5929518484) is delivered.
Resolving federation and requalifying affected gates precede any merge approval.
