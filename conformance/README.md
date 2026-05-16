# Stages 6 + 9 — Conformance harness

This directory wires the [OGC Team Engine] conformance suite into local +
CI workflows for `semconnect`. The harness boots NATS + `semstreams-backend`
+ `cs-api-server` + Team Engine (with the CS API ETS baked in) on a
shared Docker network, seeds a minimum fixture set via cs-api-server's
write endpoints, invokes the suite via Team Engine's REST API, and
archives the TestNG XML report alongside container logs from every
service.

ADR-S001 §4 is the binding decision-set; read it for *why* this looks the
way it does. The short version: pin the ETS by commit SHA, never check
its source into this repo, materialise it into a gitignored
`.vendor/ets/` directory at run time, then build the image fresh on each
cold run.

[OGC Team Engine]: https://github.com/opengeospatial/teamengine

## Calibration reality (Stage 9 cut)

Stage 9 wired a real graph backend (`semstreams-backend`) under
`cs-api-server` and added a pre-suite fixture-seed step. The harness now
**exercises real CS API behavior** — read paths go through
`graph.index.query.predicate` / `graph.query.entity` /
`graph.spatial.query.*` to a live framework instance, write paths go
through `graph.mutation.triple.add_batch`. Pre-Stage 9 every read 503'd
with `nats.ErrNoResponders`, so failures were infrastructure-shaped and
hid the real spec picture.

Current numbers (`v1.0.0-beta.73` framework pin + Botts ETS `0.1-SNAPSHOT`
@ `d9caf33`): `total=137 passed=13 failed=2 skipped=122`. The headline
counts didn't move post-Stage 9 because the cascade-blockers are
upstream:

- 2 upstream-ETS bugs fail in the `core` group
  (`landingPageHasApiDefinitionLink` + `apiDefinitionResourceReturnsContent`,
  both filed in `docs/upstream-asks/botts-ets-api-definition-unconditional.md`).
  Every `systemfeatures`-dependent test SKIPs through this gate.
- 1 net-new CS API spec gap surfaced by the seed working:
  `fetchGeoJsonInputs` now reports *"`/systems` response has no CS API
  'items' array"*. Our `systemCollection.Systems` field needs to be named
  `items` per OGC Common collection conventions. Tracked separately.

**What changed across Stage 9:** `fetchSensorMlInputs` flipped from
503 to PASS; `fetchGeoJsonInputs` flipped from 503 to a real spec
assertion. Real conformance work begins when the upstream ETS bugs
unblock the systemfeatures cascade.

The two known sister-side deferrals (`X-CS-Reconstructed-Lossy` on
`GET /systems/{id}`; `X-CS-Geometry-Available: false` on `GET /areas`)
will surface as assertion failures when the corresponding ETS tests
run. Track them upstream on `semstreams` per ADR-S001 §9.

## Running locally

Prerequisites: Docker 20.10+ with BuildKit, `git`, `python3` (TestNG XML
parsing + ephemeral-port allocation), `curl`. No host JDK / Maven
required — the Botts ETS Dockerfile bakes the full Maven lifecycle into
its builder stage.

```bash
# end-to-end run (cold: ~6-8 min ETS build + framework build + suite run)
./conformance/run.sh

# warm runs reuse Docker BuildKit cache; ~1-2 min
./conformance/run.sh

# tear down a wedged stack
./conformance/run.sh --teardown-only

# override host ports when 4222 / 8081 / 8222 are busy locally
TE_HOST_PORT=8181 NATS_HOST_PORT=14222 NATS_MON_HOST_PORT=18222 ./conformance/run.sh

# force teardown on success (default is KEEP_STACK=1 so you can triage afterward)
KEEP_STACK=0 ./conformance/run.sh
```

Outputs land in `conformance/output/` (gitignored):

- `testng-report-<UTC>.xml` — TestNG XML; the conformance picture.
- `teamengine-container-<UTC>.log` — `docker compose logs teamengine`.
- `cs-api-server-container-<UTC>.log` — gateway logs (Stage 9).
- `semstreams-backend-container-<UTC>.log` — framework backend logs (Stage 9).
- `nats-container-<UTC>.log` — captured on failure only (Stage 9).
- `seed-<UTC>.log` — POST /systems + POST /datastreams responses (Stage 9).
- `compose-build-<UTC>.log` — full build log (all three buildable services).
- `summary.txt` — human-readable summary with TestNG attribute counts.

Container logs from every service are captured both on success (after
suite invocation) and on failure (before teardown), so triaging a 503
or healthcheck timeout doesn't require reading framework or ETS Java
source.

Exit codes:

| Code | Meaning |
|------|---------|
| 0 | Harness ran end-to-end; read the TestNG XML for pass/fail. |
| 1 | Infrastructure failure (Docker, build, network, healthcheck). |
| 2 | Team Engine REST API returned non-2xx on suite invocation. |

## Bumping pins

`.ets-pin` carries upstream pins for both the ETS and the framework:

```ini
# Botts CS API ETS (Stage 6)
ETS_GIT_URL=https://github.com/Botts-Innovative-Research/ets-ogcapi-connectedsystems10.git
ETS_COMMIT=d9caf33fcd0c4a3c1a582e8ba9b12b753277afd4
ETS_COMMIT_DATE=2026-05-13
ETS_VERSION=0.1-SNAPSHOT
ETS_CODE=ogcapi-connectedsystems10
TEAMENGINE_VERSION=5.6.1

# semstreams framework backend (Stage 9)
SEMSTREAMS_GIT_URL=https://github.com/C360Studio/semstreams.git
SEMSTREAMS_COMMIT=bea1407b81f3d47b806e4e1600da9c033048af64
SEMSTREAMS_COMMIT_DATE=2026-05-15
SEMSTREAMS_VERSION=v1.0.0-beta.73
```

Bumping is intentional, not auto-pulled.

### ETS bump procedure

1. Pick the new commit SHA (`gh api repos/Botts-Innovative-Research/ets-ogcapi-connectedsystems10/commits/main --jq .sha`).
2. Edit `ETS_COMMIT` + `ETS_COMMIT_DATE`. Update `TEAMENGINE_VERSION` if the
   upstream `Dockerfile`'s `ARG TEAMENGINE_VERSION` changed.
3. Run `./conformance/run.sh` locally and inspect the TestNG delta — new
   tests at this stage mean new assertion failures to triage.
4. Open the PR with the TestNG delta in the description so the reviewer
   can see what conformance picture moved.

### Framework bump procedure

Pin order matters: **bump the Go module first, the conformance pin second**
so the gateway's wire-protocol expectations match the running backend.

1. Bump `go.mod`: `go get github.com/c360studio/semstreams@v1.0.0-beta.NN && go mod tidy`.
2. Re-resolve the commit SHA: `gh api repos/C360Studio/semstreams/git/tags/<tag-obj-sha> --jq .object.sha` (tags are annotated, so the ref SHA is the *tag object*, not the commit — `git rev-parse HEAD` returns the commit, so the pin file needs the commit SHA or `ensure_semstreams_vendor` re-clones every run).
3. Edit `SEMSTREAMS_COMMIT` + `SEMSTREAMS_COMMIT_DATE` + `SEMSTREAMS_VERSION`.
4. Run `./conformance/run.sh` locally. Watch for changes in handler-registration
   logs (`semstreams-backend-container-*.log`) — a new framework release may
   add or rename NATS subjects the gateway depends on.
5. Run `go test ./... -race` to confirm gateway compatibility.
6. Open the PR with the framework delta in the description.

## Migrating off the Botts pin

When the OGC org adopts the ETS into `opengeospatial/ets-ogcapi-connectedsystems10`
and publishes a tagged image to a registry (GHCR or Docker Hub):

1. Replace `ETS_GIT_URL` + `ETS_COMMIT` in `.ets-pin` with `ETS_IMAGE` (e.g.
   `ghcr.io/opengeospatial/ets-ogcapi-connectedsystems10:1.0.0`).
2. Update `compose.yml`'s `teamengine` service from `build:` to `image:`.
3. Drop `ensure_ets_vendor` and the `.vendor/ets` clone step from
   `run.sh` — pulling a registry image needs nothing more than `compose
   pull` (which `compose up` does on its own).

The harness shape (NATS + semstreams-backend + cs-api-server + TE on a
shared network, REST invocation, TestNG capture) is unchanged across
the migration; only the image source moves.

A symmetric migration applies when the framework publishes a registry
image (`ghcr.io/c360studio/semstreams:vX.Y.Z`): replace the
`semstreams-backend.build:` block with `image:`, drop `ensure_semstreams_vendor`
+ `.vendor/semstreams` from `run.sh`.

## Fixtures + seed step (Stage 9)

`fixtures/` carries small CS-API-shaped inputs (`system.sml.json`,
`observations.om.json`, `area.geojson.json`). `run.sh`'s
`seed_fixtures` step POSTs `system.sml.json` to `/systems` and a
generated Datastream body (referencing the just-seeded System) to
`/datastreams` after readiness gates and before suite invocation.
Both responses are captured in `output/seed-<UTC>.log`; a non-201
on either is fatal.

The Botts ETS `@BeforeClass` fixture loaders (`fetchSensorMlInputs`,
`fetchGeoJsonInputs`) read these via `GET /systems` to drive
SensorML + GeoJSON test groups. Without the seed step, the loaders
see an empty collection and either SkipException or assert-fail,
cascading SKIP through ~120 dependent tests.

`observations.om.json` is not seeded today — `POST /datastreams/{id}/observations`
is wired (Stage 3) but no ETS test currently reads stored observations
back. Add to the seed step when an observation-shape test lands upstream.

## NATS config (Stage 9)

`nats.conf` pins JetStream `max_file_store` + `max_memory_store`
explicitly. nats-server 2.10's CLI doesn't expose those flags
(config-file only), and the framework's `nats.jetstream` schema
declares them but doesn't apply them to the connected server. Auto-sizing
on Docker for Mac under image-cache pressure can compute a limit too
small for the framework's baseline streams (`LOGS`, `HEALTH`, `METRICS`,
`FLOWS`), surfacing as `nats: API error: code=500 err_code=10047
description=insufficient storage resources available` at boot. 10GB/1GB
is ample for a conformance run.

## CI

`.github/workflows/conformance.yml` runs this same harness on push to
`main` and on PRs labelled `conformance`. It is **not** a PR-blocking
gate at this stage — see the calibration reality above. The TestNG XML
report is uploaded as a workflow artifact for triage.
