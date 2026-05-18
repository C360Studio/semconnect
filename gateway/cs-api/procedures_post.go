// Stage 20 — POST /procedures. Mirrors POST /systems exactly
// except for:
//   - rdf:type triple object → sosa.Procedure (not sosa.SSNSystem)
//   - ID prefix → cfg.ProcedureIDPrefix
//   - NO position triple (procedures don't carry location per
//     /req/procedure/location)
//
// Accepts the same four media types POST /systems does
// (sml+json / sensorml+json / json / geo+json) — the SensorML path
// for full spec parity (SOSA Procedure ↔ SensorML SimpleProcess),
// the Feature path for the ETS-style minimum-shape POST.
package csapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/parser/sensorml"
	"github.com/c360studio/semstreams/vocabulary/sosa"
)

// handleProcedurePost serves POST /procedures.
func (c *Component) handleProcedurePost(w http.ResponseWriter, r *http.Request) {
	ct := r.Header.Get("Content-Type")
	if err := requireMediaTypeAny(ct,
		string(MediaSensorML), string(MediaSensorMLLegacy),
		string(MediaJSON), string(MediaGeoJSON)); err != nil {
		w.Header().Set("Accept-Post", strings.Join([]string{
			string(MediaSensorML), string(MediaSensorMLLegacy),
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

	mt, _, _ := mime.ParseMediaType(ct)
	var (
		entityID string
		triples  []message.Triple
		buildErr error
	)
	switch mt {
	case string(MediaSensorML), string(MediaSensorMLLegacy):
		entityID, triples, buildErr = c.buildProcedureTriplesFromSensorML(body)
	default:
		entityID, triples, buildErr = c.buildProcedureTriplesFromFeature(body)
	}
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
	w.Header().Set("Location", "/procedures/"+entityID)
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(struct {
		Status string `json:"status"`
		ID     string `json:"id"`
		Type   string `json:"type"`
	}{Status: "created", ID: entityID, Type: "Procedure"})
}

// mintProcedureEntityID is the procedure-namespace mirror of
// mintSystemEntityID.
func (c *Component) mintProcedureEntityID(uniqueID string) string {
	return c.cfg.ProcedureIDPrefix + "." + uniqueIDToToken(uniqueID)
}

// buildProcedureTriplesFromSensorML mirrors buildSystemTriplesFromSensorML
// but emits the Procedure rdf:type and skips the position triple.
// The framework's sensorml.Asset.Triples() already emits the correct
// rdf:type when the Process is a SimpleProcess/AggregateProcess
// (mapped to sosa.Procedure); we override it explicitly here so a
// PhysicalSystem mistakenly POSTed to /procedures still gets
// classified correctly.
func (c *Component) buildProcedureTriplesFromSensorML(body []byte) (string, []message.Triple, error) {
	process, err := sensorml.UnmarshalProcess(body)
	if err != nil {
		return "", nil, fmt.Errorf("invalid SensorML JSON: %w", err)
	}
	if process == nil || process.Base() == nil {
		return "", nil, errors.New("empty SensorML process")
	}
	entityID := c.mintProcedureEntityID(process.Base().UniqueID)
	asset := sensorml.NewAsset(entityID, process)
	triples := asset.Triples()
	if len(triples) == 0 {
		return entityID, nil, errors.New("SensorML process produced no representable triples")
	}
	// Override rdf:type to sosa.Procedure — the framework emits a
	// type triple based on the source kind (PhysicalSystem → SSNSystem,
	// SimpleProcess → Procedure). For /procedures we MUST land under
	// the Procedure class so the predicate-query collection sees it.
	//
	// Defensive fallback: if no PredType triple exists in the emission
	// (today the framework always emits one, but a future upstream
	// split of rdf:type emission off Asset.Triples would silently land
	// procedures unindexed), prepend one so the entity is queryable.
	var overrode bool
	for i, t := range triples {
		if t.Predicate == sensorml.PredType {
			triples[i].Object = sosa.Procedure
			overrode = true
			break
		}
	}
	if !overrode {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: sensorml.PredType, Object: sosa.Procedure,
		})
	}
	// Stage 18 uid preservation — same predicate as /systems.
	if uid := process.Base().UniqueID; uid != "" {
		triples = append(triples, message.Triple{
			Subject: entityID, Predicate: PredSystemUID, Object: uid,
		})
	}
	return entityID, triples, nil
}

// buildProcedureTriplesFromFeature mirrors buildSystemTriplesFromFeature
// without the geometry triple (procedures don't carry location).
// Re-uses systemFeatureBody since the body shape is identical — the
// minimum Feature schema (type + properties.uid + name + description)
// is resource-kind-agnostic at v0.1.
func (c *Component) buildProcedureTriplesFromFeature(body []byte) (string, []message.Triple, error) {
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
	entityID := c.mintProcedureEntityID(feat.Properties.UID)
	triples := []message.Triple{
		{Subject: entityID, Predicate: sensorml.PredType, Object: sosa.Procedure},
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
	// NO position triple — /req/procedure/location forbids it.
	return entityID, triples, nil
}
