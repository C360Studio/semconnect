// Stage 22 — CS API Sampling Features resource. A sampling feature is
// a SOSA Sample / Feature-of-Interest proxy used by observations.
// Unlike procedures, sampling features carry first-class geometry.
//
// Endpoints at Stage 22: GET collection/item, POST, OPTIONS. PUT /
// DELETE / PATCH stay absent at v0.1 for the same OSH-bar scoping
// used by procedures and deployments.
package csapi

import (
	"encoding/json"
	"net/http"

	"github.com/c360studio/semconnect/parser/sensorml"
	"github.com/c360studio/semconnect/vocabulary/sosa"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
)

type samplingFeatureCollection struct {
	Type           string               `json:"type"` // "SamplingFeatureCollection"
	NumberMatched  int                  `json:"numberMatched"`
	NumberReturned int                  `json:"numberReturned"`
	Truncated      bool                 `json:"truncated,omitempty"`
	Items          []samplingFeatureRef `json:"items"`
	Links          []link               `json:"links"`
}

type samplingFeatureRef struct {
	ID    string `json:"id"`
	Type  string `json:"type"` // "SamplingFeature"
	Links []link `json:"links,omitempty"`
}

type samplingFeature struct {
	ID                string             `json:"id"`
	Type              string             `json:"type"` // "SamplingFeature"
	Label             string             `json:"label,omitempty"`
	Description       string             `json:"description,omitempty"`
	UID               string             `json:"uid,omitempty"`
	UniqueID          string             `json:"uniqueId,omitempty"`
	FeatureProperties *featureProperties `json:"properties,omitempty"`
	Geometry          json.RawMessage    `json:"geometry,omitempty"`
	Links             []link             `json:"links"`
}

func samplingFeatureFromState(state graph.EntityState) samplingFeature {
	sf := samplingFeature{
		ID:   state.ID,
		Type: "SamplingFeature",
		Links: []link{
			{Href: "/samplingFeatures/" + state.ID, Rel: "self", Type: string(MediaJSON)},
			{Href: "/samplingFeatures/" + state.ID, Rel: "canonical", Type: string(MediaJSON)},
			{Href: "/datastreams", Rel: "datastreams", Type: string(MediaJSON)},
			{Href: "/controlstreams", Rel: "controlstreams", Type: string(MediaJSON)},
		},
	}
	if v, ok := firstStringObject(state.Triples, sensorml.PredLabel); ok {
		sf.Label = v
	}
	if v, ok := firstStringObject(state.Triples, sensorml.PredDescription); ok {
		sf.Description = v
	}
	if v, ok := firstSystemUIDObject(state.Triples); ok {
		sf.UID = v
		sf.UniqueID = v
		sf.FeatureProperties = &featureProperties{
			UID:         v,
			Name:        sf.Label,
			Description: sf.Description,
		}
	}
	if href, ok := firstStringObject(state.Triples, predSamplingFeatureHostedProcedure); ok {
		if sf.FeatureProperties == nil {
			sf.FeatureProperties = &featureProperties{}
		}
		sf.FeatureProperties.HostedProcedureLink = firstLinkFromHref(href, "hostedProcedure")
	}
	if v, ok := firstSystemPositionObject(state.Triples); ok {
		sf.Geometry = json.RawMessage(v)
	}
	return sf
}

func geoJSONFeaturePropertiesFromState(featureType string, state graph.EntityState) map[string]any {
	props := map[string]any{"featureType": featureType}
	if v, ok := firstSystemUIDObject(state.Triples); ok {
		props["uid"] = v
	}
	if v, ok := firstStringObject(state.Triples, sensorml.PredLabel); ok {
		props["name"] = v
	}
	if v, ok := firstStringObject(state.Triples, sensorml.PredDescription); ok {
		props["description"] = v
	}
	return props
}

func isSamplingFeatureKind(triples []message.Triple) bool {
	typeIRI, ok := firstStringObject(triples, sensorml.PredType)
	if !ok {
		return false
	}
	return typeIRI == sosa.Sample || typeIRI == sosa.FeatureOfInterest
}

func (c *Component) handleSamplingFeatures(w http.ResponseWriter, r *http.Request) {
	media, ok := NegotiateRequest(r, FamilySamplingFeatureCollection)
	if !ok {
		WriteNotAcceptable(w, FamilySamplingFeatureCollection)
		return
	}

	limit, err := parseLimit(r.URL.Query().Get("limit"), c.cfg.DefaultListLimit, c.cfg.MaxListLimit)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	ids, err := c.listEntitiesByType(r.Context(), sosa.Sample, limit, "listSamplingFeatureEntities")
	if err != nil {
		c.writeBackendError(w, err)
		return
	}

	if media == MediaGeoJSON {
		c.writeSamplingFeaturesGeoJSON(w, r, ids, limit)
		return
	}

	coll := samplingFeatureCollection{
		Type:           "SamplingFeatureCollection",
		NumberMatched:  len(ids),
		NumberReturned: len(ids),
		Items:          make([]samplingFeatureRef, 0, len(ids)),
		Links: []link{
			{Href: "/samplingFeatures", Rel: "self", Type: string(MediaJSON)},
		},
	}
	for _, id := range ids {
		coll.Items = append(coll.Items, samplingFeatureRef{
			ID:   id,
			Type: "SamplingFeature",
			Links: []link{
				{Href: "/samplingFeatures/" + id, Rel: "self", Type: string(MediaJSON)},
				{Href: "/samplingFeatures/" + id, Rel: "canonical", Type: string(MediaJSON)},
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

func (c *Component) writeSamplingFeaturesGeoJSON(w http.ResponseWriter, r *http.Request, ids []string, limit int) {
	type samplingFeatureFeature struct {
		Type       string          `json:"type"`
		ID         string          `json:"id"`
		Geometry   json.RawMessage `json:"geometry"`
		Properties map[string]any  `json:"properties"`
		Links      []link          `json:"links,omitempty"`
	}
	type samplingFeatureFeatureCollection struct {
		Type           string                   `json:"type"`
		NumberMatched  int                      `json:"numberMatched"`
		NumberReturned int                      `json:"numberReturned"`
		Truncated      bool                     `json:"truncated,omitempty"`
		Features       []samplingFeatureFeature `json:"features"`
		Links          []link                   `json:"links"`
	}

	nullGeom := json.RawMessage("null")
	fc := samplingFeatureFeatureCollection{
		Type:           "FeatureCollection",
		NumberMatched:  len(ids),
		NumberReturned: len(ids),
		Features:       make([]samplingFeatureFeature, 0, len(ids)),
		Links: []link{
			{Href: "/samplingFeatures", Rel: "self", Type: string(MediaGeoJSON)},
		},
	}
	statesByID, err := c.fetchEntitiesBatch(r.Context(), ids)
	if err != nil {
		c.writeBackendError(w, err)
		return
	}
	for _, id := range ids {
		geom := nullGeom
		props := map[string]any{"featureType": "SamplingFeature"}
		if state, ok := statesByID[id]; ok {
			props = geoJSONFeaturePropertiesFromState("SamplingFeature", state)
			if href, ok := firstStringObject(state.Triples, predSamplingFeatureHostedProcedure); ok {
				props["hostedProcedure@link"] = firstLinkFromHref(href, "hostedProcedure")
			}
			if v, ok := firstSystemPositionObject(state.Triples); ok && v != "" {
				geom = json.RawMessage(v)
			}
		} else {
			c.logger.Warn("batch entity fetch for sampling FeatureCollection missed entity; degrading to null geometry",
				"entity", id)
		}
		fc.Features = append(fc.Features, samplingFeatureFeature{
			Type:       "Feature",
			ID:         id,
			Geometry:   geom,
			Properties: props,
			Links: []link{
				{Href: "/samplingFeatures/" + id, Rel: "self", Type: string(MediaJSON)},
				{Href: "/samplingFeatures/" + id, Rel: "canonical", Type: string(MediaJSON)},
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

func (c *Component) handleSamplingFeature(w http.ResponseWriter, r *http.Request) {
	if _, ok := NegotiateRequest(r, FamilySamplingFeatureItem); !ok {
		WriteNotAcceptable(w, FamilySamplingFeatureItem)
		return
	}

	id := r.PathValue("id")
	if err := validateEntityID(id); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid sampling feature id: "+err.Error())
		return
	}

	state, err := c.fetchEntity(r.Context(), id)
	if err != nil {
		c.writeBackendError(w, err)
		return
	}
	if !isSamplingFeatureKind(state.Triples) {
		c.logger.Info("entity not a sampling feature kind", "id", id)
		writeJSONError(w, http.StatusNotFound, "entity is not a SamplingFeature")
		return
	}

	w.Header().Set("Content-Type", string(MediaJSON))
	w.Header().Set("X-CS-Reconstructed-Lossy", "true")
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	_ = json.NewEncoder(w).Encode(samplingFeatureFromState(state))
}

func (c *Component) handleSamplingFeaturesOptions(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Allow", "GET, HEAD, POST, OPTIONS")
	w.WriteHeader(http.StatusNoContent)
}

func (c *Component) handleSamplingFeatureOptions(w http.ResponseWriter, r *http.Request) {
	pathID := r.PathValue("id")
	if err := validateEntityID(pathID); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid sampling feature id: "+err.Error())
		return
	}
	w.Header().Set("Allow", "GET, HEAD, OPTIONS")
	w.WriteHeader(http.StatusNoContent)
}
