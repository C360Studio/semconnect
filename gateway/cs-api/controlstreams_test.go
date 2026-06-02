// Stage 24 — ControlStream handler tests.
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
)

const (
	testControlStreamID = "c360.semconnect.systems.csapi.controlstream.ptz"
	testControlSystemID = "c360.semconnect.systems.csapi.system.camera001"
)

func controlStreamState(t *testing.T) []byte {
	t.Helper()
	artifactID := "c360.semconnect.systems.csapi.schema." + uniqueIDToToken(testControlStreamID+"-commandSchema")
	propsBytes, _ := json.Marshal([]controlledProperty{{
		Definition: "http://sensorml.com/ont/swe/property/PanAngle",
		Label:      "Pan Angle",
	}})
	return encodeEntityState(t, graph.EntityState{
		ID: testControlStreamID,
		Triples: []message.Triple{
			{Subject: testControlStreamID, Predicate: sensorml.PredType, Object: ControlStreamTypeIRI},
			{Subject: testControlStreamID, Predicate: sensorml.PredLabel, Object: "PTZ Control"},
			{Subject: testControlStreamID, Predicate: PredControlStreamSystem, Object: testControlSystemID},
			{Subject: testControlStreamID, Predicate: predControlStreamInputName, Object: "ptz"},
			{Subject: testControlStreamID, Predicate: predControlStreamAsync, Object: "false"},
			{Subject: testControlStreamID, Predicate: predControlStreamCommandFormat, Object: string(MediaJSON)},
			{Subject: testControlStreamID, Predicate: predControlStreamSchema, Object: artifactID},
			{Subject: testControlStreamID, Predicate: predControlStreamControlledProperties, Object: string(propsBytes)},
		},
	})
}

func testCommandParametersSchema() json.RawMessage {
	return json.RawMessage(`{"type":"DataRecord","fields":[{"name":"pan","type":"Quantity","definition":"http://sensorml.com/ont/swe/property/PanAngle","label":"Pan Angle"}]}`)
}

func TestHandleControlStreams_GoldenPath(t *testing.T) {
	fake := &multiReplyFakeRequester{
		predicateReply: encodeReply(t, []string{testControlStreamID}),
		entityRepliesByID: map[string][]byte{
			testControlStreamID: controlStreamState(t),
		},
	}
	c := newComponentWithRequester(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/controlstreams", nil)
	rr := httptest.NewRecorder()
	c.handleControlStreams(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	var coll controlStreamCollection
	if err := json.Unmarshal(rr.Body.Bytes(), &coll); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(coll.Items) != 1 {
		t.Fatalf("items: got %d want 1", len(coll.Items))
	}
	if coll.Items[0].ID != testControlStreamID || coll.Items[0].InputName != "ptz" {
		t.Errorf("item: %+v", coll.Items[0])
	}
	if coll.Items[0].SystemID != testControlSystemID {
		t.Errorf("system@id: got %q", coll.Items[0].SystemID)
	}
	if fake.calls != 2 {
		t.Errorf("requests: got %d want 2 (1 predicate + 1 batch)", fake.calls)
	}
}

func TestHandleControlStream_JSON(t *testing.T) {
	fake := &fakeRequester{
		reply:  controlStreamState(t),
		status: natsclient.StatusConnected,
	}
	c := newTestComponent(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/controlstreams/"+testControlStreamID, nil)
	req.SetPathValue("id", testControlStreamID)
	rr := httptest.NewRecorder()
	c.handleControlStream(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	var cs controlStream
	if err := json.Unmarshal(rr.Body.Bytes(), &cs); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if cs.Name != "PTZ Control" || cs.InputName != "ptz" {
		t.Errorf("control stream: %+v", cs)
	}
	if len(cs.ControlledProperties) != 1 {
		t.Errorf("controlledProperties: %+v", cs.ControlledProperties)
	}
}

func TestHandleControlStreamSchema(t *testing.T) {
	fake := &multiReplyFakeRequester{entityRepliesByID: map[string][]byte{}}
	c := newComponentWithRequester(t, fake)
	store := &fakeSchemaObjectStore{}
	wireSchemaStore(c, store)
	artifactID := schemaArtifactIDForTest(c, testControlStreamID, predControlStreamSchema)
	fake.entityRepliesByID[testControlStreamID] = controlStreamState(t)
	fake.entityRepliesByID[artifactID] = seedSchemaArtifact(t, c, store, artifactID, testCommandParametersSchema())

	req := httptest.NewRequest(http.MethodGet, "/controlstreams/"+testControlStreamID+"/schema", nil)
	req.SetPathValue("id", testControlStreamID)
	rr := httptest.NewRecorder()
	c.handleControlStreamSchema(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	var schema commandSchema
	if err := json.Unmarshal(rr.Body.Bytes(), &schema); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if schema.CommandFormat != string(MediaJSON) {
		t.Errorf("commandFormat: got %q", schema.CommandFormat)
	}
	if schema.ParametersSchema["type"] != "DataRecord" {
		t.Errorf("parametersSchema: %+v", schema.ParametersSchema)
	}
}

func TestHandleControlStreamCommands_EmptyCollection(t *testing.T) {
	fake := &fakeRequester{
		reply:  controlStreamState(t),
		status: natsclient.StatusConnected,
	}
	c := newTestComponent(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/controlstreams/"+testControlStreamID+"/commands", nil)
	req.SetPathValue("id", testControlStreamID)
	rr := httptest.NewRecorder()
	c.handleControlStreamCommands(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"items":[]`) {
		t.Errorf("body: %s", rr.Body.String())
	}
}

func TestHandleCommands_EmptyCollection(t *testing.T) {
	c := newTestComponent(t, &fakeRequester{status: natsclient.StatusConnected})

	req := httptest.NewRequest(http.MethodGet, "/commands?limit=2", nil)
	rr := httptest.NewRecorder()
	c.handleCommands(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	var coll struct {
		Items []any  `json:"items"`
		Links []link `json:"links"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &coll); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(coll.Items) != 0 {
		t.Fatalf("items: got %+v want empty", coll.Items)
	}
	if len(coll.Links) != 1 || coll.Links[0].Href != "/commands" {
		t.Fatalf("links: %+v", coll.Links)
	}
}

func TestHandleCommands_BadLimit(t *testing.T) {
	c := newTestComponent(t, &fakeRequester{status: natsclient.StatusConnected})

	req := httptest.NewRequest(http.MethodGet, "/commands?limit=0", nil)
	rr := httptest.NewRecorder()
	c.handleCommands(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d want 400; body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleControlStreamPost_JSON(t *testing.T) {
	fake := &fakeRequester{
		status: natsclient.StatusConnected,
		reply:  encodeBatchOK(t, 7),
	}
	c := newTestComponent(t, fake)
	store := &fakeSchemaObjectStore{}
	wireSchemaStore(c, store)

	body := []byte(`{"name":"PTZ Control","system@id":"` + testControlSystemID + `","inputName":"ptz","async":false,"schema":{"commandFormat":"application/json","parametersSchema":{"type":"DataRecord","fields":[{"name":"pan","type":"Quantity","definition":"http://sensorml.com/ont/swe/property/PanAngle","label":"Pan Angle"}]}}}`)
	req := httptest.NewRequest(http.MethodPost, "/controlstreams", bytes.NewReader(body))
	req.Header.Set("Content-Type", string(MediaJSON))
	rr := httptest.NewRecorder()
	c.handleControlStreamPost(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status: got %d want 201; body=%s", rr.Code, rr.Body.String())
	}
	if loc := rr.Header().Get("Location"); !strings.HasPrefix(loc, "/controlstreams/"+c.cfg.ControlStreamIDPrefix+".") {
		t.Errorf("Location: got %q", loc)
	}
	var batch graph.AddTriplesBatchRequest
	if err := json.Unmarshal(fake.gotBody, &batch); err != nil {
		t.Fatalf("decode batch: %v", err)
	}
	var sawType, sawSystem, sawSchema, sawFormat bool
	var artifactID string
	for _, tr := range batch.Triples {
		switch tr.Predicate {
		case sensorml.PredType:
			sawType = tr.Object == ControlStreamTypeIRI
		case PredControlStreamSystem:
			sawSystem = true
		case predControlStreamCommandFormat:
			sawFormat = tr.Object == string(MediaJSON)
		case predControlStreamSchema:
			sawSchema = true
			artifactID, _ = tr.Object.(string)
		}
	}
	if !sawType || !sawSystem || !sawFormat || !sawSchema {
		t.Errorf("batch missing triples: type=%v system=%v format=%v schema=%v batch=%+v",
			sawType, sawSystem, sawFormat, sawSchema, batch.Triples)
	}
	stored := store.puts[schemaArtifactObjectKey(artifactID)]
	var params map[string]any
	if err := json.Unmarshal(stored, &params); err != nil {
		t.Fatalf("decode schema artifact bytes: %v", err)
	}
	fields, _ := params["fields"].([]any)
	if len(fields) != 1 {
		t.Fatalf("canonical schema fields: %+v", params)
	}
	field, _ := fields[0].(map[string]any)
	if field["type"] != "Quantity" {
		t.Fatalf("canonical field type: %+v", field)
	}
}

func TestHandleControlStreamPost_InvalidParametersSchema(t *testing.T) {
	fake := &fakeRequester{status: natsclient.StatusConnected}
	c := newTestComponent(t, fake)

	body := []byte(`{"name":"PTZ Control","inputName":"ptz","schema":{"commandFormat":"application/json","parametersSchema":{"type":"DataRecord","fields":[{"name":"pan","label":"Pan Angle"}]}}}`)
	req := httptest.NewRequest(http.MethodPost, "/controlstreams", bytes.NewReader(body))
	req.Header.Set("Content-Type", string(MediaJSON))
	rr := httptest.NewRecorder()
	c.handleControlStreamPost(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d want 400; body=%s", rr.Code, rr.Body.String())
	}
	if fake.gotSubject != "" {
		t.Fatalf("publish should not happen on invalid command schema; got %q", fake.gotSubject)
	}
}

func TestHandleSystemControlStreams_FiltersBySystem(t *testing.T) {
	otherID := "c360.semconnect.systems.csapi.controlstream.other"
	otherState := encodeEntityState(t, graph.EntityState{
		ID: otherID,
		Triples: []message.Triple{
			{Subject: otherID, Predicate: sensorml.PredType, Object: ControlStreamTypeIRI},
			{Subject: otherID, Predicate: sensorml.PredLabel, Object: "Other"},
			{Subject: otherID, Predicate: PredControlStreamSystem, Object: "c360.semconnect.systems.csapi.system.other"},
			{Subject: otherID, Predicate: predControlStreamInputName, Object: "other"},
		},
	})
	fake := &multiReplyFakeRequester{
		predicateReply: encodeReply(t, []string{testControlStreamID, otherID}),
		entityRepliesByID: map[string][]byte{
			testControlStreamID: controlStreamState(t),
			otherID:             otherState,
		},
	}
	c := newComponentWithRequester(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/systems/"+testControlSystemID+"/controlstreams", nil)
	req.SetPathValue("id", testControlSystemID)
	rr := httptest.NewRecorder()
	c.handleSystemControlStreams(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	var coll controlStreamCollection
	if err := json.Unmarshal(rr.Body.Bytes(), &coll); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(coll.Items) != 1 || coll.Items[0].ID != testControlStreamID {
		t.Errorf("items: %+v", coll.Items)
	}
}
