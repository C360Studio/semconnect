// Stage 35 — PATCH /datastreams/{id} parity for the CS API `conf/update`
// surface. Mirrors Stage 19's /systems PATCH stance: partial-update,
// no upsert, and no JSON Merge Patch null-as-delete at v0.1.
package csapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/c360studio/semconnect/parser/sensorml"
	csapivocab "github.com/c360studio/semconnect/vocabulary/csapi"
	"github.com/c360studio/semstreams/message"
)

// handleDatastreamPatch serves PATCH /datastreams/{id} — CS API §10
// partial-update parity with /systems.
//
// Body shape is the same Datastream JSON shape POST/PUT accepts, but
// every field is optional. Non-empty fields replace the corresponding
// triple; absent/empty fields leave the existing triple untouched.
// The path ID is authoritative, and a body `id` (when supplied) must
// match it before any destructive operation begins.
//
// 404 if the entity does not exist (no upsert; PUT is the upsert path).
// `schema: null` is rejected rather than treated as delete; omit schema
// to leave the stored SWE Common schema unchanged.
func (c *Component) handleDatastreamPatch(w http.ResponseWriter, r *http.Request) {
	pathID := r.PathValue("id")
	if err := validateEntityID(pathID); err != nil {
		w.Header().Set("X-CS-Attempted-ID", pathID)
		writeJSONError(w, http.StatusBadRequest, "invalid datastream id: "+err.Error())
		return
	}
	w.Header().Set("X-CS-Attempted-ID", pathID)

	if err := requireMediaType(r.Header.Get("Content-Type"), string(MediaJSON)); err != nil {
		w.Header().Set("Accept", string(MediaJSON))
		writeJSONError(w, http.StatusUnsupportedMediaType, err.Error())
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeJSONError(w, http.StatusRequestEntityTooLarge,
				fmt.Sprintf("request body exceeds %d bytes", maxErr.Limit))
			return
		}
		writeJSONError(w, http.StatusBadRequest, "could not read request body")
		return
	}

	var in Datastream
	if err := json.Unmarshal(body, &in); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid Datastream JSON: "+err.Error())
		return
	}
	if in.Type != "" && in.Type != "Datastream" {
		writeJSONError(w, http.StatusBadRequest,
			fmt.Sprintf("expected Datastream, got %q", in.Type))
		return
	}
	if in.ID != "" && in.ID != pathID {
		writeJSONError(w, http.StatusBadRequest,
			fmt.Sprintf("body id %q does not match path %q", in.ID, pathID))
		return
	}
	if in.System != "" {
		if err := validateEntityIDStrict(in.System); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid system reference: "+err.Error())
			return
		}
	}

	hasSchema := len(bytes.TrimSpace(in.Schema)) > 0
	if hasSchema {
		if bytes.Equal(bytes.TrimSpace(in.Schema), jsonNull) {
			writeJSONError(w, http.StatusBadRequest,
				"schema null is not supported on PATCH; omit schema to leave it unchanged")
			return
		}
		schema, err := normalizeDatastreamSchema(in.Schema)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid Datastream schema: "+err.Error())
			return
		}
		in.Schema = schema
	}

	existing, err := c.fetchEntity(r.Context(), pathID)
	if err != nil {
		c.writeBackendError(w, err)
		return
	}
	if !isDatastreamKind(existing.Triples) {
		writeJSONError(w, http.StatusNotFound, "no datastream: "+pathID)
		return
	}

	current := datastreamFromState(existing)
	mergedSystem := current.System
	if in.System != "" {
		mergedSystem = in.System
	}
	if mergedSystem == "" {
		writeJSONError(w, http.StatusBadRequest, "system required after PATCH")
		return
	}
	if err := validateEntityIDStrict(mergedSystem); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid system reference after PATCH: "+err.Error())
		return
	}

	mergedObservedProperty := current.ObservedProperty
	if in.ObservedProperty != "" {
		mergedObservedProperty = in.ObservedProperty
	}
	if mergedObservedProperty == "" {
		writeJSONError(w, http.StatusBadRequest, "observedProperty required after PATCH")
		return
	}

	identity := IdentityFrom(r.Context())
	var schemaRel *message.Triple
	if hasSchema {
		rel, err := c.createSchemaArtifact(r.Context(), pathID, PredDatastreamSchema, in.Schema, identity)
		if err != nil {
			c.writeBackendError(w, err)
			return
		}
		schemaRel = &rel
	}

	merged := mergePatchDatastreamTriples(pathID, existing.Triples, in, schemaRel)
	if err := c.replaceEntityTriples(r.Context(), existing, merged, identity); err != nil {
		c.writeBackendError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func mergePatchDatastreamTriples(entityID string, existing []message.Triple, patch Datastream, schemaRel *message.Triple) []message.Triple {
	hasName := patch.Name != ""
	hasDescription := patch.Description != ""
	hasSystem := patch.System != ""
	hasObservedProperty := patch.ObservedProperty != ""
	hasSchema := schemaRel != nil

	out := make([]message.Triple, 0, len(existing)+5)
	var sawLabel, sawDescription, sawSystem, sawObservedProperty, sawSchema bool
	for _, t := range existing {
		switch t.Predicate {
		case sensorml.PredLabel:
			sawLabel = true
			if hasName {
				out = append(out, message.Triple{Subject: entityID, Predicate: sensorml.PredLabel, Object: patch.Name})
				continue
			}
		case sensorml.PredDescription:
			sawDescription = true
			if hasDescription {
				out = append(out, message.Triple{Subject: entityID, Predicate: sensorml.PredDescription, Object: patch.Description})
				continue
			}
		case PredDatastreamSystem:
			sawSystem = true
			if hasSystem {
				out = append(out, message.Triple{Subject: entityID, Predicate: PredDatastreamSystem, Object: patch.System, Datatype: message.EntityReferenceDatatype})
				continue
			}
		case csapivocab.ObservedProperty:
			sawObservedProperty = true
			if hasObservedProperty {
				out = append(out, message.Triple{Subject: entityID, Predicate: csapivocab.ObservedProperty, Object: patch.ObservedProperty})
				continue
			}
		case PredDatastreamSchema:
			sawSchema = true
			if hasSchema {
				out = append(out, *schemaRel)
				continue
			}
		}
		out = append(out, t)
	}
	if hasName && !sawLabel {
		out = append(out, message.Triple{Subject: entityID, Predicate: sensorml.PredLabel, Object: patch.Name})
	}
	if hasDescription && !sawDescription {
		out = append(out, message.Triple{Subject: entityID, Predicate: sensorml.PredDescription, Object: patch.Description})
	}
	if hasSystem && !sawSystem {
		out = append(out, message.Triple{Subject: entityID, Predicate: PredDatastreamSystem, Object: patch.System, Datatype: message.EntityReferenceDatatype})
	}
	if hasObservedProperty && !sawObservedProperty {
		out = append(out, message.Triple{Subject: entityID, Predicate: csapivocab.ObservedProperty, Object: patch.ObservedProperty})
	}
	if hasSchema && !sawSchema {
		out = append(out, *schemaRel)
	}
	return out
}
