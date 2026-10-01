# Conformance Harness

This directory wires the [OGC Team Engine] conformance suite into local and CI
workflows for `semconnect`.

The harness boots NATS, `semstreams-backend`, `cs-api-server`, and Team Engine
on a shared Docker network; seeds CS API fixtures through the gateway's HTTP
write endpoints; invokes the CS API ETS through Team Engine's REST API; and
archives the TestNG XML report plus logs from every service.

[OGC Team Engine]: https://github.com/opengeospatial/teamengine

## Current Picture

Final SETUP 03A target r3 and the fresh beta.160 baseline both report:

```text
total=137 passed=137 failed=0 skipped=0
```

The migration remains **DRAFT / UNQUALIFIED**. The complete Go integration/race
suite preserves a separate foreign Datastream create assertion: beta.160 returns
201, while the frozen target returns 400 `authority_foreign`. ETS success does
not waive that existing consumer contract. Independent Go review approves the
scoped implementation and comparison evidence; merge qualification is withheld.

The execution pins are:

- Botts CS API ETS `0.1-SNAPSHOT` at
  `d9caf33fcd0c4a3c1a582e8ba9b12b753277afd4`.
- TeamEngine `5.6.1` and NATS `2.14.4`, with exact image pins in Compose.
- SemStreams `v1.0.0-beta.162.0.20260930150212-8b99efe9c66a`, commit
  `8b99efe9c66a4faa4fa509f9f62cc6bad8392128`, tree
  `605f83cb8492eda3bd347ac30a211b9babf3931f`.
- Pre-upgrade SemConnect checkpoint `55ea4121aba8658aeac9d70bdfe80f19c15fd4cd`
  uses beta.160 at `8403a2218000e45a31c5132fbfe01af42ed04f14`.

The consumer-owned `cmd/cs-graph-backend` registers all eleven resource types and
uses the gateway's configuration. The unchanged seed/readiness, root-only bake,
ETS assertions, fixtures, OAS and declarations remain protected. Independent
review compares every one of the 137 TestNG method/status tuples, not just totals.

Final r3 completed on 2026-10-01 using separate fresh storage. Normal stop/no-write
restart preserves exact graph, schema and observation proof bytes; bbox/polygon
responses also match after a second restart. Fresh-storage preflight precedes
explicit create-only identity provisioning. No baseline volume was opened by
the target. Earlier target attempts and older migrations remain historical.

See [qualification](../openspec/changes/migrate-semstreams-setup03a/qualification.md),
[review](../openspec/changes/migrate-semstreams-setup03a/review.md), and
[final runtime evidence](../openspec/changes/migrate-semstreams-setup03a/evidence/target/README.md).

The run exercises real gateway/framework behavior:

- graph reads through `graph.query.entity`, `graph.query.batch`,
  `graph.index.query.predicate`, and `graph.spatial.query.*`
- graph writes through typed `semstreams.graph.mutation/v1` create, reconcile,
  and exact-revision delete
- observation publish/readback through JetStream
- schema artifact storage through NATS ObjectStore and typed artifact entities
- OGC Common discovery, OpenAPI, content negotiation, and all claimed CS API
  conformance classes

## Running Locally

Prerequisites: Docker 20.10+ with BuildKit, `git`, `python3`, and `curl`. No
host JDK or Maven is required because the ETS Dockerfile builds Team Engine and
the test suite inside Docker.

```bash
# end-to-end run
./conformance/run.sh

# tear down a wedged stack
./conformance/run.sh --teardown-only

# override host ports when 4222 / 8081 / 8222 are busy locally
TE_HOST_PORT=8181 NATS_HOST_PORT=14222 NATS_MON_HOST_PORT=18222 ./conformance/run.sh

# force teardown on success; default keeps the stack for triage
KEEP_STACK=0 ./conformance/run.sh
```

Cold runs build the ETS and framework images. Warm runs reuse Docker BuildKit
cache.

Outputs land in `conformance/output/` (gitignored):

- `testng-report-<UTC>.xml` - TestNG XML; the conformance source of truth.
- `summary.txt` - human-readable TestNG counts.
- `compose-build-<UTC>.log` - image build logs.
- `seed-<UTC>.log` - fixture POST responses and readiness probes.
- `seed-evidence/index-readiness-<UTC>.jsonl` - fresh `GRAPH_STATUS`
  graph-index updates and the captured/final revision decision.
- `root-only-bake-<UTC>.txt` - root readable and embedded child absent.
- `teamengine-container-<UTC>.log` - Team Engine logs.
- `cs-api-server-container-<UTC>.log` - gateway logs.
- `semstreams-backend-container-<UTC>.log` - framework backend logs.
- `nats-container-<UTC>.log` - NATS logs.

Exit codes:

| Code | Meaning |
|------|---------|
| 0 | Harness ran end to end; read the TestNG XML for pass/fail counts. |
| 1 | Infrastructure failure: Docker, build, network, or healthcheck. |
| 2 | Team Engine REST API returned non-2xx on suite invocation. |

## Fixtures And Seed Step

`fixtures/` carries small CS-API-shaped documents used by `run.sh`.
The seed phase creates the resource graph that the ETS reads back:

- Systems and subsystem relationships.
- Datastreams, stored SWE result schemas, and one OMS observation.
- Procedures.
- Deployments and subdeployment relationships.
- Sampling Features.
- Properties.
- ControlStreams, command schemas, Commands, and Command Feasibility metadata.
- SystemEvents.

The seed phase is intentionally fatal: if a fixture cannot be created, cannot
be observed through the corresponding read endpoint, or the graph index does
not reach the captured post-seed `ENTITY_STATES` revision, the suite does not
start. Collection polling remains query evidence; health checks and fixed
delays do not substitute for the revision gate. This keeps failures shaped like
gateway/framework regressions instead of cascading Team Engine skips.

## Bumping Pins

`.ets-pin` carries upstream pins for both the ETS and the framework:

```ini
ETS_GIT_URL=https://github.com/Botts-Innovative-Research/ets-ogcapi-connectedsystems10.git
ETS_COMMIT=d9caf33fcd0c4a3c1a582e8ba9b12b753277afd4
ETS_COMMIT_DATE=2026-05-13
ETS_VERSION=0.1-SNAPSHOT
ETS_CODE=ogcapi-connectedsystems10
TEAMENGINE_VERSION=5.6.1

SEMSTREAMS_GIT_URL=https://github.com/C360Studio/semstreams.git
SEMSTREAMS_TAG_OBJECT=8b99efe9c66a4faa4fa509f9f62cc6bad8392128
SEMSTREAMS_COMMIT=8b99efe9c66a4faa4fa509f9f62cc6bad8392128
SEMSTREAMS_TREE=605f83cb8492eda3bd347ac30a211b9babf3931f
SEMSTREAMS_COMMIT_DATE=2026-09-30
SEMSTREAMS_VERSION=v1.0.0-beta.162.0.20260930150212-8b99efe9c66a
```

Bumping is intentional, not auto-pulled. SETUP 03A has already frozen the main
commit above because beta.163 was unavailable. Its tag-object compatibility field
contains that commit; it does not claim a release tag. Do not independently select
a newer revision while closing this migration.

### ETS Bump Procedure

1. Pick the new commit SHA, for example with
   `gh api repos/Botts-Innovative-Research/ets-ogcapi-connectedsystems10/commits/main --jq .sha`.
2. Edit `ETS_COMMIT` and `ETS_COMMIT_DATE`. Update `TEAMENGINE_VERSION` if
   the upstream Dockerfile changes its Team Engine version.
3. Run `./conformance/run.sh` locally and inspect the TestNG delta.
4. Include the TestNG delta in the PR description.

### Framework Bump Procedure

SETUP 03A uses the already frozen shared pin above. Dependencies, both binaries, deployment and conformance
pins move together only after the current baseline is archived. The source module for this migration is:

```text
github.com/c360studio/semstreams v1.0.0-beta.162.0.20260930150212-8b99efe9c66a
```

A later migration must record the shared owner decision, exact version/full commit/tree, affected imports and
contracts, then run the full Go/race/integration, unchanged ETS and fresh-storage persistence/spatial gates.
A passing ETS does not waive an active consumer integration failure. See the
[SETUP 03A qualification](../openspec/changes/migrate-semstreams-setup03a/qualification.md) for current results.

Beta.147 through beta.160 procedures and beta.160 accepted-risk authorization are historical. They do not authorize
this target. Every revision uses its own fresh NATS storage; never cross-open baseline and target volumes.

## NATS Config

`nats.conf` pins JetStream `max_file_store` and `max_memory_store`. The harness
uses exact NATS 2.14.4 and owns the server-side limits; SemStreams validates the
connected account's observed limits.

## Migrating Off Source Builds

When the OGC org adopts the ETS into
`opengeospatial/ets-ogcapi-connectedsystems10` and publishes a tagged image:

1. Replace `ETS_GIT_URL` and `ETS_COMMIT` in `.ets-pin` with `ETS_IMAGE`, for
   example `ghcr.io/opengeospatial/ets-ogcapi-connectedsystems10:1.0.0`.
2. Update `compose.yml`'s `teamengine` service from `build:` to `image:`.
3. Drop the `.vendor/ets` clone/build path from `run.sh`.

The graph backend is now the consumer-owned `cmd/cs-graph-backend`. A future registry image must contain this
composition and its eleven consumer registrations at the exact shared module pin; a stock framework image does
not satisfy that host contract.

## CI

`.github/workflows/conformance.yml` runs this harness on push to `main`, on
manual dispatch, and on PRs labelled `conformance`. The TestNG XML report is
uploaded as a workflow artifact for triage.

For this draft PR the existing label trigger skipped the conformance CI job. Local final r3 evidence above
records the executed suite; all 137 ETS cases ran and none were skipped.
