package csapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/message/oms"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/payloadbuiltins"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// fakePublisher implements streamPublisher and captures the published message
// for wire-shape assertions.
type fakePublisher struct {
	gotMsg *nats.Msg
	pubErr error
}

func (f *fakePublisher) PublishMsg(_ context.Context, msg *nats.Msg, _ ...jetstream.PublishOpt) (*jetstream.PubAck, error) {
	f.gotMsg = msg
	if f.pubErr != nil {
		return nil, f.pubErr
	}
	return &jetstream.PubAck{Stream: "CS_API_OBSERVATIONS", Sequence: 1}, nil
}

// wireObservationsComponent builds a Component with the request mock + a
// publish mock, bypassing Start() (which would need real JetStream).
func wireObservationsComponent(t *testing.T, fake *fakeRequester, pub *fakePublisher) *Component {
	t.Helper()
	c := newTestComponent(t, fake)
	var sp streamPublisher = pub
	c.publisher.Store(&sp)
	c.initialized = true
	return c
}

func validObservation() oms.Observation {
	return oms.Observation{
		ID:               "obs-123",
		Procedure:        "http://example.org/procedures/voltmeter",
		ObservedProperty: "http://example.org/properties/battery-voltage",
		ResultTime:       "2026-05-15T14:30:00.250Z",
		PhenomenonTime:   "2026-05-15T14:30:00Z",
		Result:           12.4,
	}
}

// postObservation drives a request through the full middleware chain (so
// Identity, body-limit, and counting all run end-to-end).
func postObservation(t *testing.T, c *Component, datastreamID string, body []byte, ct string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	c.RegisterHTTPHandlers("", mux)
	req := httptest.NewRequest(http.MethodPost,
		"/datastreams/"+datastreamID+"/observations", bytes.NewReader(body))
	if ct != "" {
		req.Header.Set("Content-Type", ct)
	}
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	return rr
}

func TestHandleObservationsPost_GoldenPath(t *testing.T) {
	fake := &fakeRequester{status: natsclient.StatusConnected}
	pub := &fakePublisher{}
	c := wireObservationsComponent(t, fake, pub)

	obs := validObservation()
	body, err := json.Marshal(&obs)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	rr := postObservation(t, c, "acme.ops.robotics.gcs.drone.001", body, "application/om+json")

	if rr.Code != http.StatusCreated {
		t.Fatalf("status: got %d want 201; body=%s", rr.Code, rr.Body.String())
	}
	if loc := rr.Header().Get("Location"); loc != "/datastreams/acme.ops.robotics.gcs.drone.001/observations/obs-123" {
		t.Errorf("Location: got %q", loc)
	}

	if pub.gotMsg == nil {
		t.Fatal("publish was not called")
	}
	wantSubject := "cs-api.observations.acme.ops.robotics.gcs.drone.001"
	if pub.gotMsg.Subject != wantSubject {
		t.Errorf("subject: got %q want %q", pub.gotMsg.Subject, wantSubject)
	}
	// Audit headers from anonymous Identity should still carry the
	// payload-type tag; X-CS-Subject is absent because no proxy headers.
	if pub.gotMsg.Header.Get("X-CS-Payload-Type") == "" {
		t.Errorf("expected X-CS-Payload-Type header on publish, got headers=%v", pub.gotMsg.Header)
	}

	// Decode the BaseMessage envelope so we know the payload round-tripped
	// through the framework's serializer, not just a raw OMS dump.
	reg := payloadbuiltins.NewTestRegistry(t)
	dec := message.NewDecoder(reg)
	decoded, err := dec.Decode(pub.gotMsg.Data)
	if err != nil {
		t.Fatalf("decode BaseMessage: %v; data=%s", err, pub.gotMsg.Data)
	}

	// Envelope assertions — the BaseMessage wrap is load-bearing per the
	// framework's discipline note ("every publish wraps, even when the
	// obvious consumer reads raw"). Verify the envelope itself, not just
	// the payload survival.
	if got := decoded.Type(); got != oms.SchemaType() {
		t.Errorf("envelope Type: got %v want %v", got, oms.SchemaType())
	}
	if src := decoded.Meta().Source(); src != publishSource {
		t.Errorf("envelope Source: got %q want %q", src, publishSource)
	}
	if drift := time.Since(decoded.Meta().CreatedAt()); drift < 0 || drift > 5*time.Second {
		t.Errorf("envelope CreatedAt drift: %v (want within 5s of now)", drift)
	}

	gotObs, ok := decoded.Payload().(*oms.Observation)
	if !ok {
		t.Fatalf("payload type: got %T want *oms.Observation", decoded.Payload())
	}
	if gotObs.Procedure != obs.Procedure {
		t.Errorf("Procedure round-trip: got %q want %q", gotObs.Procedure, obs.Procedure)
	}
	if gotObs.ResultTime != obs.ResultTime {
		t.Errorf("ResultTime round-trip: got %q want %q", gotObs.ResultTime, obs.ResultTime)
	}

	// Trace propagation — must match natsclient.PublishToStream's
	// auto-injection behavior since we dropped to raw js.PublishMsg.
	// Without this header, cross-component correlation breaks.
	if tp := pub.gotMsg.Header.Get(natsclient.TraceparentHeader); tp == "" {
		t.Errorf("missing traceparent header on publish (headers=%v)", pub.gotMsg.Header)
	}
}

func TestHandleObservationsPost_ForwardedUserBecomesAuditHeader(t *testing.T) {
	// Regression cover for ADR-S001 §3: even though v0.1 auth is anonymous,
	// X-Forwarded-User must flow onto every publish for audit. When real
	// JWT verification arrives, the same headers carry the verified subject
	// with no handler edit.
	fake := &fakeRequester{status: natsclient.StatusConnected}
	pub := &fakePublisher{}
	c := wireObservationsComponent(t, fake, pub)

	body, _ := json.Marshal(validObservation())

	mux := http.NewServeMux()
	c.RegisterHTTPHandlers("", mux)
	req := httptest.NewRequest(http.MethodPost,
		"/datastreams/acme.ops.robotics.gcs.drone.001/observations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/om+json")
	req.Header.Set("X-Forwarded-User", "alice@example.org")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status: got %d want 201; body=%s", rr.Code, rr.Body.String())
	}
	if got := pub.gotMsg.Header.Get("X-CS-Forwarded-User"); got != "alice@example.org" {
		t.Errorf("X-CS-Forwarded-User header: got %q want %q", got, "alice@example.org")
	}
}

func TestHandleObservationsPost_ContentTypeValidation(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		wantStatus  int
	}{
		{"correct om+json", "application/om+json", http.StatusCreated},
		{"om+json with charset param", "application/om+json; charset=utf-8", http.StatusCreated},
		{"om+json with profile param", `application/om+json; profile="https://example.org/profiles/omsi"`, http.StatusCreated},
		{"missing Content-Type rejected", "", http.StatusUnsupportedMediaType},
		{"plain JSON rejected (must be om+json)", "application/json", http.StatusUnsupportedMediaType},
		{"XML rejected", "application/xml", http.StatusUnsupportedMediaType},
		{"malformed Content-Type", "garbage//", http.StatusUnsupportedMediaType},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeRequester{status: natsclient.StatusConnected}
			pub := &fakePublisher{}
			c := wireObservationsComponent(t, fake, pub)
			body, _ := json.Marshal(validObservation())
			rr := postObservation(t, c, "ds.001", body, tt.contentType)
			if rr.Code != tt.wantStatus {
				t.Errorf("status: got %d want %d; body=%s", rr.Code, tt.wantStatus, rr.Body.String())
			}
			if tt.wantStatus == http.StatusUnsupportedMediaType {
				if ap := rr.Header().Get("Accept-Post"); ap != "application/om+json" {
					t.Errorf("Accept-Post header missing or wrong: %q", ap)
				}
				if pub.gotMsg != nil {
					t.Errorf("publish should not be called on 415")
				}
			}
		})
	}
}

func TestHandleObservationsPost_BodyValidation(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		wantCode int
	}{
		{"empty body rejected", "", http.StatusBadRequest},
		{"non-JSON body rejected", "not json at all", http.StatusBadRequest},
		{"missing procedure rejected (OMS required)", `{"observedProperty":"x","resultTime":"2026-05-15T14:30:00Z"}`, http.StatusBadRequest},
		{"missing resultTime rejected (OMS required)", `{"procedure":"x","observedProperty":"y"}`, http.StatusBadRequest},
		{"wrong type discriminator rejected", `{"type":"Sample","procedure":"x","observedProperty":"y","resultTime":"2026-05-15T14:30:00Z"}`, http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeRequester{status: natsclient.StatusConnected}
			pub := &fakePublisher{}
			c := wireObservationsComponent(t, fake, pub)
			rr := postObservation(t, c, "ds.001", []byte(tt.body), "application/om+json")
			if rr.Code != tt.wantCode {
				t.Errorf("status: got %d want %d; body=%s", rr.Code, tt.wantCode, rr.Body.String())
			}
			if pub.gotMsg != nil {
				t.Errorf("publish should not be called on validation failure")
			}
		})
	}
}

func TestHandleObservationsPost_DatastreamIDValidation(t *testing.T) {
	// Whitespace-bearing IDs are rejected at the validateDatastreamID
	// helper (covered by TestValidateDatastreamID) — they cannot reach
	// the handler via HTTP because the request line parser fails first.
	tests := []struct {
		name         string
		datastreamID string
		wantCode     int
	}{
		{"valid dotted ID", "acme.ops.robotics.gcs.drone.001", http.StatusCreated},
		{"wildcard char rejected", "ds.with.*.token", http.StatusBadRequest},
		{"NATS-greedy-wildcard rejected", "ds.with.>", http.StatusBadRequest},
		{"oversize rejected", strings.Repeat("a", 257), http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeRequester{status: natsclient.StatusConnected}
			pub := &fakePublisher{}
			c := wireObservationsComponent(t, fake, pub)
			body, _ := json.Marshal(validObservation())
			rr := postObservation(t, c, tt.datastreamID, body, "application/om+json")
			if rr.Code != tt.wantCode {
				t.Errorf("status: got %d want %d; body=%s", rr.Code, tt.wantCode, rr.Body.String())
			}
		})
	}
}

func TestValidateDatastreamID(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		wantErr bool
	}{
		{"valid dotted ID", "acme.ops.robotics.gcs.drone.001", false},
		{"single token", "alpha", false},
		{"empty rejected", "", true},
		{"whitespace rejected", "ds with spaces", true},
		{"tab rejected", "ds\twith\ttabs", true},
		{"NATS wildcard rejected", "ds.*.x", true},
		{"NATS greedy wildcard rejected", "ds.>", true},
		{"oversize rejected", strings.Repeat("a", 257), true},
		{"exactly 256 bytes accepted", strings.Repeat("a", 256), false},
		{"leading dot rejected", ".foo", true},
		{"trailing dot rejected", "foo.", true},
		{"consecutive dots rejected", "acme..ops", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateDatastreamID(tt.id)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateDatastreamID(%q): got err=%v want err=%v", tt.id, err, tt.wantErr)
			}
		})
	}
}

func TestHandleObservationsPost_OversizeBodyReturns413(t *testing.T) {
	// Regression cover for the Stage-2 MaxBytesReader middleware: the
	// limit lands at Stage 2 even though the first POST consumer is
	// Stage 3, and Stage 3 must observe the seam working.
	fake := &fakeRequester{status: natsclient.StatusConnected}
	pub := &fakePublisher{}
	c := wireObservationsComponent(t, fake, pub)
	// Squeeze the limit so a small body trips it deterministically.
	c.cfg.MaxRequestBytes = 64

	huge := make([]byte, 2048)
	for i := range huge {
		huge[i] = 'a'
	}
	rr := postObservation(t, c, "ds.001", huge, "application/om+json")
	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("status: got %d want 413; body=%s", rr.Code, rr.Body.String())
	}
	if pub.gotMsg != nil {
		t.Errorf("publish should not be called on oversize body")
	}
}

func TestHandleObservationsPost_PublisherErrorClassification(t *testing.T) {
	tests := []struct {
		name     string
		pubErr   error
		wantCode int
	}{
		{"ErrNoResponders → 503", nats.ErrNoResponders, http.StatusServiceUnavailable},
		{"ErrConnectionClosed → 503", nats.ErrConnectionClosed, http.StatusServiceUnavailable},
		{"DeadlineExceeded → 503", context.DeadlineExceeded, http.StatusServiceUnavailable},
		{"unclassified error → 500", errors.New("something else"), http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeRequester{status: natsclient.StatusConnected}
			pub := &fakePublisher{pubErr: tt.pubErr}
			c := wireObservationsComponent(t, fake, pub)
			body, _ := json.Marshal(validObservation())
			rr := postObservation(t, c, "ds.001", body, "application/om+json")
			if rr.Code != tt.wantCode {
				t.Errorf("status: got %d want %d; body=%s", rr.Code, tt.wantCode, rr.Body.String())
			}
		})
	}
}

func TestHandleObservationsPost_GETReturns405(t *testing.T) {
	// The mux registration is "POST /…/observations" — non-POST should not
	// match the pattern at all. Standard mux returns 405 for the matched
	// path when only the method differs.
	fake := &fakeRequester{status: natsclient.StatusConnected}
	pub := &fakePublisher{}
	c := wireObservationsComponent(t, fake, pub)

	mux := http.NewServeMux()
	c.RegisterHTTPHandlers("", mux)
	req := httptest.NewRequest(http.MethodGet, "/datastreams/ds.001/observations", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("status: got %d want 405", rr.Code)
	}
}
