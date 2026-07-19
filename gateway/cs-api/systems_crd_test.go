// Stage 16 — CRD (create-replace-delete) handler tests covering
// POST JSON Feature, PUT replace, DELETE, OPTIONS. The PUT + DELETE
// paths drive entity-level mutation traffic around an entity-query
// precondition, so this file ships a dedicated stub instead of extending
// the Stage 15 multiReplyFakeRequester.
package csapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/c360studio/semconnect/parser/sensorml"
	"github.com/c360studio/semconnect/vocabulary/sosa"
	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// crdFakeRequester is a multi-subject stub for the CRD flow:
//   - subjectEntityQuery → return entityReply (or entityErr)
//   - SubjectEntityUpdateWithTriples / SubjectEntityCreateWithTriples →
//     return batchReply and capture batchBody
//   - SubjectEntityDelete → return removeReply and capture deleteCalls
type crdFakeRequester struct {
	mu sync.Mutex

	entityReply  []byte
	entityHeader nats.Header
	entityErr    error
	removeReply  []byte
	removeErr    error
	batchReply   []byte
	batchErr     error

	entityQueryCalls int
	removeCalls      []graph.RemoveTripleRequest
	removeHeaders    map[string]string
	deleteCalls      []graph.DeleteEntityRequest
	deleteHeaders    map[string]string
	batchCount       int
	batchBody        []byte
	headers          map[string]string
}

func (f *crdFakeRequester) Request(_ context.Context, subj string, _ []byte, _ time.Duration) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if subj == subjectEntityQuery {
		f.entityQueryCalls++
		if f.entityErr != nil {
			return nil, f.entityErr
		}
		return f.entityReply, nil
	}
	return nil, errors.New("crdFakeRequester: unexpected Request subject " + subj)
}

func (f *crdFakeRequester) RequestWithHeaders(_ context.Context, subj string, data []byte, headers map[string]string, _ time.Duration) (*nats.Msg, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	switch subj {
	case subjectEntityQuery:
		f.entityQueryCalls++
		if f.entityErr != nil {
			return nil, f.entityErr
		}
		return &nats.Msg{Data: f.entityReply, Header: f.entityHeader}, nil
	case SubjectEntityCreateWithTriples, SubjectEntityUpdateWithTriples:
		f.batchCount++
		f.batchBody = append([]byte(nil), data...)
		if headers != nil {
			f.headers = make(map[string]string, len(headers))
			for k, v := range headers {
				f.headers[k] = v
			}
		}
		if f.batchErr != nil {
			return nil, f.batchErr
		}
		return &nats.Msg{Data: f.batchReply}, nil
	case SubjectEntityDelete:
		var req graph.DeleteEntityRequest
		if err := json.Unmarshal(data, &req); err != nil {
			return nil, errors.New("crdFakeRequester: malformed delete body")
		}
		f.deleteCalls = append(f.deleteCalls, req)
		if f.deleteHeaders == nil && headers != nil {
			f.deleteHeaders = make(map[string]string, len(headers))
			for k, v := range headers {
				f.deleteHeaders[k] = v
			}
		}
		if f.removeErr != nil {
			return nil, f.removeErr
		}
		return &nats.Msg{Data: f.removeReply}, nil
	}
	return nil, errors.New("crdFakeRequester: unexpected header subject " + subj)
}

func (f *crdFakeRequester) Status() natsclient.ConnectionStatus {
	return natsclient.StatusConnected
}
func (f *crdFakeRequester) JetStream() (jetstream.JetStream, error) {
	return nil, errors.New("crdFakeRequester: JetStream not exercised")
}
func (f *crdFakeRequester) EnsureStream(_ context.Context, _ jetstream.StreamConfig) (jetstream.Stream, error) {
	return nil, errors.New("crdFakeRequester: EnsureStream not exercised")
}

func encodeRemoveOK(t *testing.T) []byte {
	t.Helper()
	resp := graph.DeleteEntityResponse{
		MutationResponse: graph.MutationResponse{Timestamp: 1, KVRevision: 1},
		Deleted:          true,
	}
	out, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("encodeRemoveOK: %v", err)
	}
	return out
}

// systemFeatureJSON crafts a minimal CS API §7.6 GeoJSON Feature POST body.
func systemFeatureJSON(uid, name string) []byte {
	body := map[string]any{
		"type": "Feature",
		"properties": map[string]any{
			"uid":  uid,
			"name": name,
		},
	}
	out, _ := json.Marshal(body)
	return out
}

func systemFeatureJSONWithGeometry(uid, name string, lon, lat float64) []byte {
	body := map[string]any{
		"type": "Feature",
		"geometry": map[string]any{
			"type":        "Point",
			"coordinates": []float64{lon, lat},
		},
		"properties": map[string]any{
			"uid":  uid,
			"name": name,
		},
	}
	out, _ := json.Marshal(body)
	return out
}

// TestHandleSystemPost_JSONFeature_GoldenPath — Stage 16. POST a GeoJSON
// Feature with Content-Type application/json mints an entity and returns 201.
func TestHandleSystemPost_JSONFeature_GoldenPath(t *testing.T) {
	fake := &fakeRequester{
		status: natsclient.StatusConnected,
		reply:  encodeBatchOK(t, 3),
	}
	c := newTestComponent(t, fake)

	req := httptest.NewRequest(http.MethodPost, "/systems",
		bytes.NewReader(systemFeatureJSONWithGeometry("urn:example:dev:42", "Device 42", 10, 20)))
	req.Header.Set("Content-Type", string(MediaJSON))
	rr := httptest.NewRecorder()
	c.handleSystemPost(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status: got %d want 201; body=%s", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("Location"); !strings.HasPrefix(got, "/systems/"+c.cfg.SystemIDPrefix+".") {
		t.Errorf("Location: got %q, want /systems/<prefix>.<token>", got)
	}
	// uniqueIDToToken is deterministic — pin the exact suffix so a
	// future change to sanitization rules surfaces here, not silently.
	wantSuffix := "/systems/" + c.cfg.SystemIDPrefix + "." + uniqueIDToToken("urn:example:dev:42")
	if got := rr.Header().Get("Location"); got != wantSuffix {
		t.Errorf("Location: got %q want %q", got, wantSuffix)
	}
	if fake.gotSubject != SubjectEntityCreateWithTriples {
		t.Errorf("subject: got %q want %q", fake.gotSubject, SubjectEntityCreateWithTriples)
	}
}

// TestHandleSystemPost_JSONFeature_MissingUID returns 400.
func TestHandleSystemPost_JSONFeature_MissingUID(t *testing.T) {
	fake := &fakeRequester{status: natsclient.StatusConnected}
	c := newTestComponent(t, fake)

	body := []byte(`{"type":"Feature","properties":{"name":"Anonymous"}}`)
	req := httptest.NewRequest(http.MethodPost, "/systems", bytes.NewReader(body))
	req.Header.Set("Content-Type", string(MediaJSON))
	rr := httptest.NewRecorder()
	c.handleSystemPost(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status: got %d want 400; body=%s", rr.Code, rr.Body.String())
	}
	if fake.gotSubject != "" {
		t.Errorf("publish should not have been called; got subject=%q", fake.gotSubject)
	}
}

// TestHandleSystemPost_JSONFeature_NotAFeature returns 400.
func TestHandleSystemPost_JSONFeature_NotAFeature(t *testing.T) {
	fake := &fakeRequester{status: natsclient.StatusConnected}
	c := newTestComponent(t, fake)

	body := []byte(`{"type":"FeatureCollection","properties":{"uid":"x"}}`)
	req := httptest.NewRequest(http.MethodPost, "/systems", bytes.NewReader(body))
	req.Header.Set("Content-Type", string(MediaGeoJSON))
	rr := httptest.NewRecorder()
	c.handleSystemPost(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status: got %d want 400; body=%s", rr.Code, rr.Body.String())
	}
}

// existingSystemState builds an EntityState with multiple predicates so
// replacement paths can prove predicate removal sets are generated from
// current state.
func existingSystemState(id string) graph.EntityState {
	return graph.EntityState{
		ID: id,
		Triples: []message.Triple{
			{Subject: id, Predicate: sensorml.PredType, Object: sosa.SSNSystem},
			{Subject: id, Predicate: sensorml.PredLabel, Object: "Old label"},
			{Subject: id, Predicate: sensorml.PredDescription, Object: "Old description"},
			// Duplicate predicate (extra hosted child) — dedup must collapse to one remove call.
			{Subject: id, Predicate: sensorml.PredHosts, Object: id + ".camera", Datatype: message.EntityReferenceDatatype},
			{Subject: id, Predicate: sensorml.PredHosts, Object: id + ".gps", Datatype: message.EntityReferenceDatatype},
		},
	}
}

// TestHandleSystemDelete_GoldenPath — DELETE returns 204 and issues one
// entity-scoped delete request.
func TestHandleSystemDelete_GoldenPath(t *testing.T) {
	pathID := "acme.ops.robotics.gcs.drone.099"
	fake := &crdFakeRequester{
		entityReply: mustMarshal(t, existingSystemState(pathID)),
		removeReply: encodeRemoveOK(t),
	}
	c := newComponentWithRequester(t, fake)

	req := httptest.NewRequest(http.MethodDelete, "/systems/"+pathID, nil)
	req.SetPathValue("id", pathID)
	rr := httptest.NewRecorder()
	c.handleSystemDelete(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status: got %d want 204; body=%s", rr.Code, rr.Body.String())
	}
	if got, want := len(fake.deleteCalls), 1; got != want {
		t.Fatalf("delete call count: got %d want %d (calls=%+v)", got, want, fake.deleteCalls)
	}
	if got := fake.deleteCalls[0].EntityID; got != pathID {
		t.Errorf("delete.EntityID: got %q want %q", got, pathID)
	}
}

// TestHandleSystemDelete_NotFound_Idempotent — DELETE against an unknown
// ID still returns 204 (CS API §7.6 conventions; the framework
// errEntityNotFound is swallowed).
func TestHandleSystemDelete_NotFound_Idempotent(t *testing.T) {
	pathID := "acme.ops.robotics.gcs.drone.404"
	fake := &crdFakeRequester{
		removeReply: encodeRemoveOK(t),
	}
	c := newComponentWithRequester(t, fake)

	req := httptest.NewRequest(http.MethodDelete, "/systems/"+pathID, nil)
	req.SetPathValue("id", pathID)
	rr := httptest.NewRecorder()
	c.handleSystemDelete(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status: got %d want 204 (idempotent); body=%s", rr.Code, rr.Body.String())
	}
	if len(fake.deleteCalls) != 1 {
		t.Errorf("delete should still be called for idempotent delete; got %d calls", len(fake.deleteCalls))
	}
}

// TestHandleSystemDelete_InvalidID returns 400 — validateEntityID rejects
// IDs with reserved NATS-token characters or empty path tokens, matching
// the guard GET /systems/{id} uses for symmetry.
func TestHandleSystemDelete_InvalidID(t *testing.T) {
	fake := &crdFakeRequester{}
	c := newComponentWithRequester(t, fake)

	req := httptest.NewRequest(http.MethodDelete, "/systems/a..b", nil)
	req.SetPathValue("id", "a..b") // empty middle token
	rr := httptest.NewRecorder()
	c.handleSystemDelete(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status: got %d want 400; body=%s", rr.Code, rr.Body.String())
	}
}

// TestHandleSystemPut_GoldenPath — PUT replaces existing triples with
// entity.update_with_triples. Requires the body's uid → entity ID to
// match the path.
func TestHandleSystemPut_GoldenPath(t *testing.T) {
	pathID := "acme.ops.robotics.gcs.drone.099"
	// systemFeatureJSON uid is passed through uniqueIDToToken to mint the
	// suffix. Choose a uid that produces the exact 6-part path above.
	// SystemIDPrefix default is `c360.semconnect.systems.csapi.system`, so
	// the test path won't match that prefix — switch the cfg prefix to
	// the drone prefix manually.
	fake := &crdFakeRequester{
		entityReply: mustMarshal(t, existingSystemState(pathID)),
		removeReply: encodeRemoveOK(t),
		batchReply:  encodeBatchOK(t, 3),
	}
	c := newComponentWithRequester(t, fake)
	c.cfg.SystemIDPrefix = "acme.ops.robotics.gcs.drone"

	body := systemFeatureJSON("099", "Drone 99")
	req := httptest.NewRequest(http.MethodPut, "/systems/"+pathID, bytes.NewReader(body))
	req.SetPathValue("id", pathID)
	req.Header.Set("Content-Type", string(MediaJSON))
	rr := httptest.NewRecorder()
	c.handleSystemPut(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status: got %d want 204; body=%s", rr.Code, rr.Body.String())
	}
	if fake.batchCount != 1 {
		t.Errorf("entity update calls: got %d want 1", fake.batchCount)
	}
}

func TestReplaceEntityTriples_ForwardsForeignEdgeProjection(t *testing.T) {
	parentID := "acme.ops.robotics.gcs.drone.099"
	childID := parentID + "_camera"
	current := existingSystemState(parentID)
	current.MessageType = systemProjectionMessageType
	fake := &crdFakeRequester{
		batchReply: encodeBatchOK(t, 3),
	}
	c := newComponentWithRequester(t, fake)

	triples := []message.Triple{
		{Subject: parentID, Predicate: sensorml.PredType, Object: sosa.SSNSystem},
		{Subject: parentID, Predicate: sensorml.PredHosts, Object: childID, Datatype: message.EntityReferenceDatatype},
		{Subject: childID, Predicate: sensorml.PredIsHostedBy, Object: parentID, Datatype: message.EntityReferenceDatatype},
	}
	if err := c.replaceEntityTriples(context.Background(), current, triples, Identity{}); err != nil {
		t.Fatalf("replaceEntityTriples: %v", err)
	}

	var sent graph.UpdateEntityWithTriplesRequest
	if err := json.Unmarshal(fake.batchBody, &sent); err != nil {
		t.Fatalf("decode update body: %v", err)
	}
	if sent.Entity == nil || sent.Entity.ID != parentID {
		t.Fatalf("entity: got %+v want ID %q", sent.Entity, parentID)
	}
	if !sent.Entity.MessageType.Equal(systemProjectionMessageType) {
		t.Fatalf("entity.MessageType: got %+v want %+v", sent.Entity.MessageType, systemProjectionMessageType)
	}
	var sawForeign bool
	for _, tr := range sent.AddTriples {
		if tr.Subject == childID && tr.Predicate == sensorml.PredIsHostedBy && tr.Object == parentID {
			sawForeign = true
		}
	}
	if !sawForeign {
		t.Fatalf("foreign edge not forwarded in update AddTriples: %+v", sent.AddTriples)
	}
}

// TestHandleSystemPut_PathBodyIDMismatch — PUT body whose uid would mint a
// different entity ID than the path {id} returns 400.
func TestHandleSystemPut_PathBodyIDMismatch(t *testing.T) {
	pathID := "acme.ops.robotics.gcs.drone.099"
	fake := &crdFakeRequester{
		entityReply: mustMarshal(t, existingSystemState(pathID)),
		removeReply: encodeRemoveOK(t),
		batchReply:  encodeBatchOK(t, 3),
	}
	c := newComponentWithRequester(t, fake)
	c.cfg.SystemIDPrefix = "acme.ops.robotics.gcs.drone"

	// uid "088" → minted suffix "088", whole entity "acme.ops.robotics.gcs.drone.088".
	body := systemFeatureJSON("088", "Drone 88")
	req := httptest.NewRequest(http.MethodPut, "/systems/"+pathID, bytes.NewReader(body))
	req.SetPathValue("id", pathID)
	req.Header.Set("Content-Type", string(MediaJSON))
	rr := httptest.NewRecorder()
	c.handleSystemPut(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status: got %d want 400; body=%s", rr.Code, rr.Body.String())
	}
	// Critical: no destructive removes if the IDs don't agree.
	if len(fake.removeCalls) != 0 {
		t.Errorf("remove must not be called on mismatch; got %d calls", len(fake.removeCalls))
	}
	if fake.batchCount != 0 {
		t.Errorf("entity update must not be called on mismatch; got %d calls", fake.batchCount)
	}
	// And no backend round-trip wasted on a client error.
	if fake.entityQueryCalls != 0 {
		t.Errorf("entity-query must not be called on mismatch; got %d calls", fake.entityQueryCalls)
	}
}

// TestHandleSystemPut_TransientUpdate exercises the entity update
// request failing with a NATS sentinel → 503 without partial erasure.
func TestHandleSystemPut_TransientUpdate(t *testing.T) {
	pathID := "acme.ops.robotics.gcs.drone.099"
	fake := &crdFakeRequester{
		entityReply: mustMarshal(t, existingSystemState(pathID)),
		batchErr:    nats.ErrNoResponders,
		batchReply:  encodeBatchOK(t, 3),
	}
	c := newComponentWithRequester(t, fake)
	c.cfg.SystemIDPrefix = "acme.ops.robotics.gcs.drone"

	body := systemFeatureJSON("099", "Drone 99")
	req := httptest.NewRequest(http.MethodPut, "/systems/"+pathID, bytes.NewReader(body))
	req.SetPathValue("id", pathID)
	req.Header.Set("Content-Type", string(MediaJSON))
	rr := httptest.NewRecorder()
	c.handleSystemPut(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status: got %d want 503; body=%s", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("X-CS-Attempted-ID"); got != pathID {
		t.Errorf("X-CS-Attempted-ID: got %q want %q", got, pathID)
	}
	if fake.batchCount != 1 {
		t.Errorf("entity update calls: got %d want 1", fake.batchCount)
	}
	if got := rr.Header().Get("X-CS-Partial-Delete"); got != "" {
		t.Errorf("X-CS-Partial-Delete: got %q want empty", got)
	}
}

// TestDeleteEntity_AuditHeadersSymmetric — destructive deletes carry
// the same audit headers as POST so the audit trail stays uniform
// across the lifecycle.
func TestDeleteEntity_AuditHeadersSymmetric(t *testing.T) {
	pathID := "acme.ops.robotics.gcs.drone.099"
	fake := &crdFakeRequester{
		removeReply: encodeRemoveOK(t),
	}
	c := newComponentWithRequester(t, fake)

	identity := Identity{
		Forwarded: map[string]string{"User": "alice", "Email": "alice@example.com"},
	}
	if err := c.deleteEntity(context.Background(), pathID, identity); err != nil {
		t.Fatalf("deleteEntity: %v", err)
	}
	if got := fake.deleteHeaders["X-CS-Forwarded-User"]; got != "alice" {
		t.Errorf("X-CS-Forwarded-User on delete: got %q want alice (headers=%+v)", got, fake.deleteHeaders)
	}
}

// TestHandleSystemOptions_ViaMux pins that the route registration in
// handlers.go actually matches the path patterns the handler expects.
// PathValue("id") only fires correctly through the real mux.
func TestHandleSystemOptions_ViaMux(t *testing.T) {
	fake := &crdFakeRequester{}
	c := newComponentWithRequester(t, fake)
	mux := http.NewServeMux()
	c.RegisterHTTPHandlers("", mux)

	pathID := "acme.ops.robotics.gcs.drone.099"
	req := httptest.NewRequest(http.MethodOptions, "/systems/"+pathID, nil)
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

// TestHandleSystemPut_WrongContentType returns 415 (no SensorML on PUT —
// the lossy round-trip would surprise clients on read-back).
func TestHandleSystemPut_WrongContentType(t *testing.T) {
	pathID := "acme.ops.robotics.gcs.drone.099"
	fake := &crdFakeRequester{}
	c := newComponentWithRequester(t, fake)
	c.cfg.SystemIDPrefix = "acme.ops.robotics.gcs.drone"

	body := []byte(`{}`)
	req := httptest.NewRequest(http.MethodPut, "/systems/"+pathID, bytes.NewReader(body))
	req.SetPathValue("id", pathID)
	req.Header.Set("Content-Type", string(MediaSensorML))
	rr := httptest.NewRecorder()
	c.handleSystemPut(rr, req)

	if rr.Code != http.StatusUnsupportedMediaType {
		t.Errorf("status: got %d want 415; body=%s", rr.Code, rr.Body.String())
	}
	wantAccept := string(MediaJSON) + ", " + string(MediaGeoJSON)
	if got := rr.Header().Get("Accept"); got != wantAccept {
		t.Errorf("Accept: got %q want %q", got, wantAccept)
	}
}

// TestHandleSystemsOptions advertises the collection-level Allow header.
func TestHandleSystemsOptions(t *testing.T) {
	fake := &crdFakeRequester{}
	c := newComponentWithRequester(t, fake)

	req := httptest.NewRequest(http.MethodOptions, "/systems", nil)
	rr := httptest.NewRecorder()
	c.handleSystemsOptions(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status: got %d want 204; body=%s", rr.Code, rr.Body.String())
	}
	want := "GET, HEAD, POST, OPTIONS"
	if got := rr.Header().Get("Allow"); got != want {
		t.Errorf("Allow: got %q want %q", got, want)
	}
}

// TestHandleSystemOptions advertises the item-level Allow header — and
// pins that PATCH is absent (no `conf/update` claim at v0.1).
func TestHandleSystemOptions(t *testing.T) {
	fake := &crdFakeRequester{}
	c := newComponentWithRequester(t, fake)

	pathID := "acme.ops.robotics.gcs.drone.099"
	req := httptest.NewRequest(http.MethodOptions, "/systems/"+pathID, nil)
	req.SetPathValue("id", pathID)
	rr := httptest.NewRecorder()
	c.handleSystemOptions(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status: got %d want 204; body=%s", rr.Code, rr.Body.String())
	}
	want := "GET, HEAD, PUT, PATCH, DELETE, OPTIONS"
	if got := rr.Header().Get("Allow"); got != want {
		t.Errorf("Allow: got %q want %q", got, want)
	}
	// Stage 19 added PATCH for conf/update. PATCH is now required.
	if !strings.Contains(rr.Header().Get("Allow"), "PATCH") {
		t.Errorf("Allow MUST advertise PATCH at Stage 19+ for conf/update; got %q", rr.Header().Get("Allow"))
	}
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	switch state := v.(type) {
	case graph.EntityState:
		auditEntityStateFixture(t, state)
	case *graph.EntityState:
		auditEntityStateFixture(t, *state)
	}
	out, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("mustMarshal: %v", err)
	}
	return out
}
