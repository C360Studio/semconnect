# Vendored OGC API — Connected Systems OAS3 source

This directory contains a vendored snapshot of the OAS3 source from
[`opengeospatial/ogcapi-connected-systems`](https://github.com/opengeospatial/ogcapi-connected-systems)
at commit `3fd86c73e744b7e2faaf7f1c17366bfb9ff4cd6f` (2026-04-20).

## What's here

- `part1/` — OGC API Connected Systems Part 1: Feature Resources OAS source
  (paths, schemas, parameters, responses, examples for systems, deployments,
  procedures, sampling features, properties, collections).
- `part2/` — Part 2: Dynamic Data OAS source (datastreams, observations,
  control streams, commands, system events).
- `common/` — shared OGC API Common schemas.
- `LICENSE-OGC.txt` — original OGC license file (unmodified).

## What's served

This directory is **not** what cs-api-server serves at `GET /api`. The
served document is `../openapi.yaml`, hand-authored to reflect cs-api's
actual v0.1 surface (honest schemas, `X-CS-*` response headers for
documented deferrals, `x-not-implemented-at-v01: true` on paths
deferred to follow-up stages). `api/upstream/` is the source-of-truth
reference we work from when expanding the served spec stage by stage.

## License

OGC's license (`LICENSE-OGC.txt`) is permissive ("free to implement,
use, copy, modify, merge, publish, distribute, sublicense") with two
conditions:

1. Original copyright notices retained — see `LICENSE-OGC.txt`.
2. If modified, add a notice that modifications haven't been OGC-approved
   — see `NOTICE-OF-MODIFICATIONS.md`.

Both conditions are satisfied. `api/openapi.yaml` (the served document)
references this directory's content but is a separate hand-authored work
adapted from the OGC source.

## Bumping the vendored snapshot

To pull a newer OGC OAS revision:

1. Pick the commit: `gh api repos/opengeospatial/ogcapi-connected-systems/commits/master --jq '.sha'`
2. `rm -rf api/upstream/{part1,part2,common}/`
3. Clone OGC repo at the new SHA, copy `api/part1/openapi/` →
   `api/upstream/part1/`, `api/part2/openapi/` → `api/upstream/part2/`,
   `common/` → `api/upstream/common/`, `LICENSE` → `LICENSE-OGC.txt`.
4. Update the SHA + date at the top of this file.
5. Diff `api/openapi.yaml` against the new upstream — adopt schema
   changes for paths we serve; check `x-not-implemented-at-v01` paths
   for shape drift.
6. Re-run conformance harness.
