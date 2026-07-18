// Stage 55 — CS API Part 2 Command Feasibility read-side.
//
// Feasibility resources describe a command preflight/probe result for a
// ControlStream. v0.1 exposes read resources plus a JSON fixture POST helper
// used by the conformance harness; it does not execute commands or evaluate
// device-side feasibility.
package csapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/c360studio/semconnect/parser/sensorml"
	csapivocab "github.com/c360studio/semconnect/vocabulary/csapi"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
)

const (
	// semstreams does not yet expose a CS API Feasibility vocabulary term.
	// Keep the local type IRI explicit so it is easy to retire when the
	// framework grows one.
	FeasibilityTypeIRI = csapivocab.Feasibility

	PredFeasibilityControlStream = csapivocab.FeasibilityControlStream
	predFeasibilityStatus        = csapivocab.FeasibilityStatus
	predFeasibilityParams        = csapivocab.FeasibilityParams
	predFeasibilityResult        = csapivocab.FeasibilityResult
)

type feasibilityCollection struct {
	Items []feasibility `json:"items"`
	Links []link        `json:"links,omitempty"`
}

type feasibility struct {
	ID              string          `json:"id"`
	Type            string          `json:"type"`
	ControlStreamID string          `json:"controlstream@id,omitempty"`
	ControlStream   *link           `json:"controlstream@link,omitempty"`
	Status          string          `json:"status,omitempty"`
	Params          json.RawMessage `json:"params,omitempty"`
	Parameters      json.RawMessage `json:"parameters,omitempty"`
	Result          json.RawMessage `json:"result,omitempty"`
	Links           []link          `json:"links,omitempty"`
}

type feasibilityPostBody struct {
	ID              string          `json:"id,omitempty"`
	ControlStreamID string          `json:"controlstream@id"`
	Status          string          `json:"status,omitempty"`
	Params          json.RawMessage `json:"params,omitempty"`
	Parameters      json.RawMessage `json:"parameters,omitempty"`
	Result          json.RawMessage `json:"result,omitempty"`
}

type feasibilityStatusCollection struct {
	Items []feasibilityStatus `json:"items"`
	Links []link              `json:"links,omitempty"`
}

type feasibilityStatus struct {
	Status string `json:"status"`
}

type feasibilityResultCollection struct {
	Items []json.RawMessage `json:"items"`
	Links []link            `json:"links,omitempty"`
}

func feasibilityFromState(state graph.EntityState) feasibility {
	f := feasibility{
		ID:     state.ID,
		Type:   "Feasibility",
		Status: "accepted",
		Links: []link{
			{Href: "/feasibility/" + state.ID, Rel: "self", Type: string(MediaJSON)},
			{Href: "/feasibility/" + state.ID, Rel: "canonical", Type: string(MediaJSON)},
			{Href: "/feasibility/" + state.ID + "/status", Rel: "status", Type: string(MediaJSON)},
			{Href: "/feasibility/" + state.ID + "/result", Rel: "result", Type: string(MediaJSON)},
		},
	}
	if v, ok := firstStringObject(state.Triples, PredFeasibilityControlStream); ok {
		f.ControlStreamID = v
		f.ControlStream = &link{Href: "/controlstreams/" + v, Rel: "controlstream", Type: string(MediaJSON), Title: v}
		f.Links = append(f.Links, link{Href: "/controlstream/" + v + "/feasibility", Rel: "controlstream-feasibility", Type: string(MediaJSON)})
	}
	if v, ok := firstStringObject(state.Triples, predFeasibilityStatus); ok {
		f.Status = v
	}
	if v, ok := firstStringObject(state.Triples, predFeasibilityParams); ok && v != "" {
		f.Params = json.RawMessage(v)
		f.Parameters = json.RawMessage(v)
	}
	if v, ok := firstStringObject(state.Triples, predFeasibilityResult); ok && v != "" {
		f.Result = json.RawMessage(v)
	}
	return f
}

func isFeasibilityKind(triples []message.Triple) bool {
	typeIRI, ok := firstStringObject(triples, sensorml.PredType)
	return ok && typeIRI == FeasibilityTypeIRI
}

func (c *Component) handleFeasibilities(w http.ResponseWriter, r *http.Request) {
	if _, ok := NegotiateRequest(r, FamilyFeasibilityCollection); !ok {
		WriteNotAcceptable(w, FamilyFeasibilityCollection)
		return
	}
	limit, err := parseLimit(r.URL.Query().Get("limit"), c.cfg.DefaultListLimit, c.cfg.MaxListLimit)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	ids, err := c.listEntitiesByType(r.Context(), FeasibilityTypeIRI, limit, "listFeasibilityEntities")
	if err != nil {
		c.writeBackendError(w, err)
		return
	}
	c.writeFeasibilityCollection(w, r, ids, "", "/feasibility", limit)
}

func (c *Component) handleControlStreamFeasibility(w http.ResponseWriter, r *http.Request) {
	if _, ok := NegotiateRequest(r, FamilyFeasibilityCollection); !ok {
		WriteNotAcceptable(w, FamilyFeasibilityCollection)
		return
	}
	controlStreamID := r.PathValue("id")
	if err := validateEntityID(controlStreamID); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid controlstream id: "+err.Error())
		return
	}
	limit, err := parseLimit(r.URL.Query().Get("limit"), c.cfg.DefaultListLimit, c.cfg.MaxListLimit)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	state, err := c.fetchEntity(r.Context(), controlStreamID)
	if err != nil {
		c.writeBackendError(w, err)
		return
	}
	if !isControlStreamKind(state.Triples) {
		writeJSONError(w, http.StatusNotFound, "entity is not a ControlStream")
		return
	}
	ids, err := c.listEntitiesByType(r.Context(), FeasibilityTypeIRI, c.cfg.MaxListLimit, "listControlStreamFeasibilityEntities")
	if err != nil {
		c.writeBackendError(w, err)
		return
	}
	c.writeFeasibilityCollection(w, r, ids, controlStreamID, "/controlstream/"+controlStreamID+"/feasibility", limit)
}

func (c *Component) writeFeasibilityCollection(w http.ResponseWriter, r *http.Request, ids []string, controlStreamFilter string, selfHref string, limit int) {
	coll := feasibilityCollection{
		Items: make([]feasibility, 0, len(ids)),
		Links: []link{{Href: selfHref, Rel: "self", Type: string(MediaJSON)}},
	}
	statesByID, err := c.fetchEntitiesBatch(r.Context(), ids)
	if err != nil {
		c.writeBackendError(w, err)
		return
	}
	for _, id := range ids {
		state, ok := statesByID[id]
		if !ok {
			c.logger.Warn("batch entity fetch for Feasibility collection missed entity; skipping",
				"entity", id)
			continue
		}
		if !isFeasibilityKind(state.Triples) {
			continue
		}
		f := feasibilityFromState(state)
		if controlStreamFilter != "" && f.ControlStreamID != controlStreamFilter {
			continue
		}
		coll.Items = append(coll.Items, f)
		if len(coll.Items) == limit {
			break
		}
	}

	w.Header().Set("Content-Type", string(MediaJSON))
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	_ = json.NewEncoder(w).Encode(coll)
}

func (c *Component) handleFeasibility(w http.ResponseWriter, r *http.Request) {
	if _, ok := NegotiateRequest(r, FamilyFeasibilityItem); !ok {
		WriteNotAcceptable(w, FamilyFeasibilityItem)
		return
	}
	id := r.PathValue("id")
	if err := validateEntityID(id); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid feasibility id: "+err.Error())
		return
	}
	state, err := c.fetchEntity(r.Context(), id)
	if err != nil {
		c.writeBackendError(w, err)
		return
	}
	if !isFeasibilityKind(state.Triples) {
		writeJSONError(w, http.StatusNotFound, "entity is not a Feasibility resource")
		return
	}
	w.Header().Set("Content-Type", string(MediaJSON))
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	_ = json.NewEncoder(w).Encode(feasibilityFromState(state))
}

func (c *Component) handleFeasibilityStatus(w http.ResponseWriter, r *http.Request) {
	if _, ok := NegotiateRequest(r, FamilyFeasibilityCollection); !ok {
		WriteNotAcceptable(w, FamilyFeasibilityCollection)
		return
	}
	f, ok := c.fetchFeasibilityForSubresource(w, r)
	if !ok {
		return
	}
	body := feasibilityStatusCollection{
		Items: []feasibilityStatus{{Status: f.Status}},
		Links: []link{{Href: "/feasibility/" + f.ID + "/status", Rel: "self", Type: string(MediaJSON)}},
	}
	w.Header().Set("Content-Type", string(MediaJSON))
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	_ = json.NewEncoder(w).Encode(body)
}

func (c *Component) handleFeasibilityResult(w http.ResponseWriter, r *http.Request) {
	if _, ok := NegotiateRequest(r, FamilyFeasibilityCollection); !ok {
		WriteNotAcceptable(w, FamilyFeasibilityCollection)
		return
	}
	f, ok := c.fetchFeasibilityForSubresource(w, r)
	if !ok {
		return
	}
	items := []json.RawMessage{}
	if len(f.Result) > 0 {
		items = append(items, f.Result)
	}
	body := feasibilityResultCollection{
		Items: items,
		Links: []link{{Href: "/feasibility/" + f.ID + "/result", Rel: "self", Type: string(MediaJSON)}},
	}
	w.Header().Set("Content-Type", string(MediaJSON))
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	_ = json.NewEncoder(w).Encode(body)
}

func (c *Component) fetchFeasibilityForSubresource(w http.ResponseWriter, r *http.Request) (feasibility, bool) {
	id := r.PathValue("id")
	if err := validateEntityID(id); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid feasibility id: "+err.Error())
		return feasibility{}, false
	}
	state, err := c.fetchEntity(r.Context(), id)
	if err != nil {
		c.writeBackendError(w, err)
		return feasibility{}, false
	}
	if !isFeasibilityKind(state.Triples) {
		writeJSONError(w, http.StatusNotFound, "entity is not a Feasibility resource")
		return feasibility{}, false
	}
	return feasibilityFromState(state), true
}

func (c *Component) handleFeasibilityPost(w http.ResponseWriter, r *http.Request) {
	if err := requireMediaTypeAny(r.Header.Get("Content-Type"), string(MediaJSON)); err != nil {
		w.Header().Set("Accept-Post", string(MediaJSON))
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
	entityID, triples, buildErr := c.buildFeasibilityTriples(body)
	if buildErr != nil {
		writeJSONError(w, http.StatusBadRequest, buildErr.Error())
		return
	}
	if err := c.ingestTriples(r.Context(), triples, IdentityFrom(r.Context())); err != nil {
		w.Header().Set("X-CS-Attempted-ID", entityID)
		c.writeBackendError(w, err)
		return
	}
	w.Header().Set("Content-Type", string(MediaJSON))
	w.Header().Set("Location", "/feasibility/"+entityID)
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(struct {
		Status string `json:"status"`
		ID     string `json:"id"`
		Type   string `json:"type"`
	}{Status: "created", ID: entityID, Type: "Feasibility"})
}

func (c *Component) mintFeasibilityEntityID(uniqueID string) string {
	return mintEntityID(c.cfg.FeasibilityIDPrefix, []byte(uniqueID))
}

func (c *Component) buildFeasibilityTriples(body []byte) (string, []message.Triple, error) {
	var in feasibilityPostBody
	if err := json.Unmarshal(body, &in); err != nil {
		return "", nil, fmt.Errorf("invalid feasibility JSON: %w", err)
	}
	if in.ControlStreamID == "" {
		return "", nil, errors.New("controlstream@id required")
	}
	if err := validateEntityID(in.ControlStreamID); err != nil {
		return "", nil, fmt.Errorf("controlstream@id invalid: %w", err)
	}
	entityID := in.ID
	if entityID == "" {
		entityID = c.mintFeasibilityEntityID(in.ControlStreamID)
	}
	if err := validateEntityID(entityID); err != nil {
		return "", nil, fmt.Errorf("id invalid: %w", err)
	}
	if len(in.Params) == 0 && len(in.Parameters) > 0 {
		in.Params = in.Parameters
	}
	if len(in.Params) > 0 && !json.Valid(in.Params) {
		return "", nil, errors.New("params must be valid JSON")
	}
	if len(in.Result) > 0 && !json.Valid(in.Result) {
		return "", nil, errors.New("result must be valid JSON")
	}
	if in.Status == "" {
		in.Status = "accepted"
	}
	triples := []message.Triple{
		{Subject: entityID, Predicate: sensorml.PredType, Object: FeasibilityTypeIRI},
		{Subject: entityID, Predicate: PredFeasibilityControlStream, Object: in.ControlStreamID, Datatype: message.EntityReferenceDatatype},
		{Subject: entityID, Predicate: predFeasibilityStatus, Object: in.Status},
	}
	if len(in.Params) > 0 && string(in.Params) != "null" {
		triples = append(triples, message.Triple{Subject: entityID, Predicate: predFeasibilityParams, Object: string(in.Params)})
	}
	if len(in.Result) > 0 && string(in.Result) != "null" {
		triples = append(triples, message.Triple{Subject: entityID, Predicate: predFeasibilityResult, Object: string(in.Result)})
	}
	return entityID, triples, nil
}

func (c *Component) handleFeasibilitiesOptions(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Allow", "GET, HEAD, POST, OPTIONS")
	w.WriteHeader(http.StatusNoContent)
}

func (c *Component) handleFeasibilityOptions(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Allow", "GET, HEAD, OPTIONS")
	w.WriteHeader(http.StatusNoContent)
}
