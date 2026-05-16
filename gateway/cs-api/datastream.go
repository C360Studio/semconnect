// Package csapi: this file defines the v0.1 Datastream entity model.
//
// SOSA has no Datastream class and the semstreams framework
// (v1.0.0-beta.73) ships no datastream vocabulary. v0.1 emits a minimal
// subset using locally-minted HTTPS IRIs (DatastreamTypeIRI,
// PredDatastreamSystem) plus reused dc:title / dc:description /
// sosa:observedProperty. Deferred-field enumeration + upstream-ask
// rationale live in docs/upstream-asks/semstreams-datastream-vocabulary.md.
//
// The `X-CS-Datastream-Subset: true` response header on GET signals the
// deferred-field gap to clients, matching the X-CS-Reconstructed-Lossy
// (Stage 4) and X-CS-Geometry-Available (Stage 5) deferral pattern.
package csapi

import (
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/parser/sensorml"
	"github.com/c360studio/semstreams/vocabulary/sosa"
)

const (
	// DatastreamTypeIRI is the rdf:type Object value for v0.1 Datastream
	// entities. The OGC CS API spec does not yet publish a canonical IRI
	// and SOSA has none; we mint an HTTPS-shaped IRI under c360studio's
	// own vocabulary namespace. HTTPS form (vs urn:c360studio:...) so
	// JSON-LD consumers can dereference (or 404 cleanly) rather than
	// reject as an unregistered URN. When the spec publishes one, this
	// constant flips to it; existing entities re-tag in a one-shot
	// upstream migration.
	DatastreamTypeIRI = "https://c360studio.github.io/semconnect/vocabulary/v0.1/Datastream"

	// PredDatastreamSystem links a Datastream entity to the 6-part
	// SemStreams entity ID of the System (Sensor) that produces it.
	// Symmetric with sensorml.PredIsHostedBy in shape (entity-ID Object)
	// but distinct in semantics — a Datastream is OUTPUT of a System,
	// not PHYSICALLY MOUNTED ON it. SOSA has no equivalent (sosa:madeBySensor
	// is for Observations, not Datastreams). HTTPS form for the same
	// JSON-LD compatibility reason as DatastreamTypeIRI.
	PredDatastreamSystem = "https://c360studio.github.io/semconnect/vocabulary/v0.1/producedBy"
)

// Datastream is the v0.1 JSON shape for CS API §10 Datastream resources.
// Fields are the subset semstreams' vocabulary can losslessly round-trip
// today; deferred fields are listed in the package doc comment above.
type Datastream struct {
	ID               string `json:"id"`
	Type             string `json:"type"` // "Datastream"
	Name             string `json:"name,omitempty"`
	Description      string `json:"description,omitempty"`
	System           string `json:"system,omitempty"`           // 6-part entity ID
	ObservedProperty string `json:"observedProperty,omitempty"` // IRI
	Links            []link `json:"links"`
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
	return triples
}
