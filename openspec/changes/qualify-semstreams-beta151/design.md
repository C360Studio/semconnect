## Context

The user requested "515", but neither the local release set nor an exact GitHub remote tag query contains
`v1.0.0-beta.515`. On 2026-07-18 the latest remote release is `v1.0.0-beta.151`. This qualification therefore targets
beta.151 and records the resolution as evidence.

Release provenance is exact:

- beta.149 baseline: tag object `7c0c5aae9c02d148e118627b18281f34d9adf2f8`, peeled
  `7db0cdcb21577eaa52eb842c4ffb06a854f9a9b2`;
- beta.150: tag object `cb42ac3d9743134f1a9194fba2824424833669f7`, peeled
  `d058bddfae0326487f0a86023ffe3d155992fd87`;
- beta.151: tag object `784f22dc8d549d7781b88a2878bb679112aad494`, peeled
  `ac75c322140fb2a6b55759d07a79874b4cb4d9cc`.

## Complete upstream delta

| Commit | Change | Semconnect disposition |
|---|---|---|
| `d058bddf` (#554) | Structural gate/canonical predicates | Live entity create/update; qualify |
| `fa8e90ed` (#556) | Archive/sync structural OpenSpec | Contract evidence; no executable delta |
| `58893068` (#558) | Synchronize graph-index/rule tests | Test-only; relevant upstream proof |
| `cb74a6de` (#559) | Fix rule test watcher race | Test-only; rule is not configured |
| `ce130e34` (#561) | Detach rule persistence from shutdown | Rule absent; run upstream regression |
| `31f16c98` (#563) | Graph-retention adversarial review document | Documentation only |
| `c5b29a3b` (#565) | Increase graph-index CI smoke timing headroom | Test-only; no runtime contract change |
| `38697908` (#564) | Shared agent skills/adapters | Tooling only; not runtime |
| `ac75c322` (#567) | Trusted owner-RMW decode | Live entity updates/merges; qualify |

PR #554 also repairs noncanonical internal predicates in agentic, research, rule, clustering, and example producers.
Those producers are absent from semconnect's runtime configuration, but the shared graph-ingest enforcement is live.

## Exposure and contract analysis

Semconnect's conformance backend enables graph-ingest, graph-index, spatial/temporal indexes, and graph-query. It does
not enable rule, agentic-loop, agentic-tools, research-graph, or other changed producers. The graph-ingest input is
bound to `_semconnect.unused.ingest`; product writes use NATS request/reply subjects:

- `graph.mutation.entity.create_with_triples`;
- `graph.mutation.entity.update_with_triples`;
- `graph.mutation.entity.delete`.

Create and update are directly subject to PR #554. Delete has no candidate triples. Semconnect already validates final
entity state with `graph.MarshalEntityState` before NATS and uses six-part IDs, three-part predicates, canonical entity
references, and registered foreign-edge claims. Direct NATS probes are still required because local validation alone
cannot prove the backend choke point.

The structural gate does not change the canonical graph or foreign-edge contract. It makes malformed candidates fail
closed before persistence and leaves valid normalization/claim routing intact. Consequently there is no graph-layout
migration, dual-read period, or legacy predicate mode. The qualification exercises malformed direct mutations and the
positive foreign-edge bake. Production is greenfield-only: the target NATS namespace must be empty, so no retained
graph is inspected, repaired, translated, wiped, or accepted as a compatibility input.

PR #567 uses a non-validating decoder only for graph-ingest's owner RMW reads, then validates every committed candidate
through `MarshalEntityState`. External readers retain authoritative validation. The critical proof is that resident
poison cannot be merged into a new valid-looking state: a write must fail with the reset-required classification and
must not change bytes or revision. Canonical RMW behavior and true remove no-ops must also remain correct.

## Decisions

### Pin beta.151 exactly and reject the nonexistent beta.515 label

The Go requirement SHALL be `github.com/c360studio/semstreams v1.0.0-beta.151`. The conformance source SHALL use
commit `ac75c322140fb2a6b55759d07a79874b4cb4d9cc` and record tag `v1.0.0-beta.151`. Module, tag, commit, source tree,
image digest, and manifest must agree. A later beta.515 publication would be a new request, not silent retargeting.

### Preserve the no-legacy policy

No compatibility alias, relaxed parser, dual read/write, predicate rewrite, or cleanup lane is allowed. A structurally
invalid candidate fails qualification. Pre-existing target production state fails the greenfield preflight and
requires a separate architecture decision; the beta.151 bundle has no destructive or migration mode.

### Use TDD at both caller and persistence boundaries

Before pin changes, tests SHALL fail against beta.149 for the beta.151 identity and proof surface. Implementation then
updates pins only. Required structural tests cover valid create/update, invalid predicate, invalid ID/reference,
atomic rejection, unchanged bytes/revision, canonical foreign-edge routing, and rejection observability. Focused
upstream tests cover PR #554's handler/seam layers, PR #567's trusted decode/poison classification/no-op behavior, PR
#561's detached rule persistence, graph-index synchronization, and service shutdown regressions inherited from beta.149.

### Require a clean production namespace

The production preflight SHALL resolve the literal NATS server, account, domain, and persistent volume, then prove the
target has no semconnect graph, index, observation, artifact, or guard resources. A non-empty result is no-go. The
preflight has no delete flag and the production bundle contains no wipe, translation, or migration command.

### Ship one reproducible production Compose bundle

The bundle SHALL run exactly the required topology: NATS with JetStream, SemStreams beta.151 graph services, and
semconnect. It SHALL pin immutable image digests or reproducible build inputs, record committed source revisions and
configuration hashes, and pass deterministic `docker compose config` validation. NATS file storage SHALL use one
explicit named persistent volume; anonymous or ephemeral production storage is forbidden.

Checked-in Compose and configuration files contain only non-secret values. NATS is reachable only on the private
Compose network, so the default topology has no credential or secret interface. Exposing NATS outside that boundary
requires a separate security design; it is not a configurable mode hidden inside this bundle.

### Prove first-start readiness and restart persistence

After empty-namespace proof, the operator SHALL start the exact bundle and run one versioned canonical seed whose
source hash, command, expected IDs, and counts are recorded. Readiness is layered: NATS health plus JetStream
availability; SemStreams health; semconnect health; then discovery of the canonical entity through both the
graph-index-backed collection and direct item query. The already accepted `137/0/0` run remains the broad API proof.

The operator SHALL stop services by normal Compose/SIGTERM without removing the volume, capture clean exit metadata,
restart the same bundle over the same volume, and prove equivalent resource counts and normalized collection/item
queries. This is a greenfield persistence proof, not old-state replay compatibility.

### Keep external conformance authority fixed

A fresh-volume run MUST report exactly `137 passed, 0 failed, 0 skipped`. The ETS pin and tests, fixture intent,
OpenAPI, conformance declarations, filters, skips, and result parser must be hash/diff reviewed against the accepted
beta.149 evidence. A manufactured green is rejected.

### Require an immutable production-bundle manifest

Task 6.3 SHALL produce an approval-free, content-addressed manifest binding the committed semconnect revision,
beta.151 source and image digests, NATS image, rendered Compose/config hashes, the explicit internal-only/no-secret
boundary, published CS API port, persistent volume identity, empty-namespace preflight, seed contract, layered
readiness, clean-stop/restart persistence evidence, and accepted `137/0/0` qualification. It contains no destructive
scope, old-state inventory, migration reseed, or old-binary rollback plan.

Product-owner and operator decisions are detached records referencing the same manifest SHA-256, avoiding a
self-referential hash. One person may hold both roles only through two explicit role-specific attestations. GO requires
both roles; any no-go, absence, hash mismatch, or changed input leaves execution unauthorized.

### Handoff the greenfield production work explicitly

The go-developer owns production Compose/config implementation and static tests. The operator owns the published port
and volume values, empty-namespace proof, first start, seed, readiness, clean stop, and restart-persistence evidence.
The go-reviewer independently reruns bundle validation and the smoke procedure. The technical writer hashes the bundle
manifest and evidence index. Product owner and operator alone issue task 6.4 go/no-go. Historical beta.147/beta.149
manifests and the earlier beta.151 architect signature remain evidence history; they are not active production gates.

## Verification and production gates

1. exact beta.151 pin/provenance alignment and complete nine-commit disposition;
2. failing-then-passing downstream pin and structural invariant tests;
3. focused upstream PR #554, #567, #561, graph-index, and service lifecycle tests;
4. unit, race, integration, vet, integration vet, and build gates;
5. atomic fail-closed direct mutation probes and greenfield empty-namespace preflight;
6. production Compose static validation, layered readiness, clean SIGTERM, and persistent-volume restart parity;
7. positive foreign-edge bake and zero unexpected structural rejections for canonical fixtures;
8. fresh external `137/0/0` and independent no-weakening review;
9. immutable production-bundle manifest plus explicit product-owner and operator go decisions.

Gates 1-8 qualify a candidate. They do not authorize production.

## Risks / Trade-offs

- [Trusted decode launders resident poison] -> Revalidate stored bytes on failure and prove no write/revision change.
- [Fail-closed gate rejects a semconnect predicate] -> Inventory and exercise every emitted predicate;
  do not add aliases.
- [Foreign-edge routing changes behind the gate] -> Seed a real hosted child and require claimed,
  non-dropped routing.
- [Rule shutdown change is assumed irrelevant from tier name] -> Audit enabled components and subjects,
  then run the upstream test.
- [The target is not actually greenfield] -> Fail preflight on any target resource; never auto-delete or translate.
- [Compose appears healthy before graph readiness] -> Poll the graph-index-backed canonical collection query.
- [Restart recreates empty storage] -> Require an explicit named volume and compare counts/query hashes after restart.
- [NATS is accidentally exposed] -> Keep it on the internal network with no published ports.
- [Green ETS hides reduced scope] -> Independent hash/diff no-weakening review.
- [Mutable bundle input outlives approval] -> Rehash the manifest and require new task 6.4 decisions.

## Rollback

There is no old production deployment or old-state rollback. Before first start, failure changes nothing. After first
start, operational recovery stops the new stack while preserving its named volume, corrects configuration or the
same-version bundle, and reruns readiness. Removing the volume or starting another SemStreams version requires a new
explicit greenfield/destructive decision outside this change. Compatibility fallback is forbidden.

## Open Questions

The product-owner and operator attestations remain intentionally unresolved until the task 6.3 manifest is final.
Destructive scope and old-state migration are explicitly out of scope.
