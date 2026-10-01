# Independent Go review: SETUP 03A migration

Review owner: independent `go-reviewer` agent. Review date: 2026-10-01.
Comparison base: `d0d06e00bf05a545f30ceea798db1c2b1ee47d4f`.
Baseline checkpoint: `55ea4121aba8658aeac9d70bdfe80f19c15fd4cd`.
Reviewed implementation commit: `2d45b651b3bfac8ad4f96bceab7f5a0b3cf225c1`.
Framework target: `8b99efe9c66a4faa4fa509f9f62cc6bad8392128`, module
`v1.0.0-beta.162.0.20260930150212-8b99efe9c66a`.

## Decision

**Qualification and merge approval withheld.** The active federated Datastream regression remains a product
compatibility blocker even when all 137 external OGC assertions pass. **Implementation and baseline-comparison
evidence approved** for the bounded SemStreams adaptation, including the resolved startup repair and final r3
operational workload. This is not full migration qualification or merge approval.

Signed scope: 179 source/test/configuration/fixture/script inputs in
[reviewed-source.json](evidence/review/reviewed-source.json), manifest SHA-256
`b97173723b28cd43975da7a1fddd31a50b0987bed134afacf5eeb044e56ab4b8`.
Final audit: 2026-10-01T10:13:36Z. Both the reviewed manifest and the final runtime input manifest have zero drift.
The committed bytes match all 179 signed inputs; [commit verification](evidence/review/commit-verification.json)
records the check. No further correctness finding remains open beyond the federation blocker below.

## Findings

### P1: The target rejects the established federated Datastream identity contract

`TestSetup03AFederatedDatastreamCreate` asserts HTTP 201, unchanged client identity in `Location`, and a nonzero
exact graph revision for `foreign.remote.systems.csapi.datastream.setup03a-reference`. The identical assertion
passes at beta.160 and fails with HTTP 400 at the frozen target. Graph-ingest reports `authority_foreign` on the
local lane. The upstream ADR-102/ADR-104 authority fence is intentional; its incompatibility with SemConnect's
existing federated create contract is still a blocker. An upstream intent does not waive a consumer regression.

The independent full integration race run reproduced this as its sole failed test. Keep the test active and keep
the draft blocked until a qualified contract resolves it. Do not remove assertions, narrow accepted HTTP identity
semantics, hide a retry, bypass authority checks, or select a different pin without the shared baseline owner.
See [reproduction](evidence/federation-gap/summary.json) and
[independent checks](evidence/review/checks-authorized.json).

### Resolved P2: A shutdown signal must abort incomplete HTTP gateway startup

Review found that replacing `comp.Start(signalContext)` with a detached runtime context removed signal authority
during synchronous stream/ObjectStore provisioning. The graph backend already bridges bootstrap cancellation and
detaches that bridge after successful startup. The HTTP binary now does the same, joins the callback before
readiness, and invokes bounded cleanup if a signal races acquired resources. Failing-first evidence is retained in
[the startup red log](evidence/target-red/gateway-startup-signal.log). Independent race tests passed ten repetitions,
including blocked startup, post-start live authority, and signal/success acquisition race checks. The full final
Go gate rerun also passes these tests. This finding is resolved.

## Contracts reviewed

- The host registers all eleven consumer graph payload types with the registry and their existing projection
  contracts. Factories bind type/contract; wire data cannot choose a broader contract. Round-trip tests preserve
  identity, triples, storage reference and the `content` indexing floor. Shared gateway configuration preserves
  custom resource prefixes.
- Structured `MessageType` adaptation leaves exact create/reconcile/delete requests intact. Existing nonzero read
  revisions fence writes; stale operations conflict; absent targets stay absent. Conflict remains HTTP 409 and
  uncertain commit remains HTTP 503 with correlation. Tests count one request and retain failure expectations.
- Root-only SensorML projection preserves references while removing foreign-subject facts. Predicate declaration
  datatypes change to upstream neutral names; per-triple `@id`, standard IRIs and inverse mappings remain intact.
- Immutable artifacts retain canonical digest bytes and final graph `StorageReference`; duplicate creation verifies
  integrity without reconciling the artifact. Observation publish/read/purge and spatial semantics stay unchanged.
- The new backend composes explicit graph/index/query/ObjectStore component factories on SemStreams. Identity
  provisioning uses atomic KV create and verifies an existing value; a differing identity is never overwritten.
  Controlled stop keeps runtime authority alive through joins. Nil contexts, partial startup, caller stop bounds
  and custom-prefix births have explicit tests. The frozen configuration manager's duration stop remains an
  upstream seam; the host passes its remaining bounded cleanup budget.

## Independent evidence

The reviewer ran full `go test -race -count=1 ./...`, full
`go test -race -tags=integration -count=1 ./...`, `go vet -tags=integration ./...`, and `go build ./...`.
The unit race, vet and build runs pass. The integration race run fails only the active federation assertion; it contains
no race report. The first sandbox runs could not bind local listeners; both raw failures and the authorized reruns
are retained, without classifying environment denials as code failures or weakening tests. The final full checks
finished at 2026-10-01T10:10:53Z; [their commands and outcomes](evidence/review/final-checks.json) are preserved.
Source hashes did not drift during those checks. The synchronized real-NATS reference suite also passes
independently; identical receipt synchronization was run at both framework pins. Watchdogs only bound deadlock;
the mutation assertions no longer depend on a 50 ms callback scheduling assumption.

The XML audit compares every non-configuration `(class, method, status)` tuple, not just summary totals. Baseline
and target have the same 137 passing methods. `conformance/run.sh`, all four fixture files, ETS pin assignments,
OpenAPI and conformance declarations are unchanged from the comparison base. NATS remains exactly 2.14.4.

The independent persistence byte comparison matches before/after restart and across both framework pins:
`4544c19899ccf1c9b461146be542c47dd85d4201aad5aeda2c9cf48946b63e32`.
Index readiness is revision 3/3 before and after; normal stops report exit 0 and no OOM. Bbox hit, bbox miss and
polygon hit response bytes match across both pins and both restart phases. Fresh volume identities, empty-storage
preflight and same-volume restart evidence are recorded by the operator. The final r3 project/volume is isolated
from beta.160 and both earlier target attempts. The reviewer re-read the final r3 XML, canonical proof and spatial
response files, and verified the runtime manifest matches the signed source inputs.
See [independent tuple/byte audit](evidence/review/conformance-persistence-audit.json).

## Coverage and extraction boundary

Coverage uses executed Go statement blocks and enumerated critical function groups; it does not claim every legacy
handler has 80% coverage. [Raw profiles and the critical-group breakdown](evidence/target/critical-coverage.json)
are retained. The meaningful failure cases include exact fences, malformed/uncertain outcomes, immutable byte
mismatches, projection escape, invalid configuration, canceled startup and identity mismatch.

| Critical group | Covered / statements | Coverage |
| --- | ---: | ---: |
| Typed mutation adapter | 95 / 103 | 92.23% |
| Root-only projection | 19 / 21 | 90.48% |
| Immutable artifacts | 70 / 82 | 85.37% |
| Gateway lifecycle | 75 / 87 | 86.21% |
| Registry and payloads | 30 / 31 | 96.77% |
| Backend boot and stop | 69 / 81 | 85.19% |
| Gateway startup abort | 12 / 12 | 100% |
| Backend identity provisioning | 23 / 26 | 88.46% |

These enumerated critical groups meet the 80% gate. Individual functions remain visible; `readSchemaArtifact`
is 70.59%, and the whole gateway/backend/HTTP CLI package totals are 74.3%/65.0%/21.5%. Container qualification
exercises binary entrypoints without contributing to instrumented Go statement percentages. No blanket coverage
or behavior waiver is implied.

The stable bounded backend closure is distinct from the complete port boundary. Supplemental combined measurement
unions actual consumer and backend production packages and enumerates the tests of every retained framework
package under default/custom/race-custom tags. Actual observed custom constraints are `integration` and `live_llm`;
measurement lists imports and executes no paid workload. Canonicalization removes synthetic test mains and external
test-package duplicate source directories.

Combined framework closure grows from 63 production / 99 with retained tests to 67 / 113. The target's 63-package
bounded backend excludes `gateway`, `vocabulary/builtins`, `vocabulary/export` and `vocabulary/rulepacks`, which are
included in the combined 67-package boundary. These additional packages require extraction disposition. Broad
service and builtin-registry transitive dependencies remain measured obligations, not automatic SemEngine admission.

Frontend review is **N/A**: no Svelte/TypeScript, UI assets or frontend dependencies change. OGC semantics and
conformance ownership stay in SemConnect. No binary or dependency inventory mixes SemEngine with SemStreams.
