## Context

Beta.151 is the current qualified baseline. Beta.153 is an annotated release with exact provenance:

- tag object: `ee011caee8a137b8dfb01d7634e9bb09519818b8`;
- peeled commit: `d2654e5a027138b8a9056863da5ed463ef767f37`;
- source tree: `dc7422aa9fd93ec446dca73a33e0c602b6601111`.

The baseline beta.151 commit is `ac75c322140fb2a6b55759d07a79874b4cb4d9cc`, tree
`120eeb353afb7d07aa1b3180de05f75494bac1a8`.

## Upstream delta and exposure

| Commits | Change | Semconnect disposition |
|---|---|---|
| `d12daf34` | Memoized predicate parsing and collapsed write-path validation | Live graph-ingest path; qualify |
| `7485c785` | Per-entity poison response and guard-watcher retirement | Live graph-ingest path; highest risk |
| `2b960e00` | Race-free rule Health/DataFlow getters | Rule absent; focused upstream race test |
| `08579384` | Rule `length_gte` and `length_lte` | Rule absent; compile/test only |
| `e88dc5e0` | Agentic terminal-reason fact | Agentic-loop absent; compile/test only |
| `f54c06bd` | Opt-in fusion graph projection | Fusion unused; compile/test only |
| `4568b8de`, `a1087ddd`, `d2654e5a` | OpenSpec/release bookkeeping | No executable downstream effect |

Semconnect enables graph-ingest, graph-index, spatial and temporal indexes, and graph-query. Product writes use
`graph.mutation.entity.create_with_triples`, `update_with_triples`, and `delete`; the configured Graphable input is
unused. Rule, agentic-loop, graph-clustering, and fusion are not part of the composition.

## Decisions

### Pin one exact release identity

The Go module, conformance source tag and commit, Compose build context and labels, documentation, and alignment tests
shall agree on beta.153 and its peeled commit. Mixed identities fail before runtime qualification.

### Preserve canonical behavior without compatibility code

Semconnect retains six-part entity IDs, three-part predicates, canonical `@id` references, and registered foreign
edges. Invalid direct mutations must still fail atomically. No alias, repair, migration, dual-path, or relaxed parser
is permitted. Production remains pre-v1 greenfield: standard Compose startup targets clean NATS storage.

### Qualify the live graph-ingest delta at both boundaries

The downstream live-NATS structural test proves canonical create/update, foreign-edge routing, atomic rejection,
resident-poison non-laundering and classification, and true no-op removal. Focused upstream tests cover poison
scoping, snapshot-watcher retirement, predicate memoization, write gates, and the rule health race. Unused additions
must compile and pass their package tests but do not create product requirements.

### Keep production proof proportional

The checked-in Compose bundle is statically validated, then exercised on a fresh named volume. It must pass empty
preflight, canonical seed and indexed/direct query readiness, normal stop, identical-volume restart, and persistence
parity. These are ordinary qualification records. Nothing at runtime or deployment reads an approval manifest, so no
manifest or role-specific hash approval is required.

### Preserve external conformance authority

A fresh-volume external run must report exactly `137 passed, 0 failed, 0 skipped`. Independent review must confirm
that the ETS, fixtures, OpenAPI, declarations, filters, skips, and parser were not weakened to obtain green.

## Verification gates

1. Exact pin and source-tree alignment.
2. `go test ./...`, `go test -race ./...`, `go vet ./...`, and `go build ./...`.
3. Live-NATS structural integration and focused upstream regression tests.
4. Deterministic Compose validation plus fresh-volume smoke and restart persistence.
5. External `137/0/0` with independent no-weakening review.

## Risks / Trade-offs

- Memoization could accept warmed invalid predicates: cold, warm, concurrent, and cache-cap tests must remain green.
- Watcher retirement could weaken graph readiness: live startup, canonical query, clean stop, and restart tests cover
  the configured path.
- Per-entity poison behavior could alter mutation classification: downstream poison non-laundering proof remains
  required even though production starts clean.
- Unused additive packages could create accidental exposure: configuration and import audits must remain negative.

## Rollback

Before deployment, a failed candidate leaves beta.151 as the qualified pin. There is no data migration or legacy
rollback. After beta.153 writes a production volume, switching framework versions requires its own qualification;
operational recovery stops the stack while preserving the volume and corrects the same-version deployment.

## Architecture disposition

Approved for implementation subject to the gates above. No new ADR is justified because the durable product
boundary, canonical graph contract, and greenfield deployment model do not change.
