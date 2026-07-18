# Conformance Harness

This directory wires the [OGC Team Engine] conformance suite into local and CI
workflows for `semconnect`.

The harness boots NATS, `semstreams-backend`, `cs-api-server`, and Team Engine
on a shared Docker network; seeds CS API fixtures through the gateway's HTTP
write endpoints; invokes the CS API ETS through Team Engine's REST API; and
archives the TestNG XML report plus logs from every service.

[OGC Team Engine]: https://github.com/opengeospatial/teamengine

## Current Picture

The authoritative post-review fresh-volume run `2026-07-18T17-09-45Z`
against SemStreams beta.151 is:

```text
total=137 passed=137 failed=0 skipped=0
```

The migration candidate pins are:

- Botts CS API ETS `0.1-SNAPSHOT` at `d9caf33fcd0c4a3c1a582e8ba9b12b753277afd4`.
- TeamEngine `5.6.1`, bundled by the ETS Dockerfile.
- semstreams backend `v1.0.0-beta.151` at
  `ac75c322140fb2a6b55759d07a79874b4cb4d9cc`.

The run reached graph-index revision `80/80` before Team Engine and the
foreign-edge bake passed with the hosted-child lane exercised. The reviewed
same-stack beta.151 rehearsal scanned retained `ENTITY_STATES` with zero poison,
froze writers at `118/118`, stopped the backend with normal SIGTERM and exit
code zero, returned to `118/118`, and reproduced all 12 normalized probes
without writes. Structural rejection and trusted-RMW atomicity gates also
passed. Evidence is indexed under
`openspec/changes/qualify-semstreams-beta151/evidence/`.

This does not authorize production. The inherited ADR-S003 immutable cutover
manifest, literal deployment values, product-owner/operator approvals, and
production execution/archive remain open. The beta.141, beta.147, and beta.149
results remain historical evidence; their signed records are not rewritten.
The earlier beta.151 run `2026-07-18T17-06-05Z` is also non-authoritative
rehearsal evidence because it began before final reviewer signature.

The run exercises real gateway/framework behavior:

- graph reads through `graph.query.entity`, `graph.query.batch`,
  `graph.index.query.predicate`, and `graph.spatial.query.*`
- graph writes through `graph.mutation.entity.create_with_triples`,
  `graph.mutation.entity.update_with_triples`, and
  `graph.mutation.entity.delete`
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
- `seed-evidence/index-readiness-<UTC>.jsonl` - revision-based graph-index
  status samples.
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
SEMSTREAMS_COMMIT=5cc22c109594e48b7f1cec04bcaaf0106d85495a
SEMSTREAMS_COMMIT_DATE=2026-07-17
SEMSTREAMS_VERSION=v1.0.0-beta.147
```

Bumping is intentional, not auto-pulled.

### ETS Bump Procedure

1. Pick the new commit SHA, for example with
   `gh api repos/Botts-Innovative-Research/ets-ogcapi-connectedsystems10/commits/main --jq .sha`.
2. Edit `ETS_COMMIT` and `ETS_COMMIT_DATE`. Update `TEAMENGINE_VERSION` if
   the upstream Dockerfile changes its Team Engine version.
3. Run `./conformance/run.sh` locally and inspect the TestNG delta.
4. Include the TestNG delta in the PR description.

### Framework Bump Procedure

Pin order matters: bump the Go module first, then the conformance pin, so the
gateway's compiled wire expectations match the running backend.

1. Bump `go.mod`: `go get github.com/c360studio/semstreams@v1.0.0-beta.NN`.
2. Run `go mod tidy`.
3. Resolve the tag commit SHA. Tags are annotated, so distinguish the tag
   object SHA from the commit SHA.
4. Edit `SEMSTREAMS_COMMIT`, `SEMSTREAMS_COMMIT_DATE`, and
   `SEMSTREAMS_VERSION`.
5. Run `go test ./...`, `go build ./...`, and `./conformance/run.sh`.
6. Include the framework delta and conformance result in the PR description.

For a graph-state-breaking release such as beta.147, the pin procedure is not
the deployment procedure. Follow ADR-S003 and
`openspec/changes/migrate-semstreams-beta147/`: use a deployment-specific
manifest, stop every writer, remove only approved incompatible graph state,
reseed canonically, and prove revision/query/replay parity. Never start
beta.147 on retained beta.141 graph state or beta.141 on rebuilt beta.147 state.

## NATS Config

`nats.conf` pins JetStream `max_file_store` and `max_memory_store`.
nats-server 2.10's CLI does not expose those flags, and Docker defaults can be
too small for the framework baseline streams plus the CS API observation and
artifact stores. The harness owns the server-side limits; semstreams validates
and warns against the connected account's observed limits.

## Migrating Off Source Builds

When the OGC org adopts the ETS into
`opengeospatial/ets-ogcapi-connectedsystems10` and publishes a tagged image:

1. Replace `ETS_GIT_URL` and `ETS_COMMIT` in `.ets-pin` with `ETS_IMAGE`, for
   example `ghcr.io/opengeospatial/ets-ogcapi-connectedsystems10:1.0.0`.
2. Update `compose.yml`'s `teamengine` service from `build:` to `image:`.
3. Drop the `.vendor/ets` clone/build path from `run.sh`.

A symmetric migration applies when semstreams publishes a registry image:
replace the `semstreams-backend.build:` block with `image:` and drop the
`.vendor/semstreams` clone/build path.

## CI

`.github/workflows/conformance.yml` runs this harness on push to `main`, on
manual dispatch, and on PRs labelled `conformance`. The TestNG XML report is
uploaded as a workflow artifact for triage.
