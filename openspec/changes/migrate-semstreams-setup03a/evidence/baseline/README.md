# SemStreams beta.160 baseline

Captured on 2026-10-01 before dependency or production configuration changes.
SemConnect source: `d0d06e00bf05a545f30ceea798db1c2b1ee47d4f`.
SemStreams: `v1.0.0-beta.160`, commit `8403a2218000e45a31c5132fbfe01af42ed04f14`,
tree `9ed5dd3792bca63ce87ebf449a180add918f59ed`.
Go: `go1.26.4 darwin/arm64`. Docker: `29.8.0`, `aarch64`.

## Results

| Gate | Result | Evidence |
| --- | --- | --- |
| `go test -race -count=1 ./...` | PASS | `go-checks.json`, `go-test-race.log` |
| `go vet ./...` | PASS | `go-checks.json`, `go-vet.log` |
| `go vet -tags=integration ./...` | PASS | `go-checks.json`, `go-vet-integration.log` |
| `go test -race -tags=integration -count=1 ./...` | PASS | `go-checks.json`, `go-test-integration-race.log` |
| `go build ./...` | PASS | `go-additional-checks.json`, `go-build.log` |
| `go mod verify` | PASS | `go-additional-checks.json`, `go-mod-verify.log` |
| Unchanged external ETS | **137 passed, 0 failed, 0 skipped** | `conformance/summary.txt`, TestNG XML |
| Root-only SensorML bake | PASS; root readable, embedded child absent | `conformance/root-only-bake-*.txt` |
| Greenfield preflight | Zero streams, consumers, messages, bytes | `persistence/first-start-jsz.json` |
| Real-NATS normal stop/start | PASS; exact canonical query proof parity | `persistence-execution.json` |
| Spatial bbox/polygon hits and remote bbox miss | PASS before and after second no-write restart | `spatial/` |

The ordinary Go suite includes real embedded NATS mutation, observation delivery,
ObjectStore, and lifecycle tests. The Docker persistence proof additionally uses
the pinned real SemStreams binary and NATS `2.14.4` image on isolated file storage.

The original full test and vet suites completed before new SETUP 03A reference
tests were added. The later build and module verification did not change source.
The supplemental spatial probe was added after the original gates completed;
it only reads `/areas`. Original dependency, configuration, harness, and OAS
hashes in `source-inputs.json` remained unchanged through these baseline runs.

## Isolation and persistence

Docker project and volume preflight found no matching existing objects.
Conformance project: `semconnect-03a-baseline-20261001`.
Persistence project: `semconnect-03a-base-persistence`.
Persistence volume: `semconnect-03a-beta160-20261001-fresh`.

The canonical proof SHA-256 before and after the first normal stop/start is
`4544c19899ccf1c9b461146be542c47dd85d4201aad5aeda2c9cf48946b63e32`.
Both index-readiness captures prove target revision 3 and indexed revision 3.
All three persistent services stopped with exit code 0 and `OOMKilled=false`.
The volume identity was unchanged. Exact images and input hashes are recorded
under `persistence/images.txt` and `persistence/inputs.sha256`.

The supplemental second restart performs no writes. Both bbox and polygon
queries return only `c360.semconnect.systems.csapi.system.v1` with Point
`[-96.797, 32.777]`; a distant bbox returns no features. Status, headers, and
raw response bodies are retained, with byte-equal normalized response summaries
before and after restart. Its index again reports target=indexed=3.

Final logs were captured before removing only the two baseline project stacks.
The beta.160 persistence volume is preserved; it must never be opened by the
target revision. Cleanup commands and retained volume identity are recorded
in `cleanup.json` and `preserved-baseline-volume.json`.

## Evidence handling

Execution JSON records exact commands, timestamps, and exit codes. Authoritative
container-health and HTTP observations are retained in `active-polls.jsonl`.
All gates passed on their first execution; no assertion, skip, filter, or fixture
change was used to obtain these results.
