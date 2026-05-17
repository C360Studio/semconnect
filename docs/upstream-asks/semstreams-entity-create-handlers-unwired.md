# Upstream ask — semstreams: `CreateEntityRequest` handlers are defined but not wired

**Repo:** <https://github.com/C360Studio/semstreams>
**Drafted from:** semconnect Stage 8 implementation (2026-05-16), framework pin `v1.0.0-beta.73`.
**Status:** **Filed at [C360Studio/semstreams#98](https://github.com/C360Studio/semstreams/issues/98) (OPEN, as of 2026-05-16; still OPEN at v1.0.0-beta.75 — verified `processor/graph-ingest/*.go` has no `CreateEntityRequest` subject wiring).** semconnect workaround: `gateway/cs-api/systems_post.go` `ingestTriples` uses `graph.mutation.triple.add_batch` (upsert semantics). Trade-off: no `409 Conflict` on duplicate POST — see CS API §7.6 which spec-requires it.

## Summary

`graph/mutation_requests.go` defines the entity-level mutation request types — `CreateEntityRequest`, `CreateEntityWithTriplesRequest`, `UpdateEntityRequest`, `UpdateEntityWithTriplesRequest`, `DeleteEntityRequest` — but `processor/graph-ingest/` does **not** wire NATS subjects for any of them. The triple-level mutations (`graph.mutation.triple.add`, `.add_batch`, `.remove`) are the only wired entity-write subjects.

This forces consumers that want "create a new entity with N triples atomically" to use `add_batch`, which has *upsert* semantics (CAS read-modify-write). For the CS API gateway specifically:

- `POST /systems` and `POST /datastreams` in semconnect (Stage 8) issue `AddTriplesBatchRequest` to write a new entity. Works because `AddTriple`'s CAS path creates the entity if it doesn't exist.
- Lost: `CreateEntityRequest` would let us distinguish "create new" from "update existing" — important for CS API §7.6 (POST is strictly create; 409 Conflict when the entity already exists). Today the gateway can't return 409, because `add_batch` happily upserts.
- Lost: `CreateEntityWithTriplesRequest` would carry the full `EntityState` shape (including `MessageType`, `Version`, `StorageRef`) — fields `AddTriple` synthesizes with defaults. Operators that want to capture provenance (e.g. "this entity was created from SensorML version X stored at S3 key K") have no way to express it through `add_batch`.

## File / line refs

- `graph/mutation_requests.go:17` — `CreateEntityRequest` defined, no handler.
- `graph/mutation_requests.go:25` — `UpdateEntityRequest` defined, no handler.
- `graph/mutation_requests.go:32` — `DeleteEntityRequest` defined, no handler.
- `graph/mutation_requests.go:39` — `CreateEntityWithTriplesRequest` defined, no handler.
- `graph/mutation_requests.go:47` — `UpdateEntityWithTriplesRequest` defined, no handler.
- `processor/graph-ingest/mutations.go:24-50` — `setupMutationHandlers` wires *only* triple-level subjects.
- `processor/graph-ingest/component.go:910-952` — `CreateEntity` (the public method) exists and is exercised by tests; it's just not exposed over NATS.

## Proposed change

Wire request-reply handlers in `processor/graph-ingest/mutations.go`:

```go
const (
    SubjectEntityCreate            = "graph.mutation.entity.create"
    SubjectEntityCreateWithTriples = "graph.mutation.entity.create_with_triples"
    SubjectEntityUpdate            = "graph.mutation.entity.update"
    SubjectEntityUpdateWithTriples = "graph.mutation.entity.update_with_triples"
    SubjectEntityDelete            = "graph.mutation.entity.delete"
)
```

Implementations are thin — `CreateEntity` / `UpdateEntity` / `DeleteEntity` already exist as methods. Each handler:

1. Decode `CreateEntityRequest` from request data.
2. Call the existing method (e.g. `c.CreateEntity(ctx, req.Entity)`).
3. Translate errors to `MutationResponse{Success: false, Error: err.Error()}`.
4. On success, return the post-write KVRevision (the same pattern `handleTripleAdd` already uses).

Suggested response: `MutationResponse` + `Entity *EntityState` for create/update (so callers see the assigned ID and the post-merge triple set).

`CreateEntityRequest` semantics should be **create-or-fail** (return Error if the entity already exists), distinct from `UpdateEntityRequest`'s **must-exist** semantics. This is what lets a CS API gateway return 409 Conflict on duplicate POST.

## Backward-compat note

Pure addition — new subjects, new handlers. Existing `add_batch` callers see no change. semconnect's Stage 8 `ingestTriples` helper would migrate from `add_batch` to `entity.create_with_triples` in a one-line swap, then gain the 409-Conflict path.

## Observable impact in semconnect

The Stage 8 implementation works around this with `add_batch` and accepts the upsert semantics. Doc-comments at `gateway/cs-api/systems_post.go` (`handleSystemPost` and `ingestTriples`) call out the deferral. When upstream lands the entity-level subjects, we swap and add the 409 path.

## Suggested triage

- **Interim** (no upstream change): the workaround in semconnect is small and well-commented. CS API conformance does not currently exercise the 409 case (it's not in the Common Part 1 spec, and the Botts ETS doesn't check it).
- **Ideal** (this proposal): wire all five entity-level handlers in one PR (the implementations are 5-10 LOC each). semconnect cuts ~20 LOC of workaround prose and gains the 409 path.
