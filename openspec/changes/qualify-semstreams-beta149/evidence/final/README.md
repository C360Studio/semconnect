# Beta.149 qualification evidence index

This directory closes the non-production qualification gates for SemStreams
`v1.0.0-beta.149` at peeled commit
`7db0cdcb21577eaa52eb842c4ffb06a854f9a9b2`. The qualifying external and runtime
run is `2026-07-18T10-52-31Z`, built from the final normalized source. The earlier
`2026-07-18T10-35-05Z` run is explicitly superseded and has no qualification
authority.

## Final disposition

- External Botts ETS: `137 passed, 0 failed, 0 skipped` on a fresh disposable
  volume.
- No weakening: ETS source, fixtures, OpenAPI, conformance claims, harness, and
  filter/skip behavior match the accepted beta.147 baseline.
- Runtime: clean normal SIGTERM, zero issue #549 signatures, readiness restored
  to `118/118`, and all 12 normalized replay probes matched without a write.
- Frontend/Svelte: not applicable; no UI source, contract, or build dependency
  changed.
- Production: unauthorized. The inherited ADR-S003 manifest and explicit
  architect/product-owner/operator decision remain open.

## Evidence map

- `external-conformance.json` binds the fresh-volume report, logs, readiness,
  source pins, images, and hashes.
- `no-weakening-audit.md` records the technical-writer audit and independent Go
  reviewer approval.
- `frontend-disposition.json` explains why no Svelte implementation or review
  lane was required.
- `technical-writer-handoff.json` is the final candidate-qualification
  attestation and production hold.
- `evidence-hashes.sha256` signs the final evidence files that precede the
  handoff. The handoff records that manifest's hash.

Supporting role evidence remains in sibling directories:

- `../architecture-handoff.json` — immutable qualification contract.
- `../development/go/` — pin-alignment TDD, delta audit, and local verification.
- `../operations/` — qualifying shutdown, replay, source/image, and 70-file
  checksum evidence.
- `../review/go/` — independent final Go and no-weakening approval.

Passing this index qualifies the dependency candidate. It does not complete
OpenSpec task 5.4 and must not be read as a production go decision.
