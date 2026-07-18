# Rollback, no-go, and maintenance communication

## Literal rollback boundary

Before the first approved bucket-removal command runs, any failure leaves the
beta.141 deployment and state untouched. The operator may restart or roll back
the beta.141 application normally.

After the first bucket removal begins, binary rollback is prohibited:

- Never start beta.141 against rebuilt beta.147 graph state.
- Never start beta.147 against retained beta.141 graph state.
- Never restore only one graph or index bucket into a mixed layout.
- Never enable a predicate alias, dual read/write, permissive validator, or
  in-process state translator to rescue the window.

Post-destructive recovery is: keep writers stopped, correct the producer,
source, or deployment, obtain review for the corrected immutable manifest, and
repeat a clean rebuild from authoritative sources.

## No-go and stop criteria

No destructive command may run if any of these is true:

- manifest checksum or literal commands differ between reviewers;
- semconnect and SemStreams revisions or image digests are missing or mixed;
- NATS server, account, context, writer inventory, bucket names, or counts do
  not match the manifest;
- a retained observation or artifact reference changes identity;
- authoritative source revision, checksum, expected input count, or owner is
  missing;
- operator, architect, product owner, or either destructive reviewer has not
  approved the same manifest checksum;
- the prior rehearsal, engineering gates, or external ETS evidence is missing
  or stale.

After deletion starts, keep writers stopped and fail closed on:

- `graph_state_reset_required`;
- source or resulting entity-count mismatch;
- graph-index readiness false, degraded, stalled, or below the captured target
  revision;
- predicate, entity, relationship, batch, spatial, scoped collection, schema,
  observation, or foreign-edge parity delta;
- no-write restart/replay revision or query delta;
- any external ETS failure or skip;
- evidence that a test, fixture intent, OpenAPI contract, conformance claim, or
  harness assertion was weakened.

## Required human review

Production requires five approvals over one immutable manifest checksum:

1. architect;
2. product owner;
3. operator and rollback owner;
4. destructive reviewer A;
5. destructive reviewer B.

Names, timestamps, and checksum values are intentionally absent until assigned.
Placeholders are not approval. The conformance rehearsal must also name its
operator and two independent destructive reviewers before execution.

## Maintenance communication checkpoints

| Checkpoint | Audience | Required content |
|---|---|---|
| T-24h or earlier | users and owners | window, impact, writer freeze, rollback boundary, contacts |
| T-30m | owners and reviewers | manifest checksum, revisions, resources, retained-state result |
| Writers stopped | operator and reviewers | connection/service proof; explicit authorization before first deletion |
| Reseed complete | approvers | input/result counts and captured target revision |
| Readiness complete | all approvers | status samples, query parity, no-write restart parity |
| External ETS complete | all approvers | TestNG path and exact `137/0/0` summary; no-weakening review |
| Go or no-go | affected users and approvers | decision, timestamps, current writer state, follow-up owner |

## Evidence envelope

The final go/no-go review must reference:

- ADR-S003 and all four OpenSpec capability specs;
- architecture provenance, semantic ledger, compatibility matrix, manifest
  schema, and validation output;
- signed Go and Svelte developer/reviewer handoffs;
- immutable deployment manifest and checksum;
- pre/post resource inventory and retained-state identity report;
- seed source revision, checksum, expected inputs, and actual result counts;
- graph-index status timeline and captured revisions;
- live query and no-write replay parity;
- observation, artifact, ownership/projection, and foreign-edge evidence;
- rule/event not-applicable audit;
- fresh TestNG, service, seed, readiness, and build logs;
- final architect, product-owner, operator, and destructive-reviewer approvals.

Missing or stale evidence is a no-go, not a waiver.
