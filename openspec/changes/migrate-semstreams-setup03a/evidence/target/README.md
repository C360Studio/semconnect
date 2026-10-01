# Frozen target runtime qualification

Final runtime execution uses SemStreams
`v1.0.0-beta.162.0.20260930150212-8b99efe9c66a`, commit
`8b99efe9c66a4faa4fa509f9f62cc6bad8392128`, with NATS unchanged at `2.14.4`.
The registered SemConnect graph host uses the gateway's configuration and the
explicit create-only platform-identity provisioning service.

These local runtime checks pass. They do not remove the separately reproduced
cross-authority Datastream mutation incompatibility or grant migration approval.
Independent Go review and the complete migration disposition are recorded by the
parent proposal, not inferred from a passing ETS result.

## Final results

| Gate | Result | Evidence |
| --- | --- | --- |
| Deployment/config/pin topology TDD | RED then GREEN | `topology-tdd.json`, `topology-final-green.log` |
| Unchanged external ETS | **137 passed, 0 failed, 0 skipped** | `conformance/summary.txt`, TestNG XML |
| Root-only SensorML bake | PASS; root readable and inline child absent | `conformance/root-only-bake-*.txt` |
| Fresh-storage preflight | Zero streams, consumers, messages, bytes | `persistence/first-start-jsz.json` |
| Real-NATS normal stop/no-write restart | PASS | `persistence-execution.json`, `persistence/` |
| Bbox hit, polygon hit, distant bbox miss | PASS before/after second normal restart | `spatial/` |
| Baseline/target canonical and spatial response bytes | Identical | `baseline-comparison.json` |

The final conformance run completed at `2026-10-01T10:09:50Z`; persistence
completed at `2026-10-01T10:10:18Z`. Exact command, timestamp, and exit-code
records accompany the raw outputs. The first target attempt also passed, but is
retained only as history under `attempt1/` because the graph host subsequently
gained explicit shared gateway configuration. Final runs use separate fresh r3
storage and the final production source. The prior r2 runtime pass is preserved
under `attempt2/`; final r3 includes the reviewer-requested gateway startup-only
signal bridge fix. `attempt2-to-final-production-diff.json` attributes its sole
production source change to `cmd/cs-api-server/main.go`.

Canonical proof SHA-256 at both revisions, before and after restart:
`4544c19899ccf1c9b461146be542c47dd85d4201aad5aeda2c9cf48946b63e32`.
Spatial response-summary SHA-256 at both revisions, before and after restart:
`0dfd7b356c14534142a4b3644be7183130f562b9dfc23de7ba32fd89dab07f20`.
All persistent service stops exit zero without OOM, and graph target/indexed
readiness remains `3/3` across both target restarts.

## Isolation and source identity

Final conformance project: `semconnect-03a-target-r3-20261001`.
Final persistence project: `semconnect-03a-target-r3-persistence`.
Final persistence volume: `semconnect-03a-8b99efe9-20261001-r3-fresh`.
Both projects and the volume were absent before execution.

`runtime-source-inputs.json` captures source hashes at the start of final
qualification. Every captured source file still matches at the final comparison;
no production, test, bootstrap, dependency, Docker, or runtime configuration file
drifted. The checked paths are recorded in `runtime-source-inputs.json`, and the
empty drift list is recorded in `baseline-comparison.json`. The ETS harness,
fixtures, OAS, and conformance assertions were not weakened.

Final logs were captured before removing only the two named r3 projects.
The target volume is preserved separately from the beta.160 volume and the
earlier target attempt. Cleanup commands/results and preserved volume identity
are recorded in `cleanup.json` and `preserved-target-volume.json`.

## Runtime differences and attribution

The canonical System, Datastream, schema, global/scoped Observation proof, and
spatial response bytes have no baseline/target difference. The external suite
reports the same `137/0/0` counts, and root-only projection still passes.

The following configuration and diagnostic changes are intentional upstream
contract adaptations:

- ADR-103 makes the payload registry authoritative. The consumer-owned host
  registers SemConnect resource contracts instead of using the stock framework
  binary. It receives the same CS API configuration as the gateway.
- ADR-102 removes `platform.instance_id`; ADR-104 mints or adopts persistent
  platform identity. The explicit provision command preserves the public
  `c360.semconnect` authority through create-only adoption before graph startup.
  It never rewrites a different existing identity or reuses older storage.
- The target's canonical composition analysis emits nine warning shapes at each
  startup: six optional index outputs have no in-process watchers, two request
  API inputs have no in-process producers, and graph-query appears disconnected
  in the declared component graph. Their consumers live in the separate gateway
  process. All nine are warnings, with zero boot composition errors. The source
  is `service/component_manager.go:389–424` at the frozen target, documenting
  the intended retained boot analysis. Actual graph request/reply behavior is
  exercised by unchanged ETS and persistence probes. Details are retained in
  `non-clustering-warning-details.json`.
- The baseline stock host logged a missing optional persona directory and one
  empty configuration-key lookup warning. The narrow target host has no persona
  loader, and its explicitly provisioned configuration catalog is nonempty.
  These baseline warnings therefore do not recur.

The recurring optional clustering warnings are **unchanged baseline behavior**,
not a target regression. Both revisions report the exact same missing
`COMMUNITY_INDEX` and `COMMUNITY_SUMMARIES` owner, messages, and five-second retry
interval. The graph-query source explicitly installs query responders before
starting optional reader supervisors (`processor/graph-query/component.go:565`
and `summary_view.go:124` at the target). Neither composition config enables
graph-clustering. Their absence does not gate the exercised graph queries;
adding clustering solely to silence logs would change the qualified composition.
`optional-clustering-comparison.json` retains paired examples and the source
locations; differing warning counts reflect differing observation durations.

No ERROR-level JSON records occurred in either persistence runtime log. This is
supplemental evidence only: authoritative health, graph readiness, exact response
bytes, exit codes, and TestNG results establish the passing local checks.
