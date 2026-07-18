# Final external conformance no-weakening audit

This audit approves OpenSpec task 9.5 only. It does not authorize production
and does not silently complete the separate rehearsal, replay, or release
decision tasks.

## External result identity

The raw TestNG report is
`conformance/output/testng-report-2026-07-18T02-03-23Z.xml`.

```text
$ xmllint --xpath 'name(/*)' conformance/output/testng-report-2026-07-18T02-03-23Z.xml
testng-results

$ xmllint --xpath 'string(/*/@total)' ...
137
$ xmllint --xpath 'string(/*/@passed)' ...
137
$ xmllint --xpath 'string(/*/@failed)' ...
0
$ xmllint --xpath 'string(/*/@skipped)' ...
0

$ xmllint --xpath 'count(//test-method[@status="PASS" and not(@is-config="true")])' ...
137
$ xmllint --xpath 'count(//test-method[@status="FAIL" and not(@is-config="true")])' ...
0
$ xmllint --xpath 'count(//test-method[@status="SKIP" and not(@is-config="true")])' ...
0
$ xmllint --xpath 'count(//test-method[@status="PASS" and @is-config="true"])' ...
23
```

The suite is `ogcapi-connectedsystems10-0.1-SNAPSHOT`. Its report contains
all 23 expected test classes, including the mutation lifecycle, update,
GeoJSON, SensorML, advanced filtering, and all advertised Part 2 slices.

## ETS pin and pristine vendor tree

```text
$ sed -n 's/^ETS_COMMIT=//p' conformance/.ets-pin
d9caf33fcd0c4a3c1a582e8ba9b12b753277afd4

$ git show HEAD:conformance/.ets-pin | sed -n 's/^ETS_COMMIT=//p'
d9caf33fcd0c4a3c1a582e8ba9b12b753277afd4

$ git -C conformance/.vendor/ets rev-parse HEAD
d9caf33fcd0c4a3c1a582e8ba9b12b753277afd4

$ git -C conformance/.vendor/ets rev-parse HEAD^{tree}
875847e194ffdb2aef2b617c9a9a0d9add037331

$ git -C conformance/.vendor/ets status --porcelain=v1 --untracked-files=all
[no output]
```

Tracked and cached diff checks also returned exit 0. The ETS commit was not
bumped, patched, or supplemented with untracked test code.

## TeamEngine invocation

`conformance/run.sh` verifies the pinned suite registration and invokes only:

```text
iut=http://cs-api-server:8080
mutation-tests-enabled=true
mutation-iut-policy=dedicated-mutable-iut
```

No include, exclude, group-selection, or skip parameter is passed. Mutation
tests are explicitly enabled against the disposable mutable IUT. The raw
report confirms `CreateReplaceDeleteTests` and `UpdateTests` ran and passed.

## Fixture, OpenAPI, and conformance declarations

The only tracked fixture diff is one additive field:

```json
"uniqueId": "urn:ets:system:weather-station-01"
```

This freezes deterministic retained-state identity; it does not remove an ETS
input or weaken fixture intent.

The OpenAPI diff is 10 additions and 10 removals, all in descriptions. It
removes the retired `cs-api.system.position` fallback claim, documents strict
client-supplied Datastream IDs, and updates canonical lower-kebab predicates.
No path, operation, media type, required property, or response requirement was
removed or relaxed.

`gateway/cs-api/conformance.go` is unchanged from the repository base and
still declares 25 classes. Its handler/test count invariant remains intact.

## Spatial and legacy-fallback closeout

The signed spatial review is
`evidence/review/go-spatial/approval.json`. A final source audit returned zero
matches for old UID/position aliases, pre-beta response logic, and body-text
error parsing. `runSpatialQuery` uses `RequestWithHeaders`, `ClassifyReply`,
and a top-level-array gate.

A final normalized live post-restart request returned five stable GeoJSON
Point Features:

```text
$ docker exec semconnect-conformance-teamengine-1 \
    curl -fsS 'http://cs-api-server:8080/areas?bbox=-180,-90,180,90'
FeatureCollection with 5 Point features: `deployment.01`,
`deployment.child01`, `system.01`, `hosted-platform-bake-01`, and
`weather-station-01`.
```

## No-write restart evidence

The archived readiness records prove the same stable revision across restart:

```text
pre-restart:  target=118 indexed=118 ready=true
post-restart: target=118 indexed=118 ready=true
```

The follow-up archive closes the earlier query-equivalence caveat. It contains
11 normalized JSON bodies on each side of the no-write restart:

- System item and collection;
- Datastream collection and System-scoped Datastreams;
- System events and subsystems;
- ControlStream commands;
- Datastream schema;
- Observation collection;
- spatial bbox and polygon results.

```text
$ find conformance/output/replay-2026-07-18T02-03-23Z/pre -type f | wc -l
11
$ find conformance/output/replay-2026-07-18T02-03-23Z/post -type f | wc -l
11

$ diff -ru conformance/output/replay-2026-07-18T02-03-23Z/pre \
    conformance/output/replay-2026-07-18T02-03-23Z/post
[no output; exit 0]
```

Every corresponding SHA-256 pair also matches; all 22 file hashes are bound in
`evidence-hashes.sha256`. The authoritative archived post-restart readiness
record has SHA-256
`9adfd4d3120bc99b51759b6725311da6052f3d682557293216670d67dd9b22a3`
and again records target/indexed revision `118/118`. Task 9.3 evidence is
therefore satisfied. This reviewer did not modify its task checkbox while
updating the task 9.5 artifact.

## Heartbeat shutdown error

The archived repeated-restart backend log contains the same real beta.147
shutdown failure:

```text
2026-07-18T02:14:58.296894377Z ERROR Service stop failed
service=heartbeat
error="heartbeat service not running (status: stopped)"

ERROR Error stopping services
error="stop errors: [failed to stop service heartbeat: heartbeat service not running (status: stopped)]"

Error: graceful shutdown failed: stop errors: [failed to stop service heartbeat:
heartbeat service not running (status: stopped)]
```

The archived log SHA-256 is
`b905725bb93b78c2cb2b1b3316907b4be9ce499ddf99dead655f51ec4fe0e0f9`.
The backend restarted, returned index readiness `118/118`, and served all 11
normalized query bodies unchanged. The error therefore did not manufacture or
conceal the already-completed TestNG result or corrupt replay state. It
indicates a non-idempotent/double-stop heartbeat lifecycle in SemStreams
beta.147 and is an upstream production-readiness concern. It must be fixed or
explicitly dispositioned before any production authorization; it is not
hidden, filtered, or relabeled as success here.

## Decision

Task 9.5 is approved: the `137 passed, 0 failed, 0 skipped` result was not
obtained by changing the pinned ETS, filtering its groups, disabling mutation
tests, weakening fixture intent, relaxing the OAS, or reducing the claimed
conformance set.

Production remains unauthorized.
