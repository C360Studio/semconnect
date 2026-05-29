// Package csapi: this file defines the v0.1 Datastream entity model.
//
// Stage 13 — semstreams v1.0.0-beta.75 landed the `vocabulary/csapi`
// package with native OGC CS API IRIs (`csapi.Datastream`,
// `csapi.ProducedBy`, etc.). Pre-Stage-13 we minted local HTTPS IRIs
// under c360studio's own namespace because no canonical IRIs existed;
// the new constants flip us to the spec-rooted IRI stem and retire the
// `X-CS-Datastream-Subset: true` deferral header.
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
// Stage 13: aliases csapi.ProducedBy.
const PredDatastreamSystem = csapi.ProducedBy

// PredDatastreamSchema stores the v0.1 SWE Common DataRecord schema
// JSON that observation SWE encoders use for this Datastream. semstreams
// beta.88 intentionally scope-cuts schema storage from vocabulary/csapi
// (the long-term primitive is a StorageRef), so this gateway-local
// predicate is a narrow bridge rather than an upstream vocabulary fork.
const PredDatastreamSchema = "cs-api.datastream.schema"

// Datastream is the v0.1 JSON shape for CS API §10 Datastream resources.
// Fields are the subset semstreams' vocabulary can losslessly round-trip
// today; deferred fields are listed in the package doc comment above.
type Datastream struct {
	ID               string          `json:"id"`
	Type             string          `json:"type"` // "Datastream"
	Name             string          `json:"name,omitempty"`
	Description      string          `json:"description,omitempty"`
	System           string          `json:"system,omitempty"`           // 6-part entity ID
	ObservedProperty string          `json:"observedProperty,omitempty"` // IRI
	Schema           json.RawMessage `json:"schema,omitempty"`           // SWE Common DataRecord JSON
	Links            []link          `json:"links"`
}

// datastreamRef is the per-item shape inside a DatastreamCollection.
// Mirrors systemRef.
type datastreamRef struct {
	ID    string `json:"id"`
	Type  string `json:"type"` // "Datastream"
	Links []link `json:"links,omitempty"`
}

type datastreamCollection struct {
	Type           string `json:"type"` // "DatastreamCollection"
	NumberMatched  int    `json:"numberMatched"`
	NumberReturned int    `json:"numberReturned"`
	Truncated      bool   `json:"truncated,omitempty"`
	// `items` (not `datastreams`) per CS API §10 / OGC API Common §7.14.
	// Mirrors systemCollection.Items — see Stage 10 note there.
	Items []datastreamRef `json:"items"`
	Links []link          `json:"links"`
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
	}
	if v, ok := firstStringObject(state.Triples, sosa.ObservedProperty); ok {
		d.ObservedProperty = v
	}
	if v, ok := firstStringObject(state.Triples, PredDatastreamSchema); ok && json.Valid([]byte(v)) {
		d.Schema = json.RawMessage(v)
		d.Links = append(d.Links, link{
			Href: "/datastreams/" + state.ID + "/schema",
			Rel:  "schema",
			Type: string(MediaJSON),
		})
	}
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
	if len(d.Schema) > 0 {
		triples = append(triples, message.Triple{Subject: entityID, Predicate: PredDatastreamSchema, Object: string(d.Schema)})
	}
	return triples
}

func normalizeDatastreamSchema(raw json.RawMessage) (json.RawMessage, error) {
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
