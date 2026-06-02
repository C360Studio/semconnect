// Stage 21 — Deployments handler tests. Re-uses fakeRequester /
// encodeReply / encodeEntityState helpers.
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

const testDeploymentID = "c360.semconnect.systems.csapi.deployment.alpha"

func TestHandleDeployments_GoldenPath(t *testing.T) {
	fake := &fakeRequester{
		reply:  encodeReply(t, []string{testDeploymentID}),
		status: natsclient.StatusConnected,
	}
	c := newTestComponent(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/deployments", nil)
	rr := httptest.NewRecorder()
	c.handleDeployments(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	var coll deploymentCollection
	if err := json.Unmarshal(rr.Body.Bytes(), &coll); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if coll.Type != "DeploymentCollection" || len(coll.Items) != 1 {
		t.Errorf("collection: type=%q items=%d", coll.Type, len(coll.Items))
	}
	// Pin predicate-query targets the SSN Deployment IRI, not
	// sosa.SSNSystem or sosa.Procedure.
	if !strings.Contains(string(fake.gotBody), ssnDeployment) {
		t.Errorf("predicate-query should reference %q; body=%s",
			ssnDeployment, string(fake.gotBody))
	}
}

func TestHandleDeployment_JSON(t *testing.T) {
	state := graph.EntityState{
		ID: testDeploymentID,
		Triples: []message.Triple{
			{Subject: testDeploymentID, Predicate: sensorml.PredType, Object: ssnDeployment},
			{Subject: testDeploymentID, Predicate: sensorml.PredLabel, Object: "Weather station deploy"},
			{Subject: testDeploymentID, Predicate: PredSystemUID, Object: "urn:example:deploy:1"},
			{Subject: testDeploymentID, Predicate: predDeploymentDeployedSystems, Object: "/systems/c360.semconnect.systems.csapi.system.alpha"},
			{Subject: testDeploymentID, Predicate: PredSystemPosition, Object: `{"type":"Point","coordinates":[5,10]}`},
		},
	}
	fake := &fakeRequester{
		reply:  encodeEntityState(t, state),
		status: natsclient.StatusConnected,
	}
	c := newTestComponent(t, fake)
	mux := http.NewServeMux()
	c.RegisterHTTPHandlers("", mux)

	req := httptest.NewRequest(http.MethodGet, "/deployments/"+testDeploymentID, nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	var d deployment
	if err := json.Unmarshal(rr.Body.Bytes(), &d); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if d.Type != "Deployment" {
		t.Errorf("Type: got %q want Deployment", d.Type)
	}
	if d.Label != "Weather station deploy" {
		t.Errorf("Label: got %q", d.Label)
	}
	if d.UID != "urn:example:deploy:1" {
		t.Errorf("UID: got %q", d.UID)
	}
	if string(d.Geometry) != `{"type":"Point","coordinates":[5,10]}` {
		t.Errorf("Geometry: got %q", string(d.Geometry))
	}
	if d.FeatureProperties == nil || len(d.FeatureProperties.DeployedSystemsLinks) != 1 {
		t.Fatalf("properties.deployedSystems@link missing: %+v", d.FeatureProperties)
	}
	if got := d.FeatureProperties.DeployedSystemsLinks[0].Href; got != "/systems/c360.semconnect.systems.csapi.system.alpha" {
		t.Errorf("deployedSystems@link href: got %q", got)
	}
	// Canonical link required.
	var hasCanonical, hasAssociation bool
	for _, l := range d.Links {
		if l.Rel == "canonical" {
			hasCanonical = true
		}
		if l.Rel == "samplingFeatures" || l.Rel == "datastreams" {
			hasAssociation = true
		}
	}
	if !hasCanonical {
		t.Errorf("links missing rel=canonical: %+v", d.Links)
	}
	if !hasAssociation {
		t.Errorf("links missing deployment association rel: %+v", d.Links)
	}
}

func TestHandleDeployment_NotADeploymentKind(t *testing.T) {
	state := graph.EntityState{
		ID: testDeploymentID,
		Triples: []message.Triple{
			{Subject: testDeploymentID, Predicate: sensorml.PredType, Object: sosa.SSNSystem},
		},
	}
	fake := &fakeRequester{
		reply:  encodeEntityState(t, state),
		status: natsclient.StatusConnected,
	}
	c := newTestComponent(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/deployments/"+testDeploymentID, nil)
	req.SetPathValue("id", testDeploymentID)
	rr := httptest.NewRecorder()
	c.handleDeployment(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status: got %d want 404", rr.Code)
	}
}

func TestHandleDeploymentPost_Feature(t *testing.T) {
	fake := &fakeRequester{
		status: natsclient.StatusConnected,
		reply:  encodeBatchOK(t, 4),
	}
	c := newTestComponent(t, fake)

	body := []byte(`{"type":"Feature","geometry":{"type":"Point","coordinates":[5,10]},"properties":{"uid":"urn:example:deploy:2","name":"D2","deployedSystems@link":[{"href":"/systems/c360.semconnect.systems.csapi.system.alpha"}]}}`)
	req := httptest.NewRequest(http.MethodPost, "/deployments", bytes.NewReader(body))
	req.Header.Set("Content-Type", string(MediaJSON))
	rr := httptest.NewRecorder()
	c.handleDeploymentPost(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status: got %d want 201; body=%s", rr.Code, rr.Body.String())
	}
	loc := rr.Header().Get("Location")
	if !strings.HasPrefix(loc, "/deployments/"+c.cfg.DeploymentIDPrefix+".") {
		t.Errorf("Location: got %q, want /deployments/<prefix>.<token>", loc)
	}

	var batch graph.AddTriplesBatchRequest
	if err := json.Unmarshal(fake.gotBody, &batch); err != nil {
		t.Fatalf("decode batch: %v", err)
	}
	var sawDeploymentType, sawUID, sawPosition, sawDeployedSystems bool
	for _, tr := range batch.Triples {
		if tr.Predicate == sensorml.PredType {
			if s, ok := tr.Object.(string); ok && s == ssnDeployment {
				sawDeploymentType = true
			}
		}
		if tr.Predicate == PredSystemUID {
			sawUID = true
		}
		if tr.Predicate == PredSystemPosition {
			sawPosition = true
		}
		if tr.Predicate == predDeploymentDeployedSystems {
			sawDeployedSystems = true
		}
	}
	if !sawDeploymentType {
		t.Errorf("rdf:type should be ssnDeployment; batch=%+v", batch.Triples)
	}
	if !sawUID {
		t.Errorf("uid triple missing; batch=%+v", batch.Triples)
	}
	if !sawPosition {
		t.Errorf("position triple missing — deployments DO carry geometry; batch=%+v", batch.Triples)
	}
	if !sawDeployedSystems {
		t.Errorf("deployedSystems@link triple missing; batch=%+v", batch.Triples)
	}
}

func TestHandleDeploymentPost_NoGeometry(t *testing.T) {
	fake := &fakeRequester{
		status: natsclient.StatusConnected,
		reply:  encodeBatchOK(t, 3),
	}
	c := newTestComponent(t, fake)

	body := []byte(`{"type":"Feature","properties":{"uid":"urn:example:deploy:3","name":"D3"}}`)
	req := httptest.NewRequest(http.MethodPost, "/deployments", bytes.NewReader(body))
	req.Header.Set("Content-Type", string(MediaJSON))
	rr := httptest.NewRecorder()
	c.handleDeploymentPost(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status: got %d want 201; body=%s", rr.Code, rr.Body.String())
	}
	var batch graph.AddTriplesBatchRequest
	_ = json.Unmarshal(fake.gotBody, &batch)
	for _, tr := range batch.Triples {
		if tr.Predicate == PredSystemPosition {
			t.Errorf("position triple should be absent when body has no geometry; got %+v", tr)
		}
	}
}

func TestHandleDeploymentPost_WrongContentType(t *testing.T) {
	fake := &fakeRequester{status: natsclient.StatusConnected}
	c := newTestComponent(t, fake)

	req := httptest.NewRequest(http.MethodPost, "/deployments", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/sml+json") // SensorML not accepted
	rr := httptest.NewRecorder()
	c.handleDeploymentPost(rr, req)

	if rr.Code != http.StatusUnsupportedMediaType {
		t.Errorf("status: got %d want 415; body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleDeploymentsOptions(t *testing.T) {
	fake := &fakeRequester{status: natsclient.StatusConnected}
	c := newTestComponent(t, fake)

	req := httptest.NewRequest(http.MethodOptions, "/deployments", nil)
	rr := httptest.NewRecorder()
	c.handleDeploymentsOptions(rr, req)

	want := "GET, HEAD, POST, OPTIONS"
	if got := rr.Header().Get("Allow"); got != want {
		t.Errorf("Allow: got %q want %q", got, want)
	}
}

func TestHandleDeployments_GeoJSON(t *testing.T) {
	// Predicate-query reply: 1 deployment ID. The batch hydration reply
	// omits that entity, so the geo+json path degrades to null geometry.
	fake := &multiReplyFakeRequester{
		predicateReply: encodeReply(t, []string{testDeploymentID}),
		entityRepliesByID: map[string][]byte{
			testDeploymentID: encodeEntityState(t, graph.EntityState{
				ID: testDeploymentID,
				Triples: []message.Triple{
					{Subject: testDeploymentID, Predicate: sensorml.PredType, Object: ssnDeployment},
					{Subject: testDeploymentID, Predicate: PredSystemUID, Object: "urn:example:deploy:1"},
					{Subject: testDeploymentID, Predicate: predDeploymentDeployedSystems, Object: "/systems/c360.semconnect.systems.csapi.system.alpha"},
				},
			}),
		},
	}
	c := newComponentWithRequester(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/deployments", nil)
	req.Header.Set("Accept", string(MediaGeoJSON))
	rr := httptest.NewRecorder()
	c.handleDeployments(rr, req)

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
	if fc.Features[0].Properties["featureType"] != "Deployment" {
		t.Errorf("featureType: got %v want Deployment", fc.Features[0].Properties["featureType"])
	}
	if links, ok := fc.Features[0].Properties["deployedSystems@link"].([]any); !ok || len(links) != 1 {
		t.Errorf("deployedSystems@link: got %#v want one link", fc.Features[0].Properties["deployedSystems@link"])
	}
}
