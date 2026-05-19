package csapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// TestHandleAPI_DefaultJSON — Accept absent / wildcard returns the OAS3
// document as application/vnd.oai.openapi+json;version=3.0 by default
// (the FamilyAPI ordering puts OAS3 JSON first).
func TestHandleAPI_DefaultJSON(t *testing.T) {
	fake := &fakeRequester{}
	c := newTestComponent(t, fake)
	req := httptest.NewRequest(http.MethodGet, "/api", nil)
	rr := httptest.NewRecorder()
	c.handleAPI(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); ct != string(MediaOAS3JSON) {
		t.Errorf("Content-Type: got %q want %q", ct, string(MediaOAS3JSON))
	}
	// Body must be valid JSON; the OAS3 root has openapi + info + paths.
	var doc map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &doc); err != nil {
		t.Fatalf("body not valid JSON: %v", err)
	}
	if v, _ := doc["openapi"].(string); !strings.HasPrefix(v, "3.") {
		t.Errorf("openapi version: got %q want 3.x", v)
	}
	if _, ok := doc["paths"].(map[string]any); !ok {
		t.Error("missing paths object")
	}
}

// TestHandleAPI_YAMLAlternate — Accept the OAS3 YAML form returns the
// raw embedded YAML.
func TestHandleAPI_YAMLAlternate(t *testing.T) {
	fake := &fakeRequester{}
	c := newTestComponent(t, fake)
	req := httptest.NewRequest(http.MethodGet, "/api", nil)
	req.Header.Set("Accept", string(MediaOAS3YAML))
	rr := httptest.NewRecorder()
	c.handleAPI(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != string(MediaOAS3YAML) {
		t.Errorf("Content-Type: got %q want %q", ct, string(MediaOAS3YAML))
	}
	// Body must be valid YAML and the root must contain `openapi: 3.x`.
	var doc map[string]any
	if err := yaml.Unmarshal(rr.Body.Bytes(), &doc); err != nil {
		t.Fatalf("body not valid YAML: %v", err)
	}
	if v, _ := doc["openapi"].(string); !strings.HasPrefix(v, "3.") {
		t.Errorf("openapi version: got %q want 3.x", v)
	}
}

// TestHandleAPI_FShortNames — ?f=yaml and ?f=openapi work per OGC Common
// Part 1 §7 short-name override.
func TestHandleAPI_FShortNames(t *testing.T) {
	for _, tc := range []struct {
		name      string
		query     string
		wantMedia MediaType
	}{
		{"?f=yaml", "?f=yaml", MediaOAS3YAML},
		{"?f=openapi", "?f=openapi", MediaOAS3JSON},
		{"?f=json (CS-API JSON alias)", "?f=json", MediaJSON},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeRequester{}
			c := newTestComponent(t, fake)
			req := httptest.NewRequest(http.MethodGet, "/api"+tc.query, nil)
			rr := httptest.NewRecorder()
			c.handleAPI(rr, req)
			if rr.Code != http.StatusOK {
				t.Fatalf("status: got %d want 200; body=%s", rr.Code, rr.Body.String())
			}
			if ct := rr.Header().Get("Content-Type"); ct != string(tc.wantMedia) {
				t.Errorf("Content-Type: got %q want %q", ct, string(tc.wantMedia))
			}
		})
	}
}

// TestHandleAPI_NotAcceptable — XML / SensorML / GeoJSON Accept all 406.
// FamilyAPI is {OAS3+json, json, OAS3+yaml} — anything else is rejected.
func TestHandleAPI_NotAcceptable(t *testing.T) {
	fake := &fakeRequester{}
	c := newTestComponent(t, fake)
	for _, accept := range []string{
		"application/xml",
		"application/sensorml+json",
		"application/geo+json",
		"text/html",
	} {
		req := httptest.NewRequest(http.MethodGet, "/api", nil)
		req.Header.Set("Accept", accept)
		rr := httptest.NewRecorder()
		c.handleAPI(rr, req)
		if rr.Code != http.StatusNotAcceptable {
			t.Errorf("Accept %q: got %d want 406", accept, rr.Code)
		}
	}
}

// TestHandleAPI_HEAD — HEAD pins status + Content-Type, no body.
func TestHandleAPI_HEAD(t *testing.T) {
	fake := &fakeRequester{}
	c := newTestComponent(t, fake)
	req := httptest.NewRequest(http.MethodHead, "/api", nil)
	rr := httptest.NewRecorder()
	c.handleAPI(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", rr.Code)
	}
	if rr.Body.Len() != 0 {
		t.Errorf("HEAD body length: got %d want 0", rr.Body.Len())
	}
}

// TestEmbeddedOpenAPI_Shape — the embedded openapi.yaml must declare
// all 10 v0.1-implemented paths AND all 14 schemas we depend on. Catches
// the "moved an endpoint but forgot to update the spec" class of bug
// at compile-test time, before clients see the drift.
func TestEmbeddedOpenAPI_Shape(t *testing.T) {
	var doc map[string]any
	if err := yaml.Unmarshal(openAPIYAML, &doc); err != nil {
		t.Fatalf("embedded openapi.yaml not valid YAML: %v", err)
	}
	paths, _ := doc["paths"].(map[string]any)
	wantImpl := []string{
		"/", "/api", "/conformance", "/health",
		"/systems", "/systems/{id}",
		"/datastreams", "/datastreams/{id}",
		"/datastreams/{datastreamID}/observations",
		"/areas",
		// Stage 20 — /procedures + /procedures/{id} now implemented.
		"/procedures", "/procedures/{id}",
		// Stage 21 — /deployments + /deployments/{id} now implemented.
		"/deployments", "/deployments/{id}",
		// Stage 22 — /samplingFeatures + /samplingFeatures/{id} now implemented.
		"/samplingFeatures", "/samplingFeatures/{id}",
		// Stage 23 — /properties + /properties/{id} now implemented.
		"/properties", "/properties/{id}",
	}
	for _, p := range wantImpl {
		entry, ok := paths[p].(map[string]any)
		if !ok {
			t.Errorf("openapi.yaml missing implemented path: %s", p)
			continue
		}
		// Implemented paths must NOT carry x-not-implemented-at-v01 — a
		// stale annotation here would lie to clients.
		if v, ok := entry["x-not-implemented-at-v01"]; ok {
			t.Errorf("path %s carries x-not-implemented-at-v01=%v but is wired", p, v)
		}
	}
	wantStub := []string{
		"/collections",
		"/controlstreams", "/commands", "/systemEvents",
	}
	for _, p := range wantStub {
		entry, ok := paths[p].(map[string]any)
		if !ok {
			t.Errorf("openapi.yaml missing stub path: %s", p)
			continue
		}
		if v, _ := entry["x-not-implemented-at-v01"].(bool); !v {
			t.Errorf("stub path %s missing x-not-implemented-at-v01: true", p)
		}
	}

	components, _ := doc["components"].(map[string]any)
	schemas, _ := components["schemas"].(map[string]any)
	wantSchemas := []string{
		"LandingPage", "ConformanceDeclaration", "Link",
		"SystemCollection", "SystemRef", "System",
		"DatastreamCollection", "DatastreamRef", "Datastream", "DatastreamCreate",
		"ObservationCollection", "Observation",
		"ProcedureCollection", "ProcedureRef", "Procedure",
		"DeploymentCollection", "DeploymentRef", "Deployment",
		"SamplingFeatureCollection", "SamplingFeatureRef", "SamplingFeature",
		"FeatureCollection", "ErrorResponse",
	}
	for _, s := range wantSchemas {
		if _, ok := schemas[s]; !ok {
			t.Errorf("openapi.yaml missing schema: %s", s)
		}
	}
}
