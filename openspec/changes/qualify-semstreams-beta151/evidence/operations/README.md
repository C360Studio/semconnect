# Beta.151 operator qualification evidence

This directory records the disposable beta.151 shutdown and no-write replay
rehearsal for run `2026-07-18T16-40-05Z`. It is qualifying evidence only. It is
not a production authorization and does not replace independent review.

## Result

- Exact beta.151 source, clean vendor trees, build log, images, containers, and
  pre-build signed handoff are bound in `build-provenance.json`.
- Every retained `ENTITY_STATES` stream record was enumerated. Of 67 retained
  records, all 63 PUT values passed beta.151 `graph.UnmarshalEntityState`; four
  valid KV tombstones were accounted for. Coverage is complete, poison count is
  zero, and the frozen stream revision is 118.
- TeamEngine and cs-api were stopped before backend SIGTERM. The backend exited
  zero with `OOMKilled=false`, no Docker error, no forced kill, and none of the
  beta.149 shutdown signatures.
- The same backend container/image restarted without writes and graph-index
  reached the frozen 118/118 target. All twelve normalized API probes are exact
  pre/post matches. Entity state, observations, and ObjectStore inventory are
  unchanged. The spatial KV revision advanced only because the index rebuilt;
  its two public spatial probes are exact matches.
- The hosted-child lane fired once, persisted its claimed child edge, and
  produced zero unclaimed/dropped evidence. The signed isolated integration
  proof supplies positive invalid-mutation rejection and atomicity evidence
  without contaminating this retained state.

## Evidence map

- `source-identity.json`, `image-identities.json`, and
  `container-identities-pre-stop.json`: raw provenance captures.
- `retained-state-scan.json` and `post/retained-state-scan.json`: exhaustive
  beta.151 validation before and after restart.
- `pre/` and `post/`: normalized probes, raw responses, metrics, logs,
  index-readiness samples, and JetStream inventory.
- `writer-freeze.json`, `shutdown-metadata.json`, and `shutdown-window.log`:
  immutable rehearsal boundary and shutdown result.
- `structural-rejection-evidence.json`, `foreign-edge-evidence.json`, and
  `filter-verification.md`: gate-specific proof and tested filter shapes.
- `replay-summary.json`: concise pre/post result.
- `evidence-hashes.sha256` and `operator-handoff.json`: reproducible qualifying
  manifest and operator handoff.

The external TeamEngine result for this same disposable run is exactly
137 passed, 0 failed, 0 skipped at beta.151 and ETS commit
`d9caf33fcd0c4a3c1a582e8ba9b12b753277afd4`. It is informative and available in
`conformance/output/summary.txt`; it remains pre-final-review evidence and is
not treated here as final conformance or production authority.

The reviewer stack is intentionally left with NATS, beta.151 backend, and the
cs-api server running; TeamEngine remains stopped so it cannot write or launch
another suite.
