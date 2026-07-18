## 1. Architect delta contract and handoff

- [x] 1.1 **architect** verifies tag `v1.0.0-beta.149` peels to
  `7db0cdcb21577eaa52eb842c4ffb06a854f9a9b2` and records tag-object provenance.
- [x] 1.2 **architect** audits beta.147..beta.149, identifies issue #549 / PR #550 commit
  `cc34be3d565405d86d9e13375013ce3522fa1d5f`, and classifies every other commit.
- [x] 1.3 **architect** defines exact pin, local, shutdown, readiness/replay, external conformance, no-weakening, and
  production no-go gates without modifying signed beta.147 evidence.
- [x] 1.4 **architect -> go-developer/operator/reviewer/technical-writer** signs a formal implementation handoff.

## 2. Go developer pin-alignment TDD

- [x] 2.1 **go-developer** writes a failing alignment test covering `go.mod`, `go.sum`, conformance tag, and commit.
- [x] 2.2 **go-developer** updates the module and conformance backend together to beta.149 commit
  `7db0cdcb21577eaa52eb842c4ffb06a854f9a9b2`; no product compatibility code is permitted.
- [x] 2.3 **go-developer** records a source/config/subject audit proving agentic-loop, agentic-tools, publish-agent, and
  rule-action deltas are not consumed; a finding blocks this change and requires a new spec.
- [x] 2.4 **go-developer** runs focused upstream beta.149 service tests for HeartbeatService, MetricsForwarder,
  BaseService draining, repeated Stop, cancellation-before-StopAll, and spent-instance Start rejection.
- [x] 2.5 **go-developer** runs `go test ./...`, `go test -race ./...`, `go test -tags=integration ./...`,
  `go vet ./...`, `go vet -tags=integration ./...`, and `go build ./...` and archives exact outputs.

## 3. Operator shutdown and replay rehearsal

- [x] 3.1 **operator/program manager** starts a disposable beta.149 stack, seeds canonical fixtures, captures the
  authoritative target revision, and records normalized pre-stop probes.
- [x] 3.2 **operator/program manager** freezes graph writers, sends normal SIGTERM to the backend, and captures
  container ID, start/finish timestamps, exit code, OOM/forced-kill/error state, and bounded shutdown logs.
- [x] 3.3 **operator/program manager** fails on any issue #549 signature, generic service-stop failure, nonzero exit,
  forced kill, timeout, missing metadata, or genuine teardown failure.
- [x] 3.4 **operator/program manager** restarts without a write, actively polls graph-index readiness to the captured
  target, and archives equivalent normalized query/API probes plus preserved observation/artifact evidence.

## 4. Go reviewer gate

- [x] 4.1 **go-reviewer** independently verifies tag/commit/module alignment and the complete delta disposition.
- [x] 4.2 **go-reviewer** reruns all task 2 commands and rejects compatibility code, weakened assertions, or hidden use
  of unrelated beta.148/beta.149 behavior.
- [x] 4.3 **go-reviewer** reviews shutdown synchronization, process metadata, log filter shape, writer freeze,
  readiness target, and replay evidence; log silence alone is not approval.
- [x] 4.4 **go-reviewer -> technical-writer/program manager** signs approval or returns actionable blockers.

## 5. External conformance and evidence

- [x] 5.1 **program manager** runs a fresh-volume external suite and archives exactly
  `137 passed, 0 failed, 0 skipped`, TestNG output, service logs, revision readiness, and exact source pins.
- [x] 5.2 **technical-writer/go-reviewer** hash/diff the ETS pin, tests, fixture intent, OpenAPI, conformance claims,
  and filter/skip logic against the accepted beta.147 baseline and prove no weakening.
- [x] 5.3 **technical-writer** updates current-target documentation without rewriting historical or signed beta.147
  evidence and assembles one beta.149 evidence index.
- [ ] 5.4 **architect/product owner/operator** issue an explicit production go/no-go decision only after this change
  and every inherited ADR-S003 deployment manifest and approval gate are complete.
- [x] 5.5 **operator** performs no production action unless task 5.4 records explicit go authorization.
