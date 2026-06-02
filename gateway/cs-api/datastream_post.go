package csapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// handleDatastreamPost serves POST /datastreams — CS API §10.6. Input is
// the CS API §10 Datastream JSON shape (subset documented in datastream.go
// package-level comment). Output is 201 Created with Location header.
//
// Flow:
//  1. Content-Type must be application/json (no SensorML wrapper for
//     datastreams — CS API uses raw JSON).
//  2. Decode body to a Datastream value.
//  3. Validate required fields (System reference + ObservedProperty IRI).
//  4. Mint a 6-part SemStreams entity ID from cfg.DatastreamIDPrefix +
//     either the client-supplied ID (sanitized) or a fresh UUID.
//  5. Convert to triples via datastreamToTriples.
//  6. Publish via the same ingestTriples path POST /systems uses.
//  7. Respond 201 Created with Location: /datastreams/{id}.
func (c *Component) handleDatastreamPost(w http.ResponseWriter, r *http.Request) {
	if err := requireMediaType(r.Header.Get("Content-Type"), string(MediaJSON)); err != nil {
		w.Header().Set("Accept-Post", string(MediaJSON))
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

	// Required fields per CS API §10.6: every datastream MUST link to a
	// producing system + an observable property. We reject early so the
	// graph doesn't accept an orphan datastream that no downstream
	// observation could attach to meaningfully. Name and description are
	// optional — a client may register a datastream by IDs alone and
	// label it later.
	if in.System == "" {
		writeJSONError(w, http.StatusBadRequest, "system required")
		return
	}
	if err := validateEntityIDStrict(in.System); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid system reference: "+err.Error())
		return
	}
	if in.ObservedProperty == "" {
		writeJSONError(w, http.StatusBadRequest, "observedProperty required")
		return
	}
	if schema, err := normalizeDatastreamSchema(in.Schema); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid Datastream schema: "+err.Error())
		return
	} else {
		in.Schema = schema
	}

	// Honor client-supplied id if shaped as a SemStreams 6-part ID;
	// otherwise mint from prefix + sanitized client id (or a fresh UUID
	// if id was empty). The honor-client-id path lets a caller that
	// already has its own ID space (typical for federated deployments)
	// supply an authoritative id; the mint path covers the new-resource
	// case where the server assigns.
	entityID := c.mintDatastreamEntityID(in.ID)

	triples := datastreamToTriples(entityID, &in)

	id := IdentityFrom(r.Context())
	if len(in.Schema) > 0 {
		rel, err := c.createSchemaArtifact(r.Context(), entityID, PredDatastreamSchema, in.Schema, id)
		if err != nil {
			w.Header().Set("X-CS-Attempted-ID", entityID)
			c.writeBackendError(w, err)
			return
		}
		triples = append(triples, rel)
	}
	if err := c.ingestTriples(r.Context(), triples, id); err != nil {
		w.Header().Set("X-CS-Attempted-ID", entityID)
		c.writeBackendError(w, err)
		return
	}

	w.Header().Set("Content-Type", string(MediaJSON))
	w.Header().Set("Location", "/datastreams/"+entityID)
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(struct {
		Status string `json:"status"`
		ID     string `json:"id"`
		Type   string `json:"type"`
	}{Status: "created", ID: entityID, Type: "Datastream"})
}

// mintDatastreamEntityID returns the entity ID to use for a datastream
// being created. Honors a client-supplied id when it already conforms
// to the SemStreams 6-part shape (the federation idiom: caller owns
// the ID space). Otherwise appends a sanitized last token to the
// configured 5-part prefix.
func (c *Component) mintDatastreamEntityID(clientID string) string {
	if clientID != "" && validateEntityIDStrict(clientID) == nil {
		return clientID
	}
	return c.cfg.DatastreamIDPrefix + "." + uniqueIDToToken(clientID)
}
