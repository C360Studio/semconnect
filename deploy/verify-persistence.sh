#!/bin/sh
set -eu

COMPOSE_FILE=${COMPOSE_FILE:-deploy/compose.yml}
HEALTH_TEMPLATE='{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}'
VOLUME_TEMPLATE='{{range .Mounts}}{{if eq .Destination "/data"}}{{.Name}}{{end}}{{end}}'

compose() {
  docker compose -f "$COMPOSE_FILE" "$@"
}

sha256_file() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

wait_healthy() {
  service=$1
  attempts=0
  while [ "$attempts" -lt 180 ]; do
    container_id=$(compose ps -q "$service")
    if [ -n "$container_id" ]; then
      status=$(docker inspect --format "$HEALTH_TEMPLATE" "$container_id")
      if [ "$status" = healthy ]; then
        return 0
      fi
      if [ "$status" = unhealthy ] || [ "$status" = exited ] || [ "$status" = dead ]; then
        echo "$service entered terminal state: $status" >&2
        compose logs "$service" >&2
        return 1
      fi
    fi
    attempts=$((attempts + 1))
    sleep 1
  done
  echo "$service did not become healthy" >&2
  compose logs "$service" >&2
  return 1
}

evidence_dir=${EVIDENCE_DIR:-$(mktemp -d)}
mkdir -p "$evidence_dir"

compose up -d nats
wait_healthy nats
compose --profile smoke build canonical-smoke
compose --profile smoke run --rm greenfield-preflight >"$evidence_dir/first-start-jsz.json"
volume_before=$(docker inspect --format "$VOLUME_TEMPLATE" "$(compose ps -q nats)")
printf '%s\n' "$volume_before" >"$evidence_dir/volume-before.txt"

compose up -d --build semstreams semconnect
wait_healthy semstreams
wait_healthy semconnect
compose --profile smoke run --rm canonical-smoke seed >"$evidence_dir/before-restart.json"

stop_started_at=$(date -u '+%Y-%m-%dT%H:%M:%SZ')
compose stop semconnect semstreams nats
compose logs --since "$stop_started_at" --no-color nats >"$evidence_dir/nats-stop.log"
: >"$evidence_dir/normal-stop-state.txt"
for service in semconnect semstreams nats; do
  container_id=$(compose ps -aq "$service")
  stop_state=$(docker inspect --format '{{.State.ExitCode}} {{.State.OOMKilled}}' "$container_id")
  printf '%s exit_code=%s oom_killed=%s\n' "$service" $stop_state >>"$evidence_dir/normal-stop-state.txt"
  if [ "$service" = nats ] && [ "$stop_state" != "1 false" ]; then
    echo "NATS stop status was not the pinned image's clean signal status: $stop_state" >&2
    exit 1
  fi
  if [ "$service" != nats ] && [ "$stop_state" != "0 false" ]; then
    echo "$service did not stop normally: $stop_state" >&2
    exit 1
  fi
done
if ! grep -q 'JetStream Shutdown' "$evidence_dir/nats-stop.log" || \
  ! grep -q 'Server Exiting' "$evidence_dir/nats-stop.log" || \
  grep -q 'panic\|fatal' "$evidence_dir/nats-stop.log"; then
  echo "NATS did not record a clean JetStream shutdown" >&2
  exit 1
fi
if grep -q 'nats exit_code=.*oom_killed=true' "$evidence_dir/normal-stop-state.txt"; then
  echo "NATS was OOM-killed" >&2
  exit 1
fi
compose start nats semstreams semconnect
wait_healthy nats
wait_healthy semstreams
wait_healthy semconnect
volume_after=$(docker inspect --format "$VOLUME_TEMPLATE" "$(compose ps -q nats)")
printf '%s\n' "$volume_after" >"$evidence_dir/volume-after.txt"
if [ -z "$volume_before" ] || [ "$volume_after" != "$volume_before" ]; then
  echo "NATS volume identity changed across normal stop/start" >&2
  exit 1
fi
compose --profile smoke run --rm canonical-smoke verify-only >"$evidence_dir/after-restart.json"

if ! cmp -s "$evidence_dir/before-restart.json" "$evidence_dir/after-restart.json"; then
  echo "canonical query proof changed across normal stop/start" >&2
  diff -u "$evidence_dir/before-restart.json" "$evidence_dir/after-restart.json" >&2 || true
  exit 1
fi

compose config >"$evidence_dir/compose.rendered.yml"
for image in \
  'nats:2.10-alpine@sha256:b83efabe3e7def1e0a4a31ec6e078999bb17c80363f881df35edc70fcb6bb927' \
  'semconnect-semstreams:v1.0.0-beta.153' \
  'semconnect-cs-api:beta.153' \
  'semconnect-canonical-smoke:beta.153'; do
  docker image inspect --format '{{.RepoTags}} {{.Id}} {{.Architecture}}/{{.Os}}' "$image"
done >"$evidence_dir/images.txt"
for input in \
  Dockerfile \
  deploy/compose.yml \
  deploy/nats.conf \
  deploy/semconnect.json \
  deploy/semstreams.json \
  deploy/canonical-system.v1.json \
  deploy/probe/Dockerfile \
  deploy/probe/main.go \
  deploy/verify-persistence.sh; do
  printf '%s  %s\n' "$(sha256_file "$input")" "$input"
done >"$evidence_dir/inputs.sha256"
sha256_file "$evidence_dir/compose.rendered.yml" >"$evidence_dir/compose.rendered.sha256"
sha256_file "$evidence_dir/before-restart.json" >"$evidence_dir/canonical-proof.sha256"

echo "greenfield first-start and restart-persistence proof passed"
echo "evidence: $evidence_dir"
echo "compose sha256: $(cat "$evidence_dir/compose.rendered.sha256")"
echo "canonical proof sha256: $(cat "$evidence_dir/canonical-proof.sha256")"
