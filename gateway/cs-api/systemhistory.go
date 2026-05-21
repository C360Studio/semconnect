// Stage 26 — System History read-side vendor extension. OGC 23-002
// Annex A does not define a /conf/system-history class in the pinned ETS,
// but OSH exposes this surface. v0.1 serves the current System description
// as the single available historical revision.
package csapi

import (
	"encoding/json"
	"net/http"

	"github.com/c360studio/semstreams/graph"
)

const currentSystemRevisionID = "current"

func (c *Component) handleSystemHistory(w http.ResponseWriter, r *http.Request) {
	if _, ok := NegotiateRequest(r, FamilySystemCollection); !ok {
		WriteNotAcceptable(w, FamilySystemCollection)
		return
	}
	id := r.PathValue("id")
	state, ok := c.fetchSystemForHistory(w, r, id)
	if !ok {
		return
	}

	coll := systemCollection{
		Type:           "SystemCollection",
		NumberMatched:  1,
		NumberReturned: 1,
		Items: []systemRef{{
			ID:   state.ID,
			Type: "System",
			Links: []link{
				{Href: "/systems/" + state.ID + "/history/" + currentSystemRevisionID, Rel: "self", Type: string(MediaJSON), Title: "current revision"},
				{Href: "/systems/" + state.ID, Rel: "canonical", Type: string(MediaJSON), Title: "current system resource"},
			},
		}},
		Links: []link{
			{Href: "/systems/" + state.ID + "/history", Rel: "self", Type: string(MediaJSON)},
		},
	}
	w.Header().Set("Content-Type", string(MediaJSON))
	w.Header().Set("X-CS-History-Current-Only", "true")
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	_ = json.NewEncoder(w).Encode(coll)
}

func (c *Component) handleSystemHistoryItem(w http.ResponseWriter, r *http.Request) {
	if _, ok := NegotiateRequest(r, FamilySystemItem); !ok {
		WriteNotAcceptable(w, FamilySystemItem)
		return
	}
	revID := r.PathValue("revID")
	if revID != currentSystemRevisionID {
		writeJSONError(w, http.StatusNotFound, "system history revision not found: "+revID)
		return
	}
	id := r.PathValue("id")
	state, ok := c.fetchSystemForHistory(w, r, id)
	if !ok {
		return
	}
	c.writeSystemJSON(w, r, state)
}

func (c *Component) fetchSystemForHistory(w http.ResponseWriter, r *http.Request, id string) (graph.EntityState, bool) {
	if err := validateEntityID(id); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid system id: "+err.Error())
		return graph.EntityState{}, false
	}
	state, err := c.fetchEntity(r.Context(), id)
	if err != nil {
		c.writeBackendError(w, err)
		return graph.EntityState{}, false
	}
	if !isSystemKind(state.Triples) {
		c.logger.Info("entity not a system kind", "id", id)
		writeJSONError(w, http.StatusNotFound, "no system: "+id)
		return graph.EntityState{}, false
	}
	return state, true
}

func (c *Component) handleSystemHistoryOptions(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Allow", "GET, HEAD, OPTIONS")
	w.WriteHeader(http.StatusNoContent)
}
