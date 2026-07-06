package csapi

import (
	"encoding/json"
	"net/http"

	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/parser/sensorml"
)

// handleSystemSubsystems serves GET /systems/{id}/subsystems. The relation
// is modeled from the child side with sensorml.PredIsHostedBy; the endpoint is
// a CS API scoped collection view over ordinary System entities.
func (c *Component) handleSystemSubsystems(w http.ResponseWriter, r *http.Request) {
	media, ok := NegotiateRequest(r, FamilySystemCollection)
	if !ok || media != MediaJSON {
		WriteNotAcceptable(w, FamilySystemCollection)
		return
	}

	parentID := r.PathValue("id")
	if err := validateEntityID(parentID); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid system id: "+err.Error())
		return
	}
	limit, err := parseLimit(r.URL.Query().Get("limit"), c.cfg.DefaultListLimit, c.cfg.MaxListLimit)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	parent, err := c.fetchEntity(r.Context(), parentID)
	if err != nil {
		c.writeBackendError(w, err)
		return
	}
	if !isSystemKind(parent.Triples) {
		writeJSONError(w, http.StatusNotFound, "no system: "+parentID)
		return
	}

	ids, err := c.listSystemEntities(r.Context(), c.cfg.MaxListLimit)
	if err != nil {
		c.writeBackendError(w, err)
		return
	}
	statesByID, err := c.fetchEntitiesBatch(r.Context(), ids)
	if err != nil {
		c.writeBackendError(w, err)
		return
	}

	items := make([]systemRef, 0)
	for _, id := range ids {
		state, ok := statesByID[id]
		if !ok || !isSystemKind(state.Triples) || !systemHostedBy(state, parentID) {
			continue
		}
		sys := systemFromState(state)
		items = append(items, systemRef{
			ID:          state.ID,
			Type:        "System",
			Name:        sys.Label,
			Description: sys.Description,
			Links: []link{
				{Href: "/systems/" + parentID + "/subsystems/" + state.ID, Rel: "self", Type: string(MediaJSON)},
				{Href: "/systems/" + state.ID, Rel: "canonical", Type: string(MediaJSON)},
			},
		})
		if len(items) == limit {
			break
		}
	}

	coll := systemCollection{
		Type:           "SystemCollection",
		NumberMatched:  len(items),
		NumberReturned: len(items),
		Truncated:      len(items) == limit,
		Items:          items,
		Links: []link{
			{Href: "/systems/" + parentID + "/subsystems", Rel: "self", Type: string(MediaJSON)},
		},
	}
	w.Header().Set("Content-Type", string(MediaJSON))
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	_ = json.NewEncoder(w).Encode(coll)
}

// handleSystemSubsystem serves GET /systems/{id}/subsystems/{subsystemID}.
// Subsystems are Systems, so all item representations use the same encoders
// as /systems/{id} after the parent relation check.
func (c *Component) handleSystemSubsystem(w http.ResponseWriter, r *http.Request) {
	parentID := r.PathValue("id")
	if err := validateEntityID(parentID); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid system id: "+err.Error())
		return
	}
	subsystemID := r.PathValue("subsystemID")
	if err := validateEntityID(subsystemID); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid subsystem id: "+err.Error())
		return
	}

	media, ok := NegotiateRequest(r, FamilySystemItem)
	if !ok {
		WriteNotAcceptable(w, FamilySystemItem)
		return
	}

	parent, err := c.fetchEntity(r.Context(), parentID)
	if err != nil {
		c.writeBackendError(w, err)
		return
	}
	if !isSystemKind(parent.Triples) {
		writeJSONError(w, http.StatusNotFound, "no system: "+parentID)
		return
	}

	state, err := c.fetchEntity(r.Context(), subsystemID)
	if err != nil {
		c.writeBackendError(w, err)
		return
	}
	if !isSystemKind(state.Triples) || !systemHostedBy(state, parentID) {
		writeJSONError(w, http.StatusNotFound, "no subsystem: "+subsystemID)
		return
	}

	switch media {
	case MediaJSON:
		c.writeSystemJSONWithLinks(w, r, state, []link{{
			Href:  "/systems/" + parentID,
			Rel:   "parent",
			Type:  string(MediaJSON),
			Title: "Parent system",
		}})
	case MediaSensorML, MediaSensorMLLegacy:
		c.writeSystemSensorML(w, r, state, media)
	case MediaJSONLD:
		c.writeSystemJSONLD(w, r, state)
	default:
		WriteNotAcceptable(w, FamilySystemItem)
	}
}

func systemHostedBy(state graph.EntityState, parentID string) bool {
	hostedBy, ok := firstStringObject(state.Triples, sensorml.PredIsHostedBy)
	return ok && hostedBy == parentID
}
