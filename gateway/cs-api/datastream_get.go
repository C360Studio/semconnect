package csapi

import (
	"encoding/json"
	"net/http"
)

// handleDatastreams serves GET /datastreams — CS API §10.5. Mirrors
// handleSystems: predicate-query for rdf:type = DatastreamTypeIRI,
// returns IDs in a DatastreamCollection. JSON-only at v0.1 (no SensorML
// wrapper for datastream collections, same as system collections).
func (c *Component) handleDatastreams(w http.ResponseWriter, r *http.Request) {
	if _, ok := NegotiateRequest(r, FamilyDatastreamCollection); !ok {
		WriteNotAcceptable(w, FamilyDatastreamCollection)
		return
	}

	limit, err := parseLimit(r.URL.Query().Get("limit"), c.cfg.DefaultListLimit, c.cfg.MaxListLimit)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	entities, err := c.listEntitiesByType(r.Context(), DatastreamTypeIRI, limit, "listDatastreamEntities")
	if err != nil {
		c.writeBackendError(w, err)
		return
	}

	coll := datastreamCollection{
		Type:           "DatastreamCollection",
		NumberMatched:  len(entities),
		NumberReturned: len(entities),
		Truncated:      len(entities) == limit, // see systems.go systemCollection.NumberMatched comment
		Datastreams:    make([]datastreamRef, 0, len(entities)),
		Links: []link{
			{Href: "/datastreams", Rel: "self", Type: string(MediaJSON)},
		},
	}
	for _, id := range entities {
		coll.Datastreams = append(coll.Datastreams, datastreamRef{
			ID:   id,
			Type: "Datastream",
			Links: []link{
				{Href: "/datastreams/" + id, Rel: "self", Type: string(MediaJSON)},
			},
		})
	}

	w.Header().Set("Content-Type", string(MediaJSON))
	w.Header().Set("X-CS-Datastream-Subset", "true") // see datastream.go file doc
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	if encErr := json.NewEncoder(w).Encode(coll); encErr != nil {
		c.errs.Add(1)
		c.logger.Error("encode datastreams response", "err", encErr)
	}
}

// handleDatastream serves GET /datastreams/{id} — CS API §10.4. Mirrors
// handleSystem's read-side, narrowed to JSON (no SensorML wrapper for
// datastreams). Returns 404 if the entity exists but is not a Datastream
// (i.e. URL points at a different resource kind).
func (c *Component) handleDatastream(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := validateEntityID(id); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid datastream id: "+err.Error())
		return
	}

	if _, ok := NegotiateRequest(r, FamilyDatastreamItem); !ok {
		WriteNotAcceptable(w, FamilyDatastreamItem)
		return
	}

	state, err := c.fetchEntity(r.Context(), id)
	if err != nil {
		c.writeBackendError(w, err)
		return
	}

	if !isDatastreamKind(state.Triples) {
		c.logger.Info("entity not a datastream kind", "id", id)
		writeJSONError(w, http.StatusNotFound, "no datastream: "+id)
		return
	}

	d := datastreamFromState(state)

	w.Header().Set("Content-Type", string(MediaJSON))
	w.Header().Set("X-CS-Datastream-Subset", "true")
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	if err := json.NewEncoder(w).Encode(d); err != nil {
		c.errs.Add(1)
		c.logger.Error("encode datastream response", "id", state.ID, "err", err)
	}
}
