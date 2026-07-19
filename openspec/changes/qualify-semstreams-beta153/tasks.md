## 1. Architecture contract

- [x] 1.1 **architect** records exact tag, commit, tree, complete delta, exposure, non-goals, and test gates.
- [x] 1.2 **architect -> go-developer/go-reviewer/technical-writer** approves the dependency-only handoff.

## 2. Pin and downstream verification

- [x] 2.1 **go-developer** updates all executable module, conformance, Compose, test, and documentation pins together.
- [x] 2.2 **go-developer** updates alignment tests first and proves mixed beta.151/beta.153 identities fail.
- [x] 2.3 **go-developer** runs full Go test, race, vet, and build gates and archives exact commands/results.
- [x] 2.4 **go-developer** runs the live-NATS structural contract test without compatibility or product behavior code.

## 3. Upstream and deployment qualification

- [x] 3.1 **go-developer** runs focused upstream graph-ingest poison, watcher, memoization, and write-gate tests at the
  exact beta.153 commit.
- [x] 3.2 **go-developer** runs the upstream rule health race test and package tests for rule, agentic-loop, and
  fusion.
- [x] 3.3 **operator** validates Compose and proves fresh-volume preflight, seed/query readiness, normal stop,
  same-volume restart, and persistence parity.
- [x] 3.4 **program manager** runs the unchanged external suite to exactly `137 passed, 0 failed, 0 skipped`.

## 4. Independent review and closeout

- [x] 4.1 **go-reviewer** verifies provenance, exposure, no legacy/compatibility code, and independently reruns the
  proportional regression gates.
- [x] 4.2 **go-reviewer/technical-writer** prove no ETS, fixture, OpenAPI, conformance, filter, skip, or parser
  weakening.
- [x] 4.3 **technical-writer** updates active dependency status and records ordinary evidence links; frontend is N/A.
- [x] 4.4 **program manager** closes qualification when every gate is green; no approval manifest or hash ceremony is
  required.
