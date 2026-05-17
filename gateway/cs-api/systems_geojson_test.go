package csapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/graph/geo/geojson"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// multiReplyFakeRequester returns different replies per subject —
// needed for the GeoJSON path on /systems which makes N+1 requests:
// one predicate-query, then one entity-query per matching system.
//
// Per-entity replies are looked up by the entity ID extracted from the
// entity-query request body. The predicate-query reply is the
// `predicateReply` field. Falls back to an error reply on miss so a
// test that forgets to seed a state for an enumerated entity fails
// loudly instead of silently dropping a Feature.
type multiReplyFakeRequester struct {
	predicateReply     []byte
	entityRepliesByID  map[string][]byte
	entityErrorsByID   map[string]error
	predicateErr       error
	calls              int // request count for assertions about N+1
}

func (f *multiReplyFakeRequester) Request(_ context.Context, subj string, data []byte, _ time.Duration) ([]byte, error) {
	f.calls++
	switch subj {
	case subjectPredicateQuery:
		if f.predicateErr != nil {
			return nil, f.predicateErr
		}
		return f.predicateReply, nil
	case subjectEntityQuery:
		var req struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(data, &req); err != nil {
			return nil, errors.New("multiReplyFakeRequester: malformed entity-query body")
		}
		if e, ok := f.entityErrorsByID[req.ID]; ok {
			return nil, e
		}
		if r, ok := f.entityRepliesByID[req.ID]; ok {
			return r, nil
		}
		return nil, errors.New("multiReplyFakeRequester: no reply seeded for entity " + req.ID)
	}
	return nil, errors.New("multiReplyFakeRequester: unexpected subject " + subj)
}

func (f *multiReplyFakeRequester) RequestWithHeaders(_ context.Context, _ string, _ []byte, _ map[string]string, _ time.Duration) (*nats.Msg, error) {
	return nil, errors.New("multiReplyFakeRequester: not exercised by GeoJSON tests")
}
func (f *multiReplyFakeRequester) Status() natsclient.ConnectionStatus {
	return natsclient.StatusConnected
}
func (f *multiReplyFakeRequester) JetStream() (jetstream.JetStream, error) {
	return nil, errors.New("multiReplyFakeRequester: JetStream not exercised")
}
func (f *multiReplyFakeRequester) EnsureStream(_ context.Context, _ jetstream.StreamConfig) (jetstream.Stream, error) {
	return nil, errors.New("multiReplyFakeRequester: EnsureStream not exercised")
}

// newComponentWithRequester is newTestComponent's interface-typed
// sibling — same construction, but accepts any natsRequester so the
// Stage 15 multiReplyFakeRequester (which differs from fakeRequester
// in shape but not in interface) can drive a Component without
// reaching for a workaround. Kept here rather than in systems_test.go
// to keep the Stage 15 dependency footprint local.
func newComponentWithRequester(t *testing.T, req natsRequester) *Component {
	t.Helper()
	cfg := DefaultConfig()
	cfg.QueryTimeout = 500 * time.Millisecond
	c, err := New(cfg, req, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

// systemStateWithPosition produces the graph.query.entity reply for
// an entity carrying optional position + label triples. position is
// the raw GeoJSON-shaped JSON bytes — pass `""` to omit. Helper is
// dedicated to Stage 15 GeoJSON-path tests; the existing
// encodeEntityState (systems_test.go:393) takes a pre-built state.
func systemStateWithPosition(t *testing.T, id, label, positionJSON string) []byte {
	t.Helper()
	state := graph.EntityState{
		ID: id,
		Triples: []message.Triple{
			{Predicate: "sensorml.process.type", Object: "http://www.w3.org/ns/ssn/System"},
			{Predicate: "sensorml.process.label", Object: label},
		},
	}
	if positionJSON != "" {
		state.Triples = append(state.Triples, message.Triple{
			Predicate: PredSystemPosition,
			Object:    positionJSON,
		})
	}
	b, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("systemStateWithPosition: %v", err)
	}
	return b
}

// TestHandleSystems_GeoJSONFeatureCollection — Accept geo+json
// triggers the FeatureCollection path. Each system with a
// cs-api.system.position triple gets a real Point geometry; those
// without get a null geometry. RFC 7946 §3.2 explicitly permits
// null geometry on a Feature, so this is conformant.
func TestHandleSystems_GeoJSONFeatureCollection(t *testing.T) {
	id1 := "c360.semconnect.systems.csapi.system.with-pos"
	id2 := "c360.semconnect.systems.csapi.system.no-pos"
	fake := &multiReplyFakeRequester{
		predicateReply: encodeReply(t, []string{id1, id2}),
		entityRepliesByID: map[string][]byte{
			id1: systemStateWithPosition(t, id1, "Has position",
				`{"type":"Point","coordinates":[-122.4,37.8,10]}`),
			id2: systemStateWithPosition(t, id2, "No position", ""),
		},
	}
	c := newComponentWithRequester(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/systems", nil)
	req.Header.Set("Accept", "application/geo+json")
	rr := httptest.NewRecorder()
	c.handleSystems(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); ct != string(MediaGeoJSON) {
		t.Errorf("Content-Type: got %q want %q", ct, MediaGeoJSON)
	}

	fc, err := geojson.UnmarshalFeatureCollection(rr.Body.Bytes())
	if err != nil {
		t.Fatalf("decode FeatureCollection: %v; body=%s", err, rr.Body.String())
	}
	if len(fc.Features) != 2 {
		t.Fatalf("features: got %d want 2", len(fc.Features))
	}

	// First feature: has Point geometry from the position triple.
	pt, ok := fc.Features[0].Geometry.(geojson.Point)
	if !ok {
		t.Errorf("feature[0].Geometry: got %T want geojson.Point", fc.Features[0].Geometry)
	} else if len(pt.Coordinates) < 2 || pt.Coordinates[0] != -122.4 || pt.Coordinates[1] != 37.8 {
		t.Errorf("feature[0].Geometry.Coordinates: got %v want [-122.4 37.8 ...]", pt.Coordinates)
	}
	// Properties carry the system's reconstructed fields.
	if label, _ := fc.Features[0].Properties["label"].(string); label != "Has position" {
		t.Errorf("feature[0].Properties.label: got %q want %q", label, "Has position")
	}

	// Second feature: null geometry (no position triple).
	if fc.Features[1].Geometry != nil {
		t.Errorf("feature[1].Geometry: got %+v want nil (no position triple)", fc.Features[1].Geometry)
	}

	// N+1 request shape: 1 predicate query + 2 entity queries.
	if fake.calls != 3 {
		t.Errorf("requests: got %d want 3 (1 predicate + 2 entity for N+1)", fake.calls)
	}
}

// TestHandleSystems_GeoJSONEmptyCollection — empty system list still
// returns 200 + empty FeatureCollection (not 404, not 503).
func TestHandleSystems_GeoJSONEmptyCollection(t *testing.T) {
	fake := &multiReplyFakeRequester{
		predicateReply: encodeReply(t, nil),
	}
	c := newComponentWithRequester(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/systems", nil)
	req.Header.Set("Accept", "application/geo+json")
	rr := httptest.NewRecorder()
	c.handleSystems(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", rr.Code)
	}
	fc, err := geojson.UnmarshalFeatureCollection(rr.Body.Bytes())
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(fc.Features) != 0 {
		t.Errorf("features: got %d want 0", len(fc.Features))
	}
}

// TestHandleSystems_JSONUnchangedAfterStage15 — Accept JSON (or
// no Accept) still returns the CS API SystemCollection wrapper.
// Regression guard against the GeoJSON branch swallowing the JSON
// path.
func TestHandleSystems_JSONUnchangedAfterStage15(t *testing.T) {
	fake := &multiReplyFakeRequester{
		predicateReply: encodeReply(t, []string{"acme.x", "acme.y"}),
	}
	c := newComponentWithRequester(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/systems", nil)
	// No Accept header → family default (MediaJSON).
	rr := httptest.NewRecorder()
	c.handleSystems(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != string(MediaJSON) {
		t.Errorf("Content-Type: got %q want %q", ct, MediaJSON)
	}
	var coll systemCollection
	if err := json.Unmarshal(rr.Body.Bytes(), &coll); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if coll.Type != "SystemCollection" || len(coll.Items) != 2 {
		t.Errorf("collection shape: %+v", coll)
	}
	// JSON path does NOT do the per-entity entity-query — only one
	// request (the predicate query) reaches the backend.
	if fake.calls != 1 {
		t.Errorf("requests: got %d want 1 (JSON path is NOT N+1)", fake.calls)
	}
}

// TestHandleSystems_GeoJSONFirstEntityTransientFails503 — when the
// FIRST entity-query fails transiently, the whole request 503s
// (subsequent entities would fail identically — no point degrading).
func TestHandleSystems_GeoJSONFirstEntityTransientFails503(t *testing.T) {
	id1 := "c360.semconnect.systems.csapi.system.first"
	id2 := "c360.semconnect.systems.csapi.system.second"
	fake := &multiReplyFakeRequester{
		predicateReply: encodeReply(t, []string{id1, id2}),
		entityErrorsByID: map[string]error{
			id1: nats.ErrNoResponders,
			id2: nats.ErrNoResponders,
		},
	}
	c := newComponentWithRequester(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/systems", nil)
	req.Header.Set("Accept", "application/geo+json")
	rr := httptest.NewRecorder()
	c.handleSystems(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("status: got %d want 503; body=%s", rr.Code, rr.Body.String())
	}
}

// TestHandleSystems_GeoJSONLaterEntityFailDegradesToNullGeometry —
// a per-entity backend error AFTER the first successful fetch logs
// the failure and emits a null-geometry Feature. The page is partial
// but the request still 200s — one bad row doesn't poison the page.
func TestHandleSystems_GeoJSONLaterEntityFailDegradesToNullGeometry(t *testing.T) {
	id1 := "c360.semconnect.systems.csapi.system.ok"
	id2 := "c360.semconnect.systems.csapi.system.broken"
	fake := &multiReplyFakeRequester{
		predicateReply: encodeReply(t, []string{id1, id2}),
		entityRepliesByID: map[string][]byte{
			id1: systemStateWithPosition(t, id1, "OK system",
				`{"type":"Point","coordinates":[0,0]}`),
		},
		entityErrorsByID: map[string]error{
			id2: nats.ErrTimeout,
		},
	}
	c := newComponentWithRequester(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/systems", nil)
	req.Header.Set("Accept", "application/geo+json")
	rr := httptest.NewRecorder()
	c.handleSystems(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200 (partial degrades, not 503)", rr.Code)
	}
	fc, err := geojson.UnmarshalFeatureCollection(rr.Body.Bytes())
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(fc.Features) != 2 {
		t.Fatalf("features: got %d want 2 (broken entity still represented with null geom)", len(fc.Features))
	}
	if fc.Features[0].Geometry == nil {
		t.Error("feature[0].Geometry: got nil want Point (this one fetched OK)")
	}
	if fc.Features[1].Geometry != nil {
		t.Errorf("feature[1].Geometry: got %+v want nil (this one's entity-query failed)", fc.Features[1].Geometry)
	}
}

// TestHandleSystems_GeoJSONMalformedPositionDegrades — a position
// triple containing garbage JSON shouldn't crash; log + null geom.
func TestHandleSystems_GeoJSONMalformedPositionDegrades(t *testing.T) {
	id := "c360.semconnect.systems.csapi.system.bad-pos"
	fake := &multiReplyFakeRequester{
		predicateReply: encodeReply(t, []string{id}),
		entityRepliesByID: map[string][]byte{
			id: systemStateWithPosition(t, id, "Bad position", `{"not": "geojson"}`),
		},
	}
	c := newComponentWithRequester(t, fake)

	req := httptest.NewRequest(http.MethodGet, "/systems", nil)
	req.Header.Set("Accept", "application/geo+json")
	rr := httptest.NewRecorder()
	c.handleSystems(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200 (malformed position degrades, not 500)", rr.Code)
	}
	fc, err := geojson.UnmarshalFeatureCollection(rr.Body.Bytes())
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(fc.Features) != 1 {
		t.Fatalf("features: got %d want 1", len(fc.Features))
	}
	if fc.Features[0].Geometry != nil {
		t.Errorf("feature[0].Geometry: got %+v want nil (malformed position)", fc.Features[0].Geometry)
	}
}
