// Stage 21 — CS API §8 Deployment resource. A Deployment links a
// System to a deployment context (location + valid time interval).
// Unlike /procedures, deployments DO carry geometry — a deployment
// is a physical placement of equipment at a location.
//
// Endpoints at Stage 21: GET collection/item, POST, OPTIONS.
// PUT/DELETE/PATCH absent (ETS CRD/Update groups only target
// /systems — same Stage-20 reasoning).
//
// **rdf:type IRI**: SSN's `Deployment` class IRI is
// `http://www.w3.org/ns/ssn/Deployment`. The framework's
// `vocabulary/sosa` exports `SSNNamespace` + `SSNHasDeployment` but
// not the class constant itself. We define `ssnDeployment` locally
// from the same namespace prefix; flip to `sosa.SSNDeployment`
// (upstream-ask drafted in stage 21 description) once the framework
// adds the constant.
package csapi

import (
	"bytes"
	"encoding/json"
	"net/http"

	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/parser/sensorml"
	"github.com/c360studio/semstreams/vocabulary/sosa"
)

// ssnDeployment is the SSN class IRI for Deployment entities. Local
// const because `sosa.SSNDeployment` doesn't exist upstream as of
// framework v1.0.0-beta.75; same convention as the local
// `PredSystemPosition` / `PredSystemUID` workarounds.
const ssnDeployment = sosa.SSNNamespace + "Deployment"

// deploymentCollection mirrors systemCollection / procedureCollection.
type deploymentCollection struct {
	Type           string          `json:"type"` // "DeploymentCollection"
	NumberMatched  int             `json:"numberMatched"`
	NumberReturned int             `json:"numberReturned"`
	Truncated      bool            `json:"truncated,omitempty"`
	Items          []deploymentRef `json:"items"`
	Links          []link          `json:"links"`
}

type deploymentRef struct {
	ID    string `json:"id"`
	Type  string `json:"type"` // "Deployment"
	Links []link `json:"links,omitempty"`
}

// deployment is the JSON shape returned by GET /deployments/{id}.
// Mirrors `system` (Stage 18 uid preservation + Stage 14 geometry)
// but with `Type: "Deployment"` and no SensorML reverse-mapping
// (no spec encoding pairs SensorML with Deployment).
type deployment struct {
	ID                string             `json:"id"`
	Type              string             `json:"type"` // "Deployment"
	Label             string             `json:"label,omitempty"`
	Description       string             `json:"description,omitempty"`
	UID               string             `json:"uid,omitempty"`
	UniqueID          string             `json:"uniqueId,omitempty"`
	FeatureProperties *featureProperties `json:"properties,omitempty"`
	Geometry          json.RawMessage    `json:"geometry,omitempty"`
	Links             []link             `json:"links"`
}

// deploymentFromState collapses an EntityState into the JSON shape.
func deploymentFromState(state graph.EntityState) deployment {
	d := deployment{
		ID:   state.ID,
		Type: "Deployment",
		Links: []link{
			{Href: "/deployments/" + state.ID, Rel: "self", Type: string(MediaJSON)},
			{Href: "/deployments/" + state.ID, Rel: "canonical", Type: string(MediaJSON)},
		},
	}
	if v, ok := firstStringObject(state.Triples, sensorml.PredLabel); ok {
		d.Label = v
	}
	if v, ok := firstStringObject(state.Triples, sensorml.PredDescription); ok {
		d.Description = v
	}
	if v, ok := firstStringObject(state.Triples, PredSystemUID); ok {
		d.UID = v
		d.UniqueID = v
		d.FeatureProperties = &featureProperties{
			UID:         v,
			Name:        d.Label,
			Description: d.Description,
		}
	}
	if v, ok := firstStringObject(state.Triples, PredSystemPosition); ok {
		d.Geometry = json.RawMessage(v)
	}
	return d
}

// isDeploymentKind reports whether the entity's rdf:type is the
// SSN Deployment class.
func isDeploymentKind(triples []message.Triple) bool {
	typeIRI, ok := firstStringObject(triples, typeAliases...)
	if !ok {
		return false
	}
	return typeIRI == ssnDeployment
}

// handleDeployments serves GET /deployments. Supports JSON +
// geo+json content negotiation.
func (c *Component) handleDeployments(w http.ResponseWriter, r *http.Request) {
	media, ok := NegotiateRequest(r, FamilyDeploymentCollection)
	if !ok {
		WriteNotAcceptable(w, FamilyDeploymentCollection)
		return
	}

	limit, err := parseLimit(r.URL.Query().Get("limit"), c.cfg.DefaultListLimit, c.cfg.MaxListLimit)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	ids, err := c.listEntitiesByType(r.Context(), ssnDeployment, limit, "listDeploymentEntities")
	if err != nil {
		c.writeBackendError(w, err)
		return
	}

	if media == MediaGeoJSON {
		c.writeDeploymentsGeoJSON(w, r, ids, limit)
		return
	}

	coll := deploymentCollection{
		Type:           "DeploymentCollection",
		NumberMatched:  len(ids),
		NumberReturned: len(ids),
		Items:          make([]deploymentRef, 0, len(ids)),
		Links: []link{
			{Href: "/deployments", Rel: "self", Type: string(MediaJSON)},
		},
	}
	for _, id := range ids {
		coll.Items = append(coll.Items, deploymentRef{
			ID:   id,
			Type: "Deployment",
			Links: []link{
				{Href: "/deployments/" + id, Rel: "self", Type: string(MediaJSON)},
				{Href: "/deployments/" + id, Rel: "canonical", Type: string(MediaJSON)},
			},
		})
	}
	if len(ids) >= limit {
		coll.Truncated = true
	}

	w.Header().Set("Content-Type", string(MediaJSON))
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	_ = json.NewEncoder(w).Encode(coll)
}

// writeDeploymentsGeoJSON emits FeatureCollection with per-entity
// geometry recovered from the cs-api.system.position triple (same
// Stage-14 sister-side predicate /systems uses — see the upstream
// uid-preservation ask thread for the eventual cleanup). N+1
// entity-query, same as /systems Stage 15.
func (c *Component) writeDeploymentsGeoJSON(w http.ResponseWriter, r *http.Request, ids []string, limit int) {
	type deploymentFeature struct {
		Type       string          `json:"type"`
		ID         string          `json:"id"`
		Geometry   json.RawMessage `json:"geometry"`
		Properties map[string]any  `json:"properties"`
		Links      []link          `json:"links,omitempty"`
	}
	type deploymentFeatureCollection struct {
		Type           string              `json:"type"`
		NumberMatched  int                 `json:"numberMatched"`
		NumberReturned int                 `json:"numberReturned"`
		Truncated      bool                `json:"truncated,omitempty"`
		Features       []deploymentFeature `json:"features"`
		Links          []link              `json:"links"`
	}

	nullGeom := json.RawMessage("null")
	fc := deploymentFeatureCollection{
		Type:           "FeatureCollection",
		NumberMatched:  len(ids),
		NumberReturned: len(ids),
		Features:       make([]deploymentFeature, 0, len(ids)),
		Links: []link{
			{Href: "/deployments", Rel: "self", Type: string(MediaGeoJSON)},
		},
	}
	for _, id := range ids {
		geom := nullGeom
		state, ferr := c.fetchEntity(r.Context(), id)
		if ferr == nil {
			if v, ok := firstStringObject(state.Triples, PredSystemPosition); ok && v != "" {
				geom = json.RawMessage(v)
			}
		} else {
			// Per-entity backend failure → degrade to null geometry.
			// One bad row shouldn't poison the page; mirrors the
			// Stage 15 /systems geo+json pattern.
			c.logger.Warn("fetch entity for FeatureCollection failed; degrading to null geometry",
				"entity", id, "err", ferr.Error())
		}
		fc.Features = append(fc.Features, deploymentFeature{
			Type:       "Feature",
			ID:         id,
			Geometry:   geom,
			Properties: map[string]any{"featureType": "Deployment"},
			Links: []link{
				{Href: "/deployments/" + id, Rel: "self", Type: string(MediaJSON)},
				{Href: "/deployments/" + id, Rel: "canonical", Type: string(MediaJSON)},
			},
		})
	}
	if len(ids) >= limit {
		fc.Truncated = true
	}

	w.Header().Set("Content-Type", string(MediaGeoJSON))
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	_ = json.NewEncoder(w).Encode(fc)
}

// handleDeployment serves GET /deployments/{id}.
func (c *Component) handleDeployment(w http.ResponseWriter, r *http.Request) {
	if _, ok := NegotiateRequest(r, FamilyDeploymentItem); !ok {
		WriteNotAcceptable(w, FamilyDeploymentItem)
		return
	}

	id := r.PathValue("id")
	if err := validateEntityID(id); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid deployment id: "+err.Error())
		return
	}

	state, err := c.fetchEntity(r.Context(), id)
	if err != nil {
		c.writeBackendError(w, err)
		return
	}

	if !isDeploymentKind(state.Triples) {
		c.logger.Info("entity not a deployment kind", "id", id)
		writeJSONError(w, http.StatusNotFound, "entity is not a Deployment")
		return
	}

	w.Header().Set("Content-Type", string(MediaJSON))
	w.Header().Set("X-CS-Reconstructed-Lossy", "true")
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	_ = json.NewEncoder(w).Encode(deploymentFromState(state))
}

// handleDeploymentsOptions / handleDeploymentOptions
func (c *Component) handleDeploymentsOptions(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Allow", "GET, HEAD, POST, OPTIONS")
	w.WriteHeader(http.StatusNoContent)
}

func (c *Component) handleDeploymentOptions(w http.ResponseWriter, r *http.Request) {
	pathID := r.PathValue("id")
	if err := validateEntityID(pathID); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid deployment id: "+err.Error())
		return
	}
	w.Header().Set("Allow", "GET, HEAD, OPTIONS")
	w.WriteHeader(http.StatusNoContent)
}

// avoid unused-import errors when the geo+json branch is the only
// caller of bytes / json package helpers.
var _ = bytes.Equal
