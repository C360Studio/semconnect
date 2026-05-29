# RESOLVED upstream ask — semstreams: `parser/sensorml` drops `uniqueId` from the triple set

**Repo:** <https://github.com/C360Studio/semstreams>
**Drafted from:** semconnect Stage 18 conformance probe (2026-05-17), framework pin `v1.0.0-beta.75`.
**Status:** **RESOLVED in v1.0.0-beta.79** (semstreams commit `edf51da` / issue #115). semconnect Stage 29 adopted `sensorml.PredUniqueID`, removed the SensorML uid workaround append, and keeps a legacy read fallback for data written under `cs-api.system.uid`.

## Summary

`parser/sensorml`'s `Asset.Triples()` emitter preserves a process's `Label`, `Description`, `Definition`, `Identifiers[].Value`, `Capabilities[].Value`, `Characteristics[].Value`, hosts/hostedBy/usedProcedure/attachedTo — but silently drops `AbstractProcess.UniqueID`. The minted SemStreams entity ID survives (it's the triple Subject), but the *original* client-submitted `uniqueId` (typically a URN or UUID like `urn:example:dev:42`) does not.

A read-back via `graph.query.entity` followed by reverse-mapping to either SensorML JSON or a CS API JSON System loses the client's identifier completely.

## Observable impact (semconnect Stage 18)

Two CS API ETS assertions fail because of this:

- `sensorMlMediaTypeWriteParsesSystemBodyWhenMutationEnabled` — POSTs an `application/sml+json` body with a `uniqueId`, GETs the resulting Location back, and asserts the response carries the submitted uid via `uid`, `uniqueId`, or `properties.uid`. Without preservation: all three are null.
- `geoJsonMediaTypeWriteParsesSystemBodyWhenMutationEnabled` — same scenario for `application/geo+json` Feature bodies.

Stage 18 ships a sister-side workaround: cs-api stores the uid under a `cs-api.system.uid` predicate at POST time and echoes it on every read shape. Symmetric with the Stage 14 `cs-api.system.position` workaround. Both retire when the framework owns the mapping.

## File / line refs (at framework pin `v1.0.0-beta.75`, commit `e118099`)

- `parser/sensorml/types_process.go:40-55` — `AbstractProcess.UniqueID` field is declared. The parser DOES decode it correctly from `application/sml+json` bodies. The data is present on the in-memory model; the loss happens at emission.
- `parser/sensorml/graphable.go:46-83` — `Asset.Triples()` walks every per-field emission. No uid case in the switch — the field is just never emitted.
- `parser/sensorml/predicates.go:12-61` — no `PredUniqueID` constant exists.

## Spec reference

SensorML 2.0 (OGC 12-000r2) §7.1.3 defines `uniqueId` on every Process — a globally-unique identifier the producer assigns (URN, UUID, vendor-scoped string). Distinct from the gateway-assigned entity ID (which is shaped for NATS-token-safety and may not even be related to the source identifier).

OGC API Connected Systems v1.0 §7 specifies that a System resource representation MUST surface the producer's identifier so a client can correlate the resource it created via POST against subsequent GETs. The CS API ETS asserts this via three field-name fallbacks: `uid` (JSON / Feature short form), `uniqueId` (SensorML lineage), `properties.uid` (Feature container).

## Proposed change

Three-layer change in `parser/sensorml`:

1. **`vocabulary/sosa/iris.go` (or a new `parser/sensorml/predicates.go` const)**: add `PredUID = "sensorml.process.uid"` to match the existing `sensorml.process.*` namespace (all-lower-snake convention — matches `sensorml.process.type`, `sensorml.process.position`).
2. **`parser/sensorml/graphable.go:46-83`**: in `Asset.Triples()`, after the existing per-field emissions and before `typeSpecificTriples()`, emit `{Subject: id, Predicate: PredUniqueID, Object: process.Base().UniqueID}` when `UniqueID != ""`.
3. **Reverse map** (if the framework owns a reconstruction primitive): surface the uid back onto `AbstractProcess.UniqueID` when the triple is present.

## Migration path for downstream consumers

Symmetric to the position-preservation ask: cs-api will keep its `cs-api.system.uid` workaround predicate active during a deprecation window. Once the framework predicate ships, cs-api's read code reads from both (preferring the framework one). Forward-compat for any data written under the workaround predicate before the cutover.

## Files affected on the downstream (cs-api) side once this lands

- `gateway/cs-api/systems_post.go` — drop the post-Triples append of the uid workaround.
- `gateway/cs-api/systems.go:systemFromState` — read from the framework predicate first, fall back to PredSystemUID for legacy data.
- `gateway/cs-api/sensorml.go:buildAbstractProcess` — same.
- `gateway/cs-api/uid_preservation_test.go` — flip tests to assert the framework predicate is preferred.

## Related

- [[RESOLVED-semstreams-sensorml-position-preservation]] — sibling workaround for the same emission gap on `position`. Both have the same shape (field present on the type model, missing from the emission list).
