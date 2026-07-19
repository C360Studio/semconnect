# Qualifying beta.149 shutdown and replay evidence

Run `2026-07-18T10-52-31Z` is the sole qualifying operations run for this
change. It used the normalized source attested at `2026-07-18T10:39:30Z`,
completed the external suite at exactly `137 passed, 0 failed, 0 skipped`, and
then completed the disposable shutdown/replay rehearsal. It does not authorize
or perform a production cutover.

## Source and image binding

The signed pre-build handoff binds `go.mod` at `a41ce208...`, normalized
`go.sum` at `6fb8194...`, and `.ets-pin` at `bf219bf...`. The qualifying build
log shows uncached IUT dependency/source copies and an uncached `go build`,
then exports image `sha256:0a0e733...`; the new IUT container was created at
`2026-07-18T10:52:36Z`. The final runtime COPY layer was cached because the
normalization rebuilt to identical binary bytes. The cached backend is bound
to immutable vendor HEAD `7db0cdcb...`, image `sha256:b04da49...`, and a new
container created at `2026-07-18T10:52:36Z`. See `build-provenance.json`.

## Runtime result

- The post-ETS authoritative target was 118; graph-index reported `118/118`
  before shutdown and after the no-write restart.
- TeamEngine and cs-api-server stopped, in that order, before backend SIGTERM.
- The backend exited 0 inside one second with no OOM, runtime error, timeout,
  or SIGKILL. Its ten-line shutdown window is all INFO and ends with
  `SemStreams shutdown complete`.
- The failure filter matched the known beta.147 issue #549 shape and matched
  no qualifying beta.149 line.
- Eleven beta.147-equivalent normalized probes and one additional command
  schema probe are byte-for-byte identical across restart.
- Observation, ObjectStore, and authoritative entity state are identical.
  The derived spatial index advances only its rebuild sequence and preserves
  the same two subjects and exact query results.

## Evidence index

- `build-provenance.json`, `source-identity.json`,
  `container-identities-pre-stop.json`, and `image-identities.json` bind source,
  build context, vendor commits, image IDs, container IDs, and timestamps.
- `pre/` and `post/` hold the qualifying raw/normalized captures, readiness
  JSONL, inventories, and hashes.
- `writer-freeze.json`, `shutdown-metadata.json`, `shutdown-window.log`, and
  `filter-verification.md` prove the shutdown gate.
- `replay-summary.json` proves the no-write readiness/replay and retention gate.
- `evidence-hashes.sha256` and `operator-handoff.json` bind only the qualifying
  files described above.

## Superseded provenance

`superseded-2026-07-18T10-35-05Z/` is retained only because destructive removal
was not approved. That earlier capture preceded the final normalized-source
attestation and is non-qualifying. It is excluded from the qualifying checksum
manifest, operator signature, runtime decision, and reviewer handoff.

TeamEngine remains stopped. NATS, the healthy beta.149 backend, and the final
IUT remain running for independent review. Production authorization remains
withheld pending inherited ADR-S003 deployment manifest and approval gates.
