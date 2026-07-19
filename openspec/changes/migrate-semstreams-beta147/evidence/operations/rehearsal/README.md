# Disposable beta.147 validation evidence

Run `2026-07-18T02-03-23Z` completed the fresh-volume runtime, replay,
foreign-edge, and external ETS checks. It did not execute or approve a
production cutover.

## Gate result

- OpenSpec 9.2: complete. Initial readiness reached revision `80/80`; after
  the ETS writes it reached `118/118`. Eleven normalized query probes passed.
- OpenSpec 9.3: complete. A no-write backend restart returned to `118/118`,
  retained authoritative counts, and reproduced all eleven query hashes.
- OpenSpec 9.4: complete. The external Botts ETS reported
  `137 passed, 0 failed, 0 skipped`; the foreign-edge bake passed.
- OpenSpec 8.3 and 9.1: open. Fresh volumes proved the rebuilt candidate, but
  no approved immutable stop/wipe/reseed manifest was executed.
- OpenSpec 9.5: complete. Independent review found no ETS filtering, fixture
  weakening, OAS relaxation, or conformance-claim reduction.
- OpenSpec 9.6 through 9.8: open. Production remains no-go.

## Evidence index

- `runtime-resource-inventory.json` - before/after-restart authoritative,
  retained, and spatial-index stream counts.
- `live-query-parity.json` - eleven canonicalized response hashes.
- `replay-parity.json` - readiness revisions and no-write restart comparison.
- `retained-state-proof.json` - observation/ObjectStore retention evidence and
  the bounded proof limitations.
- `external-conformance.json` - pins, image IDs, TestNG result, foreign-edge
  result, artifact paths, and SHA-256 checksums.
- `../../review/final-conformance/approval.json` - independent task 9.5
  no-weakening approval.
- `shutdown-anomaly.md` - the beta.147 heartbeat shutdown errors, recovery,
  and production disposition.
- `go-no-go.json` - current production no-go and unresolved gates.

The source artifacts remain under `conformance/output/`. They are gitignored
runtime output, so each evidence record carries the repository-relative path
and a content hash. The image IDs and normalized query hashes were captured by
the program manager from the live disposable stack.

## Intentionally absent

The following proposed artifacts were not created because no corresponding
action occurred: `cutover-manifest.json`, `cutover-manifest.sha256`,
`writer-stop-evidence.md`, and `deletion-receipt.json`. Empty or
success-shaped placeholders would misrepresent task 8.3 or 9.1.

A production deployment still needs its own deployment-specific manifest,
literal destructive commands, actual pre-cutover counts, maintenance window,
rollback owner, and five approvals. The disposable run does not supply them.
