## Context

The beta.147 migration is implemented and has fresh external conformance plus graph readiness/replay evidence. Its
live no-write restart also reproduced SemStreams issue #549: signal-context cancellation stopped heartbeat before
`Manager.StopAll`, then the heartbeat override rejected the already-stopped state and made graceful shutdown fail.

Upstream beta.149 provenance is exact:

- annotated tag object: `7c0c5aae9c02d148e118627b18281f34d9adf2f8`;
- peeled release commit: `7db0cdcb21577eaa52eb842c4ffb06a854f9a9b2`;
- issue #549 / PR #550 fix: `cc34be3d565405d86d9e13375013ce3522fa1d5f`;
- second beta.149 commit: agentic-tools advertised-set enforcement at the release commit;
- inherited beta.148 commits: agentic-loop iteration budgets and rule scalar/recovery completeness.

The beta.147..beta.149 diff changes service lifecycle, agentic, and rule packages. It does not change SemStreams graph,
NATS client, message, vocabulary, OGC ownership, or `go.mod`. Semconnect's conformance config enables only graph
processors; it has no agentic-loop, agentic-tools, `publish_agent`, or rule-action consumer. Those deltas are recorded
as not applicable, not silently ignored.

## Goals / Non-Goals

**Goals:**

- replace beta.147 with the exact beta.149 release candidate across every executable pin;
- prove local compile/runtime compatibility without a compatibility branch;
- prove issue #549 is closed under the real cancellation-before-`StopAll` ordering;
- preserve and re-prove graph state, readiness, replay, CS API wire behavior, and external conformance;
- leave a reviewable evidence envelope for a separate production go/no-go decision.

**Non-goals:**

- graph wipe, reseed redesign, new product behavior, public CS API changes, agentic/rule adoption, or production action;
- modifying signed beta.147 evidence to describe beta.149 after the fact;
- treating a healthy HTTP endpoint, silent logs, or a fixed delay as shutdown/readiness proof.

## Decisions

### Use a dependent qualification change

ADR-S003 remains the durable boundary decision. Beta.149 is a dependency qualification over the same canonical graph
contract, so this change references the signed beta.147 artifacts rather than editing them. All new evidence lives
under `qualify-semstreams-beta149`.

### Pin the immutable commit and human-readable tag together

The Go requirement SHALL be `github.com/c360studio/semstreams v1.0.0-beta.149`. The conformance checkout SHALL use
commit `7db0cdcb21577eaa52eb842c4ffb06a854f9a9b2` and record tag `v1.0.0-beta.149`. A test SHALL fail if the module,
tag, or conformance commit diverges. The annotated tag object is provenance evidence, not an executable pin.

### Treat the whole release delta as qualification scope

PR #550 fixes heartbeat and metrics-forwarder idempotency with `sync.Once`, drains BaseService goroutines on an
already-terminal stop, and rejects restart of spent single-use service instances. The release also carries unrelated
agentic/rule behavior. A source/config audit SHALL prove those processors and actions are absent from semconnect. If
the audit finds a live consumer, this proposal is incomplete and a separate behavior specification is required.

### Prove shutdown using authoritative process state

The qualification harness SHALL capture backend identity and timestamps, freeze semconnect graph writers, send the
backend SIGTERM through Docker's normal stop path, and wait for termination. Evidence SHALL require exit code zero,
`OOMKilled=false`, no container error, no timeout/forced kill, and no matching false-shutdown log signature:

- `heartbeat service not running`;
- `metrics forwarder not running`;
- `Service stop failed`;
- `Error stopping services`;
- `graceful shutdown failed`.

The harness SHALL also run the upstream beta.149 service regression tests that cover both overrides, because the
semconnect graph-only configuration exercises heartbeat but does not enable metrics-forwarder.

### Couple clean restart to no-write replay proof

Before stopping, the harness SHALL capture authoritative `ENTITY_STATES` target revision and normalized API/query
responses. No graph writer may run between that capture and backend restart. After restart, active polling SHALL wait
for graph-index readiness with indexed revision at least the captured target; then the same probes SHALL be equivalent.
Container health, log silence, and elapsed time are insufficient.

### Preserve state and fail closed

Beta.149 introduces no additional graph migration, so this qualification SHALL NOT wipe graph, observation, or
artifact state. A beta.149 failure before production leaves beta.147 as the existing candidate, but beta.147 remains
production no-go because issue #549 is unresolved there. Any detected graph-layout, identity, or replay delta blocks
qualification and reopens architecture review; it is not handled with dual reads or an ad hoc wipe.

### Keep external conformance immutable

The final disposable fresh-volume run MUST report `137 passed, 0 failed, 0 skipped`. Review SHALL show no changes to
the ETS pin, tests, fixture intent, OpenAPI, conformance declarations, or skip/filter logic used to obtain that result.

## Verification and production gates

Qualification requires all of the following on one reviewed source revision:

1. exact module/tag/commit alignment and upstream-delta audit;
2. focused SemStreams service regression tests from the beta.149 checkout;
3. `go test ./...`, `go test -race ./...`, integration tests, vet, integration vet, and build;
4. real clean SIGTERM exit and bounded log audit;
5. revision-based readiness plus no-write replay parity;
6. fresh external ETS `137/0/0` with no weakening;
7. independent Go review and technical-writer evidence verification;
8. explicit architect, product-owner, and operator production approval.

Passing gates 1-7 qualifies a candidate. It does not satisfy gate 8 or authorize production.

## Risks / Trade-offs

- [The issue test passes but Docker shutdown still fails] -> Require real process exit metadata and log audit.
- [An unrelated agentic/rule delta reaches semconnect] -> Audit configured components and subjects; fail on any use.
- [A restart hides graph replay loss] -> Freeze writers and compare authoritative revision plus normalized probes.
- [Pins identify different source] -> Enforce module/tag/commit alignment in an automated test and evidence manifest.
- [Green ETS is manufactured] -> Independently hash and diff ETS, fixtures, claims, and filter/skip configuration.
- [Signed beta.147 history drifts] -> Hash-check it read-only; write all beta.149 evidence under this dependent change.

## Rollback

Before production, a failed qualification simply retains beta.147 as a non-production candidate and records the
blocker. No destructive action is required. A production rollback is outside this change and cannot be inferred from
graph compatibility; it requires the inherited ADR-S003 manifest, an operator procedure, and explicit approval.

## Open Questions

No architectural question blocks implementation. Deployment-specific production context, owners, schedule, and
approvals remain intentionally unresolved and keep production unauthorized.
