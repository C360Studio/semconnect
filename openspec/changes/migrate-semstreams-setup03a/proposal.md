# Migrate SemConnect to the shared SETUP 03A SemStreams pin

## Why

SemEngine extraction needs consumer evidence at one frozen SemStreams revision. SemConnect currently runs
`v1.0.0-beta.160`; its OGC behavior is an independent consumer baseline and cannot be inferred from SemSource's
results. Qualify SemConnect on the already selected SETUP 03A pin while SemEngine development proceeds.

## Exact revisions

| Role | Go module version | Full commit | Source tree |
| --- | --- | --- | --- |
| Current | `v1.0.0-beta.160` | `8403a2218000e45a31c5132fbfe01af42ed04f14` | `9ed5dd3792bca63ce87ebf449a180add918f59ed` |
| Shared target | `v1.0.0-beta.162.0.20260930150212-8b99efe9c66a` | `8b99efe9c66a4faa4fa509f9f62cc6bad8392128` | `605f83cb8492eda3bd347ac30a211b9babf3931f` |

The target was frozen by the [SETUP 03A owner ruling][pin-ruling]. `v1.0.0-beta.163` was absent when the baseline
was established. Reuse this exact commit; do not select a newer main or a subsequently published tag independently.
Source identity was cross-checked against the local SemStreams Git object. The target commit date is
2026-09-30T15:02:12Z.

## What changes

- Capture a fresh current conformance, real-NATS, persistence/restart, and dependency-closure baseline before any
  dependency change. Historical `137 passed / 0 failed / 0 skipped` is context, not a fresh result.
- Adapt to the frozen SemStreams contracts with failing-first tests for changed behavior. Preserve SemConnect's
  OGC responses, exact revision fences, one-attempt mutations, conflict and uncertain-commit handling, root-only
  SensorML projection, immutable schema artifacts, observation delivery, and spatial queries.
- Register SemConnect's eleven graph resource payload types in a SemConnect-owned backend composition root,
  as required by upstream ADR-103. Replace the stock backend binary where it cannot register consumer types.
- Re-run the unchanged 137-case external conformance suite and the same lifecycle and no-write restart workload
  on isolated fresh target storage. Attribute every difference to an intended upstream contract or reproduced defect.
- Supply three compact, consumer-owned reference cases for SemEngine: SensorML/vocabulary; typed exact-revision
  create/reconcile/delete with an absent relationship target; immutable bytes plus graph `StorageReference`.
- Record direct imports, composed backend dependencies, retained tagged-test closure, framework gaps, evidence,
  blockers, and independent Go reviewer sign-off.

## Boundaries

OGC semantics, SensorML/OMS/SWE mappings, vocabulary, fixtures, and conformance ownership stay in SemConnect.
This migration remains on SemStreams. A later SemEngine adoption needs its own qualified contract; no binary may
link both frameworks. No production deployment or cross-version volume reuse is part of this proposal.

## Qualification and review

A passing migration requires the exact pins above, complete Go and real-NATS gates, equal persistence/readiness
behavior after restart, `137 passed / 0 failed / 0 skipped`, an attribution for every observed difference, and
independent Go reviewer approval. Do not change ETS assertions, filters, skip behavior, fixtures' intent, OAS
claims, or result parsing to manufacture a pass. Failing or unavailable evidence stays a blocker in the draft PR.

[pin-ruling]: https://github.com/C360Studio/semengine/issues/7#issuecomment-5928488878
