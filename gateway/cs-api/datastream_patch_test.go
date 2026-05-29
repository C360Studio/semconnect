// Stage 35 — PATCH /datastreams/{id} tests. Shares the CRD fake with
// PUT/DELETE because PATCH uses the same entity-query → remove fan-out
// → add-batch write path.
package csapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/parser/sensorml"
	"github.com/c360studio/semstreams/vocabulary/sosa"
)

func TestHandleDatastreamPatch_NameAndSchema(t *testing.T) {
	fake := &crdFakeRequester{
		entityReply: mustMarshal(t, existingDatastreamState(testDatastreamID)),
		removeReply: encodeRemoveOK(t),
		batchReply:  encodeBatchOK(t, 6),
	}
	c := newComponentWithRequester(t, fake)

	body, _ := json.Marshal(map[string]any{
		"name":   "PATCHED flow stream",
		"schema": json.RawMessage(testSWEDataRecordSchema),
	})
	req := httptest.NewRequest(http.MethodPatch, "/datastreams/"+testDatastreamID, bytes.NewReader(body))
	req.SetPathValue("id", testDatastreamID)
	req.Header.Set("Content-Type", string(MediaJSON))
	rr := httptest.NewRecorder()
	c.handleDatastreamPatch(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status: got %d want 204; body=%s", rr.Code, rr.Body.String())
	}
	if fake.batchCount != 1 {
		t.Fatalf("add_batch calls: got %d want 1", fake.batchCount)
	}
	if len(fake.removeCalls) == 0 {
		t.Fatalf("remove calls: got 0 want >0")
	}

	var batch graph.AddTriplesBatchRequest
	if err := json.Unmarshal(fake.batchBody, &batch); err != nil {
		t.Fatalf("decode batch body: %v", err)
	}
	got := tripleObjectByPredicate(batch.Triples)
	if got[sensorml.PredLabel] != "PATCHED flow stream" {
		t.Errorf("label triple: got %q", got[sensorml.PredLabel])
	}
	if got[sensorml.PredDescription] != "Old description" {
		t.Errorf("description preserved: got %q", got[sensorml.PredDescription])
	}
	if got[PredDatastreamSystem] != testSystemRef {
		t.Errorf("system preserved: got %q", got[PredDatastreamSystem])
	}
	if got[sosa.ObservedProperty] != "http://www.w3.org/ns/sosa/Property/Temperature" {
		t.Errorf("observedProperty preserved: got %q", got[sosa.ObservedProperty])
	}
	if got[PredDatastreamSchema] == "" {
		t.Fatalf("schema triple missing: %+v", batch.Triples)
	}
	if !json.Valid([]byte(got[PredDatastreamSchema])) {
		t.Fatalf("schema triple not JSON: %s", got[PredDatastreamSchema])
	}
}

func TestHandleDatastreamPatch_IDMismatch_400BeforeFetch(t *testing.T) {
	fake := &crdFakeRequester{
		entityReply: mustMarshal(t, existingDatastreamState(testDatastreamID)),
		removeReply: encodeRemoveOK(t),
		batchReply:  encodeBatchOK(t, 5),
	}
	c := newComponentWithRequester(t, fake)

	body := []byte(`{"id":"acme.ops.weather.station.flow.other","name":"Nope"}`)
	req := httptest.NewRequest(http.MethodPatch, "/datastreams/"+testDatastreamID, bytes.NewReader(body))
	req.SetPathValue("id", testDatastreamID)
	req.Header.Set("Content-Type", string(MediaJSON))
	rr := httptest.NewRecorder()
	c.handleDatastreamPatch(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d want 400; body=%s", rr.Code, rr.Body.String())
	}
	if fake.entityQueryCalls != 0 {
		t.Errorf("entity-query must not run on body/path mismatch; got %d", fake.entityQueryCalls)
	}
	if len(fake.removeCalls) != 0 {
		t.Errorf("remove must not run on mismatch; got %d", len(fake.removeCalls))
	}
	if fake.batchCount != 0 {
		t.Errorf("add_batch must not run on mismatch; got %d", fake.batchCount)
	}
}

func TestHandleDatastreamPatch_NotFoundNoUpsert(t *testing.T) {
	fake := &crdFakeRequester{
		entityReply: []byte("error: not found: " + testDatastreamID),
		removeReply: encodeRemoveOK(t),
		batchReply:  encodeBatchOK(t, 5),
	}
	c := newComponentWithRequester(t, fake)

	body := []byte(`{"name":"PATCHED"}`)
	req := httptest.NewRequest(http.MethodPatch, "/datastreams/"+testDatastreamID, bytes.NewReader(body))
	req.SetPathValue("id", testDatastreamID)
	req.Header.Set("Content-Type", string(MediaJSON))
	rr := httptest.NewRecorder()
	c.handleDatastreamPatch(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status: got %d want 404; body=%s", rr.Code, rr.Body.String())
	}
	if len(fake.removeCalls) != 0 {
		t.Errorf("remove must not run on not-found PATCH; got %d", len(fake.removeCalls))
	}
	if fake.batchCount != 0 {
		t.Errorf("add_batch must not run on not-found PATCH; got %d", fake.batchCount)
	}
}

func TestHandleDatastreamPatch_InvalidSchemaBeforeFetch(t *testing.T) {
	fake := &crdFakeRequester{}
	c := newComponentWithRequester(t, fake)

	body := []byte(`{"schema":{"type":"Quantity"}}`)
	req := httptest.NewRequest(http.MethodPatch, "/datastreams/"+testDatastreamID, bytes.NewReader(body))
	req.SetPathValue("id", testDatastreamID)
	req.Header.Set("Content-Type", string(MediaJSON))
	rr := httptest.NewRecorder()
	c.handleDatastreamPatch(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d want 400; body=%s", rr.Code, rr.Body.String())
	}
	if fake.entityQueryCalls != 0 {
		t.Errorf("entity-query must not run on invalid schema; got %d", fake.entityQueryCalls)
	}
	if fake.batchCount != 0 {
		t.Errorf("add_batch must not run on invalid schema; got %d", fake.batchCount)
	}
}

func TestHandleDatastreamPatch_NullSchemaRejected(t *testing.T) {
	fake := &crdFakeRequester{}
	c := newComponentWithRequester(t, fake)

	body := []byte(`{"schema":null}`)
	req := httptest.NewRequest(http.MethodPatch, "/datastreams/"+testDatastreamID, bytes.NewReader(body))
	req.SetPathValue("id", testDatastreamID)
	req.Header.Set("Content-Type", string(MediaJSON))
	rr := httptest.NewRecorder()
	c.handleDatastreamPatch(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d want 400; body=%s", rr.Code, rr.Body.String())
	}
	if fake.entityQueryCalls != 0 {
		t.Errorf("entity-query must not run on schema:null; got %d", fake.entityQueryCalls)
	}
}
