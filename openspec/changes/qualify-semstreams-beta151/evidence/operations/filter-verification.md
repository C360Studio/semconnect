# Operator filter verification

The structural rejection filter covers `structural contract rejection`,
`structurally invalid ingest`, `graph_state_reset_required`, and the exported
predicate/entity-state contract rejection metric names. It was sanity-checked
against the exact beta.151 source used to build the backend:

- `processor/graph-ingest/keyed_ingest.go:135,184` contains
  `Failed to terminate structurally invalid ingest`;
- `processor/graph-ingest/keyed_ingest.go:242` contains
  `graph-ingest: structural contract rejection; terminating`;
- `processor/graph-ingest/keyed_ingest_test.go:288` contains
  `code=graph_state_reset_required`.

The pre- and post-restart backend windows have zero matches and zero
`"level":"ERROR"` records. The extracted structural/foreign metric files are
both empty, meaning the applicable counters were not emitted and therefore
read as zero. Silence is not used as the positive rejection proof: the signed
developer integration run exercised invalid predicate, ID, and reference
mutations directly over production NATS subjects and proved classified atomic
rejection with unchanged entity bytes and KV revision. The live rehearsal then
proved the retained bucket was unchanged and unpoisoned without injecting an
invalid record into qualifying state.

The shutdown filter covers `heartbeat service not running`,
`metrics forwarder not running`, `Service stop failed`,
`Error stopping services`, `graceful shutdown failed`, and JSON ERROR records.
It matches the known beta.149 failure shape in
`conformance/output/semstreams-backend-restart-2026-07-18T02-03-23Z.log:9-11`
and its archived repetition at the same lines. The bounded beta.151 shutdown
window contains ten INFO records, ends with `SemStreams shutdown complete`, and
has zero matches for every issue signature.
