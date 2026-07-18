#!/bin/sh
set -eu

docker=/usr/local/bin/docker
jq=/usr/bin/jq
sha=/usr/bin/shasum
git=/usr/bin/git
evidence_root=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repo_root=$(CDPATH= cd -- "$evidence_root/../../../../.." && pwd)

containers='semconnect-conformance-nats-1 semconnect-conformance-semstreams-backend-1 semconnect-conformance-cs-api-server-1 semconnect-conformance-teamengine-1'
$docker inspect $containers \
  | $jq -S '[.[] | {
      name: (.Name | ltrimstr("/")),
      containerId: .Id,
      imageId: .Image,
      configuredImage: .Config.Image,
      createdAt: .Created,
      startedAt: .State.StartedAt,
      status: .State.Status,
      composeProject: .Config.Labels["com.docker.compose.project"],
      composeService: .Config.Labels["com.docker.compose.service"],
      composeConfigHash: .Config.Labels["com.docker.compose.config-hash"]
    }]' >"$evidence_root/container-identities-pre-stop.json"

images='semconnect-conformance-semstreams-backend semconnect-conformance-cs-api-server semconnect-conformance-teamengine nats:2.10-alpine'
$docker image inspect $images \
  | $jq -S '[.[] | {
      id: .Id,
      repoTags: .RepoTags,
      repoDigests: .RepoDigests,
      createdAt: .Created,
      architecture: .Architecture,
      os: .Os
    }]' >"$evidence_root/image-identities.json"

cd "$repo_root"
repo_head=$($git rev-parse HEAD)
repo_branch=$($git branch --show-current)
semstreams_vendor_head=$($git -C conformance/.vendor/semstreams rev-parse HEAD)
ets_vendor_head=$($git -C conformance/.vendor/ets rev-parse HEAD)
go_mod_sha=$($sha -a 256 go.mod | /usr/bin/awk '{print $1}')
go_sum_sha=$($sha -a 256 go.sum | /usr/bin/awk '{print $1}')
pin_sha=$($sha -a 256 conformance/.ets-pin | /usr/bin/awk '{print $1}')
compose_sha=$($sha -a 256 conformance/compose.yml | /usr/bin/awk '{print $1}')
semstreams_cfg_sha=$($sha -a 256 conformance/compose.semstreams.config.json | /usr/bin/awk '{print $1}')
cs_api_cfg_sha=$($sha -a 256 conformance/compose.cs-api.config.json | /usr/bin/awk '{print $1}')
captured_at=$(/bin/date -u +%Y-%m-%dT%H:%M:%SZ)

$jq -n -S \
  --arg capturedAt "$captured_at" \
  --arg repoHead "$repo_head" \
  --arg repoBranch "$repo_branch" \
  --arg semstreamsVendorHead "$semstreams_vendor_head" \
  --arg etsVendorHead "$ets_vendor_head" \
  --arg goModSha "$go_mod_sha" \
  --arg goSumSha "$go_sum_sha" \
  --arg pinSha "$pin_sha" \
  --arg composeSha "$compose_sha" \
  --arg semstreamsCfgSha "$semstreams_cfg_sha" \
  --arg csApiCfgSha "$cs_api_cfg_sha" \
  '{
    capturedAt: $capturedAt,
    repository: {head: $repoHead, branch: $repoBranch},
    upstreamVendors: {
      semstreams: $semstreamsVendorHead,
      ets: $etsVendorHead
    },
    expectedSemstreams: {
      version: "v1.0.0-beta.149",
      commit: "7db0cdcb21577eaa52eb842c4ffb06a854f9a9b2"
    },
    sourceHashes: {
      "go.mod": $goModSha,
      "go.sum": $goSumSha,
      "conformance/.ets-pin": $pinSha,
      "conformance/compose.yml": $composeSha,
      "conformance/compose.semstreams.config.json": $semstreamsCfgSha,
      "conformance/compose.cs-api.config.json": $csApiCfgSha
    }
  }' >"$evidence_root/source-identity.json"
