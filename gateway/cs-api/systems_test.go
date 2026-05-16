package csapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/parser/sensorml"
	"github.com/c360studio/semstreams/pkg/errs"
	"github.com/c360studio/semstreams/vocabulary/sosa"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// fakeRequester implements natsRequester deterministically. The captured
// request is exposed so tests can assert on the wire shape. JetStream-side
// methods exist to satisfy the interface; tests that exercise the publish
// path inject a *fakePublisher onto c.publisher directly.
type fakeRequester struct {
	gotSubject string
	gotBody    []byte
	gotTimeout time.Duration
	gotHeaders map[string]string
	reply      []byte
	replyErr   error
	status     natsclient.ConnectionStatus
}

func (f *fakeRequester) Request(_ context.Context, subj string, data []byte, to time.Duration) ([]byte, error) {
	f.gotSubject = subj
	f.gotBody = append([]byte(nil), data...)
	f.gotTimeout = to
	if f.replyErr != nil {
		return nil, f.replyErr
	}
	return f.reply, nil
}

func (f *fakeRequester) RequestWithHeaders(_ context.Context, subj string, data []byte, headers map[string]string, to time.Duration) (*nats.Msg, error) {
	f.gotSubject = subj
	f.gotBody = append([]byte(nil), data...)
	f.gotTimeout = to
	if headers != nil {
		f.gotHeaders = make(map[string]string, len(headers))
		for k, v := range headers {
			f.gotHeaders[k] = v
		}
	}
	if f.replyErr != nil {
		return nil, f.replyErr
	}
	return &nats.Msg{Data: f.reply}, nil
}

func (f *fakeRequester) Status() natsclient.ConnectionStatus {
	return f.status
}

// JetStream / EnsureStream satisfy the natsRequester interface but are not
// exercised by the unit tests — Stage 3 tests inject a *fakePublisher onto
// c.publisher directly rather than driving the full Start() path.
func (f *fakeRequester) JetStream() (jetstream.JetStream, error) {
	return nil, errors.New("fakeRequester: JetStream not implemented (use fakePublisher directly)")
}
func (f *fakeRequester) EnsureStream(_ context.Context, _ jetstream.StreamConfig) (jetstream.Stream, error) {
	return nil, errors.New("fakeRequester: EnsureStream not implemented")
}

// newTestComponent assembles a Component wired to the fake. Helper isolates
// the construction noise from the test bodies.
func newTestComponent(t *testing.T, fake *fakeRequester) *Component {
	t.Helper()
	cfg := DefaultConfig()
	cfg.QueryTimeout = 500 * time.Millisecond
	c, err := New(cfg, fake, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func encodeReply(t *testing.T, entities []string) []byte {
	t.Helper()
	resp := graph.NewQueryResponse(graph.PredicateData{Entities: entities})
	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("encodeReply: %v", err)
	}
	return b
}

func encodeReplyErr(t *testing.T, msg string) []byte {
	t.Helper()
	resp := graph.NewQueryError[graph.PredicateData](msg)
	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("encodeReplyErr: %v", err)
	}
	return b
}

func TestHandleSystems_GoldenPath(t *testing.T) {
	fake := &fakeRequester{
		reply:  encodeReply(t, []string{"acme.ops.robotics.gcs.drone.001", "acme.ops.robotics.gcs.drone.002"}),
		status: natsclient.StatusConnected,
	}
	c := newTestComponent(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/systems", nil)
	rr := httptest.NewRecorder()
	// Drive through the middleware chain so Identity is populated end-to-end.
	c.middleware(http.HandlerFunc(c.handleSystems)).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); ct != string(MediaJSON) {
		t.Errorf("Content-Type: got %q want %q", ct, MediaJSON)
	}

	var coll systemCollection
	if err := json.Unmarshal(rr.Body.Bytes(), &coll); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, rr.Body.String())
	}
	if coll.Type != "SystemCollection" {
		t.Errorf("Type: got %q want SystemCollection", coll.Type)
	}
	if coll.NumberMatched != 2 || coll.NumberReturned != 2 {
		t.Errorf("counts: matched=%d returned=%d want 2/2", coll.NumberMatched, coll.NumberReturned)
	}
	if len(coll.Systems) != 2 || coll.Systems[0].ID != "acme.ops.robotics.gcs.drone.001" {
		t.Errorf("Systems: %+v", coll.Systems)
	}

	// Wire shape: subject + predicate + value must be exact, otherwise
	// graph-index won't match anything in real NATS.
	if fake.gotSubject != subjectPredicateQuery {
		t.Errorf("subject: got %q want %q", fake.gotSubject, subjectPredicateQuery)
	}
	var body struct {
		Predicate string  `json:"predicate"`
		Value     *string `json:"value,omitempty"`
		Limit     int     `json:"limit"`
	}
	if err := json.Unmarshal(fake.gotBody, &body); err != nil {
		t.Fatalf("decode captured body: %v", err)
	}
	if body.Predicate != predicateRDFType {
		t.Errorf("predicate: got %q want %q", body.Predicate, predicateRDFType)
	}
	if body.Value == nil || *body.Value != sosa.SSNSystem {
		t.Errorf("value: got %v want %q", body.Value, sosa.SSNSystem)
	}
	if body.Limit != DefaultConfig().DefaultListLimit {
		t.Errorf("limit: got %d want %d", body.Limit, DefaultConfig().DefaultListLimit)
	}
	if fake.gotTimeout != 500*time.Millisecond {
		t.Errorf("timeout: got %v want 500ms", fake.gotTimeout)
	}
}

func TestHandleSystems_NotAcceptable(t *testing.T) {
	fake := &fakeRequester{status: natsclient.StatusConnected}
	c := newTestComponent(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/systems", nil)
	req.Header.Set("Accept", "application/xml")
	rr := httptest.NewRecorder()
	c.handleSystems(rr, req)

	if rr.Code != http.StatusNotAcceptable {
		t.Fatalf("status: got %d want 406", rr.Code)
	}
	if fake.gotSubject != "" {
		t.Errorf("backend should not be called on 406 (got subject %q)", fake.gotSubject)
	}
}

func TestHandleSystems_CollectionNarrowsToJSON(t *testing.T) {
	// FamilySystem.supported() includes SensorML + JSON-LD at Stage 4
	// because the *item* endpoint (GET /systems/{id}) supports them. The
	// collection handler narrows that set: there is no SensorML
	// "SystemCollection" type and a collection-wide JSON-LD aggregation
	// is post-v0.1. A SensorML Accept on the collection therefore 406s
	// honestly rather than silently degrading to JSON.
	tests := []struct {
		name   string
		accept string
	}{
		{"SensorML on collection → 406", "application/sensorml+json"},
		{"JSON-LD on collection → 406", "application/ld+json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeRequester{status: natsclient.StatusConnected}
			c := newTestComponent(t, fake)

			req := httptest.NewRequest(http.MethodGet, "/systems", nil)
			req.Header.Set("Accept", tt.accept)
			rr := httptest.NewRecorder()
			c.handleSystems(rr, req)

			if rr.Code != http.StatusNotAcceptable {
				t.Fatalf("status: got %d want 406 (body=%s)", rr.Code, rr.Body.String())
			}
		})
	}
}

func TestHandleSystems_LimitValidation(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		wantCode int
	}{
		{"default when missing", "", http.StatusOK},
		{"valid in range", "10", http.StatusOK},
		{"non-integer rejected", "abc", http.StatusBadRequest},
		{"zero rejected", "0", http.StatusBadRequest},
		{"negative rejected", "-1", http.StatusBadRequest},
		{"above ceiling rejected", "999999", http.StatusBadRequest},
		{"int64 overflow rejected", "9999999999999999999999", http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeRequester{
				reply:  encodeReply(t, nil),
				status: natsclient.StatusConnected,
			}
			c := newTestComponent(t, fake)
			url := "/systems"
			if tt.raw != "" {
				url = "/systems?limit=" + tt.raw
			}
			req := httptest.NewRequest(http.MethodGet, url, nil)
			rr := httptest.NewRecorder()
			c.handleSystems(rr, req)
			if rr.Code != tt.wantCode {
				t.Errorf("status: got %d want %d; body=%s", rr.Code, tt.wantCode, rr.Body.String())
			}
		})
	}
}

func TestHandleSystems_MethodNotAllowedViaMux(t *testing.T) {
	// Stage 4 migrated /systems to method+path mux patterns; Stage 8 added
	// POST /systems. The mux 405s any method outside {GET, HEAD, POST} and
	// advertises the allowed set in the Allow header.
	fake := &fakeRequester{status: natsclient.StatusConnected}
	c := newTestComponent(t, fake)
	mux := http.NewServeMux()
	c.RegisterHTTPHandlers("", mux)

	req := httptest.NewRequest(http.MethodDelete, "/systems", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("status: got %d want 405", rr.Code)
	}
	// Order is non-deterministic across the set; check membership.
	allow := rr.Header().Get("Allow")
	for _, want := range []string{"GET", "HEAD", "POST"} {
		if !strings.Contains(allow, want) {
			t.Errorf("Allow header missing %s: %q", want, allow)
		}
	}
}

func TestHandleSystems_NoRespondersClassifiedAs503(t *testing.T) {
	// Regression cover for review must-fix #3: natsclient returns raw
	// nats.ErrNoResponders; we wrap it Transient at the boundary so /health
	// can distinguish "backend down" from "client sent garbage".
	fake := &fakeRequester{
		replyErr: nats.ErrNoResponders,
		status:   natsclient.StatusConnected,
	}
	c := newTestComponent(t, fake)
	req := httptest.NewRequest(http.MethodGet, "/systems", nil)
	rr := httptest.NewRecorder()
	c.handleSystems(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("status: got %d want 503; body=%s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); ct != string(MediaJSON) {
		t.Errorf("Content-Type: got %q want %q", ct, MediaJSON)
	}
}

func TestHandleSystems_4xxDoesNotPollHealth(t *testing.T) {
	// Regression cover for review must-fix #1: a malformed ?limit= must not
	// flip /health to 503 forever. /health treats c.errs as a backend-error
	// signal, not a client-traffic signal.
	fake := &fakeRequester{status: natsclient.StatusConnected}
	c := newTestComponent(t, fake)
	c.initialized = true // bypass Start() so Health() reports running state

	req := httptest.NewRequest(http.MethodGet, "/systems?limit=abc", nil)
	rr := httptest.NewRecorder()
	c.middleware(http.HandlerFunc(c.handleSystems)).ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad limit, got %d", rr.Code)
	}
	if errCount := c.errs.Load(); errCount != 0 {
		t.Errorf("4xx response leaked into health counter: errs=%d want 0", errCount)
	}
}

func TestHandleSystems_HEADReturnsHeadersWithoutBody(t *testing.T) {
	fake := &fakeRequester{
		reply:  encodeReply(t, []string{"acme.ops.robotics.gcs.drone.001"}),
		status: natsclient.StatusConnected,
	}
	c := newTestComponent(t, fake)
	req := httptest.NewRequest(http.MethodHead, "/systems", nil)
	rr := httptest.NewRecorder()
	c.handleSystems(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", rr.Code)
	}
	if rr.Body.Len() != 0 {
		t.Errorf("HEAD response should have empty body, got %q", rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); ct != string(MediaJSON) {
		t.Errorf("Content-Type: got %q want %q", ct, MediaJSON)
	}
}

func TestHandleSystems_BackendErrorClassification(t *testing.T) {
	tests := []struct {
		name      string
		replyErr  error
		replyBody []byte
		wantCode  int
	}{
		{
			name:     "transient NATS error → 503",
			replyErr: errs.WrapTransient(errors.New("nats timeout"), "test", "Request", "boom"),
			wantCode: http.StatusServiceUnavailable,
		},
		{
			name:     "invalid request → 400",
			replyErr: errs.WrapInvalid(errors.New("bad body"), "test", "Request", "boom"),
			wantCode: http.StatusBadRequest,
		},
		{
			name:     "unclassified error → 500",
			replyErr: errors.New("unexpected"),
			wantCode: http.StatusInternalServerError,
		},
		{
			name:      "graph-index reports error in response envelope → 503",
			replyBody: encodeReplyErr(t, "internal error"),
			wantCode:  http.StatusServiceUnavailable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeRequester{
				reply:    tt.replyBody,
				replyErr: tt.replyErr,
				status:   natsclient.StatusConnected,
			}
			c := newTestComponent(t, fake)
			req := httptest.NewRequest(http.MethodGet, "/systems", nil)
			rr := httptest.NewRecorder()
			c.handleSystems(rr, req)
			if rr.Code != tt.wantCode {
				t.Errorf("status: got %d want %d; body=%s", rr.Code, tt.wantCode, rr.Body.String())
			}
		})
	}
}

// -----------------------------------------------------------------------------
// Stage 4: GET /systems/{id}
// -----------------------------------------------------------------------------

// encodeEntityState marshals an EntityState as graph-ingest would put on the
// wire. The Stage-4 fetchEntity expects raw EntityState JSON (no envelope).
func encodeEntityState(t *testing.T, state graph.EntityState) []byte {
	t.Helper()
	b, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("encodeEntityState: %v", err)
	}
	return b
}

func droneState() graph.EntityState {
	return graph.EntityState{
		ID: "acme.ops.robotics.gcs.drone.001",
		Triples: []message.Triple{
			{Subject: "acme.ops.robotics.gcs.drone.001", Predicate: sensorml.PredType, Object: sosa.SSNSystem},
			{Subject: "acme.ops.robotics.gcs.drone.001", Predicate: sensorml.PredLabel, Object: "ACME Drone 001"},
			{Subject: "acme.ops.robotics.gcs.drone.001", Predicate: sensorml.PredDescription, Object: "Hex rotor"},
			{Subject: "acme.ops.robotics.gcs.drone.001", Predicate: sensorml.PredHosts, Object: "acme.ops.robotics.gcs.drone.001.camera"},
		},
	}
}

func TestHandleSystem_JSON(t *testing.T) {
	fake := &fakeRequester{
		reply:  encodeEntityState(t, droneState()),
		status: natsclient.StatusConnected,
	}
	c := newTestComponent(t, fake)
	mux := http.NewServeMux()
	c.RegisterHTTPHandlers("", mux)

	req := httptest.NewRequest(http.MethodGet, "/systems/acme.ops.robotics.gcs.drone.001", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); ct != string(MediaJSON) {
		t.Errorf("Content-Type: got %q want %q", ct, MediaJSON)
	}
	if lossy := rr.Header().Get("X-CS-Reconstructed-Lossy"); lossy != "true" {
		t.Errorf("X-CS-Reconstructed-Lossy: got %q want true", lossy)
	}

	var sys system
	if err := json.Unmarshal(rr.Body.Bytes(), &sys); err != nil {
		t.Fatalf("decode: %v; body=%s", err, rr.Body.String())
	}
	if sys.ID != "acme.ops.robotics.gcs.drone.001" {
		t.Errorf("ID: got %q", sys.ID)
	}
	if sys.Type != "System" {
		t.Errorf("Type: got %q", sys.Type)
	}
	if sys.Label != "ACME Drone 001" {
		t.Errorf("Label: got %q", sys.Label)
	}
	if len(sys.Hosts) != 1 || sys.Hosts[0] != "acme.ops.robotics.gcs.drone.001.camera" {
		t.Errorf("Hosts: got %+v", sys.Hosts)
	}

	// fetchEntity must have hit graph.query.entity with the right ID.
	if fake.gotSubject != subjectEntityQuery {
		t.Errorf("subject: got %q want %q", fake.gotSubject, subjectEntityQuery)
	}
	var body struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(fake.gotBody, &body); err != nil {
		t.Fatalf("decode captured body: %v", err)
	}
	if body.ID != "acme.ops.robotics.gcs.drone.001" {
		t.Errorf("query ID: got %q", body.ID)
	}
}

func TestHandleSystem_SensorML(t *testing.T) {
	fake := &fakeRequester{
		reply:  encodeEntityState(t, droneState()),
		status: natsclient.StatusConnected,
	}
	c := newTestComponent(t, fake)
	mux := http.NewServeMux()
	c.RegisterHTTPHandlers("", mux)

	req := httptest.NewRequest(http.MethodGet, "/systems/acme.ops.robotics.gcs.drone.001", nil)
	req.Header.Set("Accept", string(MediaSensorML))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); ct != string(MediaSensorML) {
		t.Errorf("Content-Type: got %q want %q", ct, MediaSensorML)
	}
	// Round-trip back through the framework parser to confirm the body is
	// valid SensorML JSON.
	proc, err := sensorml.UnmarshalProcess(rr.Body.Bytes())
	if err != nil {
		t.Fatalf("framework parse: %v; body=%s", err, rr.Body.String())
	}
	if proc.Type() != sensorml.TypePhysicalSystem {
		t.Errorf("type: got %q want %q", proc.Type(), sensorml.TypePhysicalSystem)
	}
}

func TestHandleSystem_JSONLD(t *testing.T) {
	fake := &fakeRequester{
		reply:  encodeEntityState(t, droneState()),
		status: natsclient.StatusConnected,
	}
	c := newTestComponent(t, fake)
	mux := http.NewServeMux()
	c.RegisterHTTPHandlers("", mux)

	req := httptest.NewRequest(http.MethodGet, "/systems/acme.ops.robotics.gcs.drone.001", nil)
	req.Header.Set("Accept", string(MediaJSONLD))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); ct != string(MediaJSONLD) {
		t.Errorf("Content-Type: got %q want %q", ct, MediaJSONLD)
	}
	// JSON-LD bodies are JSON-shaped; confirm it decodes.
	var generic map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &generic); err != nil {
		t.Fatalf("decode JSON-LD body: %v; body=%s", err, rr.Body.String())
	}
	if _, ok := generic["@context"]; !ok {
		t.Errorf("JSON-LD missing @context: %v", generic)
	}
}

func TestHandleSystem_NotFound(t *testing.T) {
	// Framework's request-reply error format is `"error: not found: <id>"`
	// (see classifyEntityQueryError TODO upstream). Detect → 404.
	fake := &fakeRequester{
		reply:  []byte("error: not found: acme.ops.robotics.gcs.drone.999"),
		status: natsclient.StatusConnected,
	}
	c := newTestComponent(t, fake)
	mux := http.NewServeMux()
	c.RegisterHTTPHandlers("", mux)

	req := httptest.NewRequest(http.MethodGet, "/systems/acme.ops.robotics.gcs.drone.999", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status: got %d want 404; body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleSystem_BadID(t *testing.T) {
	fake := &fakeRequester{status: natsclient.StatusConnected}
	c := newTestComponent(t, fake)
	mux := http.NewServeMux()
	c.RegisterHTTPHandlers("", mux)

	// Empty-token ID — same rule as datastream IDs.
	req := httptest.NewRequest(http.MethodGet, "/systems/.bad.id.", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d want 400; body=%s", rr.Code, rr.Body.String())
	}
	if fake.gotSubject != "" {
		t.Errorf("backend should not be called on 400; got subject %q", fake.gotSubject)
	}
}

func TestHandleSystem_HEAD(t *testing.T) {
	fake := &fakeRequester{
		reply:  encodeEntityState(t, droneState()),
		status: natsclient.StatusConnected,
	}
	c := newTestComponent(t, fake)
	mux := http.NewServeMux()
	c.RegisterHTTPHandlers("", mux)

	req := httptest.NewRequest(http.MethodHead, "/systems/acme.ops.robotics.gcs.drone.001", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", rr.Code)
	}
	if rr.Body.Len() != 0 {
		t.Errorf("HEAD body should be empty; got %d bytes", rr.Body.Len())
	}
	if ct := rr.Header().Get("Content-Type"); ct != string(MediaJSON) {
		t.Errorf("Content-Type: got %q", ct)
	}
}

func TestHandleSystem_NonSystemEntity404sConsistentlyAcrossMedia(t *testing.T) {
	// An entity that exists at /systems/{id} but is not a System kind
	// (no rdf:type triple, or rdf:type is e.g. an Observation) 404s
	// uniformly across JSON / SensorML / JSON-LD. Pre-Stage 4 had a
	// divergent response per media (406 SensorML / degraded JSON / empty
	// JSON-LD); review M-3 made it consistent.
	stateMissingType := graph.EntityState{
		ID: "acme.x",
		Triples: []message.Triple{
			{Subject: "acme.x", Predicate: sensorml.PredLabel, Object: "Mystery"},
		},
	}
	stateWrongKind := graph.EntityState{
		ID: "acme.y",
		Triples: []message.Triple{
			{Subject: "acme.y", Predicate: sensorml.PredType, Object: "http://example.org/types/Observation"},
		},
	}

	for _, state := range []graph.EntityState{stateMissingType, stateWrongKind} {
		for _, mt := range []MediaType{MediaJSON, MediaSensorML, MediaJSONLD} {
			t.Run(state.ID+"/"+string(mt), func(t *testing.T) {
				fake := &fakeRequester{
					reply:  encodeEntityState(t, state),
					status: natsclient.StatusConnected,
				}
				c := newTestComponent(t, fake)
				mux := http.NewServeMux()
				c.RegisterHTTPHandlers("", mux)

				req := httptest.NewRequest(http.MethodGet, "/systems/"+state.ID, nil)
				req.Header.Set("Accept", string(mt))
				rr := httptest.NewRecorder()
				mux.ServeHTTP(rr, req)

				if rr.Code != http.StatusNotFound {
					t.Errorf("status: got %d want 404; body=%s", rr.Code, rr.Body.String())
				}
			})
		}
	}
}

func TestHandleSystem_MinimalValidEntity_AllMedia(t *testing.T) {
	// rdf:type triple only — the smallest possible System. All three
	// media types must produce well-formed output (no nil-deref, no
	// empty-bodies, no 500). This is what Team Engine's conformance
	// suite is most likely to throw first.
	state := graph.EntityState{
		ID: "acme.minimal.001",
		Triples: []message.Triple{
			{Subject: "acme.minimal.001", Predicate: sensorml.PredType, Object: sosa.SSNSystem},
		},
	}

	for _, mt := range []MediaType{MediaJSON, MediaSensorML, MediaJSONLD} {
		t.Run(string(mt), func(t *testing.T) {
			fake := &fakeRequester{
				reply:  encodeEntityState(t, state),
				status: natsclient.StatusConnected,
			}
			c := newTestComponent(t, fake)
			mux := http.NewServeMux()
			c.RegisterHTTPHandlers("", mux)

			req := httptest.NewRequest(http.MethodGet, "/systems/acme.minimal.001", nil)
			req.Header.Set("Accept", string(mt))
			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("status: got %d want 200; body=%s", rr.Code, rr.Body.String())
			}
			if rr.Body.Len() == 0 {
				t.Errorf("body empty for %s — minimal entity should still encode", mt)
			}
		})
	}
}

func TestHandleSystem_BackendTransientErrorClassifiedAs503(t *testing.T) {
	fake := &fakeRequester{
		replyErr: nats.ErrNoResponders,
		status:   natsclient.StatusConnected,
	}
	c := newTestComponent(t, fake)
	mux := http.NewServeMux()
	c.RegisterHTTPHandlers("", mux)

	req := httptest.NewRequest(http.MethodGet, "/systems/acme.ops.robotics.gcs.drone.001", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("status: got %d want 503; body=%s", rr.Code, rr.Body.String())
	}
}

func TestClassifyEntityQueryError(t *testing.T) {
	// Direct unit cover for the framework-error-prefix workaround. When
	// upstream ships structured errors this function becomes a no-op;
	// these cases pin the current taxonomy.
	tests := []struct {
		name    string
		body    []byte
		wantNil bool
		probe   func(error) bool // optional further check
	}{
		{
			name:    "success body returns nil",
			body:    []byte(`{"id":"x","triples":[]}`),
			wantNil: true,
		},
		{
			name: "not found wraps errEntityNotFound sentinel",
			body: []byte("error: not found: acme.ops.robotics.gcs.drone.999"),
			probe: func(err error) bool {
				return errors.Is(err, errEntityNotFound)
			},
		},
		{
			name: "invalid request wraps as Invalid",
			body: []byte("error: invalid request: empty id"),
			probe: func(err error) bool {
				return errs.IsInvalid(err)
			},
		},
		{
			name: "internal error is unclassified",
			body: []byte("error: internal error: kv get failed"),
			probe: func(err error) bool {
				return !errs.IsInvalid(err) && !errs.IsTransient(err) && !errors.Is(err, errEntityNotFound)
			},
		},
		{
			name: "unknown tail also unclassified",
			body: []byte("error: something unexpected"),
			probe: func(err error) bool {
				return !errs.IsInvalid(err) && !errs.IsTransient(err)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyEntityQueryError(tt.body)
			if tt.wantNil {
				if got != nil {
					t.Errorf("expected nil; got %v", got)
				}
				return
			}
			if got == nil {
				t.Fatalf("expected non-nil error")
			}
			if tt.probe != nil && !tt.probe(got) {
				t.Errorf("probe failed: err=%v", got)
			}
		})
	}
}

func TestHandleConformance_ClaimsOnlyWiredClasses(t *testing.T) {
	// Stage 2 has core + json wired. Stages 3–5 add sensorml / oms / geojson
	// / json-ld as their encoders land — this test grows with them.
	fake := &fakeRequester{status: natsclient.StatusConnected}
	c := newTestComponent(t, fake)
	req := httptest.NewRequest(http.MethodGet, "/conformance", nil)
	rr := httptest.NewRecorder()
	c.handleConformance(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", rr.Code)
	}
	var decl conformanceDeclaration
	body, _ := io.ReadAll(rr.Body)
	if err := json.Unmarshal(body, &decl); err != nil {
		t.Fatalf("decode: %v; body=%s", err, body)
	}
	// Stage 7 added the OGC API Common Part 1 Core + JSON URIs (CS API
	// Core inherits from Common Core; the Botts ETS asserts the inheritance
	// is named explicitly). The strong invariant remains:
	// count(declared) == count(stageConformanceClasses) — a future stage
	// that adds a class to ADR-S001's roadmap without wiring it fails loudly.
	wantClaimed := []string{
		"http://www.opengis.net/spec/ogcapi-common-1/1.0/conf/core",
		"http://www.opengis.net/spec/ogcapi-common-1/1.0/conf/json",
		"http://www.opengis.net/spec/ogcapi-connectedsystems-1/1.0/conf/core",
		"http://www.opengis.net/spec/ogcapi-connectedsystems-1/1.0/conf/json",
		"http://www.opengis.net/spec/ogcapi-connectedsystems-2/1.0/conf/oms",
		"http://www.opengis.net/spec/ogcapi-connectedsystems-1/1.0/conf/sensorml",
		"http://www.opengis.net/spec/ogcapi-connectedsystems-1/1.0/conf/json-ld",
		"http://www.opengis.net/spec/ogcapi-connectedsystems-1/1.0/conf/geojson",
	}
	if len(decl.ConformsTo) != len(stageConformanceClasses) {
		t.Errorf("ConformsTo count: got %d want %d (declared classes drift from stageConformanceClasses)",
			len(decl.ConformsTo), len(stageConformanceClasses))
	}
	contains := func(s []string, want string) bool {
		for _, v := range s {
			if v == want {
				return true
			}
		}
		return false
	}
	for _, want := range wantClaimed {
		if !contains(decl.ConformsTo, want) {
			t.Errorf("conformance class missing: %s (got %v)", want, decl.ConformsTo)
		}
	}
}
