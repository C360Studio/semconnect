package csapi

import (
	"bytes"
	"context"
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

// encodeDatastreamEntityState builds a graph.EntityState shaped as a
// Datastream (rdf:type=DatastreamTypeIRI + label + system + observed
// property), JSON-encoded as the framework's graph.query.entity replies.
func encodeDatastreamEntityState(t *testing.T, id, name, system, obsProp string) []byte {
	t.Helper()
	triples := []message.Triple{
		{Subject: id, Predicate: sensorml.PredType, Object: DatastreamTypeIRI},
		{Subject: id, Predicate: sensorml.PredLabel, Object: name},
		{Subject: id, Predicate: PredDatastreamSystem, Object: system},
		{Subject: id, Predicate: sosa.ObservedProperty, Object: obsProp},
	}
	state := graph.EntityState{ID: id, Triples: triples}
	out, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("encode datastream entity state: %v", err)
	}
	return out
}

// TestHandleDatastreams_GoldenPath pins the list shape for a populated
// datastream graph.
func TestHandleDatastreams_GoldenPath(t *testing.T) {
	ids := []string{
		"c360.semconnect.systems.csapi.datastream.alpha",
		"c360.semconnect.systems.csapi.datastream.beta",
	}
	fake := &fakeRequester{
		status: natsclient.StatusConnected,
		reply:  encodeReply(t, ids),
	}
	c := newTestComponent(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/datastreams", nil)
	rr := httptest.NewRecorder()
	c.handleDatastreams(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", rr.Code)
	}
	if got := rr.Header().Get("X-CS-Datastream-Subset"); got != "true" {
		t.Errorf("X-CS-Datastream-Subset: got %q want %q", got, "true")
	}
	var coll datastreamCollection
	if err := json.Unmarshal(rr.Body.Bytes(), &coll); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if coll.Type != "DatastreamCollection" {
		t.Errorf("type: got %q want %q", coll.Type, "DatastreamCollection")
	}
	if coll.NumberReturned != len(ids) {
		t.Errorf("numberReturned: got %d want %d", coll.NumberReturned, len(ids))
	}
	if len(coll.Datastreams) != len(ids) {
		t.Errorf("datastreams: got %d want %d", len(coll.Datastreams), len(ids))
	}
}

// TestHandleDatastream_GoldenPath pins the single-datastream JSON shape.
func TestHandleDatastream_GoldenPath(t *testing.T) {
	id := "c360.semconnect.systems.csapi.datastream.001"
	fake := &fakeRequester{
		status: natsclient.StatusConnected,
		reply: encodeDatastreamEntityState(t, id,
			"Temperature feed",
			"c360.semconnect.systems.csapi.system.sensor1",
			"http://example.org/properties/temperature"),
	}
	c := newTestComponent(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/datastreams/"+id, nil)
	req.SetPathValue("id", id)
	rr := httptest.NewRecorder()
	c.handleDatastream(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200 (body=%s)", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("X-CS-Datastream-Subset"); got != "true" {
		t.Errorf("X-CS-Datastream-Subset: got %q want %q", got, "true")
	}
	var d Datastream
	if err := json.Unmarshal(rr.Body.Bytes(), &d); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if d.ID != id || d.Type != "Datastream" {
		t.Errorf("id/type: got %s/%s want %s/Datastream", d.ID, d.Type, id)
	}
	if d.Name != "Temperature feed" {
		t.Errorf("name: got %q", d.Name)
	}
	if d.System != "c360.semconnect.systems.csapi.system.sensor1" {
		t.Errorf("system: got %q", d.System)
	}
	if d.ObservedProperty != "http://example.org/properties/temperature" {
		t.Errorf("observedProperty: got %q", d.ObservedProperty)
	}
}

// TestHandleDatastream_NotDatastreamKind: an entity that exists but is not
// a Datastream returns 404 (preserves CS API §10.4 resource-not-found
// semantics — the URL space owes a 404, not a degraded body).
func TestHandleDatastream_NotDatastreamKind(t *testing.T) {
	id := "c360.semconnect.systems.csapi.system.sensor1"
	// Encode as a System entity, not a Datastream.
	triples := []message.Triple{
		{Subject: id, Predicate: sensorml.PredType, Object: sosa.SSNSystem},
		{Subject: id, Predicate: sensorml.PredLabel, Object: "I'm a System, not a Datastream"},
	}
	state := graph.EntityState{ID: id, Triples: triples}
	stateBytes, _ := json.Marshal(state)

	fake := &fakeRequester{status: natsclient.StatusConnected, reply: stateBytes}
	c := newTestComponent(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/datastreams/"+id, nil)
	req.SetPathValue("id", id)
	rr := httptest.NewRecorder()
	c.handleDatastream(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status: got %d want 404", rr.Code)
	}
}

// TestHandleDatastreamPost_GoldenPath: POST → 201 with Location pointing
// at the minted entity ID + triples published to add_batch.
func TestHandleDatastreamPost_GoldenPath(t *testing.T) {
	fake := &fakeRequester{status: natsclient.StatusConnected, reply: encodeBatchOK(t, 4)}
	c := newTestComponent(t, fake)

	body, _ := json.Marshal(map[string]any{
		"id":               "urn:uuid:22222222-3333-4444-5555-666666666666",
		"name":             "Temperature feed",
		"description":      "Hourly air temp from sensor 1",
		"system":           "c360.semconnect.systems.csapi.system.sensor1",
		"observedProperty": "http://example.org/properties/temperature",
	})
	req := httptest.NewRequest(http.MethodPost, "/datastreams", bytes.NewReader(body))
	req.Header.Set("Content-Type", string(MediaJSON))
	rr := httptest.NewRecorder()
	c.handleDatastreamPost(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status: got %d want 201 (body=%s)", rr.Code, rr.Body.String())
	}
	loc := rr.Header().Get("Location")
	if !strings.HasPrefix(loc, "/datastreams/"+c.cfg.DatastreamIDPrefix+".") {
		t.Errorf("Location: got %q, want /datastreams/<prefix>.<token>", loc)
	}

	// Wire-shape: every triple Subject should be the minted entity ID,
	// and the triple list should contain rdf:type → DatastreamTypeIRI.
	var sent graph.AddTriplesBatchRequest
	if err := json.Unmarshal(fake.gotBody, &sent); err != nil {
		t.Fatalf("decode published body: %v", err)
	}
	mintedID := strings.TrimPrefix(loc, "/datastreams/")
	var hasType bool
	for _, tr := range sent.Triples {
		if tr.Subject != mintedID {
			t.Errorf("triple Subject=%q want %q", tr.Subject, mintedID)
		}
		if tr.Predicate == sensorml.PredType && tr.Object == DatastreamTypeIRI {
			hasType = true
		}
	}
	if !hasType {
		t.Errorf("missing rdf:type triple for DatastreamTypeIRI; triples=%+v", sent.Triples)
	}
}

// TestHandleDatastreamPost_MissingSystem returns 400 — CS API §10.6
// makes the producer link required (orphan datastream is meaningless).
func TestHandleDatastreamPost_MissingSystem(t *testing.T) {
	fake := &fakeRequester{status: natsclient.StatusConnected}
	c := newTestComponent(t, fake)

	body, _ := json.Marshal(map[string]any{
		"name":             "Orphan",
		"observedProperty": "http://example.org/foo",
	})
	req := httptest.NewRequest(http.MethodPost, "/datastreams", bytes.NewReader(body))
	req.Header.Set("Content-Type", string(MediaJSON))
	rr := httptest.NewRecorder()
	c.handleDatastreamPost(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status: got %d want 400", rr.Code)
	}
	if fake.gotSubject != "" {
		t.Errorf("publish should not happen on validation failure; got=%q", fake.gotSubject)
	}
}

// TestHandleDatastreamPost_InvalidSystemRef proves system-ref validation
// catches non-6-part values before publish (prevents an orphan datastream
// pointing at a non-entity).
func TestHandleDatastreamPost_InvalidSystemRef(t *testing.T) {
	fake := &fakeRequester{status: natsclient.StatusConnected}
	c := newTestComponent(t, fake)

	body, _ := json.Marshal(map[string]any{
		"system":           "not-a-6-part-id",
		"observedProperty": "http://example.org/foo",
	})
	req := httptest.NewRequest(http.MethodPost, "/datastreams", bytes.NewReader(body))
	req.Header.Set("Content-Type", string(MediaJSON))
	rr := httptest.NewRecorder()
	c.handleDatastreamPost(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status: got %d want 400", rr.Code)
	}
}

// TestHandleDatastreamPost_MissingObservedProperty returns 400.
func TestHandleDatastreamPost_MissingObservedProperty(t *testing.T) {
	fake := &fakeRequester{status: natsclient.StatusConnected}
	c := newTestComponent(t, fake)

	body, _ := json.Marshal(map[string]any{
		"system": "c360.semconnect.systems.csapi.system.sensor1",
	})
	req := httptest.NewRequest(http.MethodPost, "/datastreams", bytes.NewReader(body))
	req.Header.Set("Content-Type", string(MediaJSON))
	rr := httptest.NewRecorder()
	c.handleDatastreamPost(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status: got %d want 400", rr.Code)
	}
}

// TestHandleDatastreamPost_TransientBackend → 503.
func TestHandleDatastreamPost_TransientBackend(t *testing.T) {
	fake := &fakeRequester{
		status:   natsclient.StatusConnected,
		replyErr: context.DeadlineExceeded,
	}
	c := newTestComponent(t, fake)

	body, _ := json.Marshal(map[string]any{
		"system":           "c360.semconnect.systems.csapi.system.sensor1",
		"observedProperty": "http://example.org/properties/temperature",
	})
	req := httptest.NewRequest(http.MethodPost, "/datastreams", bytes.NewReader(body))
	req.Header.Set("Content-Type", string(MediaJSON))
	rr := httptest.NewRecorder()
	c.handleDatastreamPost(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("status: got %d want 503", rr.Code)
	}
}

// TestHandleDatastreamPost_SystemRefStrictRejectsLooseShape covers the
// strict-vs-lax distinction in validateEntityIDStrict. A value that
// passes validateEntityID (NATS-token-safe, non-empty tokens) but fails
// validateEntityIDStrict (wrong dot count, bad token shape) must be
// rejected at the gateway boundary — otherwise the request would land
// at graph-ingest and produce a confusing 500.
func TestHandleDatastreamPost_SystemRefStrictRejectsLooseShape(t *testing.T) {
	cases := []struct {
		name      string
		systemRef string
	}{
		// 5 tokens — passes lax (no NATS-reserved chars, no empty
		// tokens) but fails strict (wrong dot count).
		{"5 tokens", "a.b.c.d.e"},
		// 6 tokens but a leading underscore token — passes lax but
		// fails strict (token regex requires alphanumeric start).
		{"6 tokens leading underscore", "_a.b.c.d.e.f"},
		// 7 tokens — passes lax but fails strict (wrong dot count).
		{"7 tokens", "a.b.c.d.e.f.g"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeRequester{status: natsclient.StatusConnected}
			c := newTestComponent(t, fake)
			body, _ := json.Marshal(map[string]any{
				"system":           tc.systemRef,
				"observedProperty": "http://example.org/foo",
			})
			req := httptest.NewRequest(http.MethodPost, "/datastreams", bytes.NewReader(body))
			req.Header.Set("Content-Type", string(MediaJSON))
			rr := httptest.NewRecorder()
			c.handleDatastreamPost(rr, req)
			if rr.Code != http.StatusBadRequest {
				t.Errorf("status: got %d want 400 (body=%s)", rr.Code, rr.Body.String())
			}
			if fake.gotSubject != "" {
				t.Errorf("publish should not happen on strict-validation failure; got=%q", fake.gotSubject)
			}
		})
	}
}

// TestHandleDatastreamPost_HonorsClientSuppliedSixPartID proves the
// federation idiom: a client that supplies an authoritative 6-part ID
// in the request body gets that ID stored, NOT a re-minted prefix.id.
// Pins B-2 (review): without honoring, federated tenants can't keep
// their own ID space across multiple deployments.
func TestHandleDatastreamPost_HonorsClientSuppliedSixPartID(t *testing.T) {
	fake := &fakeRequester{status: natsclient.StatusConnected, reply: encodeBatchOK(t, 4)}
	c := newTestComponent(t, fake)

	clientID := "tenantA.proj.feeds.csapi.datastream.thermistor1"
	body, _ := json.Marshal(map[string]any{
		"id":               clientID,
		"name":             "Tenant A thermistor",
		"system":           "tenantA.proj.feeds.csapi.system.sensor1",
		"observedProperty": "http://example.org/properties/temperature",
	})
	req := httptest.NewRequest(http.MethodPost, "/datastreams", bytes.NewReader(body))
	req.Header.Set("Content-Type", string(MediaJSON))
	rr := httptest.NewRecorder()
	c.handleDatastreamPost(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status: got %d want 201 (body=%s)", rr.Code, rr.Body.String())
	}
	loc := rr.Header().Get("Location")
	if loc != "/datastreams/"+clientID {
		t.Errorf("Location: got %q want %q (server should honor client-supplied 6-part id)", loc, "/datastreams/"+clientID)
	}
}

// TestDatastreamRoundTrip: build via datastreamToTriples → re-parse via
// datastreamFromState → fields match. Guards against drift between the
// forward and reverse encoders.
func TestDatastreamRoundTrip(t *testing.T) {
	id := "c360.semconnect.systems.csapi.datastream.rt"
	in := Datastream{
		ID:               id,
		Type:             "Datastream",
		Name:             "RT",
		Description:      "round-trip",
		System:           "c360.semconnect.systems.csapi.system.s1",
		ObservedProperty: "http://example.org/x",
	}
	triples := datastreamToTriples(id, &in)
	out := datastreamFromState(graph.EntityState{ID: id, Triples: triples})

	if out.Name != in.Name {
		t.Errorf("name: %q vs %q", out.Name, in.Name)
	}
	if out.Description != in.Description {
		t.Errorf("description: %q vs %q", out.Description, in.Description)
	}
	if out.System != in.System {
		t.Errorf("system: %q vs %q", out.System, in.System)
	}
	if out.ObservedProperty != in.ObservedProperty {
		t.Errorf("observedProperty: %q vs %q", out.ObservedProperty, in.ObservedProperty)
	}
}
