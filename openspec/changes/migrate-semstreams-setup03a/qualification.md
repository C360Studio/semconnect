# SETUP 03A qualification and disposition

## Decision

**DRAFT / UNQUALIFIED; full migration and merge approval withheld.** The frozen target rejects an existing
federated Datastream create. Independent Go review approves the bounded implementation and baseline comparison,
with this P1 incompatibility left open. Passing local OGC, persistence and spatial gates does not waive it.

Reviewed implementation commit: `2d45b651b3bfac8ad4f96bceab7f5a0b3cf225c1`.
The reviewer binds 179 source/test/configuration/fixture/script inputs to that commit in
[review.md](review.md) and [signoff.json](evidence/review/signoff.json).

## Exact source identities

- Baseline SemConnect checkpoint: `55ea4121aba8658aeac9d70bdfe80f19c15fd4cd`.
- Baseline SemStreams: `v1.0.0-beta.160`;
  commit `8403a2218000e45a31c5132fbfe01af42ed04f14`;
  tree `9ed5dd3792bca63ce87ebf449a180add918f59ed`.
- Shared target: `v1.0.0-beta.162.0.20260930150212-8b99efe9c66a`;
  commit `8b99efe9c66a4faa4fa509f9f62cc6bad8392128`;
  tree `605f83cb8492eda3bd347ac30a211b9babf3931f`.
- Unchanged ETS commit: `d9caf33fcd0c4a3c1a582e8ba9b12b753277afd4`;
  TeamEngine 5.6.1; NATS 2.14.4.

SETUP 03A froze a main commit because beta.163 was unavailable. A subsequent tag or newer main is not an
independent upgrade option. [Module provenance](evidence/target-module.json) resolves to the same full target SHA;
[module verification](evidence/target-module-verify.log) passes. Both product binaries use this SemStreams pin.

## Gate results

| Gate | Baseline | Final target |
| --- | --- | --- |
| Unit/race, build and vet including integration tags | PASS | PASS |
| Full real-NATS integration/race | PASS | FAIL: sole active federation assertion |
| Three synchronized consumer reference cases | PASS | PASS |
| Eleven registered payload round-trips and host birth | Baseline adapters | PASS |
| Unchanged external ETS | 137/0/0 | 137/0/0 |
| Root-only SensorML bake | PASS | PASS |
| Fresh storage and normal no-write restart | PASS | PASS |
| Bbox hit/miss and polygon hit after second restart | PASS | PASS |
| Independent implementation/evidence review | Baseline comparison | APPROVED within scoped review |
| Full migration/merge qualification | Existing baseline | WITHHELD |

[Baseline evidence](evidence/baseline/README.md), [final r3 runtime evidence](evidence/target/README.md),
[paired references](evidence/reference-synchronized/summary.json), and
[final independent Go commands](evidence/review/final-checks.json) retain raw outputs and timestamps.
The final integration run has no race report and only the active federation failure.

The GitHub PR conformance job is skipped by its pre-existing unlabeled-PR trigger. This is not an ETS testcase
skip and is not represented as a CI pass. The recorded local Docker ETS run executes all 137 cases with zero skips.
Independent review compares every TestNG class/method/status tuple and verifies unchanged harness, fixture,
OpenAPI, declaration, filter and assertion inputs. No new bypass, skip, softened assertion or retry is introduced.

## Open compatibility blocker

`TestSetup03AFederatedDatastreamCreate` submits client ID
`foreign.remote.systems.csapi.datastream.setup03a-reference` through the real HTTP handler. Both reproductions
use explicit `c360.semconnect` graph authority and fresh NATS. Beta.160 returns HTTP 201 with the exact Location
and a nonzero read revision. The target returns HTTP 400; graph-ingest records `authority_foreign` on its local lane.

This reproduces an incompatibility between intended upstream ADR-102/ADR-104 authority fencing and SemConnect's
established client-supplied six-part Datastream identity contract. Upstream intent does not make the consumer
regression acceptable. [Paired evidence](evidence/federation-gap/summary.json) includes identical test-source identity.
The expected 201 remains an active assertion in the full integration suite. No narrower ID acceptance, authority
bypass, hidden retry, waiver, alternate pin or claim of completed qualification resolves it here.

Resolution requires the shared baseline owner and consumer owners to agree a qualified contract, then repeat
all affected gates. SemEngine extraction may use these findings; it gains no automatic dependency admission.

## Intended adaptations and reproduced differences

| Difference | Attribution and retained behavior |
| --- | --- |
| `Stop(context.Context)` | ADR-095; exact bounded drain/join before runtime cancellation. |
| Structured contract `MessageType` | ADR-103; wire mutation IDs, predicates and revision fences retained. |
| Eleven registered graph payloads | ADR-103; content floor, bound contracts, Graphable decoding and storage refs. |
| Consumer-owned graph host | Upstream SemConnect instruction; explicit retained components, shared gateway config. |
| `entity_id`, `bool`, `float` declarations | ADR-107; triple `@id`, standard IRIs and inverses unchanged. |
| Stable persisted platform identity | ADR-102/104; atomic provisioning; never overwrite another identity. |
| Foreign Datastream rejection | Reproduced blocking incompatibility; expected HTTP 201 retained. |
| Startup signal ownership | Reviewer found regression; failing-first repair and callback join tests pass. |

The CLI startup bridge sends a shutdown signal to runtime authority only during incomplete Start. Once Start
succeeds it detaches and joins the callback, preserving admitted work during bounded Stop. If a signal races
successful acquisition, bounded cleanup joins acquired resources before NATS closes. The component uses the Start
context for HTTP requests and joins owned server work with the exact Stop context. The upstream configuration
manager still accepts a duration; the host derives its remaining duration from the caller's shutdown deadline.

The deployment opts out of automatic suffixed identity creation through the explicit one-shot
`-provision-identity semconnect` operator action. It atomically creates `platform_identity` on fresh storage,
accepts an identical existing value, and rejects a different one without overwrite. Normal startup adopts it.
This preserves public `c360.semconnect.*` IDs and is not a mutation-authority bypass.

## Storage isolation and byte parity

Baseline volume: `semconnect-03a-beta160-20261001-fresh`.
Final target r3 volume: `semconnect-03a-8b99efe9-20261001-r3-fresh`.
Each was absent before its run; empty JetStream state was verified before identity provisioning and writes.
Normal restarts reuse only that revision's own volume. Neither revision opens the other's volume. Earlier target
attempts are preserved as history; final r3 includes the reviewed startup repair and has zero source-manifest drift.

System/Datastream/schema/global-and-scoped Observation proof bytes match before/after restart and across pins:
`4544c19899ccf1c9b461146be542c47dd85d4201aad5aeda2c9cf48946b63e32`.
Spatial response summaries match before/after the second normal restart and across pins:
`0dfd7b356c14534142a4b3644be7183130f562b9dfc23de7ba32fd89dab07f20`.
Readiness remains target/indexed revision 3/3; normal stops exit zero without OOM.
See [byte and source comparison](evidence/target/baseline-comparison.json).

## Diagnostic parity

Optional clustering warnings occur at both pins with the same missing `COMMUNITY_INDEX` and
`COMMUNITY_SUMMARIES` owner, messages and five-second retry interval. Neither composition enables graph-clustering;
query responders are installed independently. Differing warning counts reflect observation duration, not drift.
See [paired clustering evidence](evidence/target/optional-clustering-comparison.json).

The target adds nine canonical composition warning shapes: six optional unwatched outputs, two request inputs
without in-process producers, and graph-query disconnected in the declared graph. Its consumers are in the separate
HTTP process; zero composition errors and exercised request/reply results establish their disposition. The narrow
host removes the baseline stock binary's missing-persona warning; explicit identity provisioning also removes its
empty-config-key warning. [Paired diagnostic details](evidence/target/non-clustering-warning-details.json) and actual
readiness/proof results establish this attribution. Silence in logs is not treated as proof of health.

## Coverage limits

| Explicit critical group | Covered/statements | Percent |
| --- | ---: | ---: |
| Typed mutation adapter | 95/103 | 92.23% |
| Root-only projection | 19/21 | 90.48% |
| Immutable artifacts | 70/82 | 85.37% |
| Gateway lifecycle | 75/87 | 86.21% |
| Registry and payloads | 30/31 | 96.77% |
| Backend boot and stop | 69/81 | 85.19% |
| Gateway startup abort | 12/12 | 100% |
| Backend identity provisioning | 23/26 | 88.46% |

These groups meet 80%; individual functions remain visible, including `readSchemaArtifact` at 70.59%.
Whole-package totals are gateway 74.3%, backend 65.0%, HTTP CLI 21.5%; no whole-package 80% claim is made.
Container execution does not contribute to instrumented Go statement coverage. Tracked raw profiles and individual
counts are linked from [critical-coverage.json](evidence/target/critical-coverage.json).

The [OpenSpec strict validation](evidence/proposal-validation.json) passes. Its migration specification records
these acceptance gates; validation does not mean the unresolved federation requirement is satisfied.

## Handoff and remaining action

[Inventory](inventory.md) records combined production/retained-test closure 63/99 to 67/113 and extraction
obligations. [The SemEngine handoff](semengine-reference-cases.md) supplies the three consumer-owned cases and
known gap. The [SemEngine issue-8 handoff](https://github.com/C360Studio/semengine/issues/8#issuecomment-5929518484) is delivered.
OGC semantics and conformance stay in SemConnect. A later SemEngine switch requires a separate qualified contract
and separate implementation; no binary may mix SemStreams and SemEngine.
