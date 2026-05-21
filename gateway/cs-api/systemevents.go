// Stage 25 — CS API Part 2 System Events read-side. SystemEvent facts
// describe noteworthy changes or lifecycle notices about a System. v0.1
// exposes the read resources the ETS exercises plus a JSON POST helper for
// conformance fixture seeding.
package csapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/parser/sensorml"
)

const (
	SystemEventTypeIRI = "http://www.opengis.net/spec/ogcapi-connectedsystems-2/1.0/SystemEvent"

	PredSystemEventSystem    = "cs-api.systemevent.system"
	predSystemEventTime      = "cs-api.systemevent.time"
	predSystemEventType      = "cs-api.systemevent.type"
	predSystemEventMessage   = "cs-api.systemevent.message"
	predSystemEventSeverity  = "cs-api.systemevent.severity"
	predSystemEventSource    = "cs-api.systemevent.source"
	predSystemEventPayload   = "cs-api.systemevent.payload"
	predSystemEventKeywords  = "cs-api.systemevent.keywords"
	defaultSystemEventType   = "SystemChanged"
	defaultSystemEventSource = "semconnect"
)

type systemEventCollection struct {
	Items []systemEvent `json:"items"`
	Links []link        `json:"links,omitempty"`
}

type systemEvent struct {
	ID          string         `json:"id"`
	Time        string         `json:"time"`
	EventTime   string         `json:"eventTime,omitempty"`
	EventType   string         `json:"eventType"`
	Message     string         `json:"message,omitempty"`
	Description string         `json:"description,omitempty"`
	SystemID    string         `json:"system@id,omitempty"`
	SystemLink  *link          `json:"system@link,omitempty"`
	SystemUID   string         `json:"systemUid,omitempty"`
	Severity    string         `json:"severity,omitempty"`
	Source      string         `json:"source,omitempty"`
	Keywords    []string       `json:"keywords,omitempty"`
	Payload     map[string]any `json:"payload,omitempty"`
	Links       []link         `json:"links,omitempty"`
}

type systemEventPostBody struct {
	ID          string         `json:"id,omitempty"`
	Time        string         `json:"time,omitempty"`
	EventTime   string         `json:"eventTime,omitempty"`
	EventType   string         `json:"eventType,omitempty"`
	Message     string         `json:"message,omitempty"`
	Description string         `json:"description,omitempty"`
	SystemID    string         `json:"system@id,omitempty"`
	SystemUID   string         `json:"systemUid,omitempty"`
	Severity    string         `json:"severity,omitempty"`
	Source      string         `json:"source,omitempty"`
	Keywords    []string       `json:"keywords,omitempty"`
	Payload     map[string]any `json:"payload,omitempty"`
}

func systemEventFromState(state graph.EntityState) systemEvent {
	ev := systemEvent{
		ID:        state.ID,
		Time:      time.Now().UTC().Format(time.RFC3339),
		EventType: defaultSystemEventType,
		Source:    defaultSystemEventSource,
		Links: []link{
			{Href: "/systemEvents/" + state.ID, Rel: "self", Type: string(MediaJSON)},
			{Href: "/systemEvents/" + state.ID, Rel: "canonical", Type: string(MediaJSON)},
		},
	}
	if v, ok := firstStringObject(state.Triples, predSystemEventTime); ok {
		ev.Time = v
		ev.EventTime = v
	}
	if v, ok := firstStringObject(state.Triples, predSystemEventType); ok {
		ev.EventType = v
	}
	if v, ok := firstStringObject(state.Triples, predSystemEventMessage); ok {
		ev.Message = v
	}
	if v, ok := firstStringObject(state.Triples, sensorml.PredDescription); ok {
		ev.Description = v
	}
	if v, ok := firstStringObject(state.Triples, PredSystemEventSystem); ok {
		ev.SystemID = v
		ev.SystemLink = &link{Href: "/systems/" + v, Rel: "system", Type: string(MediaJSON), Title: v}
		ev.Links = append(ev.Links, link{Href: "/systems/" + v + "/events/" + state.ID, Rel: "alternate", Type: string(MediaJSON)})
	}
	if v, ok := firstStringObject(state.Triples, PredSystemUID); ok {
		ev.SystemUID = v
	}
	if v, ok := firstStringObject(state.Triples, predSystemEventSeverity); ok {
		ev.Severity = v
	}
	if v, ok := firstStringObject(state.Triples, predSystemEventSource); ok {
		ev.Source = v
	}
	if v, ok := firstStringObject(state.Triples, predSystemEventKeywords); ok {
		var keywords []string
		if err := json.Unmarshal([]byte(v), &keywords); err == nil {
			ev.Keywords = keywords
		}
	}
	if v, ok := firstStringObject(state.Triples, predSystemEventPayload); ok {
		var payload map[string]any
		if err := json.Unmarshal([]byte(v), &payload); err == nil {
			ev.Payload = payload
		}
	}
	return ev
}

func isSystemEventKind(triples []message.Triple) bool {
	typeIRI, ok := firstStringObject(triples, typeAliases...)
	return ok && typeIRI == SystemEventTypeIRI
}

func (c *Component) handleSystemEvents(w http.ResponseWriter, r *http.Request) {
	if _, ok := NegotiateRequest(r, FamilySystemEventCollection); !ok {
		WriteNotAcceptable(w, FamilySystemEventCollection)
		return
	}
	limit, err := parseLimit(r.URL.Query().Get("limit"), c.cfg.DefaultListLimit, c.cfg.MaxListLimit)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	ids, err := c.listEntitiesByType(r.Context(), SystemEventTypeIRI, limit, "listSystemEventEntities")
	if err != nil {
		c.writeBackendError(w, err)
		return
	}
	c.writeSystemEventCollection(w, r, ids, "")
}

func (c *Component) handleSystemScopedEvents(w http.ResponseWriter, r *http.Request) {
	if _, ok := NegotiateRequest(r, FamilySystemEventCollection); !ok {
		WriteNotAcceptable(w, FamilySystemEventCollection)
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
	ids, err := c.listEntitiesByType(r.Context(), SystemEventTypeIRI, limit, "listSystemScopedEventEntities")
	if err != nil {
		c.writeBackendError(w, err)
		return
	}
	c.writeSystemEventCollection(w, r, ids, systemID)
}

func (c *Component) writeSystemEventCollection(w http.ResponseWriter, r *http.Request, ids []string, systemFilter string) {
	coll := systemEventCollection{
		Items: make([]systemEvent, 0, len(ids)),
		Links: []link{{Href: "/systemEvents", Rel: "self", Type: string(MediaJSON)}},
	}
	for _, id := range ids {
		state, err := c.fetchEntity(r.Context(), id)
		if err != nil {
			c.logger.Warn("fetch entity for SystemEvent collection failed; skipping",
				"entity", id, "err", err.Error())
			continue
		}
		if !isSystemEventKind(state.Triples) {
			continue
		}
		ev := systemEventFromState(state)
		if systemFilter != "" && ev.SystemID != systemFilter {
			continue
		}
		coll.Items = append(coll.Items, ev)
	}
	w.Header().Set("Content-Type", string(MediaJSON))
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	_ = json.NewEncoder(w).Encode(coll)
}

func (c *Component) handleSystemEvent(w http.ResponseWriter, r *http.Request) {
	c.writeSystemEventItem(w, r, r.PathValue("id"), "")
}

func (c *Component) handleSystemScopedEvent(w http.ResponseWriter, r *http.Request) {
	systemID := r.PathValue("id")
	if err := validateEntityID(systemID); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid system id: "+err.Error())
		return
	}
	c.writeSystemEventItem(w, r, r.PathValue("eventID"), systemID)
}

func (c *Component) writeSystemEventItem(w http.ResponseWriter, r *http.Request, eventID string, systemFilter string) {
	if _, ok := NegotiateRequest(r, FamilySystemEventItem); !ok {
		WriteNotAcceptable(w, FamilySystemEventItem)
		return
	}
	if err := validateEntityID(eventID); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid system event id: "+err.Error())
		return
	}
	state, err := c.fetchEntity(r.Context(), eventID)
	if err != nil {
		c.writeBackendError(w, err)
		return
	}
	if !isSystemEventKind(state.Triples) {
		writeJSONError(w, http.StatusNotFound, "entity is not a SystemEvent")
		return
	}
	ev := systemEventFromState(state)
	if systemFilter != "" && ev.SystemID != systemFilter {
		writeJSONError(w, http.StatusNotFound, "SystemEvent is not associated with the requested system")
		return
	}
	w.Header().Set("Content-Type", string(MediaJSON))
	w.Header().Set("X-CS-Reconstructed-Lossy", "true")
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	_ = json.NewEncoder(w).Encode(ev)
}

func (c *Component) handleSystemEventPost(w http.ResponseWriter, r *http.Request) {
	c.handleSystemEventPostWithSystem(w, r, "")
}

func (c *Component) handleSystemScopedEventPost(w http.ResponseWriter, r *http.Request) {
	systemID := r.PathValue("id")
	if err := validateEntityID(systemID); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid system id: "+err.Error())
		return
	}
	c.handleSystemEventPostWithSystem(w, r, systemID)
}

func (c *Component) handleSystemEventPostWithSystem(w http.ResponseWriter, r *http.Request, pathSystemID string) {
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
	entityID, triples, buildErr := c.buildSystemEventTriples(body, pathSystemID)
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
	w.Header().Set("Location", "/systemEvents/"+entityID)
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(struct {
		Status string `json:"status"`
		ID     string `json:"id"`
		Type   string `json:"type"`
	}{Status: "created", ID: entityID, Type: "SystemEvent"})
}

func (c *Component) mintSystemEventEntityID(seed string) string {
	return c.cfg.SystemEventIDPrefix + "." + uniqueIDToToken(seed)
}

func (c *Component) buildSystemEventTriples(body []byte, pathSystemID string) (string, []message.Triple, error) {
	var in systemEventPostBody
	if err := json.Unmarshal(body, &in); err != nil {
		return "", nil, fmt.Errorf("invalid system event JSON: %w", err)
	}
	if pathSystemID != "" {
		if in.SystemID != "" && in.SystemID != pathSystemID {
			return "", nil, errors.New("system@id must match path system id")
		}
		in.SystemID = pathSystemID
	}
	if in.SystemID == "" {
		return "", nil, errors.New("system@id required")
	}
	if err := validateEntityID(in.SystemID); err != nil {
		return "", nil, fmt.Errorf("system@id invalid: %w", err)
	}
	eventTime := in.EventTime
	if eventTime == "" {
		eventTime = in.Time
	}
	if eventTime == "" {
		eventTime = time.Now().UTC().Format(time.RFC3339)
	}
	if in.EventType == "" {
		in.EventType = defaultSystemEventType
	}
	if in.Source == "" {
		in.Source = defaultSystemEventSource
	}
	entityID := in.ID
	if entityID == "" {
		entityID = c.mintSystemEventEntityID(in.SystemID + "-" + in.EventType + "-" + eventTime)
	}
	if err := validateEntityID(entityID); err != nil {
		return "", nil, fmt.Errorf("id invalid: %w", err)
	}
	triples := []message.Triple{
		{Subject: entityID, Predicate: sensorml.PredType, Object: SystemEventTypeIRI},
		{Subject: entityID, Predicate: PredSystemEventSystem, Object: in.SystemID},
		{Subject: entityID, Predicate: predSystemEventTime, Object: eventTime},
		{Subject: entityID, Predicate: predSystemEventType, Object: in.EventType},
		{Subject: entityID, Predicate: predSystemEventSource, Object: in.Source},
	}
	if in.Message != "" {
		triples = append(triples, message.Triple{Subject: entityID, Predicate: predSystemEventMessage, Object: in.Message})
	}
	if in.Description != "" {
		triples = append(triples, message.Triple{Subject: entityID, Predicate: sensorml.PredDescription, Object: in.Description})
	}
	if in.SystemUID != "" {
		triples = append(triples, message.Triple{Subject: entityID, Predicate: PredSystemUID, Object: in.SystemUID})
	}
	if in.Severity != "" {
		triples = append(triples, message.Triple{Subject: entityID, Predicate: predSystemEventSeverity, Object: in.Severity})
	}
	if len(in.Keywords) > 0 {
		keywords, _ := json.Marshal(in.Keywords)
		triples = append(triples, message.Triple{Subject: entityID, Predicate: predSystemEventKeywords, Object: string(keywords)})
	}
	if len(in.Payload) > 0 {
		payload, _ := json.Marshal(in.Payload)
		triples = append(triples, message.Triple{Subject: entityID, Predicate: predSystemEventPayload, Object: string(payload)})
	}
	return entityID, triples, nil
}

func (c *Component) handleSystemEventsOptions(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Allow", "GET, HEAD, POST, OPTIONS")
	w.WriteHeader(http.StatusNoContent)
}

func (c *Component) handleSystemEventOptions(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Allow", "GET, HEAD, OPTIONS")
	w.WriteHeader(http.StatusNoContent)
}
