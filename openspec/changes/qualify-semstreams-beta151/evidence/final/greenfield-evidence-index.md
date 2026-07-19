# Greenfield task 6.3 evidence

The immutable task 6.3 record is `greenfield-production-manifest.json`. Its exact
checked-in UTF-8 SHA-256 is recorded in `evidence-hashes.sha256`; the manifest
does not contain task 6.4 decisions. Product-owner and operator decisions must
be detached records that both name that same digest.

## Bound evidence

- Source: semconnect commit `dc657825af421874f9e56b04874479b4cf644eb1`
  and SemStreams `v1.0.0-beta.151` commit `ac75c322140f...`, tree
  `120eeb353afb...`.
- Images and build inputs: `../operations/greenfield-compose/images.txt` and
  `../operations/greenfield-compose/inputs.sha256`.
- Rendered deployment: `../operations/greenfield-compose/compose.rendered.yml`
  at SHA-256 `a86d670d38f...`, CS API rehearsal port `18080`, and named volume
  `semconnect-beta151-rehearsal-5`.
- Greenfield proof: typed zero-state JetStream summary, canonical seed query,
  scoped normal shutdown, same-volume restart, and byte-identical before/after
  proof in `../operations/greenfield-compose/`.
- Qualification: authoritative run `2026-07-18T17-09-45Z` at `137/0/0`, the
  no-weakening audit, and the independent Go reviewer record.

## Scope

This is the first deployment on a new NATS namespace. NATS remains private at
`nats://nats:4222` on account `$G`, with the domain unset and no credentials or
published NATS port. The bundle has no migration, reseed or import of old
state, destructive operation, retained-state compatibility path, volume
removal, or prior deployment rollback. Its canonical fixture is a first-start
smoke seed only.

Task 6.3 completes technical manifest assembly only. Production remains
unauthorized until task 6.4 contains matching product-owner and operator `go`
decisions over the manifest digest.
