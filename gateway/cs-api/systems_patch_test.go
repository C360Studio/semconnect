// Stage 19 — PATCH /systems/{id} tests. Re-uses the crdFakeRequester
// stub from systems_crd_test.go (same package) since PATCH shares
// the same NATS-subject fan-out as PUT: entity-query → per-predicate
// removes → add_batch.
//
// Mirrors the ETS UpdateTests.systemPatchLifecycle scenario:
// POST a Feature with uid+name → PATCH with `properties:
// {uid, name: <new>}` → GET back should show the new name with
// description/geometry/etc preserved.
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
	"github.com/c360studio/semstreams/parser/sensorml"
	"github.com/c360studio/semstreams/vocabulary/sosa"
)

const testPatchID = "acme.ops.robotics.gcs.drone.099"

// patchedSystemState mirrors existingSystemState (systems_crd_test.go)
// + adds a Stage 18 cs-api.system.uid triple and a Stage 14
// cs-api.system.position triple, so the merge logic has a full
// realistic triple set to preserve.
func patchedSystemState(id string) graph.EntityState {
	return graph.EntityState{
		ID: id,
		Triples: []message.Triple{
			{Subject: id, Predicate: sensorml.PredType, Object: sosa.SSNSystem},
			{Subject: id, Predicate: sensorml.PredLabel, Object: "Original name"},
			{Subject: id, Predicate: sensorml.PredDescription, Object: "Original description"},
			{Subject: id, Predicate: PredSystemUID, Object: "urn:ets:patch:42"},
			{Subject: id, Predicate: PredSystemPosition, Object: `{"type":"Point","coordinates":[10,20]}`},
		},
	}
}

func patchFeatureJSON(t *testing.T, props map[string]any) []byte {
	t.Helper()
	out, err := json.Marshal(map[string]any{
		"type":       "Feature",
		"properties": props,
	})
	if err != nil {
		t.Fatalf("marshal patch body: %v", err)
	}
	return out
}

// TestHandleSystemPatch_NameOnly — PATCH a single `properties.name`
// field. Description + geometry + uid triples must survive untouched.
func TestHandleSystemPatch_NameOnly(t *testing.T) {
	fake := &crdFakeRequester{
		entityReply: mustMarshal(t, patchedSystemState(testPatchID)),
		removeReply: encodeRemoveOK(t),
		batchReply:  encodeBatchOK(t, 5),
	}
	c := newComponentWithRequester(t, fake)

	body := patchFeatureJSON(t, map[string]any{
		"uid":  "urn:ets:patch:42",
		"name": "PATCHED name",
	})
	req := httptest.NewRequest(http.MethodPatch, "/systems/"+testPatchID, bytes.NewReader(body))
	req.SetPathValue("id", testPatchID)
	req.Header.Set("Content-Type", string(MediaJSON))
	rr := httptest.NewRecorder()
	c.handleSystemPatch(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status: got %d want 204; body=%s", rr.Code, rr.Body.String())
	}
	if fake.batchCount != 1 {
		t.Fatalf("add_batch calls: got %d want 1", fake.batchCount)
	}

	// Decode the published merged batch. The label triple should be the
	// PATCHED name; description, uid, position, type triples should
	// match the existing entity's values.
	var batch graph.AddTriplesBatchRequest
	if err := json.Unmarshal(fake.batchBody, &batch); err != nil {
		t.Fatalf("decode batch body: %v", err)
	}
	got := tripleObjectByPredicate(batch.Triples)
	if got[sensorml.PredLabel] != "PATCHED name" {
		t.Errorf("label triple: got %q want %q", got[sensorml.PredLabel], "PATCHED name")
	}
	if got[sensorml.PredDescription] != "Original description" {
		t.Errorf("description preserved: got %q want %q", got[sensorml.PredDescription], "Original description")
	}
	if got[PredSystemUID] != "urn:ets:patch:42" {
		t.Errorf("uid preserved: got %q want %q", got[PredSystemUID], "urn:ets:patch:42")
	}
	if got[PredSystemPosition] != `{"type":"Point","coordinates":[10,20]}` {
		t.Errorf("position preserved: got %q", got[PredSystemPosition])
	}
	if got[sensorml.PredType] != sosa.SSNSystem {
		t.Errorf("type preserved: got %q want %q", got[sensorml.PredType], sosa.SSNSystem)
	}
}

// TestHandleSystemPatch_AddsAbsentField — PATCH a field the entity
// didn't have (description). The merge appends rather than replaces.
func TestHandleSystemPatch_AddsAbsentField(t *testing.T) {
	state := graph.EntityState{
		ID: testPatchID,
		Triples: []message.Triple{
			{Subject: testPatchID, Predicate: sensorml.PredType, Object: sosa.SSNSystem},
			{Subject: testPatchID, Predicate: sensorml.PredLabel, Object: "Bare"},
		},
	}
	fake := &crdFakeRequester{
		entityReply: mustMarshal(t, state),
		removeReply: encodeRemoveOK(t),
		batchReply:  encodeBatchOK(t, 3),
	}
	c := newComponentWithRequester(t, fake)

	body := patchFeatureJSON(t, map[string]any{"description": "Added by PATCH"})
	req := httptest.NewRequest(http.MethodPatch, "/systems/"+testPatchID, bytes.NewReader(body))
	req.SetPathValue("id", testPatchID)
	req.Header.Set("Content-Type", string(MediaJSON))
	rr := httptest.NewRecorder()
	c.handleSystemPatch(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status: got %d want 204; body=%s", rr.Code, rr.Body.String())
	}

	var batch graph.AddTriplesBatchRequest
	_ = json.Unmarshal(fake.batchBody, &batch)
	got := tripleObjectByPredicate(batch.Triples)
	if got[sensorml.PredDescription] != "Added by PATCH" {
		t.Errorf("description: got %q want %q", got[sensorml.PredDescription], "Added by PATCH")
	}
	if got[sensorml.PredLabel] != "Bare" {
		t.Errorf("label preserved: got %q want %q", got[sensorml.PredLabel], "Bare")
	}
}

// TestHandleSystemPatch_EmptyBodyIsNoOp — empty Feature properties
// leaves every triple untouched (the merge just rewrites identical
// triples). 204 still returned.
func TestHandleSystemPatch_EmptyBodyIsNoOp(t *testing.T) {
	fake := &crdFakeRequester{
		entityReply: mustMarshal(t, patchedSystemState(testPatchID)),
		removeReply: encodeRemoveOK(t),
		batchReply:  encodeBatchOK(t, 5),
	}
	c := newComponentWithRequester(t, fake)

	body := patchFeatureJSON(t, map[string]any{})
	req := httptest.NewRequest(http.MethodPatch, "/systems/"+testPatchID, bytes.NewReader(body))
	req.SetPathValue("id", testPatchID)
	req.Header.Set("Content-Type", string(MediaJSON))
	rr := httptest.NewRecorder()
	c.handleSystemPatch(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status: got %d want 204; body=%s", rr.Code, rr.Body.String())
	}

	var batch graph.AddTriplesBatchRequest
	_ = json.Unmarshal(fake.batchBody, &batch)
	got := tripleObjectByPredicate(batch.Triples)
	if got[sensorml.PredLabel] != "Original name" {
		t.Errorf("label: got %q want %q", got[sensorml.PredLabel], "Original name")
	}
}

// TestHandleSystemPatch_UIDMismatch_400 — body uid that disagrees
// with the existing entity's uid triple is a 400 BEFORE any
// destructive operation.
func TestHandleSystemPatch_UIDMismatch_400(t *testing.T) {
	fake := &crdFakeRequester{
		entityReply: mustMarshal(t, patchedSystemState(testPatchID)),
		removeReply: encodeRemoveOK(t),
		batchReply:  encodeBatchOK(t, 5),
	}
	c := newComponentWithRequester(t, fake)

	body := patchFeatureJSON(t, map[string]any{"uid": "urn:ets:other:99", "name": "Hostile"})
	req := httptest.NewRequest(http.MethodPatch, "/systems/"+testPatchID, bytes.NewReader(body))
	req.SetPathValue("id", testPatchID)
	req.Header.Set("Content-Type", string(MediaJSON))
	rr := httptest.NewRecorder()
	c.handleSystemPatch(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status: got %d want 400; body=%s", rr.Code, rr.Body.String())
	}
	if len(fake.removeCalls) != 0 {
		t.Errorf("remove must not be called on uid mismatch; got %d calls", len(fake.removeCalls))
	}
	if fake.batchCount != 0 {
		t.Errorf("add_batch must not be called on uid mismatch; got %d calls", fake.batchCount)
	}
	// Pin the ordering invariant: entity-query DID happen (mismatch
	// is detected from fetched state, unlike PUT's pre-fetch
	// uid→id mint check). Mirrors the symmetry asymmetry deliberately:
	// PATCH must read existing to know what to compare.
	if fake.entityQueryCalls != 1 {
		t.Errorf("entity-query calls: got %d want 1 (PATCH mismatch detected post-fetch)", fake.entityQueryCalls)
	}
}

// TestHandleSystemPatch_PreStage18Entity_RejectsBodyUID — a body
// uid against an entity with no preserved uid triple is a 400
// (not silent acceptance). PATCH cannot establish identity
// retroactively without proof; re-POST/PUT is the right move.
func TestHandleSystemPatch_PreStage18Entity_RejectsBodyUID(t *testing.T) {
	preStage18State := graph.EntityState{
		ID: testPatchID,
		Triples: []message.Triple{
			{Subject: testPatchID, Predicate: sensorml.PredType, Object: sosa.SSNSystem},
			{Subject: testPatchID, Predicate: sensorml.PredLabel, Object: "Pre-Stage-18"},
			// no PredSystemUID
		},
	}
	fake := &crdFakeRequester{
		entityReply: mustMarshal(t, preStage18State),
		removeReply: encodeRemoveOK(t),
		batchReply:  encodeBatchOK(t, 3),
	}
	c := newComponentWithRequester(t, fake)

	body := patchFeatureJSON(t, map[string]any{"uid": "urn:client:bestguess:42", "name": "PATCHED"})
	req := httptest.NewRequest(http.MethodPatch, "/systems/"+testPatchID, bytes.NewReader(body))
	req.SetPathValue("id", testPatchID)
	req.Header.Set("Content-Type", string(MediaJSON))
	rr := httptest.NewRecorder()
	c.handleSystemPatch(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status: got %d want 400; body=%s", rr.Code, rr.Body.String())
	}
	if len(fake.removeCalls) != 0 {
		t.Errorf("remove must not run on missing-uid PATCH; got %d", len(fake.removeCalls))
	}
}

// TestHandleSystemPatch_GeometryNullIsNoOp — `"geometry": null` is
// treated as a no-op per the documented stance (RFC 7396 null-as-
// delete is NOT implemented at v0.1). Existing position triple must
// survive untouched.
func TestHandleSystemPatch_GeometryNullIsNoOp(t *testing.T) {
	fake := &crdFakeRequester{
		entityReply: mustMarshal(t, patchedSystemState(testPatchID)),
		removeReply: encodeRemoveOK(t),
		batchReply:  encodeBatchOK(t, 5),
	}
	c := newComponentWithRequester(t, fake)

	body := []byte(`{"type":"Feature","properties":{"name":"Just-rename"},"geometry":null}`)
	req := httptest.NewRequest(http.MethodPatch, "/systems/"+testPatchID, bytes.NewReader(body))
	req.SetPathValue("id", testPatchID)
	req.Header.Set("Content-Type", string(MediaJSON))
	rr := httptest.NewRecorder()
	c.handleSystemPatch(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status: got %d want 204; body=%s", rr.Code, rr.Body.String())
	}

	var batch graph.AddTriplesBatchRequest
	_ = json.Unmarshal(fake.batchBody, &batch)
	got := tripleObjectByPredicate(batch.Triples)
	if got[PredSystemPosition] != `{"type":"Point","coordinates":[10,20]}` {
		t.Errorf("position triple must survive geometry:null; got %q", got[PredSystemPosition])
	}
}

// TestHandleSystemPatch_GeoJSONContentType — PATCH with
// application/geo+json Content-Type is honored. Pins that the media-
// type guard accepts both Feature-shape encodings symmetrically with
// PUT/POST.
func TestHandleSystemPatch_GeoJSONContentType(t *testing.T) {
	fake := &crdFakeRequester{
		entityReply: mustMarshal(t, patchedSystemState(testPatchID)),
		removeReply: encodeRemoveOK(t),
		batchReply:  encodeBatchOK(t, 5),
	}
	c := newComponentWithRequester(t, fake)

	body := patchFeatureJSON(t, map[string]any{"name": "GeoJSON-typed PATCH"})
	req := httptest.NewRequest(http.MethodPatch, "/systems/"+testPatchID, bytes.NewReader(body))
	req.SetPathValue("id", testPatchID)
	req.Header.Set("Content-Type", string(MediaGeoJSON))
	rr := httptest.NewRecorder()
	c.handleSystemPatch(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status: got %d want 204 (geo+json accepted); body=%s", rr.Code, rr.Body.String())
	}
}

// TestHandleSystemPatch_MalformedJSON_400 — a body the JSON decoder
// rejects must produce a clean 400 with the error context (no
// literal `%s` slip-through).
func TestHandleSystemPatch_MalformedJSON_400(t *testing.T) {
	fake := &crdFakeRequester{}
	c := newComponentWithRequester(t, fake)

	body := []byte(`{"type":"Feature","properties":{"name":}`) // syntax error
	req := httptest.NewRequest(http.MethodPatch, "/systems/"+testPatchID, bytes.NewReader(body))
	req.SetPathValue("id", testPatchID)
	req.Header.Set("Content-Type", string(MediaJSON))
	rr := httptest.NewRecorder()
	c.handleSystemPatch(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status: got %d want 400; body=%s", rr.Code, rr.Body.String())
	}
	// The %s format-string bug would surface as the literal "%s" in
	// the response body. Pin against that regression.
	if strings.Contains(rr.Body.String(), "%s") {
		t.Errorf("response body contains literal %%s — format string bug; body=%s", rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "invalid JSON Feature") {
		t.Errorf("response body should mention 'invalid JSON Feature'; got %s", rr.Body.String())
	}
}

// TestHandleSystemPatch_NotFound — PATCH against a non-existent
// entity returns 404 (no upsert; partial-update of nothing is
// meaningless). PUT is the upsert path; PATCH is strict.
func TestHandleSystemPatch_NotFound(t *testing.T) {
	fake := &crdFakeRequester{
		entityReply: []byte("error: not found: " + testPatchID),
	}
	c := newComponentWithRequester(t, fake)

	body := patchFeatureJSON(t, map[string]any{"name": "new"})
	req := httptest.NewRequest(http.MethodPatch, "/systems/"+testPatchID, bytes.NewReader(body))
	req.SetPathValue("id", testPatchID)
	req.Header.Set("Content-Type", string(MediaJSON))
	rr := httptest.NewRecorder()
	c.handleSystemPatch(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status: got %d want 404; body=%s", rr.Code, rr.Body.String())
	}
}

// TestHandleSystemPatch_WrongContentType returns 415.
func TestHandleSystemPatch_WrongContentType(t *testing.T) {
	fake := &crdFakeRequester{}
	c := newComponentWithRequester(t, fake)

	req := httptest.NewRequest(http.MethodPatch, "/systems/"+testPatchID, bytes.NewReader([]byte(`{}`)))
	req.SetPathValue("id", testPatchID)
	req.Header.Set("Content-Type", "application/xml")
	rr := httptest.NewRecorder()
	c.handleSystemPatch(rr, req)

	if rr.Code != http.StatusUnsupportedMediaType {
		t.Errorf("status: got %d want 415; body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Header().Get("Accept"), string(MediaJSON)) {
		t.Errorf("Accept should advertise %s; got %q", MediaJSON, rr.Header().Get("Accept"))
	}
}

// TestHandleSystemPatch_OmittedTypeAccepted — some PATCH clients
// omit `type: "Feature"`. We tolerate the omission (path implies
// the type) but reject an explicit wrong type.
func TestHandleSystemPatch_OmittedTypeAccepted(t *testing.T) {
	fake := &crdFakeRequester{
		entityReply: mustMarshal(t, patchedSystemState(testPatchID)),
		removeReply: encodeRemoveOK(t),
		batchReply:  encodeBatchOK(t, 5),
	}
	c := newComponentWithRequester(t, fake)

	body := []byte(`{"properties":{"name":"Typeless PATCH"}}`)
	req := httptest.NewRequest(http.MethodPatch, "/systems/"+testPatchID, bytes.NewReader(body))
	req.SetPathValue("id", testPatchID)
	req.Header.Set("Content-Type", string(MediaJSON))
	rr := httptest.NewRecorder()
	c.handleSystemPatch(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status: got %d want 204; body=%s", rr.Code, rr.Body.String())
	}
}

// TestHandleSystemPatch_ViaMux pins that the PATCH route registration
// matches and PathValue resolves correctly through the real mux.
func TestHandleSystemPatch_ViaMux(t *testing.T) {
	fake := &crdFakeRequester{
		entityReply: mustMarshal(t, patchedSystemState(testPatchID)),
		removeReply: encodeRemoveOK(t),
		batchReply:  encodeBatchOK(t, 5),
	}
	c := newComponentWithRequester(t, fake)
	mux := http.NewServeMux()
	c.RegisterHTTPHandlers("", mux)

	body := patchFeatureJSON(t, map[string]any{"name": "Mux-routed PATCH"})
	req := httptest.NewRequest(http.MethodPatch, "/systems/"+testPatchID, bytes.NewReader(body))
	req.Header.Set("Content-Type", string(MediaJSON))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status: got %d want 204; body=%s", rr.Code, rr.Body.String())
	}
}

// tripleObjectByPredicate collapses a triple list into a
// predicate→object map for assertion ergonomics. Multiple triples
// sharing a predicate: last wins (production code dedups before
// emit, so collisions in test data are a test bug).
func tripleObjectByPredicate(triples []message.Triple) map[string]string {
	m := make(map[string]string, len(triples))
	for _, t := range triples {
		if s, ok := t.Object.(string); ok {
			m[t.Predicate] = s
		}
	}
	return m
}
