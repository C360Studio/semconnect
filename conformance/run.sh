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
for var in ETS_GIT_URL ETS_COMMIT ETS_CODE TE_USER TE_PASS; do
    if [[ -z "${!var:-}" ]]; then
        echo "[run.sh] FATAL: $PIN_FILE missing $var" >&2
        exit 1
    fi
done

OUTPUT_DIR="${CONFORMANCE_OUTPUT_DIR:-$SCRIPT_DIR/output}"
UTC_STAMP="$(date -u +%Y-%m-%dT%H-%M-%SZ)"
REPORT_XML="$OUTPUT_DIR/testng-report-${UTC_STAMP}.xml"
TE_LOG="$OUTPUT_DIR/teamengine-container-${UTC_STAMP}.log"
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
die() { echo "[run.sh FATAL] $*" >&2; teardown_silent; exit 1; }

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
    if [[ "$rc" -ne 0 ]] || [[ "$KEEP_STACK" -eq 0 ]]; then
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

log "Stage 6 conformance harness — $UTC_STAMP"
log "ETS pin: $ETS_GIT_URL @ $ETS_COMMIT (TE $TEAMENGINE_VERSION, $ETS_VERSION)"
log "IUT URL (inside compose network): $IUT_URL"
log "Output dir: $OUTPUT_DIR"

# 1. Sanity check docker + git.
command -v docker >/dev/null 2>&1 || die "docker not found in PATH"
docker info >/dev/null 2>&1 || die "docker daemon not reachable"
command -v git >/dev/null 2>&1 || die "git not found in PATH"

# 2. Materialise the pinned ETS source + tear down any previous stack.
log "step 1/6 — materialising pinned ETS source + tearing down previous stack"
ensure_ets_vendor
compose down -v --remove-orphans >/dev/null 2>&1 || true

# 3. Build + start everything. Compose handles dep ordering via depends_on.
#    BuildKit is required for cs-api-server's --mount=type=cache directives.
export DOCKER_BUILDKIT=1
export COMPOSE_DOCKER_CLI_BUILD=1

log "step 2/6 — docker compose build (cold ETS build is ~5–6 min)"
BUILD_LOG="$OUTPUT_DIR/compose-build-${UTC_STAMP}.log"
if ! compose build >"$BUILD_LOG" 2>&1; then
    tail -100 "$BUILD_LOG"
    die "compose build failed (full log: $BUILD_LOG)"
fi

log "step 3/6 — docker compose up -d"
compose up -d --wait --wait-timeout "$HEALTH_TIMEOUT_S" \
    || die "compose up failed or service healthchecks timed out at ${HEALTH_TIMEOUT_S}s"

# 4. Sanity-poke the IUT — prove cs-api-server is up before we burn a
#    suite-run timeout on a networking failure. We probe from the host
#    side (TE container reaches cs-api-server via the compose network;
#    if the host can reach TE, and TE depends_on cs-api-server, the path
#    is wired). The host probe avoids relying on `curl` being installed
#    inside whatever teamengine image the pin produces.
log "step 4/6 — verifying TeamEngine is reachable on host port ${TE_HOST_PORT}"
curl -fsS -o /dev/null "http://localhost:${TE_HOST_PORT}/teamengine/" \
    || die "TeamEngine UI not reachable on http://localhost:${TE_HOST_PORT}/teamengine/"

# 5. Confirm suite is registered, then invoke it.
TE_BASE="http://localhost:${TE_HOST_PORT}/teamengine"
log "step 5/6 — verifying suite ${ETS_CODE} is registered"
suites_xml="$(curl -fsS -u "${TE_USER}:${TE_PASS}" \
                    -H 'Accept: application/xml' \
                    "${TE_BASE}/rest/suites")" \
    || die "GET ${TE_BASE}/rest/suites failed"
if ! echo "$suites_xml" | grep -q "<etscode>${ETS_CODE}</etscode>"; then
    echo "$suites_xml" >"$OUTPUT_DIR/suites-${UTC_STAMP}.xml"
    die "${ETS_CODE} not present in /rest/suites — see $OUTPUT_DIR/suites-${UTC_STAMP}.xml"
fi

log "step 6/6 — invoking suite ${ETS_CODE} against ${IUT_URL} (timeout ${RUN_TIMEOUT_S}s)"
http_code="$(curl -s -u "${TE_USER}:${TE_PASS}" -G \
                "${TE_BASE}/rest/suites/${ETS_CODE}/run" \
                --data-urlencode "iut=${IUT_URL}" \
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
compose logs teamengine >"$TE_LOG" 2>&1 || true

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
    echo "Stage 6 conformance harness — $UTC_STAMP"
    echo "ETS pin: $ETS_GIT_URL @ $ETS_COMMIT"
    echo "TeamEngine: $TEAMENGINE_VERSION ($ETS_VERSION)"
    echo "IUT: $IUT_URL"
    echo
    echo "TestNG: total=$total passed=$passed failed=$failed skipped=$skipped"
    echo "Report:    $REPORT_XML"
    echo "TE log:    $TE_LOG"
    echo
    echo "Note: at the 0.1-SNAPSHOT ETS pin, a zero-failure result validates"
    echo "harness wiring, not spec conformance. Bump .ets-pin when the Botts"
    echo "repo lands real CS API conformance test classes and re-run."
} | tee "$SUMMARY"

log "done — exit 0 means harness ran; read $REPORT_XML for the conformance picture"
