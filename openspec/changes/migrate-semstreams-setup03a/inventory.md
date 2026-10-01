# SETUP 03A import and contract inventory

## Scope and direct imports

The original inventory was captured before the dependency change on 2026-10-01. Baseline checkpoint
`55ea4121aba8658aeac9d70bdfe80f19c15fd4cd` uses SemStreams `v1.0.0-beta.160`. Target comparison uses the
exact frozen SETUP 03A revision in [qualification](qualification.md). Measurement records retain the source
commit/status observed before the reviewed implementation was committed as `2d45b651b3bfac8ad4f96bceab7f5a0b3cf225c1`.

At baseline, thirteen SemStreams packages are directly imported by production Go source; three additional packages
are imported only by tests. The direct import inventory uses actual Go import declarations, not matches in comments
or test data.

| Imported package suffix | Production use | Retained contract / migration disposition |
| --- | --- | --- |
| `component` | Lifecycle, typed ports | Adapt Stop context; preserve one provider. |
| `gateway` | HTTP handler registration | Carry; OGC handlers stay consumer-owned. |
| `graph` | Reads, hydration, mutations | Preserve exact revisions and registered births. |
| `graph/geo/geojson` | Spatial requests, geometry, OMS | Carry exact coordinate and polygon behavior. |
| `graph/readiness` | Conformance index readiness probe | Carry authoritative/indexed revision gate through restart. |
| `message` | Triples, types, envelopes, storage refs | Register types; preserve wire shape. |
| `natsclient` | Requests, errors, JetStream | Adapt lifecycle; keep publish acknowledgement. |
| `payloadregistry` | Exported OMS payload registration | Add eleven graph types to the actual host registry. |
| `pkg/errs` | Invalid/transient/internal mapping | Carry 400/503/500 boundaries plus local 409/uncertain commit. |
| `pkg/projection` | Contracts, fact validation | Adapt structured MessageType and bound attributes. |
| `pkg/types` | Six-part entity ID validation | Preserve external IDs; verify target segment interpretation. |
| `vocabulary` | Metadata, inverse relations | Adapt neutral datatypes; preserve relationships. |
| `vocabulary/export` | JSON-LD and domain IRI registration | Preserve semantic-web export behavior under ADR-107. |
| `config` (tests) | Deployment configuration | Adapt upstream boot/identity changes. |
| `payloadbuiltins` (tests) | OMS decoding, NATS tests | Explicit target host dependency. |
| `processor/graph-ingest` (tests) | Real-NATS mutation lifecycle | Becomes an explicit backend host dependency. |

Key consumer files are `gateway/cs-api/{component,graph_mutations,projection_contracts,schema_artifacts,spatial}.go`,
`parser/sensorml/`, `message/oms/`, `vocabulary/`, `cmd/cs-api-server/main.go`, and `conformance/cmd/index-readiness/`.
Configuration and binary pins in `conformance/`, `deploy/`, the module files, and Docker definitions must move
together.

The target directly imports 27 framework packages. In addition to the baseline production packages, it imports
`composition`, `config`, `metric`, `payloadbuiltins`, `pkg/projection/contract`, `service`, `types`,
`storage/objectstore`, `vocabulary/builtins`, and the five `processor/graph-*` packages listed below.
`cmd/cs-graph-backend` explicitly composes graph-ingest, graph-index, graph-index-spatial,
graph-index-temporal and graph-query. `pkg/projection/contract` provides the neutral contract seam.
The configuration, builtin and processor imports previously used only in tests now participate in production.
The exact direct-import list is retained in the target closure JSON.

## Dependency closure measurement

```bash
python3 scripts/setup03a/dependency-closure.py \
  --label setup03a-frozen-combined-supplement \
  --output openspec/changes/migrate-semstreams-setup03a/evidence/dependency-combined-target \
  --raw-output /tmp/semconnect-setup03a-target-combined
```

The original [baseline measurement](evidence/dependency-baseline/closure.json) remains unchanged. The additive
combined scope was measured against a clean archive of baseline checkpoint `55ea412`, supplying only the updated
measurement script, and against target source. [Baseline supplement][combined-baseline],
[target supplement][combined-target] and their command lists record provenance, package sets and exact modules.
No dependency was changed by measurement; the script uses `go list -mod=readonly`.

| Framework package scope | Baseline production / with tests | Target production / with tests |
| --- | ---: | ---: |
| Actual consumer and its own retained tests | 39 / 55 | 67 / 67 |
| Stable bounded backend and every retained package's tests | 61 / 97 | 63 / 110 |
| Combined consumer/backend and every retained package's tests | 63 / 99 | 67 / 113 |
| Stock upstream binary, comparison only | 108 / not measured | 117 / not measured |

The combined scope is the complete extraction boundary. It unions actual consumer production framework directories
with stable backend production directories, then enumerates every retained framework package's tests under the
union of default, custom and race-plus-custom tags. The target bounded backend excludes `gateway`,
`vocabulary/builtins`, `vocabulary/export` and `vocabulary/rulepacks`; the combined 67 includes them.
The actual consumer scope now includes the new graph host. The stock target binary is not shipped by this migration.

Observed custom tags are `integration` and `live_llm`. These listings execute no tests or paid LLM workload.
Synthetic test mains and external `_test` duplicates are canonicalized to source package directories. Measurements
are host-specific darwin/arm64, not all-platform compile claims. Raw non-test framework source lines count all
non-test `.go` files in each retained directory, including inactive platform files: combined production changes
133,843 to 123,481; combined with retained tests changes 209,590 to 231,684.

## Added dependency paths and dispositions

[Exact delta and import paths](evidence/dependency-delta.json) record nine production additions and five removals.
All paths below start at the consumer graph host; arrows denote static imports, not runtime feature activation.
These are explicit extraction obligations; no package is automatically admitted into SemEngine.

| Added framework package | Import path after host | Proposed extraction disposition |
| --- | --- | --- |
| `composition` | direct | Adapt required composition validation and diagnostics. |
| `pkg/projection/contract` | consumer `gateway/cs-api` | Carry neutral contract; retain consumer OGC definitions. |
| `internal/lifecyclecleanup` | `processor/graph-index` | Carry bounded lifecycle cleanup behavior. |
| `vocabulary/builtins` | direct | Audit/split required vocabulary registration. |
| `vocabulary/rulepacks` | `vocabulary/builtins` | Audit builtin reach; exclude unused rulepack capability. |
| `internal/agentterminal` | `service` → `agentic/agentrun` | Split service dependency; exclude agent runtime. |
| `internal/deliverylane` | `service` → `agentic/agentrun` | Split service dependency; exclude agent runtime. |
| `internal/logforwarderpolicy` | `service` | Audit logging need; adapt or exclude at service split. |
| `internal/looptoken` | `component` → `agentic` | Split agent coupling from retained component contract. |

Production removes `engine`, `flowstore`, `model/wire/responses`, `processor/agentic-dispatch` and
`processor/agentic-model`. Test-inclusive closure adds eighteen and removes four package directories; exact sets are
in the delta JSON. Test-only reach includes broad `componentregistry`, graph/HTTP/lifecycle gateways, OTEL adapters
and output, graph research, and agent-loop helpers. Preserve those test obligations until an explicit extraction
contract replaces or excludes the corresponding capability; imports alone do not authorize shipping them.

## Framework obligations and gaps for extraction planning

| Finding | Evidence / owner | Disposition |
| --- | --- | --- |
| Eleven unregistered birth types | ADR-103 | Consumer host registers and round-trips all types. |
| Exact Stop context | ADR-095 | Adapt drain/join; retain lifecycle and restart proof. |
| Structured MessageType | Projection contract | Adapt typed literals and comparisons. |
| Neutral declaration datatypes | ADR-107 | Attribute spelling delta; preserve IRIs and triple markers. |
| Identity/config semantics | ADR-104 | Atomic provisioning on fresh storage; preserve restart identity. |
| Broad framework imports | Combined package lists | Explicit carry/adapt/repair/exclude decisions remain required. |
| Product-specific OGC surface | SemConnect ADR-S003/S004 | Keep consumer-owned; exclude from generic engine. |
| Schema garbage collection absent | Immutable artifact contract | Defer cleanup; preserve immutable orphans. |
| Foreign Datastream authority | Paired HTTP 201 → 400 | Blocking incompatibility; keep assertion active. |
| ETS/persistence/spatial/review | Final r3 comparison | Scoped approval; full qualification withheld. |

Existing OMS typed-result/quality, richer SensorML and Part 3 binding limits remain unchanged. This migration
neither expands those claims nor implements substitutes. The upstream configuration manager's duration-based
Stop remains an adaptation seam; the host derives its budget from the caller's bounded shutdown context.

A compile success is not closure approval. Each newly reached framework package needs an explicit retained need or
extraction disposition; no SemEngine dependency is admitted. A reproduced target regression stays a blocker unless
an intended upstream contract explains it and SemConnect's preserved behavior remains satisfied. Do not silently
select a later upstream fix: the owner-frozen pin remains fixed.

[combined-baseline]: evidence/dependency-combined-baseline/closure.json
[combined-target]: evidence/dependency-combined-target/closure.json
