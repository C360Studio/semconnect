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
// `http://www.w3.org/ns/ssn/Deployment`, exported by semstreams as
// `sosa.SSNDeployment` since v1.0.0-beta.79.
package csapi

import (
	"bytes"
	"encoding/json"
	"net/http"

	"github.com/c360studio/semconnect/parser/sensorml"
	"github.com/c360studio/semconnect/vocabulary/sosa"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
)

// ssnDeployment is the SSN class IRI for Deployment entities.
const ssnDeployment = sosa.SSNDeployment

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
// but with `Type: "Deployment"`.
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
			{Href: "/deployments/" + state.ID, Rel: "alternate", Type: string(MediaSensorML)},
			{Href: "/samplingFeatures", Rel: "samplingFeatures", Type: string(MediaJSON)},
			{Href: "/datastreams", Rel: "datastreams", Type: string(MediaJSON)},
		},
	}
	if v, ok := firstStringObject(state.Triples, sensorml.PredLabel); ok {
		d.Label = v
	}
	if v, ok := firstStringObject(state.Triples, sensorml.PredDescription); ok {
		d.Description = v
	}
	if v, ok := firstSystemUIDObject(state.Triples); ok {
		d.UID = v
		d.UniqueID = v
		d.FeatureProperties = &featureProperties{
			UID:         v,
			Name:        d.Label,
			Description: d.Description,
		}
	}
	if hrefs := allStringObjects(state.Triples, predDeploymentDeployedSystems); len(hrefs) > 0 {
		if d.FeatureProperties == nil {
			d.FeatureProperties = &featureProperties{}
		}
		d.FeatureProperties.DeployedSystemsLinks = linksFromHrefs(hrefs, "deployedSystem")
	}
	if v, ok := firstSystemPositionObject(state.Triples); ok {
		d.Geometry = json.RawMessage(v)
	}
	return d
}

// isDeploymentKind reports whether the entity's rdf:type is the
// SSN Deployment class.
func isDeploymentKind(triples []message.Triple) bool {
	typeIRI, ok := firstStringObject(triples, sensorml.PredType)
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
				{Href: "/deployments/" + id, Rel: "alternate", Type: string(MediaSensorML)},
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

// writeDeploymentsGeoJSON emits FeatureCollection with geometry recovered
// from batch-hydrated entity states.
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
	statesByID, err := c.fetchEntitiesBatch(r.Context(), ids)
	if err != nil {
		c.writeBackendError(w, err)
		return
	}
	for _, id := range ids {
		geom := nullGeom
		props := map[string]any{"featureType": "Deployment"}
		if state, ok := statesByID[id]; ok {
			props = geoJSONFeaturePropertiesFromState("Deployment", state)
			if hrefs := allStringObjects(state.Triples, predDeploymentDeployedSystems); len(hrefs) > 0 {
				props["deployedSystems@link"] = linksFromHrefs(hrefs, "deployedSystem")
			}
			if v, ok := firstSystemPositionObject(state.Triples); ok && v != "" {
				geom = json.RawMessage(v)
			}
		} else {
			c.logger.Warn("batch entity fetch for FeatureCollection missed entity; degrading to null geometry",
				"entity", id)
		}
		fc.Features = append(fc.Features, deploymentFeature{
			Type:       "Feature",
			ID:         id,
			Geometry:   geom,
			Properties: props,
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
	media, ok := NegotiateRequest(r, FamilyDeploymentItem)
	if !ok {
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

	switch media {
	case MediaJSON:
		c.writeDeploymentJSON(w, r, state)
	case MediaSensorML, MediaSensorMLLegacy:
		c.writeDeploymentSensorML(w, r, state, media)
	default:
		WriteNotAcceptable(w, FamilyDeploymentItem)
	}
}

func (c *Component) writeDeploymentJSON(w http.ResponseWriter, r *http.Request, state graph.EntityState) {
	w.Header().Set("Content-Type", string(MediaJSON))
	w.Header().Set("X-CS-Reconstructed-Lossy", "true")
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	_ = json.NewEncoder(w).Encode(deploymentFromState(state))
}

type deploymentSensorML struct {
	ID              string          `json:"id"`
	Type            string          `json:"type"`
	Label           string          `json:"label,omitempty"`
	Description     string          `json:"description,omitempty"`
	UID             string          `json:"uid,omitempty"`
	UniqueID        string          `json:"uniqueId,omitempty"`
	Position        json.RawMessage `json:"position,omitempty"`
	DeployedSystems []link          `json:"deployedSystems,omitempty"`
	Links           []link          `json:"links,omitempty"`
}

func deploymentSensorMLFromState(state graph.EntityState) deploymentSensorML {
	d := deploymentFromState(state)
	out := deploymentSensorML{
		ID:          d.ID,
		Type:        "Deployment",
		Label:       d.Label,
		Description: d.Description,
		UID:         d.UID,
		UniqueID:    d.UniqueID,
		Position:    d.Geometry,
		Links:       d.Links,
	}
	if d.FeatureProperties != nil && len(d.FeatureProperties.DeployedSystemsLinks) > 0 {
		out.DeployedSystems = d.FeatureProperties.DeployedSystemsLinks
	}
	return out
}

func (c *Component) writeDeploymentSensorML(w http.ResponseWriter, r *http.Request, state graph.EntityState, media MediaType) {
	body, err := json.Marshal(deploymentSensorMLFromState(state))
	if err != nil {
		c.writeBackendError(w, err)
		return
	}
	w.Header().Set("Content-Type", string(media))
	w.Header().Set("X-CS-Reconstructed-Lossy", "true")
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	if _, err := w.Write(body); err != nil {
		c.errs.Add(1)
		c.logger.Error("write deployment SensorML response", "id", state.ID, "err", err)
	}
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
