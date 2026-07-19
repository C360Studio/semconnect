// Stage 24 — ControlStream handler tests.
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
	testControlStreamID = "c360.semconnect.systems.csapi.controlstream.ptz"
	testControlSystemID = "c360.semconnect.systems.csapi.system.camera001"
	testCommandID       = "c360.semconnect.systems.csapi.command.ptz001"
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
			{Subject: testControlStreamID, Predicate: PredControlStreamSystem, Object: testControlSystemID, Datatype: message.EntityReferenceDatatype},
			{Subject: testControlStreamID, Predicate: predControlStreamInputName, Object: "ptz"},
			{Subject: testControlStreamID, Predicate: predControlStreamAsync, Object: "false"},
			{Subject: testControlStreamID, Predicate: predControlStreamCommandFormat, Object: string(MediaJSON)},
			{Subject: testControlStreamID, Predicate: predControlStreamSchema, Object: artifactID, Datatype: message.EntityReferenceDatatype},
			{Subject: testControlStreamID, Predicate: predControlStreamControlledProperties, Object: string(propsBytes)},
			{Subject: testControlStreamID, Predicate: predControlStreamIssueTime, Object: "2026-06-02T18:00:00Z"},
			{Subject: testControlStreamID, Predicate: predControlStreamExecutionTime, Object: "2026-06-02T18:05:00Z"},
		},
	})
}

func testCommandParametersSchema() json.RawMessage {
	return json.RawMessage(`{"type":"DataRecord","fields":[{"name":"pan","type":"Quantity","definition":"http://sensorml.com/ont/swe/property/PanAngle","label":"Pan Angle"}]}`)
}

func commandState(t *testing.T) []byte {
	t.Helper()
	return encodeEntityState(t, graph.EntityState{
		ID: testCommandID,
		Triples: []message.Triple{
			{Subject: testCommandID, Predicate: sensorml.PredType, Object: CommandTypeIRI},
			{Subject: testCommandID, Predicate: PredCommandControlStream, Object: testControlStreamID, Datatype: message.EntityReferenceDatatype},
			{Subject: testCommandID, Predicate: predCommandStatus, Object: "accepted"},
			{Subject: testCommandID, Predicate: predCommandIssueTime, Object: "2026-05-19T12:00:00Z"},
			{Subject: testCommandID, Predicate: predCommandExecutionTime, Object: "2026-05-19T12:01:00Z"},
			{Subject: testCommandID, Predicate: predCommandSender, Object: "ets"},
			{Subject: testCommandID, Predicate: predCommandParams, Object: `{"pan":10}`},
		},
	})
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
	if cs.IssueTime != "2026-06-02T18:00:00Z" || cs.ExecutionTime != "2026-06-02T18:05:00Z" {
		t.Errorf("times: issue=%v execution=%v", cs.IssueTime, cs.ExecutionTime)
	}
}

func TestHandleControlStreams_AdvancedFilters(t *testing.T) {
	otherID := "c360.semconnect.systems.csapi.controlstream.other"
	propsBytes, _ := json.Marshal([]controlledProperty{{Definition: "http://example.org/control/zoom"}})
	otherState := encodeEntityState(t, graph.EntityState{
		ID: otherID,
		Triples: []message.Triple{
			{Subject: otherID, Predicate: sensorml.PredType, Object: ControlStreamTypeIRI},
			{Subject: otherID, Predicate: sensorml.PredLabel, Object: "Other Control"},
			{Subject: otherID, Predicate: predControlStreamInputName, Object: "other"},
			{Subject: otherID, Predicate: predControlStreamControlledProperties, Object: string(propsBytes)},
			{Subject: otherID, Predicate: predControlStreamIssueTime, Object: "2026-06-02T19:00:00Z"},
			{Subject: otherID, Predicate: predControlStreamExecutionTime, Object: "2026-06-02T19:05:00Z"},
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

	req := httptest.NewRequest(http.MethodGet, "/controlstreams?controlledProperty=http%3A%2F%2Fsensorml.com%2Font%2Fswe%2Fproperty%2FPanAngle&issueTime=2026-06-02T18%3A00%3A00Z&executionTime=2026-06-02T18%3A05%3A00Z", nil)
	rr := httptest.NewRecorder()
	c.handleControlStreams(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	var coll controlStreamCollection
	if err := json.Unmarshal(rr.Body.Bytes(), &coll); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(coll.Items) != 1 || coll.Items[0].ID != testControlStreamID {
		t.Fatalf("items: %+v", coll.Items)
	}
}

func TestHandleControlStream_ControlsCanonicalAlias(t *testing.T) {
	fake := &fakeRequester{
		reply:  controlStreamState(t),
		status: natsclient.StatusConnected,
	}
	c := newTestComponent(t, fake)
	mux := http.NewServeMux()
	c.RegisterHTTPHandlers("", mux)

	req := httptest.NewRequest(http.MethodGet, "/controls/"+testControlStreamID, nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	var cs controlStream
	if err := json.Unmarshal(rr.Body.Bytes(), &cs); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if cs.ID != testControlStreamID {
		t.Errorf("id: got %q want %q", cs.ID, testControlStreamID)
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

func TestHandleControlStreamCommands_ReturnsReferencingCommands(t *testing.T) {
	fake := &multiReplyFakeRequester{
		predicateReply: encodeReply(t, []string{testCommandID}),
		entityRepliesByID: map[string][]byte{
			testControlStreamID: controlStreamState(t),
			testCommandID:       commandState(t),
		},
	}
	c := newComponentWithRequester(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/controlstreams/"+testControlStreamID+"/commands", nil)
	req.SetPathValue("id", testControlStreamID)
	rr := httptest.NewRecorder()
	c.handleControlStreamCommands(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	var coll commandCollection
	if err := json.Unmarshal(rr.Body.Bytes(), &coll); err != nil {
		t.Fatalf("decode: %v; body=%s", err, rr.Body.String())
	}
	if len(coll.Items) != 1 || coll.Items[0].ID != testCommandID {
		t.Fatalf("items: %+v", coll.Items)
	}
	if coll.Items[0].ControlStreamID != testControlStreamID {
		t.Fatalf("controlstream@id: got %q want %q", coll.Items[0].ControlStreamID, testControlStreamID)
	}
}

func TestHandleCommands_ReturnsGlobalCommandCollection(t *testing.T) {
	fake := &multiReplyFakeRequester{
		predicateReply: encodeReply(t, []string{testCommandID}),
		entityRepliesByID: map[string][]byte{
			testCommandID: commandState(t),
		},
	}
	c := newComponentWithRequester(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/commands?limit=2", nil)
	rr := httptest.NewRecorder()
	c.handleCommands(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	var coll commandCollection
	if err := json.Unmarshal(rr.Body.Bytes(), &coll); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(coll.Items) != 1 || coll.Items[0].ID != testCommandID {
		t.Fatalf("items: %+v", coll.Items)
	}
	if len(coll.Links) != 1 || coll.Links[0].Href != "/commands" {
		t.Fatalf("links: %+v", coll.Links)
	}
}

func TestHandleCommand_JSON(t *testing.T) {
	fake := &fakeRequester{
		reply:  commandState(t),
		status: natsclient.StatusConnected,
	}
	c := newTestComponent(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/commands/"+testCommandID, nil)
	req.SetPathValue("id", testCommandID)
	rr := httptest.NewRecorder()
	c.handleCommand(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	var cmd command
	if err := json.Unmarshal(rr.Body.Bytes(), &cmd); err != nil {
		t.Fatalf("decode: %v; body=%s", err, rr.Body.String())
	}
	if cmd.ID != testCommandID || cmd.ControlStreamID != testControlStreamID {
		t.Fatalf("command: %+v", cmd)
	}
	if cmd.CurrentStatus != "accepted" {
		t.Fatalf("currentStatus: got %q", cmd.CurrentStatus)
	}
}

func TestHandleCommands_AdvancedFilters(t *testing.T) {
	otherID := "c360.semconnect.systems.csapi.command.other"
	otherState := encodeEntityState(t, graph.EntityState{
		ID: otherID,
		Triples: []message.Triple{
			{Subject: otherID, Predicate: sensorml.PredType, Object: CommandTypeIRI},
			{Subject: otherID, Predicate: PredCommandControlStream, Object: testControlStreamID, Datatype: message.EntityReferenceDatatype},
			{Subject: otherID, Predicate: predCommandStatus, Object: "failed"},
			{Subject: otherID, Predicate: predCommandIssueTime, Object: "2026-05-19T13:00:00Z"},
			{Subject: otherID, Predicate: predCommandExecutionTime, Object: "2026-05-19T13:01:00Z"},
			{Subject: otherID, Predicate: predCommandSender, Object: "other"},
		},
	})
	fake := &multiReplyFakeRequester{
		predicateReply: encodeReply(t, []string{testCommandID, otherID}),
		entityRepliesByID: map[string][]byte{
			testCommandID: commandState(t),
			otherID:       otherState,
		},
	}
	c := newComponentWithRequester(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/commands?issueTime=2026-05-19T12%3A00%3A00Z&executionTime=2026-05-19T12%3A01%3A00Z&statusCode=accepted&sender=ets", nil)
	rr := httptest.NewRecorder()
	c.handleCommands(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	var coll commandCollection
	if err := json.Unmarshal(rr.Body.Bytes(), &coll); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(coll.Items) != 1 || coll.Items[0].ID != testCommandID {
		t.Fatalf("items: %+v", coll.Items)
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

func TestHandleCommandPost_JSON(t *testing.T) {
	fake := &fakeRequester{
		status: natsclient.StatusConnected,
		reply:  encodeBatchOK(t, 9),
	}
	c := newTestComponent(t, fake)

	body := []byte(`{"id":"` + testCommandID + `","controlstream@id":"` + testControlStreamID + `","sender":"ets","params":{"pan":10}}`)
	req := httptest.NewRequest(http.MethodPost, "/commands", bytes.NewReader(body))
	req.Header.Set("Content-Type", string(MediaJSON))
	rr := httptest.NewRecorder()
	c.handleCommandPost(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status: got %d want 201; body=%s", rr.Code, rr.Body.String())
	}
	var batch graph.AddTriplesBatchRequest
	if err := json.Unmarshal(fake.gotBody, &batch); err != nil {
		t.Fatalf("decode batch: %v", err)
	}
	var sawType, sawControlStream, sawParams bool
	for _, tr := range batch.Triples {
		switch tr.Predicate {
		case sensorml.PredType:
			sawType = tr.Object == CommandTypeIRI
		case PredCommandControlStream:
			sawControlStream = tr.Object == testControlStreamID
		case predCommandParams:
			sawParams = tr.Object == `{"pan":10}`
		}
	}
	if !sawType || !sawControlStream || !sawParams {
		t.Errorf("batch missing triples: type=%v controlstream=%v params=%v batch=%+v",
			sawType, sawControlStream, sawParams, batch.Triples)
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
			{Subject: otherID, Predicate: PredControlStreamSystem, Object: "c360.semconnect.systems.csapi.system.other", Datatype: message.EntityReferenceDatatype},
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
