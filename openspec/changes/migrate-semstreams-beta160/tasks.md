## 1. Architect contract and handoff

- [x] 1.1 **architect** records ADR-S004 and approves typed graph, concurrency, projection, SensorML, artifact, audit,
  port, fresh-state, and NATS 2.14 decisions.
- [x] 1.2 **architect** verifies beta.160 tag commit `8403a2218000e45a31c5132fbfe01af42ed04f14`, source tree
  `9ed5dd3792bca63ce87ebf449a180add918f59ed`, and the upstream migration guide.
- [x] 1.3 **architect** classifies unused trajectory, tool, agent-run, metric, GraphQL, aggregate-client, component
  status, and context or structural persistence surfaces as not applicable.
- [x] 1.4 **architect -> go-developer/go-reviewer/technical-writer** issues the formal architecture handoff and records
  no unresolved design blockers.

## 2. Go developer failing-first graph contract tests

- [x] 2.1 **go-developer** writes failing tests for exact entity decoding, nonzero revision propagation, batch
  `missing[]`, and `response_too_large` classification before changing production code.
- [x] 2.2 **go-developer** writes failing single-request tests for create, reconcile, delete, revision mismatch,
  conflict, missing, unavailable, invalid, internal, and commit-unknown outcomes.
- [x] 2.3 **go-developer** writes failing HTTP tests for PUT-create races, PATCH conflicts and missing targets,
  idempotent DELETE, 409 mapping, and the explicit uncertain-commit 503 marker and correlation value.
- [x] 2.4 **go-developer** writes failing tests proving no raw mutation subject, hidden retry, or second exact read
  exists in a read-merge-write operation.

## 3. Go developer typed graph implementation

- [x] 3.1 **go-developer** introduces the domain-shaped graph interface and production beta.160 adapter while retaining
  focused handler fakes.
- [x] 3.2 **go-developer** uses typed `entity.create`, an exact-revision local reconcile adapter, and exact-revision
  delete, deriving operation subjects from the declared typed mutation family.
- [x] 3.3 **go-developer** updates exact and batch reads to authoritative beta.160 response shapes and preserves
  context cancellation and classified errors.
- [x] 3.4 **go-developer** removes raw mutation subjects and old entity-state response assumptions from production,
  tests, fixtures, and documentation.

## 4. Go developer projection and SensorML TDD

- [x] 4.1 **go-developer** writes failing contract tables for all eleven resource families, message types, entity
  patterns, birth predicates, atomic groups, and indexing profiles.
- [x] 4.2 **go-developer** implements local projection contracts, with type birth-only, System and Datastream
  `representation` groups, and create-only resource facts birth-only.
- [x] 4.3 **go-developer** proves Datastream accepts local and federated six-part IDs through its wildcard pattern and
  rejects invalid IDs and anonymous message types before I/O.
- [x] 4.4 **go-developer** proves typed creates keep embedded entity facts empty, all create subjects equal the target,
  and missing relationship targets never create stubs.
- [x] 4.5 **go-developer** writes failing embedded SensorML tests, then retains root facts and root-to-child references
  while dropping all foreign-subject inverse facts.
- [x] 4.6 **go-developer** removes ownership imports and binding calls with no compatibility facade.

## 5. Go developer immutable artifact and audit TDD

- [x] 5.1 **go-developer** writes failing tests for canonical-byte digest IDs and keys, immutable create,
  exact conflict
  verification, integrity mismatch, and parent link reconcile.
- [x] 5.2 **go-developer** configures storage instance `objectstore` over bucket `CS_API_ARTIFACTS`, proves exact
  lookup, and removes fallback and custom-port behavior.
- [x] 5.3 **go-developer** records orphan artifact garbage collection as deferred and performs no migration-time scan
  or delete.
- [x] 5.4 **go-developer** writes failing structured-audit tests for every mutation attempt, then records request ID,
  trace ID, identity hints, operation, entity, commit state, revision, and classified error.
- [x] 5.5 **go-developer** removes graph-mutation custom-header propagation claims while preserving the independent
  observation publish-header contract.

## 6. Go developer dependency, port, and configuration alignment

- [x] 6.1 **go-developer** aligns the Go toolchain, module, checksum, backend, test, Compose, and evidence pins to the
  exact beta.160 release and aligns `nats.go` with the supported dependency set.
- [x] 6.2 **go-developer** writes failing declaration tests, then regenerates canonical port envelopes, typed mutation
  and query families, canonical NATS request and JetStream ports, and handled resolution errors.
- [x] 6.3 **go-developer** removes flat fields, aliases, service names, synthetic HTTP and objectstore ports, and any
  duplicate graph mutation provider; graph ingest resolves exactly one typed provider.
- [x] 6.4 **go-developer** bumps top-level configuration versions and validates both deployment and conformance
  configuration against generated beta.160 schemas.
- [x] 6.5 **go-developer** aligns development, conformance, and deployment to NATS 2.14 and records exact image digest
  and server version.

## 7. Go developer verification and formal review handoff

- [x] 7.1 **go-developer** runs formatting, `go mod verify`, `go vet ./...`, integration vet, `go test ./...`, race,
  integration tests, and `go build ./...`, archiving uncached output and the exact Go environment.
- [x] 7.2 **go-developer** proves live NATS create, reconcile, conflict, delete, missing reference, batch missing,
  uncertain commit classification where injectable, artifact read, observation, and projection behavior.
- [x] 7.3 **go-developer** performs the bounded not-applicable audit and records no newly used beta.160 surface that
  lacks a migration disposition.
- [x] 7.4 **go-developer -> go-reviewer** hands off implementation scope, failing-first evidence, exact pins, known
  risks, test output, configuration validation, and zero unresolved developer blockers.

## 8. Reviewer quality gates

- [x] 8.1 **go-reviewer** verifies exact release identity, no raw subject or compatibility path, one-attempt mutation,
  exact revision use, error mapping, context propagation, and critical-path test quality.
- [x] 8.2 **go-reviewer** verifies all resource contracts, root-only SensorML projection, no stub creation, immutable
  artifacts, exact storage resolution, structured audit, and canonical declarations.
- [x] 8.3 **go-reviewer** independently reruns the full Go, real-NATS, schema, and configuration gates and signs exact
  output or returns actionable findings.
- [x] 8.4 **svelte-reviewer** records frontend as not applicable after confirming no generated public type, UI label,
  accessibility, or interaction change; any discovered change returns work through the Svelte TDD cycle.
- [x] 8.5 **reviewers -> technical-writer** issue formal approval; conditional approval does not advance release work.

## 9. Technical writer and operational qualification

- [x] 9.1 **technical-writer** updates current dependency, configuration, operations, OpenAPI, and migration guidance
  after reviewer approval without rewriting historical beta.159 evidence.
- [x] 9.2 **technical-writer** records explicit 409 and uncertain-commit 503 behavior, structured audit semantics,
  embedded-child limits, immutable artifact behavior, deferred garbage collection, and fresh-state requirements.
- [x] 9.3 **operator/program manager** provisions a new beta.160 NATS volume, verifies it is empty, seeds the stack,
  captures authoritative and indexed revisions, and actively polls readiness without relying on silence or sleeps.
- [x] 9.4 **operator/program manager** performs a normal stop and no-write restart on the same beta.160 volume and
  proves equal or later revision, query parity, schema artifact access, and observation persistence.
  Authoritative r5 evidence on volume `semconnect-beta160-qualification-20260812-r5` is byte-identical before and
  after restart for the System, schema-backed Datastream, schema endpoint, and global and scoped Observation reads.
  Graph readiness is target/indexed `3/3` before and after restart. Historical r3 proves System parity only.
- [ ] 9.5 **operator/program manager** records the separate preserved beta.159 rollback deployment and volume and
  proves that neither version opens the other version's state.
  **WAIVED BY PRODUCT OWNER (2026-08-12), NOT PROVEN.** Authorization accepted the residual lack of a beta.159
  rollback environment. Fresh beta.160 storage, no cross-version volume access, evidence retention, backup,
  active monitoring, and abort-on-guardrail-failure remain mandatory.

## 10. External conformance and release decision

- [x] 10.1 **program manager** runs `./conformance/run.sh` from disposable fresh volumes on the aligned beta.160 and
  NATS 2.14 stack and archives exactly `137 passed, 0 failed, 0 skipped` plus TestNG and service logs.
- [x] 10.2 **go-reviewer/technical-writer** prove the ETS pin, tests, fixture intent, OpenAPI surface, and claimed
  conformance set were not removed, filtered, skipped, or weakened to obtain green.
- [x] 10.3 **architect/product owner/operator** review exact pins, approvals, fresh-volume identity, restart parity,
  rollback isolation, full verification, and external evidence and issue an explicit production go or no-go.
  Decision: **GO WITH ACCEPTED RISK**. The product owner authorized proceeding on 2026-08-12 despite task 9.5 being
  unproven. The exact authorization is archived in `evidence/product-owner-release-decision.json`; this is a waiver,
  not evidence that rollback isolation passed.
- [x] 10.4 **technical-writer** archives the final revisions, image digests, counts, readiness, parity, external
  result, timestamps, and sign-offs; retained state or missing evidence remains a blocker.
