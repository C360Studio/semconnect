# SETUP 03A SemConnect migration tasks

## 1. Architect and baseline

- [x] 1.1 Reuse the frozen SETUP 03A commit and record exact module, commit, tree, and owner-ruling evidence.
- [x] 1.2 Approve preserved consumer contracts and the three reference-case requirements before implementation.
- [ ] 1.3 Inventory direct production/test imports and composed backend dependencies at both pins.
- [ ] 1.4 Capture the current full Go, real-NATS, unchanged external conformance, and fresh-storage restart baseline.
- [ ] 1.5 Open the migration draft PR with explicit incomplete gates and evidence links.

## 2. Go developer compatibility and reference cases

- [ ] 2.1 Add failing-first tests for required lifecycle, registry/type, configuration, and vocabulary changes.
- [ ] 2.2 Supply consumer-owned SensorML triples and vocabulary-mapping reference evidence.
- [ ] 2.3 Supply typed create/reconcile/delete, exact revisions, and absent-target/no-stub reference evidence.
- [ ] 2.4 Supply immutable artifact bytes and graph StorageReference reference evidence.
- [ ] 2.5 After the current baseline is archived, align module, backend, deployment, conformance, and evidence pins.
- [ ] 2.6 Apply bounded compatibility changes; retain strict one-attempt mutations and root-only projection.
- [ ] 2.7 Record every behavior difference with an upstream source or reproduced defect and proving test.

## 3. Target qualification

- [ ] 3.1 Run formatting, module verification, vet including integration tags, build, unit, race, and integration gates.
- [ ] 3.2 Run real-NATS lifecycle tests with fresh storage and explicit readiness synchronization.
- [ ] 3.3 Run normal stop and no-write restart; prove graph revisions, query, artifacts, and observation parity.
- [ ] 3.4 Run unchanged ETS: exactly 137 passed, 0 failed, 0 skipped; retain raw report and service logs.
- [ ] 3.5 Re-measure consumer and composed backend dependency closure, including retained tagged tests.

## 4. Independent review and handoff

- [ ] 4.1 Independent Go reviewer verifies preserved contracts, context ownership, no hidden retries, and evidence.
- [ ] 4.2 Reviewer verifies no conformance/assertion/fixture/OAS weakening and records exact reviewed revision.
- [ ] 4.3 Record frontend review as N/A only after confirming there is no frontend or generated public-type change.
- [ ] 4.4 Technical writer records pins, changes, attribution, blockers, closure, and reference-case handoff to SemEngine.
- [ ] 4.5 Keep PR draft and migration unqualified while any required gate or reviewer approval remains incomplete.
