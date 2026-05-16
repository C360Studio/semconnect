package csapi

import "testing"

func TestNegotiate(t *testing.T) {
	// Stage 2 wires only the JSON encoder. Once Stages 3–5 land their
	// encoders the supported set widens — these tests grow with it.
	tests := []struct {
		name   string
		accept string
		fam    ResourceFamily
		want   MediaType
		wantOK bool
	}{
		{"empty Accept on systems → JSON default", "", FamilySystem, MediaJSON, true},
		{"empty Accept on spatial → JSON default (GeoJSON deferred to Stage 5)", "", FamilySpatial, MediaJSON, true},
		{"explicit JSON on systems", "application/json", FamilySystem, MediaJSON, true},
		{"wildcard subtype resolves to family default", "application/*", FamilySystem, MediaJSON, true},
		{"global wildcard resolves to family default", "*/*", FamilyObservation, MediaJSON, true},
		{"SensorML not wired yet → 406", "application/sensorml+json", FamilySystem, "", false},
		{"OMS not wired yet → 406", "application/om+json", FamilyObservation, "", false},
		{"GeoJSON not wired yet → 406", "application/geo+json", FamilySpatial, "", false},
		{"JSON-LD not wired yet → 406", "application/ld+json", FamilySystem, "", false},
		{"unsupported only → 406", "application/xml, text/html", FamilySystem, "", false},
		{"XML out of scope even when listed first", "application/xml", FamilyObservation, "", false},
		{"comma-separated with whitespace picks JSON", " application/json , application/xml ", FamilySystem, MediaJSON, true},
		{"q-weighted preference: JSON wins when only it is supported", "application/xml;q=0.9, application/json;q=0.5", FamilySystem, MediaJSON, true},
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
