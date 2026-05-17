# Upstream ask — semstreams: `parser/sensorml` drops `position` from SensorML input

**Repo:** <https://github.com/C360Studio/semstreams>
**Drafted from:** semconnect Stage 13 conformance probe (2026-05-16), framework pin `v1.0.0-beta.75`.
**Status:** ready to file (copy-paste).

## Summary

`parser/sensorml`'s type model and triple emitter both silently drop the SensorML `position` field. A POST of a SensorML PhysicalSystem document with a `position: {type: "Point", coordinates: [-122.4194, 37.7749, 10.0]}` block survives the round trip with everything else preserved (label, description, definition, identifiers, capabilities, characteristics, hosts, hostedBy, usedProcedure), but the spatial information is gone — neither readable on `graph.query.entity` nor indexable in `graph-index-spatial`.

This makes spatial-aware CS API endpoints (and any downstream consumer that wants per-entity geometry) impossible to serve correctly without re-parsing the original SensorML JSON outside the framework — defeating the framework's "SensorML is a first-class graph-bearing document" abstraction.

## Observable impact (semconnect Stage 13)

cs-api's `GET /systems/{id}` cannot satisfy the Botts ETS `systemItemHasGeometryOrValidTime` assertion (CS API §7 `/req/system/location-time`, MAY-priority). That test emits `SkipException` when geometry AND validTime are both absent — which is the only outcome cs-api can produce today regardless of input. The `SkipException` keeps the entire `systemfeatures` test group marked "not successfully finished" in TestNG, which in turn cascade-SKIPs ~26 dependent assertions in the `sensorml` and `geojson` groups.

End-to-end: a single missing triple chokepoints **~26 CS API conformance assertions** that have no other way to surface.

## File / line refs (at framework pin `v1.0.0-beta.75`, commit `e118099`)

The drop happens at the type-model layer — the parser never decodes the field, so the triple emitter has nothing to emit even if it wanted to:

- `parser/sensorml/types_process.go:40-55` — `AbstractProcess` struct (every SensorML process type embeds this). Fields: `ID`, `Label`, `Description`, `Definition`, `UniqueID`, `Identifiers`, `Classifiers`, `Characteristics`, `Capabilities`, `Keywords`, `Inputs`, `Outputs`, `Parameters`, `TypeOf`. **No `Position` / `Location`.**
- `parser/sensorml/types_physical.go:34-37` — `PhysicalSystem` extends `AbstractProcess` with `AttachedTo`, `Components`, `Connections`. **No `Position`.** Same for `PhysicalComponent` at line 11-13.
- `parser/sensorml/graphable.go:46-83` — `Asset.Triples()` emits triples for every field that exists on the type model. Per-field one-shot emissions for type, label, description, definition, identifier values, capability values, characteristic values; then `typeSpecificTriples()` for hosts / hostedBy / hasSubSystem / usedProcedure / attachedTo. Position is conspicuously absent from the emission list.
- `parser/sensorml/predicates.go:12-61` — predicate-name constants. No `PredPosition` / `PredLocation`.
- `vocabulary/sosa/iris.go` — SOSA `hasLocation` predicate IRI is not defined either (grep for `hasLocation` returns no matches). SOSA does have a `Location` concept (`<http://www.w3.org/ns/sosa/hasLocation>`) so the upstream OGC IRI exists; we just don't expose it.

## Spec reference

SensorML 2.0 (OGC 12-000r2) §7.1.6 defines the `position` element on `AbstractProcess` as a GML feature property holding either a `gml:Point`, `gml:Polygon`, or `gml:AbstractCurve` — for systems, the typical case is a `Point` with WGS84 coordinates representing the platform location.

SensorML+JSON encoding (the framework's wire form) carries `position` as a GeoJSON-shaped object, conventionally a `{type: "Point", coordinates: [lon, lat, alt?]}` block.

OGC API CS API §7 `/req/system/location-time` requires `/systems/{id}` to expose `geometry` OR `validTime` — geometry is the natural mapping from SensorML `position`.

SOSA exposes `sosa:hasLocation` (W3C SSN/SOSA recommendation 2017) as the predicate binding a Feature-of-Interest to a Geo-Feature.

## Proposed change

Three-layer change, smallest-to-largest:

### 1. `vocabulary/sosa/iris.go` — expose `hasLocation` IRI

```go
// Spatial predicates.
const (
    // HasLocation binds a Feature-of-Interest (System, Sensor,
    // Sample, etc.) to a Geo-Feature describing its spatial extent.
    // W3C SSN/SOSA recommendation 2017 §7.
    HasLocation = SOSANamespace + "hasLocation"
)
```

### 2. `parser/sensorml/types_process.go` — add `Position` to `AbstractProcess`

```go
type AbstractProcess struct {
    // ... existing fields ...
    Position *Position `json:"position,omitempty"`
}

// Position is the SensorML §7.1.6 spatial element. Wire form is a
// GeoJSON-shaped Point / Polygon / LineString. Stored verbatim as
// json.RawMessage so the framework doesn't have to parse and
// re-emit the geometry shape — consumers (graph-index-spatial,
// CS API gateways) read it as JSON.
type Position struct {
    Raw json.RawMessage `json:"-"` // populated by UnmarshalJSON
}

func (p *Position) UnmarshalJSON(data []byte) error {
    p.Raw = append(p.Raw[:0], data...)
    return nil
}

func (p Position) MarshalJSON() ([]byte, error) {
    return p.Raw, nil
}
```

(Alternative: model Position with typed Point / Polygon / etc. variants. The RawMessage approach is the minimum-viable surface — typed variants can land later without breaking the wire shape.)

### 3. `parser/sensorml/predicates.go` + `graphable.go` — emit a position triple

```go
// predicates.go
const PredPosition = "sensorml.process.position"

vocabulary.Register(PredPosition, vocabulary.WithIRI(sosa.HasLocation))
```

```go
// graphable.go — in Asset.Triples(), after the existing field emissions:
if base.Position != nil && len(base.Position.Raw) > 0 {
    out = append(out, message.Triple{
        Subject:   a.entityID,
        Predicate: PredPosition,
        Object:    string(base.Position.Raw), // serialized GeoJSON
    })
}
```

## Why "Raw GeoJSON string" as the triple object

The graph layer stores triple Objects as `string` — typed geometry would need a domain-specific encoding. Storing the GeoJSON-shaped JSON text verbatim has three properties:

1. **Round-trip is lossless** — `Object` bytes are exactly what came in over the wire.
2. **Spatial indexers can parse** — `graph-index-spatial` already consumes GeoJSON-shaped polygons (it does this for the `graph.spatial.query.polygon` request); adding Point reading is a small extension to the same parser.
3. **CS API gateways can serve directly** — `(geojson.Point){Coordinates: [...]}` is identical to the OGC API §7.4 `geometry` field shape, no transformation needed.

## Backward-compat note

Existing SensorML entities (any in long-lived dev or production graphs) gain nothing automatically — the triple is only emitted on new POSTs. Operators wanting historical entities to get position would re-POST them, or run a triple-rewrite over the existing graph. v0.1 deployments don't have this problem (only ephemeral conformance harness data exists).

## Suggested triage

- **Interim** (no upstream change): semconnect could extract `position` from the raw SensorML JSON during POST, mint our own predicate name (`cs-api.position`), and surface it on read. Works, but every sister-repo facing the same SensorML→Triple gap would have to invent its own predicate. Bad outcome long-term.
- **Ideal** (this proposal): the three-layer change above. ~150 lines of code, isolated to `parser/sensorml` + `vocabulary/sosa`, no breaking changes to existing consumers (the new field is additive on AbstractProcess; the new triple is additive on Triples()).

Once landed, semconnect drops the `X-CS-Reconstructed-Lossy: true` "position dropped" component, and `systemItemHasGeometryOrValidTime` flips from SKIP to PASS — un-gating the systemfeatures cascade and surfacing the next layer of real CS API conformance assertions in sensorml + geojson groups.
