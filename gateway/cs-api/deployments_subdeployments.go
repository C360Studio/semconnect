package csapi

import (
	"encoding/json"
	"net/http"

	"github.com/c360studio/semstreams/graph"
)

// handleDeploymentSubdeployments serves GET /deployments/{id}/subdeployments.
// A subdeployment is an ordinary Deployment whose child-side
// cs-api.deployment.parent predicate points at the parent deployment.
func (c *Component) handleDeploymentSubdeployments(w http.ResponseWriter, r *http.Request) {
	media, ok := NegotiateRequest(r, FamilyDeploymentCollection)
	if !ok || media != MediaJSON {
		WriteNotAcceptable(w, FamilyDeploymentCollection)
		return
	}

	parentID := r.PathValue("id")
	if err := validateEntityID(parentID); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid deployment id: "+err.Error())
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
	if !isDeploymentKind(parent.Triples) {
		writeJSONError(w, http.StatusNotFound, "entity is not a Deployment")
		return
	}

	ids, err := c.listEntitiesByType(r.Context(), ssnDeployment, c.cfg.MaxListLimit, "listSubdeploymentEntities")
	if err != nil {
		c.writeBackendError(w, err)
		return
	}
	statesByID, err := c.fetchEntitiesBatch(r.Context(), ids)
	if err != nil {
		c.writeBackendError(w, err)
		return
	}

	items := make([]deploymentRef, 0)
	for _, id := range ids {
		state, ok := statesByID[id]
		if !ok || !isDeploymentKind(state.Triples) || !deploymentChildOf(state, parentID) {
			continue
		}
		items = append(items, deploymentRef{
			ID:   state.ID,
			Type: "Deployment",
			Links: []link{
				{Href: "/deployments/" + state.ID, Rel: "self", Type: string(MediaJSON)},
				{Href: "/deployments/" + state.ID, Rel: "canonical", Type: string(MediaJSON)},
			},
		})
		if len(items) == limit {
			break
		}
	}

	coll := deploymentCollection{
		Type:           "DeploymentCollection",
		NumberMatched:  len(items),
		NumberReturned: len(items),
		Truncated:      len(items) == limit,
		Items:          items,
		Links: []link{
			{Href: "/deployments/" + parentID + "/subdeployments", Rel: "self", Type: string(MediaJSON)},
		},
	}
	w.Header().Set("Content-Type", string(MediaJSON))
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	_ = json.NewEncoder(w).Encode(coll)
}

func deploymentChildOf(state graph.EntityState, parentID string) bool {
	parent, ok := firstStringObject(state.Triples, predDeploymentParent)
	return ok && parent == parentID
}
