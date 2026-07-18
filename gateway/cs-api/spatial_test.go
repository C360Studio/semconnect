package csapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/c360studio/semstreams/graph/geo/geojson"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/nats-io/nats.go"
)

// boundsReply produces the bare JSON-array wire shape the framework returns
// from graph.spatial.query.bounds. Stage 13: each result carries Lat/Lon
// (Alt omitted — the framework's omitempty drops zeros) since v1.0.0-beta.75
// added coordinate echo to SpatialResult.
func boundsReply(t *testing.T, ids ...string) []byte {
	t.Helper()
	results := make([]spatialResult, 0, len(ids))
	for i, id := range ids {
		// Synthesize per-id coordinates so the test can assert each Feature's
		// geometry matches its source row, not just "non-null".
		results = append(results, spatialResult{
			ID:   id,
			Type: "entity",
			Lat:  37.0 + float64(i)*0.5,
			Lon:  -122.0 + float64(i)*0.5,
		})
	}
	b, err := json.Marshal(results)
	if err != nil {
		t.Fatalf("boundsReply marshal: %v", err)
	}
	return b
}

// spatialFakeRequester captures the subject + body so wire-shape assertions
// can verify what was sent to graph-index-spatial. fakeRequester (defined in
// systems_test.go) already does this; we reuse it.

func TestHandleAreas_BBoxGoldenPath(t *testing.T) {
	fake := &fakeRequester{
		reply:  boundsReply(t, "acme.ops.robotics.gcs.drone.001", "acme.ops.robotics.gcs.drone.002"),
		status: natsclient.StatusConnected,
	}
	c := newTestComponent(t, fake)
	mux := http.NewServeMux()
	c.RegisterHTTPHandlers("", mux)

	req := httptest.NewRequest(http.MethodGet, "/areas?bbox=-180,-90,180,90", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); ct != string(MediaGeoJSON) {
		t.Errorf("Content-Type: got %q want %q (GeoJSON is the FamilySpatial default)", ct, MediaGeoJSON)
	}
	// Stage 13 — X-CS-Geometry-Available retired; framework v1.0.0-beta.75
	// echoes coordinates on SpatialResult and we emit real Point geometry.
	// Negative-assertion pins the deprecation so a future regression that
	// re-adds the header fails locally.
	if avail := rr.Header().Get("X-CS-Geometry-Available"); avail != "" {
		t.Errorf("X-CS-Geometry-Available should be unset post-Stage-13; got %q", avail)
	}

	fc, err := geojson.UnmarshalFeatureCollection(rr.Body.Bytes())
	if err != nil {
		t.Fatalf("decode FeatureCollection: %v; body=%s", err, rr.Body.String())
	}
	if len(fc.Features) != 2 {
		t.Errorf("features: got %d want 2", len(fc.Features))
	}
	// Features carry real Point geometry derived from boundsReply's Lat/Lon.
	for i, f := range fc.Features {
		if f.Geometry == nil {
			t.Errorf("feature[%d].Geometry: got nil want Point", i)
			continue
		}
		pt, ok := f.Geometry.(geojson.Point)
		if !ok {
			t.Errorf("feature[%d].Geometry: got %T want geojson.Point", i, f.Geometry)
			continue
		}
		wantLon := -122.0 + float64(i)*0.5
		wantLat := 37.0 + float64(i)*0.5
		if len(pt.Coordinates) < 2 || pt.Coordinates[0] != wantLon || pt.Coordinates[1] != wantLat {
			t.Errorf("feature[%d].Geometry.Coordinates: got %v want [%v %v]",
				i, pt.Coordinates, wantLon, wantLat)
		}
	}

	// Wire shape: subject + bbox-shaped body must be exact.
	if fake.gotSubject != subjectSpatialBounds {
		t.Errorf("subject: got %q want %q", fake.gotSubject, subjectSpatialBounds)
	}
	var bq boundsQuery
	if err := json.Unmarshal(fake.gotBody, &bq); err != nil {
		t.Fatalf("decode captured body: %v", err)
	}
	if bq.West != -180 || bq.South != -90 || bq.East != 180 || bq.North != 90 {
		t.Errorf("bbox values: %+v", bq)
	}
}

func TestHandleAreas_BBoxJSONAcceptOverridesContentType(t *testing.T) {
	// FamilySpatial supports JSON and GeoJSON; client asking for JSON
	// gets the same FeatureCollection bytes but advertised as JSON.
	fake := &fakeRequester{
		reply:  boundsReply(t, "x"),
		status: natsclient.StatusConnected,
	}
	c := newTestComponent(t, fake)
	mux := http.NewServeMux()
	c.RegisterHTTPHandlers("", mux)

	req := httptest.NewRequest(http.MethodGet, "/areas?bbox=0,0,10,10", nil)
	req.Header.Set("Accept", string(MediaJSON))
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != string(MediaJSON) {
		t.Errorf("Content-Type: got %q want %q", ct, MediaJSON)
	}
	// Body is still valid GeoJSON.
	if _, err := geojson.UnmarshalFeatureCollection(rr.Body.Bytes()); err != nil {
		t.Errorf("body should still be valid GeoJSON: %v", err)
	}
}

func TestHandleAreas_PolygonGoldenPath(t *testing.T) {
	fake := &fakeRequester{
		reply:  boundsReply(t, "acme.ops.robotics.gcs.drone.001"),
		status: natsclient.StatusConnected,
	}
	c := newTestComponent(t, fake)
	mux := http.NewServeMux()
	c.RegisterHTTPHandlers("", mux)

	// Simple unit-square polygon at the origin.
	polyJSON := `{"type":"Polygon","coordinates":[[[0,0],[10,0],[10,10],[0,10],[0,0]]]}`
	url := "/areas?polygon=" + polyJSON // httptest will URL-encode this for us
	req := httptest.NewRequest(http.MethodGet, url, nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	if fake.gotSubject != subjectSpatialPolygon {
		t.Errorf("subject: got %q want %q", fake.gotSubject, subjectSpatialPolygon)
	}
	// Polygon body should round-trip through the framework's
	// UnmarshalGeometry (the handler already validated, so this is just
	// a sanity check on what we forwarded).
	var pq polygonQuery
	if err := json.Unmarshal(fake.gotBody, &pq); err != nil {
		t.Fatalf("decode captured body: %v", err)
	}
	if _, err := geojson.UnmarshalGeometry(pq.Polygon); err != nil {
		t.Errorf("forwarded polygon doesn't parse: %v", err)
	}
}

func TestHandleAreas_RequiresExactlyOneFilter(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want int
	}{
		{"neither bbox nor polygon → 400", "/areas", http.StatusBadRequest},
		{"both bbox and polygon → 400", "/areas?bbox=0,0,1,1&polygon=" + `{"type":"Polygon","coordinates":[[[0,0],[1,0],[1,1],[0,0]]]}`, http.StatusBadRequest},
		{"bbox only → 200", "/areas?bbox=-1,-1,1,1", http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeRequester{
				reply:  boundsReply(t),
				status: natsclient.StatusConnected,
			}
			c := newTestComponent(t, fake)
			mux := http.NewServeMux()
			c.RegisterHTTPHandlers("", mux)

			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, req)
			if rr.Code != tt.want {
				t.Errorf("status: got %d want %d; body=%s", rr.Code, tt.want, rr.Body.String())
			}
		})
	}
}

func TestParseBBox(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantErr bool
		check   func(boundsQuery) bool
	}{
		{"global bbox", "-180,-90,180,90", false, func(b boundsQuery) bool {
			return b.West == -180 && b.South == -90 && b.East == 180 && b.North == 90
		}},
		{"whitespace tolerated", " -10 , -10 , 10 , 10 ", false, func(b boundsQuery) bool {
			return b.West == -10 && b.North == 10
		}},
		{"three values rejected", "0,0,10", true, nil},
		{"five values rejected (no elevation at v0.1)", "0,0,0,10,10,10", true, nil},
		{"non-number rejected", "x,0,1,1", true, nil},
		{"longitude out of range", "200,0,210,10", true, nil},
		{"latitude out of range", "0,-100,10,100", true, nil},
		{"inverted latitude rejected", "0,10,10,0", true, nil},
		{"antimeridian-crossing rejected with hint", "170,-10,-170,10", true, nil},
		// Regression cover for review M-2: NaN bypasses ordered
		// comparisons (IEEE 754) and would slip past every range
		// guard without an explicit IsNaN/IsInf check.
		{"NaN rejected", "NaN,0,10,10", true, nil},
		{"+Inf rejected", "0,0,+Inf,10", true, nil},
		{"-Inf rejected", "-Inf,0,10,10", true, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseBBox(tt.raw)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, tt.wantErr)
			}
			if !tt.wantErr && tt.check != nil && !tt.check(got) {
				t.Errorf("check failed: %+v", got)
			}
		})
	}
}

func TestParsePolygon(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantErr bool
	}{
		{"valid polygon", `{"type":"Polygon","coordinates":[[[0,0],[10,0],[10,10],[0,0]]]}`, false},
		{"non-JSON rejected", "not json", true},
		{"valid GeoJSON but wrong geometry (Point)", `{"type":"Point","coordinates":[0,0]}`, true},
		{"valid GeoJSON but wrong geometry (LineString)", `{"type":"LineString","coordinates":[[0,0],[1,1]]}`, true},
		{"missing type discriminator", `{"coordinates":[[[0,0],[1,0],[1,1],[0,0]]]}`, true},
		// Regression cover for review M-2: framework's UnmarshalGeometry
		// does no ring validation; we enforce RFC 7946 §3.1.6 client-side.
		{"zero rings rejected", `{"type":"Polygon","coordinates":[]}`, true},
		{"ring with <4 points rejected", `{"type":"Polygon","coordinates":[[[0,0],[1,0],[0,0]]]}`, true},
		{"unclosed ring rejected", `{"type":"Polygon","coordinates":[[[0,0],[1,0],[1,1],[0,1]]]}`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parsePolygon(tt.raw)
			if (err != nil) != tt.wantErr {
				t.Errorf("err=%v wantErr=%v", err, tt.wantErr)
			}
		})
	}
}

func TestHandleAreas_NotAcceptable(t *testing.T) {
	fake := &fakeRequester{status: natsclient.StatusConnected}
	c := newTestComponent(t, fake)
	mux := http.NewServeMux()
	c.RegisterHTTPHandlers("", mux)

	req := httptest.NewRequest(http.MethodGet, "/areas?bbox=0,0,1,1", nil)
	req.Header.Set("Accept", "application/sensorml+json") // wrong family
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotAcceptable {
		t.Errorf("status: got %d want 406; body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleAreas_TransientBackendIs503(t *testing.T) {
	fake := &fakeRequester{
		replyErr: nats.ErrNoResponders,
		status:   natsclient.StatusConnected,
	}
	c := newTestComponent(t, fake)
	mux := http.NewServeMux()
	c.RegisterHTTPHandlers("", mux)

	req := httptest.NewRequest(http.MethodGet, "/areas?bbox=0,0,1,1", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("status: got %d want 503", rr.Code)
	}
}

func TestHandleAreas_ClassifiedBackendErrors(t *testing.T) {
	tests := []struct {
		name       string
		errClass   string
		wantStatus int
	}{
		{name: "invalid", errClass: "invalid", wantStatus: http.StatusBadRequest},
		{name: "transient", errClass: "transient", wantStatus: http.StatusServiceUnavailable},
		{name: "fatal", errClass: "fatal", wantStatus: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeRequester{
				reply: []byte(`{"message":"spatial query failed"}`),
				replyHeader: nats.Header{
					natsclient.HeaderStatus:     []string{natsclient.HeaderStatusError},
					natsclient.HeaderErrorClass: []string{tt.errClass},
				},
				status: natsclient.StatusConnected,
			}
			c := newTestComponent(t, fake)
			mux := http.NewServeMux()
			c.RegisterHTTPHandlers("", mux)

			req := httptest.NewRequest(http.MethodGet, "/areas?bbox=0,0,1,1", nil)
			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Errorf("status: got %d want %d; body=%s", rr.Code, tt.wantStatus, rr.Body.String())
			}
		})
	}
}

func TestHandleAreas_MalformedSuccessBodyBecomes500(t *testing.T) {
	for _, body := range []string{`{"data":[]}`, `null`, " \n\tnull\r "} {
		t.Run(body, func(t *testing.T) {
			fake := &fakeRequester{
				reply:  []byte(body),
				status: natsclient.StatusConnected,
			}
			c := newTestComponent(t, fake)
			mux := http.NewServeMux()
			c.RegisterHTTPHandlers("", mux)

			req := httptest.NewRequest(http.MethodGet, "/areas?bbox=0,0,1,1", nil)
			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, req)

			if rr.Code != http.StatusInternalServerError {
				t.Errorf("status: got %d want 500; body=%s", rr.Code, rr.Body.String())
			}
		})
	}
}

func TestHandleAreas_HEAD(t *testing.T) {
	fake := &fakeRequester{
		reply:  boundsReply(t, "x"),
		status: natsclient.StatusConnected,
	}
	c := newTestComponent(t, fake)
	mux := http.NewServeMux()
	c.RegisterHTTPHandlers("", mux)

	req := httptest.NewRequest(http.MethodHead, "/areas?bbox=0,0,1,1", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d", rr.Code)
	}
	if rr.Body.Len() != 0 {
		t.Errorf("HEAD body must be empty; got %d bytes", rr.Body.Len())
	}
	if ct := rr.Header().Get("Content-Type"); ct != string(MediaGeoJSON) {
		t.Errorf("Content-Type: got %q want %q", ct, MediaGeoJSON)
	}
}

func TestHandleAreas_LimitForwarded(t *testing.T) {
	fake := &fakeRequester{
		reply:  boundsReply(t),
		status: natsclient.StatusConnected,
	}
	c := newTestComponent(t, fake)
	mux := http.NewServeMux()
	c.RegisterHTTPHandlers("", mux)

	req := httptest.NewRequest(http.MethodGet, "/areas?bbox=0,0,1,1&limit=42", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d", rr.Code)
	}
	var bq boundsQuery
	if err := json.Unmarshal(fake.gotBody, &bq); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if bq.Limit != 42 {
		t.Errorf("limit: got %d want 42", bq.Limit)
	}
}

func TestHandleAreas_BadLimit(t *testing.T) {
	fake := &fakeRequester{status: natsclient.StatusConnected}
	c := newTestComponent(t, fake)
	mux := http.NewServeMux()
	c.RegisterHTTPHandlers("", mux)

	req := httptest.NewRequest(http.MethodGet, "/areas?bbox=0,0,1,1&limit=abc", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status: got %d want 400", rr.Code)
	}
	if fake.gotSubject != "" {
		t.Errorf("backend should not be called on bad limit; got subject %q", fake.gotSubject)
	}
}
