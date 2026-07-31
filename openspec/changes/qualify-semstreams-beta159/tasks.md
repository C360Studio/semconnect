## 1. Architecture contract

- [x] 1.1 **architect** records exact tag, commit, tree, complete exposed delta, non-goals, and test gates.
- [x] 1.2 **architect -> go-developer/go-reviewer/technical-writer** approves the dependency-only handoff and confirms
  that no ADR or product-boundary change is required.

## 2. Pin and downstream migration

- [x] 2.1 **go-developer** updates all executable module, conformance, Compose, test, and documentation pins together.
- [x] 2.2 **go-developer** updates alignment tests first and proves mixed beta.153/beta.159 identities fail.
- [x] 2.3 **go-developer** replaces removed readiness polling with fresh `GRAPH_STATUS` updates and fail-closed tests.
- [x] 2.4 **go-developer** bounds observations at 1 GiB/30 days/`DiscardOld` and rejects non-positive byte limits.
- [x] 2.5 **go-developer** runs full Go test, race, vet, module verification, and build gates.
- [x] 2.6 **go-developer** runs the live-NATS beta.159 structural contract without compatibility or HTTP behavior code.

## 3. Upstream and deployment qualification

- [x] 3.1 **go-developer** runs focused upstream race gates for readiness, NATS client, configuration, service,
  graph-ingest, graph-index, and graph-query at the exact beta.159 commit.
- [x] 3.2 **go-developer** proves add deduplication, remove no-op, poison scoping/recovery, foreign-edge routing, and
  query behavior on the live framework boundary.
- [x] 3.3 **operator** validates Compose and proves clean-volume preflight, seed/query readiness, normal stop,
  same-volume restart, and persistence parity.
- [x] 3.4 **program manager** runs the unchanged external suite to exactly `137 passed, 0 failed, 0 skipped`.

## 4. Independent review and closeout

- [x] 4.1 **go-reviewer** verifies provenance, exposed seams, storage bounds, and no legacy/compatibility code.
- [x] 4.2 **go-reviewer/technical-writer** prove no ETS, fixture, OpenAPI, conformance, filter, skip, parser, or harness
  weakening.
- [x] 4.3 **technical-writer** archives runtime evidence and updates active dependency status; frontend is N/A.
- [x] 4.4 **program manager** closes qualification when every gate is green; no old-volume or approval-ceremony claim
  is made.
