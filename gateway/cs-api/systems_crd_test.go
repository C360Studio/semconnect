// Stage 16 — CRD (create-replace-delete) handler tests covering
// POST JSON Feature, PUT replace, DELETE, OPTIONS. The PUT + DELETE
// paths drive multi-subject NATS traffic (entity-query + N triple-remove
// + 1 triple-add-batch), so this file ships a dedicated stub instead of
// extending the Stage 15 multiReplyFakeRequester.
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

	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/parser/sensorml"
	"github.com/c360studio/semstreams/vocabulary/sosa"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// crdFakeRequester is a 3-subject stub for the CRD flow:
//   - subjectEntityQuery → return entityReply (or entityErr)
//   - SubjectTripleRemove → return removeReply (one per predicate); each call
//     captures the (subject, predicate) into removeCalls so tests can pin
//     the per-predicate fanout
//   - SubjectTripleAddBatch → return batchReply (RequestWithHeaders path)
type crdFakeRequester struct {
	mu sync.Mutex

	entityReply []byte
	entityErr   error
	removeReply []byte
	removeErr   error
	batchReply  []byte
	batchErr    error

	entityQueryCalls int
	removeCalls      []graph.RemoveTripleRequest
	removeHeaders    map[string]string
	batchCount       int
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
	case SubjectTripleAddBatch:
		f.batchCount++
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
	case SubjectTripleRemove:
		var req graph.RemoveTripleRequest
		if err := json.Unmarshal(data, &req); err != nil {
			return nil, errors.New("crdFakeRequester: malformed remove body")
		}
		f.removeCalls = append(f.removeCalls, req)
		// Capture audit headers from the first remove call so tests can
		// assert anonymous-now-no-debt symmetry with POST.
		if f.removeHeaders == nil && headers != nil {
			f.removeHeaders = make(map[string]string, len(headers))
			for k, v := range headers {
				f.removeHeaders[k] = v
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
	resp := graph.RemoveTripleResponse{
		MutationResponse: graph.MutationResponse{Success: true},
		Removed:          true,
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
	if fake.gotSubject != SubjectTripleAddBatch {
		t.Errorf("subject: got %q want %q", fake.gotSubject, SubjectTripleAddBatch)
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

// existingSystemState builds an EntityState with three predicates so the
// DELETE/PUT path proves the per-predicate fan-out works (deduplicated).
func existingSystemState(id string) graph.EntityState {
	return graph.EntityState{
		ID: id,
		Triples: []message.Triple{
			{Subject: id, Predicate: sensorml.PredType, Object: sosa.SSNSystem},
			{Subject: id, Predicate: sensorml.PredLabel, Object: "Old label"},
			{Subject: id, Predicate: sensorml.PredDescription, Object: "Old description"},
			// Duplicate predicate (extra hosted child) — dedup must collapse to one remove call.
			{Subject: id, Predicate: sensorml.PredHosts, Object: id + ".camera"},
			{Subject: id, Predicate: sensorml.PredHosts, Object: id + ".gps"},
		},
	}
}

// TestHandleSystemDelete_GoldenPath — DELETE returns 204 and issues one
// remove per *unique* predicate (4 here: type, label, description, hosts).
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
	// 4 unique predicates → 4 remove calls (deduped from 5 triples).
	if got, want := len(fake.removeCalls), 4; got != want {
		t.Fatalf("remove call count: got %d want %d (calls=%+v)", got, want, fake.removeCalls)
	}
	for _, call := range fake.removeCalls {
		if call.Subject != pathID {
			t.Errorf("remove.Subject: got %q want %q", call.Subject, pathID)
		}
		if call.Predicate == "" {
			t.Errorf("remove.Predicate empty: %+v", call)
		}
	}
}

// TestHandleSystemDelete_NotFound_Idempotent — DELETE against an unknown
// ID still returns 204 (CS API §7.6 conventions; the framework
// errEntityNotFound is swallowed).
func TestHandleSystemDelete_NotFound_Idempotent(t *testing.T) {
	pathID := "acme.ops.robotics.gcs.drone.404"
	fake := &crdFakeRequester{
		// entity-query returns a not-found error envelope. classifyEntityQueryError
		// maps the raw "error: not found: ..." byte prefix to errEntityNotFound.
		entityReply: []byte("error: not found: " + pathID),
	}
	c := newComponentWithRequester(t, fake)

	req := httptest.NewRequest(http.MethodDelete, "/systems/"+pathID, nil)
	req.SetPathValue("id", pathID)
	rr := httptest.NewRecorder()
	c.handleSystemDelete(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status: got %d want 204 (idempotent); body=%s", rr.Code, rr.Body.String())
	}
	if len(fake.removeCalls) != 0 {
		t.Errorf("remove should not be called for not-found; got %d calls", len(fake.removeCalls))
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

// TestHandleSystemPut_GoldenPath — PUT removes existing triples then adds
// the new batch. Requires the body's uid → entity ID to match the path.
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
		t.Errorf("add_batch calls: got %d want 1", fake.batchCount)
	}
	if len(fake.removeCalls) < 1 {
		t.Errorf("remove calls: got %d, expected >= 1", len(fake.removeCalls))
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
		t.Errorf("add_batch must not be called on mismatch; got %d calls", fake.batchCount)
	}
	// And no backend round-trip wasted on a client error.
	if fake.entityQueryCalls != 0 {
		t.Errorf("entity-query must not be called on mismatch; got %d calls", fake.entityQueryCalls)
	}
}

// TestHandleSystemPut_TransientRemove exercises the partial-erasure
// path: remove fan-out fails mid-flight with a NATS sentinel → 503 +
// X-CS-Partial-Delete: true; add-batch never runs (entity stays in
// the partial state until the client retries the PUT).
func TestHandleSystemPut_TransientRemove(t *testing.T) {
	pathID := "acme.ops.robotics.gcs.drone.099"
	fake := &crdFakeRequester{
		entityReply: mustMarshal(t, existingSystemState(pathID)),
		removeErr:   nats.ErrNoResponders,
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
	if fake.batchCount != 0 {
		t.Errorf("add_batch must not be called when remove fails; got %d calls", fake.batchCount)
	}
}

// TestDeleteAllEntityTriples_AuditHeadersSymmetric — destructive
// removes carry the same audit headers as POST so the audit trail
// stays uniform across the lifecycle.
func TestDeleteAllEntityTriples_AuditHeadersSymmetric(t *testing.T) {
	pathID := "acme.ops.robotics.gcs.drone.099"
	fake := &crdFakeRequester{
		entityReply: mustMarshal(t, existingSystemState(pathID)),
		removeReply: encodeRemoveOK(t),
	}
	c := newComponentWithRequester(t, fake)

	identity := Identity{
		Forwarded: map[string]string{"User": "alice", "Email": "alice@example.com"},
	}
	if err := c.deleteAllEntityTriples(context.Background(), pathID, identity); err != nil {
		t.Fatalf("deleteAllEntityTriples: %v", err)
	}
	if got := fake.removeHeaders["X-CS-Forwarded-User"]; got != "alice" {
		t.Errorf("X-CS-Forwarded-User on remove: got %q want alice (headers=%+v)", got, fake.removeHeaders)
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
	want := "GET, HEAD, PUT, DELETE, OPTIONS"
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
	want := "GET, HEAD, PUT, DELETE, OPTIONS"
	if got := rr.Header().Get("Allow"); got != want {
		t.Errorf("Allow: got %q want %q", got, want)
	}
	if strings.Contains(rr.Header().Get("Allow"), "PATCH") {
		t.Errorf("Allow must NOT advertise PATCH at v0.1; got %q", rr.Header().Get("Allow"))
	}
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	out, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("mustMarshal: %v", err)
	}
	return out
}
