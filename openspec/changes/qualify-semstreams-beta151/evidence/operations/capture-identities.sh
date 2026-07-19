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
semstreams_vendor_tree=$($git -C conformance/.vendor/semstreams rev-parse 'HEAD^{tree}')
ets_vendor_head=$($git -C conformance/.vendor/ets rev-parse HEAD)
semstreams_drift=$($git -C conformance/.vendor/semstreams status --porcelain=v1 --untracked-files=all --ignored=matching --ignore-submodules=none)
ets_drift=$($git -C conformance/.vendor/ets status --porcelain=v1 --untracked-files=all --ignored=matching --ignore-submodules=none)
go_mod_sha=$($sha -a 256 go.mod | /usr/bin/awk '{print $1}')
go_sum_sha=$($sha -a 256 go.sum | /usr/bin/awk '{print $1}')
pin_sha=$($sha -a 256 conformance/.ets-pin | /usr/bin/awk '{print $1}')
run_sha=$($sha -a 256 conformance/run.sh | /usr/bin/awk '{print $1}')
compose_sha=$($sha -a 256 conformance/compose.yml | /usr/bin/awk '{print $1}')
semstreams_cfg_sha=$($sha -a 256 conformance/compose.semstreams.config.json | /usr/bin/awk '{print $1}')
cs_api_cfg_sha=$($sha -a 256 conformance/compose.cs-api.config.json | /usr/bin/awk '{print $1}')
dev_handoff_sha=$($sha -a 256 openspec/changes/qualify-semstreams-beta151/evidence/development/go/go-developer-handoff.json | /usr/bin/awk '{print $1}')
build_log_sha=$($sha -a 256 conformance/output/compose-build-2026-07-18T16-40-05Z.log | /usr/bin/awk '{print $1}')
captured_at=$(/bin/date -u +%Y-%m-%dT%H:%M:%SZ)

$jq -n -S \
  --arg capturedAt "$captured_at" \
  --arg repoHead "$repo_head" \
  --arg repoBranch "$repo_branch" \
  --arg semstreamsVendorHead "$semstreams_vendor_head" \
  --arg semstreamsVendorTree "$semstreams_vendor_tree" \
  --arg etsVendorHead "$ets_vendor_head" \
  --arg semstreamsDrift "$semstreams_drift" \
  --arg etsDrift "$ets_drift" \
  --arg goModSha "$go_mod_sha" \
  --arg goSumSha "$go_sum_sha" \
  --arg pinSha "$pin_sha" \
  --arg runSha "$run_sha" \
  --arg composeSha "$compose_sha" \
  --arg semstreamsCfgSha "$semstreams_cfg_sha" \
  --arg csApiCfgSha "$cs_api_cfg_sha" \
  --arg devHandoffSha "$dev_handoff_sha" \
  --arg buildLogSha "$build_log_sha" \
  '{
    capturedAt: $capturedAt,
    qualifyingRunId: "2026-07-18T16-40-05Z",
    repository: {head: $repoHead, branch: $repoBranch},
    upstreamVendors: {
      semstreams: {
        commit: $semstreamsVendorHead,
        tree: $semstreamsVendorTree,
        cleanMaterializedSource: ($semstreamsDrift == ""),
        drift: $semstreamsDrift
      },
      ets: {
        commit: $etsVendorHead,
        cleanMaterializedSource: ($etsDrift == ""),
        drift: $etsDrift
      }
    },
    expectedSemstreams: {
      version: "v1.0.0-beta.151",
      tagObject: "784f22dc8d549d7781b88a2878bb679112aad494",
      commit: "ac75c322140fb2a6b55759d07a79874b4cb4d9cc",
      sourceTree: "120eeb353afb7d07aa1b3180de05f75494bac1a8"
    },
    preBuildAttestation: {
      path: "openspec/changes/qualify-semstreams-beta151/evidence/development/go/go-developer-handoff.json",
      sha256: $devHandoffSha,
      signedAt: "2026-07-18T16:36:17Z",
      evidenceBundleSha256: "bb81bbba709a67c6580cc893274aa9f7d2dc79775e6dce3d7277592da361f9e0"
    },
    buildLog: {
      path: "conformance/output/compose-build-2026-07-18T16-40-05Z.log",
      sha256: $buildLogSha
    },
    sourceHashes: {
      "go.mod": $goModSha,
      "go.sum": $goSumSha,
      "conformance/.ets-pin": $pinSha,
      "conformance/run.sh": $runSha,
      "conformance/compose.yml": $composeSha,
      "conformance/compose.semstreams.config.json": $semstreamsCfgSha,
      "conformance/compose.cs-api.config.json": $csApiCfgSha
    }
  }' >"$evidence_root/source-identity.json"
