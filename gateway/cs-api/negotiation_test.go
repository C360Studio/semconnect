package csapi

import (
	"net/http/httptest"
	"testing"
)

func TestNegotiate(t *testing.T) {
	// Per-stage wiring lives in negotiation.go supported() — these tests
	// grow as encoders ship per stage. Stage 4: FamilySystemItem includes
	// JSON / SensorML / JSON-LD; FamilySystemCollection is JSON-only;
	// Stage 11: FamilyObservationCollection adds OMS (the read-side
	// shape mirrors the POST-side); FamilySpatial wired at Stage 5.
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
		{"GeoJSON wired on FamilySystemCollection at Stage 15", "application/geo+json", FamilySystemCollection, MediaGeoJSON, true},
		{"explicit JSON on system item", "application/json", FamilySystemItem, MediaJSON, true},
		{"wildcard subtype resolves to family default", "application/*", FamilySystemItem, MediaJSON, true},
		{"global wildcard resolves to family default", "*/*", FamilyObservationCollection, MediaJSON, true},
		// Stage 14: MediaSensorML is `application/sml+json` (spec form per
		// CS API §11.7 + Botts ETS). `application/sensorml+json` is the
		// long-form legacy alias kept for backward compat.
		{"SensorML spec form (sml+json) wired at Stage 14", "application/sml+json", FamilySystemItem, MediaSensorML, true},
		{"SensorML legacy long form still wired", "application/sensorml+json", FamilySystemItem, MediaSensorMLLegacy, true},
		{"SensorML NOT wired on collection — 406 (sml+json)", "application/sml+json", FamilySystemCollection, "", false},
		{"SensorML NOT wired on collection — 406 (long form)", "application/sensorml+json", FamilySystemCollection, "", false},
		{"JSON-LD wired at Stage 4 for system items", "application/ld+json", FamilySystemItem, MediaJSONLD, true},
		{"JSON-LD NOT wired on collection — 406", "application/ld+json", FamilySystemCollection, "", false},
		{"JSON-LD wired at Stage 4 for /conformance", "application/ld+json", FamilyService, MediaJSONLD, true},
		{"OMS wired on FamilyObservationCollection at Stage 11", "application/om+json", FamilyObservationCollection, MediaOMS, true},
		{"GeoJSON Accept on system item still 406 (item is JSON/SensorML/JSON-LD)", "application/geo+json", FamilySystemItem, "", false},
		{"unsupported only → 406", "application/xml, text/html", FamilySystemItem, "", false},
		{"XML out of scope even when listed first", "application/xml", FamilyObservationCollection, "", false},
		{"comma-separated with whitespace picks JSON", " application/json , application/xml ", FamilySystemItem, MediaJSON, true},
		{"q-weighted preference: SensorML over JSON when client weights it higher", "application/json;q=0.5, application/sml+json;q=0.9", FamilySystemItem, MediaSensorML, true},
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

func TestNegotiateRequest_FParameterOverridesAccept(t *testing.T) {
	// Stage 7 wires OGC API Common Part 1 §7 (req/json/content): the `?f=`
	// query parameter overrides Accept when present. Unknown short names and
	// names not in fam.supported() 406 instead of silently falling through —
	// the override is a deliberate client signal.
	tests := []struct {
		name      string
		url       string
		accept    string
		fam       ResourceFamily
		want      MediaType
		wantOK    bool
	}{
		{"f=json picks JSON regardless of Accept",
			"/systems?f=json", "application/sensorml+json", FamilySystemItem, MediaJSON, true},
		{"f=sensorml on system item",
			"/systems/x?f=sensorml", "", FamilySystemItem, MediaSensorML, true},
		{"f=jsonld on system item",
			"/systems/x?f=jsonld", "", FamilySystemItem, MediaJSONLD, true},
		{"f=geojson on spatial",
			"/areas?f=geojson&bbox=0,0,1,1", "application/json", FamilySpatial, MediaGeoJSON, true},
		{"f=json on spatial",
			"/areas?f=json", "", FamilySpatial, MediaJSON, true},
		{"f=json on service family (landing/conformance)",
			"/?f=json", "application/ld+json", FamilyService, MediaJSON, true},
		{"f=sensorml on collection — 406 (no SystemCollection SensorML type)",
			"/systems?f=sensorml", "", FamilySystemCollection, "", false},
		{"f=geojson on system item — 406 (item is JSON/SensorML/JSON-LD)",
			"/systems/x?f=geojson", "", FamilySystemItem, "", false},
		{"f=html (not implemented) — 406 not silent passthrough",
			"/systems?f=html", "application/json", FamilySystemItem, "", false},
		{"f=garbage — 406 not silent passthrough",
			"/systems?f=zzz", "application/json", FamilySystemItem, "", false},
		{"empty f= falls back to Accept (legacy long form)",
			"/systems?f=", "application/sensorml+json", FamilySystemItem, MediaSensorMLLegacy, true},
		{"absent f= falls back to Accept (legacy long form)",
			"/systems", "application/sensorml+json", FamilySystemItem, MediaSensorMLLegacy, true},
		{"f= case-insensitive",
			"/systems?f=JSON", "", FamilySystemItem, MediaJSON, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", tt.url, nil)
			if tt.accept != "" {
				r.Header.Set("Accept", tt.accept)
			}
			got, ok := NegotiateRequest(r, tt.fam)
			if ok != tt.wantOK {
				t.Fatalf("ok=%v want %v (got=%q)", ok, tt.wantOK, got)
			}
			if got != tt.want {
				t.Errorf("got %q want %q", got, tt.want)
			}
		})
	}
}
