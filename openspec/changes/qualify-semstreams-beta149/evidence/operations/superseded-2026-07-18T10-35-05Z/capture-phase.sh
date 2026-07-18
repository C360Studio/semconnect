#!/bin/sh
set -eu

phase=${1:-}
case "$phase" in
  pre|post) ;;
  *) echo "usage: $0 pre|post" >&2; exit 2 ;;
esac

docker=/usr/local/bin/docker
jq=/usr/bin/jq
evidence_root=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
phase_dir="$evidence_root/$phase"
raw_dir="$phase_dir/raw"
normalized_dir="$phase_dir/normalized"
mkdir -p "$raw_dir" "$normalized_dir"

fetch() {
  name=$1
  path=$2
  "$docker" exec semconnect-conformance-nats-1 \
    wget -qO- "http://cs-api-server:8080${path}" >"$raw_dir/$name.json"
  "$jq" -S '
    walk(
      if type == "array" and
        (length == 0 or all(.[]; type == "object" and has("id")))
      then sort_by(.id)
      else .
      end
    )
  ' "$raw_dir/$name.json" >"$normalized_dir/$name.json"
}

fetch systems-collection '/systems'
fetch system-item '/systems/c360.semconnect.systems.csapi.system.weather-station-01'
fetch system-datastreams '/systems/c360.semconnect.systems.csapi.system.weather-station-01/datastreams'
fetch system-subsystems '/systems/c360.semconnect.systems.csapi.system.weather-station-01/subsystems'
fetch datastreams-collection '/datastreams'
fetch datastream-schema '/datastreams/c360.semconnect.systems.csapi.datastream.weather-temperature-01/schema'
fetch observations-collection '/observations'
fetch spatial-bbox '/areas?bbox=-123,37,-122,38'
fetch spatial-polygon '/areas?polygon=%7B%22type%22%3A%22Polygon%22%2C%22coordinates%22%3A%5B%5B%5B-123%2C37%5D%2C%5B-122%2C37%5D%2C%5B-122%2C38%5D%2C%5B-123%2C38%5D%2C%5B-123%2C37%5D%5D%5D%7D'
fetch controlstream-commands '/controlstreams/c360.semconnect.systems.csapi.controlstream.ptz-01/commands'
fetch system-events '/systems/c360.semconnect.systems.csapi.system.weather-station-01/events'

# An additional readability proof for the second ObjectStore-backed artifact.
fetch command-schema '/controlstreams/c360.semconnect.systems.csapi.controlstream.ptz-01/schema'

"$docker" exec semconnect-conformance-nats-1 \
  wget -qO- 'http://localhost:8222/jsz?streams=true&config=true' \
  >"$raw_dir/nats-jsz.json"
"$jq" -S '{
  server_id,
  captured_at: .now,
  streams: [
    .account_details[0].stream_detail[]
    | select(.name == "KV_ENTITY_STATES" or
             .name == "KV_SPATIAL_INDEX" or
             .name == "CS_API_OBSERVATIONS" or
             .name == "OBJ_CS_API_ARTIFACTS")
    | {
        name,
        created,
        subjects: .config.subjects,
        storage: .config.storage,
        state: {
          messages: .state.messages,
          bytes: .state.bytes,
          first_seq: .state.first_seq,
          last_seq: .state.last_seq,
          num_subjects: (.state.num_subjects // 0),
          num_deleted: (.state.num_deleted // 0)
        }
      }
  ] | sort_by(.name)
}' "$raw_dir/nats-jsz.json" >"$phase_dir/resource-inventory.json"

{
  echo "phase=$phase"
  echo "captured_at=$(/bin/date -u +%Y-%m-%dT%H:%M:%SZ)"
  echo "writer_state=$($docker inspect semconnect-conformance-cs-api-server-1 --format '{{.State.Status}}')"
  echo "backend_state=$($docker inspect semconnect-conformance-semstreams-backend-1 --format '{{.State.Status}}')"
  echo "nats_state=$($docker inspect semconnect-conformance-nats-1 --format '{{.State.Status}}')"
} >"$phase_dir/capture-metadata.txt"

/usr/bin/find "$normalized_dir" -type f -name '*.json' -print0 \
  | /usr/bin/sort -z \
  | /usr/bin/xargs -0 /usr/bin/shasum -a 256 >"$phase_dir/normalized.sha256"
