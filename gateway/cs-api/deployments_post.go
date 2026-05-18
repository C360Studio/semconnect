// Stage 21 — POST /deployments. JSON / geo+json Feature body only
// (no SensorML — SensorML doesn't have a Deployment encoding).
//
// Distinct from /procedures:
//   - rdf:type → ssnDeployment (local const; sosa.SSNDeployment not
//     defined upstream as of beta.75)
//   - Position triple emitted from `geometry` (deployments DO carry
//     location, unlike procedures)
//
// Distinct from /systems POST:
//   - SensorML branch absent (no SensorML deployment encoding)
//   - Different rdf:type + different entity-ID prefix
package csapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/parser/sensorml"
)

// handleDeploymentPost serves POST /deployments.
func (c *Component) handleDeploymentPost(w http.ResponseWriter, r *http.Request) {
	if err := requireMediaTypeAny(r.Header.Get("Content-Type"),
		string(MediaJSON), string(MediaGeoJSON)); err != nil {
		w.Header().Set("Accept-Post", strings.Join([]string{
			string(MediaJSON), string(MediaGeoJSON),
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

	entityID, triples, buildErr := c.buildDeploymentTriplesFromFeature(body)
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
	w.Header().Set("Location", "/deployments/"+entityID)
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(struct {
		Status string `json:"status"`
		ID     string `json:"id"`
		Type   string `json:"type"`
	}{Status: "created", ID: entityID, Type: "Deployment"})
}

func (c *Component) mintDeploymentEntityID(uniqueID string) string {
	return c.cfg.DeploymentIDPrefix + "." + uniqueIDToToken(uniqueID)
}

// buildDeploymentTriplesFromFeature mirrors the /systems Feature
// builder. Same uid preservation + position handling.
func (c *Component) buildDeploymentTriplesFromFeature(body []byte) (string, []message.Triple, error) {
	var feat systemFeatureBody
	if err := json.Unmarshal(body, &feat); err != nil {
		return "", nil, fmt.Errorf("invalid JSON Feature: %w", err)
	}
	if feat.Type != "Feature" {
		return "", nil, fmt.Errorf("expected Feature, got %q", feat.Type)
	}
	if feat.Properties.UID == "" {
		return "", nil, errors.New("properties.uid required")
	}
	entityID := c.mintDeploymentEntityID(feat.Properties.UID)
	triples := []message.Triple{
		{Subject: entityID, Predicate: sensorml.PredType, Object: ssnDeployment},
		{Subject: entityID, Predicate: PredSystemUID, Object: feat.Properties.UID},
	}
	if feat.Properties.Name != "" {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: sensorml.PredLabel, Object: feat.Properties.Name,
		})
	}
	if feat.Properties.Description != "" {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: sensorml.PredDescription, Object: feat.Properties.Description,
		})
	}
	if posTriple, ok := positionTripleFromGeometry(entityID, feat.Geometry); ok {
		triples = append(triples, posTriple)
	}
	return entityID, triples, nil
}

// positionTripleFromGeometry is a small shared helper. Both
// /systems and /deployments use the same predicate so factoring this
// out reduces duplication. Returns ok=false on missing or literal-
// null geometry.
func positionTripleFromGeometry(entityID string, geom json.RawMessage) (message.Triple, bool) {
	if len(geom) == 0 || string(geom) == "null" {
		return message.Triple{}, false
	}
	return message.Triple{
		Subject:   entityID,
		Predicate: PredSystemPosition,
		Object:    string(geom),
	}, true
}
