# Beta.151 final evidence index

This directory closes the technical qualification evidence for SemStreams
`v1.0.0-beta.151` at peeled commit
`ac75c322140fb2a6b55759d07a79874b4cb4d9cc` and source tree
`120eeb353afb7d07aa1b3180de05f75494bac1a8`.

The sole authoritative external run is `2026-07-18T17-09-45Z`. It started
after final task 5.4 reviewer approval, used a fresh disposable NATS volume,
and returned exactly `137 passed, 0 failed, 0 skipped`. The foreign-edge lane
was exercised with zero unclaimed and zero dropped edges. The earlier
`17-06-05Z` result is green rehearsal evidence without qualification authority.

## Technical disposition

- Pin, structural fail-closed, trusted-RMW, local, retained-state, clean
  SIGTERM, readiness, no-write replay, foreign-edge, and external gates pass.
- The independent no-weakening review confirms the same test/config name sets
  as beta.149 and no fixture, OpenAPI, claim, selector, skip, or parser
  relaxation.
- Frontend/Svelte is not applicable; no public CS API or UI contract changed.
- The dependency candidate is technically qualified.

## Production disposition

Task 6.3 is complete. The approval-free greenfield manifest is
`greenfield-production-manifest.json`, SHA-256
`dbddd8dfb85f65e3746f8c6f8496bc9c779707d172101fc54ea17093bb707ca8`.
Task 6.4 requires detached product-owner and operator decisions over exactly
that digest. Production remains unauthorized until both decisions are `go`.

## Evidence map

- `external-conformance.json` — fresh run, exact pins, images, readiness,
  report, log, seed, and foreign-edge hashes.
- `external-image-identities.json` — live image and fresh-volume identities.
- `no-weakening-audit.md` — technical-writer comparison and final reviewer
  approval.
- `greenfield-production-manifest.json` — immutable task 6.3 source, image,
  deployment, fresh-state, seed, readiness, shutdown, restart, and
  qualification record. It contains no task 6.4 decisions.
- `greenfield-evidence-index.md` — concise map of the manifest's bound proof
  and explicit greenfield-only scope.
- `frontend-disposition.json` — frontend N/A evidence.
- `production-no-action-record.json` — receipt of the operator's explicit
  no-production-action attestation before the greenfield amendment.
- `evidence-hashes.sha256` — hashes the final technical and greenfield task
  6.3 evidence. It excludes itself and the mutable task checklist.
- `technical-writer-handoff.json` — task 6.3 closeout and task 6.4 hold.

Supporting role evidence remains in sibling directories: architecture and Go
development handoffs, the 81-file operator rehearsal bundle, and final Go
review. Signed beta.147 and beta.149 evidence is historical and unchanged.
