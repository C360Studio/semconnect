# Stage 6 — Conformance harness

This directory wires the [OGC Team Engine] conformance suite into local +
CI workflows for `semconnect`. The harness boots NATS + `cs-api-server`
+ Team Engine (with the CS API ETS baked in) on a shared Docker network,
invokes the suite via Team Engine's REST API, and archives the TestNG
XML report.

ADR-S001 §4 is the binding decision-set; read it for *why* this looks the
way it does. The short version: pin the ETS by commit SHA, never check
its source into this repo, materialise it into a gitignored
`.vendor/ets/` directory at run time, then build the image fresh on each
cold run.

[OGC Team Engine]: https://github.com/opengeospatial/teamengine

## Stage 6 calibration reality

> A zero-failure run **today** validates the harness, not the spec.

The CS API ETS we pin (Botts-Innovative-Research/ets-ogcapi-connectedsystems10)
is `0.1-SNAPSHOT` and its own README explicitly states the current sprint
lands the **green-build scaffold only** — real CS API conformance test
classes are deferred to follow-up sprints. The full target at v1.0 is the
14 Part 1 conformance classes (Core + 13 dependents).

So the value of this harness today is **infrastructure**: when upstream
lands real tests (or when the OGC org publishes an official ETS image),
re-running `run.sh` lights up the conformance picture without further
plumbing work. The two known sister-side deferrals — `X-CS-Reconstructed-Lossy`
on `GET /systems/{id}` SensorML reconstruction and `X-CS-Geometry-Available:
false` on `GET /areas` Features — will surface as Team Engine assertion
failures once tests for those resources exist. Track them upstream on
`semstreams` per ADR-S001 §9 when they show up.

## Running locally

Prerequisites: Docker 20.10+ with BuildKit, `git`, `python3` (TestNG XML
parsing + ephemeral-port allocation), `curl`. No host JDK / Maven
required — the Botts ETS Dockerfile bakes the full Maven lifecycle into
its builder stage.

```bash
# end-to-end run (cold: ~6-8 min ETS build + ~30s NATS+cs-api boot + suite run)
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
- `compose-build-<UTC>.log` — full build log (cs-api-server + ETS image).
- `summary.txt` — human-readable summary with TestNG attribute counts.

Exit codes:

| Code | Meaning |
|------|---------|
| 0 | Harness ran end-to-end; read the TestNG XML for pass/fail. |
| 1 | Infrastructure failure (Docker, build, network, healthcheck). |
| 2 | Team Engine REST API returned non-2xx on suite invocation. |

## Bumping the ETS pin

`.ets-pin` carries the upstream commit SHA + informational metadata:

```ini
ETS_GIT_URL=https://github.com/Botts-Innovative-Research/ets-ogcapi-connectedsystems10.git
ETS_COMMIT=d9caf33fcd0c4a3c1a582e8ba9b12b753277afd4
ETS_COMMIT_DATE=2026-05-13
ETS_VERSION=0.1-SNAPSHOT
ETS_CODE=ogcapi-connectedsystems10
TEAMENGINE_VERSION=5.6.1
```

Bumping is intentional, not auto-pulled. Procedure:

1. Pick the new commit SHA (`gh api repos/Botts-Innovative-Research/ets-ogcapi-connectedsystems10/commits/main --jq .sha`).
2. Edit `ETS_COMMIT` + `ETS_COMMIT_DATE`. Update `TEAMENGINE_VERSION` if the
   upstream `Dockerfile`'s `ARG TEAMENGINE_VERSION` changed.
3. Run `./conformance/run.sh` locally and inspect the TestNG delta — new
   tests at this stage mean new assertion failures to triage.
4. Open the PR with the TestNG delta in the description so the reviewer
   can see what conformance picture moved.

## Migrating off the Botts pin

When the OGC org adopts the ETS into `opengeospatial/ets-ogcapi-connectedsystems10`
and publishes a tagged image to a registry (GHCR or Docker Hub):

1. Replace `ETS_GIT_URL` + `ETS_COMMIT` in `.ets-pin` with `ETS_IMAGE` (e.g.
   `ghcr.io/opengeospatial/ets-ogcapi-connectedsystems10:1.0.0`).
2. Update `compose.yml`'s `teamengine` service from `build:` to `image:`.
3. Drop `ensure_ets_vendor` and the `.vendor/ets` clone step from
   `run.sh` — pulling a registry image needs nothing more than `compose
   pull` (which `compose up` does on its own).

The harness shape (NATS + cs-api-server + TE on a shared network, REST
invocation, TestNG capture) is unchanged across the migration; only the
image source moves.

## Fixtures

`fixtures/` carries small CS-API-shaped inputs (`system.sml.json`,
`observations.om.json`, `area.geojson.json`). The Botts ETS does not
consume these directly today (its scaffold-only tests don't POST yet);
they exist so that as real tests land, the harness has known-good inputs
to seed `cs-api-server` against.

## CI

`.github/workflows/conformance.yml` runs this same harness on push to
`main` and on PRs labelled `conformance`. It is **not** a PR-blocking
gate at this stage — see the calibration reality above. The TestNG XML
report is uploaded as a workflow artifact for triage.
