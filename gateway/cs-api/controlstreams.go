// Stage 24 — CS API Part 2 Control Streams read-side. A ControlStream
// describes a command channel for a System. v0.1 exposes the read
// resources the ETS exercises plus a small JSON POST helper used by
// the conformance harness to seed fixture data.
package csapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/parser/sensorml"
	"github.com/c360studio/semstreams/pkg/swecommon"
	csapivocab "github.com/c360studio/semstreams/vocabulary/csapi"
)

const (
	ControlStreamTypeIRI = csapivocab.ControlStream

	PredControlStreamSystem               = csapivocab.ControlsSystem
	predControlStreamInputName            = "cs-api.controlstream.inputName"
	predControlStreamAsync                = "cs-api.controlstream.async"
	predControlStreamCommandFormat        = "cs-api.controlstream.commandFormat"
	predControlStreamSchema               = csapivocab.HasCommandSchema
	predControlStreamControlledProperties = "cs-api.controlstream.controlledProperties"
)

type controlStreamCollection struct {
	Items []controlStream `json:"items"`
	Links []link          `json:"links,omitempty"`
}

type controlStream struct {
	ID                   string               `json:"id"`
	Name                 string               `json:"name"`
	Description          string               `json:"description,omitempty"`
	SystemID             string               `json:"system@id,omitempty"`
	SystemLink           *link                `json:"system@link,omitempty"`
	InputName            string               `json:"inputName"`
	ControlledProperties []controlledProperty `json:"controlledProperties"`
	IssueTime            any                  `json:"issueTime"`
	ExecutionTime        any                  `json:"executionTime"`
	Live                 bool                 `json:"live"`
	Async                bool                 `json:"async"`
	Formats              []string             `json:"formats"`
	Links                []link               `json:"links,omitempty"`
}

type controlledProperty struct {
	Definition  string `json:"definition,omitempty"`
	Label       string `json:"label,omitempty"`
	Description string `json:"description,omitempty"`
}

type commandSchema struct {
	CommandFormat    string         `json:"commandFormat"`
	ParametersSchema map[string]any `json:"parametersSchema"`
}

type controlStreamPostBody struct {
	ID                   string               `json:"id,omitempty"`
	Name                 string               `json:"name"`
	Description          string               `json:"description,omitempty"`
	SystemID             string               `json:"system@id,omitempty"`
	InputName            string               `json:"inputName"`
	ControlledProperties []controlledProperty `json:"controlledProperties,omitempty"`
	Async                bool                 `json:"async"`
	Schema               commandSchema        `json:"schema"`
}

func controlStreamFromState(state graph.EntityState) controlStream {
	cs := controlStream{
		ID:            state.ID,
		Name:          state.ID,
		InputName:     "command",
		IssueTime:     nil,
		ExecutionTime: nil,
		Live:          true,
		Formats:       []string{string(MediaJSON)},
		Links: []link{
			{Href: "/controlstreams/" + state.ID, Rel: "self", Type: string(MediaJSON)},
			{Href: "/controlstreams/" + state.ID, Rel: "canonical", Type: string(MediaJSON)},
			{Href: "/controlstreams/" + state.ID + "/schema", Rel: "schema", Type: string(MediaJSON)},
			{Href: "/controlstreams/" + state.ID + "/commands", Rel: "commands", Type: string(MediaJSON)},
		},
	}
	if v, ok := firstStringObject(state.Triples, sensorml.PredLabel); ok {
		cs.Name = v
	}
	if v, ok := firstStringObject(state.Triples, sensorml.PredDescription); ok {
		cs.Description = v
	}
	if v, ok := firstStringObject(state.Triples, PredControlStreamSystem); ok {
		cs.SystemID = v
		cs.SystemLink = &link{Href: "/systems/" + v, Rel: "system", Type: string(MediaJSON), Title: v}
	}
	if v, ok := firstStringObject(state.Triples, predControlStreamInputName); ok {
		cs.InputName = v
	}
	if v, ok := firstStringObject(state.Triples, predControlStreamAsync); ok && v == "true" {
		cs.Async = true
	}
	if v, ok := firstStringObject(state.Triples, predControlStreamCommandFormat); ok && v != "" {
		cs.Formats = []string{v}
	}
	cs.ControlledProperties = controlledPropertiesFromTriples(state.Triples)
	return cs
}

func isControlStreamKind(triples []message.Triple) bool {
	typeIRI, ok := firstStringObject(triples, typeAliases...)
	return ok && typeIRI == ControlStreamTypeIRI
}

func (c *Component) handleControlStreams(w http.ResponseWriter, r *http.Request) {
	if _, ok := NegotiateRequest(r, FamilyControlStreamCollection); !ok {
		WriteNotAcceptable(w, FamilyControlStreamCollection)
		return
	}

	limit, err := parseLimit(r.URL.Query().Get("limit"), c.cfg.DefaultListLimit, c.cfg.MaxListLimit)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	ids, err := c.listEntitiesByType(r.Context(), ControlStreamTypeIRI, limit, "listControlStreamEntities")
	if err != nil {
		c.writeBackendError(w, err)
		return
	}
	c.writeControlStreamCollection(w, r, ids, "")
}

func (c *Component) writeControlStreamCollection(w http.ResponseWriter, r *http.Request, ids []string, systemFilter string) {
	coll := controlStreamCollection{
		Items: make([]controlStream, 0, len(ids)),
		Links: []link{
			{Href: "/controlstreams", Rel: "self", Type: string(MediaJSON)},
		},
	}
	statesByID, err := c.fetchEntitiesBatch(r.Context(), ids)
	if err != nil {
		c.writeBackendError(w, err)
		return
	}
	for _, id := range ids {
		state, ok := statesByID[id]
		if !ok {
			c.logger.Warn("batch entity fetch for ControlStream collection missed entity; skipping",
				"entity", id)
			continue
		}
		if !isControlStreamKind(state.Triples) {
			continue
		}
		cs := controlStreamFromState(state)
		if systemFilter != "" && cs.SystemID != systemFilter {
			continue
		}
		coll.Items = append(coll.Items, cs)
	}

	w.Header().Set("Content-Type", string(MediaJSON))
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	_ = json.NewEncoder(w).Encode(coll)
}

func (c *Component) handleControlStream(w http.ResponseWriter, r *http.Request) {
	if _, ok := NegotiateRequest(r, FamilyControlStreamItem); !ok {
		WriteNotAcceptable(w, FamilyControlStreamItem)
		return
	}
	id := r.PathValue("id")
	if err := validateEntityID(id); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid controlstream id: "+err.Error())
		return
	}
	state, err := c.fetchEntity(r.Context(), id)
	if err != nil {
		c.writeBackendError(w, err)
		return
	}
	if !isControlStreamKind(state.Triples) {
		writeJSONError(w, http.StatusNotFound, "entity is not a ControlStream")
		return
	}
	w.Header().Set("Content-Type", string(MediaJSON))
	w.Header().Set("X-CS-Reconstructed-Lossy", "true")
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	_ = json.NewEncoder(w).Encode(controlStreamFromState(state))
}

func (c *Component) handleControlStreamSchema(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := validateEntityID(id); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid controlstream id: "+err.Error())
		return
	}
	state, err := c.fetchEntity(r.Context(), id)
	if err != nil {
		c.writeBackendError(w, err)
		return
	}
	if !isControlStreamKind(state.Triples) {
		writeJSONError(w, http.StatusNotFound, "entity is not a ControlStream")
		return
	}
	var schema commandSchema
	if r.Method != http.MethodHead {
		schema, err = c.controlStreamSchemaFromState(r.Context(), state)
		if err != nil {
			c.writeBackendError(w, err)
			return
		}
	}
	w.Header().Set("Content-Type", string(MediaJSON))
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	_ = json.NewEncoder(w).Encode(schema)
}

func (c *Component) handleControlStreamCommands(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := validateEntityID(id); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid controlstream id: "+err.Error())
		return
	}
	state, err := c.fetchEntity(r.Context(), id)
	if err != nil {
		c.writeBackendError(w, err)
		return
	}
	if !isControlStreamKind(state.Triples) {
		writeJSONError(w, http.StatusNotFound, "entity is not a ControlStream")
		return
	}
	w.Header().Set("Content-Type", string(MediaJSON))
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	_ = json.NewEncoder(w).Encode(struct {
		Items []any  `json:"items"`
		Links []link `json:"links,omitempty"`
	}{
		Items: []any{},
		Links: []link{{Href: "/controlstreams/" + id + "/commands", Rel: "self", Type: string(MediaJSON)}},
	})
}

func (c *Component) handleCommands(w http.ResponseWriter, r *http.Request) {
	if _, ok := NegotiateRequest(r, FamilyControlStreamCollection); !ok {
		WriteNotAcceptable(w, FamilyControlStreamCollection)
		return
	}
	if _, err := parseLimit(r.URL.Query().Get("limit"), c.cfg.DefaultListLimit, c.cfg.MaxListLimit); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	w.Header().Set("Content-Type", string(MediaJSON))
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	_ = json.NewEncoder(w).Encode(struct {
		Items []any  `json:"items"`
		Links []link `json:"links,omitempty"`
	}{
		Items: []any{},
		Links: []link{{Href: "/commands", Rel: "self", Type: string(MediaJSON)}},
	})
}

func (c *Component) handleSystemControlStreams(w http.ResponseWriter, r *http.Request) {
	if _, ok := NegotiateRequest(r, FamilyControlStreamCollection); !ok {
		WriteNotAcceptable(w, FamilyControlStreamCollection)
		return
	}
	systemID := r.PathValue("id")
	if err := validateEntityID(systemID); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid system id: "+err.Error())
		return
	}
	limit, err := parseLimit(r.URL.Query().Get("limit"), c.cfg.DefaultListLimit, c.cfg.MaxListLimit)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	ids, err := c.listEntitiesByType(r.Context(), ControlStreamTypeIRI, limit, "listSystemControlStreamEntities")
	if err != nil {
		c.writeBackendError(w, err)
		return
	}
	c.writeControlStreamCollection(w, r, ids, systemID)
}

func (c *Component) handleControlStreamPost(w http.ResponseWriter, r *http.Request) {
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
	entityID, triples, schema, buildErr := c.buildControlStreamTriples(body)
	if buildErr != nil {
		writeJSONError(w, http.StatusBadRequest, buildErr.Error())
		return
	}
	identity := IdentityFrom(r.Context())
	rawSchema, err := commandParametersSchemaRaw(schema)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "schema invalid: "+err.Error())
		return
	}
	rel, err := c.createSchemaArtifact(r.Context(), entityID, predControlStreamSchema, rawSchema, identity)
	if err != nil {
		w.Header().Set("X-CS-Attempted-ID", entityID)
		c.writeBackendError(w, err)
		return
	}
	triples = append(triples, rel)
	if err := c.ingestTriples(r.Context(), triples, identity); err != nil {
		w.Header().Set("X-CS-Attempted-ID", entityID)
		c.writeBackendError(w, err)
		return
	}
	w.Header().Set("Content-Type", string(MediaJSON))
	w.Header().Set("Location", "/controlstreams/"+entityID)
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(struct {
		Status string `json:"status"`
		ID     string `json:"id"`
		Type   string `json:"type"`
	}{Status: "created", ID: entityID, Type: "ControlStream"})
}

func (c *Component) mintControlStreamEntityID(uniqueID string) string {
	return c.cfg.ControlStreamIDPrefix + "." + uniqueIDToToken(uniqueID)
}

func (c *Component) buildControlStreamTriples(body []byte) (string, []message.Triple, commandSchema, error) {
	var in controlStreamPostBody
	if err := json.Unmarshal(body, &in); err != nil {
		return "", nil, commandSchema{}, fmt.Errorf("invalid control stream JSON: %w", err)
	}
	if in.Name == "" {
		return "", nil, commandSchema{}, errors.New("name required")
	}
	if in.InputName == "" {
		return "", nil, commandSchema{}, errors.New("inputName required")
	}
	if in.SystemID != "" {
		if err := validateEntityID(in.SystemID); err != nil {
			return "", nil, commandSchema{}, fmt.Errorf("system@id invalid: %w", err)
		}
	}
	entityID := in.ID
	if entityID == "" {
		entityID = c.mintControlStreamEntityID(in.Name + "-" + in.InputName)
	}
	if err := validateEntityID(entityID); err != nil {
		return "", nil, commandSchema{}, fmt.Errorf("id invalid: %w", err)
	}
	if in.Schema.CommandFormat == "" {
		in.Schema.CommandFormat = string(MediaJSON)
	}
	schema, err := normalizeCommandSchema(in.Schema)
	if err != nil {
		return "", nil, commandSchema{}, fmt.Errorf("schema invalid: %w", err)
	}
	in.Schema = schema
	if len(in.ControlledProperties) == 0 {
		in.ControlledProperties = controlledPropertiesFromSchema(in.Schema)
	}
	propsBytes, _ := json.Marshal(in.ControlledProperties)
	triples := []message.Triple{
		{Subject: entityID, Predicate: sensorml.PredType, Object: ControlStreamTypeIRI},
		{Subject: entityID, Predicate: sensorml.PredLabel, Object: in.Name},
		{Subject: entityID, Predicate: predControlStreamInputName, Object: in.InputName},
		{Subject: entityID, Predicate: predControlStreamAsync, Object: fmt.Sprintf("%t", in.Async)},
		{Subject: entityID, Predicate: predControlStreamCommandFormat, Object: in.Schema.CommandFormat},
		{Subject: entityID, Predicate: predControlStreamControlledProperties, Object: string(propsBytes)},
	}
	if in.Description != "" {
		triples = append(triples, message.Triple{Subject: entityID, Predicate: sensorml.PredDescription, Object: in.Description})
	}
	if in.SystemID != "" {
		triples = append(triples, message.Triple{Subject: entityID, Predicate: PredControlStreamSystem, Object: in.SystemID})
	}
	return entityID, triples, in.Schema, nil
}

func controlledPropertiesFromTriples(triples []message.Triple) []controlledProperty {
	if v, ok := firstStringObject(triples, predControlStreamControlledProperties); ok && v != "" {
		var props []controlledProperty
		if err := json.Unmarshal([]byte(v), &props); err == nil {
			return props
		}
	}
	return []controlledProperty{}
}

func (c *Component) controlStreamSchemaFromState(ctx context.Context, state graph.EntityState) (commandSchema, error) {
	format := string(MediaJSON)
	if v, ok := firstStringObject(state.Triples, predControlStreamCommandFormat); ok && v != "" {
		format = v
	}
	raw, ok, err := c.readSchemaArtifact(ctx, state.Triples, predControlStreamSchema)
	if err != nil {
		return commandSchema{}, err
	}
	if !ok {
		return commandSchema{
			CommandFormat:    format,
			ParametersSchema: map[string]any{"type": "DataRecord", "fields": []any{}},
		}, nil
	}
	var params map[string]any
	if err := json.Unmarshal(raw, &params); err != nil {
		return commandSchema{}, fmt.Errorf("decode command schema artifact: %w", err)
	}
	schema, err := normalizeCommandSchema(commandSchema{CommandFormat: format, ParametersSchema: params})
	if err != nil {
		return commandSchema{}, err
	}
	return schema, nil
}

func normalizeCommandSchema(schema commandSchema) (commandSchema, error) {
	if schema.CommandFormat == "" {
		schema.CommandFormat = string(MediaJSON)
	}
	if schema.ParametersSchema == nil {
		return commandSchema{}, errors.New("parametersSchema required")
	}
	raw, err := json.Marshal(schema.ParametersSchema)
	if err != nil {
		return commandSchema{}, fmt.Errorf("marshal parametersSchema: %w", err)
	}
	record, err := swecommon.UnmarshalSchema(raw)
	if err != nil {
		return commandSchema{}, err
	}
	canonical, err := swecommon.MarshalSchema(record)
	if err != nil {
		return commandSchema{}, err
	}
	var params map[string]any
	if err := json.Unmarshal(canonical, &params); err != nil {
		return commandSchema{}, fmt.Errorf("decode canonical parametersSchema: %w", err)
	}
	schema.ParametersSchema = params
	return schema, nil
}

func commandParametersSchemaRaw(schema commandSchema) (json.RawMessage, error) {
	raw, err := json.Marshal(schema.ParametersSchema)
	if err != nil {
		return nil, fmt.Errorf("marshal parametersSchema: %w", err)
	}
	return json.RawMessage(raw), nil
}

func controlledPropertiesFromSchema(schema commandSchema) []controlledProperty {
	fields, ok := schema.ParametersSchema["fields"].([]any)
	if !ok {
		return []controlledProperty{}
	}
	props := make([]controlledProperty, 0, len(fields))
	for _, field := range fields {
		m, ok := field.(map[string]any)
		if !ok {
			continue
		}
		props = append(props, controlledProperty{
			Definition:  stringAny(m["definition"]),
			Label:       stringAny(m["label"]),
			Description: stringAny(m["description"]),
		})
	}
	return props
}

func stringAny(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func (c *Component) handleControlStreamsOptions(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Allow", "GET, HEAD, POST, OPTIONS")
	w.WriteHeader(http.StatusNoContent)
}

func (c *Component) handleControlStreamOptions(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Allow", "GET, HEAD, OPTIONS")
	w.WriteHeader(http.StatusNoContent)
}

func (c *Component) handleCommandsOptions(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Allow", "GET, HEAD, OPTIONS")
	w.WriteHeader(http.StatusNoContent)
}
