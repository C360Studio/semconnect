# Upstream ask — semstreams: datastream vocabulary (predicates + class IRI)

**Repo:** <https://github.com/C360Studio/semstreams>
**Drafted from:** semconnect Stage 8 implementation (2026-05-16), framework pin `v1.0.0-beta.73`.
**Status:** ready to file (copy-paste).

## Summary

SOSA defines `Sensor`, `Observation`, `Procedure`, `ObservableProperty`, but not the OGC API CS API §10 "Datastream" concept (a stream of Observations produced by a System for a single ObservableProperty). The `vocabulary/sosa/` package ships no Datastream IRIs and no predicates for datastream-specific relations.

semconnect Stage 8 (`gateway/cs-api/datastream.go`) v0.1's POST /datastreams / GET /datastreams therefore mint a local IRI (`urn:c360studio:csapi:Datastream`) and one local predicate (`csapi.datastream.system`) to bridge the gap. SOSA's `observedProperty` predicate is correctly reused; the gap is the type IRI and the "datastream produced by sensor" relation.

This works at v0.1 but means:
- JSON-LD export of a Datastream entity emits the `urn:c360studio:csapi:` IRI — a downstream JSON-LD consumer can't resolve it (no registered ontology behind that URI).
- Sister gateways or other framework consumers can't discover Datastreams using a shared SemStreams vocabulary primitive — they'd have to grep for the local IRI string.

## File / line refs

- `vocabulary/sosa/iris.go` — SOSA IRIs (no Datastream, no `producedBy`).
- `parser/sensorml/predicates.go` — sensorml's predicate constants (we reuse `PredLabel` and `PredDescription`, which works).
- (semconnect) `gateway/cs-api/datastream.go:35-55` — the local IRI + predicate we mint.

## Proposed change

Add a `vocabulary/csapi/` (or `vocabulary/datastream/`) package in semstreams with:

```go
package csapi

const (
    // DatastreamClass is the IRI for CS API §10 Datastream entities —
    // a stream of Observations produced by one System (sensor or system
    // of systems) for one ObservableProperty.
    DatastreamClass = "http://www.opengis.net/spec/ogcapi-connectedsystems-1/1.0/Datastream"

    // ProducedBySystem binds a Datastream to the entity ID of the
    // System that produces it. Inverse of (forthcoming) ProducesDatastream.
    ProducedBySystem = "http://www.opengis.net/spec/ogcapi-connectedsystems-1/1.0/producedBy"

    // ResultTimeRange, PhenomenonTimeRange: ISO 8601 time-interval
    // representations for the datastream's temporal bounds (CS API §10.4).
    ResultTimeRange     = "http://www.opengis.net/spec/ogcapi-connectedsystems-1/1.0/resultTimeRange"
    PhenomenonTimeRange = "http://www.opengis.net/spec/ogcapi-connectedsystems-1/1.0/phenomenonTimeRange"

    // ResultType: om:Measurement, om:Category, om:CountObservation, etc.
    ResultType = "http://www.opengis.net/spec/ogcapi-connectedsystems-1/1.0/resultType"

    // Schema: SWE Common DataRecord describing the observation result
    // structure. Best modeled as a StorageRef pointer rather than an
    // inline triple — see SWE Common 3.0 in framework-primitives §Scope-cut.
)
```

The IRI choice mirrors how the OGC publishes spec-rooted IRIs for OGC API concepts (e.g. `/req/...` for requirements, `/Datastream` for the resource type). When the OGC publishes a canonical Datastream IRI (the spec is still WD), swap the constant; existing entities re-tag in a one-shot migration.

## Backward-compat note

Pure addition. Existing consumers ignore unknown predicates per RDF semantics.

## Observable impact in semconnect

When this lands, `gateway/cs-api/datastream.go` swaps:
- `DatastreamTypeIRI` (currently `urn:c360studio:csapi:Datastream`) → `vocabulary/csapi.DatastreamClass`
- `PredDatastreamSystem` (currently `"csapi.datastream.system"`) → `vocabulary/csapi.ProducedBySystem`

…and the `X-CS-Datastream-Subset: true` header on GET responses can be removed if the framework also gains predicates for `resultTimeRange`, `phenomenonTimeRange`, `resultType`, and `schema`. The subset header explicitly signals these gaps today.

## Suggested triage

- **Interim** (no upstream change): semconnect carries the local IRIs. Doc-comments at `gateway/cs-api/datastream.go` enumerate the gap; the `X-CS-Datastream-Subset` header signals it to clients.
- **Ideal** (this proposal): add the four-IRI module. Cheap to implement (it's constants), unlocks a richer datastream wire shape across the framework's consumer surface.

If the team prefers to wait until OGC stabilizes the CS API spec IRIs, an interim form is fine — the constant-name surface matters more than the URI form, since a one-line update to the constants ships against any URI choice.
