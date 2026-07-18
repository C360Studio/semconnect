## ADDED Requirements

### Requirement: Beta.149 dependency pins are exact and aligned
Semconnect SHALL use SemStreams module version `v1.0.0-beta.149`, and its conformance backend SHALL check out commit
`7db0cdcb21577eaa52eb842c4ffb06a854f9a9b2` identified as `v1.0.0-beta.149`. No mixed module, tag, commit, image, or
documentation pin is permitted.

#### Scenario: A pin diverges
- **WHEN** any executable or documented current-target pin resolves to another SemStreams revision
- **THEN** automated verification fails before a stack starts
- **AND** qualification and production approval remain blocked

### Requirement: The complete upstream delta has an explicit disposition
The qualification SHALL record all commits from beta.147 through beta.149. It SHALL identify issue #549's fix as
PR #550 commit `cc34be3d565405d86d9e13375013ce3522fa1d5f` and prove semconnect does not configure or consume the unrelated
agentic-loop, agentic-tools, publish-agent, or rule-action behavior in the release delta.

#### Scenario: An unrelated delta is live
- **WHEN** source, configuration, or subject audit finds a semconnect consumer of an unrelated changed subsystem
- **THEN** not-applicable disposition fails
- **AND** a separate behavior contract and tests are required before qualification continues

### Requirement: Local and integration verification is green
The aligned source revision MUST pass focused upstream service regression tests, semconnect unit and race tests,
real-NATS integration tests, vet, integration vet, and build. Compatibility aliases, dual paths, or test-only
workarounds SHALL NOT be introduced to obtain a green result.

#### Scenario: A local gate fails
- **WHEN** any required command fails or requires an undocumented exception
- **THEN** beta.149 is not qualified
- **AND** no external or production result may waive the failure

### Requirement: Coordinated shutdown is clean under the issue 549 ordering
The live beta.149 backend SHALL exit cleanly when parent-context cancellation precedes `Manager.StopAll`. Evidence
MUST include normal SIGTERM delivery, exit code zero, no forced kill, `OOMKilled=false`, no container error, and a
bounded log audit with no heartbeat, metrics-forwarder, service-manager, or graceful-shutdown failure signature.

#### Scenario: Heartbeat already stopped before manager teardown
- **WHEN** the running backend receives SIGTERM and heartbeat reaches terminal state before `Manager.StopAll`
- **THEN** coordinated shutdown completes successfully
- **AND** repeated cleanup does not double-close a channel or report a false service error

#### Scenario: Process or log evidence is ambiguous
- **WHEN** exit metadata is missing, the stop times out, a forced kill occurs, or a forbidden signature is present
- **THEN** the shutdown gate fails
- **AND** health checks or later successful startup do not override the failure

### Requirement: No-write restart preserves graph readiness and query results
The harness SHALL freeze graph writers, capture authoritative target revision and normalized probes, restart the
beta.149 backend without an intervening write, actively poll graph-index readiness to the captured target, and prove
equivalent post-restart results. It SHALL preserve graph, observation, and artifact state.

#### Scenario: Replay parity passes
- **WHEN** beta.149 restarts after a clean no-write stop
- **THEN** indexed revision reaches at least the captured target
- **AND** normalized predicate, entity, relationship, spatial, schema, observation, and API probes are equivalent

#### Scenario: State or revision diverges
- **WHEN** state requires a new wipe, readiness stalls, revision regresses, or a normalized probe differs
- **THEN** qualification fails closed
- **AND** no compatibility reader, unreviewed reseed, or destructive recovery is attempted

### Requirement: External conformance remains complete and unweakened
A fresh-volume beta.149 run against the unchanged Botts ETS MUST report exactly
`137 passed, 0 failed, 0 skipped`. The ETS pin, tests, fixtures, OpenAPI, conformance declarations, and filter/skip
logic SHALL be reviewed against the beta.147 accepted baseline.

#### Scenario: Green result changes the authority or scope
- **WHEN** a test, fixture intent, conformance claim, filter, or skip condition changed to obtain green
- **THEN** the result is rejected
- **AND** beta.149 remains unqualified

### Requirement: Qualification does not authorize production
Production SHALL remain no-go until this change's complete signed evidence and all inherited ADR-S003 manifest,
retained-state, destructive-scope, owner, rollback, and approval gates receive explicit architect, product-owner, and
operator approval.

#### Scenario: Technical qualification passes without production approvals
- **WHEN** all automated beta.149 gates pass but any inherited manifest value or approval is absent
- **THEN** beta.149 is only a qualified candidate
- **AND** no production stop, wipe, deployment, or writer release occurs
