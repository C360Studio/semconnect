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
	"github.com/c360studio/semstreams/natsclient"
)

// sensorMLWithPosition extends minimalSensorML with a top-level
// `position` field carrying a GeoJSON Point. Mirrors what the
// conformance fixture (conformance/fixtures/system.sml.json) ships and
// what the Botts ETS `systemItemHasGeometryOrValidTime` test reads back
// from `/systems/{id}`.
func sensorMLWithPosition(uniqueID, label string, lon, lat, alt float64) []byte {
	body := map[string]any{
		"type":     "PhysicalSystem",
		"id":       "doc-id-1",
		"uniqueId": uniqueID,
		"label":    label,
		"position": map[string]any{
			"type":        "Point",
			"coordinates": []float64{lon, lat, alt},
		},
	}
	out, _ := json.Marshal(body)
	return out
}

// TestExtractPositionTriple pins the helper that peeks the raw body
// for `position` — Stage 14 sister-side workaround. Direct unit test
// (no HTTP) so the parsing behavior is exercised independent of the
// full handler.
func TestExtractPositionTriple(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		wantOK  bool
		wantObj string
	}{
		{
			name:    "Point position present",
			body:    `{"type":"PhysicalSystem","position":{"type":"Point","coordinates":[-122.4,37.8,10]}}`,
			wantOK:  true,
			wantObj: `{"type":"Point","coordinates":[-122.4,37.8,10]}`,
		},
		{
			name:    "Polygon position present",
			body:    `{"type":"PhysicalSystem","position":{"type":"Polygon","coordinates":[[[0,0],[1,0],[1,1],[0,0]]]}}`,
			wantOK:  true,
			wantObj: `{"type":"Polygon","coordinates":[[[0,0],[1,0],[1,1],[0,0]]]}`,
		},
		{
			name:    "position absent",
			body:    `{"type":"PhysicalSystem","label":"no position"}`,
			wantOK:  false,
			wantObj: "",
		},
		{
			name:    "position literal null skipped",
			body:    `{"type":"PhysicalSystem","position":null}`,
			wantOK:  false,
			wantObj: "",
		},
		{
			name:    "invalid outer JSON returns false (don't crash)",
			body:    `not json`,
			wantOK:  false,
			wantObj: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tri, ok := extractPositionTriple("entity-x", []byte(tc.body))
			if ok != tc.wantOK {
				t.Fatalf("ok: got %v want %v", ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if tri.Subject != "entity-x" {
				t.Errorf("Subject: got %q want entity-x", tri.Subject)
			}
			if tri.Predicate != PredSystemPosition {
				t.Errorf("Predicate: got %q want %q", tri.Predicate, PredSystemPosition)
			}
			if got, _ := tri.Object.(string); got != tc.wantObj {
				t.Errorf("Object: got %q want %q", got, tc.wantObj)
			}
		})
	}
}

// TestHandleSystemPost_PositionForwardedAsTriple — full POST handler
// path: a SensorML body with `position` results in a
// cs-api.system.position triple in the request to
// graph.mutation.triple.add_batch. The triple's Object preserves the
// exact GeoJSON bytes (including number precision and field ordering)
// from the input.
func TestHandleSystemPost_PositionForwardedAsTriple(t *testing.T) {
	fake := &fakeRequester{
		status: natsclient.StatusConnected,
		reply:  encodeBatchOK(t, 5),
	}
	c := newTestComponent(t, fake)

	req := httptest.NewRequest(http.MethodPost, "/systems",
		bytes.NewReader(sensorMLWithPosition(
			"urn:uuid:11111111-2222-3333-4444-555555555555",
			"Test System with position",
			-122.4194, 37.7749, 10.0)))
	req.Header.Set("Content-Type", string(MediaSensorML))
	rr := httptest.NewRecorder()
	c.handleSystemPost(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status: got %d want 201 (body=%s)", rr.Code, rr.Body.String())
	}

	var sent graph.AddTriplesBatchRequest
	if err := json.Unmarshal(fake.gotBody, &sent); err != nil {
		t.Fatalf("decode published body: %v", err)
	}
	var found *message.Triple
	for i, tr := range sent.Triples {
		if tr.Predicate == PredSystemPosition {
			found = &sent.Triples[i]
			break
		}
	}
	if found == nil {
		t.Fatal("no cs-api.system.position triple published")
	}
	obj, ok := found.Object.(string)
	if !ok {
		t.Fatalf("position Object: got %T want string", found.Object)
	}
	// Preservation: parse back to a Point and confirm coordinates.
	var pt struct {
		Type        string    `json:"type"`
		Coordinates []float64 `json:"coordinates"`
	}
	if err := json.Unmarshal([]byte(obj), &pt); err != nil {
		t.Fatalf("position Object not valid GeoJSON: %v (body=%s)", err, obj)
	}
	if pt.Type != "Point" {
		t.Errorf("position type: got %q want Point", pt.Type)
	}
	if len(pt.Coordinates) != 3 || pt.Coordinates[0] != -122.4194 || pt.Coordinates[1] != 37.7749 || pt.Coordinates[2] != 10.0 {
		t.Errorf("position coordinates: got %v want [-122.4194 37.7749 10]", pt.Coordinates)
	}
}

// TestHandleSystemPost_PositionAbsent — when the input has no position,
// no cs-api.system.position triple is added. Regression guard against
// emitting a noise triple with an empty Object value.
func TestHandleSystemPost_PositionAbsent(t *testing.T) {
	fake := &fakeRequester{
		status: natsclient.StatusConnected,
		reply:  encodeBatchOK(t, 3),
	}
	c := newTestComponent(t, fake)

	req := httptest.NewRequest(http.MethodPost, "/systems",
		bytes.NewReader(minimalSensorML(
			"urn:uuid:22222222-3333-4444-5555-666666666666",
			"no position here")))
	req.Header.Set("Content-Type", string(MediaSensorML))
	rr := httptest.NewRecorder()
	c.handleSystemPost(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status: got %d want 201", rr.Code)
	}
	var sent graph.AddTriplesBatchRequest
	if err := json.Unmarshal(fake.gotBody, &sent); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, tr := range sent.Triples {
		if tr.Predicate == PredSystemPosition {
			t.Errorf("unexpected position triple: %+v", tr)
		}
	}
}

// TestSystemFromState_SurfacesGeometry — the read side: a state with
// a cs-api.system.position triple produces a System JSON with the
// `geometry` field carrying the raw GeoJSON bytes (no re-quoting,
// no shape mangling).
func TestSystemFromState_SurfacesGeometry(t *testing.T) {
	state := graph.EntityState{
		ID: "c360.semconnect.systems.csapi.system.abc",
		Triples: []message.Triple{
			{Predicate: "sensorml.process.type", Object: "http://www.w3.org/ns/ssn/System"},
			{Predicate: "sensorml.process.label", Object: "My System"},
			{Predicate: PredSystemPosition, Object: `{"type":"Point","coordinates":[-122.4,37.8,10]}`},
		},
	}
	sys := systemFromState(state)
	if sys.Geometry == nil {
		t.Fatal("geometry: got nil want non-nil")
	}
	// Strict byte-equality: the read side must not re-marshal through
	// interface{} and lose number precision / key ordering.
	want := `{"type":"Point","coordinates":[-122.4,37.8,10]}`
	if string(sys.Geometry) != want {
		t.Errorf("geometry bytes:\n got %s\nwant %s", string(sys.Geometry), want)
	}
	// And the marshaled JSON should embed the geometry as an object
	// (not a JSON string literal — that would be the re-quoting bug).
	out, err := json.Marshal(sys)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(out), `"geometry":{"type":"Point","coordinates":[-122.4,37.8,10]}`) {
		t.Errorf("marshaled system doesn't contain expected geometry block:\n%s", out)
	}
}

// TestHandleSystem_SensorMLContentTypeEchoesNegotiated — pin that
// the negotiated SensorML form (spec `application/sml+json` vs
// legacy `application/sensorml+json`) is what comes back as
// Content-Type. Without this test, a regression that hardcodes
// `string(MediaSensorML)` in writeSystemSensorML would pass every
// other test but break the long-form client experience.
func TestHandleSystem_SensorMLContentTypeEchoesNegotiated(t *testing.T) {
	id := "c360.semconnect.systems.csapi.system.abc"
	stateBytes := encodeSystemEntityStateMinimal(t, id)

	cases := []struct {
		name      string
		accept    string
		wantMedia MediaType
	}{
		{"spec form sml+json echoes spec form", "application/sml+json", MediaSensorML},
		{"legacy long form echoes legacy", "application/sensorml+json", MediaSensorMLLegacy},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeRequester{
				status: natsclient.StatusConnected,
				reply:  stateBytes,
			}
			c := newTestComponent(t, fake)

			req := httptest.NewRequest(http.MethodGet, "/systems/"+id, nil)
			req.SetPathValue("id", id)
			req.Header.Set("Accept", tc.accept)
			rr := httptest.NewRecorder()
			c.handleSystem(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("status: got %d want 200 (body=%s)", rr.Code, rr.Body.String())
			}
			if got := rr.Header().Get("Content-Type"); got != string(tc.wantMedia) {
				t.Errorf("Content-Type: got %q want %q", got, tc.wantMedia)
			}
		})
	}
}

// encodeSystemEntityStateMinimal — minimum triple set that satisfies
// isSystemKind so handleSystem reaches writeSystemSensorML.
func encodeSystemEntityStateMinimal(t *testing.T, id string) []byte {
	t.Helper()
	state := graph.EntityState{
		ID: id,
		Triples: []message.Triple{
			{Predicate: "sensorml.process.type", Object: "http://www.w3.org/ns/ssn/System"},
			{Predicate: "sensorml.process.label", Object: "Test"},
		},
	}
	b, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal state: %v", err)
	}
	return b
}

// TestSystemFromState_NoGeometryWhenAbsent — without the triple,
// `geometry` is omitted from the JSON output (omitempty).
func TestSystemFromState_NoGeometryWhenAbsent(t *testing.T) {
	state := graph.EntityState{
		ID: "c360.semconnect.systems.csapi.system.no-pos",
		Triples: []message.Triple{
			{Predicate: "sensorml.process.type", Object: "http://www.w3.org/ns/ssn/System"},
			{Predicate: "sensorml.process.label", Object: "no pos"},
		},
	}
	sys := systemFromState(state)
	if sys.Geometry != nil {
		t.Errorf("geometry: got %s want nil", string(sys.Geometry))
	}
	out, _ := json.Marshal(sys)
	if strings.Contains(string(out), `"geometry"`) {
		t.Errorf("marshaled system should omit `geometry` field when absent:\n%s", out)
	}
}
