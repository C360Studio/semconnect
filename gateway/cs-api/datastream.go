// Package csapi: this file defines the v0.1 Datastream entity model.
//
// Stage 13 — semstreams v1.0.0-beta.75 landed the `vocabulary/csapi`
// package with native OGC CS API type IRIs (`csapi.Datastream`) and CS API
// predicates. Stage 39's beta.91 pin split predicates into dotted internal
// names (`csapi.ProducedBy`) plus IRI boundary names (`csapi.ProducedByIRI`).
package csapi

import (
	"bytes"
	"encoding/json"

	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/parser/sensorml"
	"github.com/c360studio/semstreams/pkg/swecommon"
	"github.com/c360studio/semstreams/vocabulary/csapi"
	"github.com/c360studio/semstreams/vocabulary/sosa"
)

// DatastreamTypeIRI is the rdf:type Object value for Datastream entities.
// Stage 13: aliases csapi.Datastream (the framework's canonical IRI
// from v1.0.0-beta.75 onward). Kept as a named const so handler /test
// code reads naturally without leaking the package import everywhere.
const DatastreamTypeIRI = csapi.Datastream

// PredDatastreamSystem links a Datastream entity to the 6-part
// SemStreams entity ID of the System (Sensor) that produces it.
// Stage 39: aliases beta.91's dotted csapi.ProducedBy.
const PredDatastreamSystem = csapi.ProducedBy

// PredDatastreamSchema links a Datastream to the first-class
// csapi:SWESchemaDocument artifact entity that stores the canonical SWE
// Common result schema bytes. Stage 42 retires the previous gateway-local
// JSON predicate bridge and aliases beta.91's dotted relationship predicate.
const PredDatastreamSchema = csapi.HasResultSchema

const (
	predDatastreamPhenomenonTime = "cs-api.datastream.phenomenonTime"
	predDatastreamResultTime     = "cs-api.datastream.resultTime"
)

// Datastream is the v0.1 JSON shape for CS API §10 Datastream resources.
// Fields are the subset semstreams' vocabulary can losslessly round-trip
// today; deferred fields are listed in the package doc comment above.
type Datastream struct {
	ID               string             `json:"id"`
	Type             string             `json:"type"` // "Datastream"
	Name             string             `json:"name,omitempty"`
	Description      string             `json:"description,omitempty"`
	System           string             `json:"system,omitempty"`           // 6-part entity ID
	SystemID         string             `json:"system@id,omitempty"`        // CS API Part 2 shape
	SystemLink       *link              `json:"system@link,omitempty"`      // CS API Part 2 shape
	OutputName       string             `json:"outputName,omitempty"`       // CS API Part 2 shape
	ObservedProperty string             `json:"observedProperty,omitempty"` // IRI
	ObservedProps    []observedProperty `json:"observedProperties,omitempty"`
	PhenomenonTime   string             `json:"phenomenonTime,omitempty"`
	ResultTime       string             `json:"resultTime,omitempty"`
	Formats          []string           `json:"formats,omitempty"`
	ResultType       string             `json:"resultType,omitempty"`
	Schema           json.RawMessage    `json:"schema,omitempty"` // SWE Common DataRecord JSON
	Links            []link             `json:"links"`
}

type observedProperty struct {
	Definition  string `json:"definition,omitempty"`
	Label       string `json:"label,omitempty"`
	Description string `json:"description,omitempty"`
}

func (p observedProperty) getDefinition() string { return p.Definition }

type datastreamCollection struct {
	Type           string `json:"type"` // "DatastreamCollection"
	NumberMatched  int    `json:"numberMatched"`
	NumberReturned int    `json:"numberReturned"`
	Truncated      bool   `json:"truncated,omitempty"`
	// `items` (not `datastreams`) per CS API §10 / OGC API Common §7.14.
	// Mirrors systemCollection.Items — see Stage 10 note there.
	Items []Datastream `json:"items"`
	Links []link       `json:"links"`
}

type datastreamObservationSchema struct {
	ObsFormat    string         `json:"obsFormat"`
	ResultSchema map[string]any `json:"resultSchema"`
}

// datastreamFromState collapses an EntityState into the v0.1 Datastream
// JSON shape. Mirrors systemFromState — both read the same predicate
// helpers (firstStringObject / allStringObjects) for consistency.
func datastreamFromState(state graph.EntityState) Datastream {
	d := Datastream{
		ID:   state.ID,
		Type: "Datastream",
		Links: []link{
			{Href: "/datastreams/" + state.ID, Rel: "self", Type: string(MediaJSON)},
			// Parity with systemFromState — CS API §10 follows the same
			// canonical-link discipline as §7. Surfacing this early keeps
			// /datastreams/{id} from regressing the same way /systems/{id}
			// did at Stage 12 (see systems.go for the spec ref).
			{Href: "/datastreams/" + state.ID, Rel: "canonical", Type: string(MediaJSON)},
		},
	}
	if v, ok := firstStringObject(state.Triples, sensorml.PredLabel); ok {
		d.Name = v
	}
	if v, ok := firstStringObject(state.Triples, sensorml.PredDescription); ok {
		d.Description = v
	}
	if v, ok := firstStringObject(state.Triples, PredDatastreamSystem); ok {
		d.System = v
		d.SystemID = v
		d.SystemLink = &link{Href: "/systems/" + v, Rel: "system", Type: string(MediaJSON), Title: v}
	}
	if v, ok := firstStringObject(state.Triples, sosa.ObservedProperty); ok {
		d.ObservedProperty = v
		d.ObservedProps = []observedProperty{{Definition: v}}
	}
	if v, ok := firstStringObject(state.Triples, predDatastreamPhenomenonTime); ok {
		d.PhenomenonTime = v
	}
	if v, ok := firstStringObject(state.Triples, predDatastreamResultTime); ok {
		d.ResultTime = v
	}
	d.OutputName = "result"
	d.Formats = []string{string(MediaJSON), string(MediaOMS)}
	d.ResultType = "observation"
	if _, ok := firstStringObject(state.Triples, PredDatastreamSchema); ok {
		d.Links = append(d.Links, link{
			Href: "/datastreams/" + state.ID + "/schema",
			Rel:  "schema",
			Type: string(MediaJSON),
		})
	}
	d.Links = append(d.Links, link{
		Href: "/datastreams/" + state.ID + "/observations",
		Rel:  "observations",
		Type: string(MediaJSON),
	})
	return d
}

// isDatastreamKind reports whether the entity's rdf:type maps to our
// minted Datastream IRI. Symmetric with isSystemKind in systems.go.
func isDatastreamKind(triples []message.Triple) bool {
	typeIRI, ok := firstStringObject(triples, typeAliases...)
	if !ok {
		return false
	}
	return typeIRI == DatastreamTypeIRI
}

// datastreamToTriples is the inverse of datastreamFromState — builds the
// triple set the gateway publishes on POST /datastreams. The entity ID
// is set by the caller (handler mints from cfg.DatastreamIDPrefix +
// SensorML-style uniqueId sanitization).
//
// Required fields (validated by the handler before calling this):
//   - entityID (6-part SemStreams ID, validated by validateEntityID)
//   - either Name or Description (at least one human-readable)
//   - System (the producing entity, full 6-part ID)
//   - ObservedProperty (IRI)
//
// Triples emitted: rdf:type + PredLabel + PredDescription + PredSystem +
// sosa:observedProperty. Empty fields are skipped — clients sending a
// partial document get a partial triple set, not empty-string triples.
func datastreamToTriples(entityID string, d *Datastream) []message.Triple {
	triples := []message.Triple{
		{Subject: entityID, Predicate: sensorml.PredType, Object: DatastreamTypeIRI},
	}
	if d.Name != "" {
		triples = append(triples, message.Triple{Subject: entityID, Predicate: sensorml.PredLabel, Object: d.Name})
	}
	if d.Description != "" {
		triples = append(triples, message.Triple{Subject: entityID, Predicate: sensorml.PredDescription, Object: d.Description})
	}
	if d.System != "" {
		triples = append(triples, message.Triple{Subject: entityID, Predicate: PredDatastreamSystem, Object: d.System})
	}
	if d.ObservedProperty != "" {
		triples = append(triples, message.Triple{Subject: entityID, Predicate: sosa.ObservedProperty, Object: d.ObservedProperty})
	}
	if d.PhenomenonTime != "" {
		triples = append(triples, message.Triple{Subject: entityID, Predicate: predDatastreamPhenomenonTime, Object: d.PhenomenonTime})
	}
	if d.ResultTime != "" {
		triples = append(triples, message.Triple{Subject: entityID, Predicate: predDatastreamResultTime, Object: d.ResultTime})
	}
	return triples
}

func normalizeDatastreamSchema(raw json.RawMessage) (json.RawMessage, error) {
	return normalizeSWESchema(raw)
}

func normalizeSWESchema(raw json.RawMessage) (json.RawMessage, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return nil, nil
	}
	schema, err := swecommon.UnmarshalSchema(raw)
	if err != nil {
		return nil, err
	}
	canonical, err := swecommon.MarshalSchema(schema)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(canonical), nil
}
