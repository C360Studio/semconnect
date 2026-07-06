# Conformance Harness

This directory wires the [OGC Team Engine] conformance suite into local and CI
workflows for `semconnect`.

The harness boots NATS, `semstreams-backend`, `cs-api-server`, and Team Engine
on a shared Docker network; seeds CS API fixtures through the gateway's HTTP
write endpoints; invokes the CS API ETS through Team Engine's REST API; and
archives the TestNG XML report plus logs from every service.

[OGC Team Engine]: https://github.com/opengeospatial/teamengine

## Current Picture

As of the 2026-07-06 SemStreams pin refresh, the pinned suite is green:

```text
total=137 passed=137 failed=0 skipped=0
```

The current pins are:

- Botts CS API ETS `0.1-SNAPSHOT` at `d9caf33fcd0c4a3c1a582e8ba9b12b753277afd4`.
- TeamEngine `5.6.1`, bundled by the ETS Dockerfile.
- semstreams backend `v1.0.0-beta.141` at `d46c07a3a32a28259fd1c571a0445b140c8405e8`.

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

The seed phase is intentionally fatal: if a fixture cannot be created or
cannot be observed through the corresponding read endpoint, the suite does not
start. That keeps failures shaped like gateway/framework regressions instead
of cascading Team Engine skips.

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
SEMSTREAMS_COMMIT=d46c07a3a32a28259fd1c571a0445b140c8405e8
SEMSTREAMS_COMMIT_DATE=2026-07-05
SEMSTREAMS_VERSION=v1.0.0-beta.141
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
