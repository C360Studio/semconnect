package csapi

import "testing"

func TestNegotiate(t *testing.T) {
	// Per-stage wiring lives in negotiation.go supported() — these tests
	// grow as encoders ship per stage. Stage 4: FamilySystemItem includes
	// JSON / SensorML / JSON-LD; FamilySystemCollection is JSON-only;
	// FamilyObservation / FamilySpatial still JSON-only until Stages 3.5 / 5.
	tests := []struct {
		name   string
		accept string
		fam    ResourceFamily
		want   MediaType
		wantOK bool
	}{
		{"empty Accept on system item → JSON default", "", FamilySystemItem, MediaJSON, true},
		{"empty Accept on system collection → JSON default", "", FamilySystemCollection, MediaJSON, true},
		{"empty Accept on spatial → GeoJSON default (Stage 5)", "", FamilySpatial, MediaGeoJSON, true},
		{"explicit JSON on spatial → JSON", "application/json", FamilySpatial, MediaJSON, true},
		{"GeoJSON wired at Stage 5", "application/geo+json", FamilySpatial, MediaGeoJSON, true},
		{"explicit JSON on system item", "application/json", FamilySystemItem, MediaJSON, true},
		{"wildcard subtype resolves to family default", "application/*", FamilySystemItem, MediaJSON, true},
		{"global wildcard resolves to family default", "*/*", FamilyObservation, MediaJSON, true},
		{"SensorML wired at Stage 4 (item)", "application/sensorml+json", FamilySystemItem, MediaSensorML, true},
		{"SensorML NOT wired on collection — 406", "application/sensorml+json", FamilySystemCollection, "", false},
		{"JSON-LD wired at Stage 4 for system items", "application/ld+json", FamilySystemItem, MediaJSONLD, true},
		{"JSON-LD NOT wired on collection — 406", "application/ld+json", FamilySystemCollection, "", false},
		{"JSON-LD wired at Stage 4 for /conformance", "application/ld+json", FamilyService, MediaJSONLD, true},
		{"OMS not wired on FamilyObservation responses yet → 406", "application/om+json", FamilyObservation, "", false},
		{"GeoJSON Accept on system item still 406 (item is JSON/SensorML/JSON-LD)", "application/geo+json", FamilySystemItem, "", false},
		{"unsupported only → 406", "application/xml, text/html", FamilySystemItem, "", false},
		{"XML out of scope even when listed first", "application/xml", FamilyObservation, "", false},
		{"comma-separated with whitespace picks JSON", " application/json , application/xml ", FamilySystemItem, MediaJSON, true},
		{"q-weighted preference: SensorML over JSON when client weights it higher", "application/json;q=0.5, application/sensorml+json;q=0.9", FamilySystemItem, MediaSensorML, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := Negotiate(tt.accept, tt.fam)
			if ok != tt.wantOK {
				t.Fatalf("ok=%v want %v (got=%q)", ok, tt.wantOK, got)
			}
			if got != tt.want {
				t.Errorf("got %q want %q", got, tt.want)
			}
		})
	}
}
