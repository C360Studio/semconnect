# Beta.159 development handoff

Recorded on 2026-07-31 by the go-developer and technical-writer roles.

## Identity

- Release: `v1.0.0-beta.159`
- Tag object: `ba2c4f8e03bb42c56319b2363ca89f4ab0f9ccec`
- Peeled commit: `8813270c5ba441286d9120cba82fbf72bdcf9a6c`
- Source tree: `c9a8014cbf6b91837769c8fb8a3dea051f6f11d9`
- Qualified historical baseline: `v1.0.0-beta.153`

The module, conformance source, Compose build, image tags and embedded build metadata, and alignment tests use the
beta.159 identity. Alignment tests reject mixed active requirements and mismatched annotated-tag provenance.

## Downstream gates

The following commands passed on 2026-07-31:

```sh
go test ./...
go test -race ./...
go test -race ./gateway/cs-api ./conformance/cmd/index-readiness ./conformance ./deploy
go test -tags=integration ./gateway/cs-api -run '^TestBeta159StructuralMutationContract$'
go vet ./...
go build ./...
go mod verify
git diff --check
```

The integration test ran against live testcontainers NATS and covered canonical create/update, foreign-edge routing,
atomic invalid-mutation rejection, scoped poison and out-of-band recovery, classified query behavior, six-field add
deduplication, and true remove no-op behavior. Duplicate add and absent remove both preserve entity and bucket
revisions. No compatibility, migration, legacy, relaxed-validation, or public HTTP behavior code was added.

The removed request/reply status seam was replaced by an `UpdatesOnly` watcher on the beta.159 `GRAPH_STATUS`
graph-index key. Unit tests fail closed on pre-bootstrap/degraded/reset-required state, insufficient revision coverage,
target regression, malformed or deleted status, watch closure, and timeout. The observations stream defaults to a
positive 1 GiB byte cap, 30-day age, and `DiscardOld`; zero and negative bounds fail validation.

## Clean-volume Compose gate

The operator ran the persistence verifier on a clean named volume on 2026-07-31. Empty preflight, canonical seed and
query readiness, normal stop, same-volume restart, and persistence parity passed. This gate does not exercise or
qualify an existing beta.153 volume.

- Rendered Compose SHA-256: `aba156bb23c155e9c8a37d728997133adfc9782f09c1e46ecadec497645e0bac`
- Canonical proof SHA-256: `be60e53908d29de622da5f6877b6ba6294429aef2d5ffdb861b0097c1cf249cb`
- Evidence: [persistence](operations/persistence/)

## Exact upstream checkout gates

At SemStreams commit `8813270c5ba441286d9120cba82fbf72bdcf9a6c`, this focused race command passed:

```sh
go test -race ./graph/readiness ./natsclient ./config ./service \
  ./processor/graph-ingest ./processor/graph-index ./processor/graph-query
```

It covers the exposed readiness, acquisition/startup, graph mutation/index/query, and request/reply seams. GraphQL
`graphSummary`, fusion, embedding, clustering, graphview, rule, and agentic changes remain absent from semconnect's
configuration/import surface and create no product requirements.

## External conformance

The unchanged `./conformance/run.sh` completed as run `2026-07-31T18-13-41Z`. The live `GRAPH_STATUS` watcher reached
target/indexed revision `80/80` in two updates; the hosted-child foreign-edge lane was exercised with zero unclaimed
or dropped edges; TestNG reported `137 passed, 0 failed, 0 skipped`; and stack teardown completed. Exact artifact
hashes and source pins are recorded in [external-conformance.json](external-conformance.json).

## Handoff disposition

All beta.159 qualification tasks are complete. Independent approval and no-weakening findings are recorded in
[review-handoff.md](review-handoff.md). This is ordinary clean-volume qualification evidence, not an in-place
beta.153 migration claim, runtime manifest, or hash-bound deployment approval. The checked-in beta.159 Compose bundle
is production-ready for standard startup on clean NATS without additional ceremony.
