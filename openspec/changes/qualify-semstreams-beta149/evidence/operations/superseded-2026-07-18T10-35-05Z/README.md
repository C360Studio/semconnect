# Disposable beta.149 shutdown and replay evidence

Run `2026-07-18T10-35-05Z` qualifies the beta.149 lifecycle fix in the
disposable `semconnect-conformance` project. It does not authorize or perform a
production cutover.

## Result

- The external harness had already seeded canonical fixtures and completed at
  exactly `137 passed, 0 failed, 0 skipped` before this rehearsal began.
- The post-ETS authoritative graph target was `118`; graph-index reported
  `118/118` before shutdown and after the no-write restart.
- TeamEngine and cs-api-server were stopped, in that order, before the backend
  signal. The beta.149 backend received normal SIGTERM and exited 0 in less
  than one second, with no OOM, container error, timeout, or SIGKILL.
- The bounded shutdown log contains ten INFO lines, ends with
  `SemStreams shutdown complete`, and contains none of the issue #549 or
  generic teardown failure signatures.
- Eleven beta.147-equivalent normalized probes and one additional command
  schema readability probe are byte-for-byte equal before and after restart.
- `CS_API_OBSERVATIONS`, `OBJ_CS_API_ARTIFACTS`, and `KV_ENTITY_STATES`
  retain identical message, byte, subject, and sequence state. The derived
  spatial index advances its sequence during rebuild while retaining the same
  two logical subjects and identical query results.

## Evidence index

- `source-identity.json`, `container-identities-pre-stop.json`, and
  `image-identities.json` bind the repository, upstream vendors, source
  hashes, image IDs, container IDs, and creation/start timestamps.
- `pre/` and `post/` contain exact raw API/NATS captures, normalized JSON,
  checksums, readiness JSONL, and retained-state inventories.
- `writer-freeze.json` records the freeze order and the no-write boundary.
- `shutdown-metadata.json` records the stop timeout, process state, Docker
  events, and absence of SIGKILL.
- `shutdown-window.log` is the bounded shutdown log.
- `filter-verification.md` proves the failure expression matches the known
  beta.147 failure shape before it is applied to beta.149.
- `replay-summary.json` records readiness, recursive diff, and retained-state
  outcomes.
- `evidence-hashes.sha256` binds the evidence bundle used by
  `operator-handoff.json`.

The helper scripts are retained to make the capture shape reviewable. They
only issue GET requests and monitoring queries. TeamEngine remains stopped;
NATS, the healthy beta.149 backend, and cs-api-server remain running for the
independent reviewer.

Production authorization remains withheld pending the inherited ADR-S003
deployment manifest and approval gates.
