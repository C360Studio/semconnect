package csapi

import (
	"encoding/json"
	"testing"

	"github.com/c360studio/semstreams/message"
	"github.com/c360studio/semstreams/vocabulary"
)

func TestLocationBuildersProjectPointToPublicGeoPredicates(t *testing.T) {
	c := newTestComponent(t, &fakeRequester{})
	feature := []byte(`{"type":"Feature","geometry":{"type":"Point","coordinates":[-122.4,37.8,10]},"properties":{"uid":"urn:example:located"}}`)
	sensorML := []byte(`{"type":"PhysicalSystem","uniqueId":"urn:example:located-sml","position":{"type":"Point","coordinates":[-122.4,37.8,10]}}`)

	tests := []struct {
		name  string
		build func() ([]message.Triple, error)
	}{
		{name: "system Feature", build: func() ([]message.Triple, error) {
			_, triples, err := c.buildSystemTriplesFromFeature(feature)
			return triples, err
		}},
		{name: "system SensorML", build: func() ([]message.Triple, error) {
			_, triples, err := c.buildSystemTriplesFromSensorML(sensorML)
			return triples, err
		}},
		{name: "deployment Feature", build: func() ([]message.Triple, error) {
			_, triples, err := c.buildDeploymentTriplesFromFeature(feature)
			return triples, err
		}},
		{name: "sampling feature", build: func() ([]message.Triple, error) {
			_, triples, err := c.buildSamplingFeatureTriplesFromFeature(feature)
			return triples, err
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			triples, err := tt.build()
			if err != nil {
				t.Fatalf("build: %v", err)
			}
			assertGeoProjection(t, triples, -122.4, 37.8, 10)
			if _, ok := tripleObject(triples, PredSystemPosition); !ok {
				t.Fatal("round-trip position triple missing")
			}
		})
	}
}

func TestLocationBuildersDoNotInventPointForNonPointGeometry(t *testing.T) {
	c := newTestComponent(t, &fakeRequester{})
	body := []byte(`{"type":"Feature","geometry":{"type":"Polygon","coordinates":[[[0,0],[1,0],[1,1],[0,0]]]},"properties":{"uid":"urn:example:area"}}`)
	_, triples, err := c.buildSystemTriplesFromFeature(body)
	if err != nil {
		t.Fatalf("buildSystemTriplesFromFeature: %v", err)
	}
	if _, ok := tripleObject(triples, PredSystemPosition); !ok {
		t.Fatal("non-Point geometry was not preserved for round-trip")
	}
	for _, predicate := range []string{
		vocabulary.GeoLocationLatitude,
		vocabulary.GeoLocationLongitude,
		vocabulary.GeoLocationAltitude,
	} {
		if _, ok := tripleObject(triples, predicate); ok {
			t.Errorf("non-Point geometry emitted %q", predicate)
		}
	}
}

func TestMergePatchSystemTriplesReplacesSpatialProjectionWithoutStaleCoordinates(t *testing.T) {
	const entityID = "acme.ops.robotics.gcs.system.located"
	existing := []message.Triple{
		{Subject: entityID, Predicate: PredSystemPosition, Object: `{"type":"Point","coordinates":[10,20,30]}`},
		{Subject: entityID, Predicate: vocabulary.GeoLocationLongitude, Object: 10.0},
		{Subject: entityID, Predicate: vocabulary.GeoLocationLatitude, Object: 20.0},
		{Subject: entityID, Predicate: vocabulary.GeoLocationAltitude, Object: 30.0},
	}
	var pointPatch systemFeatureBody
	if err := json.Unmarshal([]byte(`{"geometry":{"type":"Point","coordinates":[40,50,60]}}`), &pointPatch); err != nil {
		t.Fatal(err)
	}
	merged := mergePatchSystemTriples(entityID, existing, pointPatch)
	assertGeoProjection(t, merged, 40, 50, 60)
	assertPredicateCount(t, merged, vocabulary.GeoLocationLongitude, 1)
	assertPredicateCount(t, merged, vocabulary.GeoLocationLatitude, 1)
	assertPredicateCount(t, merged, vocabulary.GeoLocationAltitude, 1)

	var polygonPatch systemFeatureBody
	if err := json.Unmarshal([]byte(`{"geometry":{"type":"Polygon","coordinates":[[[0,0],[1,0],[1,1],[0,0]]]}}`), &polygonPatch); err != nil {
		t.Fatal(err)
	}
	merged = mergePatchSystemTriples(entityID, merged, polygonPatch)
	for _, predicate := range []string{
		vocabulary.GeoLocationLatitude,
		vocabulary.GeoLocationLongitude,
		vocabulary.GeoLocationAltitude,
	} {
		if _, ok := tripleObject(merged, predicate); ok {
			t.Errorf("non-Point replacement retained stale %q", predicate)
		}
	}
}

func assertGeoProjection(t *testing.T, triples []message.Triple, wantLon, wantLat, wantAlt float64) {
	t.Helper()
	for predicate, want := range map[string]float64{
		vocabulary.GeoLocationLongitude: wantLon,
		vocabulary.GeoLocationLatitude:  wantLat,
		vocabulary.GeoLocationAltitude:  wantAlt,
	} {
		got, ok := tripleObject(triples, predicate)
		if !ok {
			t.Errorf("missing %q", predicate)
			continue
		}
		if got != want {
			t.Errorf("%s object: got %#v want %v", predicate, got, want)
		}
	}
}

func tripleObject(triples []message.Triple, predicate string) (any, bool) {
	for _, triple := range triples {
		if triple.Predicate == predicate {
			return triple.Object, true
		}
	}
	return nil, false
}

func assertPredicateCount(t *testing.T, triples []message.Triple, predicate string, want int) {
	t.Helper()
	got := 0
	for _, triple := range triples {
		if triple.Predicate == predicate {
			got++
		}
	}
	if got != want {
		t.Errorf("%s count: got %d want %d", predicate, got, want)
	}
}
