#!/usr/bin/env bash
# Stage 6 conformance harness — boots NATS + cs-api-server + the Botts CS API
# ETS in TeamEngine 5.6.1, invokes the suite via TeamEngine's REST API
# against the local cs-api-server, and archives the TestNG XML report.
#
# Usage:
#   conformance/run.sh                  # full run, end-to-end
#   conformance/run.sh --teardown-only  # tear down a previous stack and exit
#
# Output (gitignored):
#   conformance/output/testng-report-<UTC>.xml
#   conformance/output/teamengine-container-<UTC>.log
#   conformance/output/summary.txt
#
# Exit codes:
#   0 — harness ran end-to-end; TestNG report archived (read it for pass/fail)
#   1 — infrastructure failure (build, container start, health, network)
#   2 — TeamEngine REST API returned non-2xx during suite invocation
#
# Stage 6 calibration note: the pinned Botts ETS is 0.1-SNAPSHOT (scaffold).
# A zero-failure run today proves the harness, NOT the conformance picture.
# Re-run after ETS pin bumps to surface real assertion failures.

set -euo pipefail

# -----------------------------------------------------------------------------
# Pre-flight + arg parsing
# -----------------------------------------------------------------------------

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

TEARDOWN_ONLY=0
if [[ "${1:-}" == "--teardown-only" ]]; then
    TEARDOWN_ONLY=1
fi

# Load pinned versions from .ets-pin (key=value, shell-sourced).
PIN_FILE="$SCRIPT_DIR/.ets-pin"
if [[ ! -f "$PIN_FILE" ]]; then
    echo "[run.sh] FATAL: missing $PIN_FILE" >&2
    exit 1
fi
# shellcheck disable=SC1090
. "$PIN_FILE"
for var in ETS_GIT_URL ETS_COMMIT ETS_CODE TE_USER TE_PASS \
           SEMSTREAMS_GIT_URL SEMSTREAMS_COMMIT; do
    if [[ -z "${!var:-}" ]]; then
        echo "[run.sh] FATAL: $PIN_FILE missing $var" >&2
        exit 1
    fi
done

OUTPUT_DIR="${CONFORMANCE_OUTPUT_DIR:-$SCRIPT_DIR/output}"
UTC_STAMP="$(date -u +%Y-%m-%dT%H-%M-%SZ)"
REPORT_XML="$OUTPUT_DIR/testng-report-${UTC_STAMP}.xml"
TE_LOG="$OUTPUT_DIR/teamengine-container-${UTC_STAMP}.log"
CS_LOG="$OUTPUT_DIR/cs-api-server-container-${UTC_STAMP}.log"
BACKEND_LOG="$OUTPUT_DIR/semstreams-backend-container-${UTC_STAMP}.log"
SEED_LOG="$OUTPUT_DIR/seed-${UTC_STAMP}.log"
SUMMARY="$OUTPUT_DIR/summary.txt"
mkdir -p "$OUTPUT_DIR"

COMPOSE_FILE="$SCRIPT_DIR/compose.yml"
COMPOSE_PROJECT="${CONFORMANCE_PROJECT:-semconnect-conformance}"

# We materialise the pinned Botts ETS into a local vendor directory at run
# time. Building from `git+url#sha` directly would be smaller, but Docker
# strips `.git/` from a git-context fetch and the Botts Dockerfile's
# buildnumber-maven-plugin requires `.git` to populate manifest SCM
# attributes. A real clone keeps `.git` intact.
ETS_VENDOR_DIR="$SCRIPT_DIR/.vendor/ets"

# Stage 9 — same pattern for the framework. The semstreams Dockerfile
# does NOT need .git (no SCM-plugin equivalent), so a shallow clone is
# sufficient and faster. Same .vendor/ pattern keeps both vendor dirs
# under one gitignored umbrella.
SEMSTREAMS_VENDOR_DIR="$SCRIPT_DIR/.vendor/semstreams"

# Health timing — generous because cold Maven build is ~5–6 minutes.
HEALTH_TIMEOUT_S="${CONFORMANCE_HEALTH_TIMEOUT_S:-900}"
RUN_TIMEOUT_S="${CONFORMANCE_RUN_TIMEOUT_S:-600}"

# Inside the docker compose network, IUT is reachable at the service name.
IUT_URL_DEFAULT="http://cs-api-server:8080"
IUT_URL="${CONFORMANCE_IUT_URL:-$IUT_URL_DEFAULT}"

# Host port mappings. Defaults align with compose.yml; override here so a busy
# dev machine doesn't collide. `pick_free_port` asks the kernel for an
# ephemeral port via Python's zero-bind — atomic and portable (the prior
# /dev/tcp-based probe wasn't POSIX and was a TOCTOU race anyway).
pick_free_port() {
    python3 -c 'import socket
s = socket.socket()
s.bind(("127.0.0.1", 0))
print(s.getsockname()[1])
s.close()'
}
export TE_HOST_PORT="${TE_HOST_PORT:-$(pick_free_port)}"
export NATS_HOST_PORT="${NATS_HOST_PORT:-$(pick_free_port)}"
export NATS_MON_HOST_PORT="${NATS_MON_HOST_PORT:-$(pick_free_port)}"

# -----------------------------------------------------------------------------
# Helpers
# -----------------------------------------------------------------------------

log() { echo "[run.sh $(date -u +%H:%M:%S)] $*"; }

# die: emit reason, then let on_exit capture container logs before
# teardown. We do NOT call teardown_silent here — on_exit (trap EXIT)
# runs after exit and handles both the log capture and the teardown
# so failure-path logs survive in $OUTPUT_DIR for triage.
die() { echo "[run.sh FATAL] $*" >&2; exit 1; }

# `docker compose` v2 is the standard; fall back to docker-compose if absent.
compose() {
    if docker compose version >/dev/null 2>&1; then
        docker compose -p "$COMPOSE_PROJECT" -f "$COMPOSE_FILE" "$@"
    elif command -v docker-compose >/dev/null 2>&1; then
        docker-compose -p "$COMPOSE_PROJECT" -f "$COMPOSE_FILE" "$@"
    else
        die "neither 'docker compose' nor 'docker-compose' found in PATH"
    fi
}

ensure_ets_vendor() {
    if [[ -d "$ETS_VENDOR_DIR/.git" ]]; then
        local current
        current="$(git -C "$ETS_VENDOR_DIR" rev-parse HEAD 2>/dev/null || true)"
        if [[ "$current" == "$ETS_COMMIT" ]]; then
            log "  ETS vendor already at pinned SHA — reusing $ETS_VENDOR_DIR"
            return
        fi
        log "  ETS vendor at ${current:-<unknown>}; resetting to $ETS_COMMIT"
        rm -rf "$ETS_VENDOR_DIR"
    fi
    log "  cloning $ETS_GIT_URL @ $ETS_COMMIT into $ETS_VENDOR_DIR"
    mkdir -p "$(dirname "$ETS_VENDOR_DIR")"
    # Shallow clone with --filter=blob:none keeps history+SCM metadata cheap
    # while still satisfying buildnumber-maven-plugin (which only needs HEAD).
    git clone --filter=blob:none "$ETS_GIT_URL" "$ETS_VENDOR_DIR" >/dev/null 2>&1 \
        || die "git clone $ETS_GIT_URL failed"
    git -C "$ETS_VENDOR_DIR" checkout --quiet "$ETS_COMMIT" \
        || die "git checkout $ETS_COMMIT failed (does the SHA exist on the remote?)"
}

ensure_semstreams_vendor() {
    if [[ -d "$SEMSTREAMS_VENDOR_DIR/.git" ]]; then
        local current
        current="$(git -C "$SEMSTREAMS_VENDOR_DIR" rev-parse HEAD 2>/dev/null || true)"
        if [[ "$current" == "$SEMSTREAMS_COMMIT" ]]; then
            log "  semstreams vendor already at pinned SHA — reusing $SEMSTREAMS_VENDOR_DIR"
            return
        fi
        log "  semstreams vendor at ${current:-<unknown>}; resetting to $SEMSTREAMS_COMMIT"
        rm -rf "$SEMSTREAMS_VENDOR_DIR"
    fi
    log "  cloning $SEMSTREAMS_GIT_URL @ $SEMSTREAMS_COMMIT into $SEMSTREAMS_VENDOR_DIR"
    mkdir -p "$(dirname "$SEMSTREAMS_VENDOR_DIR")"
    # Shallow clone — framework Dockerfile reads source only, no SCM
    # metadata needed.
    git clone --filter=blob:none "$SEMSTREAMS_GIT_URL" "$SEMSTREAMS_VENDOR_DIR" >/dev/null 2>&1 \
        || die "git clone $SEMSTREAMS_GIT_URL failed"
    git -C "$SEMSTREAMS_VENDOR_DIR" checkout --quiet "$SEMSTREAMS_COMMIT" \
        || die "git checkout $SEMSTREAMS_COMMIT failed (does the SHA exist on the remote?)"
}

# Seed CS-API fixtures so the Botts ETS @BeforeClass loaders
# (fetchSensorMlInputs / fetchGeoJsonInputs) find at least one System
# entity to drive the dependent tests. Runs after both cs-api-server
# and semstreams-backend are healthy.
#
# Approach: run curlimages/curl as a one-shot container on the compose
# network so we don't have to host-expose cs-api-server's port.
# Failures here are FATAL — without fixtures the run is meaningless.
seed_fixtures() {
    local cs_api_url="http://cs-api-server:8080"
    local fixtures_dir="$SCRIPT_DIR/fixtures"
    log "  POST /systems with $(basename "$fixtures_dir/system.sml.json")"
    local sys_resp
    sys_resp="$(docker run --rm \
        --network "${COMPOSE_PROJECT}_default" \
        -v "$fixtures_dir":/fx:ro \
        curlimages/curl:8.10.1 \
        -sS -w '\nHTTP %{http_code} loc=%header{location}\n' \
        -X POST -H 'Content-Type: application/sensorml+json' \
        --data-binary @/fx/system.sml.json \
        "${cs_api_url}/systems" 2>&1)" || true
    echo "$sys_resp" >>"$SEED_LOG"
    local sys_code
    sys_code="$(echo "$sys_resp" | awk '/^HTTP /{print $2}' | tail -1)"
    if [[ "$sys_code" != "201" ]]; then
        die "POST /systems failed: $sys_code (see $SEED_LOG)"
    fi
    local sys_loc
    sys_loc="$(echo "$sys_resp" | awk '/^HTTP /{print $3}' | tail -1 | sed 's/^loc=//')"
    local sys_id="${sys_loc##*/}"
    # Defensive validation — if cs-api-server returned 201 without a
    # Location header (or with a malformed one), the next POST would
    # send `"system":""` and surface as a confusing 400. Fail loudly
    # at the parse seam instead.
    if [[ -z "$sys_id" ]]; then
        die "POST /systems returned 201 but Location header was empty or missing (see $SEED_LOG)"
    fi
    # cs-api-server's mintEntityID guarantees a 6-part dot-separated
    # NATS-token-safe ID (a-z0-9 + hyphen + underscore + colon). Reject
    # anything else before composing the Datastream body — keeps the
    # script trustworthy in isolation even if cs-api-server regresses.
    if ! [[ "$sys_id" =~ ^[A-Za-z0-9_.:-]+$ ]]; then
        die "POST /systems returned 201 with malformed id '$sys_id' (see $SEED_LOG)"
    fi
    log "  seeded system: id=$sys_id"

    # Build a Datastream pointing at the just-seeded System. CS API §10
    # shape: id (optional, will be minted), name, description, system
    # ref (6-part minted ID), observedProperty IRI. Inline JSON is safe
    # because sys_id is regex-validated above; no shell-quoting risk.
    local ds_body
    ds_body=$(cat <<EOF
{"name":"Conformance temperature stream","description":"Stage 9 seed fixture — sensor observations for the weather station.","system":"${sys_id}","observedProperty":"http://www.w3.org/ns/sosa/Property/AirTemperature"}
EOF
)
    log "  POST /datastreams referencing system=$sys_id"
    local ds_resp
    ds_resp="$(docker run --rm \
        --network "${COMPOSE_PROJECT}_default" \
        curlimages/curl:8.10.1 \
        -sS -w '\nHTTP %{http_code} loc=%header{location}\n' \
        -X POST -H 'Content-Type: application/json' \
        --data-binary "$ds_body" \
        "${cs_api_url}/datastreams" 2>&1)" || true
    echo "$ds_resp" >>"$SEED_LOG"
    local ds_code
    ds_code="$(echo "$ds_resp" | awk '/^HTTP /{print $2}' | tail -1)"
    if [[ "$ds_code" != "201" ]]; then
        die "POST /datastreams failed: $ds_code (see $SEED_LOG)"
    fi

    # Stage 12 — wait for the predicate index to reflect the seed before
    # invoking the suite. POST writes to ENTITY_STATES synchronously;
    # graph-index KV-watches ENTITY_STATES and updates PREDICATE_INDEX
    # asynchronously (eventually-consistent). The suite's
    # `systemsCollectionHasItemsArray` test reads /systems and asserts
    # non-empty — without this wait it races the KV-watch and FAILs even
    # though /systems/{id} (direct entity query) already works.
    log "  waiting for predicate index to reflect seed (eventual consistency)"
    local poll_attempt
    for poll_attempt in $(seq 1 30); do
        local count
        count="$(docker run --rm \
            --network "${COMPOSE_PROJECT}_default" \
            curlimages/curl:8.10.1 \
            -sS "${cs_api_url}/systems" 2>/dev/null \
            | python3 -c "import sys,json; print(json.loads(sys.stdin.read()).get('numberReturned', 0))" 2>/dev/null || echo 0)"
        if [[ "$count" -gt 0 ]]; then
            log "  predicate index ready after ${poll_attempt} attempt(s); /systems numberReturned=$count"
            break
        fi
        if [[ "$poll_attempt" -eq 30 ]]; then
            die "predicate index never reflected seed after 30 attempts (~30s); see $SEED_LOG"
        fi
        sleep 1
    done

    log "  seed complete (log: $SEED_LOG)"
}

teardown_silent() {
    compose down -v --remove-orphans >/dev/null 2>&1 || true
}

# Tear down only on failure paths. A successful run leaves the stack up so
# a developer can `docker compose -p semconnect-conformance logs teamengine`
# or browse to http://localhost:$TE_HOST_PORT/teamengine/ for triage.
# Override with KEEP_STACK=0 to force teardown even on success.
KEEP_STACK="${KEEP_STACK:-1}"
on_exit() {
    local rc=$?
    if [[ "$rc" -ne 0 ]]; then
        # Stage 9 — capture container logs BEFORE teardown wipes them.
        # Without this, a healthcheck timeout or seed-step failure leaves
        # nothing on disk to diagnose. Logs are best-effort: any of the
        # containers may have crashed too early to have output.
        log "failure path — capturing container logs before teardown"
        # NATS is captured only on the failure path because a healthy NATS
        # is uninteresting; an unhealthy NATS (storage / JS limit) is the
        # second-most-likely failure root cause after the framework backend.
        compose logs nats              >"$OUTPUT_DIR/nats-container-${UTC_STAMP}.log"             2>&1 || true
        compose logs semstreams-backend >"$BACKEND_LOG" 2>&1 || true
        compose logs cs-api-server     >"$CS_LOG"      2>&1 || true
        compose logs teamengine        >"$TE_LOG"      2>&1 || true
        teardown_silent
    elif [[ "$KEEP_STACK" -eq 0 ]]; then
        teardown_silent
    else
        echo "[run.sh] stack left running for triage; tear down with:" >&2
        echo "    ./conformance/run.sh --teardown-only" >&2
    fi
}
trap on_exit EXIT

# -----------------------------------------------------------------------------
# Main flow
# -----------------------------------------------------------------------------

if [[ "$TEARDOWN_ONLY" -eq 1 ]]; then
    log "teardown-only mode — bringing stack down"
    compose down -v --remove-orphans || true
    trap - EXIT
    log "teardown complete"
    exit 0
fi

log "Stages 6+9 conformance harness — $UTC_STAMP"
log "ETS pin:        $ETS_GIT_URL @ $ETS_COMMIT (TE $TEAMENGINE_VERSION, $ETS_VERSION)"
log "semstreams pin: $SEMSTREAMS_GIT_URL @ $SEMSTREAMS_COMMIT ($SEMSTREAMS_VERSION)"
log "IUT URL (inside compose network): $IUT_URL"
log "Output dir: $OUTPUT_DIR"

# 1. Sanity check docker + git.
command -v docker >/dev/null 2>&1 || die "docker not found in PATH"
docker info >/dev/null 2>&1 || die "docker daemon not reachable"
command -v git >/dev/null 2>&1 || die "git not found in PATH"

# 2. Materialise pinned ETS + framework sources + tear down any previous stack.
log "step 1/7 — materialising pinned ETS + framework sources, tearing down previous stack"
ensure_ets_vendor
ensure_semstreams_vendor
compose down -v --remove-orphans >/dev/null 2>&1 || true

# 3. Build + start everything. Compose handles dep ordering via depends_on.
#    BuildKit is required for cs-api-server's --mount=type=cache directives.
export DOCKER_BUILDKIT=1
export COMPOSE_DOCKER_CLI_BUILD=1

log "step 2/7 — docker compose build (cold ETS build is ~5–6 min)"
BUILD_LOG="$OUTPUT_DIR/compose-build-${UTC_STAMP}.log"
if ! compose build >"$BUILD_LOG" 2>&1; then
    tail -100 "$BUILD_LOG"
    die "compose build failed (full log: $BUILD_LOG)"
fi

log "step 3/7 — docker compose up -d (waits for all healthchecks)"
compose up -d --wait --wait-timeout "$HEALTH_TIMEOUT_S" \
    || die "compose up failed or service healthchecks timed out at ${HEALTH_TIMEOUT_S}s"

# 4. Sanity-poke the IUT — prove cs-api-server is up before we burn a
#    suite-run timeout on a networking failure. We probe from the host
#    side (TE container reaches cs-api-server via the compose network;
#    if the host can reach TE, and TE depends_on cs-api-server, the path
#    is wired). The host probe avoids relying on `curl` being installed
#    inside whatever teamengine image the pin produces.
log "step 4/7 — verifying TeamEngine is reachable on host port ${TE_HOST_PORT}"
curl -fsS -o /dev/null "http://localhost:${TE_HOST_PORT}/teamengine/" \
    || die "TeamEngine UI not reachable on http://localhost:${TE_HOST_PORT}/teamengine/"

# 5. Seed CS-API fixtures. Stage 9 — without this the Botts ETS
#    @BeforeClass fixture loaders (fetchSensorMlInputs /
#    fetchGeoJsonInputs) see an empty SystemCollection and SKIP every
#    test that depends on the systemfeatures group.
log "step 5/7 — seeding CS-API fixtures"
seed_fixtures

# 6. Confirm suite is registered, then invoke it.
TE_BASE="http://localhost:${TE_HOST_PORT}/teamengine"
log "step 6/7 — verifying suite ${ETS_CODE} is registered"
suites_xml="$(curl -fsS -u "${TE_USER}:${TE_PASS}" \
                    -H 'Accept: application/xml' \
                    "${TE_BASE}/rest/suites")" \
    || die "GET ${TE_BASE}/rest/suites failed"
if ! echo "$suites_xml" | grep -q "<etscode>${ETS_CODE}</etscode>"; then
    echo "$suites_xml" >"$OUTPUT_DIR/suites-${UTC_STAMP}.xml"
    die "${ETS_CODE} not present in /rest/suites — see $OUTPUT_DIR/suites-${UTC_STAMP}.xml"
fi

log "step 7/7 — invoking suite ${ETS_CODE} against ${IUT_URL} (timeout ${RUN_TIMEOUT_S}s)"
# Stage 16 — opt into the ETS mutation lifecycle tests
# (createreplacedelete + update groups). The harness's stack is
# ephemeral per run (compose down -v at start), so
# `mutation-iut-policy=dedicated-mutable-iut` is honest: the IUT IS
# dedicated and mutable. Without these flags the CRD lifecycle tests
# SKIP via `ensureMutationEnabledOrSkip` and the conformance picture
# misses the real evidence of POST/PUT/DELETE round-trip.
http_code="$(curl -s -u "${TE_USER}:${TE_PASS}" -G \
                "${TE_BASE}/rest/suites/${ETS_CODE}/run" \
                --data-urlencode "iut=${IUT_URL}" \
                --data-urlencode "mutation-tests-enabled=true" \
                --data-urlencode "mutation-iut-policy=dedicated-mutable-iut" \
                -H 'Accept: application/xml' \
                -o "$REPORT_XML" \
                -w '%{http_code}' \
                -m "$RUN_TIMEOUT_S")"
if [[ "$http_code" != "200" ]]; then
    log "TestNG response body (HTTP $http_code):"
    head -50 "$REPORT_XML" || true
    log "Container logs (last 100 lines):"
    compose logs --tail=100 teamengine | tee -a "$TE_LOG" || true
    exit 2
fi
# Capture container logs from all three service containers so 503 / 500
# triage doesn't require reading the framework's Java source. Stage 9.
compose logs teamengine >"$TE_LOG" 2>&1 || true
compose logs cs-api-server >"$CS_LOG" 2>&1 || true
compose logs semstreams-backend >"$BACKEND_LOG" 2>&1 || true

# 6. Parse TestNG attributes and emit a summary. Uses xml.etree.ElementTree
#    rather than a regex so we tolerate XML preambles, stylesheets, and
#    comments at any size, and never silently mis-match an attribute value
#    that contains '>'.
parse_testng() {
    python3 - "$REPORT_XML" <<'PY'
import sys, xml.etree.ElementTree as ET
try:
    root = ET.parse(sys.argv[1]).getroot()
except (ET.ParseError, OSError) as e:
    print(f"NA NA NA NA  # parse error: {e}")
    sys.exit(0)
attrs = root.attrib
fields = ("total", "passed", "failed", "skipped")
print(" ".join(attrs.get(f, "NA") for f in fields))
PY
}

if command -v python3 >/dev/null 2>&1; then
    read -r total passed failed skipped <<<"$(parse_testng)"
else
    total="?" passed="?" failed="?" skipped="?"
fi

{
    echo "Stages 6+9 conformance harness — $UTC_STAMP"
    echo "ETS pin:        $ETS_GIT_URL @ $ETS_COMMIT"
    echo "TeamEngine:     $TEAMENGINE_VERSION ($ETS_VERSION)"
    echo "semstreams pin: $SEMSTREAMS_GIT_URL @ $SEMSTREAMS_COMMIT ($SEMSTREAMS_VERSION)"
    echo "IUT: $IUT_URL"
    echo
    echo "TestNG: total=$total passed=$passed failed=$failed skipped=$skipped"
    echo "Report:        $REPORT_XML"
    echo "TE log:        $TE_LOG"
    echo "cs-api log:    $CS_LOG"
    echo "backend log:   $BACKEND_LOG"
    echo "seed log:      $SEED_LOG"
    echo
    echo "Stage 9 note: cs-api-server now runs against a real graph backend"
    echo "(semstreams-backend), seeded with conformance/fixtures/system.sml.json"
    echo "and a referencing datastream before suite invocation. The Botts"
    echo "fixture-loader 503s are eliminated — surviving failures are genuine"
    echo "spec assertions or upstream-ETS bugs."
} | tee "$SUMMARY"

log "done — exit 0 means harness ran; read $REPORT_XML for the conformance picture"
