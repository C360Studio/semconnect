// Stage 23 — Properties handler tests.
package csapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/parser/sensorml"
	"github.com/c360studio/semstreams/vocabulary/sosa"
)

const testPropertyID = "c360.semconnect.systems.csapi.property.airtemperature"

func TestHandleProperties_GoldenPath(t *testing.T) {
	fake := &fakeRequester{
		reply:  encodeReply(t, []string{testPropertyID}),
		status: natsclient.StatusConnected,
	}
	c := newTestComponent(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/properties", nil)
	rr := httptest.NewRecorder()
	c.handleProperties(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	var coll propertyCollection
	if err := json.Unmarshal(rr.Body.Bytes(), &coll); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if coll.Type != "PropertyCollection" || len(coll.Items) != 1 {
		t.Errorf("collection: type=%q items=%d", coll.Type, len(coll.Items))
	}
	if !strings.Contains(string(fake.gotBody), sosa.ObservableProperty) {
		t.Errorf("predicate-query should reference %q; body=%s",
			sosa.ObservableProperty, string(fake.gotBody))
	}
}

func TestHandleProperty_JSON(t *testing.T) {
	state := graph.EntityState{
		ID: testPropertyID,
		Triples: []message.Triple{
			{Subject: testPropertyID, Predicate: sensorml.PredType, Object: sosa.ObservableProperty},
			{Subject: testPropertyID, Predicate: sensorml.PredLabel, Object: "Air temperature"},
			{Subject: testPropertyID, Predicate: sensorml.PredDescription, Object: "Ambient air temperature"},
			{Subject: testPropertyID, Predicate: PredSystemUID, Object: "urn:example:prop:airtemp"},
			{Subject: testPropertyID, Predicate: predPropertyDefinition, Object: "http://qudt.org/vocab/quantitykind/Temperature"},
		},
	}
	fake := &fakeRequester{
		reply:  encodeEntityState(t, state),
		status: natsclient.StatusConnected,
	}
	c := newTestComponent(t, fake)
	mux := http.NewServeMux()
	c.RegisterHTTPHandlers("", mux)

	req := httptest.NewRequest(http.MethodGet, "/properties/"+testPropertyID, nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	var prop propertyResource
	if err := json.Unmarshal(rr.Body.Bytes(), &prop); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if prop.Type != "Property" {
		t.Errorf("Type: got %q want Property", prop.Type)
	}
	if prop.Label != "Air temperature" {
		t.Errorf("Label: got %q", prop.Label)
	}
	if prop.Definition != "http://qudt.org/vocab/quantitykind/Temperature" {
		t.Errorf("Definition: got %q", prop.Definition)
	}
	if prop.UID != "urn:example:prop:airtemp" {
		t.Errorf("UID: got %q", prop.UID)
	}
}

func TestHandleProperty_NotAPropertyKind(t *testing.T) {
	state := graph.EntityState{
		ID: testPropertyID,
		Triples: []message.Triple{
			{Subject: testPropertyID, Predicate: sensorml.PredType, Object: sosa.SSNSystem},
		},
	}
	fake := &fakeRequester{
		reply:  encodeEntityState(t, state),
		status: natsclient.StatusConnected,
	}
	c := newTestComponent(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/properties/"+testPropertyID, nil)
	req.SetPathValue("id", testPropertyID)
	rr := httptest.NewRecorder()
	c.handleProperty(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status: got %d want 404", rr.Code)
	}
}

func TestHandlePropertyPost_SensorMLProperty(t *testing.T) {
	fake := &fakeRequester{
		status: natsclient.StatusConnected,
		reply:  encodeBatchOK(t, 4),
	}
	c := newTestComponent(t, fake)

	body := []byte(`{"uniqueId":"urn:example:prop:airtemp","label":"Air temperature","description":"Ambient air temperature","definition":"http://qudt.org/vocab/quantitykind/Temperature"}`)
	req := httptest.NewRequest(http.MethodPost, "/properties", bytes.NewReader(body))
	req.Header.Set("Content-Type", string(MediaSensorML))
	rr := httptest.NewRecorder()
	c.handlePropertyPost(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status: got %d want 201; body=%s", rr.Code, rr.Body.String())
	}
	loc := rr.Header().Get("Location")
	if !strings.HasPrefix(loc, "/properties/"+c.cfg.PropertyIDPrefix+".") {
		t.Errorf("Location: got %q, want /properties/<prefix>.<token>", loc)
	}

	var batch graph.AddTriplesBatchRequest
	if err := json.Unmarshal(fake.gotBody, &batch); err != nil {
		t.Fatalf("decode batch: %v", err)
	}
	var sawType, sawUID, sawDefinition bool
	for _, tr := range batch.Triples {
		if tr.Predicate == sensorml.PredType {
			if s, ok := tr.Object.(string); ok && s == sosa.ObservableProperty {
				sawType = true
			}
		}
		if tr.Predicate == PredSystemUID {
			sawUID = true
		}
		if tr.Predicate == predPropertyDefinition {
			sawDefinition = true
		}
	}
	if !sawType {
		t.Errorf("rdf:type should be sosa.ObservableProperty; batch=%+v", batch.Triples)
	}
	if !sawUID {
		t.Errorf("uid triple missing; batch=%+v", batch.Triples)
	}
	if !sawDefinition {
		t.Errorf("definition triple missing; batch=%+v", batch.Triples)
	}
}

func TestHandlePropertiesOptions(t *testing.T) {
	fake := &fakeRequester{status: natsclient.StatusConnected}
	c := newTestComponent(t, fake)

	req := httptest.NewRequest(http.MethodOptions, "/properties", nil)
	rr := httptest.NewRecorder()
	c.handlePropertiesOptions(rr, req)

	want := "GET, HEAD, POST, OPTIONS"
	if got := rr.Header().Get("Allow"); got != want {
		t.Errorf("Allow: got %q want %q", got, want)
	}
}
