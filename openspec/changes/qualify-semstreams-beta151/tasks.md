## 1. Architect delta contract and handoff

- [x] 1.1 **architect** resolves the nonexistent beta.515 request to beta.151 using exact remote tag evidence.
- [x] 1.2 **architect** records beta.150/beta.151 tag objects, peeled commits, and all nine intervening commits.
- [x] 1.3 **architect** maps PR #554 and PR #567 to live semconnect mutation lanes and dispositions every other delta.
- [x] 1.4 **architect** defines TDD, structural, local, shutdown/replay, external, manifest, rollback, and
  approval gates.
- [x] 1.5 **architect -> developers/operators/reviewers/writer** signs the immutable implementation handoff.
- [x] 1.6 **architect** amends the production handoff to greenfield Compose deployment only: clean NATS, no migration or
  destructive gate, named persistent storage, layered readiness, restart persistence, and two-role approval.

## 2. Go developer pin and exposure TDD

- [x] 2.1 **go-developer** writes a failing alignment test for module, tag, commit, source, and conformance pins.
- [x] 2.2 **go-developer** updates pins together to beta.151 / `ac75c322...`; product compatibility code is forbidden.
- [x] 2.3 **go-developer** records a source/config/subject audit for every changed subsystem and live mutation lane.
- [x] 2.4 **go-developer** proves all semconnect-produced IDs, predicates, references, and foreign edges are canonical.
- [x] 2.5 **go-developer** runs unit, race, integration, vet, integration vet, and build and archives exact output.

## 3. Structural and upstream-focused TDD

- [x] 3.1 **go-developer** first demonstrates beta.149 lacks the beta.151 structural/trusted-RMW proof surface.
- [x] 3.2 **go-developer** proves valid entity create/update payloads persist unchanged on beta.151.
- [x] 3.3 **go-developer** sends invalid predicate, ID, and reference mutations directly over live NATS and proves
  classified atomic rejection with unchanged entity bytes and KV revision.
- [x] 3.4 **go-developer** proves trusted owner RMW cannot launder resident poison and preserves error classification.
- [x] 3.5 **go-developer** proves remove no-op causes no revision bump and valid foreign-edge routing remains intact.
- [x] 3.6 **go-developer** runs focused upstream PR #554, #567, #561, graph-index synchronization, and lifecycle tests.

## 4. Disposable qualification shutdown and replay rehearsal

- [x] 4.1 **operator** scans every retained `ENTITY_STATES` record with authoritative validation and records zero
  poison, exact record count, bucket identity, and revision.
- [x] 4.2 **operator** seeds canonical fixtures, exercises a real hosted-child foreign edge, captures normalized
  probes, and freezes every graph writer at a recorded boundary.
- [x] 4.3 **operator** sends normal SIGTERM and captures container/process identity, timestamps, exit zero,
  `OOMKilled=false`, no forced kill, and bounded shutdown-log results.
- [x] 4.4 **operator** restarts without writes, polls graph-index to the frozen target revision, and proves
  replay parity.
- [x] 4.5 **operator** proves foreign-edge claimed/dropped counters and structural rejection evidence match
  expectations.

## 5. Independent review

- [x] 5.1 **go-reviewer** verifies exact provenance, all nine commit dispositions, exposure, and no compatibility code.
- [x] 5.2 **go-reviewer** reruns local/upstream tests and independently reviews atomic rejection and trusted-RMW proof.
- [x] 5.3 **go-reviewer** verifies retained-state coverage, writer freeze, shutdown metadata, readiness, and replay
  evidence.
- [x] 5.4 **go-reviewer** signs approval or returns actionable blockers before external qualification proceeds.

## 6. External conformance, greenfield bundle, and production decision

- [x] 6.1 **program manager** runs fresh-volume external conformance and archives exactly `137/0/0` with exact pins.
- [x] 6.2 **technical-writer/go-reviewer** prove no ETS, fixture, OpenAPI, claim, filter, skip, or parser weakening.
- [ ] 6.3 **go-developer/operator/technical-writer** implement, independently verify, and hash the greenfield production
  Compose bundle and approval-free manifest. Evidence SHALL cover exact NATS/SemStreams beta.151/semconnect inputs,
  persistent named storage, non-secret config and the internal-only topology's explicit no-secret boundary,
  empty-namespace preflight, canonical seed, layered health/query readiness, clean Compose stop/start without volume
  removal, persistence parity, and the accepted `137/0/0` qualification. No migration, wipe, retained-state
  compatibility, or old-state rollback artifact is required.
- [ ] 6.4 **product owner/operator** each record explicit `go` or `no-go` over the same immutable task 6.3 manifest
  SHA-256. One person may hold both roles only through separate role-specific attestations; any missing/stale/mismatched
  input is no-go. A go authorizes only the stated first greenfield deployment.
- [x] 6.5 **operator** performs no production action without a complete task 6.3 bundle and explicit task 6.4 go.
