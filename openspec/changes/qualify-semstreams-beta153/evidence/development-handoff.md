# Beta.153 development handoff

Recorded on 2026-07-19 by the go-developer and technical-writer roles.

## Identity

- Release: `v1.0.0-beta.153`
- Tag object: `ee011caee8a137b8dfb01d7634e9bb09519818b8`
- Peeled commit: `d2654e5a027138b8a9056863da5ed463ef767f37`
- Source tree: `dc7422aa9fd93ec446dca73a33e0c602b6601111`
- Qualified historical baseline: `v1.0.0-beta.151`

The module, conformance source, Compose build, image tags and embedded build metadata, and alignment tests use the
beta.153 identity. The alignment test was first observed red against the beta.151 files, then green after the pins
moved together.

## Downstream gates

The following commands passed on 2026-07-19:

```sh
go test ./... -count=1
go test -race ./... -count=1
go vet ./...
go build ./...
go test -tags integration ./gateway/cs-api \
  -run '^TestBeta153StructuralMutationContract$' -count=1 -v
```

The integration test ran against testcontainers NATS and covered canonical create/update, foreign-edge routing,
atomic invalid-mutation rejection, per-entity resident-poison isolation and repair, error classification, and true
no-op removal. No compatibility, migration, legacy, relaxed-validation, or product behavior code was added.

Focused conformance/deployment unit tests, rendered Compose validation, and persistence-script shell syntax also
passed.

## Greenfield Compose gate

The operator ran the following command on 2026-07-19:

```sh
SEMCONNECT_PORT=18081 \
  SEMCONNECT_NATS_VOLUME=semconnect-beta153-rehearsal-1 \
  EVIDENCE_DIR=openspec/changes/qualify-semstreams-beta153/evidence/operations/greenfield-compose \
  ./deploy/verify-persistence.sh
```

The typed clean-NATS preflight, canonical seed and query readiness, normal stop, same-volume restart, and persistence
parity passed. The stack was stopped normally afterward and the named volume was retained.

- Rendered Compose SHA-256: `ea027146881015e6e200b4868b48aca91df1fa1429df8d4b4cb1f454e6ed4fb9`
- Canonical proof SHA-256: `be60e53908d29de622da5f6877b6ba6294429aef2d5ffdb861b0097c1cf249cb`
- Evidence: [greenfield-compose](operations/greenfield-compose/)

## Exact upstream checkout gates

At SemStreams commit `d2654e5a027138b8a9056863da5ed463ef767f37`, these commands passed:

```sh
go test ./graph ./vocabulary ./processor/graph-ingest ./processor/rule \
  ./processor/agentic-loop ./pkg/fusion -count=1
go test -race ./processor/rule \
  -run '^TestHealthAndDataFlowAreRaceFreeUnderConcurrentReads$' -count=1
```

These results cover the live graph-ingest poison, watcher-retirement, memoization, and write-gate delta. The unused
rule, agentic-loop, and fusion additions compile and pass their focused package gates without being adopted by
semconnect.

## External conformance

The unchanged `./conformance/run.sh` completed as run `2026-07-19T13-27-02Z` with graph-index readiness at `80/80`,
an exercised and fully claimed foreign-edge lane, and TestNG `137 passed, 0 failed, 0 skipped`. The stack teardown
completed. Exact archived artifact hashes and source pins are recorded in
[external-conformance.json](external-conformance.json).

## Handoff disposition

All beta.153 qualification tasks are complete. Independent approval and no-weakening findings are recorded in
[review-handoff.md](review-handoff.md). This is ordinary qualification evidence, not a runtime manifest or
hash-bound deployment approval. The exact greenfield Compose bundle is production-ready for standard startup on
clean NATS without additional ceremony.
