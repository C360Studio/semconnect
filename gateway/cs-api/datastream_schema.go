package csapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/c360studio/semstreams/pkg/errs"
	"github.com/c360studio/semstreams/pkg/swecommon"
)

func (c *Component) handleDatastreamSchema(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := validateEntityID(id); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid datastream id: "+err.Error())
		return
	}

	if media, ok := NegotiateRequest(r, FamilyDatastreamItem); !ok || media != MediaJSON {
		WriteNotAcceptable(w, FamilyDatastreamItem)
		return
	}

	state, err := c.fetchEntity(r.Context(), id)
	if err != nil {
		c.writeBackendError(w, err)
		return
	}
	if !isDatastreamKind(state.Triples) {
		writeJSONError(w, http.StatusNotFound, "no datastream: "+id)
		return
	}

	raw, ok, err := c.readSchemaArtifact(r.Context(), state.Triples, PredDatastreamSchema)
	if err != nil {
		c.writeBackendError(w, err)
		return
	}
	if !ok {
		writeJSONError(w, http.StatusNotFound, "no datastream schema: "+id)
		return
	}
	schema, err := swecommon.UnmarshalSchema(raw)
	if err != nil {
		c.writeBackendError(w, errs.Wrap(err, "cs-api", "handleDatastreamSchema", "decode schema"))
		return
	}
	body, err := swecommon.MarshalSchema(schema)
	if err != nil {
		c.writeBackendError(w, errs.Wrap(err, "cs-api", "handleDatastreamSchema", "encode schema"))
		return
	}
	var resultSchema map[string]any
	if err := json.Unmarshal(body, &resultSchema); err != nil {
		c.writeBackendError(w, errs.Wrap(err, "cs-api", "handleDatastreamSchema", "decode canonical schema"))
		return
	}
	resp := datastreamObservationSchema{
		ObsFormat:    string(MediaJSON),
		ResultSchema: resultSchema,
	}

	w.Header().Set("Content-Type", string(MediaJSON))
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		c.errs.Add(1)
		c.logger.Error("write datastream schema response", "id", id, "err", err)
	}
}

func (c *Component) fetchDatastreamObservationSchema(r *http.Request, datastreamID string) (*swecommon.DataRecord, bool) {
	state, err := c.fetchEntity(r.Context(), datastreamID)
	if err != nil {
		if !errors.Is(err, errEntityNotFound) {
			c.logger.Warn("datastream schema lookup failed; falling back to inferred observation schema",
				"datastream", datastreamID, "err", err)
		}
		return nil, false
	}
	raw, ok, err := c.readSchemaArtifact(r.Context(), state.Triples, PredDatastreamSchema)
	if err != nil {
		c.logger.Warn("datastream schema artifact read failed; falling back to inferred observation schema",
			"datastream", datastreamID, "err", err)
		return nil, false
	}
	if !ok {
		return nil, false
	}
	schema, err := swecommon.UnmarshalSchema(raw)
	if err != nil {
		c.logger.Warn("datastream schema decode failed; falling back to inferred observation schema",
			"datastream", datastreamID, "err", err)
		return nil, false
	}
	return schema, true
}
