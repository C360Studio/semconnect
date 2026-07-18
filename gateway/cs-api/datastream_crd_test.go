// Stage 17 — CRD handler tests for /datastreams. Re-uses the
// crdFakeRequester stub from systems_crd_test.go (same package).
package csapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/c360studio/semconnect/parser/sensorml"
	csapivocab "github.com/c360studio/semconnect/vocabulary/csapi"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/nats-io/nats.go"
)

const (
	testDatastreamID = "acme.ops.weather.station.flow.alpha"
	testSystemRef    = "acme.ops.weather.station.deploy.alpha"
)

type fakeStreamCleaner struct {
	calls   int
	subject string
	err     error
}

func (f *fakeStreamCleaner) PurgeSubject(_ context.Context, subject string) error {
	f.calls++
	f.subject = subject
	return f.err
}

func wireObservationCleaner(c *Component, cleaner *fakeStreamCleaner) {
	var sc streamCleaner = cleaner
	c.cleaner.Store(&sc)
}

func minimalDatastreamJSON(id, system string) []byte {
	body := map[string]any{
		"name":             "Flow stream alpha",
		"description":      "Stage 17 test datastream",
		"system":           system,
		"observedProperty": "http://www.w3.org/ns/sosa/Property/Flow",
	}
	if id != "" {
		body["id"] = id
	}
	out, _ := json.Marshal(body)
	return out
}

func existingDatastreamState(id string) graph.EntityState {
	return graph.EntityState{
		ID: id,
		Triples: []message.Triple{
			{Subject: id, Predicate: sensorml.PredType, Object: DatastreamTypeIRI},
			{Subject: id, Predicate: sensorml.PredLabel, Object: "Old name"},
			{Subject: id, Predicate: sensorml.PredDescription, Object: "Old description"},
			{Subject: id, Predicate: PredDatastreamSystem, Object: testSystemRef, Datatype: message.EntityReferenceDatatype},
			{Subject: id, Predicate: csapivocab.ObservedProperty, Object: "http://www.w3.org/ns/sosa/Property/Temperature"},
		},
	}
}

// TestHandleDatastreamPut_GoldenPath — PUT replaces existing triples
// through entity.update_with_triples, returns 204.
func TestHandleDatastreamPut_GoldenPath(t *testing.T) {
	fake := &crdFakeRequester{
		entityReply: mustMarshal(t, existingDatastreamState(testDatastreamID)),
		removeReply: encodeRemoveOK(t),
		batchReply:  encodeBatchOK(t, 5),
	}
	c := newComponentWithRequester(t, fake)

	body := minimalDatastreamJSON(testDatastreamID, testSystemRef)
	req := httptest.NewRequest(http.MethodPut, "/datastreams/"+testDatastreamID, bytes.NewReader(body))
	req.SetPathValue("id", testDatastreamID)
	req.Header.Set("Content-Type", string(MediaJSON))
	rr := httptest.NewRecorder()
	c.handleDatastreamPut(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status: got %d want 204; body=%s", rr.Code, rr.Body.String())
	}
	if fake.batchCount != 1 {
		t.Errorf("entity update calls: got %d want 1", fake.batchCount)
	}
	if got := rr.Header().Get("X-CS-Attempted-ID"); got != testDatastreamID {
		t.Errorf("X-CS-Attempted-ID: got %q want %q", got, testDatastreamID)
	}
}

// TestHandleDatastreamPut_NoBodyID — empty body `id` is fine (path
// is authoritative); same upsert path.
func TestHandleDatastreamPut_NoBodyID(t *testing.T) {
	fake := &crdFakeRequester{
		entityReply: mustMarshal(t, existingDatastreamState(testDatastreamID)),
		removeReply: encodeRemoveOK(t),
		batchReply:  encodeBatchOK(t, 5),
	}
	c := newComponentWithRequester(t, fake)

	body := minimalDatastreamJSON("", testSystemRef)
	req := httptest.NewRequest(http.MethodPut, "/datastreams/"+testDatastreamID, bytes.NewReader(body))
	req.SetPathValue("id", testDatastreamID)
	req.Header.Set("Content-Type", string(MediaJSON))
	rr := httptest.NewRecorder()
	c.handleDatastreamPut(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status: got %d want 204; body=%s", rr.Code, rr.Body.String())
	}
}

// TestHandleDatastreamPut_PathBodyIDMismatch — body id != path id
// yields 400 *before* any destructive operation.
func TestHandleDatastreamPut_PathBodyIDMismatch(t *testing.T) {
	fake := &crdFakeRequester{
		entityReply: mustMarshal(t, existingDatastreamState(testDatastreamID)),
		removeReply: encodeRemoveOK(t),
		batchReply:  encodeBatchOK(t, 5),
	}
	c := newComponentWithRequester(t, fake)

	body := minimalDatastreamJSON("acme.ops.weather.station.flow.beta", testSystemRef)
	req := httptest.NewRequest(http.MethodPut, "/datastreams/"+testDatastreamID, bytes.NewReader(body))
	req.SetPathValue("id", testDatastreamID)
	req.Header.Set("Content-Type", string(MediaJSON))
	rr := httptest.NewRecorder()
	c.handleDatastreamPut(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status: got %d want 400; body=%s", rr.Code, rr.Body.String())
	}
	if len(fake.removeCalls) != 0 {
		t.Errorf("remove must not be called on mismatch; got %d calls", len(fake.removeCalls))
	}
	if fake.batchCount != 0 {
		t.Errorf("entity update must not be called on mismatch; got %d calls", fake.batchCount)
	}
	if fake.entityQueryCalls != 0 {
		t.Errorf("entity-query must not be called on mismatch; got %d calls", fake.entityQueryCalls)
	}
}

// TestHandleDatastreamPut_MissingSystem — body without required
// `system` field yields 400.
func TestHandleDatastreamPut_MissingSystem(t *testing.T) {
	fake := &crdFakeRequester{}
	c := newComponentWithRequester(t, fake)

	body := []byte(`{"id":"` + testDatastreamID + `","observedProperty":"http://example/p"}`)
	req := httptest.NewRequest(http.MethodPut, "/datastreams/"+testDatastreamID, bytes.NewReader(body))
	req.SetPathValue("id", testDatastreamID)
	req.Header.Set("Content-Type", string(MediaJSON))
	rr := httptest.NewRecorder()
	c.handleDatastreamPut(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status: got %d want 400; body=%s", rr.Code, rr.Body.String())
	}
}

// TestHandleDatastreamPut_WrongContentType returns 415.
func TestHandleDatastreamPut_WrongContentType(t *testing.T) {
	fake := &crdFakeRequester{}
	c := newComponentWithRequester(t, fake)

	body := []byte(`{}`)
	req := httptest.NewRequest(http.MethodPut, "/datastreams/"+testDatastreamID, bytes.NewReader(body))
	req.SetPathValue("id", testDatastreamID)
	req.Header.Set("Content-Type", "application/xml")
	rr := httptest.NewRecorder()
	c.handleDatastreamPut(rr, req)

	if rr.Code != http.StatusUnsupportedMediaType {
		t.Errorf("status: got %d want 415; body=%s", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("Accept"); got != string(MediaJSON) {
		t.Errorf("Accept: got %q want %q", got, MediaJSON)
	}
}

// TestHandleDatastreamPut_TransientUpdate — entity update fails with a
// NATS sentinel → 503 without partial erasure.
func TestHandleDatastreamPut_TransientUpdate(t *testing.T) {
	fake := &crdFakeRequester{
		entityReply: mustMarshal(t, existingDatastreamState(testDatastreamID)),
		batchErr:    nats.ErrNoResponders,
		batchReply:  encodeBatchOK(t, 5),
	}
	c := newComponentWithRequester(t, fake)

	body := minimalDatastreamJSON(testDatastreamID, testSystemRef)
	req := httptest.NewRequest(http.MethodPut, "/datastreams/"+testDatastreamID, bytes.NewReader(body))
	req.SetPathValue("id", testDatastreamID)
	req.Header.Set("Content-Type", string(MediaJSON))
	rr := httptest.NewRecorder()
	c.handleDatastreamPut(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status: got %d want 503; body=%s", rr.Code, rr.Body.String())
	}
	if fake.batchCount != 1 {
		t.Errorf("entity update calls: got %d want 1", fake.batchCount)
	}
	if got := rr.Header().Get("X-CS-Partial-Delete"); got != "" {
		t.Errorf("X-CS-Partial-Delete: got %q want empty", got)
	}
}

// TestHandleDatastreamDelete_GoldenPath — DELETE returns 204, deletes
// the graph entity, and purges the observation subject.
func TestHandleDatastreamDelete_GoldenPath(t *testing.T) {
	fake := &crdFakeRequester{
		entityReply: mustMarshal(t, existingDatastreamState(testDatastreamID)),
		removeReply: encodeRemoveOK(t),
	}
	c := newComponentWithRequester(t, fake)
	cleaner := &fakeStreamCleaner{}
	wireObservationCleaner(c, cleaner)

	req := httptest.NewRequest(http.MethodDelete, "/datastreams/"+testDatastreamID, nil)
	req.SetPathValue("id", testDatastreamID)
	rr := httptest.NewRecorder()
	c.handleDatastreamDelete(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status: got %d want 204; body=%s", rr.Code, rr.Body.String())
	}
	if got, want := len(fake.deleteCalls), 1; got != want {
		t.Errorf("delete call count: got %d want %d (calls=%+v)", got, want, fake.deleteCalls)
	}
	if got := fake.deleteCalls[0].EntityID; got != testDatastreamID {
		t.Errorf("delete.EntityID: got %q want %q", got, testDatastreamID)
	}
	if cleaner.calls != 1 {
		t.Fatalf("observation purge calls: got %d want 1", cleaner.calls)
	}
	if want := "cs-api.observations." + testDatastreamID; cleaner.subject != want {
		t.Errorf("purge subject: got %q want %q", cleaner.subject, want)
	}
}

// TestHandleDatastreamDelete_NotFound_Idempotent — same idempotent
// contract as DELETE /systems/{id}: 204 even when entity didn't exist.
// Observation subject purge still runs so the final state is clean for
// that id.
func TestHandleDatastreamDelete_NotFound_Idempotent(t *testing.T) {
	fake := &crdFakeRequester{
		removeReply: encodeRemoveOK(t),
	}
	c := newComponentWithRequester(t, fake)
	cleaner := &fakeStreamCleaner{}
	wireObservationCleaner(c, cleaner)

	req := httptest.NewRequest(http.MethodDelete, "/datastreams/"+testDatastreamID, nil)
	req.SetPathValue("id", testDatastreamID)
	rr := httptest.NewRecorder()
	c.handleDatastreamDelete(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status: got %d want 204 (idempotent); body=%s", rr.Code, rr.Body.String())
	}
	if len(fake.deleteCalls) != 1 {
		t.Errorf("delete should still be called for idempotent delete; got %d calls", len(fake.deleteCalls))
	}
	if cleaner.calls != 1 {
		t.Errorf("observation purge calls: got %d want 1", cleaner.calls)
	}
}

// TestHandleDatastreamDelete_InvalidID returns 400.
func TestHandleDatastreamDelete_InvalidID(t *testing.T) {
	fake := &crdFakeRequester{}
	c := newComponentWithRequester(t, fake)

	req := httptest.NewRequest(http.MethodDelete, "/datastreams/a..b", nil)
	req.SetPathValue("id", "a..b")
	rr := httptest.NewRecorder()
	c.handleDatastreamDelete(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status: got %d want 400; body=%s", rr.Code, rr.Body.String())
	}
}

// TestHandleDatastreamDelete_ObservationPurgeTransient reports the
// split-brain state: triples are gone, but stream cleanup failed.
func TestHandleDatastreamDelete_ObservationPurgeTransient(t *testing.T) {
	fake := &crdFakeRequester{
		entityReply: mustMarshal(t, existingDatastreamState(testDatastreamID)),
		removeReply: encodeRemoveOK(t),
	}
	c := newComponentWithRequester(t, fake)
	cleaner := &fakeStreamCleaner{err: nats.ErrNoResponders}
	wireObservationCleaner(c, cleaner)

	req := httptest.NewRequest(http.MethodDelete, "/datastreams/"+testDatastreamID, nil)
	req.SetPathValue("id", testDatastreamID)
	rr := httptest.NewRecorder()
	c.handleDatastreamDelete(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status: got %d want 503; body=%s", rr.Code, rr.Body.String())
	}
	if cleaner.calls != 1 {
		t.Errorf("observation purge calls: got %d want 1", cleaner.calls)
	}
	if got := rr.Header().Get("X-CS-Observation-Purge-Failed"); got != "true" {
		t.Errorf("X-CS-Observation-Purge-Failed: got %q want true", got)
	}
	if got := rr.Header().Get("X-CS-Partial-Delete"); got != "true" {
		t.Errorf("X-CS-Partial-Delete: got %q want true", got)
	}
}

// TestDeleteDatastream_AuditHeadersSymmetric — destructive deletes for
// datastream entities carry the same audit headers as POST.
func TestDeleteDatastream_AuditHeadersSymmetric(t *testing.T) {
	fake := &crdFakeRequester{
		removeReply: encodeRemoveOK(t),
	}
	c := newComponentWithRequester(t, fake)

	identity := Identity{Forwarded: map[string]string{"User": "alice"}}
	if err := c.deleteEntity(context.Background(), testDatastreamID, identity); err != nil {
		t.Fatalf("deleteEntity: %v", err)
	}
	if got := fake.deleteHeaders["X-CS-Forwarded-User"]; got != "alice" {
		t.Errorf("X-CS-Forwarded-User on delete: got %q want alice (headers=%+v)", got, fake.deleteHeaders)
	}
}

// TestHandleDatastreamsOptions advertises the collection-level Allow header.
func TestHandleDatastreamsOptions(t *testing.T) {
	fake := &crdFakeRequester{}
	c := newComponentWithRequester(t, fake)

	req := httptest.NewRequest(http.MethodOptions, "/datastreams", nil)
	rr := httptest.NewRecorder()
	c.handleDatastreamsOptions(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status: got %d want 204; body=%s", rr.Code, rr.Body.String())
	}
	want := "GET, HEAD, POST, OPTIONS"
	if got := rr.Header().Get("Allow"); got != want {
		t.Errorf("Allow: got %q want %q", got, want)
	}
}

// TestHandleDatastreamOptions advertises the item-level Allow header.
func TestHandleDatastreamOptions(t *testing.T) {
	fake := &crdFakeRequester{}
	c := newComponentWithRequester(t, fake)

	req := httptest.NewRequest(http.MethodOptions, "/datastreams/"+testDatastreamID, nil)
	req.SetPathValue("id", testDatastreamID)
	rr := httptest.NewRecorder()
	c.handleDatastreamOptions(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status: got %d want 204; body=%s", rr.Code, rr.Body.String())
	}
	want := "GET, HEAD, PUT, PATCH, DELETE, OPTIONS"
	if got := rr.Header().Get("Allow"); got != want {
		t.Errorf("Allow: got %q want %q", got, want)
	}
	if !strings.Contains(rr.Header().Get("Allow"), "PATCH") {
		t.Errorf("Allow must advertise PATCH at Stage 35+; got %q", rr.Header().Get("Allow"))
	}
}

// TestHandleDatastreamOptions_ViaMux — route registration in handlers.go
// matches the path pattern. PathValue("id") populated only via real mux.
func TestHandleDatastreamOptions_ViaMux(t *testing.T) {
	fake := &crdFakeRequester{}
	c := newComponentWithRequester(t, fake)
	mux := http.NewServeMux()
	c.RegisterHTTPHandlers("", mux)

	req := httptest.NewRequest(http.MethodOptions, "/datastreams/"+testDatastreamID, nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status: got %d want 204; body=%s", rr.Code, rr.Body.String())
	}
	want := "GET, HEAD, PUT, PATCH, DELETE, OPTIONS"
	if got := rr.Header().Get("Allow"); got != want {
		t.Errorf("Allow: got %q want %q", got, want)
	}
}
