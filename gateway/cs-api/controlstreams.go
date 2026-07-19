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
	"net/url"

	"github.com/c360studio/semconnect/parser/sensorml"
	"github.com/c360studio/semconnect/pkg/swecommon"
	csapivocab "github.com/c360studio/semconnect/vocabulary/csapi"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
)

const (
	ControlStreamTypeIRI = csapivocab.ControlStream
	CommandTypeIRI       = csapivocab.Command

	PredControlStreamSystem               = csapivocab.ControlsSystem
	PredCommandControlStream              = csapivocab.PartOfControlStream
	predControlStreamInputName            = csapivocab.ControlStreamInputName
	predControlStreamAsync                = csapivocab.ControlStreamAsync
	predControlStreamCommandFormat        = csapivocab.ControlStreamCommandFormat
	predControlStreamSchema               = csapivocab.HasCommandSchema
	predControlStreamControlledProperties = csapivocab.ControlStreamControlledProperties
	predControlStreamIssueTime            = csapivocab.ControlStreamIssueTime
	predControlStreamExecutionTime        = csapivocab.ControlStreamExecutionTime
	predCommandIssueTime                  = csapivocab.CommandIssueTime
	predCommandExecutionTime              = csapivocab.CommandExecutionTime
	predCommandStatus                     = csapivocab.CommandStatus
	predCommandSender                     = csapivocab.CommandSender
	predCommandParams                     = csapivocab.CommandParams
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

func (p controlledProperty) getDefinition() string { return p.Definition }

type commandSchema struct {
	CommandFormat    string         `json:"commandFormat"`
	ParametersSchema map[string]any `json:"parametersSchema"`
}

type commandCollection struct {
	Items []command `json:"items"`
	Links []link    `json:"links,omitempty"`
}

type command struct {
	ID              string          `json:"id"`
	Type            string          `json:"type"`
	ControlStreamID string          `json:"controlstream@id,omitempty"`
	ControlStream   *link           `json:"controlstream@link,omitempty"`
	IssueTime       string          `json:"issueTime,omitempty"`
	ExecutionTime   string          `json:"executionTime,omitempty"`
	Status          string          `json:"status,omitempty"`
	CurrentStatus   string          `json:"currentStatus,omitempty"`
	Sender          string          `json:"sender,omitempty"`
	Params          json.RawMessage `json:"params,omitempty"`
	Links           []link          `json:"links,omitempty"`
}

type controlStreamPostBody struct {
	ID                   string               `json:"id,omitempty"`
	Name                 string               `json:"name"`
	Description          string               `json:"description,omitempty"`
	SystemID             string               `json:"system@id,omitempty"`
	InputName            string               `json:"inputName"`
	ControlledProperties []controlledProperty `json:"controlledProperties,omitempty"`
	IssueTime            string               `json:"issueTime,omitempty"`
	ExecutionTime        string               `json:"executionTime,omitempty"`
	Async                bool                 `json:"async"`
	Schema               commandSchema        `json:"schema"`
}

type commandPostBody struct {
	ID              string          `json:"id,omitempty"`
	ControlStreamID string          `json:"controlstream@id"`
	IssueTime       string          `json:"issueTime,omitempty"`
	ExecutionTime   string          `json:"executionTime,omitempty"`
	Status          string          `json:"status,omitempty"`
	Sender          string          `json:"sender,omitempty"`
	Params          json.RawMessage `json:"params,omitempty"`
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
	if v, ok := firstStringObject(state.Triples, predControlStreamIssueTime); ok {
		cs.IssueTime = v
	}
	if v, ok := firstStringObject(state.Triples, predControlStreamExecutionTime); ok {
		cs.ExecutionTime = v
	}
	cs.ControlledProperties = controlledPropertiesFromTriples(state.Triples)
	return cs
}

func isControlStreamKind(triples []message.Triple) bool {
	typeIRI, ok := firstStringObject(triples, sensorml.PredType)
	return ok && typeIRI == ControlStreamTypeIRI
}

func commandFromState(state graph.EntityState) command {
	cmd := command{
		ID:     state.ID,
		Type:   "Command",
		Status: "accepted",
		Links: []link{
			{Href: "/commands/" + state.ID, Rel: "canonical", Type: string(MediaJSON)},
		},
	}
	if v, ok := firstStringObject(state.Triples, PredCommandControlStream); ok {
		cmd.ControlStreamID = v
		cmd.ControlStream = &link{Href: "/controlstreams/" + v, Rel: "controlstream", Type: string(MediaJSON), Title: v}
		cmd.Links = append(cmd.Links, link{Href: "/controlstreams/" + v + "/commands", Rel: "controlstream", Type: string(MediaJSON)})
	}
	if v, ok := firstStringObject(state.Triples, predCommandIssueTime); ok {
		cmd.IssueTime = v
	}
	if v, ok := firstStringObject(state.Triples, predCommandExecutionTime); ok {
		cmd.ExecutionTime = v
	}
	if v, ok := firstStringObject(state.Triples, predCommandStatus); ok {
		cmd.Status = v
	}
	cmd.CurrentStatus = cmd.Status
	if v, ok := firstStringObject(state.Triples, predCommandSender); ok {
		cmd.Sender = v
	}
	if v, ok := firstStringObject(state.Triples, predCommandParams); ok && v != "" {
		cmd.Params = json.RawMessage(v)
	}
	return cmd
}

func isCommandKind(triples []message.Triple) bool {
	typeIRI, ok := firstStringObject(triples, sensorml.PredType)
	return ok && typeIRI == CommandTypeIRI
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

	filters := controlStreamCollectionFiltersFromQuery(r.URL.Query())
	listLimit := limit
	if filters.active() {
		listLimit = c.cfg.MaxListLimit
	}
	ids, err := c.listEntitiesByType(r.Context(), ControlStreamTypeIRI, listLimit, "listControlStreamEntities")
	if err != nil {
		c.writeBackendError(w, err)
		return
	}
	c.writeControlStreamCollection(w, r, ids, "", filters, limit)
}

type controlStreamCollectionFilters struct {
	controlledProperty string
	issueTime          string
	executionTime      string
}

func controlStreamCollectionFiltersFromQuery(query url.Values) controlStreamCollectionFilters {
	return controlStreamCollectionFilters{
		controlledProperty: queryString(query, "controlledProperty"),
		issueTime:          queryString(query, "issueTime"),
		executionTime:      queryString(query, "executionTime"),
	}
}

func (f controlStreamCollectionFilters) active() bool {
	return f.controlledProperty != "" || f.issueTime != "" || f.executionTime != ""
}

func controlStreamMatchesFilters(cs controlStream, filters controlStreamCollectionFilters) bool {
	if filters.controlledProperty != "" && !propertyDefinitionsContain(cs.ControlledProperties, filters.controlledProperty) {
		return false
	}
	if filters.issueTime != "" {
		value, _ := cs.IssueTime.(string)
		if !resourceTimeIntersects(value, filters.issueTime) {
			return false
		}
	}
	if filters.executionTime != "" {
		value, _ := cs.ExecutionTime.(string)
		if !resourceTimeIntersects(value, filters.executionTime) {
			return false
		}
	}
	return true
}

func (c *Component) writeControlStreamCollection(w http.ResponseWriter, r *http.Request, ids []string, systemFilter string, filters controlStreamCollectionFilters, limit int) {
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
		if !controlStreamMatchesFilters(cs, filters) {
			continue
		}
		coll.Items = append(coll.Items, cs)
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
	if _, ok := NegotiateRequest(r, FamilyControlStreamCollection); !ok {
		WriteNotAcceptable(w, FamilyControlStreamCollection)
		return
	}
	id := r.PathValue("id")
	if err := validateEntityID(id); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid controlstream id: "+err.Error())
		return
	}
	limit, err := parseLimit(r.URL.Query().Get("limit"), c.cfg.DefaultListLimit, c.cfg.MaxListLimit)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
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

	ids, err := c.listEntitiesByType(r.Context(), CommandTypeIRI, c.cfg.MaxListLimit, "listControlStreamCommandEntities")
	if err != nil {
		c.writeBackendError(w, err)
		return
	}
	c.writeCommandCollection(w, r, ids, id, "/controlstreams/"+id+"/commands", commandCollectionFilters{}, limit)
}

func (c *Component) handleCommands(w http.ResponseWriter, r *http.Request) {
	if _, ok := NegotiateRequest(r, FamilyControlStreamCollection); !ok {
		WriteNotAcceptable(w, FamilyControlStreamCollection)
		return
	}
	limit, err := parseLimit(r.URL.Query().Get("limit"), c.cfg.DefaultListLimit, c.cfg.MaxListLimit)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	filters := commandCollectionFiltersFromQuery(r.URL.Query())
	listLimit := limit
	if filters.active() {
		listLimit = c.cfg.MaxListLimit
	}
	ids, err := c.listEntitiesByType(r.Context(), CommandTypeIRI, listLimit, "listCommandEntities")
	if err != nil {
		c.writeBackendError(w, err)
		return
	}
	c.writeCommandCollection(w, r, ids, "", "/commands", filters, limit)
}

func (c *Component) handleCommand(w http.ResponseWriter, r *http.Request) {
	if _, ok := NegotiateRequest(r, FamilyControlStreamItem); !ok {
		WriteNotAcceptable(w, FamilyControlStreamItem)
		return
	}
	id := r.PathValue("id")
	if err := validateEntityID(id); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid command id: "+err.Error())
		return
	}
	state, err := c.fetchEntity(r.Context(), id)
	if err != nil {
		c.writeBackendError(w, err)
		return
	}
	if !isCommandKind(state.Triples) {
		writeJSONError(w, http.StatusNotFound, "entity is not a Command")
		return
	}
	w.Header().Set("Content-Type", string(MediaJSON))
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	_ = json.NewEncoder(w).Encode(commandFromState(state))
}

type commandCollectionFilters struct {
	issueTime     string
	executionTime string
	statusCode    string
	sender        string
}

func commandCollectionFiltersFromQuery(query url.Values) commandCollectionFilters {
	return commandCollectionFilters{
		issueTime:     queryString(query, "issueTime"),
		executionTime: queryString(query, "executionTime"),
		statusCode:    queryString(query, "statusCode"),
		sender:        queryString(query, "sender"),
	}
}

func (f commandCollectionFilters) active() bool {
	return f.issueTime != "" || f.executionTime != "" || f.statusCode != "" || f.sender != ""
}

func commandMatchesFilters(cmd command, filters commandCollectionFilters) bool {
	if filters.issueTime != "" && !resourceTimeIntersects(cmd.IssueTime, filters.issueTime) {
		return false
	}
	if filters.executionTime != "" && !resourceTimeIntersects(cmd.ExecutionTime, filters.executionTime) {
		return false
	}
	if filters.statusCode != "" && cmd.Status != filters.statusCode && cmd.CurrentStatus != filters.statusCode {
		return false
	}
	if filters.sender != "" && cmd.Sender != filters.sender {
		return false
	}
	return true
}

func (c *Component) writeCommandCollection(w http.ResponseWriter, r *http.Request, ids []string, controlStreamFilter string, selfHref string, filters commandCollectionFilters, limit int) {
	coll := commandCollection{
		Items: make([]command, 0, len(ids)),
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
			c.logger.Warn("batch entity fetch for Command collection missed entity; skipping",
				"entity", id)
			continue
		}
		if !isCommandKind(state.Triples) {
			continue
		}
		cmd := commandFromState(state)
		if controlStreamFilter != "" && cmd.ControlStreamID != controlStreamFilter {
			continue
		}
		if !commandMatchesFilters(cmd, filters) {
			continue
		}
		coll.Items = append(coll.Items, cmd)
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
	c.writeControlStreamCollection(w, r, ids, systemID, controlStreamCollectionFilters{}, limit)
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

func (c *Component) handleCommandPost(w http.ResponseWriter, r *http.Request) {
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
	entityID, triples, buildErr := c.buildCommandTriples(body)
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
	w.Header().Set("Location", "/commands/"+entityID)
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(struct {
		Status string `json:"status"`
		ID     string `json:"id"`
		Type   string `json:"type"`
	}{Status: "created", ID: entityID, Type: "Command"})
}

func (c *Component) mintControlStreamEntityID(uniqueID string) string {
	return mintEntityID(c.cfg.ControlStreamIDPrefix, []byte(uniqueID))
}

func (c *Component) mintCommandEntityID(uniqueID string) string {
	return mintEntityID(c.cfg.CommandIDPrefix, []byte(uniqueID))
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
		triples = append(triples, message.Triple{Subject: entityID, Predicate: PredControlStreamSystem, Object: in.SystemID, Datatype: message.EntityReferenceDatatype})
	}
	if in.IssueTime != "" {
		triples = append(triples, message.Triple{Subject: entityID, Predicate: predControlStreamIssueTime, Object: in.IssueTime})
	}
	if in.ExecutionTime != "" {
		triples = append(triples, message.Triple{Subject: entityID, Predicate: predControlStreamExecutionTime, Object: in.ExecutionTime})
	}
	return entityID, triples, in.Schema, nil
}

func (c *Component) buildCommandTriples(body []byte) (string, []message.Triple, error) {
	var in commandPostBody
	if err := json.Unmarshal(body, &in); err != nil {
		return "", nil, fmt.Errorf("invalid command JSON: %w", err)
	}
	if in.ControlStreamID == "" {
		return "", nil, errors.New("controlstream@id required")
	}
	if err := validateEntityID(in.ControlStreamID); err != nil {
		return "", nil, fmt.Errorf("controlstream@id invalid: %w", err)
	}
	entityID := in.ID
	if entityID == "" {
		entityID = c.mintCommandEntityID(in.ControlStreamID)
	}
	if err := validateEntityID(entityID); err != nil {
		return "", nil, fmt.Errorf("id invalid: %w", err)
	}
	if len(in.Params) > 0 && !json.Valid(in.Params) {
		return "", nil, errors.New("params must be valid JSON")
	}
	if in.Status == "" {
		in.Status = "accepted"
	}
	triples := []message.Triple{
		{Subject: entityID, Predicate: sensorml.PredType, Object: CommandTypeIRI},
		{Subject: entityID, Predicate: PredCommandControlStream, Object: in.ControlStreamID, Datatype: message.EntityReferenceDatatype},
		{Subject: entityID, Predicate: predCommandStatus, Object: in.Status},
	}
	if in.IssueTime != "" {
		triples = append(triples, message.Triple{Subject: entityID, Predicate: predCommandIssueTime, Object: in.IssueTime})
	}
	if in.ExecutionTime != "" {
		triples = append(triples, message.Triple{Subject: entityID, Predicate: predCommandExecutionTime, Object: in.ExecutionTime})
	}
	if in.Sender != "" {
		triples = append(triples, message.Triple{Subject: entityID, Predicate: predCommandSender, Object: in.Sender})
	}
	if len(in.Params) > 0 && string(in.Params) != "null" {
		triples = append(triples, message.Triple{Subject: entityID, Predicate: predCommandParams, Object: string(in.Params)})
	}
	return entityID, triples, nil
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
	w.Header().Set("Allow", "GET, HEAD, POST, OPTIONS")
	w.WriteHeader(http.StatusNoContent)
}
