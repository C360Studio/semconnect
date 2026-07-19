## Why

The requested SemStreams "515" release does not exist as `v1.0.0-beta.515`. An exact remote tag query on
2026-07-18 returned no beta.515 tag and identified `v1.0.0-beta.151` as the current release. This change resolves the
request to beta.151 and records that resolution rather than inventing a pin.

Beta.151 is not a routine dependency bump. Beta.150 added an unconditional structural predicate gate at graph-ingest,
and beta.151 changed the live owner read-modify-write path to trusted decode followed by authoritative write-time
validation. Semconnect publishes directly to the affected entity mutation subjects, so both changes require explicit
downstream qualification before beta.151 can replace the beta.149 candidate.

## Dependency

This change depends on `qualify-semstreams-beta149`, which in turn depends on `migrate-semstreams-beta147` and
ADR-S003. It does not rewrite their signed technical evidence. It inherits their canonical graph identity, OGC
ownership, foreign-edge, shutdown, replay, and conformance contracts. Its greenfield amendment supersedes retained
state and migration approval requirements for the active pre-v1 production gate.

## What Changes

- Align every executable SemStreams pin to `v1.0.0-beta.151`, peeled commit
  `ac75c322140fb2a6b55759d07a79874b4cb4d9cc`.
- Record beta.150 provenance (`cb42ac3d9743134f1a9194fba2824424833669f7` ->
  `d058bddfae0326487f0a86023ffe3d155992fd87`) and beta.151 provenance
  (`784f22dc8d549d7781b88a2878bb679112aad494` -> `ac75c322140fb2a6b55759d07a79874b4cb4d9cc`).
- Audit and disposition every beta.149-to-beta.151 commit, with live qualification for PR #554 and PR #567 and
  absence proofs for unconfigured rule, agentic, and Graphable producers.
- Prove all retained entity state and every semconnect-produced ID, predicate, reference, and foreign edge satisfy the
  canonical structural contract; prove malformed direct mutations fail atomically without a KV revision change.
- Prove trusted owner-RMW decode cannot launder resident poison and preserves valid create/update semantics.
- Re-run focused upstream tests, local Go/race/integration/vet/build gates, real shutdown, revision readiness,
  no-write replay, foreign-edge bake, and external `137 passed, 0 failed, 0 skipped` conformance.
- Add a greenfield production Compose bundle for clean NATS, canonical seed/query readiness, and normal restart
  persistence.
- Require a reviewed immutable bundle manifest and explicit product-owner and operator approval before production.

Non-goals are compatibility aliases, dual reads/writes, legacy predicate acceptance, graph cleanup in place, rule or
agentic adoption, public CS API changes, weakening conformance authority, or production authorization.

## Capabilities

### New Capabilities

- `semstreams-beta151-qualification`: Exact release alignment, complete delta disposition, structural fail-closed and
  trusted-RMW proof, state-preserving replay, external conformance, and greenfield production gates for beta.151.

### Modified Capabilities

None. The dependent changes remain normative for graph migration and CS API behavior.

## Impact

- `go.mod`, `go.sum`, conformance tag, commit, source, and image identities must move together during implementation.
- PR #554 is live on semconnect's `create_with_triples` and `update_with_triples` lanes. Delete carries no predicates;
  the configured Graphable input is unused, and semconnect publishes no triple-lane mutations.
- PR #567 is live on every entity update/merge RMW, but changes no persisted schema or public wire contract.
- The structural gate strengthens the existing canonical graph contract; it does not alter valid foreign-edge routing.
- Any need for compatibility code, retained-state repair, wipe, or behavior change reopens architecture review.
- Production remains no-go by default.
