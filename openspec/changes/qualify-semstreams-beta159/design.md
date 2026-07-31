## Context

Beta.153 is the qualified historical baseline. Beta.159 is an annotated release with exact provenance:

- tag object: `ba2c4f8e03bb42c56319b2363ca89f4ab0f9ccec`;
- peeled commit: `8813270c5ba441286d9120cba82fbf72bdcf9a6c`;
- source tree: `c9a8014cbf6b91837769c8fb8a3dea051f6f11d9`.

The baseline beta.153 commit is `d2654e5a027138b8a9056863da5ed463ef767f37`, tree
`dc7422aa9fd93ec446dca73a33e0c602b6601111`.

## Upstream delta and exposure

- Removed `graph.index.query.status` and distributed readiness through `GRAPH_STATUS` KV: live breaking seam; migrate
  the conformance gate to an `UpdatesOnly` watcher.
- Readiness licenses health rather than absence, and caught-up producers publish revision state: require bootstrap
  completion, healthy gate evaluation, and revision coverage.
- Framework bucket catalog, acquisition guards, component-start barrier, and fail-closed boot: runtime-exposed;
  existing port names are valid, and clean boot/restart must pass.
- Ordinary streams must declare storage bounds: give observations a positive 1 GiB cap, 30-day age, and `DiscardOld`.
- `ENTITY_STATES` history is one revision: relevant to retained old volumes, but no in-place claim is made because
  pre-v1 production is clean-volume only.
- Six-field triple-add deduplication and honest remove response: live mutation semantics; prove duplicate add and
  absent remove do not advance revisions.
- Query response-envelope detection and batch `missing[]` behavior: live wire exposure; prove existing HTTP behavior
  remains unchanged.
- GraphQL `graphSummary`, fusion, embedding, clustering, graphview, rule, and agentic changes: not configured or
  imported; compile/audit only.

Semconnect enables graph-ingest, graph-index, spatial and temporal indexes, and graph-query. Product writes use
`graph.mutation.entity.create_with_triples`, `update_with_triples`, and `delete`; the structural regression also
exercises direct triple add/remove. The unused feature families above are outside the composition.

## Decisions

### Pin one exact release identity

The Go module, conformance source tag and commit, Compose build context and labels, documentation, and alignment tests
shall agree on beta.159 and its peeled commit. Mixed identities fail before runtime qualification.

### Consume readiness as fresh distributed state

The harness shall open `GRAPH_STATUS`, watch the graph-index key with `jetstream.UpdatesOnly()`, and capture two
consecutive equal non-zero target updates. It shall proceed only after bootstrap is complete, the framework readiness
gate is healthy, and indexed revision covers both the captured and currently observed target. A retained pre-seed
value must not satisfy the gate. Tombstones, malformed updates, target regression, degraded/reset-required state,
watch closure, and timeout fail closed with JSONL evidence.

### Declare a bounded observations stream

The CS API observations stream shall retain file-backed facts for at most 30 days and 1 GiB under `LimitsPolicy`,
discarding the oldest messages when a bound is reached. Configuration with a zero or negative byte bound is invalid.
This satisfies beta.159's ordinary-stream contract without changing HTTP observation behavior.

### Preserve canonical graph behavior without compatibility code

Semconnect retains six-part entity IDs, three-part predicates, canonical `@id` references, registered foreign edges,
atomic invalid-input rejection, and per-entity poison classification/recovery. Repeated identical six-field triple
adds and absent removes must be true no-writes. No alias, repair lane, migration, dual-path, or relaxed parser is
permitted. Production remains pre-v1 greenfield: standard Compose startup targets clean NATS storage.

### Keep production proof proportional

The checked-in Compose bundle is statically validated, then exercised on a fresh named volume. It must pass empty
preflight, canonical seed and query readiness, normal stop, identical-volume restart, and persistence parity. This
does not qualify startup over a beta.153 volume. Evidence is ordinary qualification material, not a runtime manifest
or role-specific hash approval.

### Preserve external conformance authority

A fresh-volume external run must report exactly `137 passed, 0 failed, 0 skipped`. Independent review must confirm
that the ETS, fixtures, OpenAPI, declarations, filters, skips, parser, and harness authority were not weakened.

## Verification gates

1. Exact pin, annotated-tag, commit, and source-tree alignment.
2. Full downstream Go test, race, vet, module verification, and build.
3. Live-NATS structural integration plus focused upstream race tests for exposed packages.
4. Deterministic Compose validation plus clean-volume smoke and same-volume restart persistence.
5. External `137/0/0` with independent no-weakening review.

## Risks / Trade-offs

- A retained readiness value could falsely license Team Engine: `UpdatesOnly` requires live post-subscription evidence.
- An unbounded observation stream now fails startup: the default and production bundle use 1 GiB/30d/`DiscardOld`.
- Bucket catalog and start-order changes could fail composition: clean boot and same-volume restart cover the path.
- `ENTITY_STATES` history changes can affect old volumes: qualification intentionally makes no old-volume claim.
- Deduplication could hide writes or advance state: direct add/remove assertions compare entity and bucket revisions.

## Rollback

Before deployment, a failed candidate leaves beta.153 as the qualified historical pin. There is no data migration or
legacy rollback. After beta.159 writes a production volume, switching framework versions requires its own
qualification; operational recovery preserves storage and corrects the same-version deployment.

## Architecture disposition

Approved without a new ADR because ADR-S003's durable product boundary and clean-volume deployment model do not
change. Any compatibility path, old-volume support, or public behavior change reopens architecture review.
