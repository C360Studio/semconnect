// Stage 22 — POST /samplingFeatures. JSON / geo+json Feature body
// only. Sampling features carry first-class GeoJSON geometry, stored
// with the same position triple used by the other Feature-shaped
// resources until semstreams grows typed geometry primitives.
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
	"github.com/c360studio/semstreams/vocabulary/sosa"
)

func (c *Component) handleSamplingFeaturePost(w http.ResponseWriter, r *http.Request) {
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

	entityID, triples, buildErr := c.buildSamplingFeatureTriplesFromFeature(body)
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
	w.Header().Set("Location", "/samplingFeatures/"+entityID)
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(struct {
		Status string `json:"status"`
		ID     string `json:"id"`
		Type   string `json:"type"`
	}{Status: "created", ID: entityID, Type: "SamplingFeature"})
}

func (c *Component) mintSamplingFeatureEntityID(uniqueID string) string {
	return c.cfg.SamplingFeatureIDPrefix + "." + uniqueIDToToken(uniqueID)
}

func (c *Component) buildSamplingFeatureTriplesFromFeature(body []byte) (string, []message.Triple, error) {
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
	entityID := c.mintSamplingFeatureEntityID(feat.Properties.UID)
	triples := []message.Triple{
		{Subject: entityID, Predicate: sensorml.PredType, Object: sosa.Sample},
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
