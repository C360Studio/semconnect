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
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/pkg/errs"
	"github.com/c360studio/semstreams/vocabulary/sosa"
	"github.com/nats-io/nats.go"
)

// fakeRequester implements natsRequester deterministically. The captured
// request is exposed so tests can assert on the wire shape.
type fakeRequester struct {
	gotSubject string
	gotBody    []byte
	gotTimeout time.Duration
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

func (f *fakeRequester) Status() natsclient.ConnectionStatus {
	return f.status
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

func TestHandleSystems_SensorMLDeferredReturns406(t *testing.T) {
	// ADR-S001 claims SensorML conformance, but the encoder is not wired
	// at Stage 2. Until it is, 406 is the honest answer.
	fake := &fakeRequester{status: natsclient.StatusConnected}
	c := newTestComponent(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/systems", nil)
	req.Header.Set("Accept", "application/sensorml+json")
	rr := httptest.NewRecorder()
	c.handleSystems(rr, req)

	if rr.Code != http.StatusNotAcceptable {
		t.Fatalf("status: got %d want 406 (got body=%s)", rr.Code, rr.Body.String())
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

func TestHandleSystems_MethodNotAllowed(t *testing.T) {
	fake := &fakeRequester{status: natsclient.StatusConnected}
	c := newTestComponent(t, fake)
	req := httptest.NewRequest(http.MethodPost, "/systems", strings.NewReader("{}"))
	rr := httptest.NewRecorder()
	c.handleSystems(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("status: got %d want 405", rr.Code)
	}
	if got := rr.Header().Get("Allow"); got != "GET, HEAD" {
		t.Errorf("Allow header missing or wrong: %q", got)
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
	wantClaimed := []string{
		"http://www.opengis.net/spec/ogcapi-connectedsystems-1/1.0/conf/core",
		"http://www.opengis.net/spec/ogcapi-connectedsystems-1/1.0/conf/json",
	}
	wantDeferred := []string{
		"http://www.opengis.net/spec/ogcapi-connectedsystems-1/1.0/conf/sensorml",
		"http://www.opengis.net/spec/ogcapi-connectedsystems-2/1.0/conf/oms",
		"http://www.opengis.net/spec/ogcapi-connectedsystems-1/1.0/conf/geojson",
		"http://www.opengis.net/spec/ogcapi-connectedsystems-1/1.0/conf/json-ld",
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
	for _, want := range wantDeferred {
		if contains(decl.ConformsTo, want) {
			t.Errorf("conformance class %s claimed before encoder is wired", want)
		}
	}
}
