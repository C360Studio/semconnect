## 1. Architect contract and handoff

- [x] 1.1 **architect** reviews ADR-S003 and all four capability specs, records approval or exact blockers, and runs
  `openspec validate migrate-semstreams-beta147` with a green result.
- [x] 1.2 **architect** freezes a seven-package provenance manifest from SemStreams commit
  `c8f0b92edf5ad5b491d5f4e81891bec817fae3cd`, including source path, destination path, file mode, and SHA-256 hash.
- [x] 1.3 **architect** freezes the semantic ledger at 19 transferred renames, 12 local camelCase corrections, and one
  full-IRI correction, with no alias or dual-read disposition.
- [x] 1.4 **architect** freezes the public compatibility matrix for HTTP/JSON, OMS, SensorML, SWE, GeoJSON, JSON-LD,
  RDF, OpenAPI, UI labels, NATS subjects, envelope types, audit headers, and conformance claims.
- [x] 1.5 **architect** defines the immutable cutover-manifest schema and marks production execution P0-blocked until
  deployment values, destructive scope, retained state, authoritative source, rollback owner, and approvals exist.
- [x] 1.6 **architect -> go-developer/svelte-developer** issues a formal signed handoff referencing the approved ADR,
  OpenSpec revision, provenance manifest, semantic ledger, compatibility matrix, and P0 cutover gate.

## 2. Go developer owned-package TDD and transfer

- [x] 2.1 **go-developer** writes failing package-boundary tests that detect all seven removed SemStreams imports and
  prove semconnect replacement packages and registrations are absent before implementation.
- [x] 2.2 **go-developer** transfers the seven exact package trees from the provenance commit, changes ownership imports
  to `github.com/c360studio/semconnect`, and records a machine-readable source-to-destination diff.
- [x] 2.3 **go-developer** updates all production, test, fixture, generated, and conformance imports without adding an
  old-path facade, replace directive, copied compatibility package, or ambient registration.
- [x] 2.4 **go-developer** writes failing OMS decoder-composition tests, wires owned `ogc.oms.v3` registration into
  every typed decoder registry, and proves the raw observation-return path remains intentionally registry-free.
- [x] 2.5 **go-developer** promotes Feasibility and association/composition terms into owned `vocabulary/csapi`, closes
  implementation scope for issues #70 and #71, and leaves issue #69 SWE Common Phase 2 out of scope.
- [x] 2.6 **go-developer** runs `go test ./message/oms ./parser/sensorml ./pkg/swecommon ./vocabulary/...` and archives
  green package and canonical-fixture output at the implementation commit.

## 3. Go developer semantic-contract TDD

- [x] 3.1 **go-developer** writes failing table tests for all 19 transferred predicate mappings and proves every old
  identity is absent from producers, queries, registrations, fixtures, and seeds.
- [x] 3.2 **go-developer** writes failing final-state builder tests for all 12 local camelCase corrections, including
  helper-hidden `cs-api.deployment.deployedSystems`, then implements the lower-kebab mappings.
- [x] 3.3 **go-developer** writes a failing Datastream test for the `sosa.ObservedProperty` IRI misuse, replaces it with
  registered `csapi.datastream.observed-property`, and proves RDF/JSON-LD still emits the SOSA IRI.
- [x] 3.4 **go-developer** routes every resource builder through beta.147 authoritative final-state validation and adds
  backend spies proving invalid predicates cause zero NATS, JetStream, ObjectStore, mutation, or query calls.
- [x] 3.5 **go-developer** replaces graph-facing regex duplication with beta.147 public entity validators and separates
  canonical graph IDs from opaque observation/path tokens.
- [x] 3.6 **go-developer** marks every graph entity relationship `@id`, leaves external IRIs and hrefs literal, and adds
  atomic no-I/O tests for invalid subject and reference candidates.
- [x] 3.7 **go-developer** implements shared bounded minting, preserving fitting identities and using `h-` plus full
  lowercase SHA-256 for overflow; golden tests cover every resource family, artifact role, and nested SensorML child.
- [x] 3.8 **go-developer** adds prefix-budget validation that fails before component/NATS initialization when the digest
  form cannot fit, with explicit zero-I/O evidence.
- [x] 3.9 **go-developer** removes legacy `cs-api.system.uid` and `cs-api.system.position` reads and proves no alias,
  dual-read, dual-write, permissive validator, or in-process graph translator remains.
- [x] 3.10 **go-developer** runs the beta.147 predicate audit over `gateway`, `cmd`, `message`, `parser`, `pkg`,
  `vocabulary`, `conformance/fixtures`, and `conformance/compose.semstreams.config.json` with zero findings.
- [x] 3.11 **go-developer** runs
  `go run github.com/c360studio/semstreams/cmd/entity-id-audit@v1.0.0-beta.147 .` with zero unclassified findings and
  archives the exact source-set report.

## 4. Svelte developer wire and UI parity

- [x] 4.1 **svelte-developer** writes failing UI tests for stable CS API labels on migrated Datastream, ControlStream,
  Command, SystemEvent, Feasibility, association, and artifact fields.
- [x] 4.2 **svelte-developer** uses explicit product-owned labels and descriptions so neither old camelCase predicates
  nor new lower-kebab storage identities leak as unintended user-facing text.
- [x] 4.3 **svelte-developer** updates generated TypeScript inputs only from the aligned public API and verifies removed
  SemStreams package ownership does not create stale UI types or catalogs.
- [x] 4.4 **svelte-developer** precisely classifies the six `${DEMO_PREFIX}` entity-ID templates for the beta.147 audit
  and proves each resolved runtime prefix satisfies the canonical contract.
- [x] 4.5 **svelte-developer** runs `npm --prefix ui run check`, `npm --prefix ui run build`, and
  `npm --prefix ui run test:e2e`, archiving green output and Playwright artifacts.
- [x] 4.6 **go-developer/svelte-developer -> reviewers** submit a formal handoff with code scope, failing-first tests,
  audit reports, parity results, known risks, and zero unresolved developer blockers.

## 5. Go developer dependency, NATS, and integration evidence

- [x] 5.1 **go-developer** aligns `go.mod`, `go.sum`, the conformance backend tag, backend commit, and migration
  evidence to SemStreams beta.147 commit `5cc22c109594e48b7f1cec04bcaaf0106d85495a` in one change set.
- [x] 5.2 **go-developer** proves real-NATS mutation/query/error classification, ownership/projection, `@id`
  foreign-edge, schema ObjectStore, and observation JetStream behavior with beta.147 integration tests.
- [x] 5.3 **go-developer** proves issue #65 post-break behavior: zero unclaimed/dropped CS API foreign edges and intact
  `NoBirthStub` hosted-child resolution under the exact migrated producer identity.
- [x] 5.4 **go-developer** proves observation subjects remain `cs-api.observations.{datastreamID}`, envelope type
  remains `ogc.oms.v3`, and audit/trace headers retain prior semantics.
- [x] 5.5 **go-developer** records a bounded release-commit audit showing no rule-processor config, rule pack,
  graph-event constructor, or `graph.events.*` consumer; the result is explicitly not applicable, not waived.
- [x] 5.6 **go-developer** runs `git ls-files '*.go' | xargs gofmt -l` with no output, `go vet ./...`,
  `go vet -tags=integration ./...`,
  `go test ./...`, `go test -race ./...`, `go test -tags=integration ./...`, and `go build ./...`, archiving uncached
  command output and the exact Go environment.

## 6. Go reviewer quality gate

- [x] 6.1 **go-reviewer** verifies the seven-package manifest and diff against the exact provenance commit and rejects
  undocumented divergence, copied aliases, missing tests/fixtures, or license drift.
- [x] 6.2 **go-reviewer** verifies all 32 semantic corrections (19 transferred renames, 12 local renames, and the
  full-IRI correction) through final-state tests, registrations, exact queries, fixtures, seeds, and boundary mappings.
- [x] 6.3 **go-reviewer** verifies context propagation, classified errors, zero-I/O validation, `@id` semantics,
  deterministic minting, collision resistance, prefix bounds, concurrency safety, and critical-path coverage.
- [x] 6.4 **go-reviewer** independently reruns the Go, race, vet, build, predicate-audit, entity-audit, and real-NATS
  commands from tasks 3.10, 3.11, and 5.6 and signs the exact outputs.
- [x] 6.5 **go-reviewer** rejects any legacy UID/position fallback, predicate alias, dual format, permissive mode,
  graph-state translator, mixed SemStreams revision, or test-only workaround.
- [x] 6.6 **go-reviewer -> svelte-reviewer/technical-writer** records formal approval or returns actionable findings to
  the developer; no documentation or release handoff proceeds on a conditional approval.

## 7. Svelte reviewer UX and accessibility gate

- [x] 7.1 **svelte-reviewer** verifies Svelte 5/TypeScript correctness, explicit semantic labels, generated-type parity,
  accessibility, keyboard behavior, responsive rendering, and absence of internal predicate leakage.
- [x] 7.2 **svelte-reviewer** independently reruns `npm --prefix ui run check`, `npm --prefix ui run build`, and
  `npm --prefix ui run test:e2e` and signs the exact output and artifacts.
- [x] 7.3 **svelte-reviewer -> technical-writer** records formal approval or returns actionable findings; UI parity is a
  release gate even when the external ETS is green.

## 8. Technical writer migration and evidence handoff

- [x] 8.1 **technical-writer** updates ADR-S001/S002 references, getting-started/status documentation, package
  ownership, framework pins, and upstream-ask history to reflect ADR-S003 without rewriting historical evidence.
- [x] 8.2 **technical-writer** marks SemStreams asks #200/#201 as transferred and semconnect issues #70/#71 as local
  migration work; issue #69 remains the separately deferred SWE Common Phase 2 backlog.
- [ ] 8.3 **technical-writer** produces the deployment-specific immutable cutover manifest with literal commands, exact
  NATS context/account, rendered bucket names, writers, retained resources, source revision/counts, probes, and owners.
- [x] 8.4 **technical-writer** records the identity-impact report for retained observations and artifacts; any changed
  retained reference keeps the cutover no-go until a separate plan is approved.
- [x] 8.5 **technical-writer** records rollback/no-go criteria, two-person destructive review, maintenance
  communication, evidence locations, and the rule/event not-applicable audit.
- [x] 8.6 **technical-writer -> architect/product owner/operator** delivers a complete evidence envelope; missing or
  stale evidence is an explicit blocker, never a silent waiver.

## 9. Rehearsal, external conformance, and release decision

- [ ] 9.1 **operator/program manager** rehearses the exact manifest against disposable fresh volumes: stop writers,
  remove only listed graph resources, start beta.147, reseed, and record source count and `ENTITY_STATES` revision.
- [x] 9.2 **operator/program manager** actively polls graph-index status until ready and indexed revision reaches the
  captured target, then proves predicate, relationship, entity, batch, spatial, scoped, schema, and observation parity.
- [x] 9.3 **operator/program manager** restarts without an intervening write and proves the same or later indexed
  revision and equivalent query results; fixed sleeps, process health, or silent logs do not satisfy this gate.
- [x] 9.4 **program manager** runs `./conformance/run.sh` from fresh volumes against the aligned external Botts ETS and
  archives exactly `137 passed, 0 failed, 0 skipped` plus TestNG and service logs.
- [x] 9.5 **go-reviewer/technical-writer** prove the ETS pin, tests, fixture intent, OpenAPI, and claimed conformance
  set were not modified, filtered, skipped, or weakened to obtain the green result.
- [ ] 9.6 **architect/product owner/operator** review every automated result, approval, retained-state decision, literal
  destructive command, and rollback owner and issue an explicit go/no-go decision for production.
- [ ] 9.7 **operator** executes the production manifest only after go approval; any mismatch, state poison,
  readiness stall, parity delta, or external failure stops writers and triggers a corrected reviewed rebuild.
- [ ] 9.8 **technical-writer** archives the production revisions, counts, removed and retained resources, readiness,
  query/replay parity, external ETS result, operator timestamps, and final architect/product-owner sign-off.
