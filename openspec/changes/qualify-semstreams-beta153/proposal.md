## Why

SemStreams `v1.0.0-beta.153` contains graph-ingest bug and performance fixes on semconnect's live mutation path.
The release also adds rule, agentic-loop, and fusion behavior that semconnect does not configure or import. The pin
must therefore move only after exact provenance, live-path regression, greenfield Compose, and external CS API
conformance gates pass.

## What Changes

- Align every executable SemStreams pin to `v1.0.0-beta.153`, peeled commit
  `d2654e5a027138b8a9056863da5ed463ef767f37`.
- Qualify graph-ingest validation memoization, per-entity poison response, and contract-guard watcher retirement on
  semconnect's entity mutation lanes.
- Compile and test the rule health race fix and the unused additive rule, agentic-loop, and fusion changes.
- Re-run full downstream Go, fresh-volume Compose smoke/restart persistence, and unchanged external `137/0/0` gates.

## Non-goals

- No migration, old-state support, compatibility alias, dual read/write, relaxed validation, or legacy code.
- No adoption of rule, agentic-loop, or fusion features.
- No public CS API, OpenAPI, conformance-claim, fixture-intent, or ETS-authority change.
- No runtime-unused approval manifest, product-owner hash ceremony, or separate production authorization workflow.

## Capabilities

### New Capabilities

- `semstreams-beta153-qualification`: exact beta.153 alignment and proportional downstream qualification.

### Modified Capabilities

None.

## Impact

Semconnect consumes graph-ingest create, update, and delete mutation subjects; those paths carry the principal risk.
The dependency-only change has no frontend work. If qualification requires product behavior or compatibility code,
implementation stops and architecture review reopens.
