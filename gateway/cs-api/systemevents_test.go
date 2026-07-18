// Stage 25 — SystemEvent handler tests.
package csapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/c360studio/semconnect/parser/sensorml"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/natsclient"
)

const (
	testSystemEventID = "c360.semconnect.systems.csapi.systemevent.boot"
	testEventSystemID = "c360.semconnect.systems.csapi.system.weather001"
)

func systemEventState(t *testing.T) []byte {
	t.Helper()
	payload, _ := json.Marshal(map[string]any{"status": "nominal"})
	return encodeEntityState(t, graph.EntityState{
		ID: testSystemEventID,
		Triples: []message.Triple{
			{Subject: testSystemEventID, Predicate: sensorml.PredType, Object: SystemEventTypeIRI},
			{Subject: testSystemEventID, Predicate: PredSystemEventSystem, Object: testEventSystemID, Datatype: message.EntityReferenceDatatype},
			{Subject: testSystemEventID, Predicate: predSystemEventTime, Object: "2026-05-19T12:00:00Z"},
			{Subject: testSystemEventID, Predicate: predSystemEventType, Object: "SystemChanged"},
			{Subject: testSystemEventID, Predicate: predSystemEventMessage, Object: "System booted"},
			{Subject: testSystemEventID, Predicate: predSystemEventSource, Object: "ets"},
			{Subject: testSystemEventID, Predicate: predSystemEventPayload, Object: string(payload)},
		},
	})
}

func TestHandleSystemEvents_GoldenPath(t *testing.T) {
	fake := &multiReplyFakeRequester{
		predicateReply: encodeReply(t, []string{testSystemEventID}),
		entityRepliesByID: map[string][]byte{
			testSystemEventID: systemEventState(t),
		},
	}
	c := newComponentWithRequester(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/systemEvents", nil)
	rr := httptest.NewRecorder()
	c.handleSystemEvents(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	var coll systemEventCollection
	if err := json.Unmarshal(rr.Body.Bytes(), &coll); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(coll.Items) != 1 {
		t.Fatalf("items: got %d want 1", len(coll.Items))
	}
	if coll.Items[0].ID != testSystemEventID || coll.Items[0].EventType != "SystemChanged" {
		t.Errorf("item: %+v", coll.Items[0])
	}
	if coll.Items[0].SystemID != testEventSystemID {
		t.Errorf("system@id: got %q", coll.Items[0].SystemID)
	}
	if fake.calls != 2 {
		t.Errorf("requests: got %d want 2 (1 predicate + 1 batch)", fake.calls)
	}
}

func TestHandleSystemEvents_AdvancedFilterByEventType(t *testing.T) {
	otherID := "c360.semconnect.systems.csapi.systemevent.other"
	otherState := encodeEntityState(t, graph.EntityState{
		ID: otherID,
		Triples: []message.Triple{
			{Subject: otherID, Predicate: sensorml.PredType, Object: SystemEventTypeIRI},
			{Subject: otherID, Predicate: PredSystemEventSystem, Object: testEventSystemID, Datatype: message.EntityReferenceDatatype},
			{Subject: otherID, Predicate: predSystemEventTime, Object: "2026-05-19T12:01:00Z"},
			{Subject: otherID, Predicate: predSystemEventType, Object: "Maintenance"},
		},
	})
	fake := &multiReplyFakeRequester{
		predicateReply: encodeReply(t, []string{testSystemEventID, otherID}),
		entityRepliesByID: map[string][]byte{
			testSystemEventID: systemEventState(t),
			otherID:           otherState,
		},
	}
	c := newComponentWithRequester(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/systemEvents?eventType=SystemChanged", nil)
	rr := httptest.NewRecorder()
	c.handleSystemEvents(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	var coll systemEventCollection
	if err := json.Unmarshal(rr.Body.Bytes(), &coll); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(coll.Items) != 1 || coll.Items[0].ID != testSystemEventID {
		t.Fatalf("items: %+v", coll.Items)
	}
}

func TestHandleSystemEvent_JSON(t *testing.T) {
	fake := &fakeRequester{
		reply:  systemEventState(t),
		status: natsclient.StatusConnected,
	}
	c := newTestComponent(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/systemEvents/"+testSystemEventID, nil)
	req.SetPathValue("id", testSystemEventID)
	rr := httptest.NewRecorder()
	c.handleSystemEvent(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	var ev systemEvent
	if err := json.Unmarshal(rr.Body.Bytes(), &ev); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if ev.Message != "System booted" || ev.EventTime != "2026-05-19T12:00:00Z" {
		t.Errorf("event: %+v", ev)
	}
	if ev.Payload["status"] != "nominal" {
		t.Errorf("payload: %+v", ev.Payload)
	}
}

func TestHandleSystemScopedEvents_FiltersBySystem(t *testing.T) {
	otherID := "c360.semconnect.systems.csapi.systemevent.other"
	otherState := encodeEntityState(t, graph.EntityState{
		ID: otherID,
		Triples: []message.Triple{
			{Subject: otherID, Predicate: sensorml.PredType, Object: SystemEventTypeIRI},
			{Subject: otherID, Predicate: PredSystemEventSystem, Object: "c360.semconnect.systems.csapi.system.other", Datatype: message.EntityReferenceDatatype},
			{Subject: otherID, Predicate: predSystemEventTime, Object: "2026-05-19T12:01:00Z"},
			{Subject: otherID, Predicate: predSystemEventType, Object: "SystemChanged"},
		},
	})
	fake := &multiReplyFakeRequester{
		predicateReply: encodeReply(t, []string{testSystemEventID, otherID}),
		entityRepliesByID: map[string][]byte{
			testSystemEventID: systemEventState(t),
			otherID:           otherState,
		},
	}
	c := newComponentWithRequester(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/systems/"+testEventSystemID+"/events", nil)
	req.SetPathValue("id", testEventSystemID)
	rr := httptest.NewRecorder()
	c.handleSystemScopedEvents(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	var coll systemEventCollection
	if err := json.Unmarshal(rr.Body.Bytes(), &coll); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(coll.Items) != 1 || coll.Items[0].ID != testSystemEventID {
		t.Errorf("items: %+v", coll.Items)
	}
}

func TestHandleSystemScopedEventPost_JSON(t *testing.T) {
	fake := &fakeRequester{
		status: natsclient.StatusConnected,
		reply:  encodeBatchOK(t, 7),
	}
	c := newTestComponent(t, fake)

	body := []byte(`{"eventTime":"2026-05-19T12:00:00Z","eventType":"SystemChanged","message":"System booted","source":"ets","payload":{"status":"nominal"}}`)
	req := httptest.NewRequest(http.MethodPost, "/systems/"+testEventSystemID+"/events", bytes.NewReader(body))
	req.Header.Set("Content-Type", string(MediaJSON))
	req.SetPathValue("id", testEventSystemID)
	rr := httptest.NewRecorder()
	c.handleSystemScopedEventPost(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status: got %d want 201; body=%s", rr.Code, rr.Body.String())
	}
	if loc := rr.Header().Get("Location"); !strings.HasPrefix(loc, "/systemEvents/"+c.cfg.SystemEventIDPrefix+".") {
		t.Errorf("Location: got %q", loc)
	}
	var batch graph.AddTriplesBatchRequest
	if err := json.Unmarshal(fake.gotBody, &batch); err != nil {
		t.Fatalf("decode batch: %v", err)
	}
	var sawType, sawSystem, sawEventType bool
	for _, tr := range batch.Triples {
		switch tr.Predicate {
		case sensorml.PredType:
			sawType = tr.Object == SystemEventTypeIRI
		case PredSystemEventSystem:
			sawSystem = tr.Object == testEventSystemID
		case predSystemEventType:
			sawEventType = tr.Object == "SystemChanged"
		}
	}
	if !sawType || !sawSystem || !sawEventType {
		t.Errorf("batch missing triples: type=%v system=%v eventType=%v batch=%+v",
			sawType, sawSystem, sawEventType, batch.Triples)
	}
}
