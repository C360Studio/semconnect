// Stage 23 — CS API Property resource. Properties are SOSA
// ObservableProperty definitions used by Datastreams and Observations.
//
// Endpoints at Stage 23: GET collection/item, POST, OPTIONS.
// PUT/DELETE/PATCH stay absent at v0.1 for the same OSH-bar scoping
// used by procedures, deployments, and sampling features.
package csapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/parser/sensorml"
	"github.com/c360studio/semstreams/vocabulary/sosa"
)

const (
	predPropertyDefinition   = "cs-api.property.definition"
	predPropertyBaseProperty = "cs-api.property.baseProperty"
)

type propertyCollection struct {
	Type           string        `json:"type"` // "PropertyCollection"
	NumberMatched  int           `json:"numberMatched"`
	NumberReturned int           `json:"numberReturned"`
	Truncated      bool          `json:"truncated,omitempty"`
	Items          []propertyRef `json:"items"`
	Links          []link        `json:"links"`
}

type propertyRef struct {
	ID    string `json:"id"`
	Type  string `json:"type"` // "Property"
	Links []link `json:"links,omitempty"`
}

type propertyResource struct {
	ID           string `json:"id"`
	Type         string `json:"type"` // "Property"
	Label        string `json:"label,omitempty"`
	Description  string `json:"description,omitempty"`
	Definition   string `json:"definition,omitempty"`
	BaseProperty string `json:"baseProperty,omitempty"`
	UID          string `json:"uid,omitempty"`
	UniqueID     string `json:"uniqueId,omitempty"`
	Links        []link `json:"links"`
}

type propertyPostBody struct {
	UniqueID     string `json:"uniqueId"`
	UID          string `json:"uid,omitempty"`
	Label        string `json:"label,omitempty"`
	Name         string `json:"name,omitempty"`
	Description  string `json:"description,omitempty"`
	Definition   string `json:"definition,omitempty"`
	BaseProperty string `json:"baseProperty,omitempty"`
}

func propertyFromState(state graph.EntityState) propertyResource {
	prop := propertyResource{
		ID:   state.ID,
		Type: "Property",
		Links: []link{
			{Href: "/properties/" + state.ID, Rel: "self", Type: string(MediaJSON)},
			{Href: "/properties/" + state.ID, Rel: "canonical", Type: string(MediaJSON)},
		},
	}
	if v, ok := firstStringObject(state.Triples, sensorml.PredLabel); ok {
		prop.Label = v
	}
	if v, ok := firstStringObject(state.Triples, sensorml.PredDescription); ok {
		prop.Description = v
	}
	if v, ok := firstSystemUIDObject(state.Triples); ok {
		prop.UID = v
		prop.UniqueID = v
	}
	if v, ok := firstStringObject(state.Triples, predPropertyDefinition); ok {
		prop.Definition = v
	}
	if v, ok := firstStringObject(state.Triples, predPropertyBaseProperty); ok {
		prop.BaseProperty = v
	}
	return prop
}

func isPropertyKind(triples []message.Triple) bool {
	typeIRI, ok := firstStringObject(triples, typeAliases...)
	if !ok {
		return false
	}
	return typeIRI == sosa.ObservableProperty
}

func (c *Component) handleProperties(w http.ResponseWriter, r *http.Request) {
	if _, ok := NegotiateRequest(r, FamilyPropertyCollection); !ok {
		WriteNotAcceptable(w, FamilyPropertyCollection)
		return
	}

	limit, err := parseLimit(r.URL.Query().Get("limit"), c.cfg.DefaultListLimit, c.cfg.MaxListLimit)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	ids, err := c.listEntitiesByType(r.Context(), sosa.ObservableProperty, limit, "listPropertyEntities")
	if err != nil {
		c.writeBackendError(w, err)
		return
	}

	coll := propertyCollection{
		Type:           "PropertyCollection",
		NumberMatched:  len(ids),
		NumberReturned: len(ids),
		Items:          make([]propertyRef, 0, len(ids)),
		Links: []link{
			{Href: "/properties", Rel: "self", Type: string(MediaJSON)},
		},
	}
	for _, id := range ids {
		coll.Items = append(coll.Items, propertyRef{
			ID:   id,
			Type: "Property",
			Links: []link{
				{Href: "/properties/" + id, Rel: "self", Type: string(MediaJSON)},
				{Href: "/properties/" + id, Rel: "canonical", Type: string(MediaJSON)},
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

func (c *Component) handleProperty(w http.ResponseWriter, r *http.Request) {
	if _, ok := NegotiateRequest(r, FamilyPropertyItem); !ok {
		WriteNotAcceptable(w, FamilyPropertyItem)
		return
	}

	id := r.PathValue("id")
	if err := validateEntityID(id); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid property id: "+err.Error())
		return
	}

	state, err := c.fetchEntity(r.Context(), id)
	if err != nil {
		c.writeBackendError(w, err)
		return
	}
	if !isPropertyKind(state.Triples) {
		c.logger.Info("entity not a property kind", "id", id)
		writeJSONError(w, http.StatusNotFound, "entity is not a Property")
		return
	}

	w.Header().Set("Content-Type", string(MediaJSON))
	w.Header().Set("X-CS-Reconstructed-Lossy", "true")
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	_ = json.NewEncoder(w).Encode(propertyFromState(state))
}

func (c *Component) handlePropertyPost(w http.ResponseWriter, r *http.Request) {
	if err := requireMediaTypeAny(r.Header.Get("Content-Type"),
		string(MediaSensorML), string(MediaSensorMLLegacy), string(MediaJSON)); err != nil {
		w.Header().Set("Accept-Post", strings.Join([]string{
			string(MediaSensorML), string(MediaSensorMLLegacy), string(MediaJSON),
		}, ", "))
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

	entityID, triples, buildErr := c.buildPropertyTriples(body)
	if buildErr != nil {
		writeJSONError(w, http.StatusBadRequest, buildErr.Error())
		return
	}

	id := IdentityFrom(r.Context())
	if err := c.ingestTriples(r.Context(), triples, id); err != nil {
		w.Header().Set("X-CS-Attempted-ID", entityID)
		c.writeBackendError(w, err)
		return
	}

	w.Header().Set("Content-Type", string(MediaJSON))
	w.Header().Set("Location", "/properties/"+entityID)
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(struct {
		Status string `json:"status"`
		ID     string `json:"id"`
		Type   string `json:"type"`
	}{Status: "created", ID: entityID, Type: "Property"})
}

func (c *Component) mintPropertyEntityID(uniqueID string) string {
	return c.cfg.PropertyIDPrefix + "." + uniqueIDToToken(uniqueID)
}

func (c *Component) buildPropertyTriples(body []byte) (string, []message.Triple, error) {
	var in propertyPostBody
	if err := json.Unmarshal(body, &in); err != nil {
		return "", nil, fmt.Errorf("invalid property JSON: %w", err)
	}
	uid := in.UniqueID
	if uid == "" {
		uid = in.UID
	}
	if uid == "" {
		return "", nil, errors.New("uniqueId required")
	}
	entityID := c.mintPropertyEntityID(uid)
	triples := []message.Triple{
		{Subject: entityID, Predicate: sensorml.PredType, Object: sosa.ObservableProperty},
		{Subject: entityID, Predicate: PredSystemUID, Object: uid},
	}
	label := in.Label
	if label == "" {
		label = in.Name
	}
	if label != "" {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: sensorml.PredLabel, Object: label,
		})
	}
	if in.Description != "" {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: sensorml.PredDescription, Object: in.Description,
		})
	}
	if in.Definition != "" {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: predPropertyDefinition, Object: in.Definition,
		})
	}
	if in.BaseProperty != "" {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: predPropertyBaseProperty, Object: in.BaseProperty,
		})
	}
	return entityID, triples, nil
}

func (c *Component) handlePropertiesOptions(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Allow", "GET, HEAD, POST, OPTIONS")
	w.WriteHeader(http.StatusNoContent)
}

func (c *Component) handlePropertyOptions(w http.ResponseWriter, r *http.Request) {
	pathID := r.PathValue("id")
	if err := validateEntityID(pathID); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid property id: "+err.Error())
		return
	}
	w.Header().Set("Allow", "GET, HEAD, OPTIONS")
	w.WriteHeader(http.StatusNoContent)
}
