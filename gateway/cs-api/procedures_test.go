// Stage 20 — Procedures handler tests. Re-uses fakeRequester from
// systems_test.go (same package).
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
	"github.com/c360studio/semstreams/parser/sensorml"
	"github.com/c360studio/semstreams/vocabulary/sosa"
)

const testProcedureID = "c360.semconnect.systems.csapi.procedure.test01"

// TestHandleProcedures_GoldenPath — GET /procedures returns 200 with
// ProcedureCollection wrapping discovered IDs.
func TestHandleProcedures_GoldenPath(t *testing.T) {
	fake := &fakeRequester{
		reply:  encodeReply(t, []string{testProcedureID, "c360.semconnect.systems.csapi.procedure.test02"}),
		status: natsclient.StatusConnected,
	}
	c := newTestComponent(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/procedures", nil)
	rr := httptest.NewRecorder()
	c.middleware(http.HandlerFunc(c.handleProcedures)).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	var coll procedureCollection
	if err := json.Unmarshal(rr.Body.Bytes(), &coll); err != nil {
		t.Fatalf("decode: %v; body=%s", err, rr.Body.String())
	}
	if coll.Type != "ProcedureCollection" {
		t.Errorf("Type: got %q want ProcedureCollection", coll.Type)
	}
	if coll.NumberMatched != 2 || coll.NumberReturned != 2 {
		t.Errorf("counts: matched=%d returned=%d want 2/2", coll.NumberMatched, coll.NumberReturned)
	}
	if len(coll.Items) != 2 || coll.Items[0].ID != testProcedureID {
		t.Errorf("Items: %+v", coll.Items)
	}
	// Pin the predicate-query reply targets sosa.Procedure (not
	// sosa.SSNSystem) so /procedures and /systems don't accidentally
	// collide on the same predicate object.
	if !strings.Contains(string(fake.gotBody), sosa.Procedure) {
		t.Errorf("predicate-query body should reference sosa.Procedure %q; got %s",
			sosa.Procedure, string(fake.gotBody))
	}
}

// TestHandleProcedures_NonJSONAccept406 — collection endpoint
// honestly 406s on unsupported Accept (sml+json / xml are not
// negotiable on the collection).
func TestHandleProcedures_NonJSONAccept406(t *testing.T) {
	fake := &fakeRequester{status: natsclient.StatusConnected}
	c := newTestComponent(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/procedures", nil)
	req.Header.Set("Accept", "application/xml")
	rr := httptest.NewRecorder()
	c.handleProcedures(rr, req)

	if rr.Code != http.StatusNotAcceptable {
		t.Errorf("status: got %d want 406; body=%s", rr.Code, rr.Body.String())
	}
}

// TestHandleProcedures_GeoJSON — Stage 20.1 / required by the
// ETS's procedureFeatureHasGeoJsonSchemaAndMapping assertion.
// Accept: application/geo+json returns a FeatureCollection where
// every Feature carries `geometry: null` per /req/procedure/location.
func TestHandleProcedures_GeoJSON(t *testing.T) {
	ids := []string{
		"c360.semconnect.systems.csapi.procedure.alpha",
		"c360.semconnect.systems.csapi.procedure.beta",
	}
	fake := &multiReplyFakeRequester{
		predicateReply: encodeReply(t, ids),
	}
	c := newComponentWithRequester(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/procedures", nil)
	req.Header.Set("Accept", string(MediaGeoJSON))
	rr := httptest.NewRecorder()
	c.handleProcedures(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); ct != string(MediaGeoJSON) {
		t.Errorf("Content-Type: got %q want %q", ct, MediaGeoJSON)
	}
	// Decode loosely so we can assert the literal-null shape on
	// every Feature's geometry.
	var fc struct {
		Type     string `json:"type"`
		Features []struct {
			Type       string          `json:"type"`
			ID         string          `json:"id"`
			Geometry   json.RawMessage `json:"geometry"`
			Properties map[string]any  `json:"properties"`
		} `json:"features"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &fc); err != nil {
		t.Fatalf("decode: %v; body=%s", err, rr.Body.String())
	}
	if fc.Type != "FeatureCollection" {
		t.Errorf("type: got %q want FeatureCollection", fc.Type)
	}
	if len(fc.Features) != 2 {
		t.Fatalf("features: got %d want 2", len(fc.Features))
	}
	for _, f := range fc.Features {
		if string(f.Geometry) != "null" {
			t.Errorf("Feature %s geometry should be literal null; got %q", f.ID, f.Geometry)
		}
		if f.Properties["featureType"] != "Procedure" {
			t.Errorf("Feature %s featureType: got %v want Procedure", f.ID, f.Properties["featureType"])
		}
	}
}

// TestHandleProcedure_JSON — GET /procedures/{id} returns the
// reconstructed JSON shape with id/type/links.
func TestHandleProcedure_JSON(t *testing.T) {
	state := graph.EntityState{
		ID: testProcedureID,
		Triples: []message.Triple{
			{Subject: testProcedureID, Predicate: sensorml.PredType, Object: sosa.Procedure},
			{Subject: testProcedureID, Predicate: sensorml.PredLabel, Object: "Calibration procedure"},
			{Subject: testProcedureID, Predicate: PredSystemUID, Object: "urn:example:proc:calibration"},
		},
	}
	fake := &fakeRequester{
		reply:  encodeEntityState(t, state),
		status: natsclient.StatusConnected,
	}
	c := newTestComponent(t, fake)
	mux := http.NewServeMux()
	c.RegisterHTTPHandlers("", mux)

	req := httptest.NewRequest(http.MethodGet, "/procedures/"+testProcedureID, nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	if rr.Header().Get("X-CS-Reconstructed-Lossy") != "true" {
		t.Errorf("X-CS-Reconstructed-Lossy missing — every reverse-mapping response carries it")
	}
	var p procedure
	if err := json.Unmarshal(rr.Body.Bytes(), &p); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if p.Type != "Procedure" {
		t.Errorf("Type: got %q want Procedure", p.Type)
	}
	if p.Label != "Calibration procedure" {
		t.Errorf("Label: got %q", p.Label)
	}
	if p.UID != "urn:example:proc:calibration" || p.UniqueID != "urn:example:proc:calibration" {
		t.Errorf("uid/uniqueId: got %q/%q want urn:example:proc:calibration", p.UID, p.UniqueID)
	}
	// /req/procedure/location says procedures MUST NOT carry geometry.
	// Pin that the JSON body has no geometry field at all.
	if bytes.Contains(rr.Body.Bytes(), []byte(`"geometry"`)) {
		t.Errorf("procedure JSON must not contain geometry; got %s", rr.Body.String())
	}
	// Canonical link required by CS API §6.
	var hasCanonical bool
	for _, l := range p.Links {
		if l.Rel == "canonical" {
			hasCanonical = true
		}
	}
	if !hasCanonical {
		t.Errorf("links missing rel=canonical: %+v", p.Links)
	}
}

// TestHandleProcedure_NotAProcedureKind — entity exists but isn't a
// Procedure → 404.
func TestHandleProcedure_NotAProcedureKind(t *testing.T) {
	state := graph.EntityState{
		ID: testProcedureID,
		Triples: []message.Triple{
			{Subject: testProcedureID, Predicate: sensorml.PredType, Object: sosa.SSNSystem},
		},
	}
	fake := &fakeRequester{
		reply:  encodeEntityState(t, state),
		status: natsclient.StatusConnected,
	}
	c := newTestComponent(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/procedures/"+testProcedureID, nil)
	req.SetPathValue("id", testProcedureID)
	rr := httptest.NewRecorder()
	c.handleProcedure(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status: got %d want 404; body=%s", rr.Code, rr.Body.String())
	}
}

// TestHandleProcedurePost_Feature_GoldenPath — POST a JSON Feature
// to /procedures mints + publishes a sosa.Procedure entity.
func TestHandleProcedurePost_Feature_GoldenPath(t *testing.T) {
	fake := &fakeRequester{
		status: natsclient.StatusConnected,
		reply:  encodeBatchOK(t, 3),
	}
	c := newTestComponent(t, fake)

	body := []byte(`{"type":"Feature","properties":{"uid":"urn:example:proc:cal1","name":"Cal-1","description":"Daily calibration"}}`)
	req := httptest.NewRequest(http.MethodPost, "/procedures", bytes.NewReader(body))
	req.Header.Set("Content-Type", string(MediaJSON))
	rr := httptest.NewRecorder()
	c.handleProcedurePost(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status: got %d want 201; body=%s", rr.Code, rr.Body.String())
	}
	loc := rr.Header().Get("Location")
	if !strings.HasPrefix(loc, "/procedures/"+c.cfg.ProcedureIDPrefix+".") {
		t.Errorf("Location: got %q, want /procedures/<prefix>.<token>", loc)
	}

	// Pin that the publish batch carries sosa.Procedure (not SSNSystem)
	// as the rdf:type triple — collisions with /systems would be very
	// confusing.
	var batch graph.AddTriplesBatchRequest
	if err := json.Unmarshal(fake.gotBody, &batch); err != nil {
		t.Fatalf("decode publish body: %v", err)
	}
	var sawProcedureType, sawUID, sawPosition bool
	for _, tr := range batch.Triples {
		if tr.Predicate == sensorml.PredType {
			if s, ok := tr.Object.(string); ok && s == sosa.Procedure {
				sawProcedureType = true
			}
		}
		if tr.Predicate == PredSystemUID {
			sawUID = true
		}
		if tr.Predicate == PredSystemPosition {
			sawPosition = true
		}
	}
	if !sawProcedureType {
		t.Errorf("rdf:type triple object should be sosa.Procedure; batch=%+v", batch.Triples)
	}
	if !sawUID {
		t.Errorf("cs-api.system.uid triple missing; batch=%+v", batch.Triples)
	}
	if sawPosition {
		t.Errorf("/req/procedure/location forbids position; batch should not contain PredSystemPosition: %+v", batch.Triples)
	}
}

// TestHandleProcedurePost_Sensorml — POST a SensorML SimpleProcess
// body and confirm rdf:type is overridden to sosa.Procedure even if
// the framework's emission was for a different class.
func TestHandleProcedurePost_Sensorml(t *testing.T) {
	fake := &fakeRequester{
		status: natsclient.StatusConnected,
		reply:  encodeBatchOK(t, 3),
	}
	c := newTestComponent(t, fake)

	body := minimalSensorML("urn:example:proc:sml:1", "SML procedure")
	req := httptest.NewRequest(http.MethodPost, "/procedures", bytes.NewReader(body))
	req.Header.Set("Content-Type", string(MediaSensorML))
	rr := httptest.NewRecorder()
	c.handleProcedurePost(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status: got %d want 201; body=%s", rr.Code, rr.Body.String())
	}
	var batch graph.AddTriplesBatchRequest
	_ = json.Unmarshal(fake.gotBody, &batch)
	for _, tr := range batch.Triples {
		if tr.Predicate == sensorml.PredType {
			if s, ok := tr.Object.(string); ok && s != sosa.Procedure {
				t.Errorf("rdf:type should be overridden to sosa.Procedure; got %q", s)
			}
		}
	}
}

// TestHandleProcedurePost_MissingUID returns 400 on Feature path.
func TestHandleProcedurePost_MissingUID(t *testing.T) {
	fake := &fakeRequester{status: natsclient.StatusConnected}
	c := newTestComponent(t, fake)

	body := []byte(`{"type":"Feature","properties":{"name":"No uid"}}`)
	req := httptest.NewRequest(http.MethodPost, "/procedures", bytes.NewReader(body))
	req.Header.Set("Content-Type", string(MediaJSON))
	rr := httptest.NewRecorder()
	c.handleProcedurePost(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status: got %d want 400; body=%s", rr.Code, rr.Body.String())
	}
}

// TestHandleProceduresOptions advertises GET/HEAD/POST/OPTIONS.
// PUT/DELETE/PATCH intentionally absent at v0.1.
func TestHandleProceduresOptions(t *testing.T) {
	fake := &fakeRequester{status: natsclient.StatusConnected}
	c := newTestComponent(t, fake)

	req := httptest.NewRequest(http.MethodOptions, "/procedures", nil)
	rr := httptest.NewRecorder()
	c.handleProceduresOptions(rr, req)

	want := "GET, HEAD, POST, OPTIONS"
	if got := rr.Header().Get("Allow"); got != want {
		t.Errorf("Allow: got %q want %q", got, want)
	}
	if strings.Contains(rr.Header().Get("Allow"), "PUT") {
		t.Errorf("PUT must not be advertised on /procedures at v0.1")
	}
}

// TestHandleProcedurePost_GeoJSONContentType pins the geo+json
// branch of the Content-Type guard.
func TestHandleProcedurePost_GeoJSONContentType(t *testing.T) {
	fake := &fakeRequester{
		status: natsclient.StatusConnected,
		reply:  encodeBatchOK(t, 3),
	}
	c := newTestComponent(t, fake)

	body := []byte(`{"type":"Feature","properties":{"uid":"urn:example:proc:geo","name":"Geo-typed"}}`)
	req := httptest.NewRequest(http.MethodPost, "/procedures", bytes.NewReader(body))
	req.Header.Set("Content-Type", string(MediaGeoJSON))
	rr := httptest.NewRecorder()
	c.handleProcedurePost(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("status: got %d want 201; body=%s", rr.Code, rr.Body.String())
	}
}

// TestHandleProcedurePost_WrongContentType_AdvertisesAcceptPost — 415
// must advertise all four accepted media types.
func TestHandleProcedurePost_WrongContentType_AdvertisesAcceptPost(t *testing.T) {
	fake := &fakeRequester{status: natsclient.StatusConnected}
	c := newTestComponent(t, fake)

	req := httptest.NewRequest(http.MethodPost, "/procedures", bytes.NewReader([]byte(`{}`)))
	req.Header.Set("Content-Type", "application/xml")
	rr := httptest.NewRecorder()
	c.handleProcedurePost(rr, req)

	if rr.Code != http.StatusUnsupportedMediaType {
		t.Errorf("status: got %d want 415; body=%s", rr.Code, rr.Body.String())
	}
	want := strings.Join([]string{
		string(MediaSensorML), string(MediaSensorMLLegacy),
		string(MediaJSON), string(MediaGeoJSON),
	}, ", ")
	if got := rr.Header().Get("Accept-Post"); got != want {
		t.Errorf("Accept-Post: got %q want %q", got, want)
	}
}

// TestHandleSystem_RejectsProcedureEntity — Stage 20 cross-resource
// collision guard. A procedure entity (rdf:type = sosa.Procedure) at
// the same backend MUST NOT be served via /systems/{id}. Pre-Stage-20
// isSystemKind whitelisted sosa.Procedure as a System kind; that
// concession is removed because /procedures owns those entities now.
func TestHandleSystem_RejectsProcedureEntity(t *testing.T) {
	state := graph.EntityState{
		ID: testProcedureID,
		Triples: []message.Triple{
			{Subject: testProcedureID, Predicate: sensorml.PredType, Object: sosa.Procedure},
			{Subject: testProcedureID, Predicate: sensorml.PredLabel, Object: "Procedure entity"},
		},
	}
	fake := &fakeRequester{
		reply:  encodeEntityState(t, state),
		status: natsclient.StatusConnected,
	}
	c := newTestComponent(t, fake)
	mux := http.NewServeMux()
	c.RegisterHTTPHandlers("", mux)

	req := httptest.NewRequest(http.MethodGet, "/systems/"+testProcedureID, nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status: got %d want 404 (Procedure must not be served via /systems); body=%s",
			rr.Code, rr.Body.String())
	}
}

// TestHandleProcedureOptions advertises GET/HEAD/OPTIONS only.
func TestHandleProcedureOptions(t *testing.T) {
	fake := &fakeRequester{status: natsclient.StatusConnected}
	c := newTestComponent(t, fake)

	req := httptest.NewRequest(http.MethodOptions, "/procedures/"+testProcedureID, nil)
	req.SetPathValue("id", testProcedureID)
	rr := httptest.NewRecorder()
	c.handleProcedureOptions(rr, req)

	want := "GET, HEAD, OPTIONS"
	if got := rr.Header().Get("Allow"); got != want {
		t.Errorf("Allow: got %q want %q", got, want)
	}
}
