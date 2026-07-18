## Why

Semconnect's beta.147 rehearsal proved graph, conformance, and no-write replay parity, but normal backend shutdown
still reported a false graceful-shutdown failure because `HeartbeatService.Stop` was not idempotent after parent-context
cancellation. SemStreams issue #549 made that defect a production blocker. SemStreams `v1.0.0-beta.149` contains the
reviewed fix and must be qualified as the replacement dependency candidate before any production decision.

## Dependency

This change depends on `migrate-semstreams-beta147`. It does not replace ADR-S003, rewrite its signed evidence, or
repeat its graph-state ownership migration. Beta.149 qualification inherits the canonical graph, owned OGC package,
retained-state, and destructive-cutover contracts established by that change.

## What Changes

- Align the Go module and conformance backend to SemStreams `v1.0.0-beta.149`, commit
  `7db0cdcb21577eaa52eb842c4ffb06a854f9a9b2`.
- Record the complete beta.147-to-beta.149 delta, including the issue #549 fix in PR #550 at
  `cc34be3d565405d86d9e13375013ce3522fa1d5f` and the unrelated agentic/rule commits.
- Prove the issue #549 ordering with a real graceful stop: parent-context cancellation before `Manager.StopAll`, clean
  exit status, no forced kill, and no false heartbeat/metrics-forwarder shutdown errors.
- Re-prove graph revision readiness and normalized no-write replay parity after the clean stop/start.
- Re-run Go, race, integration, vet, build, and the external Botts ETS at exactly
  `137 passed, 0 failed, 0 skipped` without changing tests, fixtures, OpenAPI, or conformance claims.
- Keep production cutover unauthorized until the inherited beta.147 manifest, retained-state, approval, and operator
  gates plus this change's evidence are all explicitly approved.

Non-goals are adopting beta.149 agentic-loop, rule-engine, or advertised-tool behavior; changing CS API wire
contracts; changing graph identities or persisted layouts; weakening a test or claim; or authorizing production.

## Capabilities

### New Capabilities

- `semstreams-beta149-qualification`: Exact dependency alignment, bounded upstream-delta disposition, coordinated
  shutdown proof, graph replay proof, and downstream conformance gates for beta.149.

### Modified Capabilities

None. The dependent beta.147 change remains the normative source for graph migration and external CS API behavior.

## Impact

- `go.mod`/`go.sum` and the conformance SemStreams tag/commit pins move together.
- No semconnect production package or public API is expected to change; any required compatibility code is a blocker.
- Existing beta.147 graph state is expected to remain compatible because beta.147..beta.149 changes no graph,
  natsclient, message, vocabulary, or module dependency contract used by semconnect.
- The release decision gains a mandatory clean-shutdown evidence lane. Production remains no-go by default.
