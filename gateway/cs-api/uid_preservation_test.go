// Stage 18 — uid preservation tests. Pins the round-trip contract
// the ETS's {sensorMl,geoJson}MediaTypeWriteParsesSystemBodyWhenMutationEnabled
// assertions exercise: POST a body carrying a uid, GET back, find
// that uid on the response via any of `uid` / `uniqueId` /
// `properties.uid`.
package csapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/c360studio/semstreams/graph"
	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/natsclient"
	"github.com/c360studio/semstreams/parser/sensorml"
	"github.com/c360studio/semstreams/vocabulary/sosa"
)

// TestBuildSystemTriplesFromSensorML_EmitsUIDTriple — POST SensorML
// path preserves the submitted uniqueId as the framework uid triple.
func TestBuildSystemTriplesFromSensorML_EmitsUIDTriple(t *testing.T) {
	fake := &fakeRequester{status: natsclient.StatusConnected}
	c := newTestComponent(t, fake)

	body := minimalSensorML("urn:example:dev:stage18", "Stage 18 SensorML")
	_, triples, err := c.buildSystemTriplesFromSensorML(body)
	if err != nil {
		t.Fatalf("buildSystemTriplesFromSensorML: %v", err)
	}
	uid, ok := firstStringObject(triples, PredSystemUID)
	if !ok {
		t.Fatalf("PredSystemUID triple missing from output (triples=%+v)", triples)
	}
	if uid != "urn:example:dev:stage18" {
		t.Errorf("uid: got %q want %q", uid, "urn:example:dev:stage18")
	}
}

// TestBuildSystemTriplesFromSensorML_EmptyUniqueIDOmitsTriple — when
// the SensorML body has no uniqueId, no uid triple is
// emitted (no synthetic fallback that would mislead read-back).
func TestBuildSystemTriplesFromSensorML_EmptyUniqueIDOmitsTriple(t *testing.T) {
	fake := &fakeRequester{status: natsclient.StatusConnected}
	c := newTestComponent(t, fake)

	body := minimalSensorML("", "No-UID System")
	_, triples, err := c.buildSystemTriplesFromSensorML(body)
	if err != nil {
		t.Fatalf("buildSystemTriplesFromSensorML: %v", err)
	}
	if _, ok := firstStringObject(triples, PredSystemUID); ok {
		t.Errorf("PredSystemUID must not be emitted when uniqueId empty (triples=%+v)", triples)
	}
}

// TestBuildSystemTriplesFromFeature_EmitsUIDTriple — POST Feature
// path adds the uid triple from properties.uid.
func TestBuildSystemTriplesFromFeature_EmitsUIDTriple(t *testing.T) {
	fake := &fakeRequester{status: natsclient.StatusConnected}
	c := newTestComponent(t, fake)

	body := []byte(`{"type":"Feature","properties":{"uid":"urn:ets:feat:abc","name":"Feature ABC"}}`)
	_, triples, err := c.buildSystemTriplesFromFeature(body)
	if err != nil {
		t.Fatalf("buildSystemTriplesFromFeature: %v", err)
	}
	uid, ok := firstStringObject(triples, PredSystemUID)
	if !ok {
		t.Fatalf("PredSystemUID triple missing (triples=%+v)", triples)
	}
	if uid != "urn:ets:feat:abc" {
		t.Errorf("uid: got %q want %q", uid, "urn:ets:feat:abc")
	}
}

// TestSystemFromState_SurfacesUIDOnAllThreeFields — JSON response
// echoes the preserved uid on top-level `uid`, top-level `uniqueId`,
// AND nested `properties.uid`. The ETS checks any of the three;
// belt-and-suspenders so a Feature client and a SensorML client
// each find the spelling they expect on the same response.
func TestSystemFromState_SurfacesUIDOnAllThreeFields(t *testing.T) {
	const id = "acme.ops.robotics.gcs.drone.018"
	const uid = "urn:example:stage18:roundtrip"
	state := graph.EntityState{
		ID: id,
		Triples: []message.Triple{
			{Subject: id, Predicate: sensorml.PredType, Object: sosa.SSNSystem},
			{Subject: id, Predicate: sensorml.PredLabel, Object: "Stage 18 Drone"},
			{Subject: id, Predicate: PredSystemUID, Object: uid},
		},
	}
	s := systemFromState(state)
	if s.UID != uid {
		t.Errorf("top-level uid: got %q want %q", s.UID, uid)
	}
	if s.UniqueID != uid {
		t.Errorf("top-level uniqueId: got %q want %q", s.UniqueID, uid)
	}
	if s.FeatureProperties == nil || s.FeatureProperties.UID != uid {
		t.Errorf("properties.uid: got %+v want uid=%q", s.FeatureProperties, uid)
	}
	// Stage 19 — featureProperties.{Name,Description} re-added after
	// the ETS UpdateTests.systemsPatchLifecycleOptIn surfaced that
	// properties.name is checked on GET-after-PATCH. Derived from
	// top-level Label/Description (single-source — no separate
	// triples).
	if s.FeatureProperties.Name != "Stage 18 Drone" {
		t.Errorf("properties.name: got %q want %q", s.FeatureProperties.Name, "Stage 18 Drone")
	}
	if s.Label != "Stage 18 Drone" {
		t.Errorf("top-level label: got %q want %q", s.Label, "Stage 18 Drone")
	}
}

func TestSystemFromState_SurfacesLegacyUIDPredicate(t *testing.T) {
	const id = "acme.ops.robotics.gcs.drone.legacy"
	const uid = "urn:example:legacy:uid"
	state := graph.EntityState{
		ID: id,
		Triples: []message.Triple{
			{Subject: id, Predicate: sensorml.PredType, Object: sosa.SSNSystem},
			{Subject: id, Predicate: legacyPredSystemUID, Object: uid},
		},
	}
	s := systemFromState(state)
	if s.UID != uid || s.UniqueID != uid || s.FeatureProperties == nil || s.FeatureProperties.UID != uid {
		t.Errorf("legacy uid fallback: got %+v want uid %q on all read fields", s, uid)
	}
}

// TestSystemFromState_NoUIDTriple_OmitsAllUIDFields — absent
// preservation triple = absent fields on the response. We do NOT
// fall back to the entity ID (would be misleading — the entity ID
// is a different shape from the client's uid).
func TestSystemFromState_NoUIDTriple_OmitsAllUIDFields(t *testing.T) {
	const id = "acme.ops.robotics.gcs.drone.019"
	state := graph.EntityState{
		ID: id,
		Triples: []message.Triple{
			{Subject: id, Predicate: sensorml.PredType, Object: sosa.SSNSystem},
			{Subject: id, Predicate: sensorml.PredLabel, Object: "No UID"},
		},
	}
	s := systemFromState(state)
	if s.UID != "" {
		t.Errorf("uid: got %q want empty", s.UID)
	}
	if s.UniqueID != "" {
		t.Errorf("uniqueId: got %q want empty", s.UniqueID)
	}
	if s.FeatureProperties != nil {
		t.Errorf("properties: got %+v want nil", s.FeatureProperties)
	}
}

// TestSystemFromState_JSONSerialization_SurfacesUID — uid presence
// must show up on the marshalled JSON bytes (not just on the Go
// struct). Pins the wire format so a future framework change to
// AbstractProcess marshaling can't silently drop the field.
func TestSystemFromState_JSONSerialization_SurfacesUID(t *testing.T) {
	const id = "acme.ops.robotics.gcs.drone.022"
	const uid = "urn:example:stage18:wire"
	state := graph.EntityState{
		ID: id,
		Triples: []message.Triple{
			{Subject: id, Predicate: sensorml.PredType, Object: sosa.SSNSystem},
			{Subject: id, Predicate: PredSystemUID, Object: uid},
		},
	}
	out, err := json.Marshal(systemFromState(state))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// No label triple → properties contains uid only (Name omitempty).
	for _, want := range []string{
		`"uid":"` + uid + `"`,
		`"uniqueId":"` + uid + `"`,
		`"properties":{"uid":"` + uid + `"}`,
	} {
		if !bytes.Contains(out, []byte(want)) {
			t.Errorf("body should contain %s; got %s", want, string(out))
		}
	}
}

// TestSystemFromState_JSONSerialization_PropertiesNameSurfaces — Stage 19
// regression guard. The ETS UpdateTests.systemsPatchLifecycleOptIn GETs
// after PATCH and asserts properties.name carries the new value.
// Pin that name reaches the wire body via the featureProperties
// container (not just top-level label).
func TestSystemFromState_JSONSerialization_PropertiesNameSurfaces(t *testing.T) {
	const id = "acme.ops.robotics.gcs.drone.023"
	const uid = "urn:example:stage19:patch-target"
	state := graph.EntityState{
		ID: id,
		Triples: []message.Triple{
			{Subject: id, Predicate: sensorml.PredType, Object: sosa.SSNSystem},
			{Subject: id, Predicate: sensorml.PredLabel, Object: "Patched Name"},
			{Subject: id, Predicate: sensorml.PredDescription, Object: "Patched Desc"},
			{Subject: id, Predicate: PredSystemUID, Object: uid},
		},
	}
	out, err := json.Marshal(systemFromState(state))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, want := range []string{
		`"properties":{`,
		`"uid":"` + uid + `"`,
		`"name":"Patched Name"`,
		`"description":"Patched Desc"`,
	} {
		if !bytes.Contains(out, []byte(want)) {
			t.Errorf("body should contain %s; got %s", want, string(out))
		}
	}
}

// TestSystemFromState_JSONSerialization_OmitsEmpty — empty UID
// fields must not appear in the JSON body (omitempty discipline)
// so a Feature client doesn't see a misleading `uid: ""`.
func TestSystemFromState_JSONSerialization_OmitsEmpty(t *testing.T) {
	const id = "acme.ops.robotics.gcs.drone.020"
	state := graph.EntityState{
		ID:      id,
		Triples: []message.Triple{{Subject: id, Predicate: sensorml.PredType, Object: sosa.SSNSystem}},
	}
	out, err := json.Marshal(systemFromState(state))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	body := string(out)
	for _, tok := range []string{`"uid":`, `"uniqueId":`, `"properties":`} {
		if bytes.Contains(out, []byte(tok)) {
			t.Errorf("body should not contain %q; got %s", tok, body)
		}
	}
}

// TestSensorMLReverseMapping_SurfacesUniqueID — GET /systems/{id}
// with Accept: sml+json puts the preserved uid into the SensorML
// Process's UniqueID field AND that field survives JSON marshaling
// (the SensorML-native spelling the ETS reaches for via .uniqueId).
// Pins both the struct field AND the wire format so a future
// framework-side AbstractProcess marshaler change can't drop the
// field silently.
func TestSensorMLReverseMapping_SurfacesUniqueID(t *testing.T) {
	const id = "acme.ops.robotics.gcs.drone.021"
	const uid = "urn:example:stage18:smlroundtrip"
	state := graph.EntityState{
		ID: id,
		Triples: []message.Triple{
			{Subject: id, Predicate: sensorml.PredType, Object: sosa.SSNSystem},
			{Subject: id, Predicate: sensorml.PredLabel, Object: "SML test"},
			{Subject: id, Predicate: PredSystemUID, Object: uid},
		},
	}
	process, err := reconstructProcessFromTriples(state.Triples, id)
	if err != nil {
		t.Fatalf("reconstructProcessFromTriples: %v", err)
	}
	if process.Base().UniqueID != uid {
		t.Errorf("UniqueID: got %q want %q", process.Base().UniqueID, uid)
	}
	out, err := json.Marshal(process)
	if err != nil {
		t.Fatalf("marshal sml process: %v", err)
	}
	if want := `"uniqueId":"` + uid + `"`; !bytes.Contains(out, []byte(want)) {
		t.Errorf("sml+json wire body should contain %s; got %s", want, string(out))
	}
}

// TestHandleSystemPost_JSONFeature_UIDRoundsTripToTriples is the
// integration-style check: POST a Feature with properties.uid,
// confirm the publish payload to graph.mutation.triple.add_batch
// contains the framework uid triple.
func TestHandleSystemPost_JSONFeature_UIDRoundsTripToTriples(t *testing.T) {
	fake := &fakeRequester{
		status: natsclient.StatusConnected,
		reply:  encodeBatchOK(t, 3),
	}
	c := newTestComponent(t, fake)

	body := []byte(`{"type":"Feature","properties":{"uid":"urn:ets:roundtrip:42","name":"Round-trip 42"}}`)
	req := httptest.NewRequest(http.MethodPost, "/systems", bytes.NewReader(body))
	req.Header.Set("Content-Type", string(MediaJSON))
	rr := httptest.NewRecorder()
	c.handleSystemPost(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status: got %d want 201; body=%s", rr.Code, rr.Body.String())
	}

	// Decode the batch the gateway published — find a PredSystemUID
	// triple with the submitted uid.
	var batch graph.AddTriplesBatchRequest
	if err := json.Unmarshal(fake.gotBody, &batch); err != nil {
		t.Fatalf("decode batch body: %v (body=%s)", err, string(fake.gotBody))
	}
	var found bool
	for _, tr := range batch.Triples {
		if tr.Predicate == PredSystemUID && tr.Object == "urn:ets:roundtrip:42" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("PredSystemUID triple missing from publish batch (triples=%+v)", batch.Triples)
	}
}
