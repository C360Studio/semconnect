package csapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/parser/sensorml"
	csapivocab "github.com/c360studio/semstreams/vocabulary/csapi"
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

func encodeDatastreamEntityStateWithSchema(t *testing.T, id, name, system, obsProp, artifactID string) []byte {
	t.Helper()
	triples := []message.Triple{
		{Subject: id, Predicate: sensorml.PredType, Object: DatastreamTypeIRI},
		{Subject: id, Predicate: sensorml.PredLabel, Object: name},
		{Subject: id, Predicate: PredDatastreamSystem, Object: system},
		{Subject: id, Predicate: sosa.ObservedProperty, Object: obsProp},
		{Subject: id, Predicate: PredDatastreamSchema, Object: artifactID},
	}
	state := graph.EntityState{ID: id, Triples: triples}
	out, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("encode datastream schema entity state: %v", err)
	}
	return out
}

const testSWEDataRecordSchema = `{"type":"DataRecord","fields":[{"name":"time","type":"Time"},{"name":"temperature","type":"Quantity","uomCode":"Cel"}]}`

func wireSchemaStore(c *Component, store *fakeSchemaObjectStore) {
	var schemaStore schemaObjectStore = store
	c.schemaArtifacts.Store(&schemaStore)
}

func seedSchemaArtifact(t *testing.T, c *Component, store *fakeSchemaObjectStore, artifactID string, raw json.RawMessage) []byte {
	t.Helper()
	canonical, err := normalizeSWESchema(raw)
	if err != nil {
		t.Fatalf("normalize schema artifact: %v", err)
	}
	key := schemaArtifactObjectKey(artifactID)
	if store.puts == nil {
		store.puts = make(map[string][]byte)
	}
	store.puts[key] = append([]byte(nil), canonical...)
	state := graph.EntityState{
		ID: artifactID,
		Triples: []message.Triple{
			{Subject: artifactID, Predicate: sensorml.PredType, Object: csapivocab.SWESchemaDocument},
		},
		StorageRef: &message.StorageReference{
			StorageInstance: c.cfg.SchemaArtifactsBucket,
			Key:             key,
			ContentType:     schemaArtifactContentType,
			Size:            int64(len(canonical)),
		},
	}
	out, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("encode schema artifact entity state: %v", err)
	}
	return out
}

func schemaArtifactIDForTest(c *Component, parentID, relationshipPredicate string) string {
	role, err := schemaArtifactRole(relationshipPredicate)
	if err != nil {
		panic(fmt.Sprintf("test schema relationship: %v", err))
	}
	return c.mintSchemaArtifactEntityID(parentID, role)
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
	// Stage 13 — X-CS-Datastream-Subset retired; framework v1.0.0-beta.75
	// ships native csapi.Datastream vocabulary so the subset disclaimer
	// no longer applies. Negative-assertion pins the deprecation.
	if got := rr.Header().Get("X-CS-Datastream-Subset"); got != "" {
		t.Errorf("X-CS-Datastream-Subset should be unset post-Stage-13; got %q", got)
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
	if len(coll.Items) != len(ids) {
		t.Errorf("items: got %d want %d", len(coll.Items), len(ids))
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
	// Stage 13 — X-CS-Datastream-Subset retired (see collection-shape test).
	if got := rr.Header().Get("X-CS-Datastream-Subset"); got != "" {
		t.Errorf("X-CS-Datastream-Subset should be unset post-Stage-13; got %q", got)
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

func TestHandleDatastream_IncludesSchemaLinkWhenStored(t *testing.T) {
	id := "c360.semconnect.systems.csapi.datastream.001"
	fake := &multiReplyFakeRequester{entityRepliesByID: map[string][]byte{}}
	c := newComponentWithRequester(t, fake)
	store := &fakeSchemaObjectStore{}
	wireSchemaStore(c, store)
	artifactID := schemaArtifactIDForTest(c, id, PredDatastreamSchema)
	fake.entityRepliesByID[id] = encodeDatastreamEntityStateWithSchema(t, id,
		"Temperature feed",
		"c360.semconnect.systems.csapi.system.sensor1",
		"http://example.org/properties/temperature",
		artifactID)
	fake.entityRepliesByID[artifactID] = seedSchemaArtifact(t, c, store, artifactID, json.RawMessage(testSWEDataRecordSchema))

	req := httptest.NewRequest(http.MethodGet, "/datastreams/"+id, nil)
	req.SetPathValue("id", id)
	rr := httptest.NewRecorder()
	c.handleDatastream(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200 (body=%s)", rr.Code, rr.Body.String())
	}
	var d Datastream
	if err := json.Unmarshal(rr.Body.Bytes(), &d); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(d.Schema) == 0 {
		t.Fatalf("schema omitted from datastream response")
	}
	var hasSchemaLink bool
	for _, l := range d.Links {
		if l.Rel == "schema" && l.Href == "/datastreams/"+id+"/schema" {
			hasSchemaLink = true
		}
	}
	if !hasSchemaLink {
		t.Fatalf("schema link missing: %+v", d.Links)
	}
}

func TestHandleDatastreamSchema_GoldenPath(t *testing.T) {
	id := "c360.semconnect.systems.csapi.datastream.001"
	fake := &multiReplyFakeRequester{entityRepliesByID: map[string][]byte{}}
	c := newComponentWithRequester(t, fake)
	store := &fakeSchemaObjectStore{}
	wireSchemaStore(c, store)
	artifactID := schemaArtifactIDForTest(c, id, PredDatastreamSchema)
	fake.entityRepliesByID[id] = encodeDatastreamEntityStateWithSchema(t, id,
		"Temperature feed",
		"c360.semconnect.systems.csapi.system.sensor1",
		"http://example.org/properties/temperature",
		artifactID)
	fake.entityRepliesByID[artifactID] = seedSchemaArtifact(t, c, store, artifactID, json.RawMessage(testSWEDataRecordSchema))

	req := httptest.NewRequest(http.MethodGet, "/datastreams/"+id+"/schema", nil)
	req.SetPathValue("id", id)
	rr := httptest.NewRecorder()
	c.handleDatastreamSchema(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200 (body=%s)", rr.Code, rr.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["type"] != "DataRecord" {
		t.Fatalf("schema type: got %v", body["type"])
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
// at the minted entity ID + triples published to create_with_triples.
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

func TestHandleDatastreamPost_StoresSchema(t *testing.T) {
	fake := &fakeRequester{status: natsclient.StatusConnected, reply: encodeBatchOK(t, 5)}
	c := newTestComponent(t, fake)
	store := &fakeSchemaObjectStore{}
	wireSchemaStore(c, store)

	body, _ := json.Marshal(map[string]any{
		"id":               "tenantA.proj.feeds.csapi.datastream.temp",
		"name":             "Temperature feed",
		"system":           "tenantA.proj.feeds.csapi.system.sensor1",
		"observedProperty": "http://example.org/properties/temperature",
		"schema":           json.RawMessage(testSWEDataRecordSchema),
	})
	req := httptest.NewRequest(http.MethodPost, "/datastreams", bytes.NewReader(body))
	req.Header.Set("Content-Type", string(MediaJSON))
	rr := httptest.NewRecorder()
	c.handleDatastreamPost(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status: got %d want 201 (body=%s)", rr.Code, rr.Body.String())
	}
	var sent graph.AddTriplesBatchRequest
	if err := json.Unmarshal(fake.gotBody, &sent); err != nil {
		t.Fatalf("decode published body: %v", err)
	}
	var artifactID string
	for _, tr := range sent.Triples {
		if tr.Predicate == PredDatastreamSchema {
			artifactID, _ = tr.Object.(string)
		}
	}
	if artifactID == "" {
		t.Fatalf("missing schema artifact relationship: %+v", sent.Triples)
	}
	if !strings.HasPrefix(artifactID, c.cfg.SchemaArtifactIDPrefix+".") {
		t.Fatalf("schema artifact ID: got %q want prefix %q", artifactID, c.cfg.SchemaArtifactIDPrefix)
	}
	if stored := store.puts[schemaArtifactObjectKey(artifactID)]; !json.Valid(stored) {
		t.Fatalf("schema artifact bytes are not JSON: %s", stored)
	}
}

func TestHandleDatastreamPost_InvalidSchema(t *testing.T) {
	fake := &fakeRequester{status: natsclient.StatusConnected}
	c := newTestComponent(t, fake)

	body, _ := json.Marshal(map[string]any{
		"name":             "Bad schema feed",
		"system":           "c360.semconnect.systems.csapi.system.sensor1",
		"observedProperty": "http://example.org/properties/temperature",
		"schema":           map[string]any{"type": "Quantity"},
	})
	req := httptest.NewRequest(http.MethodPost, "/datastreams", bytes.NewReader(body))
	req.Header.Set("Content-Type", string(MediaJSON))
	rr := httptest.NewRecorder()
	c.handleDatastreamPost(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d want 400 (body=%s)", rr.Code, rr.Body.String())
	}
	if fake.gotSubject != "" {
		t.Fatalf("publish should not happen on invalid schema; got %q", fake.gotSubject)
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
