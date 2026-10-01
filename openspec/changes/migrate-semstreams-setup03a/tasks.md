# SETUP 03A SemConnect migration tasks

Checked tasks record work executed, not blanket qualification. The migration remains **DRAFT / UNQUALIFIED**.
See [qualification](qualification.md) and [independent review](review.md) for the sole active federation blocker.

## 1. Architect and baseline

- [x] 1.1 Reuse the frozen SETUP 03A commit and record exact module, commit, tree, and owner-ruling evidence.
- [x] 1.2 Approve preserved consumer contracts and the three reference-case requirements before implementation.
- [x] 1.3 Inventory direct production/test imports and composed backend dependencies at both pins.
- [x] 1.4 Capture the current full Go, real-NATS, unchanged external conformance, and fresh-storage restart baseline.
- [x] 1.5 Open the migration draft PR with explicit incomplete gates and evidence links.

## 2. Go developer compatibility and reference cases

- [x] 2.1 Add failing-first tests for required lifecycle, registry/type, configuration, and vocabulary changes.
- [x] 2.2 Supply consumer-owned SensorML triples and vocabulary-mapping reference evidence.
- [x] 2.3 Supply typed create/reconcile/delete, exact revisions, and absent-target/no-stub reference evidence.
- [x] 2.4 Supply immutable artifact bytes and graph StorageReference reference evidence.
- [x] 2.5 After the current baseline is archived, align module, backend, deployment, conformance, and evidence pins.
- [x] 2.6 Apply bounded compatibility changes; retain strict one-attempt mutations and root-only projection.
- [x] 2.7 Record every behavior difference with an upstream source or reproduced defect and proving test.

## 3. Target qualification

- [x] 3.1 Run all Go gates; unit/race, vet and build pass; integration/race fails only retained federation HTTP 201.
- [x] 3.2 Run real-NATS lifecycle tests with fresh storage and explicit readiness synchronization.
- [x] 3.3 Run normal stop and no-write restart; prove graph revisions, query, artifacts, and observation parity.
- [x] 3.4 Run unchanged ETS: exactly 137 passed, 0 failed, 0 skipped; retain raw report and service logs.
- [x] 3.5 Re-measure consumer and composed backend dependency closure, including retained tagged tests.

## 4. Independent review and handoff

- [x] 4.1 Obtain scoped independent implementation/evidence approval; full migration and merge approval are withheld.
- [x] 4.2 Reviewer verifies no conformance/assertion/fixture/OAS weakening and records exact reviewed revision.
- [x] 4.3 Record frontend review as N/A only after confirming there is no frontend or generated public-type change.
- [x] 4.4 Record pins, changes, attribution, blockers, closure and the prepared three-case SemEngine handoff.
- [x] 4.5 Keep PR draft and migration unqualified while any required gate or reviewer approval remains incomplete.

## 5. Unresolved release gates

- [ ] 5.1 Resolve the foreign Datastream contract gap without bypass, weakened assertion or independently newer pin.
- [ ] 5.2 Pass all required integration assertions and obtain full migration/merge approval.
- [ ] 5.3 Record the authorized SemEngine issue-8 handoff comment URL.
