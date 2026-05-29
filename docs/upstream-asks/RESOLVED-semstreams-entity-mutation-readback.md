# RESOLVED upstream ask — semstreams: entity mutation handlers and read-back semantics

**Repo:** <https://github.com/C360Studio/semstreams>
**Drafted from:** semconnect Stage 8 implementation (2026-05-16), framework pin `v1.0.0-beta.73`.
**Status:** **RESOLVED in `v1.0.0-beta.81`** via
[C360Studio/semstreams#120](https://github.com/C360Studio/semstreams/issues/120)
and PR #137. Entity-level mutation subjects are wired, and
`graph.MutationResponse.Degraded` now distinguishes "write committed,
post-write read-back failed" from "write did not commit".

**semconnect follow-up:** migrate the gateway write path from
`graph.mutation.triple.add_batch` plus delete fan-out to
`graph.mutation.entity.*`. That migration is local semconnect work now;
it is no longer blocked on semstreams.

## 2026-05-29 update

`v1.0.0-beta.87` includes the beta.81 fixes for the entity mutation
contract. The relevant framework surface is now:

- `graph.mutation.entity.create`
- `graph.mutation.entity.create_with_triples`
- `graph.mutation.entity.update`
- `graph.mutation.entity.update_with_triples`
- `graph.mutation.entity.delete`

The response contract is stable enough for CS API semantics:

- `Success=true, Degraded=false`: write committed and read-back succeeded.
- `Success=true, Degraded=true`: write committed, but post-write
  read-back failed. Gateways should not retry the mutation.
- `Success=false`: write did not commit.

## Summary

This ask originally existed because `graph/mutation_requests.go` defined
the entity-level mutation request types but `processor/graph-ingest/`
only wired triple-level subjects. semconnect had to write new entities
through `graph.mutation.triple.add_batch`, which has upsert semantics.

For the CS API gateway specifically:

- `POST /systems` and `POST /datastreams` in semconnect (Stage 8) issue `AddTriplesBatchRequest` to write a new entity. Works because `AddTriple`'s CAS path creates the entity if it doesn't exist.
- Lost: `CreateEntityRequest` would let us distinguish "create new" from "update existing" — important for CS API §7.6 (POST is strictly create; 409 Conflict when the entity already exists). Today the gateway can't return 409, because `add_batch` happily upserts.
- Lost: `CreateEntityWithTriplesRequest` would carry the full `EntityState` shape (including `MessageType`, `Version`, `StorageRef`) — fields `AddTriple` synthesizes with defaults. Operators that want to capture provenance (e.g. "this entity was created from SensorML version X stored at S3 key K") have no way to express it through `add_batch`.

Those are now semconnect implementation gaps, not semstreams gaps.

## Local migration target

The next semconnect write-path cleanup should:

- Use `graph.mutation.entity.create_with_triples` for POST creates and
  map `ErrorCodeEntityExists` to `409 Conflict`.
- Use `graph.mutation.entity.update_with_triples` or a single
  entity-level replace flow for PUT/PATCH, preserving the existing
  body/path safety gates.
- Use `graph.mutation.entity.delete` for DELETE and retire the
  per-predicate `deleteAllEntityTriples` fan-out plus
  `X-CS-Partial-Delete` inconsistency signaling.
- Treat `Success=true, Degraded=true` as committed, with an audit header
  or response header if we need to expose post-write read-back loss.

Keep this file for traceability; do not treat it as an open upstream ask.
