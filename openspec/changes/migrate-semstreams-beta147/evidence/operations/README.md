# Beta.147 operations evidence

This directory contains the technical-writer and operator evidence for the
SemStreams beta.147 graph-state cutover. Architecture, development, and review
evidence lives in sibling directories and is referenced rather than copied.

## Current decision

- Go quality gate: approved.
- Svelte quality gate: approved.
- Disposable run `2026-07-18T02-03-23Z`: revision/query/replay gates passed.
- External beta.147 ETS: `137 passed, 0 failed, 0 skipped`.
- Foreign-edge bake: passed.
- Production deployment: P0 blocked; only the non-destructive architecture
  template exists because deployment values and approvals are unavailable.
- Destructive execution: not authorized.
- Shutdown logs: not clean; two heartbeat-already-stopped ERROR records are
  retained as an explicit production blocker.

The retained-state identity report is approved for the signed seed contract.
The disposable runtime proved readiness at `80/80`, then `118/118` before and
after a no-write restart. Authoritative and retained counts matched and all
eleven normalized query hashes were identical. This does not complete the
executable cutover manifest: task 8.3 and the exact-manifest rehearsal in task
9.1 remain open. Independent no-weakening review for task 9.5 is approved.

## Evidence index

- `conformance-deployment-inventory.md` - exact repository-rendered names,
  writers, delete candidates, preserve set, and literal operator commands.
- `rollback-no-go-and-communications.md` - rollback boundary, stop criteria,
  review roles, communication checkpoints, and evidence envelope.
- `rule-event-not-applicable.json` - bounded beta.147 rule/event audit.
- `conformance-cutover-manifest.blocked.json` - schema-valid immutable blocked
  snapshot; it is not an executable manifest.
- `retained-state-identity-impact.md` - retained observation and artifact
  identity analysis and remaining proof gap.
- `technical-writer-progress.json` - current handoff state and blockers.
- `technical-writer-handoff.json` - formal pre-rehearsal evidence delivery.
- `technical-writer-verification.md` - commands, results, and non-claims.
- `rehearsal/README.md` - disposable runtime results, evidence index, and open
  production gates.

Architecture-controlled inputs:

- `../architecture/cutover-manifest.schema.json`
- `../architecture/cutover-manifest.template.json`
- `../architecture/compatibility-matrix.md`
- `../architecture/semantic-ledger.json`

No production command is generated from the conformance inventory. Production
must instantiate and validate its own deployment-specific manifest and collect
the required architect, product-owner, operator, and two destructive-reviewer
approvals.
