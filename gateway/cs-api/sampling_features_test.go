// Stage 22 — Sampling Features handler tests.
package csapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/c360studio/semconnect/parser/sensorml"
	"github.com/c360studio/semconnect/vocabulary/sosa"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/natsclient"
)

const testSamplingFeatureID = "c360.semconnect.systems.csapi.samplingfeature.alpha"

func TestHandleSamplingFeatures_GoldenPath(t *testing.T) {
	fake := &fakeRequester{
		reply:  encodeReply(t, []string{testSamplingFeatureID}),
		status: natsclient.StatusConnected,
	}
	c := newTestComponent(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/samplingFeatures", nil)
	rr := httptest.NewRecorder()
	c.handleSamplingFeatures(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	var coll samplingFeatureCollection
	if err := json.Unmarshal(rr.Body.Bytes(), &coll); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if coll.Type != "SamplingFeatureCollection" || len(coll.Items) != 1 {
		t.Errorf("collection: type=%q items=%d", coll.Type, len(coll.Items))
	}
	if !strings.Contains(string(fake.gotBody), sosa.Sample) {
		t.Errorf("predicate-query should reference %q; body=%s",
			sosa.Sample, string(fake.gotBody))
	}
}

func TestHandleSamplingFeature_JSON(t *testing.T) {
	state := graph.EntityState{
		ID: testSamplingFeatureID,
		Triples: []message.Triple{
			{Subject: testSamplingFeatureID, Predicate: sensorml.PredType, Object: sosa.Sample},
			{Subject: testSamplingFeatureID, Predicate: sensorml.PredLabel, Object: "Station footprint"},
			{Subject: testSamplingFeatureID, Predicate: PredSystemUID, Object: "urn:example:sf:1"},
			{Subject: testSamplingFeatureID, Predicate: predSamplingFeatureHostedProcedure, Object: "/procedures/c360.semconnect.systems.csapi.procedure.alpha"},
			{Subject: testSamplingFeatureID, Predicate: PredSystemPosition, Object: `{"type":"Polygon","coordinates":[[[0,0],[1,0],[1,1],[0,0]]]}`},
		},
	}
	fake := &fakeRequester{
		reply:  encodeEntityState(t, state),
		status: natsclient.StatusConnected,
	}
	c := newTestComponent(t, fake)
	mux := http.NewServeMux()
	c.RegisterHTTPHandlers("", mux)

	req := httptest.NewRequest(http.MethodGet, "/samplingFeatures/"+testSamplingFeatureID, nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	var sf samplingFeature
	if err := json.Unmarshal(rr.Body.Bytes(), &sf); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if sf.Type != "SamplingFeature" {
		t.Errorf("Type: got %q want SamplingFeature", sf.Type)
	}
	if sf.Label != "Station footprint" {
		t.Errorf("Label: got %q", sf.Label)
	}
	if sf.UID != "urn:example:sf:1" {
		t.Errorf("UID: got %q", sf.UID)
	}
	if string(sf.Geometry) != `{"type":"Polygon","coordinates":[[[0,0],[1,0],[1,1],[0,0]]]}` {
		t.Errorf("Geometry: got %q", string(sf.Geometry))
	}
	if sf.FeatureProperties == nil || sf.FeatureProperties.HostedProcedureLink == nil {
		t.Fatalf("properties.hostedProcedure@link missing: %+v", sf.FeatureProperties)
	}
	if got := sf.FeatureProperties.HostedProcedureLink.Href; got != "/procedures/c360.semconnect.systems.csapi.procedure.alpha" {
		t.Errorf("hostedProcedure@link href: got %q", got)
	}
	var hasAssociation bool
	for _, l := range sf.Links {
		if l.Rel == "datastreams" || l.Rel == "controlstreams" {
			hasAssociation = true
		}
	}
	if !hasAssociation {
		t.Errorf("links missing samplingFeature association rel: %+v", sf.Links)
	}
}

func TestHandleSamplingFeature_NotASamplingFeatureKind(t *testing.T) {
	state := graph.EntityState{
		ID: testSamplingFeatureID,
		Triples: []message.Triple{
			{Subject: testSamplingFeatureID, Predicate: sensorml.PredType, Object: sosa.SSNSystem},
		},
	}
	fake := &fakeRequester{
		reply:  encodeEntityState(t, state),
		status: natsclient.StatusConnected,
	}
	c := newTestComponent(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/samplingFeatures/"+testSamplingFeatureID, nil)
	req.SetPathValue("id", testSamplingFeatureID)
	rr := httptest.NewRecorder()
	c.handleSamplingFeature(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status: got %d want 404", rr.Code)
	}
}

func TestHandleSamplingFeaturePost_Feature(t *testing.T) {
	fake := &fakeRequester{
		status: natsclient.StatusConnected,
		reply:  encodeBatchOK(t, 4),
	}
	c := newTestComponent(t, fake)

	body := []byte(`{"type":"Feature","geometry":{"type":"Point","coordinates":[5,10]},"properties":{"uid":"urn:example:sf:2","name":"SF2","hostedProcedure@link":{"href":"/procedures/c360.semconnect.systems.csapi.procedure.alpha"}}}`)
	req := httptest.NewRequest(http.MethodPost, "/samplingFeatures", bytes.NewReader(body))
	req.Header.Set("Content-Type", string(MediaGeoJSON))
	rr := httptest.NewRecorder()
	c.handleSamplingFeaturePost(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status: got %d want 201; body=%s", rr.Code, rr.Body.String())
	}
	loc := rr.Header().Get("Location")
	if !strings.HasPrefix(loc, "/samplingFeatures/"+c.cfg.SamplingFeatureIDPrefix+".") {
		t.Errorf("Location: got %q, want /samplingFeatures/<prefix>.<token>", loc)
	}

	var batch graph.AddTriplesBatchRequest
	if err := json.Unmarshal(fake.gotBody, &batch); err != nil {
		t.Fatalf("decode batch: %v", err)
	}
	var sawSampleType, sawUID, sawPosition, sawHostedProcedure bool
	for _, tr := range batch.Triples {
		if tr.Predicate == sensorml.PredType {
			if s, ok := tr.Object.(string); ok && s == sosa.Sample {
				sawSampleType = true
			}
		}
		if tr.Predicate == PredSystemUID {
			sawUID = true
		}
		if tr.Predicate == PredSystemPosition {
			sawPosition = true
		}
		if tr.Predicate == predSamplingFeatureHostedProcedure {
			sawHostedProcedure = true
			if tr.Datatype != "" {
				t.Errorf("hosted-procedure href must remain a literal, got datatype %q", tr.Datatype)
			}
		}
	}
	if !sawSampleType {
		t.Errorf("rdf:type should be sosa.Sample; batch=%+v", batch.Triples)
	}
	if !sawUID {
		t.Errorf("uid triple missing; batch=%+v", batch.Triples)
	}
	if !sawPosition {
		t.Errorf("position triple missing; batch=%+v", batch.Triples)
	}
	if !sawHostedProcedure {
		t.Errorf("hostedProcedure@link triple missing; batch=%+v", batch.Triples)
	}
}

func TestHandleSamplingFeatures_GeoJSON(t *testing.T) {
	fake := &multiReplyFakeRequester{
		predicateReply: encodeReply(t, []string{testSamplingFeatureID}),
		entityRepliesByID: map[string][]byte{
			testSamplingFeatureID: encodeEntityState(t, graph.EntityState{
				ID: testSamplingFeatureID,
				Triples: []message.Triple{
					{Subject: testSamplingFeatureID, Predicate: sensorml.PredType, Object: sosa.Sample},
					{Subject: testSamplingFeatureID, Predicate: PredSystemUID, Object: "urn:example:sf:1"},
					{Subject: testSamplingFeatureID, Predicate: predSamplingFeatureHostedProcedure, Object: "/procedures/c360.semconnect.systems.csapi.procedure.alpha"},
				},
			}),
		},
	}
	c := newComponentWithRequester(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/samplingFeatures", nil)
	req.Header.Set("Accept", string(MediaGeoJSON))
	rr := httptest.NewRecorder()
	c.handleSamplingFeatures(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); ct != string(MediaGeoJSON) {
		t.Errorf("Content-Type: got %q want %q", ct, MediaGeoJSON)
	}
	var fc struct {
		Type     string `json:"type"`
		Features []struct {
			Type       string         `json:"type"`
			ID         string         `json:"id"`
			Properties map[string]any `json:"properties"`
		} `json:"features"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &fc); err != nil {
		t.Fatalf("decode: %v; body=%s", err, rr.Body.String())
	}
	if fc.Type != "FeatureCollection" {
		t.Errorf("type: got %q", fc.Type)
	}
	if len(fc.Features) != 1 {
		t.Fatalf("features: got %d want 1", len(fc.Features))
	}
	if fc.Features[0].Properties["featureType"] != "SamplingFeature" {
		t.Errorf("featureType: got %v want SamplingFeature", fc.Features[0].Properties["featureType"])
	}
	if linkObj, ok := fc.Features[0].Properties["hostedProcedure@link"].(map[string]any); !ok || linkObj["href"] == "" {
		t.Errorf("hostedProcedure@link: got %#v want link object with href", fc.Features[0].Properties["hostedProcedure@link"])
	}
}

func TestHandleSamplingFeaturesOptions(t *testing.T) {
	fake := &fakeRequester{status: natsclient.StatusConnected}
	c := newTestComponent(t, fake)

	req := httptest.NewRequest(http.MethodOptions, "/samplingFeatures", nil)
	rr := httptest.NewRecorder()
	c.handleSamplingFeaturesOptions(rr, req)

	want := "GET, HEAD, POST, OPTIONS"
	if got := rr.Header().Get("Allow"); got != want {
		t.Errorf("Allow: got %q want %q", got, want)
	}
}
