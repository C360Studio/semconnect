// Stage 35 — PATCH /datastreams/{id} tests. Shares the CRD fake with
// PUT/DELETE because PATCH uses the same entity-query →
// entity.update_with_triples write path.
package csapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/natsclient"
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
	store := &fakeSchemaObjectStore{}
	wireSchemaStore(c, store)

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
	if fake.batchCount != 2 {
		t.Fatalf("entity mutation calls: got %d want 2 (schema artifact + parent update)", fake.batchCount)
	}

	triples := updateTriplesFromBody(t, fake.batchBody)
	got := tripleObjectByPredicate(triples)
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
	artifactID := got[PredDatastreamSchema]
	if artifactID == "" {
		t.Fatalf("schema artifact relationship missing: %+v", triples)
	}
	if !strings.HasPrefix(artifactID, c.cfg.SchemaArtifactIDPrefix+".") {
		t.Fatalf("schema artifact ID: got %q want prefix %q", artifactID, c.cfg.SchemaArtifactIDPrefix)
	}
	if stored := store.puts[schemaArtifactObjectKey(artifactID)]; !json.Valid(stored) {
		t.Fatalf("schema artifact bytes are not JSON: %s", stored)
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
		t.Errorf("entity update must not run on mismatch; got %d", fake.batchCount)
	}
}

func TestHandleDatastreamPatch_NotFoundNoUpsert(t *testing.T) {
	replyBody, hdr := encodeClassifiedReply(
		t,
		natsclient.ErrorClassInvalid,
		graph.ErrorCodeEntityNotFound,
		"not found: "+testDatastreamID,
	)
	fake := &crdFakeRequester{
		entityReply:  replyBody,
		entityHeader: hdr,
		removeReply:  encodeRemoveOK(t),
		batchReply:   encodeBatchOK(t, 5),
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
		t.Errorf("entity update must not run on not-found PATCH; got %d", fake.batchCount)
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
		t.Errorf("entity update must not run on invalid schema; got %d", fake.batchCount)
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
